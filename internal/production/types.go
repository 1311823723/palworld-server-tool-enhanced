package production

import "time"

const (
	BridgeName            = "PSTProductionBridge"
	BridgeVersion         = "0.1.1"
	BridgeProtocolVersion = 1
)

type BridgeState string

const (
	BridgeUnsupported       BridgeState = "unsupported"
	BridgeUnconfigured      BridgeState = "unconfigured"
	BridgeDependencyMissing BridgeState = "dependency_missing"
	BridgeNotInstalled      BridgeState = "not_installed"
	BridgeInstalling        BridgeState = "installing"
	BridgeRestartRequired   BridgeState = "restart_required"
	BridgeHealthy           BridgeState = "healthy"
	BridgeOffline           BridgeState = "offline"
	BridgeUpgradeAvailable  BridgeState = "upgrade_available"
	BridgeModified          BridgeState = "modified"
	BridgeIncompatible      BridgeState = "incompatible"
	BridgePermissionDenied  BridgeState = "permission_denied"
	BridgeError             BridgeState = "error"
)

type ManualInstallGuide struct {
	SourceDirectory     string   `json:"source_directory,omitempty"`
	TargetDirectory     string   `json:"target_directory,omitempty"`
	SettingsPath        string   `json:"settings_path,omitempty"`
	UE4SSDirectory      string   `json:"ue4ss_directory,omitempty"`
	ManagedManifestPath string   `json:"managed_manifest_path,omitempty"`
	RuntimeDirectory    string   `json:"runtime_directory,omitempty"`
	UE4SSLogPath        string   `json:"ue4ss_log_path,omitempty"`
	Steps               []string `json:"steps"`
	AutomaticInstallOK  bool     `json:"automatic_install_available"`
	AutomaticBlockCause string   `json:"automatic_install_block_reason,omitempty"`
}

type BridgeStatus struct {
	State                BridgeState         `json:"state"`
	Message              string              `json:"message"`
	InstalledVersion     string              `json:"installed_version,omitempty"`
	BundledVersion       string              `json:"bundled_version"`
	ProtocolVersion      int                 `json:"protocol_version"`
	PalworldBuild        string              `json:"palworld_build,omitempty"`
	HeartbeatAt          *time.Time          `json:"heartbeat_at,omitempty"`
	HeartbeatAgeSeconds  int64               `json:"heartbeat_age_seconds,omitempty"`
	OrdersAvailable      bool                `json:"orders_available"`
	CatalogAvailable     bool                `json:"catalog_available"`
	Installing           bool                `json:"installing"`
	InstallStage         string              `json:"install_stage,omitempty"`
	LastError            string              `json:"last_error,omitempty"`
	ExternalProcess      bool                `json:"external_process"`
	RestartRequired      bool                `json:"restart_required"`
	ManualInstall        *ManualInstallGuide `json:"manual_install,omitempty"`
	InstalledFilesIntact bool                `json:"installed_files_intact"`
}

type RuntimeCapabilities struct {
	Catalog bool `json:"catalog"`
	Orders  bool `json:"orders"`
	Cancel  bool `json:"cancel"`
}

type Material struct {
	ItemID       string `json:"item_id"`
	Name         string `json:"name,omitempty"`
	RequiredEach int64  `json:"required_each"`
	Available    int64  `json:"available"`
}

type Recipe struct {
	ID           string     `json:"id"`
	Name         string     `json:"name,omitempty"`
	ProductID    string     `json:"product_item_id"`
	ProductName  string     `json:"product_name,omitempty"`
	ProductEach  int64      `json:"product_count"`
	Unlocked     bool       `json:"unlocked"`
	MaxAvailable int64      `json:"max_available"`
	Materials    []Material `json:"materials"`
}

type Workstation struct {
	ActorGUID string   `json:"actor_guid"`
	Name      string   `json:"name,omitempty"`
	Busy      bool     `json:"busy"`
	Recipes   []Recipe `json:"recipes"`
}

type BaseCatalog struct {
	BaseID       string        `json:"base_id"`
	BaseName     string        `json:"base_name,omitempty"`
	Workstations []Workstation `json:"workstations"`
}

type RuntimeState struct {
	InstanceID      string              `json:"instance_id"`
	BridgeVersion   string              `json:"bridge_version"`
	ProtocolVersion int                 `json:"protocol_version"`
	HeartbeatAt     time.Time           `json:"heartbeat_at"`
	PalworldBuild   string              `json:"palworld_build,omitempty"`
	Compatible      bool                `json:"compatible"`
	Reason          string              `json:"reason,omitempty"`
	Capabilities    RuntimeCapabilities `json:"capabilities"`
	Bases           []BaseCatalog       `json:"bases"`
}

