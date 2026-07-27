package supervisor

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"
)

const palworldDedicatedServerAppID = "2394010"

var buildIDPattern = regexp.MustCompile(`(?m)"buildid"\s+"([0-9]+)"`)

var ErrUpdateNotConfigured = errors.New("SteamCMD 更新未配置")

type UpdateStatus struct {
	Enabled        bool       `json:"enabled"`
	Checking       bool       `json:"checking"`
	Running        bool       `json:"running"`
	InstalledBuild string     `json:"installed_build"`
	LatestBuild    string     `json:"latest_build"`
	Available      bool       `json:"available"`
	LastCheckedAt  *time.Time `json:"last_checked_at,omitempty"`
	LastUpdatedAt  *time.Time `json:"last_updated_at,omitempty"`
	Error          string     `json:"error"`
}

type ServerUpdater interface {
	Check(ctx context.Context, processConfig ProcessConfig) (installedBuild, latestBuild string, err error)
	Apply(ctx context.Context, processConfig ProcessConfig) (installedBuild string, err error)
}

type SteamCMDUpdater struct{}

func (SteamCMDUpdater) Check(ctx context.Context, processConfig ProcessConfig) (string, string, error) {
	if runtime.GOOS != "windows" {
		return "", "", ErrUnsupportedPlatform
	}
	if err := validateSteamCMD(processConfig.SteamCMDPath); err != nil {
		return "", "", err
	}
	installed, err := installedBuildID(processConfig)
	if err != nil && !os.IsNotExist(err) {
		return "", "", err
	}
	command := exec.CommandContext(ctx, processConfig.SteamCMDPath,
		"+login", "anonymous",
		"+app_info_update", "1",
		"+app_info_print", palworldDedicatedServerAppID,
		"+quit",
	)
	command.Dir = filepath.Dir(processConfig.SteamCMDPath)
	output, err := command.CombinedOutput()
	if err != nil {
		return installed, "", fmt.Errorf("SteamCMD 检查更新失败: %w", err)
	}
	matches := buildIDPattern.FindAllStringSubmatch(string(output), -1)
	if len(matches) == 0 {
		return installed, "", errors.New("SteamCMD 未返回 Palworld Dedicated Server Build ID")
	}
	return installed, matches[0][1], nil
}

func (SteamCMDUpdater) Apply(ctx context.Context, processConfig ProcessConfig) (string, error) {
	if runtime.GOOS != "windows" {
		return "", ErrUnsupportedPlatform
	}
	if err := validateSteamCMD(processConfig.SteamCMDPath); err != nil {
		return "", err
	}
	command := exec.CommandContext(ctx, processConfig.SteamCMDPath,
		"+force_install_dir", processConfig.WorkingDirectory,
		"+login", "anonymous",
		"+app_update", palworldDedicatedServerAppID, "validate",
		"+quit",
	)
	command.Dir = filepath.Dir(processConfig.SteamCMDPath)
	if output, err := command.CombinedOutput(); err != nil {
		return "", fmt.Errorf("SteamCMD 更新失败: %w（输出长度 %d 字节）", err, len(output))
	}
	return installedBuildID(processConfig)
}

func validateSteamCMD(path string) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return ErrUpdateNotConfigured
	}
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("SteamCMD 路径无效: %w", err)
	}
	if !info.Mode().IsRegular() || !strings.EqualFold(filepath.Base(path), "steamcmd.exe") {
		return ErrUpdateNotConfigured
	}
	return nil
}

func installedBuildID(processConfig ProcessConfig) (string, error) {
	manifestName := "appmanifest_" + palworldDedicatedServerAppID + ".acf"
	candidates := []string{
		filepath.Join(processConfig.WorkingDirectory, "steamapps", manifestName),
		filepath.Join(filepath.Dir(processConfig.WorkingDirectory), "steamapps", manifestName),
		filepath.Join(filepath.Dir(filepath.Dir(processConfig.WorkingDirectory)), manifestName),
		filepath.Join(filepath.Dir(processConfig.SteamCMDPath), "steamapps", manifestName),
	}
	manifestPath := candidates[0]
	for _, candidate := range candidates {
		if info, err := os.Stat(candidate); err == nil && info.Mode().IsRegular() {
			manifestPath = candidate
			break
		}
	}
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return "", err
	}
	match := buildIDPattern.FindSubmatch(data)
	if len(match) != 2 {
		return "", errors.New("本地 appmanifest 未包含 Build ID")
	}
	if _, err := strconv.ParseUint(string(match[1]), 10, 64); err != nil {
		return "", errors.New("本地 Build ID 格式无效")
	}
	return string(match[1]), nil
}

