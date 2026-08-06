package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"go.etcd.io/bbolt"
)

func TestQQBotConfigurationIsLocalOnlyAndRedacted(t *testing.T) {
	value := NormalizeQQBot(QQBotConfig{})
	if value.OneBotWebSocketURL != "ws://127.0.0.1:3001" || !value.Permissions.RestartServer || value.Permissions.StartServer || value.Permissions.StopServer {
		t.Fatalf("unexpected QQ bot defaults: %#v", value)
	}
	if !value.Persona.Enabled || value.Persona.Style != QQBotPersonaLively || value.Persona.Character != QQBotPersonaCharacterCattiva || !value.Persona.SeriousOnError {
		t.Fatalf("unexpected QQ bot persona defaults: %#v", value.Persona)
	}
	if value.AI.Model != DeepSeekModelV4Flash {
		t.Fatalf("unexpected DeepSeek model: %q", value.AI.Model)
	}
	legacy := value
	legacy.AI.Model = "deepseek-chat"
	if migrated := NormalizeQQBot(legacy); migrated.AI.Model != DeepSeekModelV4Flash {
		t.Fatalf("legacy DeepSeek model was not migrated: %q", migrated.AI.Model)
	}
	value.Enabled = true
	if err := ValidateQQBot(value); err == nil {
		t.Fatal("enabled QQ bot without token must be rejected")
	}
	value.OneBotToken = "onebot-secret"
	if err := ValidateQQBot(value); err != nil {
		t.Fatalf("valid loopback QQ bot configuration rejected: %v", err)
	}
	value.OneBotWebSocketURL = "ws://192.168.1.8:3001"
	if err := ValidateQQBot(value); err == nil {
		t.Fatal("non-loopback OneBot address must be rejected")
	}
	value.OneBotWebSocketURL = "ws://127.0.0.1:3001?access_token=leak"
	if err := ValidateQQBot(value); err == nil {
		t.Fatal("OneBot token in URL must be rejected")
	}
	value.OneBotWebSocketURL = "ws://127.0.0.1:3001"
	value.Persona.Style = "custom-prompt"
	if err := ValidateQQBot(value); err == nil {
		t.Fatal("unsupported QQ bot persona style must be rejected")
	}

	settings := Default()
	settings.QQBot.OneBotToken = "onebot-secret"
	settings.QQBot.AI.APIKey = "deepseek-secret"
	redacted := settings.Redacted()
	if redacted.QQBot.OneBotToken != "" || redacted.QQBot.AI.APIKey != "" {
		t.Fatal("QQ bot secrets must be removed from redacted configuration")
	}
}

func TestQQBotPersonaCharacterUpdatePreservesOtherSettings(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "config.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	value := store.Config().QQBot
	value.OneBotToken = "onebot-secret"
	value.AllowedGroupIDs = []string{"90000000000000000002"}
	value.Persona.Style = QQBotPersonaMischievous
	if err := store.SetQQBot(value); err != nil {
		t.Fatal(err)
	}
	if err := store.SetQQBotPersonaCharacter(QQBotPersonaCharacterLamball); err != nil {
		t.Fatal(err)
	}
	got := store.Config().QQBot
	if got.Persona.Character != QQBotPersonaCharacterLamball || got.Persona.Style != QQBotPersonaMischievous || got.OneBotToken != "onebot-secret" || len(got.AllowedGroupIDs) != 1 {
		t.Fatalf("persona switch overwrote other QQ bot settings: %#v", got)
	}
	if err := store.SetQQBotPersonaCharacter("unknown"); err == nil {
		t.Fatal("unsupported persona character must be rejected")
	}
}

func TestQQBotSensitiveSettingsPersistInConfigDB(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.db")
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	value := store.Config().QQBot
	value.OneBotToken = "persisted-onebot-secret"
	value.AI.APIKey = "persisted-deepseek-secret"
	value.Persona.Style = QQBotPersonaMischievous
	value.Persona.SeriousOnError = false
	if err := store.SetQQBot(value); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if reopened.Config().QQBot.OneBotToken != "persisted-onebot-secret" || reopened.Config().QQBot.AI.APIKey != "persisted-deepseek-secret" {
		t.Fatal("QQ bot secrets did not persist in config.db")
	}
	if persona := reopened.Config().QQBot.Persona; persona.Style != QQBotPersonaMischievous || persona.SeriousOnError {
		t.Fatalf("QQ bot persona did not persist in config.db: %#v", persona)
	}
}

