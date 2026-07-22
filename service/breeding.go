package service

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/zaigie/palworld-server-tool/internal/config"
	"github.com/zaigie/palworld-server-tool/internal/database"
	"go.etcd.io/bbolt"
)

var (
	breedingEventsBucket = []byte("breeding_events")
	breedingDedupBucket  = []byte("breeding_event_dedup")
	breedingStateBucket  = []byte("breeding_monitor_state")
	breedingEventFarmIdx = []byte("breeding_events_by_farm")
	breedingEventBaseIdx = []byte("breeding_events_by_base")
	breedingEventTypeIdx = []byte("breeding_events_by_type")
	breedingEventReadIdx = []byte("breeding_events_by_read")
	breedingParserFailed = []byte("__parser_failed_at")
)

type BreedingMonitorOptions = config.BreedingMonitorConfig

type breedingFarmState struct {
	FarmID                    string           `json:"farm_id"`
	SnapshotID                string           `json:"snapshot_id"`
	SaveFileTime              time.Time        `json:"save_file_time"`
	Reliable                  bool             `json:"reliable"`
	Identity                  bool             `json:"identity"`
	Eggs                      map[string]int64 `json:"eggs"`
	Total                     int64            `json:"total"`
	LastSuccessfulDetectionAt time.Time        `json:"last_successful_detection_at"`
}

type BreedingFarmQuery struct {
	BaseID        string
	GuildID       string
	HasEgg        *bool
	CakeEmpty     *bool
	ParentMissing *bool
	HasWarning    *bool
	Sort          string
	Page          int
	PageSize      int
}

type BreedingFarmPage struct {
	Metadata     database.SnapshotMetadata       `json:"metadata"`
	ParserStatus BreedingParserStatus            `json:"parser_status"`
	Items        []database.BreedingFarmSnapshot `json:"items"`
	Page         int                             `json:"page"`
	PageSize     int                             `json:"page_size"`
	Total        int                             `json:"total"`
}

type BreedingParserStatus struct {
	Failed   bool       `json:"failed"`
	FailedAt *time.Time `json:"failed_at"`
}

type BreedingEventQuery struct {
	Unread    *bool
	BaseID    string
	FarmID    string
	EventType string
	Page      int
	PageSize  int
}

type BreedingEventPage struct {
	Items    []database.BreedingFarmEvent `json:"items"`
	Page     int                          `json:"page"`
	PageSize int                          `json:"page_size"`
	Total    int                          `json:"total"`
}

func nullableString(value *string) string {
	if value == nil {
		return "unknown"
	}
	return strings.ToLower(strings.TrimSpace(*value))
}

func selectedFarm(options BreedingMonitorOptions, farm database.BreedingFarmSnapshot) bool {
	if !options.Enabled {
		return false
	}
	if options.SelectionMode == "all" {
		return true
	}
	for _, id := range options.SelectedFarmIDs {
		if id == farm.FarmID {
			return true
		}
	}
	for _, id := range options.SelectedBaseIDs {
		if id == farm.BaseID {
			return true
		}
	}
	return false
}

func farmEggState(farm database.BreedingFarmSnapshot, eggs []database.BreedingFarmEgg, capabilities database.BreedingFarmCapabilities) breedingFarmState {
	reliable := capabilities.FarmDetection && capabilities.BaseAssociation && capabilities.EggDetection &&
		strings.TrimSpace(capabilities.ValidatedGameVersion) != "" && farm.AssociationVerified &&
		farm.ParsingComplete && farm.GameVersionSupported && farm.Confidence == "high"
	state := breedingFarmState{FarmID: farm.FarmID, Reliable: reliable, Identity: capabilities.EggIdentity && farm.IdentitySupported, Eggs: map[string]int64{}}
	for _, egg := range eggs {
		if !egg.Ready || egg.Count <= 0 {
			continue
		}
		if !egg.AssociationVerified {
			state.Reliable = false
		}
		key := "item:" + nullableString(egg.EggItemID)
		if state.Identity && egg.EggInstanceID != "" {
			key = "instance:" + egg.EggInstanceID
		}
		state.Eggs[key] += egg.Count
		state.Total += egg.Count
	}
	return state
}

