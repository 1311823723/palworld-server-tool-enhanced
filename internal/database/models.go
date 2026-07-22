package database

import "time"

type SnapshotMetadata struct {
	SnapshotID          string            `json:"snapshot_id"`
	SnapshotTime        time.Time         `json:"snapshot_time"`
	SaveFileTime        time.Time         `json:"save_file_time"`
	IsStale             bool              `json:"is_stale"`
	Warnings            []string          `json:"warnings"`
	Capabilities        map[string]string `json:"capabilities"`
	BaseCampCount       int               `json:"base_camp_count"`
	WorkPalCount        int               `json:"work_pal_count"`
	ContainerCount      int               `json:"container_count"`
	InventorySlots      int               `json:"inventory_slot_count"`
	BreedingFarmCount   int               `json:"breeding_farm_count"`
	SyncIntervalSeconds int               `json:"sync_interval_seconds,omitempty"`
}

type SnapshotPayload struct {
	Metadata             SnapshotMetadata            `json:"metadata"`
	BaseCamps            []BaseCampSnapshot          `json:"base_camps"`
	WorkPals             []BaseWorkerPal             `json:"work_pals"`
	Containers           []ItemContainer             `json:"containers"`
	InventorySlots       []InventoryLocation         `json:"inventory_slots"`
	BreedingFarms        []BreedingFarmSnapshot      `json:"breeding_farms"`
	BreedingParents      []BreedingFarmParent        `json:"breeding_parents"`
	BreedingCakes        []BreedingFarmCakeContainer `json:"breeding_cakes"`
	BreedingEggs         []BreedingFarmEgg           `json:"breeding_eggs"`
	BreedingCapabilities BreedingFarmCapabilities    `json:"breeding_capabilities"`
}

type BreedingFarmCapabilities struct {
	FarmDetection        bool   `json:"farm_detection"`
	BaseAssociation      bool   `json:"base_association"`
	ParentSlots          bool   `json:"parent_slots"`
	CakeContainer        bool   `json:"cake_container"`
	EggDetection         bool   `json:"egg_detection"`
	EggIdentity          bool   `json:"egg_identity"`
	EggType              bool   `json:"egg_type"`
	BreedingProgress     bool   `json:"breeding_progress"`
	ValidatedGameVersion string `json:"validated_game_version"`
}

type BreedingFarmSnapshot struct {
	SnapshotID           string                     `json:"snapshot_id,omitempty"`
	FarmID               string                     `json:"farm_id"`
	BaseID               string                     `json:"base_id"`
	BaseName             string                     `json:"base_name"`
	BaseDisplayName      string                     `json:"base_display_name,omitempty"`
	GuildID              string                     `json:"guild_id"`
	GuildName            string                     `json:"guild_name"`
	MapObjectInstanceID  string                     `json:"map_object_instance_id"`
	Location             SnapshotLocation           `json:"location"`
	Status               string                     `json:"status"`
	Progress             *float64                   `json:"progress"`
	CakeCount            *int64                     `json:"cake_count"`
	EggCount             *int64                     `json:"egg_count"`
	Confidence           string                     `json:"confidence"`
	AssociationVerified  bool                       `json:"association_verified"`
	ParsingComplete      bool                       `json:"parsing_complete"`
	GameVersionSupported bool                       `json:"game_version_supported"`
	IdentitySupported    bool                       `json:"identity_supported"`
	Warnings             []string                   `json:"warnings"`
	LastEggAt            *time.Time                 `json:"last_egg_at"`
	CreatedAt            time.Time                  `json:"created_at"`
	Parents              []BreedingFarmParent       `json:"parents,omitempty"`
	Cake                 *BreedingFarmCakeContainer `json:"cake,omitempty"`
	Eggs                 []BreedingFarmEgg          `json:"eggs,omitempty"`
}

type BreedingFarmParent struct {
	SnapshotID         string   `json:"snapshot_id,omitempty"`
	FarmID             string   `json:"farm_id"`
	SlotIndex          int      `json:"slot_index"`
	PalInstanceID      string   `json:"pal_instance_id"`
	PalID              string   `json:"pal_id"`
	PalName            string   `json:"pal_name"`
	Nickname           string   `json:"nickname"`
	Gender             *string  `json:"gender"`
	Level              int32    `json:"level"`
	HP                 *int64   `json:"hp"`
	MaxHP              *int64   `json:"max_hp"`
	Sanity             *float64 `json:"san"`
	OwnerPlayerName    string   `json:"owner_player_name,omitempty"`
	AssignmentVerified bool     `json:"assignment_verified"`
}

