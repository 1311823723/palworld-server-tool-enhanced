package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/robfig/cron/v3"
	"go.etcd.io/bbolt"
	"golang.org/x/crypto/bcrypt"
)

const DefaultDatabasePath = "config.db"

var (
	configBucket = []byte("config")
	authBucket   = []byte("auth")
	configKey    = []byte("settings")
	passwordKey  = []byte("password_hash")

	ErrAlreadyInitialized = errors.New("administrator password is already initialized")
	ErrPasswordRequired   = errors.New("administrator password is required")
)

type WebConfig struct {
	Port       int                   `json:"port"`
	PortSource WebPortOverrideSource `json:"port_source"`
	TLS        bool                  `json:"tls"`
	CertPath   string                `json:"cert_path"`
	KeyPath    string                `json:"key_path"`
	PublicURL  string                `json:"public_url"`
}

type WebPortOverrideSource string

const (
	WebPortOverrideNone        WebPortOverrideSource = ""
	WebPortOverrideEnvironment WebPortOverrideSource = "environment"
	WebPortOverrideCommandLine WebPortOverrideSource = "command_line"
)

type TaskConfig struct {
	SyncInterval        int    `json:"sync_interval"`
	PlayerLogging       bool   `json:"player_logging"`
	PlayerLoginMessage  string `json:"player_login_message"`
	PlayerLogoutMessage string `json:"player_logout_message"`
}

type RconConfig struct {
	Address   string `json:"address"`
	Password  string `json:"password"`
	UseBase64 bool   `json:"use_base64"`
	Timeout   int    `json:"timeout"`
}

type RestConfig struct {
	Address  string `json:"address"`
	Username string `json:"username"`
	Password string `json:"password"`
	Timeout  int    `json:"timeout"`
}

type SaveConfig struct {
	SourceMode     string `json:"source_mode"`
	Path           string `json:"path"`
	DecodePath     string `json:"decode_path"`
	SyncInterval   int    `json:"sync_interval"`
	BackupInterval int    `json:"backup_interval"`
	BackupKeepDays int    `json:"backup_keep_days"`
}

type ManageConfig struct {
	KickNonWhitelist bool `json:"kick_non_whitelist"`
}

type InventoryVisibilityConfig struct {
	Mode               string `json:"mode"`
	AllowPublicSummary bool   `json:"allow_public_summary"`
}

type BreedingMonitorConfig struct {
	Enabled                 bool     `json:"enabled"`
	NotifyExistingOnEnable  bool     `json:"notify_existing_on_enable"`
	SelectionMode           string   `json:"selection_mode"`
	SelectedBaseIDs         []string `json:"selected_base_ids"`
	SelectedFarmIDs         []string `json:"selected_farm_ids"`
	NotifyOnEachEgg         bool     `json:"notify_on_each_egg"`
	MinimumReadyEggs        int      `json:"minimum_ready_eggs"`
	BrowserNotifications    bool     `json:"browser_notifications"`
	InAppNotifications      bool     `json:"in_app_notifications"`
	GameNotifications       bool     `json:"game_notifications"`
	GameNotificationMessage string   `json:"game_notification_message"`
	HistoryRetentionDays    int      `json:"history_retention_days"`
}

const (
	ScheduledRestartDaily        = "daily"
	ScheduledRestartIntervalDays = "interval_days"
	ScheduledRestartWeekly       = "weekly"
	ScheduledRestartMonthly      = "monthly"
	ScheduledRestartCron         = "cron"
)

