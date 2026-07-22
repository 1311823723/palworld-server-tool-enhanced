package service

import (
	"encoding/json"

	"github.com/zaigie/palworld-server-tool/internal/worldsettings"
	"go.etcd.io/bbolt"
)

var worldSettingsAuditBucket = []byte("world_settings_audit")

type WorldSettingsAuditStore struct{ db *bbolt.DB }

func NewWorldSettingsAuditStore(db *bbolt.DB) *WorldSettingsAuditStore {
	return &WorldSettingsAuditStore{db: db}
}

func (store *WorldSettingsAuditStore) RecordWorldSettingsAudit(record worldsettings.WorldSettingsAudit) error {
	data, err := json.Marshal(record)
	if err != nil {
		return err
	}
	return store.db.Update(func(tx *bbolt.Tx) error {
		bucket, err := tx.CreateBucketIfNotExists(worldSettingsAuditBucket)
		if err != nil {
			return err
		}
		key := record.CreatedAt.UTC().Format("20060102T150405.000000000Z") + "-" + record.ID
		return bucket.Put([]byte(key), data)
	})
}