type BreedingFarmCakeSlot struct {
	SlotIndex int    `json:"slot_index"`
	ItemID    string `json:"item_id"`
	Count     int64  `json:"count"`
}

type BreedingFarmCakeContainer struct {
	SnapshotID  string                 `json:"snapshot_id,omitempty"`
	FarmID      string                 `json:"farm_id"`
	ContainerID string                 `json:"container_id"`
	CakeItemID  *string                `json:"cake_item_id"`
	CakeCount   *int64                 `json:"cake_count"`
	Slots       []BreedingFarmCakeSlot `json:"slots"`
	Verified    bool                   `json:"verified"`
	Warnings    []string               `json:"warnings"`
}

type BreedingFarmEgg struct {
	SnapshotID          string  `json:"snapshot_id,omitempty"`
	FarmID              string  `json:"farm_id"`
	EggInstanceID       string  `json:"egg_instance_id,omitempty"`
	EggItemID           *string `json:"egg_item_id"`
	EggName             string  `json:"egg_name"`
	Count               int64   `json:"count"`
	SlotIndex           *int    `json:"slot_index"`
	Ready               bool    `json:"ready"`
	AssociationVerified bool    `json:"association_verified"`
}

type BreedingFarmEvent struct {
	EventID         string    `json:"event_id"`
	FarmID          string    `json:"farm_id"`
	BaseID          string    `json:"base_id"`
	BaseName        string    `json:"base_name"`
	BaseDisplayName string    `json:"base_display_name,omitempty"`
	GuildID         string    `json:"guild_id"`
	EventType       string    `json:"event_type"`
	DedupKey        string    `json:"dedup_key,omitempty"`
	PreviousCount   int64     `json:"previous_count"`
	CurrentCount    int64     `json:"current_count"`
	EggInstanceID   string    `json:"egg_instance_id,omitempty"`
	EggItemID       *string   `json:"egg_item_id"`
	SnapshotID      string    `json:"snapshot_id"`
	Read            bool      `json:"read"`
	CreatedAt       time.Time `json:"created_at"`
}

type SnapshotLocation struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
	Z float64 `json:"z"`
}

type BaseCampSnapshot struct {
	BaseID                 string           `json:"base_id"`
	BaseName               string           `json:"base_name"`
	CustomName             string           `json:"custom_name,omitempty"`
	DisplayName            string           `json:"display_name"`
	GuildID                string           `json:"guild_id"`
	GuildName              string           `json:"guild_name"`
	BaseCampLevel          int32            `json:"base_camp_level"`
	Location               SnapshotLocation `json:"location"`
	AreaRange              float64          `json:"area_range"`
	WorkerContainerID      string           `json:"worker_container_id"`
	ContainerDataAvailable bool             `json:"container_data_available"`
}

type BaseWorkerPal struct {
	InstanceID          string          `json:"instance_id"`
	PalID               string          `json:"pal_id"`
	Nickname            string          `json:"nickname"`
	Level               int32           `json:"level"`
	Gender              *string         `json:"gender"`
	HP                  *int64          `json:"hp"`
	MaxHP               *int64          `json:"max_hp"`
	FullStomach         *float64        `json:"full_stomach"`
	Sanity              *float64        `json:"sanity"`
	IsSleeping          *bool           `json:"is_sleeping"`
	IsDown              *bool           `json:"is_down"`
	IsInjured           *bool           `json:"is_injured"`
	IsSick              *bool           `json:"is_sick"`
	StatusAbnormalities []string        `json:"status_abnormalities"`
	CurrentWork         *string         `json:"current_work"`
	CurrentWorkTarget   *string         `json:"current_work_target"`
	WorkSuitability     *string         `json:"work_suitability"`
	WorkSpeed           int32           `json:"work_speed"`
	OwnerPlayerUID      string          `json:"owner_player_uid"`
	OwnerPlayerName     string          `json:"owner_player_name"`
	BaseID              string          `json:"base_id"`
	BaseName            string          `json:"base_name"`
	BaseDisplayName     string          `json:"base_display_name,omitempty"`
	GuildID             string          `json:"guild_id"`
	GuildName           string          `json:"guild_name"`
	DataAvailability    map[string]bool `json:"data_availability"`
}

