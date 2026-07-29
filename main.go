package main

import (
	"context"
	"embed"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/zaigie/palworld-server-tool/api"
	"github.com/zaigie/palworld-server-tool/docs"
	"github.com/zaigie/palworld-server-tool/internal/config"
	"github.com/zaigie/palworld-server-tool/internal/database"
	"github.com/zaigie/palworld-server-tool/internal/logger"
	"github.com/zaigie/palworld-server-tool/internal/production"
	"github.com/zaigie/palworld-server-tool/internal/supervisor"
	"github.com/zaigie/palworld-server-tool/internal/system"
	"github.com/zaigie/palworld-server-tool/internal/task"
	"github.com/zaigie/palworld-server-tool/internal/tool"
	"github.com/zaigie/palworld-server-tool/internal/worldsettings"
	"github.com/zaigie/palworld-server-tool/service"
)

var (
	version string = "Develop"
)

const startupPortEnvironment = "PST_PORT"

type startupPort struct {
	Port   int
	Source config.WebPortOverrideSource
}

type palServerController struct{}

func (palServerController) Save() error { return tool.Save() }

func (palServerController) Shutdown(seconds int, message string) error {
	return tool.Shutdown(seconds, message)
}

//go:embed assets/*
var assets embed.FS

//go:embed index.html
var indexHTML embed.FS

//go:embed pal-conf.html
var palConfHTML embed.FS

//go:embed map/*
var mapTiles embed.FS

//	@SecurityDefinitions.apikey	ApiKeyAuth
//	@in							header
//	@name						Authorization

// @license.name	Apache 2.0
// @license.url	http://www.apache.org/licenses/LICENSE-2.0.html
func main() {
	portOverride, err := startupPortOverride(os.Args[1:], os.LookupEnv, os.Stderr)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return
		}
		logger.Panic(err)
	}

	configStore, err := config.Open(config.DefaultDatabasePath)
	if err != nil {
		logger.Panic(err)
	}
	defer configStore.Close()
	config.SetCurrent(configStore)
	settings, err := applyStartupPortOverride(configStore, portOverride)
	if err != nil {
		logger.Panic(err)
	}
	config.SetRuntimeWeb(settings.Web)
	serverSupervisor := supervisor.New(
		settings.ServerProcess,
		supervisor.OSProcessLauncher{},
		supervisor.OSProcessDetector{},
		palServerController{},
	)
	if err := serverSupervisor.Bootstrap(); err != nil {
		logger.Errorf("Server process supervisor startup: %v\n", err)
	}

	db := database.GetDB()
	defer db.Close()
	productionManager, err := production.NewManager(db, serverSupervisor, func(source string) error {
		_, backupErr := tool.BackupRecord(source)
		return backupErr
	})
	if err != nil {
		logger.Panic(err)
	}
	defer func() {
		serverSupervisor.Close()
		productionManager.Close()
	}()
	settingsManager := worldsettings.NewManager(configStore, serverSupervisor, func(ctx context.Context) error {
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		var lastErr error
		for {
			if _, err := tool.Info(); err == nil {
				return nil
			} else {
				lastErr = err
			}
			select {
			case <-ctx.Done():
				return fmt.Errorf("official REST API did not become healthy: %w", lastErr)
			case <-ticker.C:
			}
		}
	}, service.NewWorldSettingsAuditStore(db))

	docs.SwaggerInfo.Title = "Palworld Manage API"
	docs.SwaggerInfo.Version = version
	docs.SwaggerInfo.Host = fmt.Sprintf("127.0.0.1:%d", settings.Web.Port)
	docs.SwaggerInfo.BasePath = "/"
	docs.SwaggerInfo.Schemes = []string{"http"}

	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("version", version)
		c.Next()
	})
	startScheduler := func() {
		go task.Schedule(db)
	}
	api.RegisterRouterWithProductionManager(router, startScheduler, serverSupervisor, settingsManager, productionManager)

	assetsFS, _ := fs.Sub(assets, "assets")
	router.StaticFS("/assets", http.FS(assetsFS))

	mapTilesFS, _ := fs.Sub(mapTiles, "map")
	router.StaticFS("/map/tiles", http.FS(mapTilesFS))

	serveWebApp := func(c *gin.Context) {
		c.Writer.WriteHeader(http.StatusOK)
		file, _ := indexHTML.ReadFile("index.html")
		c.Writer.Write(file)
	}
	router.GET("/", serveWebApp)
	for _, route := range []string{"/work-pals", "/players", "/world-map", "/pal-management", "/base-camps", "/inventory", "/world-settings", "/breeding-farms", "/server-operations", "/production-orders"} {
		router.GET(route, serveWebApp)
	}
	router.GET("/pal-conf", func(c *gin.Context) {
		c.Writer.WriteHeader(http.StatusOK)
		file, _ := palConfHTML.ReadFile("pal-conf.html")
		c.Writer.Write(file)
	})

	localIp, err := system.GetLocalIP()
	if err != nil {
		logger.Errorf("%v\n", err)
	}
	logger.Info("Starting PalWorld Server Tool...\n")
	logger.Infof("Version: %s\n", version)
	logger.Infof("Listening on http://127.0.0.1:%d or http://%s:%d\n", settings.Web.Port, localIp, settings.Web.Port)
	logger.Infof("Swagger on http://127.0.0.1:%d/swagger/index.html\n", settings.Web.Port)

	defer task.Shutdown()
	if configStore.IsInitialized() {
		startScheduler()
	} else {
		logger.Warn("Administrator password is not initialized; scheduled tasks will start after configuration\n")
	}

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		if settings.Web.TLS {
			if err := router.RunTLS(fmt.Sprintf(":%d", settings.Web.Port), settings.Web.CertPath, settings.Web.KeyPath); err != nil {
				logger.Errorf("Server exited with TLS error: %v\n", err)
			}
		} else {
			if err := router.Run(fmt.Sprintf(":%d", settings.Web.Port)); err != nil {
				logger.Errorf("Server exited with error: %v\n", err)
			}
		}
	}()

	<-sigChan

	logger.Info("Server gracefully stopped\n")
}

