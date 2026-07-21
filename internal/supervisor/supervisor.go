package supervisor

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"

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
	StateCrashLoopStopped State = "crash_loop_stopped"
	StateError            State = "error"
)

type ProcessConfig struct {
	Enabled                 bool
	ExecutablePath          string
	WorkingDirectory        string
	Arguments               []string
	WatchdogEnabled         bool
	RestartDelay            time.Duration
	GracefulShutdownSeconds int
	GracefulShutdownMessage string
	MaxRestartAttempts      int
	RestartAttemptWindow    time.Duration
}

func ProcessConfigFrom(value config.ServerProcessConfig) ProcessConfig {
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
		Enabled:                 value.Enabled,
		ExecutablePath:          value.ExecutablePath,
		WorkingDirectory:        workingDirectory,
		Arguments:               append([]string(nil), value.Arguments...),
		WatchdogEnabled:         value.WatchdogEnabled,
		RestartDelay:            time.Duration(value.RestartDelaySeconds) * time.Second,
		GracefulShutdownSeconds: value.GracefulShutdownSeconds,
		GracefulShutdownMessage: value.GracefulShutdownMessage,
		MaxRestartAttempts:      value.MaxRestartAttempts,
		RestartAttemptWindow:    time.Duration(value.RestartAttemptWindowSeconds) * time.Second,
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

type Status struct {
	State             State      `json:"state"`
	Running           bool       `json:"running"`
	PID               int        `json:"pid"`
	StartedAt         *time.Time `json:"started_at,omitempty"`
	UptimeSeconds     int64      `json:"uptime_seconds"`
	DesiredRunning    bool       `json:"desired_running"`
	WatchdogEnabled   bool       `json:"watchdog_enabled"`
	Restarting        bool       `json:"restarting"`
	ExternalProcess   bool       `json:"external_process"`
	LastExitAt        *time.Time `json:"last_exit_at,omitempty"`
	LastExitCode      int        `json:"last_exit_code"`
	LastError         string     `json:"last_error"`
	RestartCount      int        `json:"restart_count"`
	RecentCrashCount  int        `json:"recent_crash_count"`
	CrashLoopDetected bool       `json:"crash_loop_detected"`
}

type ServerSupervisor struct {
	mu sync.Mutex

	config     ProcessConfig
	launcher   ProcessLauncher
	detector   ProcessDetector
	controller GameController

	ctx    context.Context
	cancel context.CancelFunc
	closed bool

	process    ManagedProcess
	generation uint64
	state      State

	desiredRunning  bool
	watchdogEnabled bool
	plannedShutdown bool
	restarting      bool
	externalProcess bool
	operationActive bool

	pid          int
	startedAt    time.Time
	lastExitAt   time.Time
	lastExitCode int
	lastError    string
	restartCount int
	restartTimes []time.Time
	restartDelay time.Duration
}

func New(value config.ServerProcessConfig, launcher ProcessLauncher, detector ProcessDetector, controller GameController) *ServerSupervisor {
	ctx, cancel := context.WithCancel(context.Background())
	processConfig := ProcessConfigFrom(value)
	return &ServerSupervisor{
		config:          processConfig,
		launcher:        launcher,
		detector:        detector,
		controller:      controller,
		ctx:             ctx,
		cancel:          cancel,
		state:           StateStopped,
		desiredRunning:  processConfig.WatchdogEnabled,
		watchdogEnabled: processConfig.WatchdogEnabled,
		lastExitCode:    -1,
		restartDelay:    processConfig.RestartDelay,
	}
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
	defer s.mu.Unlock()
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
}

func (s *ServerSupervisor) Start() (Status, error) {
	return s.start(false)
}

func (s *ServerSupervisor) start(automatic bool) (Status, error) {
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
	if s.process != nil || s.externalProcess || s.state == StateStarting || s.state == StateStopping || (!automatic && s.state == StateRestartWaiting) || s.operationActive {
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
		s.operationActive = false
		logger.Errorf("PalServer start failed: %v\n", err)
		s.recordFailureAndMaybeRestartLocked(now)
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
	s.operationActive = false
	s.lastError = ""
	logger.Infof("PalServer started with PID %d\n", s.pid)
	status := s.statusLocked(now)
	s.mu.Unlock()

	go s.waitForProcess(process, generation)
	return status, nil
}

func (s *ServerSupervisor) Restart(options RestartOptions) (Status, error) {
	s.mu.Lock()
	if !s.isRunningLocked() {
		status := s.statusLocked(time.Now())
		s.mu.Unlock()
		return status, ErrNotRunning
	}
	if s.operationActive || s.restarting || s.state == StateStopping || s.state == StateRestartWaiting {
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
	logger.Infof("Administrator requested graceful PalServer restart (shutdown=%ds, restart delay=%s)\n", options.ShutdownSeconds, options.RestartDelay)
	return s.Status(), nil
}

func (s *ServerSupervisor) Stop(options StopOptions) (Status, error) {
	s.mu.Lock()
	if !s.isRunningLocked() {
		status := s.statusLocked(time.Now())
		s.mu.Unlock()
		return status, ErrNotRunning
	}
	if s.operationActive || s.restarting || s.state == StateStopping || s.state == StateRestartWaiting {
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
	if s.plannedShutdown {
		s.plannedShutdown = false
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
		State:             s.state,
		Running:           s.isRunningLocked(),
		PID:               s.pid,
		DesiredRunning:    s.desiredRunning,
		WatchdogEnabled:   s.watchdogEnabled,
		Restarting:        s.restarting || s.state == StateRestartWaiting || s.state == StateRestarting,
		ExternalProcess:   s.externalProcess,
		LastExitCode:      s.lastExitCode,
		LastError:         s.lastError,
		RestartCount:      s.restartCount,
		RecentCrashCount:  len(s.restartTimes),
		CrashLoopDetected: s.state == StateCrashLoopStopped,
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
