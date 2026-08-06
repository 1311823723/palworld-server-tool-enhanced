package qqbot

import (
	"strings"
	"testing"
)

func TestRenderQQText(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"bold", "**服务器状态**", "服务器状态"},
		{"inline bold", "当前在线 **6 人**", "当前在线 6 人"},
		{"underscore bold", "__服务器状态__", "服务器状态"},
		{"italic", "*注意*", "注意"},
		{"underscore italic", "_注意_", "注意"},
		{"heading", "## 在线玩家", "【在线玩家】"},
		{"inline code", "请检查 `PalServer.exe`", "请检查「PalServer.exe」"},
		{"plain url", "https://example.com/a_b?q=1", "https://example.com/a_b?q=1"},
		{"wildcard", "*.pak\nCapture*.pak", "*.pak\nCapture*.pak"},
		{"underscore names", "player_name REST_API_PORT", "player_name REST_API_PORT"},
		{"unclosed bold", "**服务器状态", "**服务器状态"},
		{"list", "- 玩家A\n- 玩家B", "• 玩家A\n• 玩家B"},
		{"blockquote", "> 服务器将在30秒后关闭", "服务器将在30秒后关闭"},
		{"emoji", "✅ 服务器状态正常", "✅ 服务器状态正常"},
		{"link", "[查看文档](https://example.com)", "查看文档：https://example.com"},
		{"same label and url", "[https://example.com](https://example.com)", "https://example.com"},
		{"image", "![地图](https://example.com/map.png)", "图片：地图\nhttps://example.com/map.png"},
		{"horizontal rule", "---", "────────"},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			if got := RenderQQText(testCase.input); got != testCase.want {
				t.Fatalf("RenderQQText(%q) = %q, want %q", testCase.input, got, testCase.want)
			}
		})
	}
}

func TestRenderQQTextCodeBlockKeepsContent(t *testing.T) {
	input := "```ini\nRESTAPIEnabled=True\nRESTAPIPort=8212\n```"
	got := RenderQQText(input)
	if !strings.Contains(got, "RESTAPIEnabled=True") || !strings.Contains(got, "RESTAPIPort=8212") {
		t.Fatalf("code block content lost: %q", got)
	}
	if strings.Contains(got, "```") {
		t.Fatalf("code fence leaked: %q", got)
	}
}

func TestRenderQQTextCodeBlockProtectsMarkdown(t *testing.T) {
	input := "```\n**keep**\n```"
	if got := RenderQQText(input); got != "**keep**" {
		t.Fatalf("code block content should stay untouched, got %q", got)
	}
}

func TestRenderQQTextTableBecomesPlainLines(t *testing.T) {
	input := "| 项目 | 状态 |\n| --- | --- |\n| 服务器 | 正常 |\n| 玩家 | 6/8 |"
	got := RenderQQText(input)
	if strings.Contains(got, "|") || strings.Contains(got, "---") {
		t.Fatalf("table syntax leaked: %q", got)
	}
	if !strings.Contains(got, "服务器：正常") || !strings.Contains(got, "玩家：6/8") {
		t.Fatalf("table rows missing: %q", got)
	}
}

func TestRenderQQTextCollapsesExcessiveBlankLines(t *testing.T) {
	input := "a\n\n\n\n\nb"
	got := RenderQQText(input)
	if strings.Count(got, "\n") > 4 {
		t.Fatalf("blank lines were not collapsed: %q", got)
	}
}

func TestRenderQQTextIsIdempotent(t *testing.T) {
	input := "**服务器状态**\n- 玩家A\n[查看文档](https://example.com)\n## 标题\n> 引用"
	once := RenderQQText(input)
	twice := RenderQQText(once)
	if once != twice {
		t.Fatalf("formatter is not idempotent:\nonce: %q\ntwice: %q", once, twice)
	}
}

func TestRenderQQTextMixedMessage(t *testing.T) {
	input := "**服务器状态**\n当前在线：6 / 8\n- CPU：42%\n- 内存：68%\n请检查 `PalServer.exe`。\n[打开管理页面](https://example.com)"
	got := RenderQQText(input)
	for _, marker := range []string{"**", "#", "`", "[", "]"} {
		if strings.Contains(got, marker) {
			t.Fatalf("markdown marker %q leaked: %q", marker, got)
		}
	}
	for _, expected := range []string{"服务器状态", "当前在线：6 / 8", "• CPU：42%", "请检查「PalServer.exe」。", "打开管理页面：https://example.com"} {
		if !strings.Contains(got, expected) {
			t.Fatalf("expected %q in %q", expected, got)
		}
	}
}

func TestFormatTextSegmentsOnlyFormatsText(t *testing.T) {
	segments := []jsonObject{
		{"type": "text", "data": jsonObject{"text": "**服务器状态**"}},
		{"type": "at", "data": jsonObject{"qq": "123456"}},
		{"type": "image", "data": jsonObject{"file": "map.png"}},
	}
	formatted := formatTextSegments(segments)
	if got := formatted[0]["data"].(jsonObject)["text"]; got != "服务器状态" {
		t.Fatalf("text segment was not formatted: %q", got)
	}
	if formatted[1]["type"] != "at" || formatted[1]["data"].(jsonObject)["qq"] != "123456" {
		t.Fatalf("at segment changed: %#v", formatted[1])
	}
	if formatted[2]["type"] != "image" || formatted[2]["data"].(jsonObject)["file"] != "map.png" {
		t.Fatalf("image segment changed: %#v", formatted[2])
	}
	if len(formatted) != len(segments) {
		t.Fatalf("segment count changed: %d != %d", len(formatted), len(segments))
	}
}
