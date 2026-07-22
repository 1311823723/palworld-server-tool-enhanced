package service

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/zaigie/palworld-server-tool/internal/database"
	"go.etcd.io/bbolt"
)

func TestBaseAliasPersistsAcrossDatabaseReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pst.db")
	open := func() *bbolt.DB {
		db, err := bbolt.Open(path, 0600, nil)
		if err != nil {
			t.Fatalf("open database: %v", err)
		}
		if err := db.Update(func(tx *bbolt.Tx) error {
			if _, err := tx.CreateBucketIfNotExists(snapshotsBucket); err != nil {
				return err
			}
			_, err := tx.CreateBucketIfNotExists(snapshotStateBucket)
			return err
		}); err != nil {
			_ = db.Close()
			t.Fatalf("initialize database: %v", err)
		}
		return db
	}

	db := open()
	if _, err := PutSnapshot(db, snapshotFixture("base-reopen")); err != nil {
		t.Fatalf("put snapshot: %v", err)
	}
	if _, err := SetBaseAlias(db, "base-reopen", "重启后仍保留", time.Now()); err != nil {
		t.Fatalf("set alias: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close database: %v", err)
	}

	db = open()
	t.Cleanup(func() { _ = db.Close() })
	base, _, err := GetBaseCamp(db, "base-reopen")
	if err != nil || base.CustomName != "重启后仍保留" || base.BaseName != "base-reopen" {
		t.Fatalf("reopened alias = %#v, %v", base, err)
	}
}

func TestBaseAliasPersistsAcrossSnapshotReplacement(t *testing.T) {
	db := openSnapshotTestDB(t)
	payload := snapshotFixture("base-123456")
	payload.BaseCamps[0].BaseName = "新規生成拠点テンプレート名2(仮)"
	if _, err := PutSnapshot(db, payload); err != nil {
		t.Fatalf("put snapshot: %v", err)
	}
	if _, err := SetBaseAlias(db, "base-123456", "  北境生产基地  ", time.Now()); err != nil {
		t.Fatalf("set alias: %v", err)
	}

	base, _, err := GetBaseCamp(db, "base-123456")
	if err != nil {
		t.Fatalf("get base: %v", err)
	}
	if base.BaseName != "新規生成拠点テンプレート名2(仮)" || base.CustomName != "北境生产基地" || base.DisplayName != "北境生产基地" {
		t.Fatalf("unexpected decorated base: %#v", base)
	}

	payload.Metadata.SnapshotTime = time.Now().Add(time.Minute)
	if _, err := PutSnapshot(db, payload); err != nil {
		t.Fatalf("replace snapshot: %v", err)
	}
	base, _, err = GetBaseCamp(db, "base-123456")
	if err != nil || base.DisplayName != "北境生产基地" {
		t.Fatalf("alias did not survive snapshot replacement: %#v, %v", base, err)
	}
}

func TestBaseAliasValidationConflictAndReset(t *testing.T) {
	db := openSnapshotTestDB(t)
	payload := snapshotFixture("base-a")
	payload.BaseCamps = []database.BaseCampSnapshot{
		{BaseID: "base-a", BaseName: "第一据点"},
		{BaseID: "base-b", BaseName: "第二据点"},
	}
	if _, err := PutSnapshot(db, payload); err != nil {
		t.Fatalf("put snapshot: %v", err)
	}
	if _, err := SetBaseAlias(db, "base-a", "第二据点", time.Now()); !errors.Is(err, ErrBaseAliasConflict) {
		t.Fatalf("duplicate display name error = %v, want conflict", err)
	}
	for _, value := range []string{"", "换行\n名称", strings.Repeat("据", 41)} {
		if _, err := SetBaseAlias(db, "base-a", value, time.Now()); !errors.Is(err, ErrInvalidBaseAlias) {
			t.Fatalf("invalid alias %q error = %v", value, err)
		}
	}
	if _, err := SetBaseAlias(db, "base-a", "生产区", time.Now()); err != nil {
		t.Fatalf("set alias: %v", err)
	}
	if err := DeleteBaseAlias(db, "base-a"); err != nil {
		t.Fatalf("delete alias: %v", err)
	}
	base, _, err := GetBaseCamp(db, "base-a")
	if err != nil || base.CustomName != "" || base.DisplayName != "第一据点" {
		t.Fatalf("reset did not restore raw display name: %#v, %v", base, err)
	}
}

