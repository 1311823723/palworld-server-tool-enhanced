package qqbot

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/zaigie/palworld-server-tool/internal/config"
	"github.com/zaigie/palworld-server-tool/internal/database"
	"github.com/zaigie/palworld-server-tool/internal/logger"
	"github.com/zaigie/palworld-server-tool/internal/supervisor"
	"github.com/zaigie/palworld-server-tool/service"
)

func (m *Manager) handleMessage(parent context.Context, event jsonObject) {
	conversation, text, messageID, ok := m.authorizeMessage(parent, event)
	if !ok || text == "" || !m.acceptMessage(messageID, conversation) {
		return
	}
	ctx, cancel := context.WithTimeout(parent, 30*time.Second)
	defer cancel()
	response, handled := m.handleLocalCommand(ctx, conversation, text)
	if !handled {
		var err error
		response, err = m.answerWithAI(ctx, conversation, text)
		if err != nil {
			logger.Warnf("AI 调用失败: %v", err)
			response = fmt.Sprintf("AI 暂时不可用（%s）。发送“帮助”查看可用命令。", err.Error())
		} else if strings.TrimSpace(response) == "" {
			response = "我暂时没理解这句话。发送“帮助”查看可用命令；DeepSeek 未配置或不可用时，基础命令仍可正常使用。"
		}
	}
	if err := m.Send(ctx, conversation, response); err != nil {
		m.recordConnectionError(err)
	}
}

func (m *Manager) authorizeMessage(ctx context.Context, event jsonObject) (Conversation, string, string, bool) {
	value := m.Config()
	messageType := stringValue(event["message_type"])
	userID := identifier(event["user_id"])
	messageID := identifier(event["message_id"])
	text, mentioned := messageText(event["message"], stringValue(event["raw_message"]), m.Status().BotQQ)
	switch messageType {
	case "group":
		groupID := identifier(event["group_id"])
		if !contains(value.AllowedGroupIDs, groupID) || !mentioned {
			return Conversation{}, "", "", false
		}
		return Conversation{Type: "group", GroupID: groupID, UserID: userID}, text, messageID, true
	case "private":
		if !contains(value.AdminQQIDs, userID) && !m.allowedPrivateMember(ctx, userID, value.AllowedGroupIDs) {
			return Conversation{}, "", "", false
		}
		return Conversation{Type: "private", UserID: userID}, text, messageID, true
	default:
		return Conversation{}, "", "", false
	}
}

func (m *Manager) allowedPrivateMember(parent context.Context, userID string, groups []string) bool {
	now := time.Now().UTC()
	m.mu.RLock()
	cached, found := m.privateMembers[userID]
	m.mu.RUnlock()
	if found && now.Before(cached.ExpiresAt) {
		return cached.Allowed
	}
	ctx, cancel := context.WithTimeout(parent, 5*time.Second)
	defer cancel()
	allowed := false
	for _, groupID := range groups {
		if _, err := m.callAction(ctx, "get_group_member_info", jsonObject{"group_id": groupID, "user_id": userID, "no_cache": false}); err == nil {
			allowed = true
			break
		}
		if ctx.Err() != nil {
			break
		}
	}
	m.mu.Lock()
	m.privateMembers[userID] = membershipCacheEntry{Allowed: allowed, ExpiresAt: now.Add(5 * time.Minute)}
	m.mu.Unlock()
	return allowed
}

func messageText(message any, raw, botQQ string) (string, bool) {
	mentioned := false
	parts := make([]string, 0)
	if segments, ok := message.([]any); ok {
		for _, segmentValue := range segments {
			segment := asObject(segmentValue)
			data := asObject(segment["data"])
			switch stringValue(segment["type"]) {
			case "at":
				if identifier(data["qq"]) == botQQ {
					mentioned = true
				}
			case "text":
				parts = append(parts, stringValue(data["text"]))
			}
		}
	}
	if len(parts) == 0 {
		if raw == "" {
			raw = stringValue(message)
		}
		parts = append(parts, raw)
		if botQQ != "" && strings.Contains(raw, "[CQ:at,qq="+botQQ+"]") {
			mentioned = true
		}
	}
	text := strings.Join(parts, " ")
	for strings.Contains(text, "[CQ:") {
		start := strings.Index(text, "[CQ:")
		end := strings.Index(text[start:], "]")
		if end < 0 {
			break
		}
		text = text[:start] + " " + text[start+end+1:]
	}
	return strings.TrimSpace(text), mentioned
}

