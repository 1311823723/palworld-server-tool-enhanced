package api

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/zaigie/palworld-server-tool/internal/config"
	"github.com/zaigie/palworld-server-tool/internal/logger"
	"github.com/zaigie/palworld-server-tool/internal/supervisor"
)

type ServerProcessManager interface {
	ProcessStatus() supervisor.Status
	SaveWorld() error
	Start() (supervisor.Status, error)
	Restart(options supervisor.RestartOptions) (supervisor.Status, error)
	Stop(options supervisor.StopOptions) (supervisor.Status, error)
	SetWatchdog(enabled bool) supervisor.Status
	UpdateConfig(value config.ServerProcessConfig)
	ServerUpdateStatus() supervisor.UpdateStatus
	CheckServerUpdate() (supervisor.UpdateStatus, error)
	ApplyServerUpdate(options supervisor.RestartOptions) (supervisor.Status, error)
}

type restartServerRequest struct {
	ShutdownSeconds     int    `json:"shutdown_seconds"`
	RestartDelaySeconds int    `json:"restart_delay_seconds"`
	Message             string `json:"message"`
}

type stopServerRequest struct {
	ShutdownSeconds int    `json:"shutdown_seconds"`
	Message         string `json:"message"`
	KeepStopped     *bool  `json:"keep_stopped"`
}

type watchdogRequest struct {
	Enabled bool `json:"enabled"`
}

type applyUpdateRequest struct {
	Confirmation        string `json:"confirmation"`
	ShutdownSeconds     int    `json:"shutdown_seconds"`
	RestartDelaySeconds int    `json:"restart_delay_seconds"`
	Message             string `json:"message"`
}

type schedulePreviewRequest struct {
	Frequency      string `json:"frequency"`
	Time           string `json:"time"`
	IntervalDays   int    `json:"interval_days"`
	StartDate      string `json:"start_date"`
	Weekday        int    `json:"weekday"`
	DayOfMonth     int    `json:"day_of_month"`
	CronExpression string `json:"cron_expression"`
}

func previewServerRestartSchedule(c *gin.Context) {
	var request schedulePreviewRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		return
	}
	value := config.Current().ServerProcess
	value.ScheduledRestartFrequency = request.Frequency
	value.ScheduledRestartTime = request.Time
	value.ScheduledRestartIntervalDays = request.IntervalDays
	value.ScheduledRestartStartDate = request.StartDate
	value.ScheduledRestartWeekday = request.Weekday
	value.ScheduledRestartDayOfMonth = request.DayOfMonth
	value.ScheduledRestartCron = request.CronExpression
	if err := config.ValidateServerProcess(value); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		return
	}
	items, err := supervisor.PreviewScheduledRestarts(value, time.Now(), 3)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"items":       items,
		"description": supervisor.DescribeScheduledRestart(value),
		"timezone":    time.Now().Location().String(),
	})
}

func getServerUpdate(manager ServerProcessManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		if manager == nil {
			c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "server process supervisor is unavailable"})
			return
		}
		c.JSON(http.StatusOK, manager.ServerUpdateStatus())
	}
}

func checkServerUpdate(manager ServerProcessManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		if manager == nil {
			c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "server process supervisor is unavailable"})
			return
		}
		status, err := manager.CheckServerUpdate()
		if err != nil {
			writeSupervisorError(c, err)
			return
		}
		c.JSON(http.StatusOK, status)
	}
}

func applyServerUpdate(manager ServerProcessManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		if manager == nil {
			c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "server process supervisor is unavailable"})
			return
		}
		settings := config.Current().ServerProcess
		request := applyUpdateRequest{
			ShutdownSeconds:     settings.GracefulShutdownSeconds,
			RestartDelaySeconds: settings.RestartDelaySeconds,
			Message:             "服务器将在 30 秒后更新，请提前回到安全位置。",
		}
		if err := c.ShouldBindJSON(&request); err != nil {
			c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
			return
		}
		if request.Confirmation != "UPDATE" {
			c.JSON(http.StatusBadRequest, ErrorResponse{Error: "请输入 UPDATE 确认更新"})
			return
		}
		if err := validateProcessRequest(request.ShutdownSeconds, request.RestartDelaySeconds, request.Message); err != nil {
			c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
			return
		}
		if !manager.ProcessStatus().Running {
			c.JSON(http.StatusBadRequest, ErrorResponse{Error: supervisor.ErrNotRunning.Error()})
			return
		}
		if err := manager.SaveWorld(); err != nil {
			c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "更新前保存世界失败：" + err.Error()})
			return
		}
		if _, err := createBackupRecord("update"); err != nil {
			c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "更新前备份失败：" + err.Error()})
			return
		}
		status, err := manager.ApplyServerUpdate(supervisor.RestartOptions{
			ShutdownSeconds: request.ShutdownSeconds,
			RestartDelay:    time.Duration(request.RestartDelaySeconds) * time.Second,
			Message:         request.Message,
		})
		if err != nil {
			writeSupervisorError(c, err)
			return
		}
		c.JSON(http.StatusOK, status)
	}
}

