package api

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/zaigie/palworld-server-tool/internal/production"
	"github.com/zaigie/palworld-server-tool/internal/supervisor"
)

func getProductionBridge(manager *production.Manager) gin.HandlerFunc {
	return func(c *gin.Context) {
		if manager == nil {
			c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "生产 Bridge 管理器不可用"})
			return
		}
		c.JSON(http.StatusOK, manager.BridgeStatus())
	}
}

func recheckProductionBridge(manager *production.Manager) gin.HandlerFunc {
	return func(c *gin.Context) {
		if manager == nil {
			c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "生产 Bridge 管理器不可用"})
			return
		}
		c.JSON(http.StatusOK, manager.RecheckBridge())
	}
}

func installProductionBridge(manager *production.Manager, repair, disable bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		if manager == nil {
			c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "生产 Bridge 管理器不可用"})
			return
		}
		var request production.InstallRequest
		if err := c.ShouldBindJSON(&request); err != nil {
			c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
			return
		}
		if err := manager.BeginInstall(request, repair, disable); err != nil {
			writeProductionError(c, err)
			return
		}
		c.JSON(http.StatusAccepted, manager.BridgeStatus())
	}
}

func getProductionCatalog(manager *production.Manager) gin.HandlerFunc {
	return func(c *gin.Context) {
		if manager == nil {
			c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "生产 Bridge 管理器不可用"})
			return
		}
		catalog, err := manager.Catalog()
		if err != nil {
			writeProductionError(c, err)
			return
		}
		c.JSON(http.StatusOK, catalog)
	}
}

func previewProductionOrder(manager *production.Manager) gin.HandlerFunc {
	return func(c *gin.Context) {
		if manager == nil {
			c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "生产 Bridge 管理器不可用"})
			return
		}
		var request production.PreviewRequest
		if err := c.ShouldBindJSON(&request); err != nil {
			c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
			return
		}
		preview, err := manager.Preview(request)
		if err != nil {
			writeProductionError(c, err)
			return
		}
		c.JSON(http.StatusOK, preview)
	}
}

func listProductionOrders(manager *production.Manager) gin.HandlerFunc {
	return func(c *gin.Context) {
		if manager == nil {
			c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "生产 Bridge 管理器不可用"})
			return
		}
		limit, _ := strconv.Atoi(c.DefaultQuery("limit", "200"))
		orders, err := manager.Orders(limit)
		if err != nil {
			writeProductionError(c, err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"items": orders})
	}
}

func createProductionOrder(manager *production.Manager) gin.HandlerFunc {
	return func(c *gin.Context) {
		if manager == nil {
			c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "生产 Bridge 管理器不可用"})
			return
		}
		var request production.PreviewRequest
		if err := c.ShouldBindJSON(&request); err != nil {
			c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
			return
		}
		order, err := manager.CreateOrder(request)
		if err != nil {
			writeProductionError(c, err)
			return
		}
		c.JSON(http.StatusCreated, order)
	}
}

func cancelProductionOrder(manager *production.Manager) gin.HandlerFunc {
	return func(c *gin.Context) {
		if manager == nil {
			c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "生产 Bridge 管理器不可用"})
			return
		}
		order, err := manager.CancelOrder(c.Param("order_id"))
		if err != nil {
			writeProductionError(c, err)
			return
		}
		c.JSON(http.StatusOK, order)
	}
}

func writeProductionError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, supervisor.ErrConflict):
		c.JSON(http.StatusConflict, ErrorResponse{Error: err.Error()})
	case errors.Is(err, production.ErrOrderNotFound):
		c.JSON(http.StatusNotFound, ErrorResponse{Error: err.Error()})
	case errors.Is(err, production.ErrInvalidOrder):
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
	case errors.Is(err, production.ErrInvalidInstall):
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
	case errors.Is(err, production.ErrBridgeUnavailable):
		c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: err.Error()})
	case errors.Is(err, production.ErrExternalProcess),
		errors.Is(err, production.ErrDependencyMissing),
		errors.Is(err, production.ErrBridgeModified),
		errors.Is(err, production.ErrBridgeIncompatible),
		errors.Is(err, production.ErrPermissionDenied),
		errors.Is(err, supervisor.ErrProcessNotConfigured),
		errors.Is(err, supervisor.ErrUnsupportedPlatform):
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
	default:
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}
}