type ServerProcessConfig struct {
	Enabled                      bool     `json:"enabled"`
	ExecutablePath               string   `json:"executable_path"`
	WorkingDirectory             string   `json:"working_directory"`
	Arguments                    []string `json:"arguments"`
	WatchdogEnabled              bool     `json:"watchdog_enabled"`
	ScheduledRestartEnabled      bool     `json:"scheduled_restart_enabled"`
	ScheduledRestartFrequency    string   `json:"scheduled_restart_frequency"`
	ScheduledRestartTime         string   `json:"scheduled_restart_time"`
	ScheduledRestartIntervalDays int      `json:"scheduled_restart_interval_days"`
	ScheduledRestartStartDate    string   `json:"scheduled_restart_start_date"`
	ScheduledRestartWeekday      int      `json:"scheduled_restart_weekday"`
	ScheduledRestartDayOfMonth   int      `json:"scheduled_restart_day_of_month"`
	ScheduledRestartCron         string   `json:"cron_expression"`
	SteamCMDPath                 string   `json:"steamcmd_path"`
	RestartDelaySeconds          int      `json:"restart_delay_seconds"`
	GracefulShutdownSeconds      int      `json:"graceful_shutdown_seconds"`
	GracefulShutdownMessage      string   `json:"graceful_shutdown_message"`
	MaxRestartAttempts           int      `json:"max_restart_attempts"`
	RestartAttemptWindowSeconds  int      `json:"restart_attempt_window_seconds"`
}

type Config struct {
	Web                 WebConfig                 `json:"web"`
	Task                TaskConfig                `json:"task"`
	Rcon                RconConfig                `json:"rcon"`
	Rest                RestConfig                `json:"rest"`
	Save                SaveConfig                `json:"save"`
	Manage              ManageConfig              `json:"manage"`
	InventoryVisibility InventoryVisibilityConfig `json:"inventory_visibility"`
	BreedingMonitor     BreedingMonitorConfig     `json:"breeding_monitor"`
	ServerProcess       ServerProcessConfig       `json:"server_process"`
}

func Default() Config {
	var value Config
	value.Web.Port = 8080
	value.Task.SyncInterval = 60
	value.Task.PlayerLoginMessage = "Player {username} has joined the server! Current online player count: {online_num}."
	value.Task.PlayerLogoutMessage = "Player {username} has left the server! Current online player count: {online_num}."
	value.Rcon.Address = "127.0.0.1:25575"
	value.Rcon.Timeout = 5
	value.Rest.Address = "http://127.0.0.1:8212"
	value.Rest.Username = "admin"
	value.Rest.Timeout = 5
	value.Save.SourceMode = "directory"
	value.Save.SyncInterval = 120
	value.Save.BackupInterval = 14400
	value.Save.BackupKeepDays = 7
	value.InventoryVisibility.Mode = "admin"
	value.BreedingMonitor.SelectionMode = "selected"
	value.BreedingMonitor.NotifyOnEachEgg = true
	value.BreedingMonitor.MinimumReadyEggs = 1
	value.BreedingMonitor.BrowserNotifications = true
	value.BreedingMonitor.InAppNotifications = true
	value.BreedingMonitor.GameNotifications = true
	value.BreedingMonitor.GameNotificationMessage = "【配种提醒】据点「{base}」有 {new_count} 枚新蛋可以拾取，当前共有 {count} 枚。"
	value.BreedingMonitor.HistoryRetentionDays = 30
	value.ServerProcess.RestartDelaySeconds = 10
	value.ServerProcess.ScheduledRestartFrequency = ScheduledRestartDaily
	value.ServerProcess.ScheduledRestartTime = "04:00"
	value.ServerProcess.ScheduledRestartIntervalDays = 2
	value.ServerProcess.ScheduledRestartStartDate = time.Now().Format(time.DateOnly)
	value.ServerProcess.ScheduledRestartWeekday = int(time.Monday)
	value.ServerProcess.ScheduledRestartDayOfMonth = 1
	value.ServerProcess.ScheduledRestartCron = "0 4 * * *"
	value.ServerProcess.GracefulShutdownSeconds = 30
	value.ServerProcess.GracefulShutdownMessage = "服务器将在 30 秒后重启，请提前回到安全位置。"
	value.ServerProcess.MaxRestartAttempts = 5
	value.ServerProcess.RestartAttemptWindowSeconds = 300
	return value
}

