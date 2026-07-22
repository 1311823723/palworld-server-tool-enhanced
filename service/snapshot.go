package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/zaigie/palworld-server-tool/internal/database"
	"go.etcd.io/bbolt"
)

var ErrSnapshotUnavailable = errors.New("save snapshot is not available")

var (
	snapshotsBucket         = []byte("save_snapshots")
	snapshotStateBucket     = []byte("save_snapshot_state")
	activeSnapshotKey       = []byte("active_snapshot_id")
	metadataKey             = []byte("metadata")
	breedingCapabilitiesKey = []byte("breeding_capabilities")
	basesBucket             = []byte("base_camps")
	workersBucket           = []byte("work_pals")
	containersBucket        = []byte("containers")
	inventoryBucket         = []byte("inventory_slots")
	breedingFarmsBucket     = []byte("breeding_farms")
	breedingParentsBucket   = []byte("breeding_parents")
	breedingCakesBucket     = []byte("breeding_cakes")
	breedingEggsBucket      = []byte("breeding_eggs")
	breedingFarmBaseIndex   = []byte("breeding_farms_by_base")
	breedingFarmGuildIndex  = []byte("breeding_farms_by_guild")
	indexNames              = [][]byte{
		[]byte("index_item"), []byte("index_source"), []byte("index_player"),
		[]byte("index_guild"), []byte("index_base"), []byte("index_container"),
	}
)

const (
	LowHPPercent      = 30.0
	LowFullStomach    = 30.0
	LowSanity         = 30.0
	defaultPageSize   = 50
	maximumPageSize   = 200
	maximumIdentifier = 512
)

type InventoryQuery struct {
	Q             string
	SourceType    string
	PlayerUID     string
	GuildID       string
	BaseID        string
	ContainerID   string
	ContainerType string
	Sort          string
	Page          int
	PageSize      int
}

type InventoryPage struct {
	Metadata database.SnapshotMetadata     `json:"metadata"`
	Items    []database.InventoryAggregate `json:"items"`
	Page     int                           `json:"page"`
	PageSize int                           `json:"page_size"`
	Total    int                           `json:"total"`
}

type containerSets struct {
	players    map[string]struct{}
	bases      map[string]struct{}
	containers map[string]struct{}
}

func marshalRecord(bucket *bbolt.Bucket, key string, value any) error {
	if strings.TrimSpace(key) == "" || len(key) > maximumIdentifier {
		return fmt.Errorf("invalid snapshot record key")
	}
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return bucket.Put([]byte(key), data)
}

func PutSnapshot(db *bbolt.DB, payload database.SnapshotPayload) (database.SnapshotMetadata, error) {
	metadata, _, err := putSnapshotWithBreedingMonitor(db, payload, nil, time.Now().UTC())
	return metadata, err
}

func PutSnapshotWithBreedingMonitor(db *bbolt.DB, payload database.SnapshotPayload, monitor *BreedingMonitorOptions, now time.Time) (database.SnapshotMetadata, error) {
	metadata, _, err := putSnapshotWithBreedingMonitor(db, payload, monitor, now)
	return metadata, err
}

func PutSnapshotWithBreedingMonitorEvents(db *bbolt.DB, payload database.SnapshotPayload, monitor *BreedingMonitorOptions, now time.Time) (database.SnapshotMetadata, []database.BreedingFarmEvent, error) {
	return putSnapshotWithBreedingMonitor(db, payload, monitor, now)
}

