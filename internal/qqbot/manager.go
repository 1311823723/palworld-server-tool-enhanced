package qqbot

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/zaigie/palworld-server-tool/internal/config"
	"github.com/zaigie/palworld-server-tool/internal/logger"
	"go.etcd.io/bbolt"
	"golang.org/x/net/websocket"
)

const (
	requestTimeout       = 8 * time.Second
	maximumQueuedNotices = 100
	noticeLifetime       = time.Hour
)

type queuedNotice struct {
	ID        string
	Message   string
	CreatedAt time.Time
}

type Manager struct {
	db      *bbolt.DB
	process ProcessManager

	mu              sync.RWMutex
	config          config.QQBotConfig
	status          Status
	ctx             context.Context
	cancel          context.CancelFunc
	connection      *websocket.Conn
	connectionEpoch uint64
	waiters         map[string]requestWaiter
	pendingActions  map[string]pendingAction
	seenMessages    map[string]time.Time
	userRequests    map[string][]time.Time
	groupRequests   map[string][]time.Time
	privateMembers  map[string]membershipCacheEntry
	queuedNotices   []queuedNotice
	sentNotices     map[string]time.Time

	writeMu sync.Mutex
}

func NewManager(db *bbolt.DB, process ProcessManager, value config.QQBotConfig) *Manager {
	ctx, cancel := context.WithCancel(context.Background())
	value = config.NormalizeQQBot(value)
	m := &Manager{
		db:             db,
		process:        process,
		config:         value,
		status:         Status{Enabled: value.Enabled},
		ctx:            ctx,
		cancel:         cancel,
		waiters:        make(map[string]requestWaiter),
		pendingActions: make(map[string]pendingAction),
		seenMessages:   make(map[string]time.Time),
		userRequests:   make(map[string][]time.Time),
		groupRequests:  make(map[string][]time.Time),
		privateMembers: make(map[string]membershipCacheEntry),
		sentNotices:    make(map[string]time.Time),
	}
	if value.Enabled {
		go m.connectLoop(ctx, value)
		go m.monitorNotifications(ctx)
	}
	return m
}

func (m *Manager) Close() {
	m.mu.Lock()
	m.cancel()
	connection := m.connection
	m.connection = nil
	m.status.Connected = false
	m.mu.Unlock()
	if connection != nil {
		_ = connection.Close()
	}
}

func (m *Manager) UpdateConfig(value config.QQBotConfig) error {
	value = config.NormalizeQQBot(value)
	if err := config.ValidateQQBot(value); err != nil {
		return err
	}
	m.mu.Lock()
	m.cancel()
	connection := m.connection
	ctx, cancel := context.WithCancel(context.Background())
	m.ctx = ctx
	m.cancel = cancel
	m.connection = nil
	m.connectionEpoch++
	m.config = value
	m.status = Status{Enabled: value.Enabled}
	m.waiters = make(map[string]requestWaiter)
	m.pendingActions = make(map[string]pendingAction)
	m.userRequests = make(map[string][]time.Time)
	m.groupRequests = make(map[string][]time.Time)
	m.privateMembers = make(map[string]membershipCacheEntry)
	m.mu.Unlock()
	if connection != nil {
		_ = connection.Close()
	}
	if value.Enabled {
		go m.connectLoop(ctx, value)
		go m.monitorNotifications(ctx)
	}
	return nil
}

func (m *Manager) Config() config.QQBotConfig {
	m.mu.RLock()
	defer m.mu.RUnlock()
	value := m.config
	value.AllowedGroupIDs = append([]string(nil), value.AllowedGroupIDs...)
	value.AdminQQIDs = append([]string(nil), value.AdminQQIDs...)
	value.Notifications.GroupIDs = append([]string(nil), value.Notifications.GroupIDs...)
	return value
}

func (m *Manager) Status() Status {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.status
}

func (m *Manager) Reconnect() {
	m.mu.RLock()
	value := m.config
	m.mu.RUnlock()
	_ = m.UpdateConfig(value)
}