type Store struct {
	db *bbolt.DB
}

func Open(path string) (*Store, error) {
	db, err := bbolt.Open(path, 0600, &bbolt.Options{Timeout: time.Minute})
	if err != nil {
		return nil, fmt.Errorf("open config database: %w", err)
	}
	store := &Store{db: db}
	if err := store.createBucketsAndDefaults(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

func (s *Store) createBucketsAndDefaults() error {
	return s.db.Update(func(tx *bbolt.Tx) error {
		bucket, err := tx.CreateBucketIfNotExists(configBucket)
		if err != nil {
			return err
		}
		if _, err := tx.CreateBucketIfNotExists(authBucket); err != nil {
			return err
		}
		if bucket.Get(configKey) != nil {
			return nil
		}
		data, err := json.Marshal(Default())
		if err != nil {
			return err
		}
		return bucket.Put(configKey, data)
	})
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) Config() Config {
	value := Default()
	_ = s.db.View(func(tx *bbolt.Tx) error {
		data := tx.Bucket(configBucket).Get(configKey)
		return json.Unmarshal(data, &value)
	})
	return value
}

func (s *Store) IsInitialized() bool {
	initialized := false
	_ = s.db.View(func(tx *bbolt.Tx) error {
		initialized = len(tx.Bucket(authBucket).Get(passwordKey)) > 0
		return nil
	})
	return initialized
}

func (s *Store) Initialize(password string) error {
	if strings.TrimSpace(password) == "" {
		return ErrPasswordRequired
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("hash administrator password: %w", err)
	}
	return s.db.Update(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket(authBucket)
		if len(bucket.Get(passwordKey)) > 0 {
			return ErrAlreadyInitialized
		}
		return bucket.Put(passwordKey, hash)
	})
}

func (s *Store) Update(value Config, newPassword string) error {
	value.ServerProcess = NormalizeServerProcess(value.ServerProcess)
	if err := Validate(value); err != nil {
		return err
	}
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("encode settings: %w", err)
	}
	var passwordHash []byte
	if newPassword != "" {
		if strings.TrimSpace(newPassword) == "" {
			return ErrPasswordRequired
		}
		passwordHash, err = bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
		if err != nil {
			return fmt.Errorf("hash administrator password: %w", err)
		}
	}
	return s.db.Update(func(tx *bbolt.Tx) error {
		if err := tx.Bucket(configBucket).Put(configKey, data); err != nil {
			return err
		}
		if len(passwordHash) > 0 {
			return tx.Bucket(authBucket).Put(passwordKey, passwordHash)
		}
		return nil
	})
}

func (s *Store) SetServerProcessWatchdog(enabled bool) error {
	return s.db.Update(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket(configBucket)
		value := Default()
		if err := json.Unmarshal(bucket.Get(configKey), &value); err != nil {
			return err
		}
		value.ServerProcess = NormalizeServerProcess(value.ServerProcess)
		value.ServerProcess.WatchdogEnabled = enabled
		data, err := json.Marshal(value)
		if err != nil {
			return err
		}
		return bucket.Put(configKey, data)
	})
}

func (s *Store) SetBreedingMonitor(value BreedingMonitorConfig) error {
	value = NormalizeBreedingMonitor(value)
	if err := ValidateBreedingMonitor(value); err != nil {
		return err
	}
	return s.db.Update(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket(configBucket)
		current := Default()
		if err := json.Unmarshal(bucket.Get(configKey), &current); err != nil {
			return err
		}
		current.BreedingMonitor = value
		data, err := json.Marshal(current)
		if err != nil {
			return err
		}
		return bucket.Put(configKey, data)
	})
}

