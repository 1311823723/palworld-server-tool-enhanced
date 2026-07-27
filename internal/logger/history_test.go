package logger

import (
	"strings"
	"testing"
)

func TestHistoryRedactsSensitiveValues(t *testing.T) {
	before := historyNextID
	Info("Authorization: Bearer secret-token password=admin-secret")
	items := List(before, 10, "info")
	if len(items) != 1 {
		t.Fatalf("history entries = %d, want 1", len(items))
	}
	if strings.Contains(items[0].Message, "secret-token") || strings.Contains(items[0].Message, "admin-secret") {
		t.Fatalf("sensitive value leaked into history: %q", items[0].Message)
	}
	if !strings.Contains(items[0].Message, "[已隐藏]") {
		t.Fatalf("redaction marker missing: %q", items[0].Message)
	}
}