func putSnapshotWithBreedingMonitor(db *bbolt.DB, payload database.SnapshotPayload, monitor *BreedingMonitorOptions, now time.Time) (database.SnapshotMetadata, []database.BreedingFarmEvent, error) {
	snapshotID := uuid.NewString()
	createdEvents := make([]database.BreedingFarmEvent, 0)
	payload.Metadata.SnapshotID = snapshotID
	payload.Metadata.BaseCampCount = len(payload.BaseCamps)
	payload.Metadata.WorkPalCount = len(payload.WorkPals)
	payload.Metadata.ContainerCount = len(payload.Containers)
	payload.Metadata.InventorySlots = len(payload.InventorySlots)
	payload.Metadata.BreedingFarmCount = len(payload.BreedingFarms)
	if payload.Metadata.SnapshotTime.IsZero() {
		payload.Metadata.SnapshotTime = time.Now().UTC()
	}
	if payload.Metadata.Capabilities == nil {
		payload.Metadata.Capabilities = map[string]string{}
	}
	if payload.Metadata.Warnings == nil {
		payload.Metadata.Warnings = []string{}
	}

	err := db.Update(func(tx *bbolt.Tx) error {
		root, err := tx.CreateBucketIfNotExists(snapshotsBucket)
		if err != nil {
			return err
		}
		snapshot, err := root.CreateBucket([]byte(snapshotID))
		if err != nil {
			return err
		}
		for _, name := range append([][]byte{basesBucket, workersBucket, containersBucket, inventoryBucket, breedingFarmsBucket, breedingParentsBucket, breedingCakesBucket, breedingEggsBucket, breedingFarmBaseIndex, breedingFarmGuildIndex}, indexNames...) {
			if _, err := snapshot.CreateBucket(name); err != nil {
				return err
			}
		}
		metadata, err := json.Marshal(payload.Metadata)
		if err != nil {
			return err
		}
		if err := snapshot.Put(metadataKey, metadata); err != nil {
			return err
		}
		capabilities, err := json.Marshal(payload.BreedingCapabilities)
		if err != nil {
			return err
		}
		if err := snapshot.Put(breedingCapabilitiesKey, capabilities); err != nil {
			return err
		}
		for _, base := range payload.BaseCamps {
			if err := marshalRecord(snapshot.Bucket(basesBucket), base.BaseID, base); err != nil {
				return fmt.Errorf("base camp: %w", err)
			}
		}
		for _, worker := range payload.WorkPals {
			key := worker.BaseID + "\x00" + worker.InstanceID
			if err := marshalRecord(snapshot.Bucket(workersBucket), key, worker); err != nil {
				return fmt.Errorf("work pal: %w", err)
			}
		}
		for _, container := range payload.Containers {
			if err := marshalRecord(snapshot.Bucket(containersBucket), container.ContainerID, container); err != nil {
				return fmt.Errorf("container: %w", err)
			}
		}
		for _, farm := range payload.BreedingFarms {
			farm.SnapshotID = snapshotID
			if farm.CreatedAt.IsZero() {
				farm.CreatedAt = payload.Metadata.SnapshotTime
			}
			if farm.Warnings == nil {
				farm.Warnings = []string{}
			}
			if err := marshalRecord(snapshot.Bucket(breedingFarmsBucket), farm.FarmID, farm); err != nil {
				return fmt.Errorf("breeding farm: %w", err)
			}
			if err := snapshot.Bucket(breedingFarmBaseIndex).Put([]byte(farm.BaseID+"\x00"+farm.FarmID), nil); err != nil {
				return err
			}
			if err := snapshot.Bucket(breedingFarmGuildIndex).Put([]byte(farm.GuildID+"\x00"+farm.FarmID), nil); err != nil {
				return err
			}
		}
		for _, parent := range payload.BreedingParents {
			parent.SnapshotID = snapshotID
			key := fmt.Sprintf("%s\x00%04d", parent.FarmID, parent.SlotIndex)
			if err := marshalRecord(snapshot.Bucket(breedingParentsBucket), key, parent); err != nil {
				return fmt.Errorf("breeding parent: %w", err)
			}
		}
		for _, cake := range payload.BreedingCakes {
			cake.SnapshotID = snapshotID
			if cake.Slots == nil {
				cake.Slots = []database.BreedingFarmCakeSlot{}
			}
			if cake.Warnings == nil {
				cake.Warnings = []string{}
			}
			if err := marshalRecord(snapshot.Bucket(breedingCakesBucket), cake.FarmID, cake); err != nil {
				return fmt.Errorf("breeding cake container: %w", err)
			}
		}
		for index, egg := range payload.BreedingEggs {
			egg.SnapshotID = snapshotID
			key := egg.FarmID + "\x00" + egg.EggInstanceID
			if egg.EggInstanceID == "" {
				key = fmt.Sprintf("%s\x00type:%s:%04d", egg.FarmID, nullableString(egg.EggItemID), index)
			}
			if err := marshalRecord(snapshot.Bucket(breedingEggsBucket), key, egg); err != nil {
				return fmt.Errorf("breeding egg: %w", err)
			}
		}
		seenLocations := make(map[string]struct{}, len(payload.InventorySlots))
		for _, location := range payload.InventorySlots {
			expectedID := fmt.Sprintf("%s:%d", location.ContainerID, location.SlotIndex)
			if location.LocationID == "" {
				location.LocationID = expectedID
			}
			if location.LocationID != expectedID || location.Count <= 0 || strings.TrimSpace(location.ItemID) == "" || strings.EqualFold(location.ItemID, "none") {
				continue
			}
			if _, exists := seenLocations[location.LocationID]; exists {
				continue
			}
			seenLocations[location.LocationID] = struct{}{}
			if err := marshalRecord(snapshot.Bucket(inventoryBucket), location.LocationID, location); err != nil {
				return fmt.Errorf("inventory slot: %w", err)
			}
			indexValues := []string{location.ItemID, location.SourceType, location.PlayerUID, location.GuildID, location.BaseID, location.ContainerID}
			for i, value := range indexValues {
				if value == "" {
					continue
				}
				key := value + "\x00" + location.LocationID
				if err := snapshot.Bucket(indexNames[i]).Put([]byte(key), nil); err != nil {
					return err
				}
			}
		}
		payload.Metadata.InventorySlots = len(seenLocations)
		metadata, err = json.Marshal(payload.Metadata)
		if err != nil {
			return err
		}
		if err := snapshot.Put(metadataKey, metadata); err != nil {
			return err
		}
		if monitor != nil {
			if err := processBreedingSnapshotTx(tx, snapshot, payload, snapshotID, *monitor, now, &createdEvents); err != nil {
				return err
			}
		}
		state, err := tx.CreateBucketIfNotExists(snapshotStateBucket)
		if err != nil {
			return err
		}
		return state.Put(activeSnapshotKey, []byte(snapshotID))
	})
	if err != nil {
		return database.SnapshotMetadata{}, nil, err
	}
	// Cleanup happens in a separate transaction after the active pointer is
	// durable, so readers can never observe a partially written snapshot.
	_ = cleanupOldSnapshots(db, snapshotID)
	return payload.Metadata, createdEvents, nil
}

