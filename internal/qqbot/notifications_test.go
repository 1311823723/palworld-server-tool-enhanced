package qqbot

import (
	"testing"
	"time"

	"github.com/zaigie/palworld-server-tool/internal/supervisor"
)

func TestShouldNotifyServerCrashSkipsPlannedExit(t *testing.T) {
	now := time.Now().UTC()
	status := supervisor.Status{
		Running:         false,
		DesiredRunning:  true,
		LastExitAt:      &now,
		LastExitPlanned: true,
		LastExitCode:    0,
	}
	if shouldNotifyServerCrash(supervisor.Status{Running: true}, status) {
		t.Fatal("planned exit must not be reported as a crash")
	}

	status.LastExitPlanned = false
	if !shouldNotifyServerCrash(supervisor.Status{Running: true}, status) {
		t.Fatal("unexpected exit must be reported as a crash")
	}
}

func TestShouldNotifyWatchdogRestartSkipsPlannedRestart(t *testing.T) {
	previous := supervisor.Status{RestartCount: 1}
	if shouldNotifyWatchdogRestart(previous, supervisor.Status{RestartCount: 2, LastExitPlanned: true}) {
		t.Fatal("planned restart must not be reported as a watchdog restart")
	}
	if !shouldNotifyWatchdogRestart(previous, supervisor.Status{RestartCount: 2, LastExitPlanned: false}) {
		t.Fatal("unexpected crash restart must be reported as a watchdog restart")
	}
}

func TestShouldNotifyScheduledRestart(t *testing.T) {
	now := time.Now().UTC()
	previous := supervisor.Status{}
	current := supervisor.Status{LastScheduledRestartAt: &now}
	if !shouldNotifyScheduledRestart(previous, current) {
		t.Fatal("first scheduled restart must be reported")
	}
	if shouldNotifyScheduledRestart(current, current) {
		t.Fatal("unchanged scheduled restart must not be reported twice")
	}
	next := now.Add(time.Minute)
	if !shouldNotifyScheduledRestart(current, supervisor.Status{LastScheduledRestartAt: &next}) {
		t.Fatal("new scheduled restart must be reported")
	}
}