func (m *Manager) acceptMessage(messageID string, conversation Conversation) bool {
	now := time.Now().UTC()
	m.mu.Lock()
	defer m.mu.Unlock()
	for key, seenAt := range m.seenMessages {
		if now.Sub(seenAt) > 10*time.Minute {
			delete(m.seenMessages, key)
		}
	}
	if messageID != "" {
		if _, duplicate := m.seenMessages[messageID]; duplicate {
			return false
		}
		m.seenMessages[messageID] = now
	}
	pruneRateRecords(m.userRequests, now)
	pruneRateRecords(m.groupRequests, now)
	userLimit := m.config.UserRatePerMinute
	groupLimit := m.config.GroupRatePerMinute
	if !withinRate(m.userRequests, conversation.UserID, userLimit, now) {
		return false
	}
	if conversation.Type == "group" && !withinRate(m.groupRequests, conversation.GroupID, groupLimit, now) {
		return false
	}
	return true
}

func pruneRateRecords(records map[string][]time.Time, now time.Time) {
	for key, values := range records {
		keep := values[:0]
		for _, item := range values {
			if now.Sub(item) < time.Minute {
				keep = append(keep, item)
			}
		}
		if len(keep) == 0 {
			delete(records, key)
		} else {
			records[key] = keep
		}
	}
}

func withinRate(records map[string][]time.Time, key string, limit int, now time.Time) bool {
	values := records[key][:0]
	for _, item := range records[key] {
		if now.Sub(item) < time.Minute {
			values = append(values, item)
		}
	}
	if len(values) >= limit {
		records[key] = values
		return false
	}
	records[key] = append(values, now)
	return true
}

func (m *Manager) handleLocalCommand(ctx context.Context, conversation Conversation, input string) (string, bool) {
	text := strings.TrimSpace(input)
	compact := strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) || strings.ContainsRune("，。！？?！", r) {
			return -1
		}
		return unicode.ToLower(r)
	}, text)
	if strings.HasPrefix(compact, "确认") {
		return m.confirmAction(conversation, strings.TrimSpace(strings.TrimPrefix(compact, "确认"))), true
	}
	if compact == "帮助" || compact == "命令" || compact == "help" {
		return commandHelp(), true
	}
	value := m.Config()
	if containsAny(compact, "服务器状态", "服务状态", "运行状态") {
		if !value.Permissions.QueryServerStatus {
			return "服务器状态查询未开放。", true
		}
		return m.serverStatusText(), true
	}
	if containsAny(compact, "谁在线", "在线玩家", "现在在线") {
		if !value.Permissions.QueryPlayers {
			return "玩家查询未开放。", true
		}
		return m.onlinePlayersText(), true
	}
	if strings.Contains(compact, "在线时间") || strings.Contains(compact, "上下线") || strings.Contains(compact, "最后在线") {
		if !value.Permissions.QueryPlayers {
			return "玩家查询未开放。", true
		}
		name := extractAfterKeywords(text, []string{"查询", "查一下", "看看"}, []string{"在线时间", "上下线时间", "最后在线", "上下线"})
		return m.playerPresenceText(name), true
	}
	if containsAny(compact, "最近备份", "最近一次备份", "备份状态", "上次备份") {
		if !value.Permissions.QueryBackups {
			return "备份查询未开放。", true
		}
		return m.backupText(), true
	}
	if containsAny(compact, "下次重启", "自动重启", "重启计划") {
		if !value.Permissions.QueryServerStatus {
			return "服务器状态查询未开放。", true
		}
		return m.restartScheduleText(), true
	}
	if containsAny(compact, "配种提醒", "产蛋提醒", "最近产蛋") {
		if !value.Permissions.QueryBreeding {
			return "配种提醒查询未开放。", true
		}
		return m.breedingText(), true
	}
	if strings.Contains(compact, "异常帕鲁") || strings.Contains(compact, "工作帕鲁") {
		if !value.Permissions.QueryBases {
			return "据点查询未开放。", true
		}
		name := strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(text, "有哪些异常帕鲁", ""), "异常帕鲁", ""))
		return m.baseWorkersText(name), true
	}
	if oldName, newName, ok := parseRename(text); ok {
		return m.requestRename(conversation, oldName, newName), true
	}
	if containsAny(compact, "重启服务器", "服务器重启", "重启服") {
		return m.requestProcessAction(conversation, "restart"), true
	}
	if containsAny(compact, "停止服务器", "关闭服务器", "服务器关机", "关服") || compact == "关机" {
		return m.requestProcessAction(conversation, "stop"), true
	}
	if containsAny(compact, "启动服务器", "开启服务器", "开服") {
		return m.requestProcessAction(conversation, "start"), true
	}
	if value.Permissions.QueryInventory {
		item := inventoryQueryWord(text)
		if item != "" {
			return m.inventoryText(item), true
		}
	}
	if compact == "据点" || compact == "据点列表" || compact == "有哪些据点" {
		if !value.Permissions.QueryBases {
			return "据点查询未开放。", true
		}
		return m.basesText(), true
	}
	return "", false
}

