package qqbot

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/zaigie/palworld-server-tool/internal/config"
)

type deepSeekMessage struct {
	Role       string             `json:"role"`
	Content    string             `json:"content"`
	ToolCalls  []deepSeekToolCall `json:"tool_calls,omitempty"`
	ToolCallID string             `json:"tool_call_id,omitempty"`
	// DeepSeek V4 默认开启 thinking 模式。带工具调用的 assistant 消息必须把
	// reasoning_content 原样回传（即使为空字符串也不能省略），否则多轮请求
	// 返回 HTTP 400 "The reasoning_content in the thinking mode must be passed
	// back to the API"。用指针区分"未返回"和"返回了空字符串"。
	ReasoningContent *string `json:"reasoning_content,omitempty"`
}

type deepSeekToolCall struct {
	ID       string               `json:"id"`
	Type     string               `json:"type"`
	Function deepSeekToolFunction `json:"function"`
}

type deepSeekToolFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type deepSeekChoice struct {
	Message deepSeekMessage `json:"message"`
}

type deepSeekResponse struct {
	Choices []deepSeekChoice `json:"choices"`
	Error   *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

var (
	ipv4Pattern        = regexp.MustCompile(`\b(?:\d{1,3}\.){3}\d{1,3}\b`)
	longIDPattern      = regexp.MustCompile(`\b\d{15,20}\b`)
	uuidPattern        = regexp.MustCompile(`(?i)\b[0-9a-f]{8}-[0-9a-f-]{27,36}\b`)
	windowsPathPattern = regexp.MustCompile(`(?i)\b[A-Z]:\\[^\r\n，。；;]+`)
)

func (m *Manager) answerWithAI(ctx context.Context, conversation Conversation, text string) (string, error) {
	value := m.Config()
	if !value.AI.Enabled || strings.TrimSpace(value.AI.APIKey) == "" {
		return "", errors.New("DeepSeek 未启用")
	}
	messages := []deepSeekMessage{
		{Role: "system", Content: deepSeekSystemPrompt(value)},
		{Role: "user", Content: redactForAI(text)},
	}
	response, err := callDeepSeek(ctx, value.AI, messages, deepSeekTools())
	if err != nil {
		return "", err
	}
	if len(response.Choices) == 0 {
		return "", errors.New("DeepSeek 未返回结果")
	}
	message := response.Choices[0].Message
	if len(message.ToolCalls) == 0 {
		return strings.TrimSpace(message.Content), nil
	}
	results := make([]string, 0, min(len(message.ToolCalls), value.AI.MaxToolCalls))
	toolMessages := make([]deepSeekMessage, 0, len(message.ToolCalls))
	writeIssued := false
	for index, call := range message.ToolCalls {
		if index >= value.AI.MaxToolCalls {
			break
		}
		result := ""
		if isAIWriteTool(call.Function.Name) && writeIssued {
			result = "一次对话只能发起一个需要确认的操作。"
		} else {
			result = m.executeAITool(conversation, call.Function.Name, call.Function.Arguments)
			writeIssued = writeIssued || isAIWriteTool(call.Function.Name)
		}
		result = redactForAI(result)
		results = append(results, result)
		toolMessages = append(toolMessages, deepSeekMessage{Role: "tool", ToolCallID: call.ID, Content: result})
	}
	if len(results) == 0 {
		return "", errors.New("DeepSeek 返回了不允许的工具")
	}
	if !value.AI.SendRedactedResults || hasConfirmationResult(results) {
		return strings.Join(results, "\n"), nil
	}
	messages = append(messages, message)
	messages = append(messages, toolMessages...)
	finalResponse, err := callDeepSeek(ctx, value.AI, messages, nil)
	if err != nil || len(finalResponse.Choices) == 0 || strings.TrimSpace(finalResponse.Choices[0].Message.Content) == "" {
		return strings.Join(results, "\n"), nil
	}
	return redactForAI(finalResponse.Choices[0].Message.Content), nil
}

func isAIWriteTool(name string) bool {
	return name == "rename_base" || name == "start_server" || name == "restart_server" || name == "stop_server"
}

func (m *Manager) executeAITool(conversation Conversation, name, arguments string) string {
	var args struct {
		Name     string `json:"name"`
		Item     string `json:"item"`
		BaseName string `json:"base_name"`
		NewName  string `json:"new_name"`
	}
	if arguments != "" && json.Unmarshal([]byte(arguments), &args) != nil {
		return "工具参数无效。"
	}
	permissions := m.Config().Permissions
	switch name {
	case "get_server_status":
		if permissions.QueryServerStatus {
			return m.serverStatusText()
		}
	case "get_online_players":
		if permissions.QueryPlayers {
			return m.onlinePlayersText()
		}
	case "get_player_presence":
		if permissions.QueryPlayers {
			return m.playerPresenceText(args.Name)
		}
	case "search_inventory":
		if permissions.QueryInventory {
			return m.inventoryText(args.Item)
		}
	case "list_bases":
		if permissions.QueryBases {
			return m.basesText()
		}
	case "get_base_workers":
		if permissions.QueryBases {
			return m.baseWorkersText(args.BaseName)
		}
	case "get_breeding_alerts":
		if permissions.QueryBreeding {
			return m.breedingText()
		}
	case "get_backup_status":
		if permissions.QueryBackups {
			return m.backupText()
		}
	case "get_restart_schedule":
		if permissions.QueryServerStatus {
			return m.restartScheduleText()
		}
	case "rename_base":
		return m.requestRename(conversation, args.BaseName, args.NewName)
	case "start_server":
		return m.requestProcessAction(conversation, "start")
	case "restart_server":
		return m.requestProcessAction(conversation, "restart")
	case "stop_server":
		return m.requestProcessAction(conversation, "stop")
	default:
		return "该操作不在 PST QQ 机器人的工具白名单中。"
	}
	return "该查询权限未开放。"
}

func callDeepSeek(ctx context.Context, value config.QQBotAIConfig, messages []deepSeekMessage, tools []jsonObject) (deepSeekResponse, error) {
	value.Model = config.DeepSeekModelV4Flash
	endpoint := strings.TrimRight(value.BaseURL, "/") + "/chat/completions"
	payload := jsonObject{"model": value.Model, "messages": messages, "temperature": 0.1}
	if len(tools) > 0 {
		payload["tools"] = tools
		payload["tool_choice"] = "auto"
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return deepSeekResponse{}, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return deepSeekResponse{}, err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+value.APIKey)
	client := &http.Client{
		Timeout:   time.Duration(value.TimeoutSeconds) * time.Second,
		Transport: &http.Transport{Proxy: nil, DialContext: (&net.Dialer{Timeout: 8 * time.Second, KeepAlive: 30 * time.Second}).DialContext, TLSHandshakeTimeout: 8 * time.Second},
	}
	response, err := client.Do(request)
	if err != nil {
		return deepSeekResponse{}, fmt.Errorf("DeepSeek 连接失败: %w", err)
	}
	defer response.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return deepSeekResponse{}, err
	}
	var result deepSeekResponse
	if err := json.Unmarshal(responseBody, &result); err != nil {
		return deepSeekResponse{}, errors.New("DeepSeek 返回了无法解析的数据")
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		switch response.StatusCode {
		case http.StatusUnauthorized:
			return result, errors.New("DeepSeek API Key 无效")
		case http.StatusPaymentRequired, http.StatusTooManyRequests:
			return result, errors.New("DeepSeek 额度不足或请求过于频繁")
		default:
			// 带上 DeepSeek 返回的具体错误信息（如参数校验失败原因），便于定位。
			if result.Error != nil && strings.TrimSpace(result.Error.Message) != "" {
				return result, fmt.Errorf("DeepSeek 请求失败（HTTP %d）: %s", response.StatusCode, result.Error.Message)
			}
			return result, fmt.Errorf("DeepSeek 请求失败（HTTP %d）", response.StatusCode)
		}
	}
	return result, nil
}

