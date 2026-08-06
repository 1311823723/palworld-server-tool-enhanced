package api

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/zaigie/palworld-server-tool/internal/auth"
	"github.com/zaigie/palworld-server-tool/internal/config"
	"github.com/zaigie/palworld-server-tool/internal/supervisor"
)

type fakeServerProcessManager struct {
	status supervisor.Status
}

func (manager *fakeServerProcessManager) ProcessStatus() supervisor.Status { return manager.status }
func (manager *fakeServerProcessManager) SaveWorld() error                 { return nil }
func (manager *fakeServerProcessManager) Start() (supervisor.Status, error) {
	if manager.status.Running {
		return manager.status, supervisor.ErrConflict
	}
	manager.status.Running = true
	return manager.status, nil
}
func (manager *fakeServerProcessManager) Restart(supervisor.RestartOptions) (supervisor.Status, error) {
	return manager.status, nil
}
func (manager *fakeServerProcessManager) Stop(supervisor.StopOptions) (supervisor.Status, error) {
	return manager.status, nil
}
func (manager *fakeServerProcessManager) SetWatchdog(enabled bool) supervisor.Status {
	manager.status.WatchdogEnabled = enabled
	return manager.status
}
func (manager *fakeServerProcessManager) UpdateConfig(config.ServerProcessConfig) {}
func (manager *fakeServerProcessManager) ServerUpdateStatus() supervisor.UpdateStatus {
	return supervisor.UpdateStatus{}
}
func (manager *fakeServerProcessManager) CheckServerUpdate() (supervisor.UpdateStatus, error) {
	return supervisor.UpdateStatus{}, nil
}
func (manager *fakeServerProcessManager) ApplyServerUpdate(supervisor.RestartOptions) (supervisor.Status, error) {
	return manager.status, nil
}

func newAuthenticatedProcessRouter(t *testing.T, manager ServerProcessManager) (*gin.Engine, string, *config.Store) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	store, err := config.Open(filepath.Join(t.TempDir(), "config.db"))
	if err != nil {
		t.Fatalf("open config store: %v", err)
	}
	config.SetCurrent(store)
	if err := store.Initialize("admin-password"); err != nil {
		store.Close()
		t.Fatalf("initialize administrator: %v", err)
	}
	token, err := auth.GenerateToken()
	if err != nil {
		store.Close()
		t.Fatalf("generate token: %v", err)
	}
	router := gin.New()
	RegisterRouterWithSupervisor(router, nil, manager)
	return router, token, store
}

func TestServerProcessAPIRequiresAdministratorJWT(t *testing.T) {
	router, _, store := newAuthenticatedProcessRouter(t, &fakeServerProcessManager{})
	defer store.Close()
	response := performJSONRequest(router, http.MethodGet, "/api/server/process", nil, "")
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated process status code = %d, want 401", response.Code)
	}
}

func TestNewOperationsAPIsRequireAdministratorJWT(t *testing.T) {
	router, _, store := newAuthenticatedProcessRouter(t, &fakeServerProcessManager{})
	defer store.Close()
	for _, path := range []string{"/api/server/update", "/api/logs", "/api/audit", "/api/player-progress"} {
		response := performJSONRequest(router, http.MethodGet, path, nil, "")
		if response.Code != http.StatusUnauthorized {
			t.Fatalf("unauthenticated %s status code = %d, want 401", path, response.Code)
		}
	}
}

func TestInventoryAPIRequiresAdministratorJWT(t *testing.T) {
	router, _, store := newAuthenticatedProcessRouter(t, &fakeServerProcessManager{})
	defer store.Close()
	response := performJSONRequest(router, http.MethodGet, "/api/inventory/summary", nil, "")
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated inventory status code = %d, want 401", response.Code)
	}
}

func TestBreedingFarmAPIRequiresAdministratorJWT(t *testing.T) {
	router, _, store := newAuthenticatedProcessRouter(t, &fakeServerProcessManager{})
	defer store.Close()
	for _, path := range []string{
		"/api/breeding-farms",
		"/api/breeding-farms/capabilities",
		"/api/breeding-farms/events",
		"/api/breeding-farms/events/unread",
		"/api/breeding-farms/notification-config",
	} {
		response := performJSONRequest(router, http.MethodGet, path, nil, "")
		if response.Code != http.StatusUnauthorized {
			t.Fatalf("unauthenticated %s status code = %d, want 401", path, response.Code)
		}
	}
}

func TestBaseAliasAPIRequiresAdministratorJWT(t *testing.T) {
	router, _, store := newAuthenticatedProcessRouter(t, &fakeServerProcessManager{})
	defer store.Close()
	for _, request := range []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/base-camps/aliases"},
		{http.MethodPut, "/api/base-camps/base-a/alias"},
		{http.MethodDelete, "/api/base-camps/base-a/alias"},
	} {
		response := performJSONRequest(router, request.method, request.path, map[string]any{"name": "测试据点"}, "")
		if response.Code != http.StatusUnauthorized {
			t.Fatalf("unauthenticated %s %s status code = %d, want 401", request.method, request.path, response.Code)
		}
	}
}

