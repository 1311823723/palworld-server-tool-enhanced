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

type ServerProcessConfig struct {
	Enabled                     bool     `json:"enabled"`
	ExecutablePath              string   `json:"executable_path"`
	WorkingDirectory            string   `json:"working_directory"`
	Arguments                   []string `json:"arguments"`
	WatchdogEnabled             bool     `json:"watchdog_enabled"`
	RestartDelaySeconds         int      `json:"restart_delay_seconds"`
	GracefulShutdownSeconds     int      `json:"graceful_shutdown_seconds"`
	GracefulShutdownMessage     string   `json:"graceful_shutdown_message"`
	MaxRestartAttempts          int      `json:"max_restart_attempts"`
	RestartAttemptWindowSeconds int      `json:"restart_attempt_window_seconds"`
}

type Config struct {
	Web           WebConfig           `json:"web"`
	Task          TaskConfig          `json:"task"`
	Rcon          RconConfig          `json:"rcon"`
	Rest          RestConfig          `json:"rest"`
	Save          SaveConfig          `json:"save"`
	Manage        ManageConfig        `json:"manage"`
	ServerProcess ServerProcessConfig `json:"server_process"`
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
	value.ServerProcess.RestartDelaySeconds = 10
	value.ServerProcess.GracefulShutdownSeconds = 30
	value.ServerProcess.GracefulShutdownMessage = "Server restart in 30 seconds"
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
	if err := ValidateServerProcess(value.ServerProcess); err != nil {
		return err
	}
	return nil
}

func NormalizeServerProcess(value ServerProcessConfig) ServerProcessConfig {
	defaults := Default().ServerProcess
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
	if value.RestartDelaySeconds < 0 || value.GracefulShutdownSeconds < 0 || value.RestartAttemptWindowSeconds < 1 {
		return errors.New("server process delays must be non-negative and restart window must be positive")
	}
	if value.MaxRestartAttempts < 1 {
		return errors.New("server process max restart attempts must be positive")
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