func cleanupOldSnapshots(db *bbolt.DB, active string) error {
	return db.Update(func(tx *bbolt.Tx) error {
		root := tx.Bucket(snapshotsBucket)
		if root == nil {
			return nil
		}
		var old [][]byte
		if err := root.ForEach(func(key, value []byte) error {
			if value == nil && string(key) != active {
				old = append(old, append([]byte(nil), key...))
			}
			return nil
		}); err != nil {
			return err
		}
		for _, key := range old {
			if err := root.DeleteBucket(key); err != nil {
				return err
			}
		}
		return nil
	})
}

func activeSnapshot(tx *bbolt.Tx) (*bbolt.Bucket, error) {
	state := tx.Bucket(snapshotStateBucket)
	root := tx.Bucket(snapshotsBucket)
	if state == nil || root == nil {
		return nil, ErrSnapshotUnavailable
	}
	id := state.Get(activeSnapshotKey)
	if len(id) == 0 {
		return nil, ErrSnapshotUnavailable
	}
	snapshot := root.Bucket(id)
	if snapshot == nil {
		return nil, ErrSnapshotUnavailable
	}
	return snapshot, nil
}

func snapshotMetadata(snapshot *bbolt.Bucket) (database.SnapshotMetadata, error) {
	var metadata database.SnapshotMetadata
	data := snapshot.Get(metadataKey)
	if data == nil {
		return metadata, ErrSnapshotUnavailable
	}
	if err := json.Unmarshal(data, &metadata); err != nil {
		return metadata, err
	}
	return metadata, nil
}

