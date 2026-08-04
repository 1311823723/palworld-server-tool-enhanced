package qqbot

import (
	"context"
	"encoding/json"
	"net"
	"net/http/httptest"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/zaigie/palworld-server-tool/internal/config"
	"github.com/zaigie/palworld-server-tool/internal/supervisor"
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