func TestBreedingNotificationConfigDoesNotLeakSecretsAndConfirmsAllFarms(t *testing.T) {
	router, token, store := newAuthenticatedProcessRouter(t, &fakeServerProcessManager{})
	defer store.Close()
	settings := store.Config()
	settings.Rcon.Password = "rcon-breeding-secret"
	settings.Rest.Password = "rest-breeding-secret"
	if err := store.Update(settings, ""); err != nil {
		t.Fatalf("store secrets: %v", err)
	}

	response := performJSONRequest(router, http.MethodGet, "/api/breeding-farms/notification-config", nil, token)
	if response.Code != http.StatusOK {
		t.Fatalf("get breeding config code = %d: %s", response.Code, response.Body.String())
	}
	if strings.Contains(response.Body.String(), "breeding-secret") {
		t.Fatalf("breeding configuration leaked a password: %s", response.Body.String())
	}

	response = performJSONRequest(router, http.MethodPut, "/api/breeding-farms/notification-config", map[string]any{
		"enabled": true, "selection_mode": "all", "minimum_ready_eggs": 1, "history_retention_days": 30,
	}, token)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("monitor all without confirmation code = %d, want 400: %s", response.Code, response.Body.String())
	}
}

func TestPublicInventorySummaryIsDisabledByDefault(t *testing.T) {
	router, _, store := newAuthenticatedProcessRouter(t, &fakeServerProcessManager{})
	defer store.Close()
	response := performJSONRequest(router, http.MethodGet, "/api/inventory/public-summary", nil, "")
	if response.Code != http.StatusForbidden {
		t.Fatalf("public inventory summary code = %d, want 403", response.Code)
	}
}

func TestWorldSettingsAPIRequiresAdministratorJWT(t *testing.T) {
	router, _, store := newAuthenticatedProcessRouter(t, &fakeServerProcessManager{})
	defer store.Close()
	for _, path := range []string{"/api/world-settings/schema", "/api/world-settings", "/api/world-settings/backups"} {
		response := performJSONRequest(router, http.MethodGet, path, nil, "")
		if response.Code != http.StatusUnauthorized {
			t.Fatalf("unauthenticated %s status code = %d, want 401", path, response.Code)
		}
	}
}

func TestServerProcessStartConflictReturns409(t *testing.T) {
	manager := &fakeServerProcessManager{status: supervisor.Status{Running: true}}
	router, token, store := newAuthenticatedProcessRouter(t, manager)
	defer store.Close()
	response := performJSONRequest(router, http.MethodPost, "/api/server/start", nil, token)
	if response.Code != http.StatusConflict {
		t.Fatalf("duplicate start code = %d, want 409: %s", response.Code, response.Body.String())
	}
}

func TestSaveServerRequiresRunningProcess(t *testing.T) {
	manager := &fakeServerProcessManager{status: supervisor.Status{State: supervisor.StateStopped}}
	router, token, store := newAuthenticatedProcessRouter(t, manager)
	defer store.Close()
	response := performJSONRequest(router, http.MethodPost, "/api/server/save", nil, token)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("save while stopped code = %d, want 400: %s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), supervisor.ErrNotRunning.Error()) {
		t.Fatalf("save while stopped body = %s", response.Body.String())
	}
}

func TestConfigRejectsInvalidExecutableAndCommandInjection(t *testing.T) {
	router, token, store := newAuthenticatedProcessRouter(t, &fakeServerProcessManager{})
	defer store.Close()

	settings := store.Config()
	settings.ServerProcess.Enabled = true
	settings.ServerProcess.ExecutablePath = filepath.Join(t.TempDir(), "PalServer.exe")
	response := performJSONRequest(router, http.MethodPut, "/api/config", map[string]any{"settings": settings}, token)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("missing executable code = %d, want 400: %s", response.Code, response.Body.String())
	}

	executable := filepath.Join(t.TempDir(), "PalServer.exe")
	if err := os.WriteFile(executable, []byte("fixture"), 0600); err != nil {
		t.Fatalf("create executable fixture: %v", err)
	}
	settings.ServerProcess.ExecutablePath = executable
	settings.ServerProcess.Arguments = []string{"-port=8211", "foo|cmd.exe"}
	response = performJSONRequest(router, http.MethodPut, "/api/config", map[string]any{"settings": settings}, token)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("unsafe argument code = %d, want 400: %s", response.Code, response.Body.String())
	}
}

func TestConfigurationAPIHidesAndPreservesPasswords(t *testing.T) {
	router, token, store := newAuthenticatedProcessRouter(t, &fakeServerProcessManager{})
	defer store.Close()
	settings := store.Config()
	settings.Rcon.Password = "rcon-secret"
	settings.Rest.Password = "rest-secret"
	if err := store.Update(settings, ""); err != nil {
		t.Fatalf("store secrets: %v", err)
	}

	response := performJSONRequest(router, http.MethodGet, "/api/config", nil, token)
	if response.Code != http.StatusOK {
		t.Fatalf("get config code = %d: %s", response.Code, response.Body.String())
	}
	if strings.Contains(response.Body.String(), "rcon-secret") || strings.Contains(response.Body.String(), "rest-secret") {
		t.Fatalf("configuration response leaked a password: %s", response.Body.String())
	}

	redacted := settings.Redacted()
	redacted.Rcon.Address = "127.0.0.1:25576"
	response = performJSONRequest(router, http.MethodPut, "/api/config", map[string]any{"settings": redacted}, token)
	if response.Code != http.StatusOK {
		t.Fatalf("update redacted config code = %d: %s", response.Code, response.Body.String())
	}
	persisted := store.Config()
	if persisted.Rcon.Password != "rcon-secret" || persisted.Rest.Password != "rest-secret" {
		t.Fatalf("blank password fields did not preserve secrets")
	}
}