func commandHelp() string {
	return "可用命令：\n" +
		"• 服务器状态 / 现在谁在线\n" +
		"• 查询 张三 在线时间\n" +
		"• 石头还有多少 / 石头在哪\n" +
		"• 据点列表 / 第一据点异常帕鲁\n" +
		"• 配种提醒 / 最近一次备份 / 下次自动重启\n" +
		"管理员还可发起：把旧基地改名为第一据点、启动服务器、重启服务器、关服。危险操作需在 60 秒内回复六位验证码。"
}

func (m *Manager) serverStatusText() string {
	if m.process == nil {
		return "PalServer 进程管理尚未配置。"
	}
	status := m.process.ProcessStatus()
	state := map[supervisor.State]string{
		supervisor.StateStopped: "已停止", supervisor.StateStarting: "启动中", supervisor.StateRunning: "运行中",
		supervisor.StateStopping: "停止中", supervisor.StateRestartWaiting: "等待重启", supervisor.StateRestarting: "重启中",
		supervisor.StateCrashLoopStopped: "已触发崩溃循环保护", supervisor.StateError: "异常",
	}[status.State]
	if state == "" {
		state = string(status.State)
	}
	lines := []string{"服务器状态：" + state}
	if status.Running {
		lines = append(lines, fmt.Sprintf("PID：%d，已运行 %s", status.PID, humanDuration(status.UptimeSeconds)))
	}
	watchdog := "关闭"
	if status.WatchdogEnabled {
		watchdog = "开启"
	}
	lines = append(lines, "崩溃守护："+watchdog)
	if status.LastError != "" {
		lines = append(lines, "最近错误："+safeError(status.LastError))
	}
	return strings.Join(lines, "\n")
}

func (m *Manager) onlinePlayersText() string {
	players, err := service.ListPlayers(m.db)
	if err != nil {
		return "暂时无法读取玩家列表。"
	}
	online := make([]database.TersePlayer, 0)
	for _, player := range players {
		if player.IsOnline {
			online = append(online, player)
		}
	}
	if len(online) == 0 {
		return "当前没有玩家在线。"
	}
	sort.Slice(online, func(i, j int) bool { return online[i].CurrentSessionSeconds > online[j].CurrentSessionSeconds })
	lines := []string{fmt.Sprintf("当前 %d 人在线：", len(online))}
	for index, player := range online {
		if index == 10 {
			lines = append(lines, fmt.Sprintf("另有 %d 人在线", len(online)-index))
			break
		}
		lines = append(lines, fmt.Sprintf("• %s，本次 %s，累计 %s", displayPlayerName(player), humanDuration(player.CurrentSessionSeconds), humanDuration(player.TotalOnlineSeconds)))
	}
	return strings.Join(lines, "\n")
}

