package supervisor

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/zaigie/palworld-server-tool/internal/config"
)

type fakeProcess struct {
	pid      int
	done     chan struct{}
	err      error
	exitCode int
	onWait   func()
	once     sync.Once
}

func newFakeProcess(pid int) *fakeProcess {
	return &fakeProcess{pid: pid, done: make(chan struct{}), exitCode: 0}
}

func (process *fakeProcess) PID() int { return process.pid }
func (process *fakeProcess) Wait() error {
	<-process.done
	if process.onWait != nil {
		process.onWait()
	}
	return process.err
}
func (process *fakeProcess) Kill() error {
	process.Exit(-1, errors.New("killed"))
	return nil
}
func (process *fakeProcess) ExitCode() int { return process.exitCode }
func (process *fakeProcess) Exit(code int, err error) {
	process.once.Do(func() {
		process.exitCode = code
		process.err = err
		close(process.done)
	})
}

type fakeLauncher struct {
	mu      sync.Mutex
	starts  int
	startFn func(attempt int) (ManagedProcess, error)
}

func (launcher *fakeLauncher) Start(_ context.Context, _ ProcessConfig) (ManagedProcess, error) {
	launcher.mu.Lock()
	defer launcher.mu.Unlock()
	launcher.starts++
	return launcher.startFn(launcher.starts)
}

func (launcher *fakeLauncher) Count() int {
	launcher.mu.Lock()
	defer launcher.mu.Unlock()
	return launcher.starts
}

type fakeDetector struct{ pid atomic.Int64 }

func (detector *fakeDetector) FindPalServer() (int, error) { return int(detector.pid.Load()), nil }

type fakeController struct {
	mu              sync.Mutex
	events          []string
	saveErr         error
	shutdownErr     error
	shutdownSeconds int
	shutdownMessage string
	saveStarted     chan struct{}
	saveContinue    chan struct{}
}

func (controller *fakeController) Save() error {
	controller.mu.Lock()
	controller.events = append(controller.events, "save")
	controller.mu.Unlock()
	if controller.saveStarted != nil {
		select {
		case controller.saveStarted <- struct{}{}:
		default:
		}
	}
	if controller.saveContinue != nil {
		<-controller.saveContinue
	}
	return controller.saveErr
}

func (controller *fakeController) Shutdown(seconds int, message string) error {
	controller.mu.Lock()
	defer controller.mu.Unlock()
	controller.events = append(controller.events, "shutdown")
	controller.shutdownSeconds = seconds
	controller.shutdownMessage = message
	return controller.shutdownErr
}

func (controller *fakeController) Events() []string {
	controller.mu.Lock()
	defer controller.mu.Unlock()
	return append([]string(nil), controller.events...)
}

func (controller *fakeController) ShutdownRequest() (int, string) {
	controller.mu.Lock()
	defer controller.mu.Unlock()
	return controller.shutdownSeconds, controller.shutdownMessage
}

func testConfig(watchdog bool) config.ServerProcessConfig {
	value := config.Default().ServerProcess
	value.Enabled = true
	value.ExecutablePath = "PalServer.exe"
	value.WorkingDirectory = "."
	value.WatchdogEnabled = watchdog
	value.RestartDelaySeconds = 0
	return value
}

func waitFor(t *testing.T, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("condition was not satisfied before timeout")
}

func TestStoppedServerStartsSuccessfullyAndRejectsDuplicateStart(t *testing.T) {
	process := newFakeProcess(101)
	launcher := &fakeLauncher{startFn: func(int) (ManagedProcess, error) { return process, nil }}
	s := New(testConfig(false), launcher, &fakeDetector{}, &fakeController{})
	defer s.Close()

	status, err := s.Start()
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	if !status.Running || status.PID != 101 || status.State != StateRunning {
		t.Fatalf("unexpected running status: %#v", status)
	}
	if _, err := s.Start(); !errors.Is(err, ErrConflict) {
		t.Fatalf("duplicate start error = %v, want conflict", err)
	}
	if launcher.Count() != 1 {
		t.Fatalf("launch count = %d, want 1", launcher.Count())
	}
}

