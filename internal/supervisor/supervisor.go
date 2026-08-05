package supervisor

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/robfig/cron/v3"
	"github.com/zaigie/palworld-server-tool/internal/config"
	"github.com/zaigie/palworld-server-tool/internal/logger"
)

var (
	ErrConflict             = errors.New("server process is already running or busy")
	ErrNotRunning           = errors.New("server process is not running")
	ErrProcessNotConfigured = errors.New("server process management is not configured")
	ErrUnsupportedPlatform  = errors.New("server process management is unsupported on this platform")
	ErrInvalidConfig        = errors.New("invalid server process configuration")
)

type State string

const (
	StateStopped          State = "stopped"
	StateStarting         State = "starting"
	StateRunning          State = "running"
	StateStopping         State = "stopping"
	StateRestartWaiting   State = "restart_waiting"
	StateRestarting       State = "restarting"
	StateUpdating         State = "updating"
	StateUpdateFailed     State = "update_failed"
	StateCrashLoopStopped State = "crash_loop_stopped"
	StateError            State = "error"
)

type ProcessConfig struct {
	Enabled                      bool
	ExecutablePath               string
	WorkingDirectory             string
	Arguments                    []string
	WatchdogEnabled              bool
	ScheduledRestartEnabled      bool
	ScheduledRestartFrequency    string
	ScheduledRestartTime         string
	ScheduledRestartIntervalDays int
	ScheduledRestartStartDate    string
	ScheduledRestartWeekday      int
	ScheduledRestartDayOfMonth   int
	ScheduledRestartCron         string
	SteamCMDPath                 string
	RestartDelay                 time.Duration
	GracefulShutdownSeconds      int
	GracefulShutdownMessage      string
	MaxRestartAttempts           int
	RestartAttemptWindow         time.Duration
}

func ProcessConfigFrom(value config.ServerProcessConfig) ProcessConfig {
	value = config.NormalizeServerProcess(value)
	if value.RestartDelaySeconds < 0 {
		value.RestartDelaySeconds = 0
	}
	if value.MaxRestartAttempts < 1 {
		value.MaxRestartAttempts = 5
	}
	if value.RestartAttemptWindowSeconds < 1 {
		value.RestartAttemptWindowSeconds = 300
	}
	workingDirectory := strings.TrimSpace(value.WorkingDirectory)
	if workingDirectory == "" && value.ExecutablePath != "" {
		workingDirectory = filepath.Dir(value.ExecutablePath)
	}
	return ProcessConfig{
		Enabled:                      value.Enabled,
		ExecutablePath:               value.ExecutablePath,
		WorkingDirectory:             workingDirectory,
		Arguments:                    append([]string(nil), value.Arguments...),
		WatchdogEnabled:              value.WatchdogEnabled,
		ScheduledRestartEnabled:      value.ScheduledRestartEnabled,
		ScheduledRestartFrequency:    value.ScheduledRestartFrequency,
		ScheduledRestartTime:         value.ScheduledRestartTime,
		ScheduledRestartIntervalDays: value.ScheduledRestartIntervalDays,
		ScheduledRestartStartDate:    value.ScheduledRestartStartDate,
		ScheduledRestartWeekday:      value.ScheduledRestartWeekday,
		ScheduledRestartDayOfMonth:   value.ScheduledRestartDayOfMonth,
		ScheduledRestartCron:         value.ScheduledRestartCron,
		SteamCMDPath:                 value.SteamCMDPath,
		RestartDelay:                 time.Duration(value.RestartDelaySeconds) * time.Second,
		GracefulShutdownSeconds:      value.GracefulShutdownSeconds,
		GracefulShutdownMessage:      value.GracefulShutdownMessage,
		MaxRestartAttempts:           value.MaxRestartAttempts,
		RestartAttemptWindow:         time.Duration(value.RestartAttemptWindowSeconds) * time.Second,
	}
}

type ManagedProcess interface {
	PID() int
	Wait() error
	Kill() error
}

type ProcessLauncher interface {
	Start(ctx context.Context, processConfig ProcessConfig) (ManagedProcess, error)
}

type ProcessDetector interface {
	FindPalServer() (int, error)
}

type GameController interface {
	Save() error
	Shutdown(seconds int, message string) error
}

type RestartOptions struct {
	ShutdownSeconds int
	RestartDelay    time.Duration
	Message         string
}

type StopOptions struct {
	ShutdownSeconds int
	Message         string
	KeepStopped     bool
}

type TransactionHooks struct {
	AfterExit   func() error
	Rollback    func() error
	HealthCheck func(context.Context) error
}

