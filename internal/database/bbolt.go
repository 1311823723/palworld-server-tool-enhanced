package database

import (
	"sync"
	"time"

	"github.com/zaigie/palworld-server-tool/internal/logger"
	"go.etcd.io/bbolt"
)

var db *bbolt.DB
var once sync.Once

func InitDB() *bbolt.DB {
	db_, err := bbolt.Open("pst.db", 0600, &bbolt.Options{Timeout: 1 * time.Minute})
	if err != nil {
		logger.Panic(err)
	}
	// players
	err = db_.Update(func(tx *bbolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists([]byte("players"))
		return err
	})
	if err != nil {
		logger.Panic(err)
	}
	// guilds
	err = db_.Update(func(tx *bbolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists([]byte("guilds"))
		return err
	})
	if err != nil {
		logger.Panic(err)
	}
	// rcons
	err = db_.Update(func(tx *bbolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists([]byte("rcons"))
		return err
	})
	if err != nil {
		logger.Panic(err)
	}
	// backups
	err = db_.Update(func(tx *bbolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists([]byte("backups"))
		return err
	})
	if err != nil {
		logger.Panic(err)
	}
	// scheduled rcon tasks
	err = db_.Update(func(tx *bbolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists([]byte("rcon_tasks"))
		return err
	})
	if err != nil {
		logger.Panic(err)
	}
	// normalized save snapshots; each snapshot is a nested bucket and the active
	// pointer is switched only after all indexes have been written.
	err = db_.Update(func(tx *bbolt.Tx) error {
		if _, err := tx.CreateBucketIfNotExists([]byte("save_snapshots")); err != nil {
			return err
		}
		_, err := tx.CreateBucketIfNotExists([]byte("save_snapshot_state"))
		return err
	})
	if err != nil {
		logger.Panic(err)
	}
	// World settings audit records contain only action metadata and changed key
	// names; secret values are never persisted here.
	err = db_.Update(func(tx *bbolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists([]byte("world_settings_audit"))
		return err
	})
	if err != nil {
		logger.Panic(err)
	}
	// Breeding notifications and baselines are persistent so re-parsing the
	// same save after a PST restart cannot produce duplicate egg alerts.
	err = db_.Update(func(tx *bbolt.Tx) error {
		for _, name := range [][]byte{[]byte("breeding_events"), []byte("breeding_event_dedup"), []byte("breeding_monitor_state"), []byte("breeding_events_by_farm"), []byte("breeding_events_by_base"), []byte("breeding_events_by_type"), []byte("breeding_events_by_read")} {
			if _, err := tx.CreateBucketIfNotExists(name); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		logger.Panic(err)
	}
	return db_
}

func GetDB() *bbolt.DB {
	once.Do(func() {
		db = InitDB()
	})
	return db
}