func Validate(value Config) error {
	value.ServerProcess = NormalizeServerProcess(value.ServerProcess)
	if err := ValidateWebPort(value.Web.Port); err != nil {
		return err
	}
	if value.Web.PortSource != WebPortOverrideNone && value.Web.PortSource != WebPortOverrideEnvironment && value.Web.PortSource != WebPortOverrideCommandLine {
		return fmt.Errorf("invalid web port override source %q", value.Web.PortSource)
	}
	if value.Save.SourceMode != "directory" && value.Save.SourceMode != "agent" {
		return errors.New("save source mode must be directory or agent")
	}
	if value.Task.SyncInterval < 0 || value.Save.SyncInterval < 0 || value.Save.BackupInterval < 0 {
		return errors.New("task intervals cannot be negative")
	}
	if value.Rcon.Timeout < 0 || value.Rest.Timeout < 0 || value.Save.BackupKeepDays < 0 {
		return errors.New("timeouts and backup retention cannot be negative")
	}
	if value.InventoryVisibility.Mode != "admin" && value.InventoryVisibility.Mode != "public_summary" {
		return errors.New("inventory visibility mode must be admin or public_summary")
	}
	if value.InventoryVisibility.AllowPublicSummary && value.InventoryVisibility.Mode != "public_summary" {
		return errors.New("public inventory summary requires public_summary visibility mode")
	}
	if err := ValidateBreedingMonitor(NormalizeBreedingMonitor(value.BreedingMonitor)); err != nil {
		return err
	}
	if err := ValidateServerProcess(value.ServerProcess); err != nil {
		return err
	}
	return nil
}

func NormalizeBreedingMonitor(value BreedingMonitorConfig) BreedingMonitorConfig {
	defaults := Default().BreedingMonitor
	if strings.TrimSpace(value.SelectionMode) == "" {
		value.SelectionMode = defaults.SelectionMode
	}
	if value.MinimumReadyEggs == 0 {
		value.MinimumReadyEggs = defaults.MinimumReadyEggs
	}
	if value.HistoryRetentionDays == 0 {
		value.HistoryRetentionDays = defaults.HistoryRetentionDays
	}
	if strings.TrimSpace(value.GameNotificationMessage) == "" {
		value.GameNotificationMessage = defaults.GameNotificationMessage
	}
	if value.SelectedBaseIDs == nil {
		value.SelectedBaseIDs = []string{}
	}
	if value.SelectedFarmIDs == nil {
		value.SelectedFarmIDs = []string{}
	}
	return value
}

func ValidateBreedingMonitor(value BreedingMonitorConfig) error {
	switch value.SelectionMode {
	case "selected", "all":
	default:
		return errors.New("breeding monitor selection mode must be selected or all")
	}
	if value.MinimumReadyEggs < 1 || value.MinimumReadyEggs > 10000 {
		return errors.New("breeding monitor minimum ready eggs must be between 1 and 10000")
	}
	if value.HistoryRetentionDays < 1 || value.HistoryRetentionDays > 3650 {
		return errors.New("breeding monitor history retention must be between 1 and 3650 days")
	}
	if utf8.RuneCountInString(value.GameNotificationMessage) > 300 || strings.ContainsAny(value.GameNotificationMessage, "\x00\r\n") {
		return errors.New("breeding game notification message must be a single line of at most 300 characters")
	}
	if len(value.SelectedBaseIDs) > 10000 || len(value.SelectedFarmIDs) > 10000 {
		return errors.New("too many breeding monitor selections")
	}
	for _, id := range append(append([]string{}, value.SelectedBaseIDs...), value.SelectedFarmIDs...) {
		id = strings.TrimSpace(id)
		if id == "" || len(id) > 512 || strings.ContainsAny(id, "\x00\r\n") {
			return errors.New("invalid breeding monitor selection identifier")
		}
	}
	return nil
}