func (s *ServerSupervisor) ServerUpdateStatus() UpdateStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	status := s.updateStatus
	status.Enabled = strings.TrimSpace(s.config.SteamCMDPath) != ""
	return status
}

func (s *ServerSupervisor) CheckServerUpdate() (UpdateStatus, error) {
	s.mu.Lock()
	if s.updateStatus.Checking || s.updateStatus.Running || s.operationActive {
		status := s.updateStatus
		s.mu.Unlock()
		return status, ErrConflict
	}
	if strings.TrimSpace(s.config.SteamCMDPath) == "" {
		status := s.updateStatus
		s.mu.Unlock()
		return status, ErrUpdateNotConfigured
	}
	processConfig := s.config
	updater := s.updater
	if updater == nil {
		updater = SteamCMDUpdater{}
	}
	s.updateStatus.Checking = true
	s.updateStatus.Error = ""
	s.mu.Unlock()

	ctx, cancel := context.WithTimeout(s.ctx, 2*time.Minute)
	installed, latest, err := updater.Check(ctx, processConfig)
	cancel()
	now := time.Now().UTC()
	s.mu.Lock()
	s.updateStatus.Checking = false
	s.updateStatus.Enabled = true
	s.updateStatus.InstalledBuild = installed
	s.updateStatus.LatestBuild = latest
	s.updateStatus.Available = installed != "" && latest != "" && installed != latest
	s.updateStatus.LastCheckedAt = &now
	if err != nil {
		s.updateStatus.Error = err.Error()
	}
	status := s.updateStatus
	s.mu.Unlock()
	return status, err
}

func (s *ServerSupervisor) ApplyServerUpdate(options RestartOptions) (Status, error) {
	s.mu.Lock()
	if s.updateStatus.Checking || s.updateStatus.Running || s.operationActive || s.transactionActive || s.restarting || s.state == StateStopping || s.state == StateRestartWaiting {
		status := s.statusLocked(time.Now())
		s.mu.Unlock()
		return status, ErrConflict
	}
	if !s.isRunningLocked() {
		status := s.statusLocked(time.Now())
		s.mu.Unlock()
		return status, ErrNotRunning
	}
	if strings.TrimSpace(s.config.SteamCMDPath) == "" {
		status := s.statusLocked(time.Now())
		s.mu.Unlock()
		return status, ErrUpdateNotConfigured
	}
	processConfig := s.config
	updater := s.updater
	if updater == nil {
		updater = SteamCMDUpdater{}
	}
	previousWatchdog := s.watchdogEnabled
	s.updateStatus.Running = true
	s.updateStatus.Error = ""
	s.mu.Unlock()

	_, err := s.applyAndRestart(options, TransactionHooks{
		AfterExit: func() error {
			s.mu.Lock()
			s.state = StateUpdating
			s.mu.Unlock()
			ctx, cancel := context.WithTimeout(s.ctx, 30*time.Minute)
			installed, updateErr := updater.Apply(ctx, processConfig)
			cancel()
			if updateErr != nil {
				return updateErr
			}
			now := time.Now().UTC()
			s.mu.Lock()
			s.updateStatus.InstalledBuild = installed
			s.updateStatus.LatestBuild = installed
			s.updateStatus.Available = false
			s.updateStatus.LastUpdatedAt = &now
			s.mu.Unlock()
			return nil
		},
		Rollback: func() error {
			return errors.New("SteamCMD 原地更新不支持自动回滚")
		},
	}, true)

	s.mu.Lock()
	s.updateStatus.Running = false
	if err != nil {
		s.updateStatus.Error = err.Error()
		s.watchdogEnabled = false
		s.desiredRunning = false
		s.state = StateUpdateFailed
		s.lastError = err.Error()
	} else {
		s.watchdogEnabled = previousWatchdog
		s.desiredRunning = true
		s.updateStatus.Error = ""
	}
	finalStatus := s.statusLocked(time.Now())
	s.mu.Unlock()
	if err != nil {
		return finalStatus, err
	}
	return finalStatus, nil
}

func (s *ServerSupervisor) SetUpdaterForTesting(updater ServerUpdater) {
	s.mu.Lock()
	s.updater = updater
	s.mu.Unlock()
}
