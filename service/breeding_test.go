package service

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/zaigie/palworld-server-tool/internal/database"
	"go.etcd.io/bbolt"
)

func breedingInt(value int64) *int64 { return &value }

func breedingPayload(saveTime time.Time, farms map[string][]string) database.SnapshotPayload {
	payload := database.SnapshotPayload{
		Metadata: database.SnapshotMetadata{SaveFileTime: saveTime, SnapshotTime: saveTime},
		BreedingCapabilities: database.BreedingFarmCapabilities{
			FarmDetection: true, BaseAssociation: true, ParentSlots: true, CakeContainer: true,
			EggDetection: true, EggIdentity: true, ValidatedGameVersion: "synthetic-current-version",
		},
	}
	for farmID, eggs := range farms {
		count := int64(len(eggs))
		payload.BreedingFarms = append(payload.BreedingFarms, database.BreedingFarmSnapshot{
			FarmID: farmID, BaseID: "base-" + farmID, BaseName: "Base " + farmID,
			GuildID: "guild-a", EggCount: breedingInt(count), Confidence: "high",
			AssociationVerified: true, ParsingComplete: true, GameVersionSupported: true, IdentitySupported: true,
		})
		for _, eggID := range eggs {
			payload.BreedingEggs = append(payload.BreedingEggs, database.BreedingFarmEgg{
				FarmID: farmID, EggInstanceID: eggID, Count: 1, Ready: true, AssociationVerified: true,
			})
		}
	}
	return payload
}

func monitorAll() BreedingMonitorOptions {
	return BreedingMonitorOptions{
		Enabled: true, SelectionMode: "all", NotifyOnEachEgg: true,
		MinimumReadyEggs: 1, HistoryRetentionDays: 30,
	}
}

func putBreeding(t *testing.T, db *bbolt.DB, saveTime, now time.Time, farms map[string][]string, options BreedingMonitorOptions) {
	t.Helper()
	if _, err := PutSnapshotWithBreedingMonitor(db, breedingPayload(saveTime, farms), &options, now); err != nil {
		t.Fatalf("put breeding snapshot: %v", err)
	}
}

func breedingEvents(t *testing.T, db *bbolt.DB) []database.BreedingFarmEvent {
	t.Helper()
	page, err := ListBreedingEvents(db, BreedingEventQuery{Page: 1, PageSize: 200})
	if err != nil {
		t.Fatalf("list breeding events: %v", err)
	}
	return page.Items
}

func TestBreedingMonitorCreatesOnlyNewEggEvents(t *testing.T) {
	db := openSnapshotTestDB(t)
	start := time.Date(2026, 7, 22, 4, 0, 0, 0, time.UTC)
	options := monitorAll()
	putBreeding(t, db, start, start, map[string][]string{"farm-a": {}}, options)
	putBreeding(t, db, start.Add(time.Minute), start.Add(time.Minute), map[string][]string{"farm-a": {"egg-1"}}, options)
	putBreeding(t, db, start.Add(2*time.Minute), start.Add(2*time.Minute), map[string][]string{"farm-a": {"egg-1", "egg-2"}}, options)
	putBreeding(t, db, start.Add(3*time.Minute), start.Add(3*time.Minute), map[string][]string{"farm-a": {}}, options)
	events := breedingEvents(t, db)
	if len(events) != 2 {
		t.Fatalf("events = %d, want 2", len(events))
	}
	if events[0].PreviousCount != 1 || events[0].CurrentCount != 2 || events[1].PreviousCount != 0 || events[1].CurrentCount != 1 {
		t.Fatalf("unexpected transitions: %#v", events)
	}
}