func NormalizeServerProcess(value ServerProcessConfig) ServerProcessConfig {
	defaults := Default().ServerProcess
	if strings.TrimSpace(value.ScheduledRestartFrequency) == "" {
		value.ScheduledRestartFrequency = defaults.ScheduledRestartFrequency
	}
	if strings.TrimSpace(value.ScheduledRestartTime) == "" {
		value.ScheduledRestartTime = defaults.ScheduledRestartTime
	}
	if value.ScheduledRestartIntervalDays == 0 {
		value.ScheduledRestartIntervalDays = defaults.ScheduledRestartIntervalDays
	}
	if strings.TrimSpace(value.ScheduledRestartStartDate) == "" {
		value.ScheduledRestartStartDate = defaults.ScheduledRestartStartDate
	}
	if value.ScheduledRestartDayOfMonth == 0 {
		value.ScheduledRestartDayOfMonth = defaults.ScheduledRestartDayOfMonth
	}
	if strings.TrimSpace(value.ScheduledRestartCron) == "" {
		value.ScheduledRestartCron = defaults.ScheduledRestartCron
	}
	if value.MaxRestartAttempts < 1 {
		value.MaxRestartAttempts = defaults.MaxRestartAttempts
	}
	if value.RestartAttemptWindowSeconds < 1 {
		value.RestartAttemptWindowSeconds = defaults.RestartAttemptWindowSeconds
	}
	if value.GracefulShutdownMessage == "" {
		value.GracefulShutdownMessage = defaults.GracefulShutdownMessage
	}
	return value
}

