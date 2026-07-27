package api

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/zaigie/palworld-server-tool/internal/database"
	"github.com/zaigie/palworld-server-tool/internal/logger"
	"github.com/zaigie/palworld-server-tool/internal/tool"
	"github.com/zaigie/palworld-server-tool/service"
)

func createBackup(manager ServerProcessManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		if manager != nil && manager.ProcessStatus().Running {
			if err := manager.SaveWorld(); err != nil {
				c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "保存世界失败，已取消备份：" + err.Error()})
				return
			}
		}
		backup, err := createBackupRecord("manual")
		if err != nil {
			c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "创建备份失败：" + err.Error()})
			return
		}
		logger.Infof("管理员创建了手动存档备份 %s\n", backup.Path)
		c.JSON(http.StatusOK, backup)
	}
}

func createBackupRecord(source string) (database.Backup, error) {
	backup := database.Backup{
		BackupId: uuid.NewString(),
		SaveTime: time.Now().UTC(),
		Source:   source,
		Status:   "failed",
	}
	path, err := tool.Backup()
	if err != nil {
		backup.Error = err.Error()
		if recordErr := service.AddBackup(database.GetDB(), backup); recordErr != nil {
			return backup, fmt.Errorf("%v; 保存失败记录失败: %w", err, recordErr)
		}
		return backup, err
	}
	backupDir, err := tool.GetBackupDir()
	if err != nil {
		backup.Path = path
		backup.Error = err.Error()
		_ = service.AddBackup(database.GetDB(), backup)
		return backup, err
	}
	var size int64
	if info, statErr := os.Stat(filepath.Join(backupDir, path)); statErr == nil {
		size = info.Size()
	}
	backup.Path = path
	backup.Size = size
	backup.Status = "success"
	if err := service.AddBackup(database.GetDB(), backup); err != nil {
		_ = os.Remove(filepath.Join(backupDir, path))
		return database.Backup{}, err
	}
	return backup, nil
}

// listBackups godoc
//
//	@Summary		List backups within a specified time range
//	@Description	List all backups or backups within a specific time range.
//	@Tags			backup
//	@Accept			json
//	@Produce		json
//	@Security		ApiKeyAuth
//	@Param			startTime	query		int	false	"Start time of the backup range in timestamp"
//	@Param			endTime		query		int	false	"End time of the backup range in timestamp"
//	@Success		200			{array}		database.Backup
//	@Failure		400			{object}	ErrorResponse
//	@Router			/api/backup [get]
func listBackups(c *gin.Context) {
	var startTimestamp, endTimestamp int64
	var startTime, endTime time.Time
	var err error

	startTimeStr, endTimeStr := c.Query("startTime"), c.Query("endTime")

	if startTimeStr != "" {
		startTimestamp, err = strconv.ParseInt(startTimeStr, 10, 64)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid start time"})
			return
		}
		startTime = time.Unix(0, startTimestamp*int64(time.Millisecond))
	}

	if endTimeStr != "" {
		endTimestamp, err = strconv.ParseInt(endTimeStr, 10, 64)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid end time"})
			return
		}
		endTime = time.Unix(0, endTimestamp*int64(time.Millisecond))
	}

	backups, err := service.ListBackups(database.GetDB(), startTime, endTime)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, backups)
}

// downloadBackup godoc
//
//	@Summary		Download Backup
//	@Description	Download a backup
//	@Tags			backup
//	@Accept			json
//	@Produce		application/octet-stream
//	@Security		ApiKeyAuth
//	@Param			backup_id	path		string	true	"Backup ID"
//	@Success		200			{file}		"Backupfile"
//	@Failure		400			{object}	ErrorResponse
//	@Failure		404			{object}	ErrorResponse
//	@Failure		500			{object}	ErrorResponse
//	@Router			/api/backup/{backup_id} [get]
func downloadBackup(c *gin.Context) {
	backupId := c.Param("backup_id")
	backup, err := service.GetBackup(database.GetDB(), backupId)
	if err != nil {
		if err == service.ErrNoRecord {
			c.JSON(http.StatusNotFound, gin.H{})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if backup.Status == "failed" || backup.Path == "" {
		c.JSON(http.StatusNotFound, ErrorResponse{Error: "该记录没有可下载的备份文件"})
		return
	}

	backupDir, err := tool.GetBackupDir()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%s", backup.Path))
	c.File(filepath.Join(backupDir, backup.Path))
}

// deleteBackup godoc
//
//	@Summary		Delete Backup
//	@Description	Delete a backup
//	@Tags			backup
//	@Accept			json
//	@Produce		json
//	@Security		ApiKeyAuth
//	@Param			backup_id	path		string	true	"Backup ID"
//	@Success		200			{object}	SuccessResponse
//	@Failure		400			{object}	ErrorResponse
//	@Router			/api/backup/{backup_id} [delete]
func deleteBackup(c *gin.Context) {
	backupId := c.Param("backup_id")
	var backup database.Backup
	backup, err := service.GetBackup(database.GetDB(), backupId)
	if err != nil {
		if err == service.ErrNoRecord {
			c.JSON(http.StatusNotFound, gin.H{})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := service.DeleteBackup(database.GetDB(), backupId); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if backup.Path == "" {
		c.JSON(http.StatusOK, gin.H{"success": true})
		return
	}
	backupDir, err := tool.GetBackupDir()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	err = os.Remove(filepath.Join(backupDir, backup.Path))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}
