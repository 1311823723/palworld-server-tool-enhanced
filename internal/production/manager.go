package production

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/zaigie/palworld-server-tool/internal/logger"
	"github.com/zaigie/palworld-server-tool/internal/supervisor"
	"go.etcd.io/bbolt"
)

var (
	ErrBridgeUnavailable = errors.New("生产 Bridge 当前不可用")
	ErrInvalidOrder      = errors.New("生产订单参数无效")
	ErrInvalidInstall    = errors.New("生产 Bridge 维护参数无效")
)

const (
	heartbeatFreshFor = 20 * time.Second
	maxIPCFileSize    = 4 << 20
	maxOrderQuantity  = 999999
)

type ProcessSupervisor interface {
	ProcessStatus() supervisor.Status
	ProcessConfig() supervisor.ProcessConfig
	ApplyAndRestart(supervisor.RestartOptions, supervisor.TransactionHooks) (supervisor.Status, error)
	RunStoppedMaintenance(func() error) (supervisor.Status, error)
}

type BackupFunc func(source string) error

type Manager struct {
	mu    sync.Mutex
	ipcMu sync.Mutex

	installer  *Installer
	process    ProcessSupervisor
	store      *Store
	backup     BackupFunc
	now        func() time.Time
	ctx        context.Context
	cancel     context.CancelFunc
	closeOnce  sync.Once
	wg         sync.WaitGroup
	closed     bool
	installing bool
	stage      string
	lastError  string
}

func NewManager(db *bbolt.DB, process ProcessSupervisor, backup BackupFunc) (*Manager, error) {
	store, err := NewStore(db)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	manager := &Manager{
		installer: NewInstaller(),
		process:   process,
		store:     store,
		backup:    backup,
		now:       time.Now,
		ctx:       ctx,
		cancel:    cancel,
	}
	manager.wg.Add(1)
	go func() {
		defer manager.wg.Done()
		manager.resultLoop()
	}()
	return manager, nil
}

func (manager *Manager) Close() {
	manager.closeOnce.Do(func() {
		manager.mu.Lock()
		manager.closed = true
		manager.mu.Unlock()
		manager.cancel()
		manager.wg.Wait()
	})
}

func (manager *Manager) SetInstallerForTesting(installer *Installer) {
	manager.mu.Lock()
	manager.installer = installer
	manager.mu.Unlock()
}

func (manager *Manager) BridgeStatus() BridgeStatus {
	manager.mu.Lock()
	installer := manager.installer
	installing := manager.installing
	stage := manager.stage
	lastError := manager.lastError
	manager.mu.Unlock()
	status := BridgeStatus{
		BundledVersion:  BridgeVersion,
		ProtocolVersion: BridgeProtocolVersion,
		Installing:      installing,
		InstallStage:    stage,
		LastError:       lastError,
	}
	if manager.process == nil {
		status.State = BridgeUnconfigured
		status.Message = "服务器进程管理器不可用"
		return status
	}
	processStatus := manager.process.ProcessStatus()
	status.ExternalProcess = processStatus.ExternalProcess
	detection := installer.Detect(manager.process.ProcessConfig())
	status.State = detection.State
	status.Message = detection.Message
	status.InstalledVersion = detection.InstalledVersion
	status.InstalledFilesIntact = detection.FilesIntact
	status.ManualInstall = detection.ManualGuide()
	status.RestartRequired = detection.State == BridgeRestartRequired
	if installing {
		status.State = BridgeInstalling
		status.Message = installStageMessage(stage)
		return status
	}
	if detection.State != BridgeOffline && detection.State != BridgeRestartRequired && detection.State != BridgeUpgradeAvailable {
		return status
	}
	runtimeState, err := manager.readRuntimeState(detection.Paths)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			status.LastError = err.Error()
		}
		return status
	}
	now := manager.now()
	age := now.Sub(runtimeState.HeartbeatAt)
	if age < 0 {
		age = 0
	}
	status.HeartbeatAt = &runtimeState.HeartbeatAt
	status.HeartbeatAgeSeconds = int64(age.Seconds())
	status.PalworldBuild = runtimeState.PalworldBuild
	status.CatalogAvailable = runtimeState.Capabilities.Catalog && age <= heartbeatFreshFor
	status.OrdersAvailable = runtimeState.Capabilities.Orders && age <= heartbeatFreshFor && runtimeState.Compatible
	if detection.State == BridgeUpgradeAvailable {
		return status
	}
	if age > heartbeatFreshFor {
		status.State = BridgeOffline
		status.Message = "Bridge 心跳已超时，请检查 PalServer 和 UE4SS 日志"
		return status
	}
	if runtimeState.ProtocolVersion != BridgeProtocolVersion {
		status.State = BridgeIncompatible
		status.Message = "Bridge IPC 协议版本不兼容，请升级或修复 Bridge"
		status.OrdersAvailable = false
		return status
	}
	if !runtimeState.Compatible {
		status.State = BridgeIncompatible
		status.Message = runtimeState.Reason
		if status.Message == "" {
			status.Message = "当前 Palworld Build 尚无经过验证的生产下单适配器"
		}
		status.OrdersAvailable = false
		return status
	}
	status.State = BridgeHealthy
	status.Message = "Bridge 运行正常"
	return status
}

