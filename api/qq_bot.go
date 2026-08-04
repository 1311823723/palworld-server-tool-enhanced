package api

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/zaigie/palworld-server-tool/internal/config"
	"github.com/zaigie/palworld-server-tool/internal/qqbot"
)

type qqBotConfigUpdateRequest struct {
	config.QQBotConfig
	ClearOneBotToken bool `json:"clear_onebot_token"`
	ClearAIAPIKey    bool `json:"clear_ai_api_key"`
}

type qqBotConnectionTestRequest struct {
	OneBotWebSocketURL string `json:"onebot_websocket_url"`
	OneBotToken        string `json:"onebot_token"`
}

type qqBotTestMessageRequest struct {
	GroupID string `json:"group_id" binding:"required"`
}

type qqBotAITestRequest struct {
	APIKey         string `json:"api_key"`
	BaseURL        string `json:"base_url"`
	Model          string `json:"model"`
	TimeoutSeconds int    `json:"timeout_seconds"`
}

func getQQBotConfig(c *gin.Context) {
	c.JSON(http.StatusOK, qqbot.Redact(config.Current().QQBot))
}

func putQQBotConfig(manager *qqbot.Manager) gin.HandlerFunc {
	return func(c *gin.Context) {
		if manager == nil {
			c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "QQ 机器人服务尚未初始化"})
			return
		}
		var request qqBotConfigUpdateRequest
		if err := c.ShouldBindJSON(&request); err != nil {
			c.JSON(http.StatusBadRequest, ErrorResponse{Error: "配置格式不正确"})
			return
		}
		previous := config.Current().QQBot
		value := request.QQBotConfig
		if request.ClearOneBotToken {
			value.OneBotToken = ""
		} else if strings.TrimSpace(value.OneBotToken) == "" {
			value.OneBotToken = previous.OneBotToken
		}
		if request.ClearAIAPIKey {
			value.AI.APIKey = ""
		} else if strings.TrimSpace(value.AI.APIKey) == "" {
			value.AI.APIKey = previous.AI.APIKey
		}
		value = config.NormalizeQQBot(value)
		if err := config.ValidateQQBot(value); err != nil {
			c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
			return
		}
		if err := config.CurrentStore().SetQQBot(value); err != nil {
			c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "保存 QQ 机器人配置失败"})
			return
		}
		if err := manager.UpdateConfig(value); err != nil {
			c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "配置已保存，但热重载失败，请重启 PST"})
			return
		}
		c.JSON(http.StatusOK, qqbot.Redact(value))
	}
}

func getQQBotStatus(manager *qqbot.Manager) gin.HandlerFunc {
	return func(c *gin.Context) {
		if manager == nil {
			c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "QQ 机器人服务尚未初始化"})
			return
		}
		c.JSON(http.StatusOK, manager.Status())
	}
}

func testQQBotConnection(manager *qqbot.Manager) gin.HandlerFunc {
	return func(c *gin.Context) {
		if manager == nil {
			c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "QQ 机器人服务尚未初始化"})
			return
		}
		var request qqBotConnectionTestRequest
		if err := c.ShouldBindJSON(&request); err != nil {
			c.JSON(http.StatusBadRequest, ErrorResponse{Error: "测试参数格式不正确"})
			return
		}
		value := manager.Config()
		if strings.TrimSpace(request.OneBotWebSocketURL) != "" {
			value.OneBotWebSocketURL = request.OneBotWebSocketURL
		}
		if strings.TrimSpace(request.OneBotToken) != "" {
			value.OneBotToken = request.OneBotToken
		}
		// Connection validation requires a token but does not require the bot to
		// be enabled in persisted settings.
		value.Enabled = true
		ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
		defer cancel()
		result, err := qqbot.TestConnection(ctx, value)
		if err != nil {
			c.JSON(http.StatusBadGateway, ErrorResponse{Error: err.Error()})
			return
		}
		c.JSON(http.StatusOK, result)
	}
}

func reconnectQQBot(manager *qqbot.Manager) gin.HandlerFunc {
	return func(c *gin.Context) {
		if manager == nil {
			c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "QQ 机器人服务尚未初始化"})
			return
		}
		manager.Reconnect()
		c.JSON(http.StatusAccepted, SuccessResponse{Success: true})
	}
}

func listQQBotGroups(manager *qqbot.Manager) gin.HandlerFunc {
	return func(c *gin.Context) {
		if manager == nil {
			c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "QQ 机器人服务尚未初始化"})
			return
		}
		ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
		defer cancel()
		groups, err := manager.Groups(ctx)
		if err != nil {
			c.JSON(http.StatusBadGateway, ErrorResponse{Error: err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"items": groups})
	}
}

func testQQBotMessage(manager *qqbot.Manager) gin.HandlerFunc {
	return func(c *gin.Context) {
		if manager == nil {
			c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "QQ 机器人服务尚未初始化"})
			return
		}
		var request qqBotTestMessageRequest
		if c.ShouldBindJSON(&request) != nil {
			c.JSON(http.StatusBadRequest, ErrorResponse{Error: "请选择允许群后再发送测试消息"})
			return
		}
		ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
		defer cancel()
		if err := manager.SendTestMessage(ctx, request.GroupID); err != nil {
			c.JSON(http.StatusBadGateway, ErrorResponse{Error: err.Error()})
			return
		}
		c.JSON(http.StatusOK, SuccessResponse{Success: true})
	}
}

func testQQBotAI(manager *qqbot.Manager) gin.HandlerFunc {
	return func(c *gin.Context) {
		if manager == nil {
			c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "QQ 机器人服务尚未初始化"})
			return
		}
		var request qqBotAITestRequest
		if err := c.ShouldBindJSON(&request); err != nil {
			c.JSON(http.StatusBadRequest, ErrorResponse{Error: "测试参数格式不正确"})
			return
		}
		value := manager.Config().AI
		value.Enabled = true
		if strings.TrimSpace(request.APIKey) != "" {
			value.APIKey = request.APIKey
		}
		if strings.TrimSpace(request.BaseURL) != "" {
			value.BaseURL = request.BaseURL
		}
		// The model is fixed by PST. Keep accepting the legacy request field for
		// API compatibility, but never allow it to select another model.
		value.Model = config.DeepSeekModelV4Flash
		if request.TimeoutSeconds > 0 {
			value.TimeoutSeconds = request.TimeoutSeconds
		}
		ctx, cancel := context.WithTimeout(c.Request.Context(), time.Duration(value.TimeoutSeconds+2)*time.Second)
		defer cancel()
		result, err := qqbot.TestAI(ctx, value)
		if err != nil {
			c.JSON(http.StatusBadGateway, ErrorResponse{Error: err.Error()})
			return
		}
		c.JSON(http.StatusOK, result)
	}
}