func SnapshotMetadata(db *bbolt.DB) (database.SnapshotMetadata, error) {
	var metadata database.SnapshotMetadata
	err := db.View(func(tx *bbolt.Tx) error {
		snapshot, err := activeSnapshot(tx)
		if err != nil {
			return err
		}
		metadata, err = snapshotMetadata(snapshot)
		return err
	})
	return metadata, err
}

func listBucket[T any](bucket *bbolt.Bucket) ([]T, error) {
	result := make([]T, 0)
	if bucket == nil {
		return result, nil
	}
	err := bucket.ForEach(func(_, value []byte) error {
		if value == nil {
			return nil
		}
		var record T
		if err := json.Unmarshal(value, &record); err != nil {
			return err
		}
		result = append(result, record)
		return nil
	})
	return result, err
}

func ListBaseCamps(db *bbolt.DB) ([]database.BaseCampSnapshot, database.SnapshotMetadata, error) {
	var bases []database.BaseCampSnapshot
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
		bases, err = listBucket[database.BaseCampSnapshot](snapshot.Bucket(basesBucket))
		if err != nil {
			return err
		}
		for index := range bases {
			if err := decorateBaseTx(tx, &bases[index]); err != nil {
				return err
			}
		}
		return nil
	})
	sort.Slice(bases, func(i, j int) bool { return bases[i].DisplayName < bases[j].DisplayName })
	return bases, metadata, err
}

func GetBaseCamp(db *bbolt.DB, baseID string) (database.BaseCampSnapshot, database.SnapshotMetadata, error) {
	var base database.BaseCampSnapshot
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
		data := snapshot.Bucket(basesBucket).Get([]byte(baseID))
		if data == nil {
			return ErrNoRecord
		}
		if err := json.Unmarshal(data, &base); err != nil {
			return err
		}
		return decorateBaseTx(tx, &base)
	})
	return base, metadata, err
}

func ListBaseWorkers(db *bbolt.DB, baseID string) ([]database.BaseWorkerPal, database.SnapshotMetadata, error) {
	workers := make([]database.BaseWorkerPal, 0)
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
		cursor := snapshot.Bucket(workersBucket).Cursor()
		prefix := []byte(baseID + "\x00")
		for key, value := cursor.Seek(prefix); key != nil && strings.HasPrefix(string(key), string(prefix)); key, value = cursor.Next() {
			var worker database.BaseWorkerPal
			if err := json.Unmarshal(value, &worker); err != nil {
				return err
			}
			workers = append(workers, worker)
		}
		for index := range workers {
			_, displayName, err := resolveBaseDisplayNameTx(tx, workers[index].BaseID, workers[index].BaseName)
			if err != nil {
				return err
			}
			workers[index].BaseDisplayName = displayName
		}
		return nil
	})
	return workers, metadata, err
}

func ListContainers(db *bbolt.DB, query InventoryQuery) ([]database.ItemContainer, database.SnapshotMetadata, error) {
	var containers []database.ItemContainer
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
		containers, err = listBucket[database.ItemContainer](snapshot.Bucket(containersBucket))
		if err != nil {
			return err
		}
		for index := range containers {
			if containers[index].BaseID == "" {
				continue
			}
			_, displayName, err := resolveBaseDisplayNameTx(tx, containers[index].BaseID, containers[index].BaseName)
			if err != nil {
				return err
			}
			containers[index].BaseDisplayName = displayName
		}
		return nil
	})
	filtered := containers[:0]
	for _, container := range containers {
		if query.SourceType != "" && container.SourceType != query.SourceType ||
			query.PlayerUID != "" && container.PlayerUID != query.PlayerUID ||
			query.GuildID != "" && container.GuildID != query.GuildID ||
			query.BaseID != "" && container.BaseID != query.BaseID ||
			query.ContainerType != "" && container.ContainerType != query.ContainerType {
			continue
		}
		filtered = append(filtered, container)
	}
	sort.Slice(filtered, func(i, j int) bool { return filtered[i].ContainerName < filtered[j].ContainerName })
	return filtered, metadata, err
}