func TestServerProcessConfigurationRejectsUnsafeValues(t *testing.T) {
	executable := filepath.Join(t.TempDir(), "PalServer.exe")
	if err := os.WriteFile(executable, []byte("fixture"), 0600); err != nil {
		t.Fatalf("create executable fixture: %v", err)
	}
	value := Default().ServerProcess
	value.Enabled = true
	value.ExecutablePath = executable
	value.Arguments = []string{"-port=8211", "& powershell.exe"}
	if err := ValidateServerProcess(value); err == nil {
		t.Fatal("unsafe process argument must be rejected")
	}
	value.Arguments = []string{"-port=8211"}
	if err := ValidateServerProcess(value); err != nil {
		t.Fatalf("valid process configuration rejected: %v", err)
	}
	value.ExecutablePath = filepath.Join(t.TempDir(), "PalServer.exe")
	if err := ValidateServerProcess(value); err == nil {
		t.Fatal("missing executable must be rejected")
	}
}

func TestScheduledRestartConfiguration(t *testing.T) {
	value := Default().ServerProcess
	if value.ScheduledRestartFrequency != ScheduledRestartDaily {
		t.Fatalf("default scheduled restart frequency = %q, want daily", value.ScheduledRestartFrequency)
	}
	if value.ScheduledRestartTime != "04:00" {
		t.Fatalf("default scheduled restart time = %q, want 04:00", value.ScheduledRestartTime)
	}

	value.ScheduledRestartTime = "4:00"
	if err := ValidateServerProcess(value); err == nil {
		t.Fatal("non-padded scheduled restart time must be rejected")
	}

	value.ScheduledRestartTime = "24:00"
	if err := ValidateServerProcess(value); err == nil {
		t.Fatal("out-of-range scheduled restart time must be rejected")
	}

	value.ScheduledRestartTime = "04:00"
	value.ScheduledRestartFrequency = ScheduledRestartIntervalDays
	value.ScheduledRestartIntervalDays = -1
	if err := ValidateServerProcess(value); err == nil {
		t.Fatal("negative scheduled restart interval must be rejected")
	}

	value.ScheduledRestartIntervalDays = 3
	value.ScheduledRestartStartDate = "2026/07/22"
	if err := ValidateServerProcess(value); err == nil {
		t.Fatal("invalid scheduled restart start date must be rejected")
	}

	value.ScheduledRestartFrequency = ScheduledRestartWeekly
	value.ScheduledRestartWeekday = 7
	if err := ValidateServerProcess(value); err == nil {
		t.Fatal("invalid scheduled restart weekday must be rejected")
	}

	value.ScheduledRestartFrequency = ScheduledRestartMonthly
	value.ScheduledRestartDayOfMonth = 32
	if err := ValidateServerProcess(value); err == nil {
		t.Fatal("invalid scheduled restart day of month must be rejected")
	}

	value.ScheduledRestartFrequency = ScheduledRestartCron
	value.ScheduledRestartCron = "0 4 * * *"
	if err := ValidateServerProcess(value); err != nil {
		t.Fatalf("valid cron scheduled restart rejected: %v", err)
	}
	value.ScheduledRestartCron = "0 4 *"
	if err := ValidateServerProcess(value); err == nil {
		t.Fatal("invalid cron scheduled restart must be rejected")
	}

	value = Default().ServerProcess
	value.ScheduledRestartEnabled = true
	if err := ValidateServerProcess(value); err == nil {
		t.Fatal("scheduled restart without process management must be rejected")
	}
}

func TestBreedingMonitorConfiguration(t *testing.T) {
	value := NormalizeBreedingMonitor(BreedingMonitorConfig{})
	if value.SelectionMode != "selected" || value.MinimumReadyEggs != 1 || value.HistoryRetentionDays != 30 || value.GameNotificationMessage == "" {
		t.Fatalf("unexpected breeding defaults: %#v", value)
	}
	value.SelectionMode = "nearby"
	if err := ValidateBreedingMonitor(value); err == nil {
		t.Fatal("unsupported breeding selection mode must be rejected")
	}
	value.SelectionMode = "selected"
	value.MinimumReadyEggs = 0
	if err := ValidateBreedingMonitor(value); err == nil {
		t.Fatal("zero egg notification threshold must be rejected")
	}
	value.MinimumReadyEggs = 1
	value.HistoryRetentionDays = 3651
	if err := ValidateBreedingMonitor(value); err == nil {
		t.Fatal("excessive event retention must be rejected")
	}
	value.HistoryRetentionDays = 30
	value.SelectedFarmIDs = []string{"farm-a\nforged"}
	if err := ValidateBreedingMonitor(value); err == nil {
		t.Fatal("invalid farm identifier must be rejected")
	}
	value.SelectedFarmIDs = nil
	value.GameNotificationMessage = "第一行\n第二行"
	if err := ValidateBreedingMonitor(value); err == nil {
		t.Fatal("multi-line game notification message must be rejected")
	}
}