func TestBuildBreedingGameNotificationsGroupsEventsByFarm(t *testing.T) {
	events := []database.BreedingFarmEvent{
		{EventID: "event-2", FarmID: "farm-b", BaseName: "第二据点", EventType: "egg_ready", PreviousCount: 0, CurrentCount: 1},
		{EventID: "event-1", FarmID: "farm-a", BaseName: "存档原名", BaseDisplayName: "第一\n据点", EventType: "egg_ready", PreviousCount: 1, CurrentCount: 3},
		{EventID: "event-3", FarmID: "farm-a", BaseName: "存档原名", BaseDisplayName: "第一\n据点", EventType: "egg_ready", PreviousCount: 1, CurrentCount: 3},
	}
	notifications := BuildBreedingGameNotifications(events, "【配种提醒】{base} 新增 {new_count} 枚，现有 {count} 枚")
	if len(notifications) != 2 {
		t.Fatalf("notifications = %d, want 2", len(notifications))
	}
	if notifications[0].FarmID != "farm-a" || notifications[0].Message != "【配种提醒】第一 据点 新增 2 枚，现有 3 枚" || len(notifications[0].EventIDs) != 2 {
		t.Fatalf("farm-a notification = %#v", notifications[0])
	}
	if notifications[1].Message != "【配种提醒】第二据点 新增 1 枚，现有 1 枚" {
		t.Fatalf("farm-b notification = %#v", notifications[1])
	}
}

func TestBuildBreedingGameNotificationsCountsStableIdentityReplacement(t *testing.T) {
	events := []database.BreedingFarmEvent{{EventID: "event-1", FarmID: "farm-a", EventType: "egg_ready", PreviousCount: 1, CurrentCount: 1}}
	notifications := BuildBreedingGameNotifications(events, "新增 {new_count} 枚")
	if len(notifications) != 1 || notifications[0].Message != "新增 1 枚" {
		t.Fatalf("replacement notification = %#v", notifications)
	}
}

func TestBreedingMonitorStableIdentityDetectsReplacementAtSameCount(t *testing.T) {
	db := openSnapshotTestDB(t)
	now := time.Now().UTC()
	options := monitorAll()
	putBreeding(t, db, now, now, map[string][]string{"farm-a": {"egg-old"}}, options)
	putBreeding(t, db, now.Add(time.Minute), now.Add(time.Minute), map[string][]string{"farm-a": {"egg-new"}}, options)
	events := breedingEvents(t, db)
	if len(events) != 1 || events[0].EggInstanceID != "egg-new" || events[0].PreviousCount != 1 || events[0].CurrentCount != 1 {
		t.Fatalf("replacement events = %#v", events)
	}
}

func TestBreedingMonitorWithoutIdentityIgnoresAmbiguousEqualCountChange(t *testing.T) {
	db := openSnapshotTestDB(t)
	now := time.Now().UTC()
	options := monitorAll()
	putTyped := func(saveTime, observedAt time.Time, itemID string, count int64) {
		payload := breedingPayload(saveTime, map[string][]string{"farm-a": {}})
		payload.BreedingCapabilities.EggIdentity = false
		payload.BreedingFarms[0].IdentitySupported = false
		payload.BreedingFarms[0].EggCount = breedingInt(count)
		payload.BreedingEggs = nil
		if count > 0 {
			payload.BreedingEggs = []database.BreedingFarmEgg{{FarmID: "farm-a", EggItemID: &itemID, Count: count, Ready: true, AssociationVerified: true}}
		}
		if _, err := PutSnapshotWithBreedingMonitor(db, payload, &options, observedAt); err != nil {
			t.Fatalf("put typed egg snapshot: %v", err)
		}
	}
	putTyped(now, now, "egg-small", 1)
	putTyped(now.Add(time.Minute), now.Add(time.Minute), "egg-large", 1)
	if got := len(breedingEvents(t, db)); got != 0 {
		t.Fatalf("ambiguous equal-count change emitted %d events", got)
	}
	putTyped(now.Add(2*time.Minute), now.Add(2*time.Minute), "egg-large", 2)
	if got := len(breedingEvents(t, db)); got != 1 {
		t.Fatalf("typed count increase events = %d, want 1", got)
	}
}