func processBreedingSnapshotTx(tx *bbolt.Tx, snapshot *bbolt.Bucket, payload database.SnapshotPayload, snapshotID string, options BreedingMonitorOptions, now time.Time, createdEvents *[]database.BreedingFarmEvent) error {
	events, err := tx.CreateBucketIfNotExists(breedingEventsBucket)
	if err != nil {
		return err
	}
	dedup, err := tx.CreateBucketIfNotExists(breedingDedupBucket)
	if err != nil {
		return err
	}
	states, err := tx.CreateBucketIfNotExists(breedingStateBucket)
	if err != nil {
		return err
	}
	if err := states.Delete(breedingParserFailed); err != nil {
		return err
	}
	for _, name := range [][]byte{breedingEventFarmIdx, breedingEventBaseIdx, breedingEventTypeIdx, breedingEventReadIdx} {
		if _, err := tx.CreateBucketIfNotExists(name); err != nil {
			return err
		}
	}
	eggsByFarm := make(map[string][]database.BreedingFarmEgg)
	for _, egg := range payload.BreedingEggs {
		eggsByFarm[egg.FarmID] = append(eggsByFarm[egg.FarmID], egg)
	}
	for _, farm := range payload.BreedingFarms {
		if !selectedFarm(options, farm) {
			continue
		}
		current := farmEggState(farm, eggsByFarm[farm.FarmID], payload.BreedingCapabilities)
		current.SnapshotID = snapshotID
		current.SaveFileTime = payload.Metadata.SaveFileTime
		if current.SaveFileTime.IsZero() {
			current.Reliable = false
		}
		var previous breedingFarmState
		hasPrevious := false
		if data := states.Get([]byte(farm.FarmID)); data != nil {
			if err := json.Unmarshal(data, &previous); err != nil {
				return err
			}
			hasPrevious = true
		}
		if current.Reliable {
			current.LastSuccessfulDetectionAt = now.UTC()
		} else if hasPrevious {
			current.LastSuccessfulDetectionAt = previous.LastSuccessfulDetectionAt
		}
		if hasPrevious && !current.SaveFileTime.IsZero() && !previous.SaveFileTime.IsZero() && !current.SaveFileTime.After(previous.SaveFileTime) {
			continue
		}
		if !current.Reliable || hasPrevious && !previous.Reliable {
			if err := putBreedingState(states, current); err != nil {
				return err
			}
			continue
		}
		if !hasPrevious && !options.NotifyExistingOnEnable {
			if err := putBreedingState(states, current); err != nil {
				return err
			}
			continue
		}
		if !hasPrevious {
			previous = breedingFarmState{FarmID: farm.FarmID, Reliable: true, Identity: current.Identity, Eggs: map[string]int64{}}
		}
		newKeys := make([]string, 0)
		identityComparison := current.Identity && previous.Identity
		if identityComparison {
			for key, count := range current.Eggs {
				for index := previous.Eggs[key]; index < count; index++ {
					newKeys = append(newKeys, key)
				}
			}
		} else if current.Total > previous.Total {
			for key, count := range current.Eggs {
				delta := count - previous.Eggs[key]
				for index := int64(0); index < delta; index++ {
					newKeys = append(newKeys, fmt.Sprintf("%s|ordinal:%d", key, index+1))
				}
			}
		}
		if current.Total >= int64(options.MinimumReadyEggs) && len(newKeys) > 0 {
			// Without stable identities, equal-count type changes are ambiguous and
			// never alert. With identities, a replacement egg is still genuinely new.
			if identityComparison || current.Total > previous.Total {
				sort.Strings(newKeys)
				if !options.NotifyOnEachEgg {
					newKeys = newKeys[:1]
				}
				for _, key := range newKeys {
					if err := createBreedingEvent(tx, events, dedup, farm, previous, current, key, snapshotID, now, createdEvents); err != nil {
						return err
					}
				}
			}
		}
		if err := putBreedingState(states, current); err != nil {
			return err
		}
	}
	return pruneBreedingEvents(tx, events, dedup, now.AddDate(0, 0, -options.HistoryRetentionDays))
}

func putBreedingState(bucket *bbolt.Bucket, state breedingFarmState) error {
	data, err := json.Marshal(state)
	if err != nil {
		return err
	}
	return bucket.Put([]byte(state.FarmID), data)
}