// RecheckBridge discards a previous maintenance failure and rebuilds the
// status from the current INI, installed files and runtime heartbeat.
func (manager *Manager) RecheckBridge() BridgeStatus {
	manager.mu.Lock()
	if !manager.installing {
		manager.lastError = ""
	}
	manager.mu.Unlock()
	return manager.BridgeStatus()
}

func installStageMessage(stage string) string {
	switch stage {
	case "save":
		return "正在保存世界"
	case "backup":
		return "正在创建安装前备份"
	case "shutdown":
		return "正在平滑关闭 PalServer"
	case "install":
		return "正在安装 PST Production Bridge"
	case "start":
		return "正在启动 PalServer"
	case "health":
		return "正在等待 Bridge 心跳"
	default:
		return "正在执行 Bridge 安全维护"
	}
}

func (manager *Manager) BeginInstall(request InstallRequest, repair, disable bool) error {
	if request.Confirmation != "INSTALL" {
		return fmt.Errorf("%w: 请输入 INSTALL 确认维护", ErrInvalidInstall)
	}
	if request.ShutdownSeconds < 0 || request.ShutdownSeconds > 600 || request.RestartDelaySeconds < 0 || request.RestartDelaySeconds > 600 {
		return fmt.Errorf("%w: 关服倒计时和重启等待时间必须在 0–600 秒之间", ErrInvalidInstall)
	}
	if len([]rune(strings.TrimSpace(request.Message))) > 200 {
		return fmt.Errorf("%w: 广播消息不能超过 200 个字符", ErrInvalidInstall)
	}
	if manager.process == nil {
		return supervisor.ErrProcessNotConfigured
	}
	processStatus := manager.process.ProcessStatus()
	if processStatus.ExternalProcess {
		return ErrExternalProcess
	}
	detection := manager.installer.Detect(manager.process.ProcessConfig())
	if detection.State == BridgeUnsupported {
		return supervisor.ErrUnsupportedPlatform
	}
	switch detection.State {
	case BridgeUnconfigured:
		return fmt.Errorf("%w: %s", supervisor.ErrProcessNotConfigured, detection.Message)
	case BridgeDependencyMissing:
		return fmt.Errorf("%w: %s", ErrDependencyMissing, detection.Message)
	case BridgeIncompatible:
		return fmt.Errorf("%w: %s", ErrBridgeIncompatible, detection.Message)
	case BridgePermissionDenied:
		return fmt.Errorf("%w: %s", ErrPermissionDenied, detection.Message)
	case BridgeError:
		return errors.New(detection.Message)
	}
	if disable && detection.State == BridgeNotInstalled {
		return fmt.Errorf("%w: Bridge 尚未安装，无需禁用", ErrInvalidInstall)
	}
	if detection.State == BridgeModified && !repair && !disable {
		return ErrBridgeModified
	}
	manager.mu.Lock()
	if manager.closed {
		manager.mu.Unlock()
		return errors.New("生产 Bridge 管理器已关闭")
	}
	if manager.installing {
		manager.mu.Unlock()
		return supervisor.ErrConflict
	}
	manager.installing = true
	manager.stage = "backup"
	manager.lastError = ""
	manager.wg.Add(1)
	manager.mu.Unlock()
	go func() {
		defer manager.wg.Done()
		manager.runInstall(request, repair, disable)
	}()
	return nil
}