func getServerProcess(manager ServerProcessManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		if manager == nil {
			c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "server process supervisor is unavailable"})
			return
		}
		c.JSON(http.StatusOK, manager.ProcessStatus())
	}
}

func saveServer(manager ServerProcessManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		if manager == nil {
			c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "server process supervisor is unavailable"})
			return
		}
		if err := manager.SaveWorld(); err != nil {
			logger.Errorf("PalServer REST save failed: %v\n", err)
			c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "save world: " + err.Error()})
			return
		}
		c.JSON(http.StatusOK, SuccessResponse{Success: true})
	}
}

func startServer(manager ServerProcessManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		if manager == nil {
			c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "server process supervisor is unavailable"})
			return
		}
		logger.Info("Administrator requested PalServer start\n")
		status, err := manager.Start()
		if err != nil {
			writeSupervisorError(c, err)
			return
		}
		c.JSON(http.StatusOK, status)
	}
}

func restartServer(manager ServerProcessManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		if manager == nil {
			c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "server process supervisor is unavailable"})
			return
		}
		settings := config.Current().ServerProcess
		request := restartServerRequest{
			ShutdownSeconds:     settings.GracefulShutdownSeconds,
			RestartDelaySeconds: settings.RestartDelaySeconds,
			Message:             settings.GracefulShutdownMessage,
		}
		if err := c.ShouldBindJSON(&request); err != nil {
			c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
			return
		}
		if err := validateProcessRequest(request.ShutdownSeconds, request.RestartDelaySeconds, request.Message); err != nil {
			c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
			return
		}
		status, err := manager.Restart(supervisor.RestartOptions{
			ShutdownSeconds: request.ShutdownSeconds,
			RestartDelay:    time.Duration(request.RestartDelaySeconds) * time.Second,
			Message:         request.Message,
		})
		if err != nil {
			writeSupervisorError(c, err)
			return
		}
		c.JSON(http.StatusOK, status)
	}
}

func stopServer(manager ServerProcessManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		if manager == nil {
			c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "server process supervisor is unavailable"})
			return
		}
		settings := config.Current().ServerProcess
		keepStopped := true
		request := stopServerRequest{
			ShutdownSeconds: settings.GracefulShutdownSeconds,
			Message:         "服务器将在 30 秒后关闭，请提前回到安全位置。",
			KeepStopped:     &keepStopped,
		}
		if err := c.ShouldBindJSON(&request); err != nil {
			c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
			return
		}
		if err := validateProcessRequest(request.ShutdownSeconds, 0, request.Message); err != nil {
			c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
			return
		}
		status, err := manager.Stop(supervisor.StopOptions{
			ShutdownSeconds: request.ShutdownSeconds,
			Message:         request.Message,
			KeepStopped:     request.KeepStopped == nil || *request.KeepStopped,
		})
		if err != nil {
			writeSupervisorError(c, err)
			return
		}
		c.JSON(http.StatusOK, status)
	}
}

func setServerWatchdog(manager ServerProcessManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		if manager == nil {
			c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "server process supervisor is unavailable"})
			return
		}
		var request watchdogRequest
		if err := c.ShouldBindJSON(&request); err != nil {
			c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
			return
		}
		if err := config.CurrentStore().SetServerProcessWatchdog(request.Enabled); err != nil {
			c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
			return
		}
		settings := config.Current()
		manager.UpdateConfig(settings.ServerProcess)
		status := manager.SetWatchdog(request.Enabled)
		c.JSON(http.StatusOK, status)
	}
}

func validateProcessRequest(shutdownSeconds, restartDelaySeconds int, message string) error {
	if shutdownSeconds < 0 || shutdownSeconds > 3600 {
		return errors.New("shutdown_seconds must be between 0 and 3600")
	}
	if restartDelaySeconds < 0 || restartDelaySeconds > 3600 {
		return errors.New("restart_delay_seconds must be between 0 and 3600")
	}
	if strings.TrimSpace(message) == "" {
		return errors.New("message cannot be empty")
	}
	return nil
}

func writeSupervisorError(c *gin.Context, err error) {
	status := http.StatusInternalServerError
	switch {
	case errors.Is(err, supervisor.ErrConflict):
		status = http.StatusConflict
	case errors.Is(err, supervisor.ErrProcessNotConfigured), errors.Is(err, supervisor.ErrNotRunning), errors.Is(err, supervisor.ErrUnsupportedPlatform):
		status = http.StatusBadRequest
	case errors.Is(err, supervisor.ErrInvalidConfig):
		status = http.StatusBadRequest
	case errors.Is(err, supervisor.ErrUpdateNotConfigured):
		status = http.StatusBadRequest
	}
	c.JSON(status, ErrorResponse{Error: err.Error()})
}