func MarkBreedingParserFailed(db *bbolt.DB, failedAt time.Time) error {
	return db.Update(func(tx *bbolt.Tx) error {
		states, err := tx.CreateBucketIfNotExists(breedingStateBucket)
		if err != nil {
			return err
		}
		updates := make(map[string][]byte)
		if err := states.ForEach(func(key, value []byte) error {
			if string(key) == string(breedingParserFailed) {
				return nil
			}
			var state breedingFarmState
			if err := json.Unmarshal(value, &state); err != nil {
				return err
			}
			state.Reliable = false
			data, err := json.Marshal(state)
			if err != nil {
				return err
			}
			updates[string(key)] = data
			return nil
		}); err != nil {
			return err
		}
		for key, value := range updates {
			if err := states.Put([]byte(key), value); err != nil {
				return err
			}
		}
		return states.Put(breedingParserFailed, []byte(failedAt.UTC().Format(time.RFC3339Nano)))
	})
}

func breedingParserStatus(tx *bbolt.Tx) BreedingParserStatus {
	states := tx.Bucket(breedingStateBucket)
	if states == nil {
		return BreedingParserStatus{}
	}
	raw := states.Get(breedingParserFailed)
	if len(raw) == 0 {
		return BreedingParserStatus{}
	}
	failedAt, err := time.Parse(time.RFC3339Nano, string(raw))
	if err != nil {
		return BreedingParserStatus{Failed: true}
	}
	return BreedingParserStatus{Failed: true, FailedAt: &failedAt}
}

func createBreedingEvent(tx *bbolt.Tx, events, dedup *bbolt.Bucket, farm database.BreedingFarmSnapshot, previous, current breedingFarmState, eggKey, snapshotID string, now time.Time, createdEvents *[]database.BreedingFarmEvent) error {
	dedupRaw := farm.FarmID + "|" + eggKey + "|" + current.SaveFileTime.UTC().Format(time.RFC3339Nano) + fmt.Sprintf("|%d", current.Total)
	digest := sha256.Sum256([]byte(dedupRaw))
	dedupKey := hex.EncodeToString(digest[:])
	if dedup.Get([]byte(dedupKey)) != nil {
		return nil
	}
	event := database.BreedingFarmEvent{
		EventID: uuid.NewString(), FarmID: farm.FarmID, BaseID: farm.BaseID, BaseName: farm.BaseName,
		GuildID: farm.GuildID, EventType: "egg_ready", DedupKey: dedupKey,
		PreviousCount: previous.Total, CurrentCount: current.Total, SnapshotID: snapshotID, CreatedAt: now.UTC(),
	}
	_, displayName, err := resolveBaseDisplayNameTx(tx, farm.BaseID, farm.BaseName)
	if err != nil {
		return err
	}
	event.BaseDisplayName = displayName
	eggValueKey := strings.SplitN(eggKey, "|ordinal:", 2)[0]
	if strings.HasPrefix(eggValueKey, "instance:") {
		event.EggInstanceID = strings.TrimPrefix(eggValueKey, "instance:")
	} else if strings.HasPrefix(eggValueKey, "item:") {
		item := strings.TrimPrefix(eggValueKey, "item:")
		if item != "unknown" {
			event.EggItemID = &item
		}
	}
	data, err := json.Marshal(event)
	if err != nil {
		return err
	}
	key := now.UTC().Format("20060102T150405.000000000Z") + "\x00" + event.EventID
	if err := events.Put([]byte(key), data); err != nil {
		return err
	}
	for name, value := range map[string]string{
		string(breedingEventFarmIdx): farm.FarmID,
		string(breedingEventBaseIdx): farm.BaseID,
		string(breedingEventTypeIdx): event.EventType,
		string(breedingEventReadIdx): "0",
	} {
		if err := tx.Bucket([]byte(name)).Put([]byte(value+"\x00"+key), nil); err != nil {
			return err
		}
	}
	if err := dedup.Put([]byte(dedupKey), []byte(key)); err != nil {
		return err
	}
	if createdEvents != nil {
		*createdEvents = append(*createdEvents, event)
	}
	return nil
}

type BreedingGameNotification struct {
	FarmID   string
	EventIDs []string
	Message  string
}