func (m *Manager) playerPresenceText(name string) string {
	players, err := service.ListPlayers(m.db)
	if err != nil {
		return "暂时无法读取玩家记录。"
	}
	name = strings.TrimSpace(name)
	matches := make([]database.TersePlayer, 0)
	for _, player := range players {
		if name == "" || strings.Contains(strings.ToLower(displayPlayerName(player)), strings.ToLower(name)) {
			matches = append(matches, player)
		}
	}
	if len(matches) == 0 {
		return "没有找到这名玩家。"
	}
	if name == "" && len(matches) > 5 {
		return "请带上玩家昵称，例如：查询 张三 在线时间。"
	}
	if len(matches) > 5 {
		matches = matches[:5]
	}
	lines := make([]string, 0, len(matches)*2)
	for _, player := range matches {
		status := "离线"
		detail := "最后在线：" + formatTime(player.LastOnline)
		if player.IsOnline {
			status = "在线，本次 " + humanDuration(player.CurrentSessionSeconds)
			detail = "上线时间：" + formatTime(player.OnlineSince)
		}
		entry := fmt.Sprintf("%s：%s\n累计在线：%s；%s", displayPlayerName(player), status, humanDuration(player.TotalOnlineSeconds), detail)
		events, eventErr := service.ListPlayerPresenceEvents(m.db, player.PlayerUid, 4)
		if eventErr == nil && len(events) > 0 {
			history := make([]string, 0, len(events))
			for _, event := range events {
				state := "下线"
				if event.Online {
					state = "上线"
				}
				history = append(history, state+" "+formatTime(event.CreatedAt))
			}
			entry += "\n最近记录：" + strings.Join(history, "；")
		}
		lines = append(lines, entry)
	}
	return strings.Join(lines, "\n")
}

func (m *Manager) inventoryText(item string) string {
	page, err := service.InventorySummary(m.db, service.InventoryQuery{Q: item, Page: 1, PageSize: 5})
	if err != nil {
		return "暂时无法读取库存快照。"
	}
	if len(page.Items) == 0 {
		return fmt.Sprintf("库存中没有找到“%s”。", item)
	}
	lines := []string{"库存快照："}
	for _, current := range page.Items {
		name := current.ItemDisplayName
		if name == "" {
			name = current.ItemName
		}
		if name == "" {
			name = current.ItemID
		}
		lines = append(lines, fmt.Sprintf("• %s：%d（%d 个位置）", name, current.TotalCount, current.LocationCount))
		locations, _, _, locationErr := service.InventoryLocations(m.db, current.ItemID, service.InventoryQuery{Page: 1, PageSize: 3})
		if locationErr == nil {
			for index, location := range locations {
				if index == 3 {
					break
				}
				place := location.BaseDisplayName
				if place == "" {
					place = location.PlayerName
				}
				if place == "" {
					place = location.ContainerName
				}
				lines = append(lines, fmt.Sprintf("  %s：%d", place, location.Count))
			}
		}
	}
	return strings.Join(lines, "\n")
}

func (m *Manager) basesText() string {
	bases, metadata, err := service.ListBaseCamps(m.db)
	if err != nil {
		return "暂时没有可用的据点快照。"
	}
	lines := []string{fmt.Sprintf("共 %d 个据点（解析于 %s）：", len(bases), formatTime(metadata.SnapshotTime))}
	for _, base := range bases {
		workers, _, _ := service.ListBaseWorkers(m.db, base.BaseID)
		lines = append(lines, fmt.Sprintf("• %s：%d 只工作帕鲁", base.DisplayName, len(workers)))
	}
	return strings.Join(lines, "\n")
}

func (m *Manager) baseWorkersText(name string) string {
	base, err := m.findBase(name)
	if err != nil {
		return err.Error()
	}
	workers, _, err := service.ListBaseWorkers(m.db, base.BaseID)
	if err != nil {
		return "暂时无法读取工作帕鲁。"
	}
	abnormal := make([]string, 0)
	for _, worker := range workers {
		reasons := worker.StatusAbnormalities
		if worker.IsDown != nil && *worker.IsDown {
			reasons = append(reasons, "倒地")
		}
		if worker.IsSick != nil && *worker.IsSick {
			reasons = append(reasons, "生病")
		}
		if worker.FullStomach != nil && *worker.FullStomach < service.LowFullStomach {
			reasons = append(reasons, "饥饿")
		}
		if worker.Sanity != nil && *worker.Sanity < service.LowSanity {
			reasons = append(reasons, "SAN 过低")
		}
		if len(reasons) > 0 {
			palName := worker.Nickname
			if palName == "" {
				palName = worker.PalID
			}
			abnormal = append(abnormal, fmt.Sprintf("• %s：%s", palName, strings.Join(uniqueStrings(reasons), "、")))
		}
	}
	if len(abnormal) == 0 {
		return fmt.Sprintf("%s 的 %d 只工作帕鲁目前没有明显异常。", base.DisplayName, len(workers))
	}
	if len(abnormal) > 10 {
		abnormal = abnormal[:10]
	}
	return fmt.Sprintf("%s 有 %d 只异常帕鲁：\n%s", base.DisplayName, len(abnormal), strings.Join(abnormal, "\n"))
}

