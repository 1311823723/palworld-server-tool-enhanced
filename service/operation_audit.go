package service

import (
	"encoding/json"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/zaigie/palworld-server-tool/internal/database"
	"go.etcd.io/bbolt"
)

var operationsAuditBucket = []byte("operations_audit")

const maxOperationAuditRecords = 5000

func AddOperationAudit(db *bbolt.DB, record database.OperationAudit) error {
	if record.ID == "" {
		record.ID = uuid.NewString()
	}
	if record.CreatedAt.IsZero() {
		record.CreatedAt = time.Now().UTC()
	}
	record.Action = strings.TrimSpace(record.Action)
	record.Detail = sanitizeAuditDetail(record.Detail)
	data, err := json.Marshal(record)
	if err != nil {
		return err
	}
	return db.Update(func(tx *bbolt.Tx) error {
		bucket, err := tx.CreateBucketIfNotExists(operationsAuditBucket)
		if err != nil {
			return err
		}
		key := []byte(record.CreatedAt.Format("20060102T150405.000000000Z") + "-" + record.ID)
		if err := bucket.Put(key, data); err != nil {
			return err
		}
		for bucket.Stats().KeyN > maxOperationAuditRecords {
			cursor := bucket.Cursor()
			oldest, _ := cursor.First()
			if oldest == nil {
				break
			}
			if err := bucket.Delete(oldest); err != nil {
				return err
			}
		}
		return nil
	})
}

func ListOperationAudits(db *bbolt.DB, limit int, action, status string, since time.Time) ([]database.OperationAudit, error) {
	if limit < 1 {
		limit = 100
	}
	if limit > 500 {
		limit = 500
	}
	action = strings.TrimSpace(action)
	status = strings.TrimSpace(status)
	items := make([]database.OperationAudit, 0, limit)
	err := db.View(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket(operationsAuditBucket)
		if bucket == nil {
			return nil
		}
		return bucket.ForEach(func(_, value []byte) error {
			var item database.OperationAudit
			if err := json.Unmarshal(value, &item); err != nil {
				return err
			}
			if action != "" && item.Action != action {
				return nil
			}
			if status != "" && item.Status != status {
				return nil
			}
			if !since.IsZero() && item.CreatedAt.Before(since) {
				return nil
			}
			items = append(items, item)
			return nil
		})
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(items, func(i, j int) bool { return items[i].CreatedAt.After(items[j].CreatedAt) })
	if len(items) > limit {
		items = items[:limit]
	}
	return items, nil
}

func sanitizeAuditDetail(value string) string {
	value = strings.ReplaceAll(value, "\r", " ")
	value = strings.ReplaceAll(value, "\n", " ")
	if len(value) > 300 {
		value = value[:300]
	}
	lower := strings.ToLower(value)
	for _, sensitive := range []string{"password", "authorization", "bearer ", "jwt", "token"} {
		if strings.Contains(lower, sensitive) {
			return "敏感详情已隐藏"
		}
	}
	return strings.TrimSpace(value)
}
