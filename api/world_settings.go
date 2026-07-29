package api

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/zaigie/palworld-server-tool/internal/supervisor"
	"github.com/zaigie/palworld-server-tool/internal/worldsettings"
)

func getWorldSettingsSchema(manager *worldsettings.Manager) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"schema_version": worldsettings.SchemaVersion, "settings": worldsettings.PublicSchema()})
	}
}

func getWorldSettings(manager *worldsettings.Manager) gin.HandlerFunc {
	return func(c *gin.Context) {
		if manager == nil {
			c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "world settings manager is unavailable"})
			return
		}
		result, err := manager.Current()
		if err != nil {
			worldSettingsError(c, err)
			return
		}
		c.JSON(http.StatusOK, result)
	}
}

func validateWorldSettings(manager *worldsettings.Manager) gin.HandlerFunc {
	return func(c *gin.Context) {
		if manager == nil {
			c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "world settings manager is unavailable"})
			return
		}
		var request worldsettings.ChangeRequest
		if err := c.ShouldBindJSON(&request); err != nil {
			c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
			return
		}
		result, err := manager.Validate(request)
		if err != nil {
			worldSettingsError(c, err)
			return
		}
		c.JSON(http.StatusOK, result)
	}
}

func applyWorldSettings(manager *worldsettings.Manager) gin.HandlerFunc {
	return func(c *gin.Context) {
		if manager == nil {
			c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "world settings manager is unavailable"})
			return
		}
		var request worldsettings.ChangeRequest
		if err := c.ShouldBindJSON(&request); err != nil {
			c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
			return
		}
		if request.Confirmation != "应用" {
			c.JSON(http.StatusBadRequest, ErrorResponse{Error: "请输入“应用”确认世界设置变更"})
			return
		}
		result, err := manager.Apply(request)
		if err != nil {
			worldSettingsError(c, err)
			return
		}
		c.JSON(http.StatusOK, result)
	}
}

func listWorldSettingsBackups(manager *worldsettings.Manager) gin.HandlerFunc {
	return func(c *gin.Context) {
		if manager == nil {
			c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "world settings manager is unavailable"})
			return
		}
		backups, err := manager.ListBackups()
		if err != nil {
			worldSettingsError(c, err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"items": backups})
	}
}

type restoreWorldSettingsRequest struct {
	ShutdownSeconds     int    `json:"shutdown_seconds"`
	RestartDelaySeconds int    `json:"restart_delay_seconds"`
	Message             string `json:"message"`
	Confirmation        string `json:"confirmation"`
}

func restoreWorldSettingsBackup(manager *worldsettings.Manager) gin.HandlerFunc {
	return func(c *gin.Context) {
		if manager == nil {
			c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "world settings manager is unavailable"})
			return
		}
		var request restoreWorldSettingsRequest
		if err := c.ShouldBindJSON(&request); err != nil {
			c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
			return
		}
		if request.Confirmation != "恢复" {
			c.JSON(http.StatusBadRequest, ErrorResponse{Error: "请输入“恢复”确认恢复设置备份"})
			return
		}
		result, err := manager.RestoreBackup(c.Param("backup_id"), request.ShutdownSeconds, request.RestartDelaySeconds, request.Message)
		if err != nil {
			worldSettingsError(c, err)
			return
		}
		c.JSON(http.StatusOK, result)
	}
}

func deleteWorldSettingsBackup(manager *worldsettings.Manager) gin.HandlerFunc {
	return func(c *gin.Context) {
		if manager == nil {
			c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "world settings manager is unavailable"})
			return
		}
		if err := manager.DeleteBackup(c.Param("backup_id")); err != nil {
			worldSettingsError(c, err)
			return
		}
		c.JSON(http.StatusOK, SuccessResponse{Success: true})
	}
}

func worldSettingsError(c *gin.Context, err error) {
	status := http.StatusBadRequest
	switch {
	case errors.Is(err, worldsettings.ErrBusy), errors.Is(err, supervisor.ErrConflict):
		status = http.StatusConflict
	case errors.Is(err, supervisor.ErrNotRunning), errors.Is(err, worldsettings.ErrNotConfigured):
		status = http.StatusBadRequest
	}
	c.JSON(status, ErrorResponse{Error: err.Error()})
}
