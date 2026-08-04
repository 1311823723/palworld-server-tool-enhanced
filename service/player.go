package service

import (
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/zaigie/palworld-server-tool/internal/database"
	"go.etcd.io/bbolt"
)

func PutPlayers(db *bbolt.DB, players []database.Player) error {
	return db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte("players"))

		// get existing players
		existingPlayers := make(map[string]database.Player)
		err := b.ForEach(func(k, v []byte) error {
			var player database.Player
			if err := json.Unmarshal(v, &player); err != nil {
				return err
			}
			uid := player.PlayerUid
			if uid == "" {
				uid = string(k)
				player.PlayerUid = uid
			}
			existingPlayers[uid] = player
			return nil
		})
		if err != nil {
			return err
		}

		// build new players map
		newPlayers := make(map[string]database.Player)
		for _, p := range players {
			if p.PlayerUid == "" {
				return errors.New("player_uid is required")
			}
			newPlayers[p.PlayerUid] = p
		}

		// process new and existing players
		for _, p := range players {
			existingPlayer, exists := existingPlayers[p.PlayerUid]

			if exists {
				if p.SteamId == "" {
					p.SteamId = existingPlayer.SteamId
				}
				p.Ip = existingPlayer.Ip
				p.Ping = existingPlayer.Ping
				p.LocationX = existingPlayer.LocationX
				p.LocationY = existingPlayer.LocationY
				p.IsOnline = existingPlayer.IsOnline
				p.OnlineSince = existingPlayer.OnlineSince
				p.OnlineLastSeenAt = existingPlayer.OnlineLastSeenAt
				p.CurrentSessionSeconds = existingPlayer.CurrentSessionSeconds
				p.TotalOnlineSeconds = existingPlayer.TotalOnlineSeconds
			}

			if p.SaveLastOnline != "" {
				if parsedTime, err := time.Parse(time.RFC3339, p.SaveLastOnline); err == nil {
					p.LastOnline = parsedTime
				}
			}

			v, err := json.Marshal(p)
			if err != nil {
				return err
			}
			if err := b.Put([]byte(p.PlayerUid), v); err != nil {
				return err
			}
		}

		// delete old players
		for uid := range existingPlayers {
			if _, exists := newPlayers[uid]; !exists {
				if err := b.Delete([]byte(uid)); err != nil {
					return err
				}
			}
		}

		return nil
	})
}

func PutPlayersOnline(db *bbolt.DB, players []database.OnlinePlayer) error {
	return putPlayersOnlineAt(db, players, time.Now().UTC())
}

func putPlayersOnlineAt(db *bbolt.DB, players []database.OnlinePlayer, now time.Time) error {
	return db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte("players"))
		presence, err := tx.CreateBucketIfNotExists([]byte("player_presence_events"))
		if err != nil {
			return err
		}
		onlinePlayerUIDs := make(map[string]struct{}, len(players))
		for _, p := range players {
			if p.PlayerUid == "" {
				continue
			}
			onlinePlayerUIDs[p.PlayerUid] = struct{}{}
			existingPlayerData := b.Get([]byte(p.PlayerUid))
			var player database.Player
			wasOnline := false
			if existingPlayerData == nil {
				// player online but not in database
				player.PlayerUid = p.PlayerUid
				player.UserId = p.UserId
				player.SteamId = p.SteamId
				player.Nickname = p.Nickname
				player.AccountName = p.AccountName
			} else {
				if err := json.Unmarshal(existingPlayerData, &player); err != nil {
					return err
				}
				wasOnline = player.IsOnline
				if player.SteamId == "" || strings.Contains(player.SteamId, "000000") {
					player.SteamId = p.SteamId
				}
				// UserId 补充
				if player.UserId == "" {
					player.UserId = p.UserId
				}
				// AccountName 补充
				if player.AccountName == "" {
					player.AccountName = p.AccountName
				}
			}
			player.Ip = p.Ip
			player.Ping = p.Ping
			player.LocationX = p.LocationX
			player.LocationY = p.LocationY
			player.Level = p.Level
			player.BuildingCount = p.BuildingCount
			player.LastOnline = now
			if !player.IsOnline || player.OnlineSince.IsZero() {
				player.OnlineSince = now
			} else if !player.OnlineLastSeenAt.IsZero() && now.After(player.OnlineLastSeenAt) {
				player.TotalOnlineSeconds += int64(now.Sub(player.OnlineLastSeenAt) / time.Second)
			}
			player.IsOnline = true
			player.OnlineLastSeenAt = now
			player.CurrentSessionSeconds = durationSeconds(player.OnlineSince, now)
			if !wasOnline {
				if err := putPlayerPresenceEvent(presence, database.PlayerPresenceEvent{PlayerUID: player.PlayerUid, Nickname: player.Nickname, Online: true, CreatedAt: now}); err != nil {
					return err
				}
			}

			v, err := json.Marshal(player)
			if err != nil {
				return err
			}
			if err := b.Put([]byte(p.PlayerUid), v); err != nil {
				return err
			}
		}

		offlineUpdates := make(map[string][]byte)
		if err := b.ForEach(func(k, v []byte) error {
			if _, online := onlinePlayerUIDs[string(k)]; online {
				return nil
			}
			var player database.Player
			if err := json.Unmarshal(v, &player); err != nil {
				return err
			}
			if !player.IsOnline {
				return nil
			}
			if err := putPlayerPresenceEvent(presence, database.PlayerPresenceEvent{PlayerUID: player.PlayerUid, Nickname: player.Nickname, Online: false, CreatedAt: now}); err != nil {
				return err
			}
			player.IsOnline = false
			player.OnlineSince = time.Time{}
			player.OnlineLastSeenAt = time.Time{}
			player.CurrentSessionSeconds = 0
			encoded, err := json.Marshal(player)
			if err != nil {
				return err
			}
			offlineUpdates[string(k)] = encoded
			return nil
		}); err != nil {
			return err
		}
		for key, value := range offlineUpdates {
			if err := b.Put([]byte(key), value); err != nil {
				return err
			}
		}
		for presence.Stats().KeyN > 10000 {
			oldest, _ := presence.Cursor().First()
			if oldest == nil {
				break
			}
			if err := presence.Delete(oldest); err != nil {
				return err
			}
		}
		return nil
	})
}