func TestGracefulRestartOrderWaitsForExitAndDelay(t *testing.T) {
	first := newFakeProcess(201)
	second := newFakeProcess(202)
	var eventMu sync.Mutex
	events := make([]string, 0)
	first.onWait = func() {
		eventMu.Lock()
		events = append(events, "wait")
		eventMu.Unlock()
	}
	controller := &fakeController{}
	launcher := &fakeLauncher{startFn: func(attempt int) (ManagedProcess, error) {
		if attempt == 1 {
			return first, nil
		}
		eventMu.Lock()
		events = append(events, "start")
		eventMu.Unlock()
		return second, nil
	}}
	s := New(testConfig(true), launcher, &fakeDetector{}, controller)
	defer s.Close()
	if _, err := s.Start(); err != nil {
		t.Fatalf("initial start: %v", err)
	}
	if _, err := s.Restart(RestartOptions{ShutdownSeconds: 1, RestartDelay: 20 * time.Millisecond, Message: "restart"}); err != nil {
		t.Fatalf("restart: %v", err)
	}
	if launcher.Count() != 1 {
		t.Fatal("restart must not launch before the old process exits")
	}
	first.Exit(0, nil)
	waitFor(t, func() bool { return launcher.Count() == 2 })
	if got := controller.Events(); len(got) != 2 || got[0] != "save" || got[1] != "shutdown" {
		t.Fatalf("controller order = %v", got)
	}
	eventMu.Lock()
	defer eventMu.Unlock()
	if len(events) != 2 || events[0] != "wait" || events[1] != "start" {
		t.Fatalf("process order = %v, want wait then start", events)
	}
}

func TestManualKeepStoppedDoesNotRestart(t *testing.T) {
	process := newFakeProcess(301)
	launcher := &fakeLauncher{startFn: func(int) (ManagedProcess, error) { return process, nil }}
	s := New(testConfig(true), launcher, &fakeDetector{}, &fakeController{})
	defer s.Close()
	_, _ = s.Start()
	if _, err := s.Stop(StopOptions{ShutdownSeconds: 1, Message: "stop", KeepStopped: true}); err != nil {
		t.Fatalf("stop: %v", err)
	}
	process.Exit(0, nil)
	waitFor(t, func() bool { return s.Status().State == StateStopped })
	time.Sleep(20 * time.Millisecond)
	if launcher.Count() != 1 || s.Status().DesiredRunning {
		t.Fatalf("manual stop restarted server: count=%d status=%#v", launcher.Count(), s.Status())
	}
}

func TestUnexpectedCrashRestartsOnlyWithWatchdog(t *testing.T) {
	for _, testCase := range []struct {
		name     string
		watchdog bool
		want     int
	}{{"enabled", true, 2}, {"disabled", false, 1}} {
		t.Run(testCase.name, func(t *testing.T) {
			first := newFakeProcess(401)
			second := newFakeProcess(402)
			launcher := &fakeLauncher{startFn: func(attempt int) (ManagedProcess, error) {
				if attempt == 1 {
					return first, nil
				}
				return second, nil
			}}
			s := New(testConfig(testCase.watchdog), launcher, &fakeDetector{}, &fakeController{})
			defer s.Close()
			_, _ = s.Start()
			first.Exit(1, errors.New("crashed"))
			if testCase.watchdog {
				waitFor(t, func() bool { return launcher.Count() == testCase.want })
			} else {
				time.Sleep(20 * time.Millisecond)
			}
			if launcher.Count() != testCase.want {
				t.Fatalf("launch count = %d, want %d", launcher.Count(), testCase.want)
			}
		})
	}
}