// BuildBreedingGameNotifications converts persisted, deduplicated egg events
// into one game announcement per farm. Keeping this channel-neutral makes it
// possible to add QQ or other delivery adapters without changing save parsing.
func BuildBreedingGameNotifications(events []database.BreedingFarmEvent, messageTemplate string) []BreedingGameNotification {
	type farmEvents struct {
		baseName string
		current  int64
		previous int64
		events   []database.BreedingFarmEvent
	}
	grouped := make(map[string]*farmEvents)
	for _, event := range events {
		if event.EventType != "egg_ready" {
			continue
		}
		group := grouped[event.FarmID]
		if group == nil {
			baseName := event.BaseDisplayName
			if baseName == "" {
				baseName = event.BaseName
			}
			group = &farmEvents{baseName: baseName, current: event.CurrentCount, previous: event.PreviousCount}
			grouped[event.FarmID] = group
		}
		if event.CurrentCount > group.current {
			group.current = event.CurrentCount
		}
		if event.PreviousCount < group.previous {
			group.previous = event.PreviousCount
		}
		group.events = append(group.events, event)
	}
	farmIDs := make([]string, 0, len(grouped))
	for farmID := range grouped {
		farmIDs = append(farmIDs, farmID)
	}
	sort.Strings(farmIDs)
	result := make([]BreedingGameNotification, 0, len(farmIDs))
	for _, farmID := range farmIDs {
		group := grouped[farmID]
		baseName := singleLineBreedingMessageValue(group.baseName)
		if baseName == "" {
			baseName = "未命名据点"
		}
		newCount := group.current - group.previous
		if eventCount := int64(len(group.events)); eventCount > newCount {
			newCount = eventCount
		}
		if newCount < 1 {
			newCount = 1
		}
		message := strings.NewReplacer(
			"{base}", baseName,
			"{count}", fmt.Sprintf("%d", group.current),
			"{new_count}", fmt.Sprintf("%d", newCount),
		).Replace(messageTemplate)
		eventIDs := make([]string, 0, len(group.events))
		for _, event := range group.events {
			eventIDs = append(eventIDs, event.EventID)
		}
		result = append(result, BreedingGameNotification{FarmID: farmID, EventIDs: eventIDs, Message: message})
	}
	return result
}

func singleLineBreedingMessageValue(value string) string {
	return strings.Join(strings.Fields(value), " ")
}

func pruneBreedingEvents(tx *bbolt.Tx, events, dedup *bbolt.Bucket, before time.Time) error {
	if before.IsZero() {
		return nil
	}
	cursor := events.Cursor()
	for key, value := cursor.First(); key != nil; key, value = cursor.Next() {
		var event database.BreedingFarmEvent
		if json.Unmarshal(value, &event) != nil || !event.CreatedAt.Before(before) {
			continue
		}
		if event.DedupKey != "" {
			_ = dedup.Delete([]byte(event.DedupKey))
		}
		indexValues := map[string]string{
			string(breedingEventFarmIdx): event.FarmID,
			string(breedingEventBaseIdx): event.BaseID,
			string(breedingEventTypeIdx): event.EventType,
			string(breedingEventReadIdx): map[bool]string{false: "0", true: "1"}[event.Read],
		}
		for name, indexValue := range indexValues {
			_ = tx.Bucket([]byte(name)).Delete([]byte(indexValue + "\x00" + string(key)))
		}
		if err := cursor.Delete(); err != nil {
			return err
		}
	}
	return nil
}

func PrepareBreedingMonitor(db *bbolt.DB, options BreedingMonitorOptions, notifyExisting bool) error {
	return db.Update(func(tx *bbolt.Tx) error {
		states, err := tx.CreateBucketIfNotExists(breedingStateBucket)
		if err != nil {
			return err
		}
		snapshot, err := activeSnapshot(tx)
		if errors.Is(err, ErrSnapshotUnavailable) {
			return nil
		}
		if err != nil {
			return err
		}
		metadata, err := snapshotMetadata(snapshot)
		if err != nil {
			return err
		}
		capabilities, err := breedingCapabilities(snapshot)
		if err != nil {
			return err
		}
		farms, err := listBucket[database.BreedingFarmSnapshot](snapshot.Bucket(breedingFarmsBucket))
		if err != nil {
			return err
		}
		eggs, err := listBucket[database.BreedingFarmEgg](snapshot.Bucket(breedingEggsBucket))
		if err != nil {
			return err
		}
		eggsByFarm := make(map[string][]database.BreedingFarmEgg)
		for _, egg := range eggs {
			eggsByFarm[egg.FarmID] = append(eggsByFarm[egg.FarmID], egg)
		}
		for _, farm := range farms {
			if !selectedFarm(options, farm) {
				continue
			}
			if notifyExisting {
				if err := states.Delete([]byte(farm.FarmID)); err != nil {
					return err
				}
				continue
			}
			state := farmEggState(farm, eggsByFarm[farm.FarmID], capabilities)
			state.SnapshotID = metadata.SnapshotID
			state.SaveFileTime = metadata.SaveFileTime
			if state.Reliable {
				state.LastSuccessfulDetectionAt = metadata.SnapshotTime
			}
			if err := putBreedingState(states, state); err != nil {
				return err
			}
		}
		return nil
	})
}