func startupPortOverride(args []string, lookupEnv func(string) (string, bool), output io.Writer) (*startupPort, error) {
	flags := flag.NewFlagSet("pst", flag.ContinueOnError)
	flags.SetOutput(output)
	port := 0
	flags.IntVar(&port, "port", 0, "Web listening port (overrides PST_PORT and persists to config.db)")
	if err := flags.Parse(args); err != nil {
		return nil, err
	}
	if flags.NArg() > 0 {
		return nil, fmt.Errorf("unexpected argument: %s", strings.Join(flags.Args(), " "))
	}

	portSet := false
	flags.Visit(func(current *flag.Flag) {
		if current.Name == "port" {
			portSet = true
		}
	})
	if portSet {
		if err := config.ValidateWebPort(port); err != nil {
			return nil, fmt.Errorf("invalid --port: %w", err)
		}
		return &startupPort{Port: port, Source: config.WebPortOverrideCommandLine}, nil
	}

	rawPort, ok := lookupEnv(startupPortEnvironment)
	if !ok {
		return nil, nil
	}
	port, err := strconv.Atoi(strings.TrimSpace(rawPort))
	if err != nil {
		return nil, fmt.Errorf("invalid %s value %q: must be an integer", startupPortEnvironment, rawPort)
	}
	if err := config.ValidateWebPort(port); err != nil {
		return nil, fmt.Errorf("invalid %s value %q: %w", startupPortEnvironment, rawPort, err)
	}
	return &startupPort{Port: port, Source: config.WebPortOverrideEnvironment}, nil
}

func applyStartupPortOverride(store *config.Store, override *startupPort) (config.Config, error) {
	settings := store.Config()
	settings.Web.PortSource = config.WebPortOverrideNone
	if override != nil {
		settings.Web.Port = override.Port
		settings.Web.PortSource = override.Source
	}
	if err := store.Update(settings, ""); err != nil {
		return config.Config{}, fmt.Errorf("persist startup web port: %w", err)
	}
	return store.Config(), nil
}
