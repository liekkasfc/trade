package api

import (
	"time"

	"quantsaas/internal/saas/auth"
	"quantsaas/internal/saas/backtests"
	"quantsaas/internal/saas/config"
	"quantsaas/internal/saas/dashboard"
	"quantsaas/internal/saas/datalab"
	"quantsaas/internal/saas/epoch"
	"quantsaas/internal/saas/instance"
	"quantsaas/internal/saas/store"
	"quantsaas/internal/saas/ws"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

type RouterDeps struct {
	Config          config.Config
	Logger          *zap.Logger
	Cache           *store.Cache
	AuthService     *auth.Service
	Hub             *ws.Hub
	Dashboard       *dashboard.Service
	InstanceManager *instance.Manager
	Backtests       *backtests.Service
	Evolution       *epoch.Service
	DataLab         *datalab.Service
}

func NewRouter(deps RouterDeps) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(gin.Recovery())
	router.Use(cors())
	router.Use(requestLogger(deps.Logger))

	router.GET("/healthz", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"status":   "ok",
			"app_role": deps.Config.AppRole,
			"time":     time.Now().UTC().Format(time.RFC3339),
		})
	})

	if deps.Hub != nil {
		router.GET("/ws/agent", deps.Hub.HandleConnection)
	}

	v1 := router.Group("/api/v1")
	authGroup := v1.Group("/auth")
	{
		authGroup.POST("/register", handleRegister(deps))
		authGroup.POST("/login", handleLogin(deps))
		authGroup.GET("/me", auth.RequireAuth(deps.AuthService), handleMe())
	}

	userGroup := v1.Group("/")
	userGroup.Use(auth.RequireAuth(deps.AuthService))
	{
		userGroup.GET("/strategies", handleListStrategies(deps.InstanceManager))
		userGroup.GET("/strategies/:id", handleGetStrategy(deps.InstanceManager))
		userGroup.GET("/instances", handleListInstances(deps.InstanceManager))
		userGroup.POST("/instances", handleCreateInstance(deps.InstanceManager))
		userGroup.POST("/instances/:id/start", handleStartInstance(deps.InstanceManager))
		userGroup.POST("/instances/:id/stop", handleStopInstance(deps.InstanceManager))
		userGroup.DELETE("/instances/:id", handleDeleteInstance(deps.InstanceManager))
		userGroup.GET("/instances/:id/lots", handleListInstanceLots(deps.InstanceManager))
		userGroup.GET("/instances/:id/trades", handleListInstanceTrades(deps.InstanceManager))
		userGroup.GET("/dashboard", handleDashboard(deps.Dashboard))
		userGroup.GET("/dashboard/equity-snapshots", handleEquitySnapshots(deps.Dashboard))
		userGroup.GET("/system/status", handleSystemStatus(deps.Dashboard))
		userGroup.GET("/agents/status", handleAgentStatus(deps.Hub))
	}

	labGroup := userGroup.Group("/")
	labGroup.Use(requireLabRole(deps.Config))
	{
		labGroup.POST("/backtests", handleCreateBacktest(deps.Backtests))
		labGroup.GET("/backtests/:id", handleGetBacktest(deps.Backtests))
		labGroup.POST("/evolution/tasks", handleCreateEvolutionTask(deps.Evolution))
		labGroup.GET("/evolution/tasks", handleListEvolutionTasks(deps.Evolution))
		labGroup.GET("/evolution/genomes", handleListEvolutionGenomes(deps.Evolution))
		labGroup.POST("/evolution/tasks/:id/promote", handlePromoteEvolutionTask(deps.Evolution))
		labGroup.GET("/genome/champion", handleGetChampionGenome(deps.Evolution))
		labGroup.GET("/genome/challengers", handleListChallengerGenomes(deps.Evolution))
		labGroup.POST("/data-lab/sync", handleSyncDataLab(deps.DataLab))
		labGroup.POST("/data-lab/import-csv", handleImportDataLabCSV(deps.DataLab))
		labGroup.GET("/data-lab/coverage", handleGetDataLabCoverage(deps.DataLab))
		labGroup.GET("/data-lab/recent", handleGetDataLabRecent(deps.DataLab))
	}

	return router
}

func requestLogger(log *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()

		log.Info(
			"http request",
			zap.String("method", c.Request.Method),
			zap.String("path", c.Request.URL.Path),
			zap.Int("status", c.Writer.Status()),
			zap.Duration("latency", time.Since(start)),
		)
	}
}

func requireLabRole(cfg config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		if cfg.AppRole != config.RoleLab && cfg.AppRole != config.RoleDev {
			c.AbortWithStatusJSON(403, gin.H{"error": "lab routes are disabled for current app role"})
			return
		}
		c.Next()
	}
}

func cors() gin.HandlerFunc {
	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		if origin == "" {
			origin = "*"
		}
		c.Header("Access-Control-Allow-Origin", origin)
		c.Header("Access-Control-Allow-Headers", "Authorization, Content-Type")
		c.Header("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
		c.Header("Vary", "Origin")
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}
		c.Next()
	}
}