func breedingCapabilities(snapshot *bbolt.Bucket) (database.BreedingFarmCapabilities, error) {
	var result database.BreedingFarmCapabilities
	data := snapshot.Get(breedingCapabilitiesKey)
	if data == nil {
		return result, nil
	}
	return result, json.Unmarshal(data, &result)
}

func BreedingCapabilities(db *bbolt.DB) (database.BreedingFarmCapabilities, database.SnapshotMetadata, error) {
	var capabilities database.BreedingFarmCapabilities
	var metadata database.SnapshotMetadata
	err := db.View(func(tx *bbolt.Tx) error {
		snapshot, err := activeSnapshot(tx)
		if err != nil {
			return err
		}
		metadata, err = snapshotMetadata(snapshot)
		if err != nil {
			return err
		}
		capabilities, err = breedingCapabilities(snapshot)
		return err
	})
	return capabilities, metadata, err
}

func ListBreedingFarms(db *bbolt.DB, query BreedingFarmQuery) (BreedingFarmPage, error) {
	result := BreedingFarmPage{}
	err := db.View(func(tx *bbolt.Tx) error {
		result.ParserStatus = breedingParserStatus(tx)
		snapshot, err := activeSnapshot(tx)
		if err != nil {
			return err
		}
		result.Metadata, err = snapshotMetadata(snapshot)
		if err != nil {
			return err
		}
		farms, err := listBucket[database.BreedingFarmSnapshot](snapshot.Bucket(breedingFarmsBucket))
		if err != nil {
			return err
		}
		parents, err := listBucket[database.BreedingFarmParent](snapshot.Bucket(breedingParentsBucket))
		if err != nil {
			return err
		}
		cakes, err := listBucket[database.BreedingFarmCakeContainer](snapshot.Bucket(breedingCakesBucket))
		if err != nil {
			return err
		}
		eggs, err := listBucket[database.BreedingFarmEgg](snapshot.Bucket(breedingEggsBucket))
		if err != nil {
			return err
		}
		parentMap := make(map[string][]database.BreedingFarmParent)
		cakeMap := make(map[string]database.BreedingFarmCakeContainer)
		eggMap := make(map[string][]database.BreedingFarmEgg)
		lastEggMap := make(map[string]time.Time)
		for _, parent := range parents {
			parentMap[parent.FarmID] = append(parentMap[parent.FarmID], parent)
		}
		for _, cake := range cakes {
			cakeMap[cake.FarmID] = cake
		}
		for _, egg := range eggs {
			eggMap[egg.FarmID] = append(eggMap[egg.FarmID], egg)
		}
		if events := tx.Bucket(breedingEventsBucket); events != nil {
			if err := events.ForEach(func(_, value []byte) error {
				var event database.BreedingFarmEvent
				if err := json.Unmarshal(value, &event); err != nil {
					return err
				}
				if event.EventType == "egg_ready" && event.CreatedAt.After(lastEggMap[event.FarmID]) {
					lastEggMap[event.FarmID] = event.CreatedAt
				}
				return nil
			}); err != nil {
				return err
			}
		}
		for _, farm := range farms {
			_, displayName, err := resolveBaseDisplayNameTx(tx, farm.BaseID, farm.BaseName)
			if err != nil {
				return err
			}
			farm.BaseDisplayName = displayName
			farm.Parents = parentMap[farm.FarmID]
			if cake, ok := cakeMap[farm.FarmID]; ok {
				farm.Cake = &cake
			}
			farm.Eggs = eggMap[farm.FarmID]
			if lastEggAt, ok := lastEggMap[farm.FarmID]; ok {
				farm.LastEggAt = &lastEggAt
			}
			if !matchesBreedingFarm(farm, query) {
				continue
			}
			result.Items = append(result.Items, farm)
		}
		return nil
	})
	if err != nil {
		return result, err
	}
	sort.SliceStable(result.Items, func(i, j int) bool {
		switch query.Sort {
		case "cake_count":
			return pointerInt(result.Items[i].CakeCount) > pointerInt(result.Items[j].CakeCount)
		case "last_egg":
			return pointerTime(result.Items[i].LastEggAt).After(pointerTime(result.Items[j].LastEggAt))
		default:
			return pointerInt(result.Items[i].EggCount) > pointerInt(result.Items[j].EggCount)
		}
	})
	result.Total = len(result.Items)
	result.Page, result.PageSize = normalizePage(query.Page, query.PageSize)
	start, end := breedingPageBounds(result.Total, result.Page, result.PageSize)
	result.Items = result.Items[start:end]
	return result, nil
}

