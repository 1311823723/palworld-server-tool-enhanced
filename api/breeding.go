package api

import (
	"errors"
	"net/http"
	"reflect"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/zaigie/palworld-server-tool/internal/config"
	"github.com/zaigie/palworld-server-tool/internal/database"
	"github.com/zaigie/palworld-server-tool/service"
)

func optionalBoolQuery(c *gin.Context, name string) (*bool, error) {
	raw, present := c.GetQuery(name)
	if !present || strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	value, err := strconv.ParseBool(raw)
	if err != nil {
		return nil, errors.New(name + " must be true or false")
	}
	return &value, nil
}

func breedingFarmQuery(c *gin.Context) (service.BreedingFarmQuery, error) {
	hasEgg, err := optionalBoolQuery(c, "has_egg")
	if err != nil {
		return service.BreedingFarmQuery{}, err
	}
	cakeEmpty, err := optionalBoolQuery(c, "cake_empty")
	if err != nil {
		return service.BreedingFarmQuery{}, err
	}
	parentMissing, err := optionalBoolQuery(c, "parent_missing")
	if err != nil {
		return service.BreedingFarmQuery{}, err
	}
	hasWarning, err := optionalBoolQuery(c, "has_warning")
	if err != nil {
		return service.BreedingFarmQuery{}, err
	}
	page, pageSize := pageValues(c)
	return service.BreedingFarmQuery{
		BaseID: strings.TrimSpace(c.Query("base_id")), GuildID: strings.TrimSpace(c.Query("guild_id")),
		HasEgg: hasEgg, CakeEmpty: cakeEmpty, ParentMissing: parentMissing, HasWarning: hasWarning,
		Sort: strings.TrimSpace(c.Query("sort")), Page: page, PageSize: pageSize,
	}, nil
}

func listBreedingFarms(c *gin.Context) {
	query, err := breedingFarmQuery(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	page, err := service.ListBreedingFarms(database.GetDB(), query)
	if err != nil {
		snapshotError(c, err)
		return
	}
	page.Metadata = snapshotMetadataForResponse(page.Metadata)
	c.JSON(http.StatusOK, page)
}

func getBreedingFarm(c *gin.Context) {
	item, metadata, err := service.GetBreedingFarm(database.GetDB(), c.Param("farm_id"))
	if err != nil {
		snapshotError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"metadata": snapshotMetadataForResponse(metadata), "item": item})
}

func getBreedingParents(c *gin.Context) {
	items, metadata, err := service.ListBreedingParents(database.GetDB(), c.Param("farm_id"))
	if err != nil {
		snapshotError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"metadata": snapshotMetadataForResponse(metadata), "items": items})
}

func getBreedingCakes(c *gin.Context) {
	item, metadata, err := service.GetBreedingCakes(database.GetDB(), c.Param("farm_id"))
	if err != nil {
		snapshotError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"metadata": snapshotMetadataForResponse(metadata), "item": item})
}

func getBreedingEggs(c *gin.Context) {
	items, metadata, err := service.ListBreedingEggs(database.GetDB(), c.Param("farm_id"))
	if err != nil {
		snapshotError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"metadata": snapshotMetadataForResponse(metadata), "items": items})
}

func getBreedingCapabilities(c *gin.Context) {
	item, metadata, err := service.BreedingCapabilities(database.GetDB())
	if err != nil {
		snapshotError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"metadata": snapshotMetadataForResponse(metadata), "capabilities": item})
}

func getBreedingNotificationConfig(c *gin.Context) {
	c.JSON(http.StatusOK, config.NormalizeBreedingMonitor(config.Current().BreedingMonitor))
}

type breedingConfigRequest struct {
	config.BreedingMonitorConfig
	ConfirmAll bool `json:"confirm_all"`
}

func putBreedingNotificationConfig(c *gin.Context) {
	var request breedingConfigRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	value := config.NormalizeBreedingMonitor(request.BreedingMonitorConfig)
	if value.SelectionMode == "all" && value.Enabled && !request.ConfirmAll {
		c.JSON(http.StatusBadRequest, gin.H{"error": "monitoring all breeding farms requires confirm_all=true"})
		return
	}
	if err := config.ValidateBreedingMonitor(value); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	previous := config.NormalizeBreedingMonitor(config.Current().BreedingMonitor)
	selectionChanged := previous.Enabled != value.Enabled || previous.SelectionMode != value.SelectionMode ||
		!reflect.DeepEqual(previous.SelectedBaseIDs, value.SelectedBaseIDs) || !reflect.DeepEqual(previous.SelectedFarmIDs, value.SelectedFarmIDs)
	if selectionChanged && value.Enabled {
		if err := service.PrepareBreedingMonitor(database.GetDB(), value, value.NotifyExistingOnEnable); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}
	if err := config.CurrentStore().SetBreedingMonitor(value); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "settings": value})
}

func breedingEventQuery(c *gin.Context) (service.BreedingEventQuery, error) {
	unread, err := optionalBoolQuery(c, "unread")
	if err != nil {
		return service.BreedingEventQuery{}, err
	}
	page, pageSize := pageValues(c)
	return service.BreedingEventQuery{Unread: unread, BaseID: strings.TrimSpace(c.Query("base_id")), FarmID: strings.TrimSpace(c.Query("farm_id")), EventType: strings.TrimSpace(c.Query("event_type")), Page: page, PageSize: pageSize}, nil
}

func listBreedingEvents(c *gin.Context) {
	query, err := breedingEventQuery(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	page, err := service.ListBreedingEvents(database.GetDB(), query)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, page)
}

func listUnreadBreedingEvents(c *gin.Context) {
	value := true
	page, err := service.ListBreedingEvents(database.GetDB(), service.BreedingEventQuery{Unread: &value, Page: 1, PageSize: 200})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, page)
}

func markBreedingEventRead(c *gin.Context) {
	if err := service.MarkBreedingEventRead(database.GetDB(), c.Param("event_id")); err != nil {
		snapshotError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

func markAllBreedingEventsRead(c *gin.Context) {
	err := service.MarkAllBreedingEventsRead(database.GetDB())
	if err != nil && !errors.Is(err, service.ErrNoRecord) {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}
