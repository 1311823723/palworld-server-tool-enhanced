package worldsettings

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/zaigie/palworld-server-tool/internal/config"
	"github.com/zaigie/palworld-server-tool/internal/supervisor"
)

type fakeConfigStore struct {
	mu    sync.Mutex
	value config.Config
}

func (store *fakeConfigStore) Config() config.Config {
	store.mu.Lock()
	defer store.mu.Unlock()
	return store.value
}

func (store *fakeConfigStore) Update(value config.Config, _ string) error {
	store.mu.Lock()
	defer store.mu.Unlock()
	store.value = value
	return nil
}

type fakeProcessManager struct {
	started chan struct{}
	resume  chan struct{}
	fail    bool
}

func (manager *fakeProcessManager) ProcessStatus() supervisor.Status {
	return supervisor.Status{Running: true, State: supervisor.StateRunning}
}

func (manager *fakeProcessManager) ApplyAndRestart(_ supervisor.RestartOptions, hooks supervisor.TransactionHooks) (supervisor.Status, error) {
	if manager.started != nil {
		close(manager.started)
	}
	if manager.resume != nil {
		<-manager.resume
	}
	if err := hooks.AfterExit(); err != nil {
		return supervisor.Status{}, err
	}
	if manager.fail {
		if hooks.Rollback != nil {
			_ = hooks.Rollback()
		}
		return supervisor.Status{}, errors.New("simulated start failure")
	}
	if hooks.HealthCheck != nil {
		if err := hooks.HealthCheck(context.Background()); err != nil {
			_ = hooks.Rollback()
			return supervisor.Status{}, err
		}
	}
	return supervisor.Status{Running: true, State: supervisor.StateRunning}, nil
}

func newManagerFixture(t *testing.T, process ProcessManager) (*Manager, *fakeConfigStore, string) {
	t.Helper()
	root := t.TempDir()
	settingsDirectory := filepath.Join(root, "Pal", "Saved", "Config", "WindowsServer")
	if err := os.MkdirAll(settingsDirectory, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(settingsDirectory, "PalWorldSettings.ini")
	contents := sectionHeader + "\nOptionSettings=(ServerName=\"Before\",AdminPassword=\"YOUR_ADMIN_PASSWORD\",RESTAPIEnabled=True,RESTAPIPort=8212,RCONPort=25575,FutureUnknown=(A=1,B=\"x,y\"))\n"
	if err := os.WriteFile(path, []byte(contents), 0600); err != nil {
		t.Fatal(err)
	}
	value := config.Default()
	value.ServerProcess.Enabled = true
	value.ServerProcess.ExecutablePath = filepath.Join(root, "PalServer.exe")
	value.ServerProcess.WorkingDirectory = root
	value.Rest.Password = "YOUR_ADMIN_PASSWORD"
	value.Rcon.Password = "YOUR_ADMIN_PASSWORD"
	store := &fakeConfigStore{value: value}
	return NewManager(store, process, func(context.Context) error { return nil }, nil), store, path
}

func TestCurrentRedactsSecretsAndPreservesUnknownKeyNames(t *testing.T) {
	manager, _, _ := newManagerFixture(t, &fakeProcessManager{})
	current, err := manager.Current()
	if err != nil {
		t.Fatal(err)
	}
	if !current.Secrets["AdminPassword"].IsSet || current.Secrets["AdminPassword"].Value != nil {
		t.Fatalf("secret state = %#v", current.Secrets["AdminPassword"])
	}
	if strings.Contains(toJSON(t, current), "YOUR_ADMIN_PASSWORD") {
		t.Fatal("current settings leaked a secret")
	}
	if current.UnknownKeyCount != 1 || current.UnknownKeys[0] != "FutureUnknown" {
		t.Fatalf("unknown keys = %#v", current.UnknownKeys)
	}
}

func TestValidateSecretDiffNeverContainsValues(t *testing.T) {
	manager, _, _ := newManagerFixture(t, &fakeProcessManager{})
	result, err := manager.Validate(ChangeRequest{Secrets: map[string]string{"AdminPassword": "YOUR_ADMIN_PASSWORD_UPDATED"}})
	if err != nil {
		t.Fatal(err)
	}
	encoded := toJSON(t, result)
	if strings.Contains(encoded, "YOUR_ADMIN_PASSWORD") || strings.Contains(encoded, "YOUR_ADMIN_PASSWORD_UPDATED") {
		t.Fatalf("validation leaked secret: %s", encoded)
	}
	if len(result.Differences) != 1 || !result.Differences[0].Secret {
		t.Fatalf("differences = %#v", result.Differences)
	}
}

func TestApplyCreatesBackupAndPreservesUnknownFields(t *testing.T) {
	manager, store, path := newManagerFixture(t, &fakeProcessManager{})
	result, err := manager.Apply(ChangeRequest{Changes: map[string]any{"ServerName": "After", "RESTAPIPort": float64(9000)}})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Success || result.BackupID == "" {
		t.Fatalf("result = %#v", result)
	}
	data, _ := os.ReadFile(path)
	text := string(data)
	if !strings.Contains(text, `ServerName="After"`) || !strings.Contains(text, `FutureUnknown=(A=1,B="x,y")`) {
		t.Fatalf("written settings = %s", text)
	}
	if !strings.HasSuffix(store.Config().Rest.Address, ":9000") {
		t.Fatalf("REST address = %s", store.Config().Rest.Address)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(path), ".pst-backups", result.BackupID)); err != nil {
		t.Fatalf("backup missing: %v", err)
	}
}