type Status struct {
	State                        State      `json:"state"`
	Running                      bool       `json:"running"`
	PID                          int        `json:"pid"`
	StartedAt                    *time.Time `json:"started_at,omitempty"`
	UptimeSeconds                int64      `json:"uptime_seconds"`
	DesiredRunning               bool       `json:"desired_running"`
	WatchdogEnabled              bool       `json:"watchdog_enabled"`
	Restarting                   bool       `json:"restarting"`
	ExternalProcess              bool       `json:"external_process"`
	LastExitAt                   *time.Time `json:"last_exit_at,omitempty"`
	LastExitCode                 int        `json:"last_exit_code"`
	LastExitPlanned              bool       `json:"last_exit_planned"`
	LastError                    string     `json:"last_error"`
	RestartCount                 int        `json:"restart_count"`
	RecentCrashCount             int        `json:"recent_crash_count"`
	CrashLoopDetected            bool       `json:"crash_loop_detected"`
	ScheduledRestartEnabled      bool       `json:"scheduled_restart_enabled"`
	ScheduledRestartFrequency    string     `json:"scheduled_restart_frequency"`
	ScheduledRestartTime         string     `json:"scheduled_restart_time"`
	ScheduledRestartIntervalDays int        `json:"scheduled_restart_interval_days"`
	ScheduledRestartStartDate    string     `json:"scheduled_restart_start_date"`
	ScheduledRestartWeekday      int        `json:"scheduled_restart_weekday"`
	ScheduledRestartDayOfMonth   int        `json:"scheduled_restart_day_of_month"`
	ScheduledRestartCron         string     `json:"scheduled_restart_cron"`
	ScheduledRestartTimezone     string     `json:"scheduled_restart_timezone"`
	NextScheduledRestartAt       *time.Time `json:"next_scheduled_restart_at,omitempty"`
	LastScheduledRestartAt       *time.Time `json:"last_scheduled_restart_at,omitempty"`
	LastScheduledRestartError    string     `json:"last_scheduled_restart_error"`
}

type ServerSupervisor struct {
	mu sync.Mutex

	config     ProcessConfig
	launcher   ProcessLauncher
	detector   ProcessDetector
	controller GameController
	updater    ServerUpdater

	ctx             context.Context
	cancel          context.CancelFunc
	closed          bool
	scheduleChanged chan struct{}

	process    ManagedProcess
	generation uint64
	state      State

	desiredRunning    bool
	watchdogEnabled   bool
	plannedShutdown   bool
	restarting        bool
	externalProcess   bool
	operationActive   bool
	transactionActive bool
	transactionExit   chan struct{}

	pid                       int
	startedAt                 time.Time
	lastExitAt                time.Time
	lastExitCode              int
	lastExitPlanned           bool
	lastError                 string
	restartCount              int
	restartTimes              []time.Time
	restartDelay              time.Duration
	lastScheduledRestartAt    time.Time
	lastScheduledRestartError string
	updateStatus              UpdateStatus
}

func New(value config.ServerProcessConfig, launcher ProcessLauncher, detector ProcessDetector, controller GameController) *ServerSupervisor {
	ctx, cancel := context.WithCancel(context.Background())
	processConfig := ProcessConfigFrom(value)
	supervisor := &ServerSupervisor{
		config:          processConfig,
		launcher:        launcher,
		detector:        detector,
		controller:      controller,
		ctx:             ctx,
		cancel:          cancel,
		scheduleChanged: make(chan struct{}, 1),
		state:           StateStopped,
		desiredRunning:  processConfig.WatchdogEnabled,
		watchdogEnabled: processConfig.WatchdogEnabled,
		lastExitCode:    -1,
		restartDelay:    processConfig.RestartDelay,
	}
	go supervisor.scheduledRestartLoop()
	return supervisor
}

func (s *ServerSupervisor) Bootstrap() error {
	if s.refreshExternal() {
		return nil
	}
	s.mu.Lock()
	shouldStart := s.config.Enabled && s.watchdogEnabled && s.desiredRunning && !s.closed
	s.mu.Unlock()
	if shouldStart {
		_, err := s.Start()
		return err
	}
	return nil
}

func (s *ServerSupervisor) UpdateConfig(value config.ServerProcessConfig) {
	s.mu.Lock()
	s.config = ProcessConfigFrom(value)
	s.watchdogEnabled = value.WatchdogEnabled
	if !value.WatchdogEnabled {
		s.desiredRunning = false
		if s.process == nil && !s.externalProcess && s.state == StateRestartWaiting {
			s.state = StateStopped
			s.restarting = false
			s.operationActive = false
		}
	}
	if s.restartDelay == 0 || !s.restarting {
		s.restartDelay = s.config.RestartDelay
	}
	s.mu.Unlock()
	s.notifyScheduleChanged()
}

func (s *ServerSupervisor) Start() (Status, error) {
	return s.start(false)
}

func (s *ServerSupervisor) start(automatic bool) (Status, error) {
	return s.startMode(automatic, false)
}