func (m *Manager) breedingText() string {
	page, err := service.ListBreedingEvents(m.db, service.BreedingEventQuery{Page: 1, PageSize: 5})
	if err != nil || len(page.Items) == 0 {
		return "当前没有配种提醒记录。"
	}
	lines := []string{"最近配种提醒："}
	for _, event := range page.Items {
		name := event.BaseDisplayName
		if name == "" {
			name = "未命名据点"
		}
		lines = append(lines, fmt.Sprintf("• %s：产蛋数量 %d（%s）", name, event.CurrentCount, formatTime(event.CreatedAt)))
	}
	return strings.Join(lines, "\n")
}

func (m *Manager) backupText() string {
	backups, err := service.ListBackups(m.db, time.Time{}, time.Time{})
	if err != nil || len(backups) == 0 {
		return "当前没有备份记录。"
	}
	backup := backups[len(backups)-1]
	status := backup.Status
	if status == "" || status == "success" {
		status = "成功"
	} else if status == "failed" {
		status = "失败"
	}
	return fmt.Sprintf("最近一次备份：%s\n来源：%s；状态：%s；大小：%s", formatTime(backup.SaveTime), backup.Source, status, humanBytes(backup.Size))
}

func (m *Manager) restartScheduleText() string {
	if m.process == nil {
		return "PalServer 进程管理尚未配置。"
	}
	status := m.process.ProcessStatus()
	if !status.ScheduledRestartEnabled {
		return "自动重启计划未启用。"
	}
	if status.NextScheduledRestartAt == nil {
		return "自动重启已启用，但暂时无法计算下次执行时间。"
	}
	return "下次自动重启：" + formatTime(*status.NextScheduledRestartAt) + "\n时区：" + status.ScheduledRestartTimezone
}

func (m *Manager) requestProcessAction(conversation Conversation, kind string) string {
	value := m.Config()
	if !contains(value.AdminQQIDs, conversation.UserID) {
		return "只有配置中的管理员 QQ 可以控制 PalServer。"
	}
	allowed := kind == "start" && value.Permissions.StartServer || kind == "restart" && value.Permissions.RestartServer || kind == "stop" && value.Permissions.StopServer
	if !allowed {
		return map[string]string{"start": "启动服务器权限未开启。", "restart": "重启服务器权限未开启。", "stop": "停服权限未开启。"}[kind]
	}
	if m.process == nil {
		return "PalServer 进程管理尚未配置。"
	}
	status := m.process.ProcessStatus()
	if kind == "start" && status.Running {
		return "PalServer 已经在运行。"
	}
	if (kind == "restart" || kind == "stop") && !status.Running {
		return "PalServer 当前没有运行。"
	}
	labels := map[string]string{"start": "启动 PalServer", "restart": "平滑重启 PalServer", "stop": "平滑停止 PalServer并保持关闭"}
	return m.createConfirmation(conversation, pendingAction{Kind: kind}, labels[kind])
}

func (m *Manager) requestRename(conversation Conversation, oldName, newName string) string {
	value := m.Config()
	if !contains(value.AdminQQIDs, conversation.UserID) {
		return "只有配置中的管理员 QQ 可以修改据点名称。"
	}
	if !value.Permissions.RenameBase {
		return "据点改名权限未开启。"
	}
	base, err := m.findBase(oldName)
	if err != nil {
		return err.Error()
	}
	name, err := service.NormalizeBaseAlias(newName)
	if err != nil {
		return err.Error()
	}
	return m.createConfirmation(conversation, pendingAction{Kind: "rename_base", BaseID: base.BaseID, Name: name}, fmt.Sprintf("将“%s”改名为“%s”", base.DisplayName, name))
}