func matchesInventory(location database.InventoryLocation, query InventoryQuery) bool {
	if query.SourceType != "" && location.SourceType != query.SourceType ||
		query.PlayerUID != "" && location.PlayerUID != query.PlayerUID ||
		query.GuildID != "" && location.GuildID != query.GuildID ||
		query.BaseID != "" && location.BaseID != query.BaseID ||
		query.ContainerID != "" && location.ContainerID != query.ContainerID ||
		query.ContainerType != "" && location.ContainerType != query.ContainerType {
		return false
	}
	if query.Q == "" {
		return true
	}
	q := strings.ToLower(query.Q)
	searchable := []string{location.ItemID, location.ItemName, location.PlayerName, location.GuildName, location.BaseID, location.BaseName, location.BaseDisplayName, location.ContainerID, location.ContainerName}
	for _, value := range searchable {
		if strings.Contains(strings.ToLower(value), q) {
			return true
		}
	}
	return false
}

func filteredLocations(db *bbolt.DB, query InventoryQuery, itemID string) ([]database.InventoryLocation, database.SnapshotMetadata, error) {
	locations := make([]database.InventoryLocation, 0)
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
		return snapshot.Bucket(inventoryBucket).ForEach(func(_, value []byte) error {
			var location database.InventoryLocation
			if err := json.Unmarshal(value, &location); err != nil {
				return err
			}
			if itemID != "" && !strings.EqualFold(location.ItemID, itemID) {
				return nil
			}
			if location.BaseID != "" {
				_, displayName, err := resolveBaseDisplayNameTx(tx, location.BaseID, location.BaseName)
				if err != nil {
					return err
				}
				location.BaseDisplayName = displayName
			}
			if matchesInventory(location, query) {
				locations = append(locations, location)
			}
			return nil
		})
	})
	return locations, metadata, err
}

func InventorySummary(db *bbolt.DB, query InventoryQuery) (InventoryPage, error) {
	locations, metadata, err := filteredLocations(db, query, "")
	if err != nil {
		return InventoryPage{}, err
	}
	aggregates := make(map[string]*database.InventoryAggregate)
	sets := make(map[string]*containerSets)
	for _, location := range locations {
		aggregate := aggregates[location.ItemID]
		if aggregate == nil {
			aggregate = &database.InventoryAggregate{ItemID: location.ItemID, ItemName: location.ItemName}
			aggregates[location.ItemID] = aggregate
			sets[location.ItemID] = &containerSets{map[string]struct{}{}, map[string]struct{}{}, map[string]struct{}{}}
		}
		aggregate.TotalCount += location.Count
		aggregate.LocationCount++
		if strings.HasPrefix(location.SourceType, "player_") {
			aggregate.PlayerTotal += location.Count
		} else {
			aggregate.BaseTotal += location.Count
		}
		if location.PlayerUID != "" {
			sets[location.ItemID].players[location.PlayerUID] = struct{}{}
		}
		if location.BaseID != "" {
			sets[location.ItemID].bases[location.BaseID] = struct{}{}
		}
		sets[location.ItemID].containers[location.ContainerID] = struct{}{}
	}
	items := make([]database.InventoryAggregate, 0, len(aggregates))
	for itemID, aggregate := range aggregates {
		aggregate.PlayerCount = len(sets[itemID].players)
		aggregate.BaseCount = len(sets[itemID].bases)
		aggregate.ContainerCount = len(sets[itemID].containers)
		items = append(items, *aggregate)
	}
	sort.Slice(items, func(i, j int) bool {
		switch query.Sort {
		case "count_asc":
			return items[i].TotalCount < items[j].TotalCount
		case "name":
			return items[i].ItemName < items[j].ItemName
		case "locations":
			return items[i].LocationCount > items[j].LocationCount
		default:
			return items[i].TotalCount > items[j].TotalCount
		}
	})
	page, pageSize := normalizePage(query.Page, query.PageSize)
	start := (page - 1) * pageSize
	if start > len(items) {
		start = len(items)
	}
	end := start + pageSize
	if end > len(items) {
		end = len(items)
	}
	return InventoryPage{Metadata: metadata, Items: items[start:end], Page: page, PageSize: pageSize, Total: len(items)}, nil
}