func putPlayerPresenceEvent(bucket *bbolt.Bucket, event database.PlayerPresenceEvent) error {
	data, err := json.Marshal(event)
	if err != nil {
		return err
	}
	state := "offline"
	if event.Online {
		state = "online"
	}
	key := event.CreatedAt.UTC().Format("20060102T150405.000000000Z") + "\x00" + event.PlayerUID + "\x00" + state
	return bucket.Put([]byte(key), data)
}

func ListPlayerPresenceEvents(db *bbolt.DB, playerUID string, limit int) ([]database.PlayerPresenceEvent, error) {
	if limit < 1 {
		limit = 10
	}
	if limit > 100 {
		limit = 100
	}
	items := make([]database.PlayerPresenceEvent, 0, limit)
	err := db.View(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket([]byte("player_presence_events"))
		if bucket == nil {
			return nil
		}
		cursor := bucket.Cursor()
		for _, value := cursor.Last(); value != nil && len(items) < limit; _, value = cursor.Prev() {
			var event database.PlayerPresenceEvent
			if err := json.Unmarshal(value, &event); err != nil {
				return err
			}
			if playerUID != "" && event.PlayerUID != playerUID {
				continue
			}
			items = append(items, event)
		}
		return nil
	})
	return items, err
}

func durationSeconds(start, end time.Time) int64 {
	if start.IsZero() || !end.After(start) {
		return 0
	}
	return int64(end.Sub(start) / time.Second)
}

func refreshPlayerDuration(player *database.TersePlayer, now time.Time) {
	if !player.IsOnline {
		player.CurrentSessionSeconds = 0
		return
	}
	player.CurrentSessionSeconds = durationSeconds(player.OnlineSince, now)
	if !player.OnlineLastSeenAt.IsZero() && now.After(player.OnlineLastSeenAt) {
		player.TotalOnlineSeconds += durationSeconds(player.OnlineLastSeenAt, now)
	}
}

func ListPlayers(db *bbolt.DB) ([]database.TersePlayer, error) {
	players := make([]database.TersePlayer, 0)
	now := time.Now().UTC()
	err := db.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte("players"))
		return b.ForEach(func(k, v []byte) error {
			if strings.Contains(string(k), "000000") {
				return nil
			}
			var player database.TersePlayer
			if err := json.Unmarshal(v, &player); err != nil {
				return err
			}
			refreshPlayerDuration(&player, now)
			players = append(players, player)
			return nil
		})
	})
	if err != nil {
		return nil, err
	}
	return players, nil
}

