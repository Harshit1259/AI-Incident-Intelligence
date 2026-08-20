/*
 * NeurOps Agent — Plugin Engine
 * Copyright (c) NeurOps 2025. All rights reserved.
 *
 * Loads and executes metric and runbook plugins written in Go or Python.
 *
 * Security model:
 *   - Each plugin directory may contain a manifest.json with the SHA-256 hash
 *     of the plugin binary. The engine verifies this hash before every execution.
 *   - If NEUROOPS_STRICT_MANIFESTS=true, plugins without a manifest.json are
 *     rejected at discovery time. Otherwise a warning is logged.
 *   - Plugin processes run inside the sandbox package, which applies kernel-level
 *     isolation (Pdeathsig + Setpgid on Linux).
 *   - Plugin output is size-capped (512 KB default) to prevent memory exhaustion.
 *
 * Plugin contract (Go):
 *   Binary called as:   <binary> <base64(context_json)>
 *   Stdout must be:     <base64(result_json)>
 *   Exit 0 = success,  non-zero = failure (stderr is logged)
 *
 * Plugin contract (Python):
 *   Script called as:   python3 plugin.py <base64(context_json)>
 *   Same output contract as Go.
 *
 * Plugin types:
 *   metric   — called on a configurable poll interval; result published as metrics
 *   runbook  — called on-demand (triggered by product command); result published as runbook output
 *   topology — called periodically; result published as topology data
 */

package plugin

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/neuroops/agent/internal/config"
	"github.com/neuroops/agent/internal/logger"
	"github.com/neuroops/agent/internal/sandbox"
)

// Publisher minimal interface for plugin results.
type Publisher interface {
	Publish(event map[string]any) bool
}

// ── Plugin registry ───────────────────────────────────────────────────────────

// PluginMeta describes a discovered plugin on disk.
type PluginMeta struct {
	ID           string // directory name, e.g. "linux_cpu"
	Type         string // "metric" | "runbook" | "topology"
	Language     string // "go" | "python"
	Binary       string // absolute path to executable / .py file
	Dir          string // plugin directory
	ExpectedHash string // SHA-256 from manifest.json (empty if no manifest)
	HasManifest  bool   // whether manifest.json was present and parsed
}

// Engine discovers, schedules, and executes plugins.
type Engine struct {
	cfg       *config.MetricAgentConfig
	agentCfg  *config.AgentConfig
	pub       Publisher
	log       *logger.Logger
	plugins   []*PluginMeta
	stopCh    chan struct{}
	wg        sync.WaitGroup
	pythonBin string
}

// New creates a plugin Engine.
func New(cfg *config.MetricAgentConfig, agentCfg *config.AgentConfig, pub Publisher, log *logger.Logger) *Engine {
	return &Engine{
		cfg:       cfg,
		agentCfg:  agentCfg,
		pub:       pub,
		log:       log,
		stopCh:    make(chan struct{}),
		pythonBin: resolvePython(),
	}
}

// Discover scans all configured plugin directories and registers plugins.
func (e *Engine) Discover() error {
	e.plugins = nil

	for _, dir := range e.cfg.PluginDirectories {
		if err := e.scanDir(dir); err != nil {
			e.log.Warnf("Plugin scan error in %s: %v", dir, err)
		}
	}

	e.log.Infof("Plugin engine discovered %d plugins", len(e.plugins))
	return nil
}

// Start launches polling goroutines for metric plugins.
// Runbook plugins are invoked on-demand via Execute().
func (e *Engine) Start() error {
	if err := e.Discover(); err != nil {
		return err
	}

	for _, p := range e.plugins {
		if p.Type == "metric" || p.Type == "topology" {
			e.wg.Add(1)
			go e.scheduledRun(p)
		}
	}

	e.log.Info("Plugin engine started")
	return nil
}

// Stop halts all plugin goroutines.
func (e *Engine) Stop() {
	close(e.stopCh)
	e.wg.Wait()
	e.log.Info("Plugin engine stopped")
}