func (manager *Manager) runInstall(request InstallRequest, repair, disable bool) {
	var transaction *FileTransaction
	setStage := func(stage string) {
		manager.mu.Lock()
		manager.stage = stage
		manager.mu.Unlock()
	}
	finish := func(err error) {
		manager.mu.Lock()
		manager.installing = false
		manager.stage = ""
		if err != nil {
			manager.lastError = err.Error()
		} else {
			manager.lastError = ""
		}
		manager.mu.Unlock()
		if err != nil {
			logger.Errorf("PST Production Bridge 维护失败: %v\n", err)
		} else {
			logger.Info("PST Production Bridge 维护完成\n")
		}
	}
	apply := func() error {
		setStage("install")
		var err error
		if disable {
			transaction, err = manager.installer.Disable(manager.process.ProcessConfig())
		} else {
			transaction, err = manager.installer.Install(manager.process.ProcessConfig(), repair)
		}
		return err
	}
	rollback := func() error {
		if transaction == nil {
			return nil
		}
		return transaction.Rollback()
	}
	backup := func() error {
		setStage("backup")
		if manager.backup == nil {
			return errors.New("存档备份服务不可用")
		}
		return manager.backup("production-bridge")
	}

	processStatus := manager.process.ProcessStatus()
	var err error
	if processStatus.Running {
		setStage("save")
		started := manager.now()
		_, err = manager.process.ApplyAndRestart(
			supervisor.RestartOptions{
				ShutdownSeconds: request.ShutdownSeconds,
				RestartDelay:    time.Duration(request.RestartDelaySeconds) * time.Second,
				Message:         strings.TrimSpace(request.Message),
			},
			supervisor.TransactionHooks{
				BeforeShutdown: backup,
				AfterExit: func() error {
					setStage("install")
					return apply()
				},
				Rollback: rollback,
				HealthCheck: func(ctx context.Context) error {
					if disable {
						return nil
					}
					setStage("health")
					return manager.waitForHeartbeat(ctx, started)
				},
			},
		)
	} else {
		_, err = manager.process.RunStoppedMaintenance(func() error {
			if err := backup(); err != nil {
				return err
			}
			return apply()
		})
	}
	if err != nil {
		_ = rollback()
		finish(err)
		return
	}
	if transaction != nil {
		if commitErr := transaction.Commit(); commitErr != nil {
			finish(commitErr)
			return
		}
	}
	finish(nil)
}

func (manager *Manager) waitForHeartbeat(ctx context.Context, after time.Time) error {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		paths, err := manager.installer.paths(manager.process.ProcessConfig())
		if err == nil {
			state, readErr := manager.readRuntimeState(paths)
			if readErr == nil && state.HeartbeatAt.After(after) && manager.now().Sub(state.HeartbeatAt) <= heartbeatFreshFor {
				return nil
			}
		}
		select {
		case <-manager.ctx.Done():
			return errors.New("生产 Bridge 管理器已关闭")
		case <-ctx.Done():
			return errors.New("启动后未收到 Bridge 心跳")
		case <-ticker.C:
		}
	}
}

func (manager *Manager) Catalog() (RuntimeState, error) {
	if manager.process == nil {
		return RuntimeState{}, supervisor.ErrProcessNotConfigured
	}
	paths, err := manager.installer.paths(manager.process.ProcessConfig())
	if err != nil {
		return RuntimeState{}, err
	}
	state, err := manager.readRuntimeState(paths)
	if err != nil {
		return RuntimeState{}, fmt.Errorf("%w: %v", ErrBridgeUnavailable, err)
	}
	if manager.now().Sub(state.HeartbeatAt) > heartbeatFreshFor || !state.Capabilities.Catalog {
		return RuntimeState{}, ErrBridgeUnavailable
	}
	state.BridgeVersion = ""
	state.InstanceID = ""
	return state, nil
}

func (manager *Manager) Preview(request PreviewRequest) (Preview, error) {
	state, err := manager.Catalog()
	if err != nil {
		return Preview{}, err
	}
	return previewFromState(state, request)
}

