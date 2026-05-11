package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	agentconfig "quantsaas/internal/agent/config"
	"quantsaas/internal/agent/exchange"
	agentws "quantsaas/internal/agent/ws"
	"quantsaas/internal/saas/logger"
)

func main() {
	var configPath string
	flag.StringVar(&configPath, "config", "config.agent.yaml", "path to agent config")
	flag.Parse()

	cfg, err := agentconfig.Load(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load agent config: %v\n", err)
		os.Exit(1)
	}

	log, err := logger.New("quantsaas-agent")
	if err != nil {
		fmt.Fprintf(os.Stderr, "init logger: %v\n", err)
		os.Exit(1)
	}
	defer func() { _ = log.Sync() }()

	executor, err := exchange.NewBitgetClient(cfg.Exchange, log)
	if err != nil {
		fmt.Fprintf(os.Stderr, "init exchange client: %v\n", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	client := agentws.NewClient(cfg, executor, log)
	if err := client.Run(ctx); err != nil && err != context.Canceled {
		fmt.Fprintf(os.Stderr, "agent stopped with error: %v\n", err)
		os.Exit(1)
	}
}
