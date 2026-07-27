package service

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/zaigie/palworld-server-tool/internal/database"
	"go.etcd.io/bbolt"
)

func TestOperationAuditFiltersAndRedacts(t *testing.T) {
	db, err := bbolt.Open(filepath.Join(t.TempDir(), "pst.db"), 0600, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	now := time.Now().UTC()
	if err := AddOperationAudit(db, database.OperationAudit{
		Action: "POST /api/server/start", Status: "success", Detail: "token=secret", CreatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	if err := AddOperationAudit(db, database.OperationAudit{
		Action: "POST /api/server/stop", Status: "error", Detail: "HTTP 500", CreatedAt: now.Add(time.Second),
	}); err != nil {
		t.Fatal(err)
	}
	items, err := ListOperationAudits(db, 10, "", "success", time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Action != "POST /api/server/start" {
		t.Fatalf("unexpected audit filter result: %#v", items)
	}
	if strings.Contains(items[0].Detail, "secret") || items[0].Detail != "敏感详情已隐藏" {
		t.Fatalf("audit detail was not redacted: %q", items[0].Detail)
	}
}