func TestApplyFailureRestoresFileAndRuntimeConfig(t *testing.T) {
	manager, store, path := newManagerFixture(t, &fakeProcessManager{fail: true})
	original, _ := os.ReadFile(path)
	_, err := manager.Apply(ChangeRequest{Changes: map[string]any{"RESTAPIPort": float64(9001)}})
	if err == nil {
		t.Fatal("expected simulated failure")
	}
	restored, _ := os.ReadFile(path)
	if string(restored) != string(original) {
		t.Fatal("settings file was not restored")
	}
	if !strings.HasSuffix(store.Config().Rest.Address, ":8212") {
		t.Fatalf("REST config was not restored: %s", store.Config().Rest.Address)
	}
}

func TestConcurrentApplyReturnsBusy(t *testing.T) {
	process := &fakeProcessManager{started: make(chan struct{}), resume: make(chan struct{})}
	manager, _, _ := newManagerFixture(t, process)
	first := make(chan error, 1)
	go func() {
		_, err := manager.Apply(ChangeRequest{Changes: map[string]any{"ServerName": "First"}})
		first <- err
	}()
	<-process.started
	if _, err := manager.Apply(ChangeRequest{Changes: map[string]any{"ServerName": "Second"}}); !errors.Is(err, ErrBusy) {
		t.Fatalf("second apply error = %v, want ErrBusy", err)
	}
	close(process.resume)
	if err := <-first; err != nil {
		t.Fatal(err)
	}
}

func TestBackupIDsRejectPathTraversal(t *testing.T) {
	manager, _, _ := newManagerFixture(t, &fakeProcessManager{})
	for _, id := range []string{"../PalWorldSettings.ini", "..\\secret.ini", "/tmp/file.ini", "not-a-backup.ini"} {
		if _, err := manager.backupPath(id); err == nil {
			t.Fatalf("backup id %q was accepted", id)
		}
	}
}

func TestRestoreBackupRestoresUnknownFieldsExactly(t *testing.T) {
	manager, _, path := newManagerFixture(t, &fakeProcessManager{})
	original, _ := os.ReadFile(path)
	id, err := manager.createBackup(original)
	if err != nil {
		t.Fatal(err)
	}
	changed := sectionHeader + "\nOptionSettings=(ServerName=\"Changed\",AdminPassword=\"YOUR_ADMIN_PASSWORD\",RESTAPIEnabled=True,RESTAPIPort=8212,RCONPort=25575,OtherUnknown=99)\n"
	if err := os.WriteFile(path, []byte(changed), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.RestoreBackup(id, 0, 0, "restore"); err != nil {
		t.Fatal(err)
	}
	restored, _ := os.ReadFile(path)
	if string(restored) != string(original) {
		t.Fatalf("restored file differs:\n%s", restored)
	}
}

func toJSON(t *testing.T, value any) string {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