func (s *ServerSupervisor) startMode(automatic, transaction bool) (Status, error) {
	if s.refreshExternal() {
		return s.Status(), ErrConflict
	}

	s.mu.Lock()
	if s.closed {
		status := s.statusLocked(time.Now())
		s.mu.Unlock()
		return status, errors.New("server supervisor is closed")
	}
	if !s.config.Enabled {
		status := s.statusLocked(time.Now())
		s.mu.Unlock()
		return status, ErrProcessNotConfigured
	}
	// A delayed automatic restart may have passed its timer check immediately
	// before an administrator disables the watchdog. Re-check the desired state
	// while holding the supervisor lock so that a stale goroutine cannot start a
	// server that was explicitly requested to remain stopped.
	if automatic && !s.desiredRunning {
		status := s.statusLocked(time.Now())
		s.mu.Unlock()
		return status, ErrConflict
	}
	if s.process != nil || s.externalProcess || s.state == StateStarting || s.state == StateStopping || (!automatic && s.state == StateRestartWaiting) || (s.operationActive && !transaction) || (s.updateStatus.Running && !transaction) {
		status := s.statusLocked(time.Now())
		s.mu.Unlock()
		return status, ErrConflict
	}
	if !automatic {
		s.desiredRunning = true
		s.plannedShutdown = false
		s.restarting = false
		s.clearCrashLoopLocked()
	}
	if s.restarting || automatic {
		s.state = StateRestarting
	} else {
		s.state = StateStarting
	}
	processConfig := s.config
	s.mu.Unlock()

	process, err := s.launcher.Start(s.ctx, processConfig)
	now := time.Now()
	s.mu.Lock()
	if err != nil {
		s.lastError = err.Error()
		s.state = StateError
		if !transaction {
			s.operationActive = false
		}
		logger.Errorf("PalServer start failed: %v\n", err)
		if !transaction {
			s.recordFailureAndMaybeRestartLocked(now)
		}
		status := s.statusLocked(now)
		s.mu.Unlock()
		return status, err
	}
	s.process = process
	s.generation++
	generation := s.generation
	s.pid = process.PID()
	s.startedAt = now
	s.externalProcess = false
	s.state = StateRunning
	s.plannedShutdown = false
	s.restarting = false
	if !transaction {
		s.operationActive = false
	}
	s.lastError = ""
	logger.Infof("PalServer started with PID %d\n", s.pid)
	status := s.statusLocked(now)
	s.mu.Unlock()

	go s.waitForProcess(process, generation)
	return status, nil
}

// ApplyAndRestart performs a restart transaction without racing the watchdog:
// save and graceful shutdown happen first, AfterExit runs only after the old
// process actually exits, and a failed start is rolled back exactly once.
func (s *ServerSupervisor) ApplyAndRestart(options RestartOptions, hooks TransactionHooks) (Status, error) {
	return s.applyAndRestart(options, hooks, false)
}

func (s *ServerSupervisor) applyAndRestart(options RestartOptions, hooks TransactionHooks, updateTransaction bool) (Status, error) {
	s.mu.Lock()
	if !s.isRunningLocked() {
		status := s.statusLocked(time.Now())
		s.mu.Unlock()
		return status, ErrNotRunning
	}
	if s.operationActive || s.transactionActive || s.restarting || s.state == StateStopping || s.state == StateRestartWaiting || (s.updateStatus.Running && !updateTransaction) {
		status := s.statusLocked(time.Now())
		s.mu.Unlock()
		return status, ErrConflict
	}
	s.operationActive = true
	s.transactionActive = true
	exitSignal := make(chan struct{})
	s.transactionExit = exitSignal
	s.mu.Unlock()

	fail := func(err error) (Status, error) {
		s.mu.Lock()
		s.transactionActive = false
		s.transactionExit = nil
		s.operationActive = false
		s.plannedShutdown = false
		s.restarting = false
		if s.isRunningLocked() {
			s.state = StateRunning
		} else {
			s.state = StateError
		}
		s.lastError = err.Error()
		status := s.statusLocked(time.Now())
		s.mu.Unlock()
		return status, err
	}

	if err := s.controller.Save(); err != nil {
		return fail(fmt.Errorf("save world: %w", err))
	}

	s.mu.Lock()
	if !s.isRunningLocked() {
		s.mu.Unlock()
		return fail(ErrNotRunning)
	}
	s.desiredRunning = true
	s.plannedShutdown = true
	s.restarting = true
	s.restartDelay = options.RestartDelay
	s.state = StateStopping
	s.mu.Unlock()
	if err := s.controller.Shutdown(options.ShutdownSeconds, options.Message); err != nil {
		return fail(fmt.Errorf("graceful shutdown: %w", err))
	}

	waitTimeout := time.Duration(options.ShutdownSeconds)*time.Second + 2*time.Minute
	if waitTimeout < 2*time.Minute {
		waitTimeout = 2 * time.Minute
	}
	timer := time.NewTimer(waitTimeout)
	defer timer.Stop()
	select {
	case <-s.ctx.Done():
		return fail(errors.New("server supervisor closed during settings transaction"))
	case <-timer.C:
		return fail(errors.New("timed out waiting for PalServer to exit"))
	case <-exitSignal:
	}

	if hooks.AfterExit != nil {
		if err := hooks.AfterExit(); err != nil {
			applyErr := err
			rollbackErr := error(nil)
			if hooks.Rollback != nil {
				rollbackErr = hooks.Rollback()
			}
			if rollbackErr != nil {
				return fail(fmt.Errorf("apply settings after shutdown: %v; rollback failed: %w", applyErr, rollbackErr))
			}
			if _, restartErr := s.startMode(true, true); restartErr != nil {
				return fail(fmt.Errorf("apply settings after shutdown: %v; restored start failed: %w", applyErr, restartErr))
			}
			return fail(fmt.Errorf("apply settings after shutdown: %v; previous settings were restored", applyErr))
		}
	}

	delayTimer := time.NewTimer(options.RestartDelay)
	select {
	case <-s.ctx.Done():
		delayTimer.Stop()
		if hooks.Rollback != nil {
			_ = hooks.Rollback()
		}
		return fail(errors.New("server supervisor closed during restart delay"))
	case <-delayTimer.C:
	}

	if _, err := s.startMode(true, true); err != nil {
		rollbackErr := error(nil)
		if hooks.Rollback != nil {
			rollbackErr = hooks.Rollback()
		}
		if rollbackErr == nil {
			_, retryErr := s.startMode(true, true)
			if retryErr == nil {
				return fail(fmt.Errorf("start with new settings failed and old settings were restored: %w", err))
			}
			return fail(fmt.Errorf("start with new settings failed: %v; restored start failed: %w", err, retryErr))
		}
		return fail(fmt.Errorf("start with new settings failed: %v; rollback failed: %w", err, rollbackErr))
	}

	if hooks.HealthCheck != nil {
		healthContext, cancel := context.WithTimeout(s.ctx, 90*time.Second)
		err := hooks.HealthCheck(healthContext)
		cancel()
		if err != nil {
			healthErr := fmt.Errorf("PalServer health check failed: %w", err)
			s.mu.Lock()
			process := s.process
			exitSignal = make(chan struct{})
			s.transactionExit = exitSignal
			s.plannedShutdown = true
			s.restarting = true
			s.state = StateStopping
			s.mu.Unlock()

			// The new process must be fully gone before restoring the previous file.
			// Prefer the official graceful shutdown path, but if REST itself is what
			// the new configuration broke, terminate only the supervised process.
			if shutdownErr := s.controller.Shutdown(0, "Settings validation failed; restoring previous settings"); shutdownErr != nil {
				if process == nil {
					return fail(fmt.Errorf("%v; rollback shutdown failed: %w", healthErr, shutdownErr))
				}
				if killErr := process.Kill(); killErr != nil {
					return fail(fmt.Errorf("%v; rollback shutdown failed: %v; terminate failed: %w", healthErr, shutdownErr, killErr))
				}
			}
			rollbackTimer := time.NewTimer(2 * time.Minute)
			select {
			case <-s.ctx.Done():
				rollbackTimer.Stop()
				return fail(fmt.Errorf("%v; supervisor closed during rollback", healthErr))
			case <-rollbackTimer.C:
				return fail(fmt.Errorf("%v; timed out waiting for failed process to exit", healthErr))
			case <-exitSignal:
				rollbackTimer.Stop()
			}
			if hooks.Rollback == nil {
				return fail(fmt.Errorf("%v; no rollback hook was provided", healthErr))
			}
			if rollbackErr := hooks.Rollback(); rollbackErr != nil {
				return fail(fmt.Errorf("%v; rollback failed: %w", healthErr, rollbackErr))
			}
			if _, restartErr := s.startMode(true, true); restartErr != nil {
				return fail(fmt.Errorf("%v; restored start failed: %w", healthErr, restartErr))
			}
			return fail(fmt.Errorf("%v; previous settings were restored", healthErr))
		}
	}

	s.mu.Lock()
	s.transactionActive = false
	s.transactionExit = nil
	s.operationActive = false
	s.plannedShutdown = false
	s.restarting = false
	s.lastError = ""
	if s.isRunningLocked() {
		s.state = StateRunning
	}
	status := s.statusLocked(time.Now())
	s.mu.Unlock()
	logger.Info("PalServer settings transaction completed\n")
	return status, nil
}

