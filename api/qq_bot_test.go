package api

import (
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/zaigie/palworld-server-tool/internal/auth"
	"github.com/zaigie/palworld-server-tool/internal/config"
	"github.com/zaigie/palworld-server-tool/internal/qqbot"
	"go.etcd.io/bbolt"
)

func TestQQBotAPIsRequireAdministratorAndNeverReturnSecrets(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store, err := config.Open(filepath.Join(t.TempDir(), "config.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	config.SetCurrent(store)
	if err := store.Initialize("admin-password"); err != nil {
		t.Fatal(err)
	}
	value := store.Config().QQBot
	value.OneBotToken = "onebot-never-return"
	value.AI.APIKey = "deepseek-never-return"
	if err := store.SetQQBot(value); err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	RegisterRouter(router, nil)
	unauthorized := performJSONRequest(router, http.MethodGet, "/api/qq-bot/config", nil, "")
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated QQ config status = %d, want 401", unauthorized.Code)
	}
	token, err := auth.GenerateToken()
	if err != nil {
		t.Fatal(err)
	}
	response := performJSONRequest(router, http.MethodGet, "/api/qq-bot/config", nil, token)
	if response.Code != http.StatusOK {
		t.Fatalf("QQ config status = %d: %s", response.Code, response.Body.String())
	}
	body := response.Body.String()
	if strings.Contains(body, "never-return") || !strings.Contains(body, `"token_is_set":true`) || !strings.Contains(body, `"api_key_is_set":true`) || !strings.Contains(body, `"persona":{"enabled":true`) || !strings.Contains(body, `"serious_on_error":true}`) {
		t.Fatalf("QQ config response did not redact secrets correctly: %s", body)
	}
}

func TestQQBotSecretUpdateKeepsBlankAndRequiresExplicitClear(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store, err := config.Open(filepath.Join(t.TempDir(), "config.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	config.SetCurrent(store)
	if err := store.Initialize("admin-password"); err != nil {
		t.Fatal(err)
	}
	value := store.Config().QQBot
	value.OneBotToken = "keep-onebot-secret"
	value.AI.APIKey = "keep-ai-secret"
	if err := store.SetQQBot(value); err != nil {
		t.Fatal(err)
	}
	db, err := bbolt.Open(filepath.Join(t.TempDir(), "pst.db"), 0600, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	manager := qqbot.NewManager(db, nil, value)
	defer manager.Close()
	router := gin.New()
	RegisterRouterWithAllManagers(router, nil, nil, nil, manager)
	token, err := auth.GenerateToken()
	if err != nil {
		t.Fatal(err)
	}
	value.OneBotToken = ""
	value.AI.APIKey = ""
	response := performJSONRequest(router, http.MethodPut, "/api/qq-bot/config", value, token)
	if response.Code != http.StatusOK {
		t.Fatalf("blank secret update status = %d: %s", response.Code, response.Body.String())
	}
	if current := store.Config().QQBot; current.OneBotToken != "keep-onebot-secret" || current.AI.APIKey != "keep-ai-secret" {
		t.Fatal("blank secret update did not preserve saved values")
	}
	payload := map[string]any{
		"enabled": false, "onebot_websocket_url": value.OneBotWebSocketURL,
		"allowed_group_ids": []string{}, "admin_qq_ids": []string{}, "trigger_mode": value.TriggerMode,
		"user_rate_per_minute": value.UserRatePerMinute, "group_rate_per_minute": value.GroupRatePerMinute,
		"permissions": value.Permissions, "notifications": value.Notifications,
		"ai":                 map[string]any{"enabled": false, "base_url": value.AI.BaseURL, "model": value.AI.Model, "timeout_seconds": value.AI.TimeoutSeconds, "max_tool_calls": value.AI.MaxToolCalls, "send_redacted_results": true},
		"clear_onebot_token": true, "clear_ai_api_key": true,
	}
	response = performJSONRequest(router, http.MethodPut, "/api/qq-bot/config", payload, token)
	if response.Code != http.StatusOK {
		t.Fatalf("explicit secret clear status = %d: %s", response.Code, response.Body.String())
	}
	if current := store.Config().QQBot; current.OneBotToken != "" || current.AI.APIKey != "" {
		t.Fatal("explicit secret clear did not remove saved values")
	}
}
