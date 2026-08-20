/*
 * NeurOps Agent - Intelligent Observability Agent
 * Copyright (c) NeurOps 2025. All rights reserved.
 *
 * Entry point. Bootstraps configuration, wires all subsystems,
 * and starts the agent process. Handles OS signals for graceful shutdown.
 */

package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/neuroops/agent/internal/agent"
	"github.com/neuroops/agent/internal/config"
	"github.com/neuroops/agent/internal/logger"
)

const (
	version     = "1.0.0"
	productName = "NeurOps Agent"
	configFlag  = "config"
	defaultConf = "config/agent.json"
)

func main() {
	configPath := flag.String(configFlag, defaultConf, "Path to agent configuration file")
	showVersion := flag.Bool("version", false, "Print version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Printf("%s v%s\n", productName, version)
		os.Exit(0)
	}

	// ── Bootstrap logger at INFO before config loads ─────────────────────────
	log := logger.New("agent", logger.INFO)
	log.Info(fmt.Sprintf("Starting %s v%s", productName, version))

	// ── Load configuration ────────────────────────────────────────────────────
	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatal(fmt.Sprintf("Failed to load configuration: %v", err))
		os.Exit(1)
	}
	log.SetLevel(logger.Level(cfg.Agent.SystemLogLevel))
	log.Info(fmt.Sprintf("Configuration loaded from %s", *configPath))

	// ── Boot agent ────────────────────────────────────────────────────────────
	a, err := agent.New(cfg, log)
	if err != nil {
		log.Fatal(fmt.Sprintf("Failed to initialise agent: %v", err))
		os.Exit(1)
	}

	if err := a.Start(); err != nil {
		log.Fatal(fmt.Sprintf("Failed to start agent: %v", err))
		os.Exit(1)
	}

	// ── Wait for shutdown signal ──────────────────────────────────────────────
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)

	sig := <-quit
	log.Info(fmt.Sprintf("Received signal %s — initiating graceful shutdown", sig))
	a.Stop()
	log.Info("NeurOps Agent stopped cleanly")
}