func (s *ServerSupervisor) Restart(options RestartOptions) (Status, error) {
	return s.restart(options, "Administrator")
}

func (s *ServerSupervisor) restart(options RestartOptions, requester string) (Status, error) {
	s.mu.Lock()
	if !s.isRunningLocked() {
		status := s.statusLocked(time.Now())
		s.mu.Unlock()
		return status, ErrNotRunning
	}
	if s.operationActive || s.restarting || s.state == StateStopping || s.state == StateRestartWaiting || s.updateStatus.Running {
		status := s.statusLocked(time.Now())
		s.mu.Unlock()
		return status, ErrConflict
	}
	s.operationActive = true
	s.mu.Unlock()

	if err := s.controller.Save(); err != nil {
		s.failOperation(fmt.Errorf("save world: %w", err))
		return s.Status(), fmt.Errorf("save world: %w", err)
	}

	s.mu.Lock()
	if !s.isRunningLocked() {
		s.operationActive = false
		status := s.statusLocked(time.Now())
		s.mu.Unlock()
		return status, ErrNotRunning
	}
	s.desiredRunning = true
	s.plannedShutdown = true
	s.restarting = true
	s.restartDelay = options.RestartDelay
	s.state = StateStopping
	s.mu.Unlock()

	if err := s.controller.Shutdown(options.ShutdownSeconds, options.Message); err != nil {
		s.mu.Lock()
		if !s.isRunningLocked() {
			status := s.statusLocked(time.Now())
			s.mu.Unlock()
			return status, nil
		}
		s.plannedShutdown = false
		s.restarting = false
		s.operationActive = false
		s.state = StateRunning
		s.lastError = fmt.Sprintf("graceful shutdown: %v", err)
		s.mu.Unlock()
		return s.Status(), fmt.Errorf("graceful shutdown: %w", err)
	}
	logger.Infof("%s requested graceful PalServer restart (shutdown=%ds, restart delay=%s)\n", requester, options.ShutdownSeconds, options.RestartDelay)
	return s.Status(), nil
}