type ItemContainer struct {
	ContainerID     string `json:"container_id"`
	SourceType      string `json:"source_type"`
	ContainerType   string `json:"container_type"`
	ContainerName   string `json:"container_name"`
	PlayerUID       string `json:"player_uid,omitempty"`
	PlayerName      string `json:"player_name,omitempty"`
	GuildID         string `json:"guild_id,omitempty"`
	GuildName       string `json:"guild_name,omitempty"`
	BaseID          string `json:"base_id,omitempty"`
	BaseName        string `json:"base_name,omitempty"`
	BaseDisplayName string `json:"base_display_name,omitempty"`
	Parsed          bool   `json:"parsed"`
}

type InventoryLocation struct {
	LocationID           string `json:"location_id"`
	ItemID               string `json:"item_id"`
	ItemName             string `json:"item_name"`
	Count                int64  `json:"count"`
	SlotIndex            int    `json:"slot_index"`
	SourceType           string `json:"source_type"`
	PlayerUID            string `json:"player_uid,omitempty"`
	PlayerName           string `json:"player_name,omitempty"`
	GuildID              string `json:"guild_id,omitempty"`
	GuildName            string `json:"guild_name,omitempty"`
	BaseID               string `json:"base_id,omitempty"`
	BaseName             string `json:"base_name,omitempty"`
	BaseDisplayName      string `json:"base_display_name,omitempty"`
	ContainerID          string `json:"container_id"`
	ContainerType        string `json:"container_type"`
	ContainerName        string `json:"container_name"`
	SpoilRemainingSecond *int64 `json:"spoil_remaining_seconds"`
}

type InventoryAggregate struct {
	ItemID         string `json:"item_id"`
	ItemName       string `json:"item_name"`
	TotalCount     int64  `json:"total_count"`
	PlayerTotal    int64  `json:"player_total"`
	BaseTotal      int64  `json:"base_total"`
	PlayerCount    int    `json:"player_count"`
	BaseCount      int    `json:"base_count"`
	ContainerCount int    `json:"container_count"`
	LocationCount  int    `json:"location_count"`
}

type BaseCampOverview struct {
	BaseCampSnapshot
	SnapshotTime       time.Time `json:"snapshot_time"`
	SaveFileTime       time.Time `json:"save_file_time"`
	IsStale            bool      `json:"is_stale"`
	MaxWorkerPals      int       `json:"max_worker_pals"`
	WorkerPalCount     int       `json:"worker_pal_count"`
	HealthyPalCount    int       `json:"healthy_pal_count"`
	HungryPalCount     int       `json:"hungry_pal_count"`
	LowSanityPalCount  int       `json:"low_sanity_pal_count"`
	SickPalCount       int       `json:"sick_pal_count"`
	DownPalCount       int       `json:"down_pal_count"`
	FeedBoxCount       int       `json:"feed_box_count"`
	FeedItemTypeCount  int       `json:"feed_item_type_count"`
	FeedTotalItemCount int64     `json:"feed_total_item_count"`
}

type BaseAlias struct {
	BaseID    string    `json:"base_id"`
	Name      string    `json:"name"`
	UpdatedAt time.Time `json:"updated_at"`
}

type BaseAliasOverview struct {
	BaseAlias
	Active      bool   `json:"active"`
	BaseName    string `json:"base_name,omitempty"`
	DisplayName string `json:"display_name"`
}

type FeedBox struct {
	ContainerID   string              `json:"container_id"`
	BaseID        string              `json:"base_id"`
	ContainerType string              `json:"container_type"`
	DisplayName   string              `json:"display_name"`
	Slots         []InventoryLocation `json:"slots"`
	TotalCount    int64               `json:"total_item_count"`
}

type Pal struct {
	Level          int32    `json:"level"`
	Exp            int64    `json:"exp"`
	Hp             int64    `json:"hp"`
	MaxHp          int64    `json:"max_hp"`
	Type           string   `json:"type"`
	Gender         string   `json:"gender"`
	Nickname       string   `json:"nickname"`
	IsLucky        bool     `json:"is_lucky"`
	IsBoss         bool     `json:"is_boss"`
	IsTower        bool     `json:"is_tower"`
	Workspeed      int32    `json:"workspeed"`
	Melee          int32    `json:"melee"`
	Ranged         int32    `json:"ranged"`
	Defense        int32    `json:"defense"`
	Rank           int32    `json:"rank"`
	RankAttack     int32    `json:"rank_attack"`
	RankDefence    int32    `json:"rank_defence"`
	RankCraftspeed int32    `json:"rank_craftspeed"`
	Skills         []string `json:"skills"`
}

