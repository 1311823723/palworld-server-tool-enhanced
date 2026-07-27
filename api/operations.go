package api

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/zaigie/palworld-server-tool/internal/database"
	"github.com/zaigie/palworld-server-tool/internal/logger"
	"github.com/zaigie/palworld-server-tool/service"
)

func operationAuditMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.Method == http.MethodGet || c.Request.Method == http.MethodHead || c.Request.Method == http.MethodOptions {
			c.Next()
			return
		}
		started := time.Now().UTC()
		c.Next()
		route := c.FullPath()
		if route == "" {
			route = c.Request.URL.Path
		}
		status := "success"
		if c.Writer.Status() >= http.StatusBadRequest {
			status = "error"
		}
		detail := fmt.Sprintf("HTTP %d", c.Writer.Status())
		record := database.OperationAudit{
			Action:    c.Request.Method + " " + route,
			Status:    status,
			Detail:    detail,
			CreatedAt: started,
		}
		if err := service.AddOperationAudit(database.GetDB(), record); err != nil {
			logger.Errorf("保存操作审计失败: %v\n", err)
		}
	}
}

func listOperationAudits(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "100"))
	var since time.Time
	if raw := strings.TrimSpace(c.Query("since")); raw != "" {
		parsed, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			c.JSON(http.StatusBadRequest, ErrorResponse{Error: "since 必须使用 RFC3339 时间格式"})
			return
		}
		since = parsed
	}
	items, err := service.ListOperationAudits(database.GetDB(), limit, c.Query("action"), c.Query("status"), since)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": items})
}

func listRuntimeLogs(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "200"))
	afterID, _ := strconv.ParseInt(c.DefaultQuery("after_id", "0"), 10, 64)
	items := logger.List(afterID, limit, c.Query("level"))
	next := afterID
	if len(items) > 0 {
		next = items[len(items)-1].ID
	}
	c.JSON(http.StatusOK, gin.H{"items": items, "next_cursor": next})
}