func (s *ServerSupervisor) Stop(options StopOptions) (Status, error) {
	s.mu.Lock()
	if !s.isRunningLocked() {
		status := s.statusLocked(time.Now())
		s.mu.Unlock()
		return status, ErrNotRunning
	}
	if s.operationActive || s.restarting || s.state == StateStopping || s.state == StateRestartWaiting || s.updateStatus.Running {
		status := s.statusLocked(time.Now())
		s.mu.Unlock()
		return status, ErrConflict
	}
	s.operationActive = true
	s.mu.Unlock()

	if err := s.controller.Save(); err != nil {
		s.failOperation(fmt.Errorf("save world: %w", err))
		return s.Status(), fmt.Errorf("save world: %w", err)
	}

	s.mu.Lock()
	if options.KeepStopped {
		s.desiredRunning = false
	}
	s.plannedShutdown = true
	s.restarting = false
	s.state = StateStopping
	s.mu.Unlock()

	if err := s.controller.Shutdown(options.ShutdownSeconds, options.Message); err != nil {
		s.mu.Lock()
		if !s.isRunningLocked() {
			status := s.statusLocked(time.Now())
			s.mu.Unlock()
			return status, nil
		}
		s.plannedShutdown = false
		s.operationActive = false
		s.state = StateRunning
		s.lastError = fmt.Sprintf("graceful shutdown: %v", err)
		s.mu.Unlock()
		return s.Status(), fmt.Errorf("graceful shutdown: %w", err)
	}
	logger.Infof("Administrator requested graceful PalServer stop (keep stopped=%t)\n", options.KeepStopped)
	return s.Status(), nil
}

func (s *ServerSupervisor) StopWatching() Status {
	return s.SetWatchdog(false)
}

func (s *ServerSupervisor) SaveWorld() error {
	return s.controller.Save()
}

func (s *ServerSupervisor) EnableWatchdog() Status {
	return s.SetWatchdog(true)
}

func (s *ServerSupervisor) SetWatchdog(enabled bool) Status {
	s.mu.Lock()
	s.watchdogEnabled = enabled
	s.desiredRunning = enabled
	if enabled {
		s.clearCrashLoopLocked()
		if s.state == StateCrashLoopStopped {
			s.state = StateStopped
		}
	} else if s.process == nil && !s.externalProcess && s.state == StateRestartWaiting {
		s.state = StateStopped
		s.restarting = false
		s.operationActive = false
	}
	status := s.statusLocked(time.Now())
	shouldStart := enabled && s.config.Enabled && !s.isRunningLocked() && !s.operationActive && s.state != StateStarting && s.state != StateRestartWaiting
	s.mu.Unlock()
	if shouldStart {
		go func() { _, _ = s.start(true) }()
	}
	return status
}

func (s *ServerSupervisor) ProcessStatus() Status {
	return s.Status()
}

func (s *ServerSupervisor) Status() Status {
	s.refreshExternal()
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.statusLocked(time.Now())
}

func (s *ServerSupervisor) scheduledRestartLoop() {
	for {
		s.mu.Lock()
		enabled := s.config.Enabled && s.config.ScheduledRestartEnabled && !s.closed
		processConfig := s.config
		s.mu.Unlock()

		if !enabled {
			select {
			case <-s.ctx.Done():
				return
			case <-s.scheduleChanged:
				continue
			}
		}

		next, err := nextScheduledRestart(time.Now(), processConfig)
		if err != nil {
			s.mu.Lock()
			s.lastScheduledRestartError = err.Error()
			s.mu.Unlock()
			select {
			case <-s.ctx.Done():
				return
			case <-s.scheduleChanged:
				continue
			}
		}

		timer := time.NewTimer(time.Until(next))
		select {
		case <-s.ctx.Done():
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			return
		case <-s.scheduleChanged:
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			continue
		case triggeredAt := <-timer.C:
			s.executeScheduledRestart(triggeredAt)
		}
	}
}

func (s *ServerSupervisor) executeScheduledRestart(triggeredAt time.Time) {
	s.mu.Lock()
	if s.closed || !s.config.Enabled || !s.config.ScheduledRestartEnabled {
		s.mu.Unlock()
		return
	}
	options := RestartOptions{
		ShutdownSeconds: s.config.GracefulShutdownSeconds,
		RestartDelay:    s.config.RestartDelay,
		Message:         s.config.GracefulShutdownMessage,
	}
	s.lastScheduledRestartAt = triggeredAt
	s.lastScheduledRestartError = ""
	s.mu.Unlock()

	logger.Infof("Scheduled PalServer restart triggered at %s\n", triggeredAt.Format(time.RFC3339))
	if _, err := s.restart(options, "Scheduled restart"); err != nil {
		s.mu.Lock()
		s.lastScheduledRestartError = fmt.Sprintf("scheduled restart skipped: %v", err)
		s.mu.Unlock()
		logger.Warnf("Scheduled PalServer restart skipped: %v\n", err)
	}
}

func (s *ServerSupervisor) notifyScheduleChanged() {
	select {
	case s.scheduleChanged <- struct{}{}:
	default:
	}
}