func TestDisablingWatchdogCancelsPendingRestart(t *testing.T) {
	first := newFakeProcess(450)
	value := testConfig(true)
	value.RestartDelaySeconds = 1
	launcher := &fakeLauncher{startFn: func(int) (ManagedProcess, error) { return first, nil }}
	s := New(value, launcher, &fakeDetector{}, &fakeController{})
	s.config.RestartDelay = 20 * time.Millisecond
	defer s.Close()
	_, _ = s.Start()
	first.Exit(1, errors.New("crashed"))
	waitFor(t, func() bool { return s.Status().State == StateRestartWaiting })
	status := s.StopWatching()
	if status.State != StateStopped || status.DesiredRunning || status.WatchdogEnabled {
		t.Fatalf("watchdog disable status = %#v", status)
	}
	time.Sleep(40 * time.Millisecond)
	if launcher.Count() != 1 {
		t.Fatalf("disabled watchdog launched %d processes, want 1", launcher.Count())
	}
}

func TestBootstrapDetectsExternalProcessAndPreventsDuplicate(t *testing.T) {
	detector := &fakeDetector{}
	detector.pid.Store(777)
	launcher := &fakeLauncher{startFn: func(int) (ManagedProcess, error) { return newFakeProcess(778), nil }}
	s := New(testConfig(true), launcher, detector, &fakeController{})
	defer s.Close()
	if err := s.Bootstrap(); err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	status := s.Status()
	if !status.Running || !status.ExternalProcess || status.PID != 777 {
		t.Fatalf("external process status = %#v", status)
	}
	if _, err := s.Start(); !errors.Is(err, ErrConflict) {
		t.Fatalf("start with external process error = %v, want conflict", err)
	}
	if launcher.Count() != 0 {
		t.Fatalf("external process caused %d duplicate launches", launcher.Count())
	}
}

func TestRepeatedStartFailureTriggersCrashLoopProtection(t *testing.T) {
	value := testConfig(true)
	value.MaxRestartAttempts = 3
	launcher := &fakeLauncher{startFn: func(int) (ManagedProcess, error) { return nil, errors.New("start failed") }}
	s := New(value, launcher, &fakeDetector{}, &fakeController{})
	defer s.Close()
	_, _ = s.Start()
	waitFor(t, func() bool { return s.Status().CrashLoopDetected })
	status := s.Status()
	if launcher.Count() != 3 || status.WatchdogEnabled || status.DesiredRunning {
		t.Fatalf("crash loop status = %#v, launches=%d", status, launcher.Count())
	}
}

func TestConcurrentRestartRequestsAllowOnlyOne(t *testing.T) {
	process := newFakeProcess(501)
	launcher := &fakeLauncher{startFn: func(int) (ManagedProcess, error) { return process, nil }}
	controller := &fakeController{saveStarted: make(chan struct{}, 1), saveContinue: make(chan struct{})}
	s := New(testConfig(true), launcher, &fakeDetector{}, controller)
	defer s.Close()
	_, _ = s.Start()

	firstResult := make(chan error, 1)
	go func() {
		_, err := s.Restart(RestartOptions{Message: "restart"})
		firstResult <- err
	}()
	<-controller.saveStarted
	if _, err := s.Restart(RestartOptions{Message: "restart"}); !errors.Is(err, ErrConflict) {
		t.Fatalf("concurrent restart error = %v, want conflict", err)
	}
	close(controller.saveContinue)
	if err := <-firstResult; err != nil {
		t.Fatalf("first restart: %v", err)
	}
	if got := controller.Events(); len(got) != 2 {
		t.Fatalf("controller calls = %v, want one save and one shutdown", got)
	}
}