func (m *Manager) createConfirmation(conversation Conversation, action pendingAction, description string) string {
	action.Code = randomCode()
	action.Conversation = conversation
	action.ExpiresAt = time.Now().UTC().Add(time.Minute)
	m.mu.Lock()
	for key, current := range m.pendingActions {
		if time.Now().UTC().After(current.ExpiresAt) {
			delete(m.pendingActions, key)
		}
	}
	m.pendingActions[conversation.key()] = action
	m.mu.Unlock()
	return fmt.Sprintf("即将%s。\n如确认，请在 60 秒内回复：确认 %s\n验证码仅限当前账号和当前会话使用。", description, action.Code)
}

func (m *Manager) confirmAction(conversation Conversation, code string) string {
	m.mu.Lock()
	action, found := m.pendingActions[conversation.key()]
	if found {
		delete(m.pendingActions, conversation.key())
	}
	m.mu.Unlock()
	if !found || time.Now().UTC().After(action.ExpiresAt) {
		return "确认请求不存在或已过期，请重新发起操作。"
	}
	if action.Code != code || action.Conversation != conversation {
		return "验证码不正确，操作未执行。"
	}
	value := m.Config()
	if !contains(value.AdminQQIDs, conversation.UserID) {
		return "管理员权限已变更，操作未执行。"
	}
	if action.Kind != "rename_base" && m.process == nil {
		return "PalServer 进程管理尚未配置，操作未执行。"
	}
	var err error
	switch action.Kind {
	case "rename_base":
		if !value.Permissions.RenameBase {
			return "据点改名权限已关闭，操作未执行。"
		}
		_, err = service.SetBaseAlias(m.db, action.BaseID, action.Name, time.Now().UTC())
	case "start":
		if !value.Permissions.StartServer {
			return "启动服务器权限已关闭，操作未执行。"
		}
		_, err = m.process.Start()
	case "restart":
		if !value.Permissions.RestartServer {
			return "重启服务器权限已关闭，操作未执行。"
		}
		processConfig := config.Current().ServerProcess
		_, err = m.process.Restart(supervisor.RestartOptions{ShutdownSeconds: processConfig.GracefulShutdownSeconds, RestartDelay: time.Duration(processConfig.RestartDelaySeconds) * time.Second, Message: processConfig.GracefulShutdownMessage})
	case "stop":
		if !value.Permissions.StopServer {
			return "停服权限已关闭，操作未执行。"
		}
		processConfig := config.Current().ServerProcess
		_, err = m.process.Stop(supervisor.StopOptions{ShutdownSeconds: processConfig.GracefulShutdownSeconds, Message: "服务器将在倒计时结束后关闭，请提前回到安全位置。", KeepStopped: true})
	default:
		return "未知操作，未执行。"
	}
	status := "success"
	if err != nil {
		status = "failed"
	}
	_ = service.AddOperationAudit(m.db, database.OperationAudit{Action: "qq_bot." + action.Kind, Status: status, Detail: fmt.Sprintf("QQ %s，会话 %s", conversation.UserID, conversation.Type), CreatedAt: time.Now().UTC()})
	if err != nil {
		return "操作失败：" + safeError(err.Error())
	}
	return "操作已提交。"
}

func (m *Manager) findBase(query string) (database.BaseCampSnapshot, error) {
	bases, _, err := service.ListBaseCamps(m.db)
	if err != nil {
		return database.BaseCampSnapshot{}, errors.New("暂时没有可用的据点快照")
	}
	query = strings.TrimSpace(query)
	if query == "" && len(bases) == 1 {
		return bases[0], nil
	}
	exact := make([]database.BaseCampSnapshot, 0)
	partial := make([]database.BaseCampSnapshot, 0)
	for _, base := range bases {
		if strings.EqualFold(base.DisplayName, query) || strings.EqualFold(base.BaseID, query) {
			exact = append(exact, base)
		} else if strings.Contains(strings.ToLower(base.DisplayName), strings.ToLower(query)) {
			partial = append(partial, base)
		}
	}
	if len(exact) == 1 {
		return exact[0], nil
	}
	if len(partial) == 1 {
		return partial[0], nil
	}
	if len(exact)+len(partial) > 1 {
		return database.BaseCampSnapshot{}, errors.New("匹配到多个据点，请使用完整据点名称")
	}
	return database.BaseCampSnapshot{}, fmt.Errorf("没有找到据点“%s”", query)
}

