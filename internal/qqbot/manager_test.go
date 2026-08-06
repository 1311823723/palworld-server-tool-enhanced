package qqbot

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http/httptest"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/zaigie/palworld-server-tool/internal/config"
	"github.com/zaigie/palworld-server-tool/internal/database"
	"github.com/zaigie/palworld-server-tool/internal/supervisor"
	"github.com/zaigie/palworld-server-tool/service"
	"go.etcd.io/bbolt"
	"golang.org/x/net/websocket"
)

type fakeProcess struct {
	status   supervisor.Status
	starts   int
	restarts int
	stops    int
}

func (f *fakeProcess) ProcessStatus() supervisor.Status { return f.status }
func (f *fakeProcess) Start() (supervisor.Status, error) {
	f.starts++
	f.status.Running = true
	return f.status, nil
}
func (f *fakeProcess) Restart(supervisor.RestartOptions) (supervisor.Status, error) {
	f.restarts++
	return f.status, nil
}
func (f *fakeProcess) Stop(supervisor.StopOptions) (supervisor.Status, error) {
	f.stops++
	f.status.Running = false
	return f.status, nil
}

func TestDangerousActionRequiresBoundOneTimeConfirmation(t *testing.T) {
	store, err := config.Open(filepath.Join(t.TempDir(), "config.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	config.SetCurrent(store)
	db, err := bbolt.Open(filepath.Join(t.TempDir(), "pst.db"), 0600, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	settings := config.Default().QQBot
	settings.AdminQQIDs = []string{"90000000000000000001"}
	process := &fakeProcess{status: supervisor.Status{Running: true, State: supervisor.StateRunning}}
	manager := NewManager(db, process, settings)
	defer manager.Close()
	conversation := Conversation{Type: "group", GroupID: "90000000000000000002", UserID: "90000000000000000001"}
	prompt := manager.requestProcessAction(conversation, "restart")
	code := regexp.MustCompile(`\d{6}`).FindString(prompt)
	if code == "" || process.restarts != 0 {
		t.Fatalf("restart executed before confirmation: %q", prompt)
	}
	otherConversation := Conversation{Type: "group", GroupID: "90000000000000000003", UserID: "90000000000000000001"}
	if response := manager.confirmAction(otherConversation, code); !strings.Contains(response, "不存在") || process.restarts != 0 {
		t.Fatalf("cross-group confirmation accepted: %q", response)
	}
	if response := manager.confirmAction(conversation, code); response != "操作已提交。" || process.restarts != 1 {
		t.Fatalf("valid confirmation failed: %q, restarts=%d", response, process.restarts)
	}
	if response := manager.confirmAction(conversation, code); !strings.Contains(response, "不存在") || process.restarts != 1 {
		t.Fatalf("confirmation replay accepted: %q", response)
	}
}

func TestAIRedactionRemovesInfrastructureAndIdentifiers(t *testing.T) {
	input := `IP 192.168.1.2，路径 D:\\PalServer\\Saved，Steam 76561198000000000，ID 550e8400-e29b-41d4-a716-446655440000`
	result := redactForAI(input)
	for _, secret := range []string{"192.168.1.2", `D:\\PalServer`, "76561198000000000", "550e8400"} {
		if strings.Contains(result, secret) {
			t.Fatalf("AI redaction leaked %q in %q", secret, result)
		}
	}
}

func TestPersonaKeepsFactsAndUsesSeriousToneForErrors(t *testing.T) {
	value := config.Default().QQBot
	manager := &Manager{config: value}
	response := manager.personaReply("服务器状态：运行中\n在线玩家：6 人")
	if !strings.Contains(response, "本喵帮训练家查到啦") || !strings.Contains(response, "在线玩家：6 人") {
		t.Fatalf("lively persona response = %q", response)
	}

	response = manager.personaReply("PalServer 意外退出，最近错误：进程崩溃")
	if !strings.Contains(response, "出状况了") || !strings.Contains(response, "最近错误：进程崩溃") {
		t.Fatalf("serious persona response = %q", response)
	}

	value.Persona.Enabled = false
	manager.config = value
	plain := "服务器状态：运行中"
	if response = manager.personaReply(plain); response != plain {
		t.Fatalf("disabled persona changed response: %q", response)
	}
}

func TestLamballPersonaProducesDistinctIntro(t *testing.T) {
	value := config.Default().QQBot
	value.Persona.Character = config.QQBotPersonaCharacterLamball
	manager := &Manager{config: value}
	response := manager.personaReply("服务器状态：运行中\n在线玩家：6 人")
	if !strings.Contains(response, "棉悠悠") || !strings.Contains(response, "在线玩家：6 人") {
		t.Fatalf("lamball persona response = %q", response)
	}
	response = manager.personaReply("PalServer 意外退出，最近错误：进程崩溃")
	if !strings.Contains(response, "棉悠悠") || !strings.Contains(response, "最近错误：进程崩溃") {
		t.Fatalf("lamball serious persona response = %q", response)
	}
}

func TestZoePersonaIsTsundereButKeepsFacts(t *testing.T) {
	value := config.Default().QQBot
	value.Persona.Character = config.QQBotPersonaCharacterZoe
	manager := &Manager{config: value}
	response := manager.personaReply("服务器状态：运行中\n在线玩家：6 人")
	if !strings.Contains(response, "哼") || !strings.Contains(response, "别以为") || !strings.Contains(response, "在线玩家：6 人") {
		t.Fatalf("zoe persona response = %q", response)
	}
	prompt := deepSeekSystemPrompt(value)
	for _, required := range []string{"佐伊", "傲娇", "别误会", "不能侮辱用户"} {
		if !strings.Contains(prompt, required) {
			t.Fatalf("zoe prompt missing %q: %s", required, prompt)
		}
	}
}

func TestPersonaChatListsAndSwitchesForAdministrators(t *testing.T) {
	store, err := config.Open(filepath.Join(t.TempDir(), "config.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	config.SetCurrent(store)
	db, err := bbolt.Open(filepath.Join(t.TempDir(), "pst.db"), 0600, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	settings := config.Default().QQBot
	settings.AdminQQIDs = []string{"90000000000000000001"}
	manager := NewManager(db, nil, settings)
	defer manager.Close()
	admin := Conversation{Type: "private", UserID: "90000000000000000001"}
	visitor := Conversation{Type: "private", UserID: "90000000000000000003"}

	list, handled := manager.handleLocalCommand(context.Background(), admin, "切换人设")
	if !handled || !strings.Contains(list, "捣蛋喵") || !strings.Contains(list, "棉悠悠") || !strings.Contains(list, "佐伊") || !strings.Contains(list, "当前") {
		t.Fatalf("persona list = %q, handled=%v", list, handled)
	}
	if response, handled := manager.handleLocalCommand(context.Background(), visitor, "切换人设 棉悠悠"); !handled || !strings.Contains(response, "只有配置中的管理员") {
		t.Fatalf("non-admin persona switch = %q, handled=%v", response, handled)
	}
	if response, handled := manager.handleLocalCommand(context.Background(), admin, "切换人设 棉悠悠"); !handled || !strings.Contains(response, "已切换为棉悠悠") {
		t.Fatalf("admin persona switch = %q, handled=%v", response, handled)
	}
	if got := manager.Config().Persona.Character; got != config.QQBotPersonaCharacterLamball {
		t.Fatalf("manager persona character = %q", got)
	}
	if got := store.Config().QQBot.Persona.Character; got != config.QQBotPersonaCharacterLamball {
		t.Fatalf("persisted persona character = %q", got)
	}
	if response, handled := manager.handleLocalCommand(context.Background(), admin, "切换人设 3"); !handled || !strings.Contains(response, "已切换为佐伊") {
		t.Fatalf("admin Zoe persona switch = %q, handled=%v", response, handled)
	}
	if got := manager.Config().Persona.Character; got != config.QQBotPersonaCharacterZoe {
		t.Fatalf("manager Zoe persona character = %q", got)
	}
}

func TestDeepSeekPersonaPromptKeepsSafetyRules(t *testing.T) {
	value := config.Default().QQBot
	prompt := deepSeekSystemPrompt(value)
	for _, required := range []string{"捣蛋喵", "不得补全、推测或编造", "未调用工具时", "由 PST 生成二次确认", "不要每句话"} {
		if !strings.Contains(prompt, required) {
			t.Fatalf("DeepSeek persona prompt missing %q: %s", required, prompt)
		}
	}
	value.Persona.Enabled = false
	plainPrompt := deepSeekSystemPrompt(value)
	if strings.Contains(plainPrompt, "来自《幻兽帕鲁》世界的捣蛋喵") || !strings.Contains(plainPrompt, "不使用角色口癖") {
		t.Fatalf("disabled persona prompt = %s", plainPrompt)
	}
}

func TestParseDSMLToolCallsDoesNotLeakInternalMarkup(t *testing.T) {
	content := `找到了 2 个据点！
<｜｜DSML｜｜tool_calls>
<｜｜DSML｜｜invoke name="get_base_details">
<｜｜DSML｜｜parameter name="base_name" string="true">夏莱</｜｜DSML｜｜parameter>
</｜｜DSML｜｜invoke>
<｜｜DSML｜｜invoke name="get_base_details">
<｜｜DSML｜｜parameter name="base_name" string="true">工厂</｜｜DSML｜｜parameter>
</｜｜DSML｜｜invoke>
</｜｜DSML｜｜tool_calls>`
	cleaned, calls := parseDSMLToolCalls(content)
	if len(calls) != 2 {
		t.Fatalf("parsed calls = %#v, want 2", calls)
	}
	if strings.Contains(cleaned, "DSML") || strings.Contains(cleaned, "invoke") {
		t.Fatalf("internal DSML markup leaked: %q", cleaned)
	}
	if !strings.Contains(cleaned, "找到了 2 个据点") || !strings.Contains(calls[0].Function.Arguments, "夏莱") || !strings.Contains(calls[1].Function.Arguments, "工厂") {
		t.Fatalf("cleaned content or arguments incorrect: cleaned=%q calls=%#v", cleaned, calls)
	}
	if got := sanitizeAIText("前缀\n<｜｜DSML｜｜tool_calls>残缺"); got != "前缀" {
		t.Fatalf("truncated DSML was not removed: %q", got)
	}
}

func TestMessageParsingRequiresBotMention(t *testing.T) {
	segments := []any{
		map[string]any{"type": "at", "data": map[string]any{"qq": "90000000000000000001"}},
		map[string]any{"type": "text", "data": map[string]any{"text": " 服务器状态 "}},
	}
	text, mentioned := messageText(segments, "", "90000000000000000001")
	if !mentioned || text != "服务器状态" {
		t.Fatalf("unexpected parsed message: mentioned=%t text=%q", mentioned, text)
	}
	_, mentioned = messageText("服务器状态", "服务器状态", "90000000000000000001")
	if mentioned {
		t.Fatal("plain group message must not count as a mention")
	}
}

func TestInventoryCommandMatchesAndRepliesWithChineseItemName(t *testing.T) {
	db, err := bbolt.Open(filepath.Join(t.TempDir(), "pst.db"), 0600, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	_, err = service.PutSnapshot(db, database.SnapshotPayload{
		Metadata: database.SnapshotMetadata{SnapshotTime: time.Now().UTC(), SaveFileTime: time.Now().UTC()},
		InventorySlots: []database.InventoryLocation{{
			LocationID: "container-a:0", ItemID: "stone", ItemName: "Stone", Count: 128,
			ContainerID: "container-a", ContainerName: "金属箱", SourceType: "base_storage",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	manager := &Manager{db: db}
	response := manager.inventoryText("石头")
	if !strings.Contains(response, "石头：128") || strings.Contains(response, "没有找到") {
		t.Fatalf("Chinese inventory response = %q", response)
	}
}

func TestInventoryReconciliationLineShowsHiddenLocations(t *testing.T) {
	db, err := bbolt.Open(filepath.Join(t.TempDir(), "pst.db"), 0600, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	slots := make([]database.InventoryLocation, 0, 5)
	for i := 1; i <= 5; i++ {
		slots = append(slots, database.InventoryLocation{
			LocationID: fmt.Sprintf("c%d:0", i), ItemID: "stone", ItemName: "Stone", Count: 100,
			ContainerID: fmt.Sprintf("c%d", i), ContainerName: fmt.Sprintf("箱%d", i),
			SourceType: "base_storage", BaseID: "b1", BaseName: "第一据点",
		})
	}
	if _, err := service.PutSnapshot(db, database.SnapshotPayload{
		Metadata:       database.SnapshotMetadata{SnapshotTime: time.Now().UTC(), SaveFileTime: time.Now().UTC()},
		InventorySlots: slots,
	}); err != nil {
		t.Fatal(err)
	}
	manager := &Manager{db: db}
	response := manager.inventoryTextFiltered("石头", "b1", "")
	if !strings.Contains(response, "石头：500") {
		t.Fatalf("total should cover all 5 locations, reply = %q", response)
	}
	if !strings.Contains(response, "另有 2 处未列出：合计 200") {
		t.Fatalf("reconciliation line should cover the 2 hidden locations, reply = %q", response)
	}
}

func TestOneBotConnectionUsesAuthorizationHeader(t *testing.T) {
	authenticated := make(chan bool, 1)
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Skipf("sandbox does not allow a local test listener: %v", err)
	}
	server := httptest.NewUnstartedServer(websocket.Handler(func(connection *websocket.Conn) {
		authorized := connection.Request().Header.Get("Authorization") == "Bearer local-onebot-token"
		authenticated <- authorized
		if !authorized {
			_ = connection.Close()
			return
		}
		var raw []byte
		if websocket.Message.Receive(connection, &raw) != nil {
			return
		}
		var request map[string]any
		if json.Unmarshal(raw, &request) != nil {
			return
		}
		response, _ := json.Marshal(map[string]any{
			"status": "ok", "retcode": 0, "echo": request["echo"],
			"data": map[string]any{"user_id": "90000000000000000001", "nickname": "测试机器人"},
		})
		_ = websocket.Message.Send(connection, response)
	}))
	server.Listener = listener
	server.Start()
	defer server.Close()
	value := config.Default().QQBot
	value.Enabled = true
	value.OneBotWebSocketURL = "ws" + strings.TrimPrefix(server.URL, "http")
	value.OneBotToken = "local-onebot-token"
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	result, err := TestConnection(ctx, value)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Success || result.BotQQ != "90000000000000000001" || result.Nickname != "测试机器人" {
		t.Fatalf("unexpected connection test result: %#v", result)
	}
	if !<-authenticated {
		t.Fatal("OneBot Authorization header was not sent")
	}
}

func TestExpiredConfirmationDoesNotExecute(t *testing.T) {
	manager := &Manager{pendingActions: map[string]pendingAction{}}
	conversation := Conversation{Type: "private", UserID: "90000000000000000001"}
	manager.pendingActions[conversation.key()] = pendingAction{Code: "123456", Conversation: conversation, Kind: "restart", ExpiresAt: time.Now().Add(-time.Second)}
	if response := manager.confirmAction(conversation, "123456"); !strings.Contains(response, "过期") {
		t.Fatalf("expired confirmation response = %q", response)
	}
}

func TestConversationHistoryCapsExpiresAndStripsCodes(t *testing.T) {
	manager := &Manager{history: map[string]chatHistory{}}
	conversation := Conversation{Type: "group", GroupID: "90000000000000000002", UserID: "90000000000000000001"}
	manager.recordHistory(conversation, "第一据点有哪些帕鲁", "第一据点：3 只工作帕鲁")
	manager.recordHistory(conversation, "第二据点呢", "第二据点：2 只工作帕鲁")
	manager.recordHistory(conversation, "重启服务器", "即将重启 PalServer。\n如确认，请在 60 秒内回复：确认 123456")

	history := manager.conversationHistory(conversation)
	if len(history) != maxHistoryEntries {
		t.Fatalf("expected %d history entries, got %d", maxHistoryEntries, len(history))
	}
	joined := ""
	for _, entry := range history {
		joined += entry.Role + ":" + entry.Content + "\n"
	}
	if strings.Contains(joined, "123456") {
		t.Fatalf("confirmation code leaked into history: %q", joined)
	}
	if !strings.Contains(joined, "第二据点呢") || !strings.Contains(joined, "[需验证码确认的操作]") {
		t.Fatalf("recent turns should be kept: %q", joined)
	}
	if strings.Contains(joined, "第一据点有哪些帕鲁") {
		t.Fatalf("oldest turn should be dropped beyond the cap: %q", joined)
	}

	// 过期后历史应返回 nil。
	manager.mu.Lock()
	key := conversation.key()
	record := manager.history[key]
	record.UpdatedAt = time.Now().UTC().Add(-historyTTL - time.Second)
	manager.history[key] = record
	manager.mu.Unlock()
	if history := manager.conversationHistory(conversation); history != nil {
		t.Fatalf("expired history should return nil, got %#v", history)
	}
}

func TestConversationHistoryIsIsolatedPerUser(t *testing.T) {
	manager := &Manager{history: map[string]chatHistory{}}
	first := Conversation{Type: "group", GroupID: "90000000000000000002", UserID: "90000000000000000001"}
	second := Conversation{Type: "group", GroupID: "90000000000000000002", UserID: "90000000000000000003"}
	manager.recordHistory(first, "查一下张三", "张三：在线")
	if history := manager.conversationHistory(second); history != nil {
		t.Fatalf("another user in the same group must not see the first user's context: %#v", history)
	}
	if history := manager.conversationHistory(first); len(history) != 2 {
		t.Fatalf("own context should be available: %#v", history)
	}
}
