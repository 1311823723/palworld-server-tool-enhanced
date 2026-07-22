package worldsettings

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/zaigie/palworld-server-tool/internal/config"
	"github.com/zaigie/palworld-server-tool/internal/logger"
	"github.com/zaigie/palworld-server-tool/internal/supervisor"
)

var (
	ErrBusy          = errors.New("a world settings operation is already in progress")
	ErrNotConfigured = errors.New("PalServer process management is not configured")
	backupIDPattern  = regexp.MustCompile(`^palworld-settings-[0-9]{8}T[0-9]{6}Z-[0-9a-f-]{36}\.ini$`)
)

type ConfigStore interface {
	Config() config.Config
	Update(config.Config, string) error
}

type ProcessManager interface {
	ProcessStatus() supervisor.Status
	ApplyAndRestart(supervisor.RestartOptions, supervisor.TransactionHooks) (supervisor.Status, error)
}

type AuditRecorder interface {
	RecordWorldSettingsAudit(WorldSettingsAudit) error
}

type WorldSettingsAudit struct {
	ID          string    `json:"id"`
	Action      string    `json:"action"`
	ChangedKeys []string  `json:"changed_keys"`
	BackupID    string    `json:"backup_id,omitempty"`
	Success     bool      `json:"success"`
	Error       string    `json:"error,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}

type Manager struct {
	operationMu sync.Mutex
	store       ConfigStore
	process     ProcessManager
	healthCheck func(context.Context) error
	audit       AuditRecorder
	now         func() time.Time
}

type SecretState struct {
	Key   string `json:"key"`
	IsSet bool   `json:"is_set"`
	Value any    `json:"value"`
}

type CurrentSettings struct {
	SchemaVersion    string                 `json:"schema_version"`
	Path             string                 `json:"path"`
	ModifiedAt       time.Time              `json:"modified_at"`
	Values           map[string]any         `json:"values"`
	Secrets          map[string]SecretState `json:"secrets"`
	UnknownKeys      []string               `json:"unknown_keys"`
	UnknownKeyCount  int                    `json:"unknown_key_count"`
	ParseWarnings    []string               `json:"parse_warnings"`
	RestartRequired  bool                   `json:"restart_required"`
	ServerConfigured bool                   `json:"server_configured"`
}

type ChangeRequest struct {
	Changes             map[string]any    `json:"changes"`
	Secrets             map[string]string `json:"secrets"`
	ClearSecrets        []string          `json:"clear_secrets"`
	ShutdownSeconds     int               `json:"shutdown_seconds"`
	RestartDelaySeconds int               `json:"restart_delay_seconds"`
	Message             string            `json:"message"`
}

type Difference struct {
	Key       string `json:"key"`
	Before    any    `json:"before"`
	After     any    `json:"after"`
	Secret    bool   `json:"secret"`
	Dangerous bool   `json:"dangerous"`
}

type ValidationResult struct {
	Valid        bool           `json:"valid"`
	Differences  []Difference   `json:"differences"`
	Warnings     []string       `json:"warnings"`
	ChangedKeys  []string       `json:"changed_keys"`
	Normalized   map[string]any `json:"normalized"`
	document     *Document
	original     []byte
	originalHash string
}

type ApplyResult struct {
	Success  bool              `json:"success"`
	BackupID string            `json:"backup_id"`
	Status   supervisor.Status `json:"process"`
}

type Backup struct {
	ID        string    `json:"id"`
	Size      int64     `json:"size"`
	CreatedAt time.Time `json:"created_at"`
}

func NewManager(store ConfigStore, process ProcessManager, healthCheck func(context.Context) error, audit AuditRecorder) *Manager {
	return &Manager{store: store, process: process, healthCheck: healthCheck, audit: audit, now: time.Now}
}

func (manager *Manager) settingsPath() (string, error) {
	settings := manager.store.Config().ServerProcess
	if !settings.Enabled || strings.TrimSpace(settings.ExecutablePath) == "" {
		return "", ErrNotConfigured
	}
	workingDirectory := strings.TrimSpace(settings.WorkingDirectory)
	if workingDirectory == "" {
		workingDirectory = filepath.Dir(settings.ExecutablePath)
	}
	if workingDirectory == "" || workingDirectory == "." {
		return "", ErrNotConfigured
	}
	return filepath.Join(workingDirectory, "Pal", "Saved", "Config", "WindowsServer", "PalWorldSettings.ini"), nil
}

func (manager *Manager) Current() (CurrentSettings, error) {
	path, err := manager.settingsPath()
	if err != nil {
		return CurrentSettings{}, err
	}
	data, info, document, err := readDocument(path)
	if err != nil {
		return CurrentSettings{}, err
	}
	_ = data
	definitions := SchemaByKey()
	result := CurrentSettings{
		SchemaVersion: SchemaVersion, Path: path, ModifiedAt: info.ModTime(), Values: map[string]any{},
		Secrets: map[string]SecretState{}, ParseWarnings: []string{}, ServerConfigured: true,
	}
	for _, entry := range document.Entries() {
		definition, known := definitions[entry.Key]
		if !known {
			result.UnknownKeys = append(result.UnknownKeys, entry.Key)
			continue
		}
		value, decodeErr := DecodeValue(definition, entry.RawValue)
		if definition.Secret {
			isSet := decodeErr == nil && strings.TrimSpace(fmt.Sprint(value)) != ""
			result.Secrets[entry.Key] = SecretState{Key: entry.Key, IsSet: isSet, Value: nil}
			continue
		}
		if decodeErr != nil {
			result.ParseWarnings = append(result.ParseWarnings, fmt.Sprintf("%s: %v", entry.Key, decodeErr))
			continue
		}
		result.Values[entry.Key] = value
	}
	for _, definition := range Schema {
		if definition.Secret {
			if _, exists := result.Secrets[definition.Key]; !exists {
				result.Secrets[definition.Key] = SecretState{Key: definition.Key, IsSet: false, Value: nil}
			}
		}
	}
	sort.Strings(result.UnknownKeys)
	result.UnknownKeyCount = len(result.UnknownKeys)
	return result, nil
}

func readDocument(path string) ([]byte, os.FileInfo, *Document, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("read PalWorldSettings.ini: %w", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		return nil, nil, nil, err
	}
	document, err := Parse(data)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("parse PalWorldSettings.ini: %w", err)
	}
	return data, info, document, nil
}

func (manager *Manager) Validate(request ChangeRequest) (ValidationResult, error) {
	path, err := manager.settingsPath()
	if err != nil {
		return ValidationResult{}, err
	}
	original, _, document, err := readDocument(path)
	if err != nil {
		return ValidationResult{}, err
	}
	definitions := SchemaByKey()
	result := ValidationResult{Valid: true, Warnings: []string{}, Normalized: map[string]any{}, document: document, original: original, originalHash: digest(original)}
	changed := map[string]bool{}

	keys := make([]string, 0, len(request.Changes))
	for key := range request.Changes {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		definition, ok := definitions[key]
		if !ok {
			return ValidationResult{}, fmt.Errorf("unknown world setting %q", key)
		}
		if definition.Secret {
			return ValidationResult{}, fmt.Errorf("secret %s must use the secrets field", key)
		}
		if definition.Deprecated || definition.Reserved {
			return ValidationResult{}, fmt.Errorf("%s is deprecated or reserved", key)
		}
		normalized, normalizeErr := NormalizeValue(definition, request.Changes[key])
		if normalizeErr != nil {
			return ValidationResult{}, fmt.Errorf("%s: %w", key, normalizeErr)
		}
		raw, encodeErr := EncodeValue(definition, normalized)
		if encodeErr != nil {
			return ValidationResult{}, fmt.Errorf("%s: %w", key, encodeErr)
		}
		beforeRaw, _ := document.Raw(key)
		before, _ := DecodeValue(definition, beforeRaw)
		if beforeRaw != raw {
			if err := document.SetRaw(key, raw); err != nil {
				return ValidationResult{}, err
			}
			result.Differences = append(result.Differences, Difference{Key: key, Before: before, After: normalized, Dangerous: definition.Dangerous})
			changed[key] = true
		}
		result.Normalized[key] = normalized
	}
	for key, value := range request.Secrets {
		definition, ok := definitions[key]
		if !ok || !definition.Secret {
			return ValidationResult{}, fmt.Errorf("%s is not an editable secret", key)
		}
		if strings.ContainsAny(value, "\x00\r\n") {
			return ValidationResult{}, fmt.Errorf("%s contains invalid control characters", key)
		}
		raw, encodeErr := EncodeValue(definition, value)
		if encodeErr != nil {
			return ValidationResult{}, fmt.Errorf("%s: %w", key, encodeErr)
		}
		beforeRaw, _ := document.Raw(key)
		if beforeRaw != raw {
			if err := document.SetRaw(key, raw); err != nil {
				return ValidationResult{}, err
			}
			result.Differences = append(result.Differences, Difference{Key: key, Before: nil, After: nil, Secret: true, Dangerous: definition.Dangerous})
			changed[key] = true
		}
	}
	for _, key := range request.ClearSecrets {
		definition, ok := definitions[key]
		if !ok || !definition.Secret {
			return ValidationResult{}, fmt.Errorf("%s is not a clearable secret", key)
		}
		beforeRaw, _ := document.Raw(key)
		raw, _ := EncodeValue(definition, "")
		if beforeRaw != raw {
			if err := document.SetRaw(key, raw); err != nil {
				return ValidationResult{}, err
			}
			result.Differences = append(result.Differences, Difference{Key: key, Before: nil, After: nil, Secret: true, Dangerous: definition.Dangerous})
			changed[key] = true
		}
	}
	for key := range changed {
		result.ChangedKeys = append(result.ChangedKeys, key)
	}
	sort.Strings(result.ChangedKeys)
	result.Warnings = validationWarnings(document, changed)
	return result, nil
}

func validationWarnings(document *Document, changed map[string]bool) []string {
	warnings := []string{}
	definitions := SchemaByKey()
	for key := range changed {
		if definitions[key].Dangerous {
			warnings = append(warnings, key+" is security-sensitive or can materially affect the world")
		}
	}
	if raw, ok := document.Raw("RESTAPIEnabled"); ok && strings.EqualFold(strings.TrimSpace(raw), "False") {
		warnings = append(warnings, "Disabling REST API will prevent PST save, graceful shutdown, player and health-check operations")
	}
	if pvpRaw, ok := document.Raw("bIsPvP"); ok && strings.EqualFold(strings.TrimSpace(pvpRaw), "False") {
		for _, key := range []string{"bEnablePlayerToPlayerDamage", "bEnableDefenseOtherGuildPlayer", "bCanPickupOtherGuildDeathPenaltyDrop"} {
			if raw, exists := document.Raw(key); exists && strings.EqualFold(strings.TrimSpace(raw), "True") {
				warnings = append(warnings, key+" may have no effect while bIsPvP is disabled")
			}
		}
	}
	sort.Strings(warnings)
	return warnings
}

func (manager *Manager) Apply(request ChangeRequest) (result ApplyResult, err error) {
	if !manager.operationMu.TryLock() {
		return result, ErrBusy
	}
	defer manager.operationMu.Unlock()
	validation, err := manager.Validate(request)
	if err != nil {
		return result, err
	}
	if len(validation.ChangedKeys) == 0 {
		return result, errors.New("no world setting changes to apply")
	}
	return manager.applyValidated(request, validation, "apply")
}

func (manager *Manager) applyValidated(request ChangeRequest, validation ValidationResult, action string) (result ApplyResult, err error) {
	if manager.process == nil {
		return result, errors.New("server process supervisor is unavailable")
	}
	path, _ := manager.settingsPath()
	oldConfig := manager.store.Config()
	var backupID string
	writeApplied := false
	rollback := func() error {
		if !writeApplied {
			return nil
		}
		backupPath, pathErr := manager.backupPath(backupID)
		if pathErr != nil {
			return pathErr
		}
		backupData, readErr := os.ReadFile(backupPath)
		if readErr != nil {
			return readErr
		}
		if writeErr := atomicWrite(path, backupData, 0600); writeErr != nil {
			return writeErr
		}
		if updateErr := manager.store.Update(oldConfig, ""); updateErr != nil {
			return updateErr
		}
		logger.Warnf("Rolled back PalWorldSettings.ini from backup %s\n", backupID)
		return nil
	}
	afterExit := func() error {
		current, _, _, readErr := readDocument(path)
		if readErr != nil {
			return readErr
		}
		if digest(current) != validation.originalHash {
			return errors.New("PalWorldSettings.ini changed after validation; refusing to overwrite")
		}
		backupID, err = manager.createBackup(current)
		if err != nil {
			return err
		}
		serialized := validation.document.Serialize()
		if err = atomicWrite(path, serialized, 0600); err != nil {
			return err
		}
		writeApplied = true
		_, _, reparsed, parseErr := readDocument(path)
		if parseErr != nil {
			_ = rollback()
			return parseErr
		}
		if digest(reparsed.Serialize()) != digest(serialized) {
			_ = rollback()
			return errors.New("written settings failed round-trip verification")
		}
		newConfig, configErr := syncRuntimeConfig(oldConfig, reparsed)
		if configErr != nil {
			_ = rollback()
			return configErr
		}
		if configErr = manager.store.Update(newConfig, ""); configErr != nil {
			_ = rollback()
			return configErr
		}
		return nil
	}
	options := supervisor.RestartOptions{ShutdownSeconds: request.ShutdownSeconds, RestartDelay: time.Duration(request.RestartDelaySeconds) * time.Second, Message: request.Message}
	if options.ShutdownSeconds < 0 || options.RestartDelay < 0 {
		return result, errors.New("restart delays cannot be negative")
	}
	if strings.TrimSpace(options.Message) == "" {
		options.Message = "服务器设置已修改，即将重启。"
	}
	healthCheck := manager.healthCheck
	if raw, ok := validation.document.Raw("RESTAPIEnabled"); ok && strings.EqualFold(strings.TrimSpace(raw), "False") {
		// A deliberately disabled REST API cannot answer the health endpoint. A
		// successful supervised process launch is the only available check.
		healthCheck = nil
	}
	status, applyErr := manager.process.ApplyAndRestart(options, supervisor.TransactionHooks{AfterExit: afterExit, Rollback: rollback, HealthCheck: healthCheck})
	audit := WorldSettingsAudit{ID: uuid.NewString(), Action: action, ChangedKeys: validation.ChangedKeys, BackupID: backupID, Success: applyErr == nil, CreatedAt: manager.now().UTC()}
	if applyErr != nil {
		audit.Error = applyErr.Error()
	}
	manager.recordAudit(audit)
	if applyErr != nil {
		return ApplyResult{BackupID: backupID, Status: status}, applyErr
	}
	logger.Infof("Applied PalWorldSettings.ini fields %s; backup %s\n", strings.Join(validation.ChangedKeys, ","), backupID)
	return ApplyResult{Success: true, BackupID: backupID, Status: status}, nil
}

func syncRuntimeConfig(value config.Config, document *Document) (config.Config, error) {
	definitions := SchemaByKey()
	if raw, ok := document.Raw("AdminPassword"); ok {
		decoded, err := DecodeValue(definitions["AdminPassword"], raw)
		if err != nil {
			return value, err
		}
		value.Rest.Password = decoded.(string)
		value.Rcon.Password = decoded.(string)
	}
	if raw, ok := document.Raw("RESTAPIPort"); ok {
		decoded, err := DecodeValue(definitions["RESTAPIPort"], raw)
		if err != nil {
			return value, err
		}
		address, err := replaceURLPort(value.Rest.Address, int(decoded.(int64)))
		if err != nil {
			return value, err
		}
		value.Rest.Address = address
	}
	if raw, ok := document.Raw("RCONPort"); ok {
		decoded, err := DecodeValue(definitions["RCONPort"], raw)
		if err != nil {
			return value, err
		}
		host, _, splitErr := net.SplitHostPort(value.Rcon.Address)
		if splitErr != nil {
			host = "127.0.0.1"
		}
		value.Rcon.Address = net.JoinHostPort(host, strconv.Itoa(int(decoded.(int64))))
	}
	return value, nil
}

func replaceURLPort(address string, port int) (string, error) {
	parsed, err := url.Parse(address)
	if err != nil {
		return "", err
	}
	host := parsed.Hostname()
	if host == "" {
		host = "127.0.0.1"
	}
	parsed.Host = net.JoinHostPort(host, strconv.Itoa(port))
	return parsed.String(), nil
}

func digest(data []byte) string { sum := sha256.Sum256(data); return hex.EncodeToString(sum[:]) }

func (manager *Manager) backupDirectory() (string, error) {
	path, err := manager.settingsPath()
	if err != nil {
		return "", err
	}
	return filepath.Join(filepath.Dir(path), ".pst-backups"), nil
}

func (manager *Manager) backupPath(id string) (string, error) {
	if !backupIDPattern.MatchString(id) {
		return "", errors.New("invalid backup id")
	}
	directory, err := manager.backupDirectory()
	if err != nil {
		return "", err
	}
	return filepath.Join(directory, id), nil
}

func (manager *Manager) createBackup(data []byte) (string, error) {
	directory, err := manager.backupDirectory()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(directory, 0700); err != nil {
		return "", err
	}
	id := fmt.Sprintf("palworld-settings-%s-%s.ini", manager.now().UTC().Format("20060102T150405Z"), uuid.NewString())
	if err := atomicWrite(filepath.Join(directory, id), data, 0600); err != nil {
		return "", err
	}
	return id, nil
}

func (manager *Manager) ListBackups() ([]Backup, error) {
	directory, err := manager.backupDirectory()
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(directory)
	if errors.Is(err, os.ErrNotExist) {
		return []Backup{}, nil
	}
	if err != nil {
		return nil, err
	}
	backups := []Backup{}
	for _, entry := range entries {
		if entry.IsDir() || !backupIDPattern.MatchString(entry.Name()) {
			continue
		}
		info, infoErr := entry.Info()
		if infoErr != nil {
			continue
		}
		backups = append(backups, Backup{ID: entry.Name(), Size: info.Size(), CreatedAt: info.ModTime().UTC()})
	}
	sort.Slice(backups, func(i, j int) bool { return backups[i].CreatedAt.After(backups[j].CreatedAt) })
	return backups, nil
}

func (manager *Manager) DeleteBackup(id string) error {
	path, err := manager.backupPath(id)
	if err != nil {
		return err
	}
	return os.Remove(path)
}

func (manager *Manager) RestoreBackup(id string, shutdownSeconds, restartDelaySeconds int, message string) (ApplyResult, error) {
	if !manager.operationMu.TryLock() {
		return ApplyResult{}, ErrBusy
	}
	defer manager.operationMu.Unlock()
	backupPath, err := manager.backupPath(id)
	if err != nil {
		return ApplyResult{}, err
	}
	data, err := os.ReadFile(backupPath)
	if err != nil {
		return ApplyResult{}, err
	}
	document, err := Parse(data)
	if err != nil {
		return ApplyResult{}, fmt.Errorf("invalid backup: %w", err)
	}
	definitions := SchemaByKey()
	for _, entry := range document.Entries() {
		definition, known := definitions[entry.Key]
		if !known {
			continue
		}
		value, decodeErr := DecodeValue(definition, entry.RawValue)
		if decodeErr != nil {
			return ApplyResult{}, fmt.Errorf("invalid backup field %s: %w", entry.Key, decodeErr)
		}
		if _, normalizeErr := NormalizeValue(definition, value); normalizeErr != nil {
			return ApplyResult{}, fmt.Errorf("invalid backup field %s: %w", entry.Key, normalizeErr)
		}
	}
	path, err := manager.settingsPath()
	if err != nil {
		return ApplyResult{}, err
	}
	original, _, currentDocument, err := readDocument(path)
	if err != nil {
		return ApplyResult{}, err
	}
	currentRaw := map[string]string{}
	for _, entry := range currentDocument.Entries() {
		currentRaw[entry.Key] = entry.RawValue
	}
	changed := []string{}
	for _, entry := range document.Entries() {
		if currentRaw[entry.Key] != entry.RawValue {
			changed = append(changed, entry.Key)
		}
		delete(currentRaw, entry.Key)
	}
	for key := range currentRaw {
		changed = append(changed, key)
	}
	sort.Strings(changed)
	if len(changed) == 0 {
		return ApplyResult{}, errors.New("the selected backup is identical to the current settings")
	}
	validation := ValidationResult{Valid: true, ChangedKeys: changed, document: document, original: original, originalHash: digest(original)}
	request := ChangeRequest{ShutdownSeconds: shutdownSeconds, RestartDelaySeconds: restartDelaySeconds, Message: message}
	return manager.applyValidated(request, validation, "restore")
}

func (manager *Manager) CurrentInt(key string, fallback int) int {
	current, err := manager.Current()
	if err != nil {
		return fallback
	}
	switch value := current.Values[key].(type) {
	case int64:
		return int(value)
	case float64:
		return int(value)
	}
	return fallback
}

func (manager *Manager) recordAudit(record WorldSettingsAudit) {
	if manager.audit != nil {
		if err := manager.audit.RecordWorldSettingsAudit(record); err != nil {
			logger.Errorf("Record world settings audit: %v\n", err)
		}
	}
}