func TestStoreFirstRunInitializationAndPersistence(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "config.db")

	store, err := Open(dbPath)
	if err != nil {
		t.Fatalf("open config store: %v", err)
	}
	if store.IsInitialized() {
		t.Fatal("new config database must require administrator setup")
	}
	if got := store.Config().Web.Port; got != 8080 {
		t.Fatalf("default web port = %d, want 8080", got)
	}
	if err := store.Initialize("correct horse battery staple"); err != nil {
		t.Fatalf("initialize administrator: %v", err)
	}
	if !store.Authenticate("correct horse battery staple") {
		t.Fatal("initialized administrator password must authenticate")
	}
	if store.Authenticate("wrong password") {
		t.Fatal("incorrect administrator password must not authenticate")
	}
	if err := store.Close(); err != nil {
		t.Fatalf("close config store: %v", err)
	}

	reopened, err := Open(dbPath)
	if err != nil {
		t.Fatalf("reopen config store: %v", err)
	}
	defer reopened.Close()
	if !reopened.IsInitialized() {
		t.Fatal("administrator setup must persist in config.db")
	}
	if !reopened.Authenticate("correct horse battery staple") {
		t.Fatal("administrator password must persist in config.db")
	}
}

func TestStoreUpdatesSettingsAndAdministratorPasswordTogether(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "config.db"))
	if err != nil {
		t.Fatalf("open config store: %v", err)
	}
	defer store.Close()
	if err := store.Initialize("old-password"); err != nil {
		t.Fatalf("initialize administrator: %v", err)
	}

	next := store.Config()
	next.Save.SourceMode = "agent"
	next.Save.Path = "http://game-host:8081/sync"
	next.Rcon.Address = "game-host:25575"
	if err := store.Update(next, "new-password"); err != nil {
		t.Fatalf("update settings: %v", err)
	}

	got := store.Config()
	if got.Save.SourceMode != "agent" || got.Save.Path != "http://game-host:8081/sync" {
		t.Fatalf("saved source = %#v, want agent URL", got.Save)
	}
	if got.Rcon.Address != "game-host:25575" {
		t.Fatalf("rcon address = %q, want game-host:25575", got.Rcon.Address)
	}
	if store.Authenticate("old-password") {
		t.Fatal("old administrator password must be invalidated")
	}
	if !store.Authenticate("new-password") {
		t.Fatal("new administrator password must authenticate")
	}
}

func TestStorePreservesLegacyWebPortWithoutPortSource(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "config.db"))
	if err != nil {
		t.Fatalf("open config store: %v", err)
	}
	defer store.Close()

	legacy := store.Config()
	legacy.Web.Port = 19090
	data, err := json.Marshal(legacy)
	if err != nil {
		t.Fatalf("encode legacy settings: %v", err)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("decode legacy settings map: %v", err)
	}
	delete(raw["web"].(map[string]any), "port_source")
	data, err = json.Marshal(raw)
	if err != nil {
		t.Fatalf("encode settings without port_source: %v", err)
	}
	if err := store.db.Update(func(tx *bbolt.Tx) error {
		return tx.Bucket(configBucket).Put(configKey, data)
	}); err != nil {
		t.Fatalf("write legacy settings: %v", err)
	}

	loaded := store.Config()
	if loaded.Web.Port != 19090 {
		t.Fatalf("legacy web port = %d, want 19090", loaded.Web.Port)
	}
	if loaded.Web.PortSource != WebPortOverrideNone {
		t.Fatalf("legacy web port source = %q, want empty", loaded.Web.PortSource)
	}
}

func TestCronRestartRequiresExactlyFiveFieldsAndUsesPublicKey(t *testing.T) {
	value := Default().ServerProcess
	value.Enabled = true
	value.ScheduledRestartEnabled = true
	value.ScheduledRestartFrequency = ScheduledRestartCron
	value.ScheduledRestartCron = "@daily"
	if err := ValidateServerProcess(value); err == nil {
		t.Fatal("Cron descriptor must not bypass the five-field requirement")
	}

	value.ScheduledRestartCron = "0 4 * * *"
	value.ExecutablePath = ""
	if err := ValidateServerProcess(value); err == nil {
		t.Fatal("enabled process config without executable unexpectedly passed")
	}

	data, err := json.Marshal(ServerProcessConfig{ScheduledRestartCron: "0 4 * * *"})
	if err != nil {
		t.Fatal(err)
	}
	var encoded map[string]any
	if err := json.Unmarshal(data, &encoded); err != nil {
		t.Fatal(err)
	}
	if encoded["cron_expression"] != "0 4 * * *" {
		t.Fatalf("cron_expression was not serialized: %s", data)
	}
	if _, legacy := encoded["scheduled_restart_cron"]; legacy {
		t.Fatalf("legacy Cron JSON key leaked: %s", data)
	}
}
