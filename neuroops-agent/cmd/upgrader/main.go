/*
 * NeurOps Agent — Upgrader / Watchdog
 * Copyright (c) NeurOps 2025. All rights reserved.
 *
 * The upgrader is a minimal, always-running watchdog that:
 *   1. Ensures the main agent process stays alive (restarts on crash)
 *   2. Polls the NeurOps product for available agent upgrades
 *   3. Downloads, verifies (SHA-256), and applies upgrades atomically
 *   4. Rolls back automatically if the new binary fails its health check
 *
 * It runs as a separate systemd unit with Restart=always so it survives
 * independent of the main agent process lifecycle.
 *
 * Upgrade flow:
 *   product signals new version available via ZMQ command "agent.upgrade"
 *   → upgrader downloads new binary to /neuroops/neuroops-agent/.new/
 *   → verifies SHA-256 checksum
 *   → stops main agent service
 *   → atomically renames binary (os.Rename is atomic on same filesystem)
 *   → starts main agent service
 *   → health-checks /health for 30s
 *   → if unhealthy: rolls back to previous binary and restarts
 */

package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/neuroops/agent/internal/config"
	"github.com/neuroops/agent/internal/logger"
	"github.com/neuroops/agent/internal/transport"
)

const (
	version          = "1.0.0"
	agentServiceName = "neuroops-agent.service"
	agentBinary      = "neuroops-agent"
	healthCheckURL   = "http://127.0.0.1:8765/health"
	healthCheckWait  = 30 * time.Second
	healthCheckInterval = 2 * time.Second
	watchInterval    = 10 * time.Second
	upgradeTopic     = "NEUROOPS.CMD."
)

func main() {
	configPath := flag.String("config", "config/agent.json", "Path to agent.json")
	flag.Parse()

	log := logger.New("upgrader", logger.INFO)
	log.Infof("NeurOps Upgrader v%s starting", version)

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
		os.Exit(1)
	}
	log.SetLevel(logger.Level(cfg.Agent.SystemLogLevel))

	u := &upgrader{cfg: cfg, log: log}

	// Subscribe to upgrade commands from product
	sub, err := transport.NewEventSubscriber(
		cfg.Agent.ProductHost,
		cfg.Agent.EventSubscriberPort,
		upgradeTopic,
		u.handleCommand,
		log.WithComponent("subscriber"),
	)
	if err != nil {
		log.Warnf("Could not create subscriber (product may be offline): %v", err)
	} else {
		if err := sub.Start(); err != nil {
			log.Warnf("Subscriber not connected: %v", err)
		}
	}

	// ── Watchdog loop ─────────────────────────────────────────────────────
	go u.watchdog()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	if sub != nil {
		sub.Shutdown()
	}
	log.Info("NeurOps Upgrader stopped")
}

// ── Upgrader ──────────────────────────────────────────────────────────────────

type upgrader struct {
	cfg *config.Config
	log *logger.Logger
}

// watchdog ensures the main agent service is running.
func (u *upgrader) watchdog() {
	ticker := time.NewTicker(watchInterval)
	defer ticker.Stop()

	for range ticker.C {
		if !u.isAgentRunning() {
			u.log.Warn("Agent process not running — restarting")
			u.startAgent()
		}
	}
}

func (u *upgrader) isAgentRunning() bool {
	cmd := exec.Command("systemctl", "is-active", "--quiet", agentServiceName)
	return cmd.Run() == nil
}

func (u *upgrader) startAgent() {
	out, err := exec.Command("systemctl", "start", agentServiceName).CombinedOutput()
	if err != nil {
		u.log.Errorf("Failed to start agent service: %v — %s", err, string(out))
	} else {
		u.log.Info("Agent service started by watchdog")
	}
}

func (u *upgrader) stopAgent() {
	out, err := exec.Command("systemctl", "stop", agentServiceName).CombinedOutput()
	if err != nil {
		u.log.Errorf("Failed to stop agent service: %v — %s", err, string(out))
	}
}

// ── Upgrade command handler ───────────────────────────────────────────────────