func previewFromState(state RuntimeState, request PreviewRequest) (Preview, error) {
	if request.Mode != QuantityExact && request.Mode != QuantityMaxAvailable {
		return Preview{}, fmt.Errorf("%w: 数量模式无效", ErrInvalidOrder)
	}
	if request.Mode == QuantityExact && (request.Quantity < 1 || request.Quantity > maxOrderQuantity) {
		return Preview{}, fmt.Errorf("%w: 数量必须在 1–%d 之间", ErrInvalidOrder, maxOrderQuantity)
	}
	var selectedBase *BaseCatalog
	var selectedStation *Workstation
	var selectedRecipe *Recipe
	for baseIndex := range state.Bases {
		base := &state.Bases[baseIndex]
		if base.BaseID != request.BaseID {
			continue
		}
		selectedBase = base
		for stationIndex := range base.Workstations {
			station := &base.Workstations[stationIndex]
			if station.ActorGUID != request.ActorGUID {
				continue
			}
			selectedStation = station
			for recipeIndex := range station.Recipes {
				recipe := &station.Recipes[recipeIndex]
				if recipe.ID == request.RecipeID {
					selectedRecipe = recipe
					break
				}
			}
		}
	}
	if selectedBase == nil || selectedStation == nil || selectedRecipe == nil {
		return Preview{}, fmt.Errorf("%w: 据点、工作台或配方已不存在", ErrInvalidOrder)
	}
	if !selectedRecipe.Unlocked {
		return Preview{}, fmt.Errorf("%w: 配方尚未解锁", ErrInvalidOrder)
	}
	maxAvailable := int64(maxOrderQuantity)
	if selectedRecipe.MaxAvailable >= 0 && selectedRecipe.MaxAvailable < maxAvailable {
		maxAvailable = selectedRecipe.MaxAvailable
	}
	if len(selectedRecipe.Materials) == 0 {
		if selectedRecipe.MaxAvailable < 0 {
			maxAvailable = maxOrderQuantity
		}
	}
	for _, material := range selectedRecipe.Materials {
		if material.RequiredEach <= 0 {
			return Preview{}, fmt.Errorf("%w: 配方材料数据无效", ErrInvalidOrder)
		}
		possible := material.Available / material.RequiredEach
		if possible < maxAvailable {
			maxAvailable = possible
		}
	}
	if maxAvailable < 0 {
		maxAvailable = 0
	}
	quantity := request.Quantity
	if request.Mode == QuantityMaxAvailable {
		quantity = maxAvailable
	}
	preview := Preview{
		BaseID:             request.BaseID,
		ActorGUID:          request.ActorGUID,
		RecipeID:           request.RecipeID,
		RequestedQuantity:  request.Quantity,
		AcceptedQuantity:   quantity,
		MaxAvailable:       maxAvailable,
		CanSubmit:          quantity > 0 && quantity <= maxAvailable,
		SnapshotHeartbeat:  state.HeartbeatAt,
		QuantityCalculated: true,
	}
	if !preview.CanSubmit {
		preview.Reason = "当前据点材料不足"
	}
	for _, material := range selectedRecipe.Materials {
		required := material.RequiredEach * quantity
		shortage := int64(math.Max(0, float64(required-material.Available)))
		preview.Materials = append(preview.Materials, MaterialPreview{
			ItemID:       material.ItemID,
			Name:         material.Name,
			RequiredEach: material.RequiredEach,
			Required:     required,
			Available:    material.Available,
			Shortage:     shortage,
		})
	}
	return preview, nil
}

