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
			Message:         "Server shutting down in 30 seconds",
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
	}
	c.JSON(status, ErrorResponse{Error: err.Error()})
}