func (u *upgrader) handleCommand(event map[string]any) {
	eventType, _ := event["event.type"].(string)
	if eventType != "agent.upgrade" {
		return
	}

	downloadURL, _ := event["download.url"].(string)
	expectedHash, _ := event["sha256"].(string)
	newVersion, _   := event["version"].(string)

	if downloadURL == "" {
		u.log.Warn("agent.upgrade command missing download.url")
		return
	}

	u.log.Infof("Upgrade requested: v%s from %s", newVersion, downloadURL)
	go u.applyUpgrade(downloadURL, expectedHash, newVersion)
}

func (u *upgrader) applyUpgrade(downloadURL, expectedHash, newVersion string) {
	workDir := filepath.Join(".", ".upgrade")
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		u.log.Errorf("Cannot create upgrade work dir: %v", err)
		return
	}
	defer os.RemoveAll(workDir)

	newBinaryPath := filepath.Join(workDir, agentBinary+".new")

	// ── Step 1: Download ──────────────────────────────────────────────────
	u.log.Infof("Downloading new binary from %s", downloadURL)
	if err := u.download(downloadURL, newBinaryPath); err != nil {
		u.log.Errorf("Download failed: %v", err)
		return
	}

	// ── Step 2: Verify checksum ───────────────────────────────────────────
	if expectedHash != "" {
		if err := u.verifyHash(newBinaryPath, expectedHash); err != nil {
			u.log.Errorf("Checksum verification failed: %v", err)
			return
		}
		u.log.Info("Checksum verified OK")
	}

	if err := os.Chmod(newBinaryPath, 0o755); err != nil {
		u.log.Errorf("Cannot chmod new binary: %v", err)
		return
	}

	// ── Step 3: Back up current binary ────────────────────────────────────
	currentBinary := filepath.Join(".", agentBinary)
	backupBinary  := currentBinary + ".prev"
	if err := copyFile(currentBinary, backupBinary); err != nil {
		u.log.Warnf("Could not back up current binary: %v", err)
	}

	// ── Step 4: Stop agent, swap binary, start agent ──────────────────────
	u.log.Info("Stopping agent for upgrade")
	u.stopAgent()
	time.Sleep(3 * time.Second)

	u.log.Infof("Installing new binary v%s", newVersion)
	if err := os.Rename(newBinaryPath, currentBinary); err != nil {
		u.log.Errorf("Failed to install new binary: %v — rolling back", err)
		u.rollback(backupBinary, currentBinary)
		return
	}

	u.log.Info("Starting agent with new binary")
	u.startAgent()

	// ── Step 5: Health-check new binary ──────────────────────────────────
	u.log.Infof("Health-checking new agent for %v ...", healthCheckWait)
	if err := u.waitHealthy(healthCheckWait); err != nil {
		u.log.Errorf("New binary is unhealthy: %v — rolling back", err)
		u.stopAgent()
		u.rollback(backupBinary, currentBinary)
		u.startAgent()
		return
	}

	u.log.Infof("Upgrade to v%s completed successfully", newVersion)
	_ = os.Remove(backupBinary) // clean up backup
}

func (u *upgrader) rollback(backupPath, targetPath string) {
	u.log.Warn("Rolling back to previous binary")
	if err := os.Rename(backupPath, targetPath); err != nil {
		u.log.Errorf("ROLLBACK FAILED: %v — manual intervention required", err)
		return
	}
	u.log.Info("Rollback successful")
}

func (u *upgrader) download(url, dest string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("server returned HTTP %d", resp.StatusCode)
	}

	f, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = io.Copy(f, resp.Body)
	return err
}

func (u *upgrader) verifyHash(path, expected string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return err
	}

	got := hex.EncodeToString(h.Sum(nil))
	if got != expected {
		return fmt.Errorf("checksum mismatch: got %s, want %s", got, expected)
	}
	return nil
}

func (u *upgrader) waitHealthy(timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		resp, err := http.Get(healthCheckURL)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
		}
		time.Sleep(healthCheckInterval)
	}
	return fmt.Errorf("agent did not become healthy within %v", timeout)
}

// ── Utilities ─────────────────────────────────────────────────────────────────

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	return err
}