func nextDailyRestart(now time.Time, value string) (time.Time, error) {
	parsed, err := time.Parse("15:04", value)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid scheduled restart time %q: %w", value, err)
	}
	next := time.Date(now.Year(), now.Month(), now.Day(), parsed.Hour(), parsed.Minute(), 0, 0, now.Location())
	if !next.After(now) {
		next = next.AddDate(0, 0, 1)
	}
	return next, nil
}

func nextScheduledRestart(now time.Time, processConfig ProcessConfig) (time.Time, error) {
	if processConfig.ScheduledRestartFrequency == config.ScheduledRestartCron {
		schedule, err := cron.ParseStandard(strings.TrimSpace(processConfig.ScheduledRestartCron))
		if err != nil {
			return time.Time{}, fmt.Errorf("invalid scheduled restart cron expression: %w", err)
		}
		return schedule.Next(now), nil
	}
	parsedTime, err := time.Parse("15:04", processConfig.ScheduledRestartTime)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid scheduled restart time %q: %w", processConfig.ScheduledRestartTime, err)
	}
	atTime := func(date time.Time) time.Time {
		return time.Date(date.Year(), date.Month(), date.Day(), parsedTime.Hour(), parsedTime.Minute(), 0, 0, now.Location())
	}

	switch processConfig.ScheduledRestartFrequency {
	case config.ScheduledRestartDaily:
		return nextDailyRestart(now, processConfig.ScheduledRestartTime)
	case config.ScheduledRestartIntervalDays:
		if processConfig.ScheduledRestartIntervalDays < 1 {
			return time.Time{}, errors.New("scheduled restart interval must be positive")
		}
		start, err := time.Parse(time.DateOnly, processConfig.ScheduledRestartStartDate)
		if err != nil {
			return time.Time{}, fmt.Errorf("invalid scheduled restart start date %q: %w", processConfig.ScheduledRestartStartDate, err)
		}
		anchor := time.Date(start.Year(), start.Month(), start.Day(), parsedTime.Hour(), parsedTime.Minute(), 0, 0, now.Location())
		if anchor.After(now) {
			return anchor, nil
		}
		today := atTime(now)
		anchorDate := time.Date(anchor.Year(), anchor.Month(), anchor.Day(), 0, 0, 0, 0, time.UTC)
		todayDate := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
		elapsedDays := int(todayDate.Sub(anchorDate) / (24 * time.Hour))
		if elapsedDays < 0 {
			return anchor, nil
		}
		remainder := elapsedDays % processConfig.ScheduledRestartIntervalDays
		if remainder == 0 && today.After(now) {
			return today, nil
		}
		daysAhead := processConfig.ScheduledRestartIntervalDays - remainder
		if remainder == 0 {
			daysAhead = processConfig.ScheduledRestartIntervalDays
		}
		return today.AddDate(0, 0, daysAhead), nil
	case config.ScheduledRestartWeekly:
		if processConfig.ScheduledRestartWeekday < int(time.Sunday) || processConfig.ScheduledRestartWeekday > int(time.Saturday) {
			return time.Time{}, errors.New("scheduled restart weekday must be between 0 and 6")
		}
		daysAhead := (processConfig.ScheduledRestartWeekday - int(now.Weekday()) + 7) % 7
		next := atTime(now.AddDate(0, 0, daysAhead))
		if !next.After(now) {
			next = next.AddDate(0, 0, 7)
		}
		return next, nil
	case config.ScheduledRestartMonthly:
		if processConfig.ScheduledRestartDayOfMonth < 1 || processConfig.ScheduledRestartDayOfMonth > 31 {
			return time.Time{}, errors.New("scheduled restart day of month must be between 1 and 31")
		}
		monthlyCandidate := func(year int, month time.Month) time.Time {
			lastDay := time.Date(year, month+1, 0, 0, 0, 0, 0, now.Location()).Day()
			day := processConfig.ScheduledRestartDayOfMonth
			if day > lastDay {
				day = lastDay
			}
			return time.Date(year, month, day, parsedTime.Hour(), parsedTime.Minute(), 0, 0, now.Location())
		}
		next := monthlyCandidate(now.Year(), now.Month())
		if !next.After(now) {
			followingMonth := time.Date(now.Year(), now.Month()+1, 1, 0, 0, 0, 0, now.Location())
			next = monthlyCandidate(followingMonth.Year(), followingMonth.Month())
		}
		return next, nil
	default:
		return time.Time{}, fmt.Errorf("unsupported scheduled restart frequency %q", processConfig.ScheduledRestartFrequency)
	}
}

func PreviewScheduledRestarts(value config.ServerProcessConfig, now time.Time, count int) ([]time.Time, error) {
	if count < 1 {
		count = 3
	}
	if count > 10 {
		count = 10
	}
	processConfig := ProcessConfigFrom(value)
	items := make([]time.Time, 0, count)
	cursor := now
	for len(items) < count {
		next, err := nextScheduledRestart(cursor, processConfig)
		if err != nil {
			return nil, err
		}
		items = append(items, next)
		cursor = next.Add(time.Second)
	}
	return items, nil
}