type QuantityMode string

const (
	QuantityExact        QuantityMode = "exact"
	QuantityMaxAvailable QuantityMode = "max_available"
)

type PreviewRequest struct {
	BaseID    string       `json:"base_id"`
	ActorGUID string       `json:"workstation_actor_guid"`
	RecipeID  string       `json:"recipe_id"`
	Mode      QuantityMode `json:"quantity_mode"`
	Quantity  int64        `json:"quantity"`
}

type MaterialPreview struct {
	ItemID       string `json:"item_id"`
	Name         string `json:"name,omitempty"`
	RequiredEach int64  `json:"required_each"`
	Required     int64  `json:"required"`
	Available    int64  `json:"available"`
	Shortage     int64  `json:"shortage"`
}

type Preview struct {
	BaseID             string            `json:"base_id"`
	ActorGUID          string            `json:"workstation_actor_guid"`
	RecipeID           string            `json:"recipe_id"`
	RequestedQuantity  int64             `json:"requested_quantity"`
	AcceptedQuantity   int64             `json:"accepted_quantity"`
	MaxAvailable       int64             `json:"max_available"`
	CanSubmit          bool              `json:"can_submit"`
	Reason             string            `json:"reason,omitempty"`
	Materials          []MaterialPreview `json:"materials"`
	SnapshotHeartbeat  time.Time         `json:"snapshot_heartbeat"`
	QuantityCalculated bool              `json:"quantity_calculated"`
}

type OrderStatus string

const (
	OrderPending          OrderStatus = "pending"
	OrderDispatching      OrderStatus = "dispatching"
	OrderAccepted         OrderStatus = "accepted"
	OrderWaitingMaterials OrderStatus = "waiting_materials"
	OrderProducing        OrderStatus = "producing"
	OrderCompleted        OrderStatus = "completed"
	OrderCancelled        OrderStatus = "cancelled"
	OrderFailed           OrderStatus = "failed"
	OrderUnknown          OrderStatus = "unknown"
)

type Order struct {
	ID                    string       `json:"order_id"`
	BaseID                string       `json:"base_id"`
	BaseName              string       `json:"base_name,omitempty"`
	ActorGUID             string       `json:"workstation_actor_guid"`
	WorkstationName       string       `json:"workstation_name,omitempty"`
	RecipeID              string       `json:"recipe_id"`
	RecipeName            string       `json:"recipe_name,omitempty"`
	ProductID             string       `json:"product_item_id,omitempty"`
	ProductName           string       `json:"product_name,omitempty"`
	QuantityMode          QuantityMode `json:"quantity_mode"`
	RequestedAmount       int64        `json:"requested_quantity"`
	AcceptedAmount        int64        `json:"accepted_quantity"`
	CompletedAmount       int64        `json:"completed_quantity"`
	Status                OrderStatus  `json:"status"`
	Error                 string       `json:"error,omitempty"`
	CreatedAt             time.Time    `json:"created_at"`
	UpdatedAt             time.Time    `json:"updated_at"`
	AcceptedAt            *time.Time   `json:"accepted_at,omitempty"`
	CompletedAt           *time.Time   `json:"completed_at,omitempty"`
	BridgeInstanceID      string       `json:"bridge_instance_id,omitempty"`
	CancellationRequested bool         `json:"cancellation_requested,omitempty"`
}

type OrderRequest struct {
	OrderID      string       `json:"order_id"`
	Key          string       `json:"key"`
	BaseID       string       `json:"base_id"`
	ActorGUID    string       `json:"workstation_actor_guid"`
	RecipeID     string       `json:"recipe_id"`
	QuantityMode QuantityMode `json:"quantity_mode"`
	Quantity     int64        `json:"quantity"`
	CreatedAt    time.Time    `json:"created_at"`
	Action       string       `json:"action"`
}

type OrderResult struct {
	OrderID           string      `json:"order_id"`
	Status            OrderStatus `json:"status"`
	AcceptedQuantity  int64       `json:"accepted_quantity"`
	CompletedQuantity int64       `json:"completed_quantity"`
	Error             string      `json:"error,omitempty"`
	BridgeInstanceID  string      `json:"bridge_instance_id,omitempty"`
	UpdatedAt         time.Time   `json:"updated_at"`
}

type InstallRequest struct {
	Confirmation        string `json:"confirmation"`
	ShutdownSeconds     int    `json:"shutdown_seconds"`
	RestartDelaySeconds int    `json:"restart_delay_seconds"`
	Message             string `json:"message"`
}