func (manager *Manager) CreateOrder(request PreviewRequest) (Order, error) {
	status := manager.BridgeStatus()
	if status.State != BridgeHealthy || !status.OrdersAvailable {
		return Order{}, ErrBridgeUnavailable
	}
	state, err := manager.Catalog()
	if err != nil {
		return Order{}, err
	}
	preview, err := previewFromState(state, request)
	if err != nil {
		return Order{}, err
	}
	if !preview.CanSubmit {
		return Order{}, fmt.Errorf("%w: %s", ErrInvalidOrder, preview.Reason)
	}
	base, station, recipe := findCatalogEntry(state, request)
	now := manager.now().UTC()
	order := Order{
		ID:              uuid.NewString(),
		BaseID:          request.BaseID,
		BaseName:        base.BaseName,
		ActorGUID:       request.ActorGUID,
		WorkstationName: station.Name,
		RecipeID:        request.RecipeID,
		RecipeName:      recipe.Name,
		ProductID:       recipe.ProductID,
		ProductName:     recipe.ProductName,
		QuantityMode:    request.Mode,
		RequestedAmount: request.Quantity,
		AcceptedAmount:  preview.AcceptedQuantity,
		Status:          OrderPending,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	if err := manager.store.PutOrder(order); err != nil {
		return Order{}, err
	}
	if err := manager.writeOrderRequest(order, "create"); err != nil {
		order.Status = OrderFailed
		order.Error = err.Error()
		order.UpdatedAt = manager.now().UTC()
		_ = manager.store.PutOrder(order)
		return order, err
	}
	order.Status = OrderDispatching
	order.UpdatedAt = manager.now().UTC()
	if err := manager.store.PutOrder(order); err != nil {
		return Order{}, err
	}
	return order, nil
}

func findCatalogEntry(state RuntimeState, request PreviewRequest) (*BaseCatalog, *Workstation, *Recipe) {
	for baseIndex := range state.Bases {
		base := &state.Bases[baseIndex]
		if base.BaseID != request.BaseID {
			continue
		}
		for stationIndex := range base.Workstations {
			station := &base.Workstations[stationIndex]
			if station.ActorGUID != request.ActorGUID {
				continue
			}
			for recipeIndex := range station.Recipes {
				recipe := &station.Recipes[recipeIndex]
				if recipe.ID == request.RecipeID {
					return base, station, recipe
				}
			}
		}
	}
	return &BaseCatalog{}, &Workstation{}, &Recipe{}
}

func (manager *Manager) CancelOrder(id string) (Order, error) {
	if _, err := uuid.Parse(id); err != nil {
		return Order{}, ErrInvalidOrder
	}
	order, err := manager.store.Order(id)
	if err != nil {
		return Order{}, err
	}
	if order.Status != OrderPending && order.Status != OrderDispatching && order.Status != OrderWaitingMaterials {
		return Order{}, errors.New("只有尚未被游戏接受的订单可以取消")
	}
	if err := manager.writeOrderRequest(order, "cancel"); err != nil {
		return Order{}, err
	}
	order.CancellationRequested = true
	order.UpdatedAt = manager.now().UTC()
	if err := manager.store.PutOrder(order); err != nil {
		return Order{}, err
	}
	return order, nil
}

func (manager *Manager) Orders(limit int) ([]Order, error) {
	return manager.store.ListOrders(limit)
}

func (manager *Manager) writeOrderRequest(order Order, action string) error {
	manager.ipcMu.Lock()
	defer manager.ipcMu.Unlock()
	paths, err := manager.installer.paths(manager.process.ProcessConfig())
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(paths.IPC, "requests"), 0700); err != nil {
		return err
	}
	secret, err := manager.store.Secret()
	if err != nil {
		return err
	}
	if err := atomicWriteFile(filepath.Join(paths.IPC, "bridge.key"), []byte(secret+"\n"), 0600); err != nil {
		return err
	}
	request := OrderRequest{
		OrderID:      order.ID,
		Key:          secret,
		BaseID:       order.BaseID,
		ActorGUID:    order.ActorGUID,
		RecipeID:     order.RecipeID,
		QuantityMode: order.QuantityMode,
		Quantity:     order.AcceptedAmount,
		CreatedAt:    manager.now().UTC(),
		Action:       action,
	}
	data, err := json.Marshal(request)
	if err != nil {
		return err
	}
	if err := atomicWriteFile(filepath.Join(paths.IPC, "requests", order.ID+".json"), data, 0600); err != nil {
		return err
	}
	indexPath := filepath.Join(paths.IPC, "requests", "index.json")
	ids := make([]string, 0)
	if indexData, readErr := os.ReadFile(indexPath); readErr == nil {
		_ = json.Unmarshal(indexData, &ids)
	}
	found := false
	for _, id := range ids {
		if id == order.ID {
			found = true
			break
		}
	}
	if !found {
		ids = append(ids, order.ID)
	}
	indexData, err := json.Marshal(ids)
	if err != nil {
		return err
	}
	return atomicWriteFile(indexPath, indexData, 0600)
}

