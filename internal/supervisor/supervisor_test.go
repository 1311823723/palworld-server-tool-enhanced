package supervisor

import (
	"context"
	"errors"
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
	mu           sync.Mutex
	events       []string
	saveErr      error
	shutdownErr  error
	saveStarted  chan struct{}
	saveContinue chan struct{}
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

func (controller *fakeController) Shutdown(_ int, _ string) error {
	controller.mu.Lock()
	defer controller.mu.Unlock()
	controller.events = append(controller.events, "shutdown")
	return controller.shutdownErr
}

func (controller *fakeController) Events() []string {
	controller.mu.Lock()
	defer controller.mu.Unlock()
	return append([]string(nil), controller.events...)
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
