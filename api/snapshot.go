package api

import (
	"errors"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/zaigie/palworld-server-tool/internal/config"
	"github.com/zaigie/palworld-server-tool/internal/database"
	"github.com/zaigie/palworld-server-tool/internal/logger"
	"github.com/zaigie/palworld-server-tool/internal/tool"
	"github.com/zaigie/palworld-server-tool/internal/worldsettings"
	"github.com/zaigie/palworld-server-tool/service"
)

const defaultBaseWorkerMaximum = 15

var broadcastBreedingGameNotification = tool.Broadcast

func putSnapshot(c *gin.Context) {
	var payload database.SnapshotPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	monitor := config.NormalizeBreedingMonitor(config.Current().BreedingMonitor)
	metadata, events, err := service.PutSnapshotWithBreedingMonitorEvents(database.GetDB(), payload, &monitor, time.Now().UTC())
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	notificationWarnings := make([]string, 0)
	if monitor.GameNotifications {
		for _, notification := range service.BuildBreedingGameNotifications(events, monitor.GameNotificationMessage) {
			if err := broadcastBreedingGameNotification(notification.Message); err != nil {
				logger.Errorf("发送配种农场游戏内提醒失败（farm=%s）：%v\n", notification.FarmID, err)
				notificationWarnings = append(notificationWarnings, "游戏内产蛋提醒发送失败，请检查 Palworld REST API 配置")
				continue
			}
			logger.Infof("已发送配种农场游戏内提醒（farm=%s，events=%d）\n", notification.FarmID, len(notification.EventIDs))
		}
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "metadata": metadata, "notification_warnings": notificationWarnings})
}

func snapshotMetadataForResponse(metadata database.SnapshotMetadata) database.SnapshotMetadata {
	metadata.SyncIntervalSeconds = config.Current().Save.SyncInterval
	if !metadata.SaveFileTime.IsZero() {
		metadata.IsStale = time.Since(metadata.SaveFileTime) > 15*time.Minute
	}
	if metadata.Warnings == nil {
		metadata.Warnings = []string{}
	}
	if metadata.Capabilities == nil {
		metadata.Capabilities = map[string]string{}
	}
	return metadata
}

func snapshotError(c *gin.Context, err error) {
	status := http.StatusInternalServerError
	if errors.Is(err, service.ErrSnapshotUnavailable) || errors.Is(err, service.ErrNoRecord) {
		status = http.StatusNotFound
	}
	c.JSON(status, gin.H{"error": err.Error()})
}

func inventoryQuery(c *gin.Context) service.InventoryQuery {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "50"))
	return service.InventoryQuery{
		Q:             strings.TrimSpace(c.Query("q")),
		SourceType:    strings.TrimSpace(c.Query("source_type")),
		PlayerUID:     strings.TrimSpace(c.Query("player_uid")),
		GuildID:       strings.TrimSpace(c.Query("guild_id")),
		BaseID:        strings.TrimSpace(c.Query("base_id")),
		ContainerID:   strings.TrimSpace(c.Query("container_id")),
		ContainerType: strings.TrimSpace(c.Query("container_type")),
		Sort:          strings.TrimSpace(c.Query("sort")),
		Page:          page,
		PageSize:      pageSize,
	}
}

func listBaseCamps(c *gin.Context) {
	listBaseCampsWithMaximum(c, defaultBaseWorkerMaximum)
}

func listBaseCampsWithSettings(manager *worldsettings.Manager) gin.HandlerFunc {
	return func(c *gin.Context) {
		maximum := defaultBaseWorkerMaximum
		if manager != nil {
			maximum = manager.CurrentInt("BaseCampWorkerMaxNum", maximum)
		}
		listBaseCampsWithMaximum(c, maximum)
	}
}

func listBaseCampsWithMaximum(c *gin.Context, maximum int) {
	items, metadata, err := service.BaseCampOverviews(database.GetDB(), maximum)
	if err != nil {
		snapshotError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"metadata": snapshotMetadataForResponse(metadata), "items": items})
}

func getBaseCampSnapshot(c *gin.Context) {
	getBaseCampSnapshotWithMaximum(c, defaultBaseWorkerMaximum)
}

func getBaseCampWithSettings(manager *worldsettings.Manager) gin.HandlerFunc {
	return func(c *gin.Context) {
		maximum := defaultBaseWorkerMaximum
		if manager != nil {
			maximum = manager.CurrentInt("BaseCampWorkerMaxNum", maximum)
		}
		getBaseCampSnapshotWithMaximum(c, maximum)
	}
}

func getBaseCampSnapshotWithMaximum(c *gin.Context, maximum int) {
	baseID := c.Param("base_id")
	items, metadata, err := service.BaseCampOverviews(database.GetDB(), maximum)
	if err != nil {
		snapshotError(c, err)
		return
	}
	for _, item := range items {
		if item.BaseID == baseID {
			c.JSON(http.StatusOK, gin.H{"metadata": snapshotMetadataForResponse(metadata), "item": item})
			return
		}
	}
	c.JSON(http.StatusNotFound, gin.H{"error": "base camp not found"})
}

