package tool

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/zaigie/palworld-server-tool/internal/auth"
	"github.com/zaigie/palworld-server-tool/internal/config"
	"github.com/zaigie/palworld-server-tool/internal/database"
	"github.com/zaigie/palworld-server-tool/internal/logger"
	"github.com/zaigie/palworld-server-tool/internal/source"
	"github.com/zaigie/palworld-server-tool/internal/system"
	"github.com/zaigie/palworld-server-tool/service"
	"go.etcd.io/bbolt"
)

type Sturcture struct {
	Players []database.Player `json:"players"`
	Guilds  []database.Guild  `json:"guilds"`
}

func getSavCli() (string, error) {
	savCliPath := config.Current().Save.DecodePath
	if savCliPath == "" || savCliPath == "/path/to/your/sav_cli" {
		ed, err := system.GetExecDir()
		if err != nil {
			logger.Errorf("error getting exec directory: %s", err)
			return "", err
		}
		savCliPath = filepath.Join(ed, "sav_cli")
		if runtime.GOOS == "windows" {
			savCliPath += ".exe"
		}
	}
	if _, err := os.Stat(savCliPath); err != nil {
		return "", err
	}
	return savCliPath, nil
}

func Decode(file string) error {
	savCli, err := getSavCli()
	if err != nil {
		return errors.New("error getting executable path: " + err.Error())
	}

	levelFilePath, err := getFromSource(file, "decode")
	if err != nil {
		return err
	}
	defer os.RemoveAll(filepath.Dir(levelFilePath))

	settings := config.RuntimeWeb()
	baseUrl := fmt.Sprintf("http://127.0.0.1:%d", settings.Port)
	if settings.TLS && settings.PublicURL != "" && !strings.HasSuffix(baseUrl, "/") {
		baseUrl = settings.PublicURL
	}

	requestUrl := fmt.Sprintf("%s/api/", baseUrl)
	tokenString, err := auth.GenerateToken()
	if err != nil {
		return errors.New("error generating token: " + err.Error())
	}
	execArgs := []string{"-f", levelFilePath, "--request", requestUrl, "--token", tokenString}
	cmd := exec.Command(savCli, execArgs...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err = cmd.Start()
	if err != nil {
		return fmt.Errorf("failed to start sav_cli: %w", err)
	}
	err = cmd.Wait()
	if err != nil {
		return fmt.Errorf("sav_cli exited with error: %w", err)
	}

	return nil
}

func Backup() (string, error) {
	sourcePath := config.Current().Save.Path

	levelFilePath, err := getFromSource(sourcePath, "backup")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(filepath.Dir(levelFilePath))

	backupDir, err := GetBackupDir()
	if err != nil {
		return "", fmt.Errorf("failed to get backup directory: %s", err)
	}

	currentTime := time.Now().Format("2006-01-02-15-04-05")
	backupZipFile := filepath.Join(backupDir, fmt.Sprintf("%s.zip", currentTime))
	err = system.ZipDir(filepath.Dir(levelFilePath), backupZipFile)
	if err != nil {
		return "", fmt.Errorf("failed to create backup zip: %s", err)
	}
	return filepath.Base(backupZipFile), nil
}

// BackupRecord creates a backup and persists its metadata. It is shared by
// manual backups, server updates and maintenance transactions so every backup
// has the same failure semantics and audit-friendly source field.
func BackupRecord(sourceName string) (database.Backup, error) {
	backup := database.Backup{
		BackupId: uuid.NewString(),
		SaveTime: time.Now().UTC(),
		Source:   sourceName,
		Status:   "failed",
	}
	path, err := Backup()
	if err != nil {
		backup.Error = err.Error()
		if recordErr := service.AddBackup(database.GetDB(), backup); recordErr != nil {
			return backup, fmt.Errorf("%v; 保存失败记录失败: %w", err, recordErr)
		}
		return backup, err
	}
	backupDir, err := GetBackupDir()
	if err != nil {
		backup.Path = path
		backup.Error = err.Error()
		_ = service.AddBackup(database.GetDB(), backup)
		return backup, err
	}
	if info, statErr := os.Stat(filepath.Join(backupDir, path)); statErr == nil {
		backup.Size = info.Size()
	}
	backup.Path = path
	backup.Status = "success"
	if err := service.AddBackup(database.GetDB(), backup); err != nil {
		_ = os.Remove(filepath.Join(backupDir, path))
		return database.Backup{}, err
	}
	return backup, nil
}

func GetBackupDir() (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	backDir := filepath.Join(wd, "backups")
	if err = system.CheckAndCreateDir(backDir); err != nil {
		return "", err
	}
	return backDir, nil
}

func CleanOldBackups(db *bbolt.DB, keepDays int) error {
	backupDir, err := GetBackupDir()
	if err != nil {
		return fmt.Errorf("failed to get backup directory: %s", err)
	}

	deadline := time.Now().AddDate(0, 0, -keepDays)

	backups, err := service.ListBackups(db, time.Time{}, time.Now())
	if err != nil {
		return fmt.Errorf("failed to list backups: %s", err)
	}

	for _, backup := range backups {
		if backup.SaveTime.Before(deadline) {
			err = os.Remove(filepath.Join(backupDir, backup.Path))
			if err != nil {
				if !os.IsNotExist(err) {
					logger.Errorf("failed to delete old backup file %s: %s", backup.Path, err)
				}
			}

			err = service.DeleteBackup(db, backup.BackupId)
			if err != nil {
				logger.Errorf("failed to delete backup record from database: %s", err)
			}
		}
	}

	return nil
}

func getFromSource(file, way string) (string, error) {
	mode := config.Current().Save.SourceMode
	if mode == "agent" {
		if !strings.HasPrefix(file, "http://") && !strings.HasPrefix(file, "https://") {
			return "", errors.New("agent source must be an http:// or https:// pst-agent URL")
		}
		levelFilePath, err := source.DownloadFromHttp(file, way)
		if err != nil {
			return "", errors.New("error downloading file: " + err.Error())
		}
		return levelFilePath, nil
	}
	if mode != "directory" {
		return "", errors.New("unsupported save source mode")
	}
	if strings.HasPrefix(file, "http://") || strings.HasPrefix(file, "https://") {
		return "", errors.New("directory source must be a local file system path")
	}
	levelFilePath, err := source.CopyFromLocal(file, way)
	if err != nil {
		return "", errors.New("error copying file from directory source: " + err.Error())
	}
	return levelFilePath, nil
}