// Execute runs a named runbook plugin immediately with the given context.
// Returns the result map or an error.
func (e *Engine) Execute(pluginID string, ctx map[string]any) (map[string]any, error) {
	for _, p := range e.plugins {
		if p.ID == pluginID && p.Type == "runbook" {
			return e.run(p, ctx)
		}
	}
	return nil, fmt.Errorf("runbook plugin %q not found", pluginID)
}

// ── Discovery ─────────────────────────────────────────────────────────────────

func (e *Engine) scanDir(root string) error {
	entries, err := os.ReadDir(root)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		pluginDir := filepath.Join(root, entry.Name())
		meta, err := e.loadMeta(pluginDir, entry.Name())
		if err != nil {
			e.log.Warnf("Skipping plugin %s: %v", pluginDir, err)
			continue
		}
		e.plugins = append(e.plugins, meta)
		e.log.Debugf("Registered plugin: %s (type=%s lang=%s manifest=%v)", meta.ID, meta.Type, meta.Language, meta.HasManifest)
	}

	return nil
}

func (e *Engine) loadMeta(dir, name string) (*PluginMeta, error) {
	pType := pluginTypeFromPath(dir)

	binary := filepath.Join(dir, "plugin")
	pyFile := filepath.Join(dir, "plugin.py")

	var binaryPath, language string
	if fileExists(binary) {
		binaryPath = binary
		language = "go"
	} else if fileExists(pyFile) {
		binaryPath = pyFile
		language = "python"
	} else {
		return nil, fmt.Errorf("no plugin binary or plugin.py found in %s", dir)
	}

	meta := &PluginMeta{
		ID:       name,
		Type:     pType,
		Language: language,
		Binary:   binaryPath,
		Dir:      dir,
	}

	// Load and verify manifest integrity at discovery time.
	if err := e.checkManifest(meta); err != nil {
		return nil, err
	}

	return meta, nil
}

// checkManifest loads the plugin's manifest.json and verifies the binary hash.
// If no manifest is present:
//   - STRICT mode (NEUROOPS_STRICT_MANIFESTS=true): returns error (plugin rejected)
//   - Normal mode: logs a warning and allows the plugin (development convenience)
func (e *Engine) checkManifest(meta *PluginMeta) error {
	manifest, err := loadManifest(meta.Dir)
	if err != nil {
		return fmt.Errorf("manifest error for plugin %s: %w", meta.ID, err)
	}

	if manifest == nil {
		// No manifest.json present.
		if strictManifestsEnabled() {
			return fmt.Errorf("plugin %s has no manifest.json and NEUROOPS_STRICT_MANIFESTS=true", meta.ID)
		}
		e.log.Warnf("Security: plugin %s/%s has no manifest.json — integrity unverified (set NEUROOPS_STRICT_MANIFESTS=true to enforce)", meta.Dir, meta.ID)
		return nil
	}

	// Verify the binary matches the hash in the manifest.
	if err := verifyBinaryIntegrity(meta.Binary, manifest.SHA256); err != nil {
		return fmt.Errorf("plugin %s failed integrity check: %w", meta.ID, err)
	}

	meta.ExpectedHash = manifest.SHA256
	meta.HasManifest = true
	e.log.Debugf("Plugin %s integrity verified (sha256=%s...)", meta.ID, manifest.SHA256[:12])
	return nil
}

// ── Execution ─────────────────────────────────────────────────────────────────

func (e *Engine) scheduledRun(p *PluginMeta) {
	defer e.wg.Done()

	interval := time.Duration(e.cfg.CPUMemoryPollSeconds) * time.Second
	if p.Type == "topology" {
		interval = time.Duration(e.cfg.SystemInfoPollSeconds) * time.Second
	}

	e.runAndPublish(p)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-e.stopCh:
			return
		case <-ticker.C:
			e.runAndPublish(p)
		}
	}
}