func dialOneBot(value config.QQBotConfig) (*websocket.Conn, error) {
	if err := config.ValidateQQBot(config.NormalizeQQBot(value)); err != nil {
		return nil, err
	}
	wsConfig, err := websocket.NewConfig(value.OneBotWebSocketURL, "http://127.0.0.1")
	if err != nil {
		return nil, err
	}
	wsConfig.Dialer = &net.Dialer{Timeout: 5 * time.Second}
	wsConfig.Header = make(http.Header)
	wsConfig.Header.Set("Authorization", "Bearer "+value.OneBotToken)
	connection, err := websocket.DialConfig(wsConfig)
	if err != nil {
		return nil, readableConnectionError(err)
	}
	return connection, nil
}

func readableConnectionError(err error) error {
	if err == nil {
		return nil
	}
	message := strings.ToLower(err.Error())
	switch {
	case strings.Contains(message, "401") || strings.Contains(message, "403"):
		return errors.New("NapCat 拒绝连接，请检查 OneBot Token 是否一致")
	case strings.Contains(message, "connection refused") || strings.Contains(message, "actively refused"):
		return errors.New("无法连接 NapCat，请确认正向 WebSocket 已启动并监听本机地址")
	case strings.Contains(message, "timeout"):
		return errors.New("连接 NapCat 超时")
	case strings.Contains(message, "eof") || strings.Contains(message, "closed network connection") || strings.Contains(message, "connection reset"):
		return errors.New("NapCat 连接已断开")
	default:
		return fmt.Errorf("连接 NapCat 失败: %w", err)
	}
}

func (m *Manager) connectLoop(ctx context.Context, value config.QQBotConfig) {
	delays := []time.Duration{time.Second, 2 * time.Second, 5 * time.Second, 10 * time.Second, 30 * time.Second}
	attempt := 0
	for ctx.Err() == nil {
		connection, err := dialOneBot(value)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			m.recordConnectionError(err)
			if !waitContext(ctx, delays[min(attempt, len(delays)-1)]) {
				return
			}
			attempt++
			continue
		}
		if ctx.Err() != nil {
			_ = connection.Close()
			return
		}
		attempt = 0
		if err := m.serveConnection(ctx, connection); err != nil && ctx.Err() == nil {
			m.recordConnectionError(err)
		}
		if ctx.Err() == nil && !waitContext(ctx, delays[0]) {
			return
		}
	}
}

func (m *Manager) recordConnectionError(err error) {
	m.mu.Lock()
	m.status.Connected = false
	m.status.ReconnectCount++
	m.status.LastError = readableConnectionError(err).Error()
	m.mu.Unlock()
}

func (m *Manager) serveConnection(ctx context.Context, connection *websocket.Conn) error {
	m.mu.Lock()
	if ctx != m.ctx || ctx.Err() != nil {
		m.mu.Unlock()
		_ = connection.Close()
		return context.Canceled
	}
	m.connectionEpoch++
	epoch := m.connectionEpoch
	m.connection = connection
	now := time.Now().UTC()
	m.status.Connected = true
	m.status.LastConnectedAt = &now
	m.status.LastError = ""
	m.mu.Unlock()

	errCh := make(chan error, 1)
	go func() { errCh <- m.receiveLoop(ctx, connection, epoch) }()

	callCtx, cancel := context.WithTimeout(ctx, requestTimeout)
	started := time.Now()
	response, err := m.callAction(callCtx, "get_login_info", jsonObject{})
	cancel()
	if err != nil {
		_ = connection.Close()
		<-errCh
		return fmt.Errorf("读取机器人账号失败: %w", err)
	}
	data := asObject(response.Data)
	m.mu.Lock()
	m.status.BotQQ = identifier(data["user_id"])
	m.status.Nickname = stringValue(data["nickname"])
	m.status.LatencyMS = time.Since(started).Milliseconds()
	m.mu.Unlock()
	go m.flushNotices(ctx)

	select {
	case err := <-errCh:
		m.clearConnection(connection, epoch)
		return err
	case <-ctx.Done():
		_ = connection.Close()
		<-errCh
		m.clearConnection(connection, epoch)
		return ctx.Err()
	}
}

func (m *Manager) clearConnection(connection *websocket.Conn, epoch uint64) {
	m.mu.Lock()
	if m.connection == connection && m.connectionEpoch == epoch {
		m.connection = nil
		m.status.Connected = false
	}
	m.mu.Unlock()
}

