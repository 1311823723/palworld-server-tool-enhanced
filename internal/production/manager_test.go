package production

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/zaigie/palworld-server-tool/internal/supervisor"
	"go.etcd.io/bbolt"
)

func openProductionTestDB(t *testing.T) *bbolt.DB {
	t.Helper()
	db, err := bbolt.Open(filepath.Join(t.TempDir(), "pst.db"), 0600, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func sampleRuntimeState() RuntimeState {
	return RuntimeState{
		HeartbeatAt: time.Now().UTC(),
		Bases: []BaseCatalog{{
			BaseID:   "base-a",
			BaseName: "第一据点",
			Workstations: []Workstation{{
				ActorGUID: "station-a",
				Name:      "高级作业流水线",
				Recipes: []Recipe{{
					ID:           "Recipe_Ingot",
					Name:         "金属铸块",
					ProductID:    "Ingot",
					ProductName:  "金属铸块",
					ProductEach:  1,
					Unlocked:     true,
					MaxAvailable: 4,
					Materials: []Material{{
						ItemID:       "Ore",
						Name:         "金属矿石",
						RequiredEach: 2,
						Available:    10,
					}},
				}},
			}},
		}},
	}
}

func TestPreviewExactAndMaxAvailable(t *testing.T) {
	state := sampleRuntimeState()
	exact, err := previewFromState(state, PreviewRequest{
		BaseID: "base-a", ActorGUID: "station-a", RecipeID: "Recipe_Ingot",
		Mode: QuantityExact, Quantity: 5,
	})
	if err != nil {
		t.Fatal(err)
	}
	if exact.CanSubmit || exact.MaxAvailable != 4 || exact.Materials[0].Shortage != 0 {
		t.Fatalf("unexpected exact preview: %#v", exact)
	}
	maximum, err := previewFromState(state, PreviewRequest{
		BaseID: "base-a", ActorGUID: "station-a", RecipeID: "Recipe_Ingot",
		Mode: QuantityMaxAvailable,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !maximum.CanSubmit || maximum.AcceptedQuantity != 4 || maximum.Materials[0].Required != 8 {
		t.Fatalf("unexpected max preview: %#v", maximum)
	}
}

func TestOrderStorePersistsAndDoesNotExposeSecret(t *testing.T) {
	db := openProductionTestDB(t)
	store, err := NewStore(db)
	if err != nil {
		t.Fatal(err)
	}
	secret, err := store.Secret()
	if err != nil || len(secret) < 32 {
		t.Fatalf("secret = %q, err=%v", secret, err)
	}
	order := Order{
		ID: "45d0338c-9277-461f-a430-d84cf78f04b1", BaseID: "base-a",
		Status: OrderPending, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}
	if err := store.PutOrder(order); err != nil {
		t.Fatal(err)
	}
	reopened, err := NewStore(db)
	if err != nil {
		t.Fatal(err)
	}
	items, err := reopened.ListOrders(10)
	if err != nil || len(items) != 1 || items[0].ID != order.ID {
		t.Fatalf("persisted orders = %#v, err=%v", items, err)
	}
	if items[0].Error == secret || items[0].BridgeInstanceID == secret {
		t.Fatal("Bridge secret leaked into an order record")
	}
}

type fakeProductionSupervisor struct {
	mu        sync.Mutex
	status    supervisor.Status
	config    supervisor.ProcessConfig
	events    []string
	onStarted func() error
}

func (fake *fakeProductionSupervisor) appendEvent(value string) {
	fake.mu.Lock()
	fake.events = append(fake.events, value)
	fake.mu.Unlock()
}

func (fake *fakeProductionSupervisor) ProcessStatus() supervisor.Status {
	fake.mu.Lock()
	defer fake.mu.Unlock()
	return fake.status
}

func (fake *fakeProductionSupervisor) ProcessConfig() supervisor.ProcessConfig {
	return fake.config
}

func (fake *fakeProductionSupervisor) ApplyAndRestart(_ supervisor.RestartOptions, hooks supervisor.TransactionHooks) (supervisor.Status, error) {
	fake.appendEvent("save")
	if hooks.BeforeShutdown != nil {
		if err := hooks.BeforeShutdown(); err != nil {
			return fake.ProcessStatus(), err
		}
	}
	fake.appendEvent("shutdown")
	if hooks.AfterExit != nil {
		if err := hooks.AfterExit(); err != nil {
			return fake.ProcessStatus(), err
		}
	}
	fake.appendEvent("install")
	fake.appendEvent("start")
	if fake.onStarted != nil {
		if err := fake.onStarted(); err != nil {
			return fake.ProcessStatus(), err
		}
	}
	if hooks.HealthCheck != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		err := hooks.HealthCheck(ctx)
		cancel()
		if err != nil {
			return fake.ProcessStatus(), err
		}
		fake.appendEvent("health")
	}
	return fake.ProcessStatus(), nil
}

func (fake *fakeProductionSupervisor) RunStoppedMaintenance(action func() error) (supervisor.Status, error) {
	err := action()
	if err == nil {
		fake.appendEvent("maintenance")
	}
	return fake.ProcessStatus(), err
}

func waitForProductionInstall(t *testing.T, manager *Manager) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		manager.mu.Lock()
		installing := manager.installing
		lastError := manager.lastError
		manager.mu.Unlock()
		if !installing {
			if lastError != "" {
				t.Fatalf("installation failed: %s", lastError)
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("timed out waiting for installation")
}

func TestRunningInstallOrder(t *testing.T) {
	installer, processConfig, paths := prepareInstallerTest(t, true)
	fake := &fakeProductionSupervisor{
		status: supervisor.Status{Running: true, State: supervisor.StateRunning},
		config: processConfig,
	}
	manager, err := NewManager(openProductionTestDB(t), fake, func(string) error {
		fake.appendEvent("backup")
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	manager.SetInstallerForTesting(installer)
	fake.onStarted = func() error {
		if err := os.MkdirAll(paths.IPC, 0700); err != nil {
			return err
		}
		state := RuntimeState{
			InstanceID:      "instance-a",
			BridgeVersion:   BridgeVersion,
			ProtocolVersion: BridgeProtocolVersion,
			HeartbeatAt:     time.Now().UTC().Add(time.Second),
			Compatible:      true,
			Capabilities:    RuntimeCapabilities{Catalog: true, Orders: true, Cancel: true},
		}
		data, _ := json.Marshal(state)
		return os.WriteFile(filepath.Join(paths.IPC, "state.json"), data, 0600)
	}
	if err := manager.BeginInstall(InstallRequest{Confirmation: "INSTALL"}, false, false); err != nil {
		t.Fatal(err)
	}
	waitForProductionInstall(t, manager)
	fake.mu.Lock()
	events := append([]string(nil), fake.events...)
	fake.mu.Unlock()
	want := []string{"save", "backup", "shutdown", "install", "start", "health"}
	if len(events) != len(want) {
		t.Fatalf("events = %v, want %v", events, want)
	}
	for index := range want {
		if events[index] != want[index] {
			t.Fatalf("events = %v, want %v", events, want)
		}
	}
}

func TestStoppedInstallBacksUpWithoutStartingServer(t *testing.T) {
	installer, processConfig, paths := prepareInstallerTest(t, true)
	fake := &fakeProductionSupervisor{
		status: supervisor.Status{State: supervisor.StateStopped},
		config: processConfig,
	}
	manager, err := NewManager(openProductionTestDB(t), fake, func(string) error {
		fake.appendEvent("backup")
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	manager.SetInstallerForTesting(installer)
	if err := manager.BeginInstall(InstallRequest{Confirmation: "INSTALL"}, false, false); err != nil {
		t.Fatal(err)
	}
	waitForProductionInstall(t, manager)
	if _, err := os.Stat(filepath.Join(paths.Target, "Info.json")); err != nil {
		t.Fatalf("Bridge was not installed: %v", err)
	}
	fake.mu.Lock()
	events := append([]string(nil), fake.events...)
	fake.mu.Unlock()
	if len(events) != 2 || events[0] != "backup" || events[1] != "maintenance" {
		t.Fatalf("stopped maintenance events = %v", events)
	}
}

func TestRecheckDetectsManualBridgeAndClearsPreviousFailure(t *testing.T) {
	installer, processConfig, paths := prepareInstallerTest(t, true)
	configuredRoot := filepath.Join(paths.PalServerRoot, "Mods", "CustomWorkshop")
	if err := os.MkdirAll(filepath.Dir(paths.Settings), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(paths.Settings, []byte(
		"WorkshopRootDir="+configuredRoot+"\r\n"+
			"bGlobalEnableMod=true\r\n"+
			"ActiveModList="+BridgeName+"\r\n",
	), 0644); err != nil {
		t.Fatal(err)
	}
	resolved, err := installer.paths(processConfig)
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := readAndVerifyManifest(resolved.Source)
	if err != nil {
		t.Fatal(err)
	}
	if err := copyManifestFiles(resolved.Source, resolved.Target, manifest); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(resolved.IPC, 0700); err != nil {
		t.Fatal(err)
	}
	runtimeState := RuntimeState{
		InstanceID:      "manual-instance",
		BridgeVersion:   BridgeVersion,
		ProtocolVersion: BridgeProtocolVersion,
		HeartbeatAt:     time.Now().UTC(),
		Compatible:      true,
		Capabilities:    RuntimeCapabilities{Catalog: true, Orders: true},
	}
	stateData, err := json.Marshal(runtimeState)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(resolved.IPC, "state.json"), stateData, 0600); err != nil {
		t.Fatal(err)
	}

	fake := &fakeProductionSupervisor{
		status: supervisor.Status{Running: true, State: supervisor.StateRunning},
		config: processConfig,
	}
	manager, err := NewManager(openProductionTestDB(t), fake, func(string) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	manager.SetInstallerForTesting(installer)
	manager.mu.Lock()
	manager.lastError = "上一轮安装失败"
	manager.mu.Unlock()

	status := manager.RecheckBridge()
	if status.State != BridgeHealthy {
		t.Fatalf("recheck state = %s, want %s (%s)", status.State, BridgeHealthy, status.Message)
	}
	if status.LastError != "" {
		t.Fatalf("recheck retained stale error: %q", status.LastError)
	}
	if status.ManualInstall == nil || status.ManualInstall.TargetDirectory != resolved.Target {
		t.Fatalf("recheck target = %#v, want %q", status.ManualInstall, resolved.Target)
	}
}

func TestInstallRejectsExternalProcessAndInvalidConfirmation(t *testing.T) {
	installer, processConfig, _ := prepareInstallerTest(t, true)
	fake := &fakeProductionSupervisor{
		status: supervisor.Status{Running: true, ExternalProcess: true},
		config: processConfig,
	}
	manager, err := NewManager(openProductionTestDB(t), fake, func(string) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	manager.SetInstallerForTesting(installer)
	if err := manager.BeginInstall(InstallRequest{Confirmation: "wrong"}, false, false); !errors.Is(err, ErrInvalidInstall) {
		t.Fatalf("invalid confirmation error = %v", err)
	}
	if err := manager.BeginInstall(InstallRequest{Confirmation: "INSTALL"}, false, false); !errors.Is(err, ErrExternalProcess) {
		t.Fatalf("external process error = %v", err)
	}
}

func TestWaitingMaterialOrderCanBeCancelled(t *testing.T) {
	root := t.TempDir()
	executable := filepath.Join(root, "PalServer.exe")
	if err := os.WriteFile(executable, []byte("test"), 0644); err != nil {
		t.Fatal(err)
	}
	fake := &fakeProductionSupervisor{
		status: supervisor.Status{Running: true, State: supervisor.StateRunning},
		config: supervisor.ProcessConfig{Enabled: true, ExecutablePath: executable},
	}
	manager, err := NewManager(openProductionTestDB(t), fake, func(string) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	order := Order{
		ID:             "45d0338c-9277-461f-a430-d84cf78f04b1",
		BaseID:         "base-a",
		ActorGUID:      "station-a",
		RecipeID:       "Recipe_Ingot",
		QuantityMode:   QuantityExact,
		AcceptedAmount: 1,
		Status:         OrderWaitingMaterials,
		CreatedAt:      time.Now().UTC(),
		UpdatedAt:      time.Now().UTC(),
	}
	if err := manager.store.PutOrder(order); err != nil {
		t.Fatal(err)
	}
	cancelled, err := manager.CancelOrder(order.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !cancelled.CancellationRequested {
		t.Fatal("waiting order should record the cancellation request")
	}
	requestPath := filepath.Join(root, "Pal", "Saved", BridgeName, "requests", order.ID+".json")
	data, err := os.ReadFile(requestPath)
	if err != nil {
		t.Fatal(err)
	}
	var request OrderRequest
	if err := json.Unmarshal(data, &request); err != nil {
		t.Fatal(err)
	}
	if request.Action != "cancel" {
		t.Fatalf("request action = %q, want cancel", request.Action)
	}
}
