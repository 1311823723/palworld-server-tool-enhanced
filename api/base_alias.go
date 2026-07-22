package api

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/zaigie/palworld-server-tool/internal/database"
	"github.com/zaigie/palworld-server-tool/service"
)

type baseAliasRequest struct {
	Name string `json:"name"`
}

func listBaseAliases(c *gin.Context) {
	items, err := service.ListBaseAliases(database.GetDB())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "读取据点名称失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": items})
}

func putBaseAlias(c *gin.Context) {
	var request baseAliasRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请提供有效的据点名称"})
		return
	}
	item, err := service.SetBaseAlias(database.GetDB(), strings.TrimSpace(c.Param("base_id")), request.Name, time.Now().UTC())
	if err != nil {
		status := http.StatusInternalServerError
		switch {
		case errors.Is(err, service.ErrInvalidBaseAlias):
			status = http.StatusBadRequest
		case errors.Is(err, service.ErrBaseAliasConflict):
			status = http.StatusConflict
		case errors.Is(err, service.ErrNoRecord), errors.Is(err, service.ErrSnapshotUnavailable):
			status = http.StatusNotFound
		}
		c.JSON(status, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "item": item})
}

func deleteBaseAlias(c *gin.Context) {
	err := service.DeleteBaseAlias(database.GetDB(), strings.TrimSpace(c.Param("base_id")))
	if err != nil {
		if errors.Is(err, service.ErrNoRecord) {
			c.JSON(http.StatusNotFound, gin.H{"error": "据点名称记录不存在"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "重置据点名称失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}