func TestSettingsTransactionOrderWaitsForExitBeforeApply(t *testing.T) {
	first := newFakeProcess(550)
	second := newFakeProcess(551)
	var eventMu sync.Mutex
	events := make([]string, 0)
	record := func(event string) {
		eventMu.Lock()
		events = append(events, event)
		eventMu.Unlock()
	}
	first.onWait = func() { record("wait") }
	launcher := &fakeLauncher{startFn: func(attempt int) (ManagedProcess, error) {
		if attempt == 1 {
			return first, nil
		}
		record("start")
		return second, nil
	}}
	controller := &fakeController{}
	s := New(testConfig(true), launcher, &fakeDetector{}, controller)
	defer s.Close()
	if _, err := s.Start(); err != nil {
		t.Fatalf("initial start: %v", err)
	}
	result := make(chan error, 1)
	go func() {
		_, err := s.ApplyAndRestart(
			RestartOptions{ShutdownSeconds: 1, RestartDelay: time.Millisecond, Message: "settings"},
			TransactionHooks{
				AfterExit:   func() error { record("apply"); return nil },
				HealthCheck: func(context.Context) error { record("health"); return nil },
			},
		)
		result <- err
	}()
	waitFor(t, func() bool { return len(controller.Events()) == 2 })
	if launcher.Count() != 1 {
		t.Fatal("settings transaction started before old process exit")
	}
	first.Exit(0, nil)
	if err := <-result; err != nil {
		t.Fatalf("settings transaction: %v", err)
	}
	if got := controller.Events(); len(got) != 2 || got[0] != "save" || got[1] != "shutdown" {
		t.Fatalf("controller order = %v", got)
	}
	eventMu.Lock()
	defer eventMu.Unlock()
	want := []string{"wait", "apply", "start", "health"}
	if strings.Join(events, ",") != strings.Join(want, ",") {
		t.Fatalf("transaction events = %v, want %v", events, want)
	}
}

func TestSettingsTransactionStartFailureRollsBackAndStartsOldSettingsOnce(t *testing.T) {
	first := newFakeProcess(560)
	restored := newFakeProcess(562)
	events := make([]string, 0)
	var eventMu sync.Mutex
	record := func(event string) {
		eventMu.Lock()
		events = append(events, event)
		eventMu.Unlock()
	}
	launcher := &fakeLauncher{startFn: func(attempt int) (ManagedProcess, error) {
		switch attempt {
		case 1:
			return first, nil
		case 2:
			record("new-start-failed")
			return nil, errors.New("new settings failed")
		default:
			record("old-start")
			return restored, nil
		}
	}}
	s := New(testConfig(true), launcher, &fakeDetector{}, &fakeController{})
	defer s.Close()
	_, _ = s.Start()
	result := make(chan error, 1)
	go func() {
		_, err := s.ApplyAndRestart(
			RestartOptions{RestartDelay: time.Millisecond},
			TransactionHooks{
				AfterExit: func() error { record("apply"); return nil },
				Rollback:  func() error { record("rollback"); return nil },
			},
		)
		result <- err
	}()
	waitFor(t, func() bool { return s.Status().State == StateStopping })
	first.Exit(0, nil)
	if err := <-result; err == nil {
		t.Fatal("start failure should be reported after rollback")
	}
	if launcher.Count() != 3 || !s.Status().Running {
		t.Fatalf("launches=%d status=%#v", launcher.Count(), s.Status())
	}
	eventMu.Lock()
	defer eventMu.Unlock()
	want := []string{"apply", "new-start-failed", "rollback", "old-start"}
	if strings.Join(events, ",") != strings.Join(want, ",") {
		t.Fatalf("rollback events = %v, want %v", events, want)
	}
}

func TestSettingsTransactionWriteFailureRestartsPreviousSettings(t *testing.T) {
	first := newFakeProcess(565)
	restored := newFakeProcess(566)
	launcher := &fakeLauncher{startFn: func(attempt int) (ManagedProcess, error) {
		if attempt == 1 {
			return first, nil
		}
		return restored, nil
	}}
	s := New(testConfig(true), launcher, &fakeDetector{}, &fakeController{})
	defer s.Close()
	_, _ = s.Start()
	rolledBack := false
	result := make(chan error, 1)
	go func() {
		_, err := s.ApplyAndRestart(RestartOptions{}, TransactionHooks{
			AfterExit: func() error { return errors.New("atomic write failed") },
			Rollback:  func() error { rolledBack = true; return nil },
		})
		result <- err
	}()
	waitFor(t, func() bool { return s.Status().State == StateStopping })
	first.Exit(0, nil)
	if err := <-result; err == nil || !strings.Contains(err.Error(), "previous settings were restored") {
		t.Fatalf("transaction error = %v", err)
	}
	if !rolledBack || launcher.Count() != 2 || !s.Status().Running || s.Status().PID != restored.PID() {
		t.Fatalf("rolledBack=%v launches=%d status=%#v", rolledBack, launcher.Count(), s.Status())
	}
}