func (m *Manager) receiveLoop(ctx context.Context, connection *websocket.Conn, epoch uint64) error {
	for ctx.Err() == nil {
		var payload []byte
		if err := websocket.Message.Receive(connection, &payload); err != nil {
			return err
		}
		decoder := json.NewDecoder(bytes.NewReader(payload))
		decoder.UseNumber()
		var event jsonObject
		if err := decoder.Decode(&event); err != nil {
			continue
		}
		if echo := stringValue(event["echo"]); echo != "" {
			response := actionResponse{Status: stringValue(event["status"]), RetCode: intValue(event["retcode"]), Data: event["data"], Echo: echo}
			m.mu.Lock()
			waiter, found := m.waiters[echo]
			if found {
				delete(m.waiters, echo)
			}
			m.mu.Unlock()
			if found {
				select {
				case waiter.response <- response:
				default:
				}
			}
			continue
		}
		if stringValue(event["meta_event_type"]) == "heartbeat" {
			now := time.Now().UTC()
			m.mu.Lock()
			if m.connectionEpoch == epoch {
				m.status.LastHeartbeatAt = &now
			}
			m.mu.Unlock()
			continue
		}
		if stringValue(event["post_type"]) == "message" {
			go m.handleMessage(ctx, event)
		}
	}
	return ctx.Err()
}

func (m *Manager) callAction(ctx context.Context, action string, params jsonObject) (actionResponse, error) {
	m.mu.RLock()
	connection := m.connection
	m.mu.RUnlock()
	if connection == nil {
		return actionResponse{}, errors.New("QQ 机器人尚未连接 NapCat")
	}
	echo := uuid.NewString()
	waiter := requestWaiter{response: make(chan actionResponse, 1), created: time.Now()}
	m.mu.Lock()
	m.waiters[echo] = waiter
	m.mu.Unlock()
	payload, err := json.Marshal(jsonObject{"action": action, "params": params, "echo": echo})
	if err != nil {
		return actionResponse{}, readableConnectionError(err)
	}
	m.writeMu.Lock()
	err = websocket.Message.Send(connection, payload)
	m.writeMu.Unlock()
	if err != nil {
		m.mu.Lock()
		delete(m.waiters, echo)
		m.mu.Unlock()
		return actionResponse{}, err
	}
	select {
	case response := <-waiter.response:
		if response.Status != "ok" || response.RetCode != 0 {
			return response, fmt.Errorf("NapCat 操作失败（代码 %d）", response.RetCode)
		}
		return response, nil
	case <-ctx.Done():
		m.mu.Lock()
		delete(m.waiters, echo)
		m.mu.Unlock()
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return actionResponse{}, errors.New("NapCat 响应超时")
		}
		return actionResponse{}, errors.New("NapCat 请求已取消")
	}
}

func (m *Manager) Send(ctx context.Context, conversation Conversation, message string) error {
	// Use an explicit text segment so save-derived player/base/item names cannot
	// be interpreted as CQ codes by OneBot implementations.
	message = m.personaReply(message)
	params := jsonObject{"message": []jsonObject{{"type": "text", "data": jsonObject{"text": message}}}}
	action := "send_private_msg"
	if conversation.Type == "group" {
		action = "send_group_msg"
		params["group_id"] = conversation.GroupID
	} else {
		params["user_id"] = conversation.UserID
	}
	_, err := m.callAction(ctx, action, params)
	return err
}

func (m *Manager) Groups(ctx context.Context) ([]Group, error) {
	response, err := m.callAction(ctx, "get_group_list", jsonObject{"no_cache": true})
	if err != nil {
		return nil, err
	}
	rows := asArray(response.Data)
	groups := make([]Group, 0, len(rows))
	for _, row := range rows {
		item := asObject(row)
		groups = append(groups, Group{
			GroupID: identifier(item["group_id"]), GroupName: stringValue(item["group_name"]),
			MemberCount: intValue(item["member_count"]), MaxMemberCount: intValue(item["max_member_count"]),
		})
	}
	return groups, nil
}

func (m *Manager) SendTestMessage(ctx context.Context, groupID string) error {
	groupID = strings.TrimSpace(groupID)
	value := m.Config()
	if !contains(value.AllowedGroupIDs, groupID) {
		return errors.New("测试群不在允许群列表中")
	}
	return m.Send(ctx, Conversation{Type: "group", GroupID: groupID}, "PST QQ 机器人连接正常。")
}