func workerIsAbnormal(worker database.BaseWorkerPal) bool {
	if worker.HP != nil && worker.MaxHP != nil && *worker.MaxHP > 0 && float64(*worker.HP)*100/float64(*worker.MaxHP) < service.LowHPPercent {
		return true
	}
	if worker.FullStomach != nil && *worker.FullStomach < service.LowFullStomach || worker.Sanity != nil && *worker.Sanity < service.LowSanity {
		return true
	}
	return len(worker.StatusAbnormalities) > 0 || worker.IsDown != nil && *worker.IsDown || worker.IsSick != nil && *worker.IsSick || worker.IsInjured != nil && *worker.IsInjured
}

func listBaseWorkers(c *gin.Context) {
	workers, metadata, err := service.ListBaseWorkers(database.GetDB(), c.Param("base_id"))
	if err != nil {
		snapshotError(c, err)
		return
	}
	q := strings.ToLower(strings.TrimSpace(c.Query("q")))
	workType := strings.ToLower(strings.TrimSpace(c.Query("work_type")))
	health := strings.ToLower(strings.TrimSpace(c.Query("health")))
	anomaliesOnly := c.Query("abnormal_only") == "true"
	filtered := workers[:0]
	for _, worker := range workers {
		abnormal := workerIsAbnormal(worker)
		if anomaliesOnly && !abnormal || health == "abnormal" && !abnormal || health == "healthy" && abnormal {
			continue
		}
		if workType != "" && (worker.CurrentWork == nil || !strings.Contains(strings.ToLower(*worker.CurrentWork), workType)) {
			continue
		}
		if q != "" {
			values := []string{worker.PalID, worker.Nickname, worker.OwnerPlayerName, worker.GuildName, worker.BaseName}
			matched := false
			for _, value := range values {
				if strings.Contains(strings.ToLower(value), q) {
					matched = true
					break
				}
			}
			if !matched {
				continue
			}
		}
		filtered = append(filtered, worker)
	}
	sortWorkers(filtered, c.DefaultQuery("sort", "name"))
	page, pageSize := pageValues(c)
	start, end := pageBounds(len(filtered), page, pageSize)
	c.JSON(http.StatusOK, gin.H{
		"metadata":  snapshotMetadataForResponse(metadata),
		"items":     filtered[start:end],
		"page":      page,
		"page_size": pageSize,
		"total":     len(filtered),
	})
}

func sortWorkers(workers []database.BaseWorkerPal, order string) {
	sort.SliceStable(workers, func(i, j int) bool {
		switch order {
		case "level":
			return workers[i].Level > workers[j].Level
		case "hp_percent":
			return nullablePercent(workers[i].HP, workers[i].MaxHP) < nullablePercent(workers[j].HP, workers[j].MaxHP)
		case "full_stomach":
			return nullableFloat(workers[i].FullStomach) < nullableFloat(workers[j].FullStomach)
		case "sanity":
			return nullableFloat(workers[i].Sanity) < nullableFloat(workers[j].Sanity)
		case "work_speed":
			return workers[i].WorkSpeed > workers[j].WorkSpeed
		default:
			return strings.ToLower(workers[i].PalID) < strings.ToLower(workers[j].PalID)
		}
	})
}

func nullablePercent(value, maximum *int64) float64 {
	if value == nil || maximum == nil || *maximum <= 0 {
		return 101
	}
	return float64(*value) * 100 / float64(*maximum)
}

func nullableFloat(value *float64) float64 {
	if value == nil {
		return 1e18
	}
	return *value
}

func pageValues(c *gin.Context) (int, int) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "50"))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 50
	}
	if pageSize > 200 {
		pageSize = 200
	}
	return page, pageSize
}

func pageBounds(total, page, pageSize int) (int, int) {
	start := (page - 1) * pageSize
	if start > total {
		start = total
	}
	end := start + pageSize
	if end > total {
		end = total
	}
	return start, end
}

func listFeedBoxes(c *gin.Context) {
	items, metadata, err := service.FeedBoxes(database.GetDB(), c.Param("base_id"))
	if err != nil {
		snapshotError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"metadata": snapshotMetadataForResponse(metadata), "items": items})
}

func inventorySummary(c *gin.Context) {
	page, err := service.InventorySummary(database.GetDB(), inventoryQuery(c))
	if err != nil {
		snapshotError(c, err)
		return
	}
	page.Metadata = snapshotMetadataForResponse(page.Metadata)
	c.JSON(http.StatusOK, page)
}

func publicInventorySummary(c *gin.Context) {
	visibility := config.Current().InventoryVisibility
	if visibility.Mode != "public_summary" || !visibility.AllowPublicSummary {
		c.JSON(http.StatusForbidden, gin.H{"error": "public inventory summary is disabled"})
		return
	}
	inventorySummary(c)
}

func inventoryItemLocations(c *gin.Context) {
	query := inventoryQuery(c)
	locations, metadata, total, err := service.InventoryLocations(database.GetDB(), c.Param("item_id"), query)
	if err != nil {
		snapshotError(c, err)
		return
	}
	page, pageSize := query.Page, query.PageSize
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 50
	}
	c.JSON(http.StatusOK, gin.H{"metadata": snapshotMetadataForResponse(metadata), "items": locations, "page": page, "page_size": pageSize, "total": total})
}

func inventoryContainers(c *gin.Context) {
	items, metadata, err := service.ListContainers(database.GetDB(), inventoryQuery(c))
	if err != nil {
		snapshotError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"metadata": snapshotMetadataForResponse(metadata), "items": items})
}