func matchesBreedingFarm(farm database.BreedingFarmSnapshot, query BreedingFarmQuery) bool {
	if query.BaseID != "" && farm.BaseID != query.BaseID || query.GuildID != "" && farm.GuildID != query.GuildID {
		return false
	}
	if query.HasEgg != nil && (pointerInt(farm.EggCount) > 0) != *query.HasEgg {
		return false
	}
	if query.CakeEmpty != nil && (farm.CakeCount != nil && *farm.CakeCount == 0) != *query.CakeEmpty {
		return false
	}
	missing := len(farm.Parents) < 2
	if query.ParentMissing != nil && missing != *query.ParentMissing {
		return false
	}
	hasWarning := len(farm.Warnings) > 0 || !farm.ParsingComplete
	if query.HasWarning != nil && hasWarning != *query.HasWarning {
		return false
	}
	return true
}

func breedingPageBounds(total, page, size int) (int, int) {
	start := (page - 1) * size
	if start > total {
		start = total
	}
	end := start + size
	if end > total {
		end = total
	}
	return start, end
}

func pointerInt(value *int64) int64 {
	if value == nil {
		return -1
	}
	return *value
}
func pointerTime(value *time.Time) time.Time {
	if value == nil {
		return time.Time{}
	}
	return *value
}

func GetBreedingFarm(db *bbolt.DB, farmID string) (database.BreedingFarmSnapshot, database.SnapshotMetadata, error) {
	var farm database.BreedingFarmSnapshot
	var metadata database.SnapshotMetadata
	err := db.View(func(tx *bbolt.Tx) error {
		snapshot, err := activeSnapshot(tx)
		if err != nil {
			return err
		}
		metadata, err = snapshotMetadata(snapshot)
		if err != nil {
			return err
		}
		bucket := snapshot.Bucket(breedingFarmsBucket)
		if bucket == nil || bucket.Get([]byte(farmID)) == nil {
			return ErrNoRecord
		}
		if err := json.Unmarshal(bucket.Get([]byte(farmID)), &farm); err != nil {
			return err
		}
		_, displayName, err := resolveBaseDisplayNameTx(tx, farm.BaseID, farm.BaseName)
		if err != nil {
			return err
		}
		farm.BaseDisplayName = displayName
		parents, err := listBucket[database.BreedingFarmParent](snapshot.Bucket(breedingParentsBucket))
		if err != nil {
			return err
		}
		for _, parent := range parents {
			if parent.FarmID == farmID {
				farm.Parents = append(farm.Parents, parent)
			}
		}
		if cakes := snapshot.Bucket(breedingCakesBucket); cakes != nil {
			if data := cakes.Get([]byte(farmID)); data != nil {
				var cake database.BreedingFarmCakeContainer
				if err := json.Unmarshal(data, &cake); err != nil {
					return err
				}
				farm.Cake = &cake
			}
		}
		eggs, err := listBucket[database.BreedingFarmEgg](snapshot.Bucket(breedingEggsBucket))
		if err != nil {
			return err
		}
		for _, egg := range eggs {
			if egg.FarmID == farmID {
				farm.Eggs = append(farm.Eggs, egg)
			}
		}
		if events := tx.Bucket(breedingEventsBucket); events != nil {
			return events.ForEach(func(_, value []byte) error {
				var event database.BreedingFarmEvent
				if err := json.Unmarshal(value, &event); err != nil {
					return err
				}
				if event.FarmID == farmID && event.EventType == "egg_ready" && (farm.LastEggAt == nil || event.CreatedAt.After(*farm.LastEggAt)) {
					lastEggAt := event.CreatedAt
					farm.LastEggAt = &lastEggAt
				}
				return nil
			})
		}
		return nil
	})
	return farm, metadata, err
}

