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
	"github.com/zaigie/palworld-server-tool/internal/worldsettings"
)

func TestWorldSettingsAPINeverReturnsPasswordValues(t *testing.T) {
	root := t.TempDir()
	executable := filepath.Join(root, "PalServer.exe")
	if err := os.WriteFile(executable, []byte("fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	settingsDirectory := filepath.Join(root, "Pal", "Saved", "Config", "WindowsServer")
	if err := os.MkdirAll(settingsDirectory, 0700); err != nil {
		t.Fatal(err)
	}
	ini := "[/Script/Pal.PalGameWorldSettings]\nOptionSettings=(ServerName=\"Test\",AdminPassword=\"YOUR_ADMIN_PASSWORD\",ServerPassword=\"YOUR_ADMIN_PASSWORD\")\n"
	if err := os.WriteFile(filepath.Join(settingsDirectory, "PalWorldSettings.ini"), []byte(ini), 0600); err != nil {
		t.Fatal(err)
	}
	store, err := config.Open(filepath.Join(root, "config.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	config.SetCurrent(store)
	if err := store.Initialize("YOUR_ADMIN_PASSWORD"); err != nil {
		t.Fatal(err)
	}
	value := store.Config()
	value.ServerProcess.Enabled = true
	value.ServerProcess.ExecutablePath = executable
	value.ServerProcess.WorkingDirectory = root
	if err := store.Update(value, ""); err != nil {
		t.Fatal(err)
	}
	token, err := auth.GenerateToken()
	if err != nil {
		t.Fatal(err)
	}
	manager := worldsettings.NewManager(store, nil, nil, nil)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	RegisterRouterWithManagers(router, nil, nil, manager)
	response := performJSONRequest(router, http.MethodGet, "/api/world-settings", nil, token)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if strings.Contains(response.Body.String(), "YOUR_ADMIN_PASSWORD") {
		t.Fatalf("world settings response leaked a password: %s", response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"is_set":true`) || !strings.Contains(response.Body.String(), `"value":null`) {
		t.Fatalf("world settings response did not expose redacted secret state: %s", response.Body.String())
	}
}

func TestWorldSettingsMutationsRequireConfirmationWords(t *testing.T) {
	gin.SetMode(gin.TestMode)
	manager := worldsettings.NewManager(nil, nil, nil, nil)
	router := gin.New()
	router.POST("/apply", applyWorldSettings(manager))
	router.POST("/backups/:backup_id/restore", restoreWorldSettingsBackup(manager))

	applyResponse := performJSONRequest(router, http.MethodPost, "/apply", map[string]any{
		"changes": map[string]any{"ServerName": "Changed"},
	}, "")
	if applyResponse.Code != http.StatusBadRequest || !strings.Contains(applyResponse.Body.String(), "应用") {
		t.Fatalf("apply confirmation status=%d body=%s", applyResponse.Code, applyResponse.Body.String())
	}

	restoreResponse := performJSONRequest(router, http.MethodPost, "/backups/test/restore", map[string]any{
		"confirmation": "wrong",
	}, "")
	if restoreResponse.Code != http.StatusBadRequest || !strings.Contains(restoreResponse.Body.String(), "恢复") {
		t.Fatalf("restore confirmation status=%d body=%s", restoreResponse.Code, restoreResponse.Body.String())
	}
}