func (manager *Manager) readRuntimeState(paths InstallPaths) (RuntimeState, error) {
	var state RuntimeState
	path := filepath.Join(paths.IPC, "state.json")
	info, err := os.Lstat(path)
	if err != nil {
		return state, err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return state, errors.New("Bridge state.json 不是普通文件")
	}
	if info.Size() > maxIPCFileSize {
		return state, errors.New("Bridge state.json 超过大小限制")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return state, err
	}
	if err := json.Unmarshal(data, &state); err != nil {
		return state, fmt.Errorf("解析 Bridge state.json: %w", err)
	}
	if state.HeartbeatAt.IsZero() || state.BridgeVersion == "" || state.ProtocolVersion < 1 {
		return state, errors.New("Bridge state.json 缺少必要字段")
	}
	return state, nil
}

func (manager *Manager) resultLoop() {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-manager.ctx.Done():
			return
		case <-ticker.C:
			manager.consumeResults()
		}
	}
}

func (manager *Manager) consumeResults() {
	if manager.process == nil {
		return
	}
	paths, err := manager.installer.paths(manager.process.ProcessConfig())
	if err != nil {
		return
	}
	resultDirectory := filepath.Join(paths.IPC, "results")
	entries, err := os.ReadDir(resultDirectory)
	if err != nil {
		return
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		id := strings.TrimSuffix(entry.Name(), ".json")
		if _, err := uuid.Parse(id); err != nil {
			continue
		}
		path := filepath.Join(resultDirectory, entry.Name())
		info, err := entry.Info()
		if err != nil || !info.Mode().IsRegular() || info.Size() > maxIPCFileSize {
			continue
		}
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var result OrderResult
		if err := json.Unmarshal(data, &result); err != nil || result.OrderID != id || !validResultStatus(result.Status) {
			continue
		}
		order, err := manager.store.Order(id)
		if err != nil {
			continue
		}
		if order.Status == OrderCancelled || order.Status == OrderCompleted || order.Status == OrderFailed {
			_ = os.Remove(path)
			continue
		}
		order.Status = result.Status
		order.AcceptedAmount = result.AcceptedQuantity
		order.CompletedAmount = result.CompletedQuantity
		order.Error = result.Error
		order.BridgeInstanceID = result.BridgeInstanceID
		order.UpdatedAt = result.UpdatedAt
		if order.UpdatedAt.IsZero() {
			order.UpdatedAt = manager.now().UTC()
		}
		if result.Status == OrderAccepted && order.AcceptedAt == nil {
			at := order.UpdatedAt
			order.AcceptedAt = &at
		}
		if result.Status == OrderCompleted {
			at := order.UpdatedAt
			order.CompletedAt = &at
		}
		if err := manager.store.PutOrder(order); err == nil {
			_ = os.Remove(path)
			if result.Status != OrderWaitingMaterials {
				manager.removeRequestIndex(paths, id)
			}
		}
	}
}

func (manager *Manager) removeRequestIndex(paths InstallPaths, id string) {
	manager.ipcMu.Lock()
	defer manager.ipcMu.Unlock()
	indexPath := filepath.Join(paths.IPC, "requests", "index.json")
	data, err := os.ReadFile(indexPath)
	if err != nil {
		return
	}
	var ids []string
	if json.Unmarshal(data, &ids) != nil {
		return
	}
	filtered := ids[:0]
	for _, current := range ids {
		if current != id {
			filtered = append(filtered, current)
		}
	}
	encoded, err := json.Marshal(filtered)
	if err == nil {
		_ = atomicWriteFile(indexPath, encoded, 0600)
		_ = os.Remove(filepath.Join(paths.IPC, "requests", id+".json"))
	}
}

func validResultStatus(status OrderStatus) bool {
	switch status {
	case OrderAccepted, OrderWaitingMaterials, OrderProducing, OrderCompleted, OrderCancelled, OrderFailed, OrderUnknown:
		return true
	default:
		return false
	}
}