func TestConnection(ctx context.Context, value config.QQBotConfig) (ConnectionTest, error) {
	connection, err := dialOneBot(value)
	if err != nil {
		return ConnectionTest{}, err
	}
	defer connection.Close()
	_ = connection.SetDeadline(time.Now().Add(requestTimeout))
	echo := uuid.NewString()
	payload, _ := json.Marshal(jsonObject{"action": "get_login_info", "params": jsonObject{}, "echo": echo})
	started := time.Now()
	if err := websocket.Message.Send(connection, payload); err != nil {
		return ConnectionTest{}, err
	}
	for {
		var raw []byte
		if err := websocket.Message.Receive(connection, &raw); err != nil {
			return ConnectionTest{}, readableConnectionError(err)
		}
		decoder := json.NewDecoder(bytes.NewReader(raw))
		decoder.UseNumber()
		var response jsonObject
		if decoder.Decode(&response) != nil || stringValue(response["echo"]) != echo {
			continue
		}
		if stringValue(response["status"]) != "ok" || intValue(response["retcode"]) != 0 {
			return ConnectionTest{}, errors.New("NapCat 已连接，但读取机器人账号失败")
		}
		data := asObject(response["data"])
		return ConnectionTest{Success: true, BotQQ: identifier(data["user_id"]), Nickname: stringValue(data["nickname"]), LatencyMS: time.Since(started).Milliseconds()}, nil
	}
}

func (m *Manager) Notify(id, message string) {
	value := m.Config()
	if !value.Notifications.Enabled || strings.TrimSpace(message) == "" {
		return
	}
	now := time.Now().UTC()
	m.mu.Lock()
	for noticeID, sentAt := range m.sentNotices {
		if now.Sub(sentAt) >= 24*time.Hour {
			delete(m.sentNotices, noticeID)
		}
	}
	if sentAt, found := m.sentNotices[id]; found && now.Sub(sentAt) < 24*time.Hour {
		m.mu.Unlock()
		return
	}
	m.sentNotices[id] = now
	connected := m.status.Connected
	if !connected {
		m.queuedNotices = append(m.queuedNotices, queuedNotice{ID: id, Message: message, CreatedAt: now})
		if len(m.queuedNotices) > maximumQueuedNotices {
			m.queuedNotices = m.queuedNotices[len(m.queuedNotices)-maximumQueuedNotices:]
		}
		m.mu.Unlock()
		return
	}
	m.mu.Unlock()
	go m.sendNotice(value, message)
}

func (m *Manager) sendNotice(value config.QQBotConfig, message string) {
	for _, groupID := range value.Notifications.GroupIDs {
		ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
		err := m.Send(ctx, Conversation{Type: "group", GroupID: groupID}, message)
		cancel()
		if err != nil {
			logger.Errorf("QQ notification failed for group %s: %v\n", groupID, err)
		}
	}
}

func (m *Manager) flushNotices(ctx context.Context) {
	m.mu.Lock()
	now := time.Now().UTC()
	items := make([]queuedNotice, 0, len(m.queuedNotices))
	for _, item := range m.queuedNotices {
		if now.Sub(item.CreatedAt) <= noticeLifetime {
			items = append(items, item)
		}
	}
	m.queuedNotices = nil
	value := m.config
	m.mu.Unlock()
	for _, item := range items {
		if ctx.Err() != nil {
			return
		}
		m.sendNotice(value, item.Message)
	}
}

func waitContext(ctx context.Context, duration time.Duration) bool {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-timer.C:
		return true
	case <-ctx.Done():
		return false
	}
}

func asObject(value any) jsonObject {
	if result, ok := value.(map[string]any); ok {
		return result
	}
	return jsonObject{}
}

func asArray(value any) []any {
	if result, ok := value.([]any); ok {
		return result
	}
	return nil
}

func stringValue(value any) string {
	switch current := value.(type) {
	case string:
		return current
	case json.Number:
		return current.String()
	case float64:
		return strconv.FormatInt(int64(current), 10)
	case int64:
		return strconv.FormatInt(current, 10)
	case int:
		return strconv.Itoa(current)
	default:
		return ""
	}
}

func identifier(value any) string { return strings.TrimSpace(stringValue(value)) }

func intValue(value any) int {
	parsed, _ := strconv.Atoi(stringValue(value))
	return parsed
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if strings.TrimSpace(value) == target {
			return true
		}
	}
	return false
}
