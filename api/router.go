package api

import (
	"fmt"
	"strings"

	"github.com/gin-gonic/gin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
	"github.com/zaigie/palworld-server-tool/internal/auth"
	"github.com/zaigie/palworld-server-tool/internal/worldsettings"
)

type SuccessResponse struct {
	Success bool `json:"success"`
}

type MessageResponse struct {
	Message string `json:"message"`
}

type ErrorResponse struct {
	Error string `json:"error"`
}

type EmptyResponse struct{}

func ignoreLogPrefix(path string) bool {
	prefixes := []string{"/swagger/", "/assets/", "/favicon.ico", "/map"}
	for _, prefix := range prefixes {
		if strings.HasPrefix(path, prefix) {
			return true
		}
	}
	return false
}

func Logger() gin.HandlerFunc {
	return gin.LoggerWithFormatter(func(param gin.LogFormatterParams) string {
		if !ignoreLogPrefix(param.Path) {
			statusColor := param.StatusCodeColor()
			methodColor := param.MethodColor()
			resetColor := param.ResetColor()
			return fmt.Sprintf("[GIN] %v |%s %3d %s| %13v | %15s |%s %-7s %s %#v\n%s",
				param.TimeStamp.Format("2006/01/02 - 15:04:05"),
				statusColor, param.StatusCode, resetColor,
				param.Latency,
				param.ClientIP,
				methodColor, param.Method, resetColor,
				param.Path,
				param.ErrorMessage,
			)
		}
		return ""
	})
}

// SecurityHeaders prevents browsers and intermediate proxies from caching
// player, inventory and server-control responses when PST is published
// through a tunnel or reverse proxy.
func SecurityHeaders() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("X-Frame-Options", "SAMEORIGIN")
		c.Header("Referrer-Policy", "strict-origin-when-cross-origin")
		c.Header("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		if strings.HasPrefix(c.Request.URL.Path, "/api") {
			c.Header("Cache-Control", "no-store, max-age=0")
			c.Header("Pragma", "no-cache")
		}
		c.Next()
	}
}

func RegisterRouter(r *gin.Engine, onConfigInitialized func()) {
	RegisterRouterWithSupervisor(r, onConfigInitialized, nil)
}

func RegisterRouterWithSupervisor(r *gin.Engine, onConfigInitialized func(), processManager ServerProcessManager) {
	RegisterRouterWithManagers(r, onConfigInitialized, processManager, nil)
}