func (e *Engine) runAndPublish(p *PluginMeta) {
	ctx := e.buildContext(p)
	result, err := e.run(p, ctx)
	if err != nil {
		e.log.Warnf("Plugin %s execution error: %v", p.ID, err)
		return
	}
	e.publishResult(p, result)
}

func (e *Engine) run(p *PluginMeta, ctx map[string]any) (map[string]any, error) {
	// Re-verify binary integrity before each execution if a manifest is present.
	// This guards against binary replacement between the discovery scan and runtime.
	if p.HasManifest && p.ExpectedHash != "" {
		if err := verifyBinaryIntegrity(p.Binary, p.ExpectedHash); err != nil {
			return nil, fmt.Errorf("plugin %s pre-execution integrity check failed: %w", p.ID, err)
		}
	}

	ctxJSON, err := json.Marshal(ctx)
	if err != nil {
		return nil, fmt.Errorf("marshalling context: %w", err)
	}
	ctxB64 := base64.StdEncoding.EncodeToString(ctxJSON)

	// Build argument list for the sandbox executor.
	var name string
	var args []string
	switch p.Language {
	case "go":
		name = p.Binary
		args = []string{ctxB64}
	case "python":
		name = e.pythonBin
		args = []string{p.Binary, ctxB64}
	default:
		return nil, fmt.Errorf("unknown plugin language: %s", p.Language)
	}

	// Execute inside sandbox with process isolation and timeout.
	execCtx := context.Background()
	out, err := sandbox.Execute(execCtx, name, args, sandbox.Options{
		Timeout:     30 * time.Second,
		MaxOutputKB: 512,
	})

	// Restore env variables the plugin expects.
	_ = os.Setenv("NEUROOPS_PLUGIN_DIR", p.Dir)
	_ = os.Setenv("NEUROOPS_PLUGIN_ID", p.ID)
	_ = os.Setenv("NEUROOPS_PLUGIN_TYPE", p.Type)

	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return nil, fmt.Errorf("plugin exited %d: %s", exitErr.ExitCode(), string(exitErr.Stderr))
		}
		return nil, fmt.Errorf("running plugin: %w", err)
	}

	// Decode base64-wrapped result (convenience: also accept raw JSON).
	decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(out)))
	if err != nil {
		decoded = out
	}

	result := make(map[string]any)
	if err := json.Unmarshal(decoded, &result); err != nil {
		return nil, fmt.Errorf("parsing plugin output: %w", err)
	}

	return result, nil
}

func (e *Engine) publishResult(p *PluginMeta, result map[string]any) {
	event := map[string]any{
		"event.type": p.Type,
		"plugin.id":  p.ID,
		"agent.id":   e.agentCfg.AgentID,
		"timestamp":  time.Now().Unix(),
	}
	for k, v := range result {
		event[k] = v
	}
	e.pub.Publish(event)
}

func (e *Engine) buildContext(p *PluginMeta) map[string]any {
	return map[string]any{
		"plugin.id":   p.ID,
		"plugin.type": p.Type,
		"agent.id":    e.agentCfg.AgentID,
		"object.ip":   e.agentCfg.ProductHost,
	}
}

// ── Utilities ─────────────────────────────────────────────────────────────────

func pluginTypeFromPath(dir string) string {
	dir = filepath.ToSlash(dir)
	parts := strings.Split(dir, "/")
	for _, p := range parts {
		switch p {
		case "metric":
			return "metric"
		case "runbook":
			return "runbook"
		case "topology":
			return "topology"
		}
	}
	return "metric"
}

func resolvePython() string {
	candidates := []string{
		"/neuroops/python-embedded/bin/python3",
		"/motadata/python-embedded/bin/python3",
		"python3",
		"python",
	}
	for _, c := range candidates {
		if path, err := exec.LookPath(c); err == nil {
			return path
		}
		if fileExists(c) {
			return c
		}
	}
	return "python3"
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
