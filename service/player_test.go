package service

import (
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/zaigie/palworld-server-tool/internal/database"
	"go.etcd.io/bbolt"
)

func openPlayerTestDB(t *testing.T) *bbolt.DB {
	t.Helper()
	db, err := bbolt.Open(filepath.Join(t.TempDir(), "pst.db"), 0600, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Update(func(tx *bbolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists([]byte("players"))
		return err
	}); err != nil {
		db.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func storedPlayer(t *testing.T, db *bbolt.DB, uid string) database.Player {
	t.Helper()
	var player database.Player
	if err := db.View(func(tx *bbolt.Tx) error {
		return json.Unmarshal(tx.Bucket([]byte("players")).Get([]byte(uid)), &player)
	}); err != nil {
		t.Fatal(err)
	}
	return player
}

func TestPutPlayersOnlineTracksSessionAndTotalDuration(t *testing.T) {
	db := openPlayerTestDB(t)
	base := time.Date(2026, 7, 22, 4, 0, 0, 0, time.UTC)
	players := []database.OnlinePlayer{{PlayerUid: "1001", Nickname: "测试玩家"}}

	if err := putPlayersOnlineAt(db, players, base); err != nil {
		t.Fatal(err)
	}
	first := storedPlayer(t, db, "1001")
	if !first.IsOnline || !first.OnlineSince.Equal(base) || first.TotalOnlineSeconds != 0 {
		t.Fatalf("unexpected first session state: %+v", first.OnlinePlayer)
	}

	if err := putPlayersOnlineAt(db, players, base.Add(90*time.Second)); err != nil {
		t.Fatal(err)
	}
	second := storedPlayer(t, db, "1001")
	if second.CurrentSessionSeconds != 90 || second.TotalOnlineSeconds != 90 {
		t.Fatalf("expected 90 seconds, got session=%d total=%d", second.CurrentSessionSeconds, second.TotalOnlineSeconds)
	}

	if err := putPlayersOnlineAt(db, nil, base.Add(120*time.Second)); err != nil {
		t.Fatal(err)
	}
	offline := storedPlayer(t, db, "1001")
	if offline.IsOnline || !offline.OnlineSince.IsZero() || offline.TotalOnlineSeconds != 90 {
		t.Fatalf("unexpected offline state: %+v", offline.OnlinePlayer)
	}

	if err := putPlayersOnlineAt(db, players, base.Add(180*time.Second)); err != nil {
		t.Fatal(err)
	}
	reconnected := storedPlayer(t, db, "1001")
	if !reconnected.OnlineSince.Equal(base.Add(180*time.Second)) || reconnected.TotalOnlineSeconds != 90 {
		t.Fatalf("unexpected reconnected state: %+v", reconnected.OnlinePlayer)
	}
	events, err := ListPlayerPresenceEvents(db, "1001", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 3 || !events[0].Online || events[1].Online || !events[2].Online {
		t.Fatalf("unexpected player presence transitions: %#v", events)
	}
}

func TestPutPlayersPreservesOnlineDurationFields(t *testing.T) {
	db := openPlayerTestDB(t)
	base := time.Date(2026, 7, 22, 4, 0, 0, 0, time.UTC)
	if err := putPlayersOnlineAt(db, []database.OnlinePlayer{{PlayerUid: "1001", Nickname: "在线名称"}}, base); err != nil {
		t.Fatal(err)
	}
	savePlayer := database.Player{}
	savePlayer.PlayerUid = "1001"
	savePlayer.Nickname = "存档名称"
	if err := PutPlayers(db, []database.Player{savePlayer}); err != nil {
		t.Fatal(err)
	}
	player := storedPlayer(t, db, "1001")
	if !player.IsOnline || !player.OnlineSince.Equal(base) {
		t.Fatalf("save sync reset online duration fields: %+v", player.OnlinePlayer)
	}
}