func TestBaseAliasOrphanRemainsListed(t *testing.T) {
	db := openSnapshotTestDB(t)
	if _, err := PutSnapshot(db, snapshotFixture("base-old")); err != nil {
		t.Fatalf("put old snapshot: %v", err)
	}
	if _, err := SetBaseAlias(db, "base-old", "旧世界据点", time.Now()); err != nil {
		t.Fatalf("set alias: %v", err)
	}
	if _, err := PutSnapshot(db, snapshotFixture("base-new")); err != nil {
		t.Fatalf("put new snapshot: %v", err)
	}
	aliases, err := ListBaseAliases(db)
	if err != nil {
		t.Fatalf("list aliases: %v", err)
	}
	if len(aliases) != 1 || aliases[0].Active || aliases[0].DisplayName != "旧世界据点" {
		t.Fatalf("unexpected orphan alias: %#v", aliases)
	}
}

func TestBaseAliasDecoratesDependentSnapshotModels(t *testing.T) {
	db := openSnapshotTestDB(t)
	payload := snapshotFixture("base-a", inventoryLocation("container-a", 0, "stone", 10))
	payload.BaseCamps[0].BaseName = "原始据点"
	payload.WorkPals = []database.BaseWorkerPal{{InstanceID: "pal-a", BaseID: "base-a", BaseName: "原始据点"}}
	payload.Containers = []database.ItemContainer{{ContainerID: "container-a", BaseID: "base-a", BaseName: "原始据点"}}
	payload.InventorySlots[0].BaseName = "原始据点"
	payload.BreedingFarms = []database.BreedingFarmSnapshot{{FarmID: "farm-a", BaseID: "base-a", BaseName: "原始据点"}}
	if _, err := PutSnapshot(db, payload); err != nil {
		t.Fatalf("put snapshot: %v", err)
	}
	if _, err := SetBaseAlias(db, "base-a", "矿业基地", time.Now()); err != nil {
		t.Fatalf("set alias: %v", err)
	}
	workers, _, err := ListBaseWorkers(db, "base-a")
	if err != nil || len(workers) != 1 || workers[0].BaseDisplayName != "矿业基地" {
		t.Fatalf("worker alias missing: %#v, %v", workers, err)
	}
	containers, _, err := ListContainers(db, InventoryQuery{BaseID: "base-a"})
	if err != nil || len(containers) != 1 || containers[0].BaseDisplayName != "矿业基地" {
		t.Fatalf("container alias missing: %#v, %v", containers, err)
	}
	locations, _, _, err := InventoryLocations(db, "stone", InventoryQuery{})
	if err != nil || len(locations) != 1 || locations[0].BaseDisplayName != "矿业基地" {
		t.Fatalf("inventory alias missing: %#v, %v", locations, err)
	}
	farms, err := ListBreedingFarms(db, BreedingFarmQuery{Page: 1, PageSize: 20})
	if err != nil || len(farms.Items) != 1 || farms.Items[0].BaseDisplayName != "矿业基地" {
		t.Fatalf("breeding farm alias missing: %#v, %v", farms.Items, err)
	}
}

func TestBreedingEventAndGameNotificationUseCurrentBaseAlias(t *testing.T) {
	db := openSnapshotTestDB(t)
	now := time.Now().UTC()
	options := monitorAll()
	initial := breedingPayload(now, map[string][]string{"farm-a": {}})
	initial.BaseCamps = []database.BaseCampSnapshot{{BaseID: "base-farm-a", BaseName: "存档原名"}}
	if _, _, err := PutSnapshotWithBreedingMonitorEvents(db, initial, &options, now); err != nil {
		t.Fatalf("put initial breeding snapshot: %v", err)
	}
	if _, err := SetBaseAlias(db, "base-farm-a", "配种一号基地", now); err != nil {
		t.Fatalf("set breeding alias: %v", err)
	}
	next := breedingPayload(now.Add(time.Minute), map[string][]string{"farm-a": {"egg-a"}})
	next.BaseCamps = initial.BaseCamps
	_, events, err := PutSnapshotWithBreedingMonitorEvents(db, next, &options, now.Add(time.Minute))
	if err != nil || len(events) != 1 || events[0].BaseDisplayName != "配种一号基地" || events[0].BaseName == "配种一号基地" {
		t.Fatalf("breeding event alias = %#v, %v", events, err)
	}
	notifications := BuildBreedingGameNotifications(events, "【配种提醒】{base} 新增 {new_count} 枚蛋")
	if len(notifications) != 1 || notifications[0].Message != "【配种提醒】配种一号基地 新增 1 枚蛋" {
		t.Fatalf("game notification alias = %#v", notifications)
	}
}

func TestBaseDisplayNameHidesGamePlaceholder(t *testing.T) {
	name := BaseDisplayName("04B8CEFF45C92ADA6DEB458AF87A9F99", "新規生成拠点テンプレート名2(仮)", "")
	if name != "未命名据点（7A9F99）" {
		t.Fatalf("fallback name = %q", name)
	}
}