func TestAI(ctx context.Context, value config.QQBotAIConfig) (AIConnectionTest, error) {
	value.Model = config.DeepSeekModelV4Flash
	if !value.Enabled || strings.TrimSpace(value.APIKey) == "" {
		return AIConnectionTest{}, errors.New("请先启用 DeepSeek 并设置 API Key")
	}
	validation := config.Default().QQBot
	validation.AI = value
	if err := config.ValidateQQBot(validation); err != nil {
		return AIConnectionTest{}, err
	}
	response, err := callDeepSeek(ctx, value, []deepSeekMessage{{Role: "user", Content: "只回复：连接正常"}}, nil)
	if err != nil {
		return AIConnectionTest{}, err
	}
	if len(response.Choices) == 0 {
		return AIConnectionTest{}, errors.New("DeepSeek 未返回测试结果")
	}
	return AIConnectionTest{Success: true, Model: value.Model, Message: strings.TrimSpace(response.Choices[0].Message.Content)}, nil
}

func deepSeekTools() []jsonObject {
	object := func(properties jsonObject, required ...string) jsonObject {
		return jsonObject{"type": "object", "properties": properties, "required": required, "additionalProperties": false}
	}
	stringProperty := func(description string) jsonObject { return jsonObject{"type": "string", "description": description} }
	tool := func(name, description string, parameters jsonObject) jsonObject {
		return jsonObject{"type": "function", "function": jsonObject{"name": name, "description": description, "parameters": parameters}}
	}
	empty := object(jsonObject{})
	return []jsonObject{
		tool("get_server_status", "查询 PalServer 运行状态", empty),
		tool("get_online_players", "查询当前在线玩家", empty),
		tool("get_player_presence", "查询玩家本次、累计和最近在线时间", object(jsonObject{"name": stringProperty("玩家昵称")}, "name")),
		tool("search_inventory", "按中文物品名称查询库存数量和位置", object(jsonObject{"item": stringProperty("物品名称")}, "item")),
		tool("list_bases", "列出当前据点", empty),
		tool("get_base_workers", "查询指定据点的异常工作帕鲁", object(jsonObject{"base_name": stringProperty("据点名称")}, "base_name")),
		tool("get_breeding_alerts", "查询最近配种产蛋提醒", empty),
		tool("get_backup_status", "查询最近一次存档备份", empty),
		tool("get_restart_schedule", "查询下次自动重启", empty),
		tool("rename_base", "发起据点自定义名称修改，必须二次确认", object(jsonObject{"base_name": stringProperty("现有据点名称"), "new_name": stringProperty("新名称")}, "base_name", "new_name")),
		tool("start_server", "发起 PalServer 启动，必须二次确认", empty),
		tool("restart_server", "发起 PalServer 平滑重启，必须二次确认", empty),
		tool("stop_server", "发起 PalServer 平滑停服并保持关闭，必须二次确认", empty),
	}
}

func redactForAI(value string) string {
	value = windowsPathPattern.ReplaceAllString(value, "[本机路径已隐藏]")
	value = ipv4Pattern.ReplaceAllString(value, "[IP已隐藏]")
	value = uuidPattern.ReplaceAllString(value, "[技术ID已隐藏]")
	value = longIDPattern.ReplaceAllString(value, "[技术ID已隐藏]")
	lower := strings.ToLower(value)
	for _, marker := range []string{"bearer ", "authorization:", "token=", "api_key="} {
		if index := strings.Index(lower, marker); index >= 0 {
			return strings.TrimSpace(value[:index]) + " [敏感内容已隐藏]"
		}
	}
	if len(value) > 4000 {
		value = value[:4000]
	}
	return strings.TrimSpace(value)
}

func hasConfirmationResult(values []string) bool {
	for _, value := range values {
		if strings.Contains(value, "确认") && strings.Contains(value, "验证码") {
			return true
		}
	}
	return false
}