func TestBreedingMonitorCreatesSeparateEventsForMultipleFarms(t *testing.T) {
	db := openSnapshotTestDB(t)
	now := time.Now().UTC()
	options := monitorAll()
	putBreeding(t, db, now, now, map[string][]string{"farm-a": {}, "farm-b": {}}, options)
	putBreeding(t, db, now.Add(time.Minute), now.Add(time.Minute), map[string][]string{"farm-a": {"egg-a"}, "farm-b": {"egg-b"}}, options)
	events := breedingEvents(t, db)
	if len(events) != 2 || events[0].FarmID == events[1].FarmID {
		t.Fatalf("multiple farm events = %#v", events)
	}
}

func TestBreedingMonitorDefaultEnableBaselinesExistingEggs(t *testing.T) {
	db := openSnapshotTestDB(t)
	now := time.Now().UTC()
	options := monitorAll()
	putBreeding(t, db, now, now, map[string][]string{"farm-a": {"egg-existing"}}, options)
	if got := len(breedingEvents(t, db)); got != 0 {
		t.Fatalf("default enable emitted %d existing-egg events", got)
	}
	putBreeding(t, db, now.Add(time.Minute), now.Add(time.Minute), map[string][]string{"farm-a": {"egg-existing", "egg-new"}}, options)
	if got := len(breedingEvents(t, db)); got != 1 {
		t.Fatalf("new egg events = %d, want 1", got)
	}
}

func TestBreedingMonitorCanNotifyExistingOnce(t *testing.T) {
	db := openSnapshotTestDB(t)
	now := time.Now().UTC()
	options := monitorAll()
	options.NotifyExistingOnEnable = true
	putBreeding(t, db, now, now, map[string][]string{"farm-a": {"egg-existing"}}, options)
	putBreeding(t, db, now, now.Add(time.Second), map[string][]string{"farm-a": {"egg-existing"}}, options)
	if got := len(breedingEvents(t, db)); got != 1 {
		t.Fatalf("existing events = %d, want exactly 1", got)
	}
}

func TestBreedingMonitorHonorsSelectionAndDisabledState(t *testing.T) {
	db := openSnapshotTestDB(t)
	now := time.Now().UTC()
	options := monitorAll()
	options.SelectionMode = "selected"
	options.SelectedFarmIDs = []string{"farm-a"}
	putBreeding(t, db, now, now, map[string][]string{"farm-a": {}, "farm-b": {}}, options)
	putBreeding(t, db, now.Add(time.Minute), now.Add(time.Minute), map[string][]string{"farm-a": {"egg-a"}, "farm-b": {"egg-b"}}, options)
	events := breedingEvents(t, db)
	if len(events) != 1 || events[0].FarmID != "farm-a" {
		t.Fatalf("selected events = %#v", events)
	}
	disabled := options
	disabled.Enabled = false
	putBreeding(t, db, now.Add(2*time.Minute), now.Add(2*time.Minute), map[string][]string{"farm-a": {"egg-a", "egg-c"}}, disabled)
	if got := len(breedingEvents(t, db)); got != 1 {
		t.Fatalf("disabled monitor emitted events: %d", got)
	}
}

