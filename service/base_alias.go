package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/zaigie/palworld-server-tool/internal/database"
	"go.etcd.io/bbolt"
)

var (
	baseAliasesBucket    = []byte("base_aliases")
	ErrInvalidBaseAlias  = errors.New("据点名称无效")
	ErrBaseAliasConflict = errors.New("据点名称冲突")
)

const maximumBaseAliasRunes = 40

// BaseDisplayName resolves the user-facing name without changing the raw name
// extracted from the save. Palworld currently writes a Japanese placeholder
// for unnamed bases; that value is intentionally treated as empty.
func BaseDisplayName(baseID, rawName, customName string) string {
	if name := strings.TrimSpace(customName); name != "" {
		return name
	}
	if name := strings.TrimSpace(rawName); meaningfulBaseName(name) {
		return name
	}
	shortID := strings.TrimSpace(baseID)
	if len(shortID) > 6 {
		shortID = shortID[len(shortID)-6:]
	}
	if shortID == "" {
		return "未命名据点"
	}
	return fmt.Sprintf("未命名据点（%s）", shortID)
}

func meaningfulBaseName(name string) bool {
	if name == "" {
		return false
	}
	lower := strings.ToLower(name)
	placeholders := []string{
		"新規生成拠点テンプレート名",
		"new base camp template",
		"basecamp template",
	}
	for _, placeholder := range placeholders {
		if strings.Contains(lower, strings.ToLower(placeholder)) {
			return false
		}
	}
	return true
}

func NormalizeBaseAlias(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" || !utf8.ValidString(value) || utf8.RuneCountInString(value) > maximumBaseAliasRunes {
		return "", fmt.Errorf("%w: 名称长度必须为 1 到 %d 个字符", ErrInvalidBaseAlias, maximumBaseAliasRunes)
	}
	for _, char := range value {
		if unicode.IsControl(char) || char == '\u2028' || char == '\u2029' {
			return "", fmt.Errorf("%w: 名称不能包含换行或控制字符", ErrInvalidBaseAlias)
		}
	}
	return value, nil
}

func baseAliasBucket(tx *bbolt.Tx) (*bbolt.Bucket, error) {
	return tx.CreateBucketIfNotExists(baseAliasesBucket)
}

func readBaseAlias(bucket *bbolt.Bucket, baseID string) (database.BaseAlias, bool, error) {
	if bucket == nil {
		return database.BaseAlias{}, false, nil
	}
	data := bucket.Get([]byte(baseID))
	if data == nil {
		return database.BaseAlias{}, false, nil
	}
	var alias database.BaseAlias
	if err := json.Unmarshal(data, &alias); err != nil {
		return database.BaseAlias{}, false, err
	}
	return alias, true, nil
}

func resolveBaseDisplayNameTx(tx *bbolt.Tx, baseID, rawName string) (string, string, error) {
	alias, found, err := readBaseAlias(tx.Bucket(baseAliasesBucket), baseID)
	if err != nil {
		return "", "", err
	}
	customName := ""
	if found {
		customName = alias.Name
	}
	return customName, BaseDisplayName(baseID, rawName, customName), nil
}

func decorateBaseTx(tx *bbolt.Tx, base *database.BaseCampSnapshot) error {
	customName, displayName, err := resolveBaseDisplayNameTx(tx, base.BaseID, base.BaseName)
	if err != nil {
		return err
	}
	base.CustomName = customName
	base.DisplayName = displayName
	return nil
}

func SetBaseAlias(db *bbolt.DB, baseID, value string, now time.Time) (database.BaseAliasOverview, error) {
	baseID = strings.TrimSpace(baseID)
	name, err := NormalizeBaseAlias(value)
	if err != nil {
		return database.BaseAliasOverview{}, err
	}
	var result database.BaseAliasOverview
	err = db.Update(func(tx *bbolt.Tx) error {
		snapshot, err := activeSnapshot(tx)
		if err != nil {
			return err
		}
		bases, err := listBucket[database.BaseCampSnapshot](snapshot.Bucket(basesBucket))
		if err != nil {
			return err
		}
		var current *database.BaseCampSnapshot
		for index := range bases {
			if bases[index].BaseID == baseID {
				current = &bases[index]
				break
			}
		}
		if current == nil {
			return ErrNoRecord
		}
		for index := range bases {
			if bases[index].BaseID == baseID {
				continue
			}
			_, displayName, err := resolveBaseDisplayNameTx(tx, bases[index].BaseID, bases[index].BaseName)
			if err != nil {
				return err
			}
			if strings.EqualFold(strings.TrimSpace(displayName), name) {
				return fmt.Errorf("%w: 当前存档中已有名为“%s”的据点", ErrBaseAliasConflict, name)
			}
		}
		if now.IsZero() {
			now = time.Now().UTC()
		}
		alias := database.BaseAlias{BaseID: baseID, Name: name, UpdatedAt: now.UTC()}
		data, err := json.Marshal(alias)
		if err != nil {
			return err
		}
		bucket, err := baseAliasBucket(tx)
		if err != nil {
			return err
		}
		if err := bucket.Put([]byte(baseID), data); err != nil {
			return err
		}
		result = database.BaseAliasOverview{BaseAlias: alias, Active: true, BaseName: current.BaseName, DisplayName: BaseDisplayName(baseID, current.BaseName, name)}
		return nil
	})
	return result, err
}

func DeleteBaseAlias(db *bbolt.DB, baseID string) error {
	baseID = strings.TrimSpace(baseID)
	if baseID == "" {
		return ErrNoRecord
	}
	return db.Update(func(tx *bbolt.Tx) error {
		bucket, err := baseAliasBucket(tx)
		if err != nil {
			return err
		}
		if bucket.Get([]byte(baseID)) == nil {
			return ErrNoRecord
		}
		return bucket.Delete([]byte(baseID))
	})
}

func ListBaseAliases(db *bbolt.DB) ([]database.BaseAliasOverview, error) {
	items := make([]database.BaseAliasOverview, 0)
	err := db.View(func(tx *bbolt.Tx) error {
		activeBases := make(map[string]database.BaseCampSnapshot)
		if snapshot, err := activeSnapshot(tx); err == nil {
			bases, err := listBucket[database.BaseCampSnapshot](snapshot.Bucket(basesBucket))
			if err != nil {
				return err
			}
			for _, base := range bases {
				activeBases[base.BaseID] = base
			}
		} else if !errors.Is(err, ErrSnapshotUnavailable) {
			return err
		}
		aliases, err := listBucket[database.BaseAlias](tx.Bucket(baseAliasesBucket))
		if err != nil {
			return err
		}
		for _, alias := range aliases {
			base, active := activeBases[alias.BaseID]
			items = append(items, database.BaseAliasOverview{
				BaseAlias:   alias,
				Active:      active,
				BaseName:    base.BaseName,
				DisplayName: BaseDisplayName(alias.BaseID, base.BaseName, alias.Name),
			})
		}
		return nil
	})
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].Active != items[j].Active {
			return items[i].Active
		}
		return strings.ToLower(items[i].DisplayName) < strings.ToLower(items[j].DisplayName)
	})
	return items, err
}

func DecorateBreedingEvents(db *bbolt.DB, events []database.BreedingFarmEvent) error {
	return db.View(func(tx *bbolt.Tx) error {
		for index := range events {
			_, displayName, err := resolveBaseDisplayNameTx(tx, events[index].BaseID, events[index].BaseName)
			if err != nil {
				return err
			}
			events[index].BaseDisplayName = displayName
		}
		return nil
	})
}