func parseRename(text string) (string, string, bool) {
	text = strings.TrimSpace(text)
	text = strings.TrimPrefix(text, "把")
	for _, separator := range []string{"改名为", "改成", "重命名为"} {
		if index := strings.Index(text, separator); index > 0 {
			oldName := strings.TrimSpace(text[:index])
			newName := strings.TrimSpace(text[index+len(separator):])
			if oldName != "" && newName != "" {
				return oldName, newName, true
			}
		}
	}
	return "", "", false
}

func inventoryQueryWord(text string) string {
	trimmed := strings.TrimSpace(text)
	for _, suffix := range []string{"还有多少", "有多少", "在哪些据点", "在哪里", "在哪", "库存"} {
		if strings.HasSuffix(trimmed, suffix) {
			return strings.TrimSpace(strings.TrimSuffix(trimmed, suffix))
		}
	}
	if strings.HasPrefix(trimmed, "查询库存") {
		return strings.TrimSpace(strings.TrimPrefix(trimmed, "查询库存"))
	}
	return ""
}

func extractAfterKeywords(text string, prefixes, suffixes []string) string {
	result := strings.TrimSpace(text)
	for _, prefix := range prefixes {
		result = strings.TrimSpace(strings.TrimPrefix(result, prefix))
	}
	for _, suffix := range suffixes {
		result = strings.TrimSpace(strings.TrimSuffix(result, suffix))
	}
	return result
}

func containsAny(value string, candidates ...string) bool {
	for _, candidate := range candidates {
		if strings.Contains(value, candidate) {
			return true
		}
	}
	return false
}

func randomCode() string {
	value, err := rand.Int(rand.Reader, big.NewInt(900000))
	if err != nil {
		return fmt.Sprintf("%06d", time.Now().UnixNano()%1000000)
	}
	return fmt.Sprintf("%06d", value.Int64()+100000)
}

func displayPlayerName(player database.TersePlayer) string {
	if strings.TrimSpace(player.Nickname) != "" {
		return player.Nickname
	}
	return "未命名玩家"
}

func humanDuration(seconds int64) string {
	if seconds < 60 {
		return fmt.Sprintf("%d 秒", max64(seconds, 0))
	}
	days := seconds / 86400
	hours := seconds % 86400 / 3600
	minutes := seconds % 3600 / 60
	parts := make([]string, 0, 3)
	if days > 0 {
		parts = append(parts, fmt.Sprintf("%d 天", days))
	}
	if hours > 0 {
		parts = append(parts, fmt.Sprintf("%d 小时", hours))
	}
	if minutes > 0 || len(parts) == 0 {
		parts = append(parts, fmt.Sprintf("%d 分钟", minutes))
	}
	return strings.Join(parts, " ")
}

func humanBytes(value int64) string {
	if value <= 0 {
		return "未知"
	}
	units := []string{"B", "KB", "MB", "GB"}
	current := float64(value)
	unit := 0
	for current >= 1024 && unit < len(units)-1 {
		current /= 1024
		unit++
	}
	return fmt.Sprintf("%.1f %s", current, units[unit])
}

func formatTime(value time.Time) string {
	if value.IsZero() {
		return "未知"
	}
	return value.Local().Format("2006-01-02 15:04:05")
}

func safeError(value string) string {
	lower := strings.ToLower(value)
	for _, forbidden := range []string{"authorization", "bearer", "token", "password", "api_key", "\\", ":/"} {
		if strings.Contains(lower, forbidden) {
			return "操作未成功，请在 PST 日志中查看脱敏后的原因。"
		}
	}
	if len(value) > 180 {
		value = value[:180]
	}
	return strings.TrimSpace(value)
}

func uniqueStrings(values []string) []string {
	seen := make(map[string]struct{})
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, found := seen[value]; found {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func max64(left, right int64) int64 {
	if left > right {
		return left
	}
	return right
}