type OnlinePlayer struct {
	PlayerUid             string    `json:"player_uid"`
	UserId                string    `json:"user_id"`
	SteamId               string    `json:"steam_id"`
	Nickname              string    `json:"nickname"`
	AccountName           string    `json:"account_name"`
	Ip                    string    `json:"ip"`
	Ping                  float64   `json:"ping"`
	LocationX             float64   `json:"location_x"`
	LocationY             float64   `json:"location_y"`
	Level                 int32     `json:"level"`
	BuildingCount         int32     `json:"building_count"`
	LastOnline            time.Time `json:"last_online"`
	IsOnline              bool      `json:"is_online"`
	OnlineSince           time.Time `json:"online_since"`
	OnlineLastSeenAt      time.Time `json:"online_last_seen_at"`
	CurrentSessionSeconds int64     `json:"current_session_seconds"`
	TotalOnlineSeconds    int64     `json:"total_online_seconds"`
}

type GuildPlayer struct {
	PlayerUid string `json:"player_uid"`
	Nickname  string `json:"nickname"`
}

type TersePlayer struct {
	PlayerUid      string           `json:"player_uid"`
	Nickname       string           `json:"nickname"`
	Level          int32            `json:"level"`
	Exp            int64            `json:"exp"`
	Hp             int64            `json:"hp"`
	MaxHp          int64            `json:"max_hp"`
	ShieldHp       int64            `json:"shield_hp"`
	ShieldMaxHp    int64            `json:"shield_max_hp"`
	MaxStatusPoint int32            `json:"max_status_point"`
	StatusPoint    map[string]int32 `json:"status_point"`
	FullStomach    float64          `json:"full_stomach"`
	SaveLastOnline string           `json:"save_last_online"`
	OnlinePlayer
}

type Player struct {
	TersePlayer
	Pals  []*Pal `json:"pals"`
	Items *Items `json:"items"`
}

type BaseCamp struct {
	Id        string  `json:"id"`
	Area      float64 `json:"area"`
	LocationX float64 `json:"location_x"`
	LocationY float64 `json:"location_y"`
}

type Guild struct {
	Name           string         `json:"name"`
	BaseCampLevel  int32          `json:"base_camp_level"`
	AdminPlayerUid string         `json:"admin_player_uid"`
	Players        []*GuildPlayer `json:"players"`
	BaseCamp       []BaseCamp     `json:"base_camp"`
}

type PlayerW struct {
	Name      string `json:"name"`
	SteamID   string `json:"steam_id"`
	PlayerUID string `json:"player_uid"`
}

type RconCommand struct {
	Command     string `json:"command"`
	Placeholder string `json:"placeholder"`
	Remark      string `json:"remark"`
}

type RconCommandList struct {
	UUID string `json:"uuid"`
	RconCommand
}

type RconTask struct {
	UUID       string     `json:"uuid"`
	Name       string     `json:"name"`
	RconUUID   string     `json:"rcon_uuid"`
	Content    string     `json:"content"`
	Cron       string     `json:"cron"`
	Enabled    bool       `json:"enabled"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
	LastRunAt  *time.Time `json:"last_run_at,omitempty"`
	LastStatus string     `json:"last_status"`
	LastResult string     `json:"last_result"`
	LastError  string     `json:"last_error"`
	RunCount   int64      `json:"run_count"`
}

type Items struct {
	CommonContainerId           []*Item `json:"CommonContainerId"`
	DropSlotContainerId         []*Item `json:"DropSlotContainerId"`
	EssentialContainerId        []*Item `json:"EssentialContainerId"`
	FoodEquipContainerId        []*Item `json:"FoodEquipContainerId"`
	PlayerEquipArmorContainerId []*Item `json:"PlayerEquipArmorContainerId"`
	WeaponLoadOutContainerId    []*Item `json:"WeaponLoadOutContainerId"`
}

type Item struct {
	SlotIndex  int32  `json:"SlotIndex"`
	ItemId     string `json:"ItemId"`
	StackCount int32  `json:"StackCount"`
}

type Backup struct {
	BackupId string    `json:"backup_id"`
	SaveTime time.Time `json:"save_time"`
	Path     string    `json:"path"`
}
