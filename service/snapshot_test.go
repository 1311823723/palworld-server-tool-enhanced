package service

import (
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/zaigie/palworld-server-tool/internal/database"
	"go.etcd.io/bbolt"
)

func openSnapshotTestDB(t *testing.T) *bbolt.DB {
	t.Helper()
	db, err := bbolt.Open(filepath.Join(t.TempDir(), "snapshot.db"), 0600, nil)
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	if err := db.Update(func(tx *bbolt.Tx) error {
		if _, err := tx.CreateBucketIfNotExists(snapshotsBucket); err != nil {
			return err
		}
		_, err := tx.CreateBucketIfNotExists(snapshotStateBucket)
		return err
	}); err != nil {
		db.Close()
		t.Fatalf("create snapshot buckets: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func snapshotFixture(baseID string, locations ...database.InventoryLocation) database.SnapshotPayload {
	return database.SnapshotPayload{
		Metadata: database.SnapshotMetadata{
			SnapshotTime: time.Now().UTC(),
			SaveFileTime: time.Now().UTC(),
			Capabilities: map[string]string{"inventory": "available"},
		},
		BaseCamps:      []database.BaseCampSnapshot{{BaseID: baseID, BaseName: baseID}},
		InventorySlots: locations,
	}
}

func inventoryLocation(container string, slot int, item string, count int64) database.InventoryLocation {
	return database.InventoryLocation{
		LocationID:    container + ":" + strconv.Itoa(slot),
		ContainerID:   container,
		ContainerType: "storage",
		SourceType:    "base_storage",
		BaseID:        "base-a",
		SlotIndex:     slot,
		ItemID:        item,
		ItemName:      item,
		Count:         count,
	}
}

func TestSnapshotSwitchIsAtomicAndOnlyActiveSnapshotIsRead(t *testing.T) {
	db := openSnapshotTestDB(t)
	first, err := PutSnapshot(db, snapshotFixture("base-a"))
	if err != nil {
		t.Fatalf("put first snapshot: %v", err)
	}
	second, err := PutSnapshot(db, snapshotFixture("base-b"))
	if err != nil {
		t.Fatalf("put second snapshot: %v", err)
	}
	if first.SnapshotID == second.SnapshotID {
		t.Fatal("snapshot IDs must be unique")
	}
	bases, metadata, err := ListBaseCamps(db)
	if err != nil {
		t.Fatalf("list active bases: %v", err)
	}
	if len(bases) != 1 || bases[0].BaseID != "base-b" {
		t.Fatalf("active bases = %#v, want only base-b", bases)
	}
	if metadata.SnapshotID != second.SnapshotID {
		t.Fatalf("active metadata = %q, want %q", metadata.SnapshotID, second.SnapshotID)
	}
}

func TestInventorySummaryDeduplicatesContainerSlotAndBalancesTotals(t *testing.T) {
	db := openSnapshotTestDB(t)
	baseStone := inventoryLocation("container-a", 0, "stone", 100)
	duplicate := baseStone
	playerStone := inventoryLocation("container-b", 1, "stone", 50)
	playerStone.SourceType = "player_inventory"
	playerStone.PlayerUID = "42"
	playerStone.BaseID = ""
	payload := snapshotFixture("base-a", baseStone, duplicate, playerStone)
	if _, err := PutSnapshot(db, payload); err != nil {
		t.Fatalf("put snapshot: %v", err)
	}
	page, err := InventorySummary(db, InventoryQuery{})
	if err != nil {
		t.Fatalf("inventory summary: %v", err)
	}
	if len(page.Items) != 1 {
		t.Fatalf("summary items = %d, want 1", len(page.Items))
	}
	item := page.Items[0]
	if item.TotalCount != 150 || item.BaseTotal != 100 || item.PlayerTotal != 50 {
		t.Fatalf("unexpected aggregate: %#v", item)
	}
	locations, _, total, err := InventoryLocations(db, "stone", InventoryQuery{})
	if err != nil {
		t.Fatalf("inventory locations: %v", err)
	}
	var sum int64
	for _, location := range locations {
		sum += location.Count
	}
	if total != 2 || sum != item.TotalCount {
		t.Fatalf("locations total=%d sum=%d, aggregate=%d", total, sum, item.TotalCount)
	}
}