func TestSettingsTransactionHealthFailureStopsNewProcessThenRollsBack(t *testing.T) {
	first := newFakeProcess(570)
	invalid := newFakeProcess(571)
	restored := newFakeProcess(572)
	launcher := &fakeLauncher{startFn: func(attempt int) (ManagedProcess, error) {
		switch attempt {
		case 1:
			return first, nil
		case 2:
			return invalid, nil
		default:
			return restored, nil
		}
	}}
	controller := &fakeController{}
	s := New(testConfig(true), launcher, &fakeDetector{}, controller)
	defer s.Close()
	_, _ = s.Start()
	rolledBack := make(chan struct{}, 1)
	result := make(chan error, 1)
	go func() {
		_, err := s.ApplyAndRestart(
			RestartOptions{RestartDelay: time.Millisecond},
			TransactionHooks{
				AfterExit:   func() error { return nil },
				Rollback:    func() error { rolledBack <- struct{}{}; return nil },
				HealthCheck: func(context.Context) error { return errors.New("unhealthy") },
			},
		)
		result <- err
	}()
	waitFor(t, func() bool { return len(controller.Events()) == 2 })
	first.Exit(0, nil)
	waitFor(t, func() bool { return len(controller.Events()) == 3 })
	select {
	case <-rolledBack:
		t.Fatal("rollback ran before the failed process exited")
	default:
	}
	invalid.Exit(1, errors.New("unhealthy process stopped"))
	if err := <-result; err == nil || !strings.Contains(err.Error(), "previous settings were restored") {
		t.Fatalf("transaction error = %v", err)
	}
	select {
	case <-rolledBack:
	default:
		t.Fatal("rollback was not called")
	}
	if launcher.Count() != 3 || !s.Status().Running || s.Status().PID != restored.PID() {
		t.Fatalf("launches=%d status=%#v", launcher.Count(), s.Status())
	}
}

func TestNextDailyRestartUsesLocalTime(t *testing.T) {
	location := time.FixedZone("UTC+8", 8*60*60)
	before := time.Date(2026, time.July, 22, 3, 59, 0, 0, location)
	next, err := nextDailyRestart(before, "04:00")
	if err != nil {
		t.Fatalf("next daily restart: %v", err)
	}
	want := time.Date(2026, time.July, 22, 4, 0, 0, 0, location)
	if !next.Equal(want) {
		t.Fatalf("next restart = %s, want %s", next, want)
	}

	atTime := time.Date(2026, time.July, 22, 4, 0, 0, 0, location)
	next, err = nextDailyRestart(atTime, "04:00")
	if err != nil {
		t.Fatalf("next daily restart at boundary: %v", err)
	}
	want = time.Date(2026, time.July, 23, 4, 0, 0, 0, location)
	if !next.Equal(want) {
		t.Fatalf("boundary next restart = %s, want %s", next, want)
	}
}

func TestNextScheduledRestartFrequencies(t *testing.T) {
	location := time.FixedZone("UTC+8", 8*60*60)
	base := ProcessConfigFrom(config.Default().ServerProcess)
	base.ScheduledRestartTime = "04:00"

	tests := []struct {
		name      string
		now       time.Time
		configure func(*ProcessConfig)
		want      time.Time
	}{
		{
			name: "every three days from start date",
			now:  time.Date(2026, time.July, 22, 3, 0, 0, 0, location),
			configure: func(value *ProcessConfig) {
				value.ScheduledRestartFrequency = config.ScheduledRestartIntervalDays
				value.ScheduledRestartIntervalDays = 3
				value.ScheduledRestartStartDate = "2026-07-20"
			},
			want: time.Date(2026, time.July, 23, 4, 0, 0, 0, location),
		},
		{
			name: "weekly after this week occurrence",
			now:  time.Date(2026, time.July, 20, 5, 0, 0, 0, location),
			configure: func(value *ProcessConfig) {
				value.ScheduledRestartFrequency = config.ScheduledRestartWeekly
				value.ScheduledRestartWeekday = int(time.Monday)
			},
			want: time.Date(2026, time.July, 27, 4, 0, 0, 0, location),
		},
		{
			name: "monthly clamps to final day",
			now:  time.Date(2026, time.February, 1, 0, 0, 0, 0, location),
			configure: func(value *ProcessConfig) {
				value.ScheduledRestartFrequency = config.ScheduledRestartMonthly
				value.ScheduledRestartDayOfMonth = 31
			},
			want: time.Date(2026, time.February, 28, 4, 0, 0, 0, location),
		},
		{
			name: "advanced cron",
			now:  time.Date(2026, time.July, 22, 3, 0, 0, 0, location),
			configure: func(value *ProcessConfig) {
				value.ScheduledRestartFrequency = config.ScheduledRestartCron
				value.ScheduledRestartCron = "15 4 * * *"
			},
			want: time.Date(2026, time.July, 22, 4, 15, 0, 0, location),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			value := base
			test.configure(&value)
			next, err := nextScheduledRestart(test.now, value)
			if err != nil {
				t.Fatalf("next scheduled restart: %v", err)
			}
			if !next.Equal(test.want) {
				t.Fatalf("next restart = %s, want %s", next, test.want)
			}
		})
	}
}