func GetPlayer(db *bbolt.DB, playerUid string) (database.Player, error) {
	var player database.Player
	err := db.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte("players"))
		v := b.Get([]byte(playerUid))
		if v == nil {
			return ErrNoRecord
		}
		if err := json.Unmarshal(v, &player); err != nil {
			return err
		}
		refreshPlayerDuration(&player.TersePlayer, time.Now().UTC())
		return nil
	})
	if err != nil {
		return database.Player{}, err
	}
	return player, nil
}

func AddWhitelist(db *bbolt.DB, player database.PlayerW) error {
	return db.Update(func(tx *bbolt.Tx) error {
		// 获取或创建白名单bucket
		b, err := tx.CreateBucketIfNotExists([]byte("whitelist"))
		if err != nil {
			return err
		}

		// 序列化玩家数据为JSON
		playerData, err := json.Marshal(player)
		if err != nil {
			return err
		}

		// 使用 findPlayerKey 检查玩家是否已经在白名单中
		key, err := findPlayerKey(b, player)
		if err != nil {
			return err
		}

		// 如果玩家已存在，更新其信息；如果不存在，创建新的键
		if key != nil {
			// 玩家已存在，更新其信息
			if err := b.Put(key, playerData); err != nil {
				return err
			}
		} else {
			// 玩家不存在，添加新玩家
			// 生成新玩家的唯一键
			newPlayerKey := []byte(player.Name + "|" + player.SteamID + "|" + player.PlayerUID)
			if err := b.Put(newPlayerKey, playerData); err != nil {
				return err
			}
		}

		return nil
	})
}

func ListWhitelist(db *bbolt.DB) ([]database.PlayerW, error) {
	var players []database.PlayerW

	err := db.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte("whitelist"))
		if b == nil {
			return nil // No error, just an empty list if the bucket doesn't exist.
		}

		return b.ForEach(func(k, v []byte) error {
			var player database.PlayerW
			if err := json.Unmarshal(v, &player); err != nil {
				return err
			}
			players = append(players, player)
			return nil
		})
	})

	return players, err
}

// findPlayerKey tries to find a player in the whitelist and returns the key if found.
func findPlayerKey(b *bbolt.Bucket, player database.PlayerW) ([]byte, error) {
	var keyFound []byte
	err := b.ForEach(func(k, v []byte) error {
		var existingPlayer database.PlayerW
		if err := json.Unmarshal(v, &existingPlayer); err != nil {
			return err
		}
		if matchesCriteria(existingPlayer, player) {
			keyFound = append([]byte(nil), k...) // Make a copy of the key
			return errors.New("player found")    // Use an error to break out of the iteration early.
		}
		return nil
	})

	if err != nil && err.Error() == "player found" {
		return keyFound, nil
	}

	return nil, err
}

// RemoveWhitelist removes a player from the whitelist.
func RemoveWhitelist(db *bbolt.DB, player database.PlayerW) error {
	return db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte("whitelist"))
		if b == nil {
			return errors.New("whitelist bucket does not exist")
		}

		key, err := findPlayerKey(b, player)
		if err != nil {
			return err
		}
		if key == nil {
			return errors.New("player not found in whitelist")
		}

		return b.Delete(key)
	})
}

// matchesCriteria checks if the given player matches the criteria.
func matchesCriteria(existingPlayer, player database.PlayerW) bool {
	// 如果PlayerUID非空且匹配，认为是同一个玩家
	if player.PlayerUID != "" && existingPlayer.PlayerUID == player.PlayerUID {
		return true
	}
	// 如果Name非空且匹配，认为是同一个玩家
	if player.Name != "" && existingPlayer.Name == player.Name {
		return true
	}
	// 如果SteamID非空且匹配，认为是同一个玩家
	if player.SteamID != "" && existingPlayer.SteamID == player.SteamID {
		return true
	}
	// 如果没有任何字段匹配，返回false
	return false
}

func PutWhitelist(db *bbolt.DB, players []database.PlayerW) error {
	return db.Update(func(tx *bbolt.Tx) error {
		// 获取或创建白名单bucket
		b, err := tx.CreateBucketIfNotExists([]byte("whitelist"))
		if err != nil {
			return err
		}

		// 清空现有的白名单
		err = b.ForEach(func(k, v []byte) error {
			return b.Delete(k)
		})
		if err != nil {
			return err
		}

		// 遍历并添加新的玩家数据到白名单
		for _, player := range players {
			playerData, err := json.Marshal(player)
			if err != nil {
				return err
			}
			identifier := player.PlayerUID
			if identifier == "" {
				if identifier = player.SteamID; identifier == "" {
					continue
				}
			}
			if err := b.Put([]byte(identifier), playerData); err != nil {
				return err
			}
		}

		return nil
	})
}