func RegisterRouterWithManagers(r *gin.Engine, onConfigInitialized func(), processManager ServerProcessManager, settingsManager *worldsettings.Manager) {
	r.Use(Logger(), gin.Recovery(), SecurityHeaders())

	r.POST("/api/login", loginHandler)
	r.GET("/api/config/status", getConfigStatus)
	r.POST("/api/config/initialize", func(c *gin.Context) {
		initializeConfig(c, onConfigInitialized)
	})
	r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	apiGroup := r.Group("/api")

	anonymousGroup := apiGroup.Group("")
	{
		anonymousGroup.GET("/server", getServer)
		anonymousGroup.GET("/server/tool", getServerTool)
		anonymousGroup.GET("/server/metrics", getServerMetrics)
		anonymousGroup.GET("/guild", listGuilds)
		anonymousGroup.GET("/guild/:admin_player_uid", getGuild)
		anonymousGroup.GET("/base-camps", listBaseCampsWithSettings(settingsManager))
		anonymousGroup.GET("/base-camps/:base_id", getBaseCampWithSettings(settingsManager))
		anonymousGroup.GET("/base-camps/:base_id/work-pals", listBaseWorkers)
		anonymousGroup.GET("/base-camps/:base_id/feed-boxes", listFeedBoxes)
		anonymousGroup.GET("/inventory/public-summary", publicInventorySummary)
	}
	// 根据登录状态返回不同结果
	OptionalGroup := apiGroup.Group("")
	OptionalGroup.Use(auth.OptionalJWTMiddleware())
	{
		OptionalGroup.GET("/online_player", listOnlinePlayers)
		OptionalGroup.GET("/player", listPlayers)
		OptionalGroup.GET("/player/:player_uid", getPlayer)
	}

	authGroup := apiGroup.Group("")
	authGroup.Use(auth.JWTAuthMiddleware())
	authGroup.Use(operationAuditMiddleware())
	{
		authGroup.GET("/logs", listRuntimeLogs)
		authGroup.GET("/audit", listOperationAudits)
		authGroup.POST("/server/broadcast", publishBroadcast)
		authGroup.POST("/server/shutdown", shutdownServer)
		authGroup.GET("/server/process", getServerProcess(processManager))
		authGroup.POST("/server/save", saveServer(processManager))
		authGroup.POST("/server/start", startServer(processManager))
		authGroup.POST("/server/restart", restartServer(processManager))
		authGroup.POST("/server/stop", stopServer(processManager))
		authGroup.POST("/server/watchdog", setServerWatchdog(processManager))
		authGroup.GET("/server/update", getServerUpdate(processManager))
		authGroup.POST("/server/update/check", checkServerUpdate(processManager))
		authGroup.POST("/server/update/apply", applyServerUpdate(processManager))
		authGroup.POST("/server/restart-schedule/preview", previewServerRestartSchedule)
		authGroup.GET("/base-camps/aliases", listBaseAliases)
		authGroup.PUT("/base-camps/:base_id/alias", putBaseAlias)
		authGroup.DELETE("/base-camps/:base_id/alias", deleteBaseAlias)
		authGroup.PUT("/player", putPlayers)
		authGroup.GET("/player-progress", listPlayerProgress)
		authGroup.POST("/player/:player_uid/kick", kickPlayer)
		authGroup.POST("/player/:player_uid/ban", banPlayer)
		authGroup.POST("/player/:player_uid/unban", unbanPlayer)
		authGroup.PUT("/guild", putGuilds)
		authGroup.PUT("/snapshot", putSnapshot)
		authGroup.GET("/inventory/summary", inventorySummary)
		authGroup.GET("/inventory/items/:item_id/locations", inventoryItemLocations)
		authGroup.GET("/inventory/containers", inventoryContainers)
		authGroup.GET("/breeding-farms/capabilities", getBreedingCapabilities)
		authGroup.GET("/breeding-farms/notification-config", getBreedingNotificationConfig)
		authGroup.PUT("/breeding-farms/notification-config", putBreedingNotificationConfig)
		authGroup.GET("/breeding-farms/events", listBreedingEvents)
		authGroup.GET("/breeding-farms/events/unread", listUnreadBreedingEvents)
		authGroup.POST("/breeding-farms/events/read-all", markAllBreedingEventsRead)
		authGroup.POST("/breeding-farms/events/:event_id/read", markBreedingEventRead)
		authGroup.GET("/breeding-farms", listBreedingFarms)
		authGroup.GET("/breeding-farms/:farm_id", getBreedingFarm)
		authGroup.GET("/breeding-farms/:farm_id/parents", getBreedingParents)
		authGroup.GET("/breeding-farms/:farm_id/cakes", getBreedingCakes)
		authGroup.GET("/breeding-farms/:farm_id/eggs", getBreedingEggs)
		authGroup.POST("/sync", syncData)
		authGroup.GET("/whitelist", listWhite)
		authGroup.POST("/whitelist", addWhite)
		authGroup.DELETE("/whitelist", removeWhite)
		authGroup.PUT("/whitelist", putWhite)
		authGroup.GET("/rcon", listRconCommand)
		authGroup.POST("/rcon", addRconCommand)
		authGroup.POST("/rcon/import", importRconCommands)
		authGroup.POST("/rcon/send", sendRconCommand)
		authGroup.GET("/rcon/tasks", listRconTasks)
		authGroup.POST("/rcon/tasks", addRconTask)
		authGroup.PUT("/rcon/tasks/:uuid", putRconTask)
		authGroup.DELETE("/rcon/tasks/:uuid", removeRconTask)
		authGroup.POST("/rcon/tasks/:uuid/run", runRconTask)
		authGroup.PUT("/rcon/:uuid", putRconCommand)
		authGroup.DELETE("/rcon/:uuid", removeRconCommand)
		authGroup.GET("/backup", listBackups)
		authGroup.POST("/backup", createBackup(processManager))
		authGroup.GET("/backup/:backup_id", downloadBackup)
		authGroup.DELETE("/backup/:backup_id", deleteBackup)
		authGroup.GET("/config", getConfig)
		authGroup.PUT("/config", putConfigWithSupervisor(processManager))
		authGroup.GET("/config/directories", listDirectories)
		authGroup.POST("/config/test/save", testSaveConfig)
		authGroup.POST("/config/test/rcon", testRconConfig)
		authGroup.GET("/world-settings/schema", getWorldSettingsSchema(settingsManager))
		authGroup.GET("/world-settings", getWorldSettings(settingsManager))
		authGroup.POST("/world-settings/validate", validateWorldSettings(settingsManager))
		authGroup.POST("/world-settings/apply", applyWorldSettings(settingsManager))
		authGroup.GET("/world-settings/backups", listWorldSettingsBackups(settingsManager))
		authGroup.POST("/world-settings/backups/:backup_id/restore", restoreWorldSettingsBackup(settingsManager))
		authGroup.DELETE("/world-settings/backups/:backup_id", deleteWorldSettingsBackup(settingsManager))
	}
}