func TestBreedingMonitorIgnoresOldAndUnreliableSnapshots(t *testing.T) {
	db := openSnapshotTestDB(t)
	now := time.Now().UTC()
	options := monitorAll()
	putBreeding(t, db, now, now, map[string][]string{"farm-a": {}}, options)
	old := breedingPayload(now.Add(-time.Minute), map[string][]string{"farm-a": {"egg-old"}})
	if _, err := PutSnapshotWithBreedingMonitor(db, old, &options, now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	unreliable := breedingPayload(now.Add(time.Minute), map[string][]string{"farm-a": {"egg-unverified"}})
	unreliable.BreedingFarms[0].Confidence = "low"
	if _, err := PutSnapshotWithBreedingMonitor(db, unreliable, &options, now.Add(2*time.Minute)); err != nil {
		t.Fatal(err)
	}
	putBreeding(t, db, now.Add(2*time.Minute), now.Add(3*time.Minute), map[string][]string{"farm-a": {"egg-unverified", "egg-new"}}, options)
	if got := len(breedingEvents(t, db)); got != 0 {
		t.Fatalf("recovery snapshot emitted %d events, want baseline", got)
	}
}

func TestBreedingMonitorParserFailureForcesRecoveryBaseline(t *testing.T) {
	db := openSnapshotTestDB(t)
	now := time.Now().UTC()
	options := monitorAll()
	putBreeding(t, db, now, now, map[string][]string{"farm-a": {}}, options)
	if err := MarkBreedingParserFailed(db, now.Add(time.Minute)); err != nil {
		t.Fatalf("mark parser failed: %v", err)
	}
	putBreeding(t, db, now.Add(2*time.Minute), now.Add(2*time.Minute), map[string][]string{"farm-a": {"egg-during-failure"}}, options)
	if got := len(breedingEvents(t, db)); got != 0 {
		t.Fatalf("parser recovery emitted %d events, want baseline", got)
	}
	page, err := ListBreedingFarms(db, BreedingFarmQuery{Page: 1, PageSize: 20})
	if err != nil {
		t.Fatal(err)
	}
	if page.ParserStatus.Failed {
		t.Fatal("successful recovery snapshot did not clear parser failure status")
	}
	putBreeding(t, db, now.Add(3*time.Minute), now.Add(3*time.Minute), map[string][]string{"farm-a": {"egg-during-failure", "egg-new"}}, options)
	if got := len(breedingEvents(t, db)); got != 1 {
		t.Fatalf("post-recovery new egg events = %d, want 1", got)
	}
}

func TestBreedingMonitorStateAndDedupSurviveDatabaseReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "breeding-restart.db")
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
			db.Close()
			t.Fatalf("initialize database: %v", err)
		}
		return db
	}
	now := time.Now().UTC()
	options := monitorAll()
	db := open()
	putBreeding(t, db, now, now, map[string][]string{"farm-a": {}}, options)
	putBreeding(t, db, now.Add(time.Minute), now.Add(time.Minute), map[string][]string{"farm-a": {"egg-1"}}, options)
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db = open()
	defer db.Close()
	putBreeding(t, db, now.Add(time.Minute), now.Add(2*time.Minute), map[string][]string{"farm-a": {"egg-1"}}, options)
	if got := len(breedingEvents(t, db)); got != 1 {
		t.Fatalf("events after reopen = %d, want 1", got)
	}
}

func TestBreedingEventCanBeMarkedReadAndRemainsInHistory(t *testing.T) {
	db := openSnapshotTestDB(t)
	now := time.Now().UTC()
	options := monitorAll()
	putBreeding(t, db, now, now, map[string][]string{"farm-a": {}}, options)
	putBreeding(t, db, now.Add(time.Minute), now.Add(time.Minute), map[string][]string{"farm-a": {"egg-1"}}, options)
	event := breedingEvents(t, db)[0]
	if err := MarkBreedingEventRead(db, event.EventID); err != nil {
		t.Fatalf("mark read: %v", err)
	}
	unread := true
	page, err := ListBreedingEvents(db, BreedingEventQuery{Unread: &unread, Page: 1, PageSize: 20})
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 0 {
		t.Fatalf("unread events = %d, want 0", page.Total)
	}
	history := breedingEvents(t, db)
	if len(history) != 1 || !history[0].Read {
		t.Fatalf("history after read = %#v", history)
	}
}

func TestBreedingFarmDetailIsNotLimitedByListPagination(t *testing.T) {
	db := openSnapshotTestDB(t)
	farms := make(map[string][]string)
	for index := 0; index < 250; index++ {
		farms["farm-"+time.Unix(int64(index), 0).UTC().Format("150405")] = nil
	}
	now := time.Now().UTC()
	putBreeding(t, db, now, now, farms, monitorAll())
	for farmID := range farms {
		if _, _, err := GetBreedingFarm(db, farmID); err != nil {
			t.Fatalf("get farm %q: %v", farmID, err)
		}
	}
}