func TestScheduledRestartReusesGracefulStateMachine(t *testing.T) {
	first := newFakeProcess(601)
	second := newFakeProcess(602)
	launcher := &fakeLauncher{startFn: func(attempt int) (ManagedProcess, error) {
		if attempt == 1 {
			return first, nil
		}
		return second, nil
	}}
	controller := &fakeController{}
	value := testConfig(false)
	value.ScheduledRestartEnabled = true
	value.ScheduledRestartFrequency = config.ScheduledRestartWeekly
	value.ScheduledRestartWeekday = int(time.Wednesday)
	value.ScheduledRestartTime = "04:00"
	value.GracefulShutdownSeconds = 30
	value.GracefulShutdownMessage = "服务器将在 30 秒后重启，请提前回到安全位置。"
	s := New(value, launcher, &fakeDetector{}, controller)
	defer s.Close()
	if _, err := s.Start(); err != nil {
		t.Fatalf("initial start: %v", err)
	}

	triggeredAt := time.Date(2026, time.July, 22, 4, 0, 0, 0, time.Local)
	s.executeScheduledRestart(triggeredAt)
	if got := controller.Events(); len(got) != 2 || got[0] != "save" || got[1] != "shutdown" {
		t.Fatalf("scheduled controller order = %v, want save then shutdown", got)
	}
	seconds, message := controller.ShutdownRequest()
	if seconds != 30 || message != "服务器将在 30 秒后重启，请提前回到安全位置。" {
		t.Fatalf("scheduled shutdown = (%d, %q)", seconds, message)
	}
	status := s.Status()
	if status.LastScheduledRestartAt == nil || !status.LastScheduledRestartAt.Equal(triggeredAt) || status.LastScheduledRestartError != "" {
		t.Fatalf("scheduled status = %#v", status)
	}
	if status.ScheduledRestartFrequency != config.ScheduledRestartWeekly || status.ScheduledRestartWeekday != int(time.Wednesday) {
		t.Fatalf("scheduled frequency status = %#v", status)
	}
	if launcher.Count() != 1 {
		t.Fatal("scheduled restart must wait for the old process to exit")
	}
	first.Exit(0, nil)
	waitFor(t, func() bool { return launcher.Count() == 2 })
}

func TestScheduledRestartDoesNotStartManuallyStoppedServer(t *testing.T) {
	launcher := &fakeLauncher{startFn: func(int) (ManagedProcess, error) { return newFakeProcess(701), nil }}
	value := testConfig(false)
	value.ScheduledRestartEnabled = true
	s := New(value, launcher, &fakeDetector{}, &fakeController{})
	defer s.Close()

	s.executeScheduledRestart(time.Now())
	status := s.Status()
	if launcher.Count() != 0 || !strings.Contains(status.LastScheduledRestartError, ErrNotRunning.Error()) {
		t.Fatalf("stopped scheduled restart status = %#v, launches=%d", status, launcher.Count())
	}
}
