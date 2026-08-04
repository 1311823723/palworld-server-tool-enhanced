package service

import (
	"testing"
	"time"

	"github.com/zaigie/palworld-server-tool/internal/database"
	"go.etcd.io/bbolt"
)

func TestGuildBasesUseNumberedDefaultsAndCustomAliases(t *testing.T) {
	db := openSnapshotTestDB(t)
	if err := db.Update(func(tx *bbolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists([]byte("guilds"))
		return err
	}); err != nil {
		t.Fatal(err)
	}
	payload := snapshotFixture("base-a")
	payload.BaseCamps = append(payload.BaseCamps, database.BaseCampSnapshot{BaseID: "base-b", BaseName: "新規生成拠点テンプレート名2(仮)"})
	if _, err := PutSnapshot(db, payload); err != nil {
		t.Fatal(err)
	}
	guild := database.Guild{
		Name: "测试公会", AdminPlayerUid: "admin-a",
		Players:  []*database.GuildPlayer{{PlayerUid: "admin-a", Nickname: "服主"}},
		BaseCamp: []database.BaseCamp{{Id: "base-a"}, {Id: "base-b"}},
	}
	if err := PutGuilds(db, []database.Guild{guild}); err != nil {
		t.Fatal(err)
	}

	guilds, err := ListGuilds(db)
	if err != nil || len(guilds) != 1 {
		t.Fatalf("list guilds = %#v, err=%v", guilds, err)
	}
	if guilds[0].BaseCamp[0].DisplayName != "据点 1" || guilds[0].BaseCamp[1].DisplayName != "据点 2" {
		t.Fatalf("numbered defaults = %#v", guilds[0].BaseCamp)
	}
	if _, err := SetBaseAlias(db, "base-b", "北境矿场", time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	detail, err := GetGuild(db, "admin-a")
	if err != nil {
		t.Fatal(err)
	}
	if detail.BaseCamp[1].DisplayName != "北境矿场" || detail.BaseCamp[1].CustomName != "北境矿场" {
		t.Fatalf("custom guild base name = %#v", detail.BaseCamp[1])
	}
}