func ListBreedingParents(db *bbolt.DB, farmID string) ([]database.BreedingFarmParent, database.SnapshotMetadata, error) {
	farm, metadata, err := GetBreedingFarm(db, farmID)
	return farm.Parents, metadata, err
}

func GetBreedingCakes(db *bbolt.DB, farmID string) (*database.BreedingFarmCakeContainer, database.SnapshotMetadata, error) {
	farm, metadata, err := GetBreedingFarm(db, farmID)
	return farm.Cake, metadata, err
}

func ListBreedingEggs(db *bbolt.DB, farmID string) ([]database.BreedingFarmEgg, database.SnapshotMetadata, error) {
	farm, metadata, err := GetBreedingFarm(db, farmID)
	return farm.Eggs, metadata, err
}

func ListBreedingEvents(db *bbolt.DB, query BreedingEventQuery) (BreedingEventPage, error) {
	result := BreedingEventPage{}
	err := db.View(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket(breedingEventsBucket)
		if bucket == nil {
			return nil
		}
		return bucket.ForEach(func(_, value []byte) error {
			var event database.BreedingFarmEvent
			if err := json.Unmarshal(value, &event); err != nil {
				return err
			}
			if query.Unread != nil && (!event.Read) != *query.Unread || query.BaseID != "" && event.BaseID != query.BaseID || query.FarmID != "" && event.FarmID != query.FarmID || query.EventType != "" && event.EventType != query.EventType {
				return nil
			}
			_, displayName, err := resolveBaseDisplayNameTx(tx, event.BaseID, event.BaseName)
			if err != nil {
				return err
			}
			event.BaseDisplayName = displayName
			result.Items = append(result.Items, event)
			return nil
		})
	})
	if err != nil {
		return result, err
	}
	sort.Slice(result.Items, func(i, j int) bool { return result.Items[i].CreatedAt.After(result.Items[j].CreatedAt) })
	result.Total = len(result.Items)
	result.Page, result.PageSize = normalizePage(query.Page, query.PageSize)
	start, end := breedingPageBounds(result.Total, result.Page, result.PageSize)
	result.Items = result.Items[start:end]
	return result, nil
}

func MarkBreedingEventRead(db *bbolt.DB, eventID string) error {
	return updateBreedingEvents(db, func(event *database.BreedingFarmEvent) bool { return event.EventID == eventID })
}

func MarkAllBreedingEventsRead(db *bbolt.DB) error {
	return updateBreedingEvents(db, func(event *database.BreedingFarmEvent) bool { return !event.Read })
}

func updateBreedingEvents(db *bbolt.DB, match func(*database.BreedingFarmEvent) bool) error {
	found := false
	err := db.Update(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket(breedingEventsBucket)
		if bucket == nil {
			return nil
		}
		cursor := bucket.Cursor()
		for key, value := cursor.First(); key != nil; key, value = cursor.Next() {
			var event database.BreedingFarmEvent
			if err := json.Unmarshal(value, &event); err != nil {
				return err
			}
			if !match(&event) {
				continue
			}
			event.Read = true
			data, err := json.Marshal(event)
			if err != nil {
				return err
			}
			if err := bucket.Put(key, data); err != nil {
				return err
			}
			readIndex := tx.Bucket(breedingEventReadIdx)
			_ = readIndex.Delete([]byte("0\x00" + string(key)))
			if err := readIndex.Put([]byte("1\x00"+string(key)), nil); err != nil {
				return err
			}
			found = true
		}
		return nil
	})
	if err != nil {
		return err
	}
	if !found {
		return ErrNoRecord
	}
	return nil
}