func normalizePage(page, pageSize int) (int, int) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = defaultPageSize
	}
	if pageSize > maximumPageSize {
		pageSize = maximumPageSize
	}
	return page, pageSize
}

func InventoryLocations(db *bbolt.DB, itemID string, query InventoryQuery) ([]database.InventoryLocation, database.SnapshotMetadata, int, error) {
	locations, metadata, err := filteredLocations(db, query, itemID)
	if err != nil {
		return nil, metadata, 0, err
	}
	sort.Slice(locations, func(i, j int) bool {
		if locations[i].Count == locations[j].Count {
			return locations[i].LocationID < locations[j].LocationID
		}
		return locations[i].Count > locations[j].Count
	})
	total := len(locations)
	page, pageSize := normalizePage(query.Page, query.PageSize)
	start := (page - 1) * pageSize
	if start > total {
		start = total
	}
	end := start + pageSize
	if end > total {
		end = total
	}
	return locations[start:end], metadata, total, nil
}

func FeedBoxes(db *bbolt.DB, baseID string) ([]database.FeedBox, database.SnapshotMetadata, error) {
	containers, metadata, err := ListContainers(db, InventoryQuery{BaseID: baseID, SourceType: "base_feed_box"})
	if err != nil {
		return nil, metadata, err
	}
	result := make([]database.FeedBox, 0, len(containers))
	for _, container := range containers {
		locations, _, _, err := InventoryLocations(db, "", InventoryQuery{BaseID: baseID, ContainerID: container.ContainerID, PageSize: maximumPageSize})
		if err != nil {
			return nil, metadata, err
		}
		box := database.FeedBox{ContainerID: container.ContainerID, BaseID: baseID, ContainerType: "feed_box", DisplayName: "饲料箱", Slots: []database.InventoryLocation{}}
		for _, location := range locations {
			box.Slots = append(box.Slots, location)
			box.TotalCount += location.Count
		}
		result = append(result, box)
	}
	return result, metadata, nil
}

func BaseCampOverviews(db *bbolt.DB, maxWorkers int) ([]database.BaseCampOverview, database.SnapshotMetadata, error) {
	bases, metadata, err := ListBaseCamps(db)
	if err != nil {
		return nil, metadata, err
	}
	result := make([]database.BaseCampOverview, 0, len(bases))
	for _, base := range bases {
		workers, _, err := ListBaseWorkers(db, base.BaseID)
		if err != nil {
			return nil, metadata, err
		}
		feedBoxes, _, err := FeedBoxes(db, base.BaseID)
		if err != nil {
			return nil, metadata, err
		}
		overview := database.BaseCampOverview{BaseCampSnapshot: base, SnapshotTime: metadata.SnapshotTime, SaveFileTime: metadata.SaveFileTime, IsStale: metadata.IsStale, MaxWorkerPals: maxWorkers, WorkerPalCount: len(workers), FeedBoxCount: len(feedBoxes)}
		feedTypes := make(map[string]struct{})
		for _, worker := range workers {
			abnormal := len(worker.StatusAbnormalities) > 0 || worker.IsDown != nil && *worker.IsDown || worker.IsSick != nil && *worker.IsSick || worker.IsInjured != nil && *worker.IsInjured
			if !abnormal {
				overview.HealthyPalCount++
			}
			if worker.FullStomach != nil && *worker.FullStomach < LowFullStomach {
				overview.HungryPalCount++
			}
			if worker.Sanity != nil && *worker.Sanity < LowSanity {
				overview.LowSanityPalCount++
			}
			if worker.IsSick != nil && *worker.IsSick || worker.IsInjured != nil && *worker.IsInjured {
				overview.SickPalCount++
			}
			if worker.IsDown != nil && *worker.IsDown {
				overview.DownPalCount++
			}
		}
		for _, box := range feedBoxes {
			for _, slot := range box.Slots {
				feedTypes[slot.ItemID] = struct{}{}
				overview.FeedTotalItemCount += slot.Count
			}
		}
		overview.FeedItemTypeCount = len(feedTypes)
		result = append(result, overview)
	}
	return result, metadata, nil
}