func DescribeScheduledRestart(value config.ServerProcessConfig) string {
	switch value.ScheduledRestartFrequency {
	case config.ScheduledRestartDaily:
		return "每天 " + value.ScheduledRestartTime
	case config.ScheduledRestartIntervalDays:
		return fmt.Sprintf("从 %s 起，每隔 %d 天的 %s", value.ScheduledRestartStartDate, value.ScheduledRestartIntervalDays, value.ScheduledRestartTime)
	case config.ScheduledRestartWeekly:
		weekdays := []string{"星期日", "星期一", "星期二", "星期三", "星期四", "星期五", "星期六"}
		if value.ScheduledRestartWeekday >= 0 && value.ScheduledRestartWeekday < len(weekdays) {
			return weekdays[value.ScheduledRestartWeekday] + " " + value.ScheduledRestartTime
		}
	case config.ScheduledRestartMonthly:
		return fmt.Sprintf("每月 %d 日 %s；短月份按当月最后一天执行", value.ScheduledRestartDayOfMonth, value.ScheduledRestartTime)
	case config.ScheduledRestartCron:
		fields := strings.Fields(value.ScheduledRestartCron)
		if len(fields) == 5 {
			minute, minuteErr := strconv.Atoi(fields[0])
			hour, hourErr := strconv.Atoi(fields[1])
			if minuteErr == nil && hourErr == nil {
				at := fmt.Sprintf("%02d:%02d", hour, minute)
				if fields[2] == "*" && fields[3] == "*" && fields[4] == "*" {
					return "每天 " + at
				}
				if fields[2] == "*" && fields[3] == "*" {
					if weekday, err := strconv.Atoi(fields[4]); err == nil && weekday >= 0 && weekday <= 6 {
						return []string{"星期日", "星期一", "星期二", "星期三", "星期四", "星期五", "星期六"}[weekday] + " " + at
					}
				}
				if day, err := strconv.Atoi(fields[2]); err == nil && fields[3] == "*" && fields[4] == "*" {
					return fmt.Sprintf("每月 %d 日 %s", day, at)
				}
			}
		}
		return "按标准五段 Cron 执行：" + strings.TrimSpace(value.ScheduledRestartCron)
	}
	return "自动重启计划"
}

func timezoneLabel(now time.Time) string {
	name, offset := now.Zone()
	if name == "" {
		name = now.Location().String()
	}
	sign := "+"
	if offset < 0 {
		sign = "-"
		offset = -offset
	}
	return fmt.Sprintf("%s (UTC%s%02d:%02d)", name, sign, offset/3600, offset%3600/60)
}

func (s *ServerSupervisor) Close() {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.closed = true
	s.cancel()
	s.mu.Unlock()
}

func (s *ServerSupervisor) waitForProcess(process ManagedProcess, generation uint64) {
	err := process.Wait()
	exitCode := exitCode(process, err)
	s.handleExit(process, generation, exitCode, err)
}

func (s *ServerSupervisor) handleExit(process ManagedProcess, generation uint64, exitCode int, waitErr error) {
	s.mu.Lock()
	if s.process != process || s.generation != generation {
		s.mu.Unlock()
		return
	}
	s.process = nil
	s.pid = 0
	s.startedAt = time.Time{}
	s.lastExitAt = time.Now()
	s.lastExitCode = exitCode
	if waitErr != nil {
		s.lastError = waitErr.Error()
	} else {
		s.lastError = ""
	}
	logger.Infof("PalServer exited with code %d\n", exitCode)
	s.handleExitedLocked(s.lastExitAt)
	s.mu.Unlock()
}

func (s *ServerSupervisor) handleExitedLocked(now time.Time) {
	if s.closed {
		s.state = StateStopped
		return
	}
	s.lastExitPlanned = s.plannedShutdown
	if s.plannedShutdown {
		s.plannedShutdown = false
		if s.transactionActive {
			s.state = StateRestartWaiting
			if s.transactionExit != nil {
				close(s.transactionExit)
				s.transactionExit = nil
			}
			return
		}
		if s.restarting && s.desiredRunning {
			s.state = StateRestartWaiting
			delay := s.restartDelay
			s.operationActive = false
			s.scheduleStartLocked(delay, "planned restart")
			return
		}
		s.restarting = false
		s.operationActive = false
		s.state = StateStopped
		return
	}
	s.operationActive = false
	s.restarting = false
	s.state = StateStopped
	if s.watchdogEnabled && s.desiredRunning {
		logger.Warn("PalServer exited unexpectedly; watchdog will attempt restart\n")
		s.recordFailureAndMaybeRestartLocked(now)
	}
}

func (s *ServerSupervisor) recordFailureAndMaybeRestartLocked(now time.Time) {
	windowStart := now.Add(-s.config.RestartAttemptWindow)
	filtered := s.restartTimes[:0]
	for _, attempt := range s.restartTimes {
		if attempt.After(windowStart) {
			filtered = append(filtered, attempt)
		}
	}
	s.restartTimes = append(filtered, now)
	if len(s.restartTimes) >= s.config.MaxRestartAttempts {
		s.state = StateCrashLoopStopped
		s.watchdogEnabled = false
		s.desiredRunning = false
		s.restarting = false
		s.operationActive = false
		s.lastError = "automatic restart paused: crash loop detected"
		logger.Errorf("PalServer crash loop protection triggered after %d attempts\n", len(s.restartTimes))
		return
	}
	if s.watchdogEnabled && s.desiredRunning {
		s.state = StateRestartWaiting
		s.scheduleStartLocked(s.config.RestartDelay, "watchdog")
	}
}

