package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"quantsaas/internal/saas/api"
	"quantsaas/internal/saas/app"
	"quantsaas/internal/saas/auth"
	"quantsaas/internal/saas/backtests"
	"quantsaas/internal/saas/config"
	"quantsaas/internal/saas/cron"
	"quantsaas/internal/saas/dashboard"
	"quantsaas/internal/saas/datalab"
	"quantsaas/internal/saas/epoch"
	"quantsaas/internal/saas/instance"
	"quantsaas/internal/saas/logger"
	"quantsaas/internal/saas/marketdata"
	"quantsaas/internal/saas/store"
	"quantsaas/internal/saas/ws"
)

func main() {
	var configPath string
	flag.StringVar(&configPath, "config", "config.yaml", "path to config file")
	flag.Parse()

	cfg, err := config.Load(configPath)
	if err != nil {
		panic(fmt.Errorf("load config: %w", err))
	}

	log, err := logger.New("quantsaas-saas")
	if err != nil {
		panic(fmt.Errorf("init logger: %w", err))
	}
	defer func() { _ = log.Sync() }()

	db, err := store.NewDB(cfg.Database)
	if err != nil {
		log.Fatal("connect database", logger.Error(err))
	}
	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			log.Error("close database", logger.Error(closeErr))
		}
	}()

	cache, err := store.NewRedis(cfg.Redis)
	if err != nil {
		log.Fatal("connect redis", logger.Error(err))
	}
	defer func() {
		if closeErr := cache.Close(); closeErr != nil {
			log.Error("close redis", logger.Error(closeErr))
		}
	}()

	authService, err := auth.NewService(db.GormDB(), cfg.JWT)
	if err != nil {
		log.Fatal("init auth service", logger.Error(err))
	}

	if err := app.SeedStrategyTemplates(context.Background(), db.GormDB()); err != nil {
		log.Fatal("seed strategy templates", logger.Error(err))
	}

	marketService := marketdata.NewService(db.GormDB())
	dataLabService := datalab.NewService(db.GormDB(), marketService)
	hub := ws.NewHub(db.GormDB(), authService, marketService, log, 0)
	instanceManager := instance.NewManager(db.GormDB(), cache, hub, marketService, log)
	backtestService := backtests.NewService(db.GormDB(), log)
	evolutionService := epoch.NewService(db.GormDB(), cache, log)
	dashboardService := dashboard.NewService(db.GormDB(), hub, cfg.AppRole)

	router := api.NewRouter(api.RouterDeps{
		Config:          cfg,
		Logger:          log,
		Cache:           cache,
		AuthService:     authService,
		Hub:             hub,
		Dashboard:       dashboardService,
		InstanceManager: instanceManager,
		Backtests:       backtestService,
		Evolution:       evolutionService,
		DataLab:         dataLabService,
	})

	var scheduler *cron.Scheduler
	if cfg.AppRole == config.RoleSaaS || cfg.AppRole == config.RoleDev {
		scheduler = cron.NewScheduler(instanceManager, log, time.Minute)
		scheduler.Start()
	}

	server := &http.Server{
		Addr:              cfg.Server.Address(),
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       time.Duration(cfg.Server.ReadTimeoutSeconds) * time.Second,
		WriteTimeout:      time.Duration(cfg.Server.WriteTimeoutSeconds) * time.Second,
	}

	go func() {
		log.Info("saas server starting", logger.String("addr", server.Addr), logger.String("role", cfg.AppRole))
		if serveErr := server.ListenAndServe(); serveErr != nil && serveErr != http.ErrServerClosed {
			log.Fatal("server stopped unexpectedly", logger.Error(serveErr))
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	log.Info("shutting down saas server")
	if err := server.Shutdown(ctx); err != nil {
		log.Error("shutdown http server", logger.Error(err))
	}
	if scheduler != nil {
		if err := scheduler.Stop(ctx); err != nil {
			log.Error("stop scheduler", logger.Error(err))
		}
	}
	if err := hub.Shutdown(ctx); err != nil {
		log.Error("shutdown websocket hub", logger.Error(err))
	}
}
