package supervisor

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

type fakeUpdater struct {
	mu           sync.Mutex
	checks       int
	applies      int
	installed    string
	latest       string
	checkErr     error
	applyErr     error
	applyStarted chan struct{}
}

func (updater *fakeUpdater) Check(context.Context, ProcessConfig) (string, string, error) {
	updater.mu.Lock()
	defer updater.mu.Unlock()
	updater.checks++
	return updater.installed, updater.latest, updater.checkErr
}

func (updater *fakeUpdater) Apply(context.Context, ProcessConfig) (string, error) {
	updater.mu.Lock()
	updater.applies++
	updater.mu.Unlock()
	if updater.applyStarted != nil {
		close(updater.applyStarted)
	}
	return updater.latest, updater.applyErr
}

func TestServerUpdateWaitsForExitThenRestarts(t *testing.T) {
	first := newFakeProcess(801)
	second := newFakeProcess(802)
	launcher := &fakeLauncher{startFn: func(attempt int) (ManagedProcess, error) {
		if attempt == 1 {
			return first, nil
		}
		return second, nil
	}}
	value := testConfig(true)
	value.SteamCMDPath = "steamcmd.exe"
	updater := &fakeUpdater{installed: "100", latest: "200", applyStarted: make(chan struct{})}
	supervisor := New(value, launcher, &fakeDetector{}, &fakeController{})
	supervisor.SetUpdaterForTesting(updater)
	defer supervisor.Close()
	if _, err := supervisor.Start(); err != nil {
		t.Fatal(err)
	}

	result := make(chan error, 1)
	go func() {
		_, err := supervisor.ApplyServerUpdate(RestartOptions{Message: "更新", RestartDelay: 0})
		result <- err
	}()
	waitFor(t, func() bool { return supervisor.Status().State == StateStopping })
	select {
	case <-updater.applyStarted:
		t.Fatal("update started before the old process exited")
	default:
	}
	first.Exit(0, nil)
	select {
	case <-updater.applyStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("update did not start after process exit")
	}
	if err := <-result; err != nil {
		t.Fatalf("apply update: %v", err)
	}
	if launcher.Count() != 2 || !supervisor.Status().Running {
		t.Fatalf("update did not restart exactly once: launches=%d status=%#v", launcher.Count(), supervisor.Status())
	}
}

func TestServerUpdateFailureDisablesWatchdog(t *testing.T) {
	first := newFakeProcess(811)
	launcher := &fakeLauncher{startFn: func(int) (ManagedProcess, error) { return first, nil }}
	value := testConfig(true)
	value.SteamCMDPath = "steamcmd.exe"
	supervisor := New(value, launcher, &fakeDetector{}, &fakeController{})
	supervisor.SetUpdaterForTesting(&fakeUpdater{applyErr: errors.New("update failed")})
	defer supervisor.Close()
	if _, err := supervisor.Start(); err != nil {
		t.Fatal(err)
	}
	result := make(chan error, 1)
	go func() {
		_, err := supervisor.ApplyServerUpdate(RestartOptions{Message: "更新"})
		result <- err
	}()
	waitFor(t, func() bool { return supervisor.Status().State == StateStopping })
	first.Exit(0, nil)
	if err := <-result; err == nil {
		t.Fatal("failed update unexpectedly succeeded")
	}
	status := supervisor.Status()
	if status.State != StateUpdateFailed || status.WatchdogEnabled || status.DesiredRunning || status.Running {
		t.Fatalf("unsafe failed update state: %#v", status)
	}
}

func TestServerUpdateConflictsWithExistingRestartWithoutDisablingWatchdog(t *testing.T) {
	first := newFakeProcess(821)
	launcher := &fakeLauncher{startFn: func(int) (ManagedProcess, error) { return first, nil }}
	value := testConfig(true)
	value.SteamCMDPath = "steamcmd.exe"
	supervisor := New(value, launcher, &fakeDetector{}, &fakeController{})
	supervisor.SetUpdaterForTesting(&fakeUpdater{})
	defer supervisor.Close()
	if _, err := supervisor.Start(); err != nil {
		t.Fatal(err)
	}
	if _, err := supervisor.Restart(RestartOptions{Message: "重启"}); err != nil {
		t.Fatal(err)
	}
	if _, err := supervisor.ApplyServerUpdate(RestartOptions{Message: "更新"}); !errors.Is(err, ErrConflict) {
		t.Fatalf("update during restart error = %v, want conflict", err)
	}
	status := supervisor.Status()
	if !status.WatchdogEnabled || status.State != StateStopping {
		t.Fatalf("conflicting update changed active restart state: %#v", status)
	}
}