func (s *ServerSupervisor) scheduleStartLocked(delay time.Duration, reason string) {
	s.restartCount++
	logger.Infof("Scheduling PalServer restart in %s (%s), attempt %d\n", delay, reason, s.restartCount)
	ctx := s.ctx
	go func() {
		timer := time.NewTimer(delay)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}
		s.mu.Lock()
		allowed := !s.closed && s.desiredRunning && s.state == StateRestartWaiting && s.process == nil && !s.externalProcess
		s.mu.Unlock()
		if allowed {
			_, _ = s.start(true)
		}
	}()
}

func (s *ServerSupervisor) refreshExternal() bool {
	if s.detector == nil {
		return false
	}
	s.mu.Lock()
	if s.process != nil || s.closed || s.state == StateStarting || s.state == StateRestarting {
		found := s.externalProcess
		s.mu.Unlock()
		return found
	}
	s.mu.Unlock()

	pid, err := s.detector.FindPalServer()
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.process != nil || s.closed {
		return s.externalProcess
	}
	if err != nil {
		s.lastError = fmt.Sprintf("detect external PalServer: %v", err)
		return false
	}
	if pid > 0 {
		newProcess := !s.externalProcess || s.pid != pid
		s.externalProcess = true
		s.pid = pid
		s.state = StateRunning
		if newProcess {
			s.startedAt = time.Time{}
			logger.Infof("Detected externally started PalServer with PID %d\n", pid)
			go s.monitorExternal(pid)
		}
		return true
	}
	if s.externalProcess {
		s.externalProcess = false
		s.pid = 0
		s.lastExitAt = time.Now()
		s.lastExitCode = -1
		s.handleExitedLocked(s.lastExitAt)
	}
	return false
}

func (s *ServerSupervisor) monitorExternal(pid int) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			s.mu.Lock()
			active := s.externalProcess && s.pid == pid && !s.closed
			s.mu.Unlock()
			if !active {
				return
			}
			if !s.refreshExternal() {
				return
			}
		}
	}
}

func (s *ServerSupervisor) failOperation(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.operationActive = false
	s.plannedShutdown = false
	s.restarting = false
	if s.isRunningLocked() {
		s.state = StateRunning
	} else {
		s.state = StateError
	}
	s.lastError = err.Error()
}

func (s *ServerSupervisor) isRunningLocked() bool {
	return s.process != nil || s.externalProcess
}

func (s *ServerSupervisor) clearCrashLoopLocked() {
	s.restartTimes = nil
	if s.state == StateCrashLoopStopped {
		s.state = StateStopped
	}
}

func (s *ServerSupervisor) statusLocked(now time.Time) Status {
	status := Status{
		State:                        s.state,
		Running:                      s.isRunningLocked(),
		PID:                          s.pid,
		DesiredRunning:               s.desiredRunning,
		WatchdogEnabled:              s.watchdogEnabled,
		Restarting:                   s.restarting || s.state == StateRestartWaiting || s.state == StateRestarting,
		ExternalProcess:              s.externalProcess,
		LastExitCode:                 s.lastExitCode,
		LastExitPlanned:              s.lastExitPlanned,
		LastError:                    s.lastError,
		RestartCount:                 s.restartCount,
		RecentCrashCount:             len(s.restartTimes),
		CrashLoopDetected:            s.state == StateCrashLoopStopped,
		ScheduledRestartEnabled:      s.config.Enabled && s.config.ScheduledRestartEnabled,
		ScheduledRestartFrequency:    s.config.ScheduledRestartFrequency,
		ScheduledRestartTime:         s.config.ScheduledRestartTime,
		ScheduledRestartIntervalDays: s.config.ScheduledRestartIntervalDays,
		ScheduledRestartStartDate:    s.config.ScheduledRestartStartDate,
		ScheduledRestartWeekday:      s.config.ScheduledRestartWeekday,
		ScheduledRestartDayOfMonth:   s.config.ScheduledRestartDayOfMonth,
		ScheduledRestartCron:         s.config.ScheduledRestartCron,
		ScheduledRestartTimezone:     timezoneLabel(now),
		LastScheduledRestartError:    s.lastScheduledRestartError,
	}
	if status.ScheduledRestartEnabled {
		if next, err := nextScheduledRestart(now, s.config); err == nil {
			status.NextScheduledRestartAt = &next
		}
	}
	if !s.lastScheduledRestartAt.IsZero() {
		lastScheduledRestartAt := s.lastScheduledRestartAt
		status.LastScheduledRestartAt = &lastScheduledRestartAt
	}
	if !s.startedAt.IsZero() {
		startedAt := s.startedAt
		status.StartedAt = &startedAt
		status.UptimeSeconds = int64(now.Sub(s.startedAt).Seconds())
	}
	if !s.lastExitAt.IsZero() {
		lastExitAt := s.lastExitAt
		status.LastExitAt = &lastExitAt
	}
	return status
}

type exitCoder interface {
	ExitCode() int
}

func exitCode(process ManagedProcess, waitErr error) int {
	if processWithCode, ok := process.(exitCoder); ok {
		return processWithCode.ExitCode()
	}
	if waitErr == nil {
		return 0
	}
	return -1
}
