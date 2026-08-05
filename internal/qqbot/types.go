package qqbot

import (
	"context"
	"time"

	"github.com/zaigie/palworld-server-tool/internal/config"
	"github.com/zaigie/palworld-server-tool/internal/supervisor"
)

// ProcessManager is deliberately limited to PalServer operations. The QQ bot
// cannot access PST process control, the Windows host, or arbitrary commands.
type ProcessManager interface {
	ProcessStatus() supervisor.Status
	Start() (supervisor.Status, error)
	Restart(supervisor.RestartOptions) (supervisor.Status, error)
	Stop(supervisor.StopOptions) (supervisor.Status, error)
}

type Status struct {
	Enabled         bool       `json:"enabled"`
	Connected       bool       `json:"connected"`
	BotQQ           string     `json:"bot_qq"`
	Nickname        string     `json:"nickname"`
	LastHeartbeatAt *time.Time `json:"last_heartbeat_at,omitempty"`
	LastConnectedAt *time.Time `json:"last_connected_at,omitempty"`
	LatencyMS       int64      `json:"latency_ms"`
	ReconnectCount  int        `json:"reconnect_count"`
	LastError       string     `json:"last_error"`
}

type Group struct {
	GroupID        string `json:"group_id"`
	GroupName      string `json:"group_name"`
	MemberCount    int    `json:"member_count"`
	MaxMemberCount int    `json:"max_member_count"`
}

type ConnectionTest struct {
	Success   bool   `json:"success"`
	BotQQ     string `json:"bot_qq"`
	Nickname  string `json:"nickname"`
	LatencyMS int64  `json:"latency_ms"`
}

type AIConnectionTest struct {
	Success bool   `json:"success"`
	Model   string `json:"model"`
	Message string `json:"message"`
}

type PublicConfig struct {
	Enabled            bool                           `json:"enabled"`
	OneBotWebSocketURL string                         `json:"onebot_websocket_url"`
	TokenIsSet         bool                           `json:"token_is_set"`
	AllowedGroupIDs    []string                       `json:"allowed_group_ids"`
	AdminQQIDs         []string                       `json:"admin_qq_ids"`
	TriggerMode        string                         `json:"trigger_mode"`
	UserRatePerMinute  int                            `json:"user_rate_per_minute"`
	GroupRatePerMinute int                            `json:"group_rate_per_minute"`
	Permissions        config.QQBotPermissionsConfig  `json:"permissions"`
	Notifications      config.QQBotNotificationConfig `json:"notifications"`
	Persona            config.QQBotPersonaConfig      `json:"persona"`
	AI                 PublicAIConfig                 `json:"ai"`
}

type PublicAIConfig struct {
	Enabled             bool   `json:"enabled"`
	BaseURL             string `json:"base_url"`
	Model               string `json:"model"`
	TimeoutSeconds      int    `json:"timeout_seconds"`
	MaxToolCalls        int    `json:"max_tool_calls"`
	SendRedactedResults bool   `json:"send_redacted_results"`
	APIKeyIsSet         bool   `json:"api_key_is_set"`
}

func Redact(value config.QQBotConfig) PublicConfig {
	value = config.NormalizeQQBot(value)
	return PublicConfig{
		Enabled:            value.Enabled,
		OneBotWebSocketURL: value.OneBotWebSocketURL,
		TokenIsSet:         value.OneBotToken != "",
		AllowedGroupIDs:    append([]string(nil), value.AllowedGroupIDs...),
		AdminQQIDs:         append([]string(nil), value.AdminQQIDs...),
		TriggerMode:        value.TriggerMode,
		UserRatePerMinute:  value.UserRatePerMinute,
		GroupRatePerMinute: value.GroupRatePerMinute,
		Permissions:        value.Permissions,
		Notifications:      value.Notifications,
		Persona:            value.Persona,
		AI: PublicAIConfig{
			Enabled:             value.AI.Enabled,
			BaseURL:             value.AI.BaseURL,
			Model:               value.AI.Model,
			TimeoutSeconds:      value.AI.TimeoutSeconds,
			MaxToolCalls:        value.AI.MaxToolCalls,
			SendRedactedResults: value.AI.SendRedactedResults,
			APIKeyIsSet:         value.AI.APIKey != "",
		},
	}
}

type Conversation struct {
	Type    string
	GroupID string
	UserID  string
}

func (c Conversation) key() string {
	return c.Type + ":" + c.GroupID + ":" + c.UserID
}

// chatEntry 是缓存在内存里的会话历史消息，只保存脱敏后的最终 user/assistant
// 文本，不保存工具调用过程，回放时可直接作为普通消息发给模型，也不会触发
// DeepSeek thinking 模式对 reasoning_content 的回传要求。
type chatEntry struct {
	Role    string
	Content string
}

// chatHistory 记录单个会话（群:用户）最近几轮的最终回复，带最近更新时间，
// 超过 historyTTL 即视为过期并丢弃。
type chatHistory struct {
	Entries   []chatEntry
	UpdatedAt time.Time
}

type pendingAction struct {
	Code         string
	Conversation Conversation
	Kind         string
	BaseID       string
	Name         string
	ExpiresAt    time.Time
}

type actionResponse struct {
	Status  string `json:"status"`
	RetCode int    `json:"retcode"`
	Data    any    `json:"data"`
	Echo    string `json:"echo"`
}

type jsonObject map[string]any

type requestWaiter struct {
	response chan actionResponse
	created  time.Time
}

type membershipCacheEntry struct {
	Allowed   bool
	ExpiresAt time.Time
}

type MessageSender interface {
	Send(context.Context, Conversation, string) error
}