func ValidateServerProcess(value ServerProcessConfig) error {
	value = NormalizeServerProcess(value)
	if value.RestartDelaySeconds < 0 || value.GracefulShutdownSeconds < 0 || value.RestartAttemptWindowSeconds < 1 {
		return errors.New("server process delays must be non-negative and restart window must be positive")
	}
	if value.MaxRestartAttempts < 1 {
		return errors.New("server process max restart attempts must be positive")
	}
	switch value.ScheduledRestartFrequency {
	case ScheduledRestartDaily:
		if err := validateScheduledRestartTime(value.ScheduledRestartTime); err != nil {
			return err
		}
	case ScheduledRestartIntervalDays:
		if err := validateScheduledRestartTime(value.ScheduledRestartTime); err != nil {
			return err
		}
		if value.ScheduledRestartIntervalDays < 1 || value.ScheduledRestartIntervalDays > 3650 {
			return errors.New("scheduled restart interval must be between 1 and 3650 days")
		}
		if _, err := time.Parse(time.DateOnly, value.ScheduledRestartStartDate); err != nil {
			return errors.New("scheduled restart start date must use YYYY-MM-DD format")
		}
	case ScheduledRestartWeekly:
		if err := validateScheduledRestartTime(value.ScheduledRestartTime); err != nil {
			return err
		}
		if value.ScheduledRestartWeekday < int(time.Sunday) || value.ScheduledRestartWeekday > int(time.Saturday) {
			return errors.New("scheduled restart weekday must be between 0 and 6")
		}
	case ScheduledRestartMonthly:
		if err := validateScheduledRestartTime(value.ScheduledRestartTime); err != nil {
			return err
		}
		if value.ScheduledRestartDayOfMonth < 1 || value.ScheduledRestartDayOfMonth > 31 {
			return errors.New("scheduled restart day of month must be between 1 and 31")
		}
	case ScheduledRestartCron:
		expression := strings.TrimSpace(value.ScheduledRestartCron)
		if len(strings.Fields(expression)) != 5 {
			return errors.New("cron_expression must contain exactly 5 fields: minute hour day month weekday")
		}
		if _, err := cron.ParseStandard(expression); err != nil {
			return fmt.Errorf("invalid scheduled restart cron expression: %w", err)
		}
	default:
		return fmt.Errorf("unsupported scheduled restart frequency %q", value.ScheduledRestartFrequency)
	}
	if value.ScheduledRestartEnabled && !value.Enabled {
		return errors.New("scheduled restart requires server process management to be enabled")
	}
	for _, argument := range value.Arguments {
		lower := strings.ToLower(argument)
		if strings.ContainsAny(argument, "&|><\r\n\x00") || strings.Contains(lower, "cmd.exe") || strings.Contains(lower, "powershell.exe") {
			return fmt.Errorf("unsafe server process argument %q", argument)
		}
	}
	if !value.Enabled {
		return nil
	}
	if steamCMDPath := strings.TrimSpace(value.SteamCMDPath); steamCMDPath != "" {
		info, err := os.Stat(steamCMDPath)
		if err != nil {
			return fmt.Errorf("steamcmd path: %w", err)
		}
		if !info.Mode().IsRegular() || !strings.EqualFold(filepath.Base(steamCMDPath), "steamcmd.exe") {
			return errors.New("steamcmd path must point to steamcmd.exe")
		}
	}
	path := strings.TrimSpace(value.ExecutablePath)
	if path == "" {
		return errors.New("server process executable path is required")
	}
	base := strings.ToLower(filepath.Base(strings.ReplaceAll(path, `\`, "/")))
	if base != "palserver.exe" && base != "palserver-win64-shipping-cmd.exe" {
		return errors.New("server process executable must be PalServer.exe or PalServer-Win64-Shipping-Cmd.exe")
	}
	foreignWindowsPath := runtime.GOOS != "windows" && (strings.Contains(path, `:\`) || strings.Contains(path, `:/`))
	if foreignWindowsPath {
		return nil
	}
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("server process executable: %w", err)
	}
	if !info.Mode().IsRegular() {
		return errors.New("server process executable must be a regular file")
	}
	workingDirectory := strings.TrimSpace(value.WorkingDirectory)
	if workingDirectory == "" {
		workingDirectory = filepath.Dir(path)
	}
	workingInfo, err := os.Stat(workingDirectory)
	if err != nil {
		return fmt.Errorf("server process working directory: %w", err)
	}
	if !workingInfo.IsDir() {
		return errors.New("server process working directory must be a directory")
	}
	return nil
}

func validateScheduledRestartTime(value string) error {
	if len(value) != 5 || value[2] != ':' {
		return errors.New("scheduled restart time must use HH:MM in 24-hour format")
	}
	if _, err := time.Parse("15:04", value); err != nil {
		return errors.New("scheduled restart time must use HH:MM in 24-hour format")
	}
	return nil
}

func (value Config) Redacted() Config {
	value.Rcon.Password = ""
	value.Rest.Password = ""
	return value
}

func ValidateWebPort(port int) error {
	if port < 1 || port > 65535 {
		return errors.New("web port must be between 1 and 65535")
	}
	return nil
}

func (s *Store) Authenticate(password string) bool {
	hash := s.passwordHash()
	return len(hash) > 0 && bcrypt.CompareHashAndPassword(hash, []byte(password)) == nil
}

func (s *Store) passwordHash() []byte {
	var result []byte
	_ = s.db.View(func(tx *bbolt.Tx) error {
		result = append(result, tx.Bucket(authBucket).Get(passwordKey)...)
		return nil
	})
	return result
}

func (s *Store) TokenKey() []byte {
	return s.passwordHash()
}

var (
	currentMu  sync.RWMutex
	current    *Store
	runtimeMu  sync.RWMutex
	runtimeWeb *WebConfig
)

func SetCurrent(store *Store) {
	currentMu.Lock()
	defer currentMu.Unlock()
	current = store
}

func CurrentStore() *Store {
	currentMu.RLock()
	defer currentMu.RUnlock()
	if current == nil {
		panic("config store is not initialized")
	}
	return current
}

func Current() Config {
	return CurrentStore().Config()
}

// SetRuntimeWeb records the web settings used by the currently running HTTP
// server. Persisted web settings may differ until PST is restarted.
func SetRuntimeWeb(value WebConfig) {
	runtimeMu.Lock()
	defer runtimeMu.Unlock()
	copy := value
	runtimeWeb = &copy
}

func RuntimeWeb() WebConfig {
	runtimeMu.RLock()
	defer runtimeMu.RUnlock()
	if runtimeWeb != nil {
		return *runtimeWeb
	}
	return Current().Web
}
