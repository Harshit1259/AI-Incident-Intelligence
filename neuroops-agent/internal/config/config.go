/*
 * NeurOps Agent — Configuration
 * Copyright (c) NeurOps 2025. All rights reserved.
 *
 * Loads, validates, and hot-reloads agent.json.
 * All subsystems read config through this package so a single reload
 * propagates everywhere without restart.
 */

package config

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
)

// ── Top-level config envelope ─────────────────────────────────────────────────

// Config is the root configuration structure, mirrors agent.json exactly.
type Config struct {
	Agent       AgentConfig       `json:"agent"`
	MetricAgent MetricAgentConfig `json:"metric.agent"`
	LogAgent    LogAgentConfig    `json:"log.agent"`
	TraceAgent  TraceAgentConfig  `json:"trace.agent"`
	FlowAgent   FlowAgentConfig   `json:"flow.agent"`
}

// AgentConfig holds global agent settings.
type AgentConfig struct {
	// Core identity
	AgentID   string `json:"agent.id"`
	AgentName string `json:"agent.name"`

	// Runtime
	SystemLogLevel int    `json:"system.log.level"`
	AgentState     string `json:"agent.state"` // "ENABLE" | "DISABLE"

	// Transport — ZeroMQ
	ProductHost           string `json:"neuroops.product.host"`
	EventPublisherPort    int    `json:"neuroops.event.publisher.port"`  // agent → product
	EventSubscriberPort   int    `json:"neuroops.event.subscriber.port"` // product → agent

	// Sub-agent toggles
	MetricAgentEnabled bool `json:"metric.agent.enabled"`
	LogAgentEnabled    bool `json:"log.agent.enabled"`
	TraceAgentEnabled  bool `json:"trace.agent.enabled"`
	FlowAgentEnabled   bool `json:"flow.agent.enabled"`

	// Cache
	CacheFlushSeconds     int `json:"cache.flush.timer.seconds"`
	CacheMaxSizeMB        int `json:"cache.file.max.size.threshold.mb"`

	// HTTP health endpoint
	HealthPort int `json:"health.port"` // 0 = disabled

	// Self-monitoring thresholds
	MetricAgentMemoryWarningMB  int `json:"metric.agent.memory.warning.threshold.mb"`
	MetricAgentMemoryCriticalMB int `json:"metric.agent.memory.critical.threshold.mb"`
	MetricAgentCPUWarningPct    int `json:"metric.agent.cpu.warning.percent"`
	MetricAgentCPUCriticalPct   int `json:"metric.agent.cpu.critical.percent"`

	// HTTP event forwarder (to AI Incident Platform)
	HTTPForwarderEnabled bool   `json:"http.forwarder.enabled"`
	HTTPForwarderURL     string `json:"http.forwarder.url"`

	// Security (TLS to product)
	TLSEnabled  bool   `json:"tls.enabled"`
	TLSCertFile string `json:"tls.cert.file"`
	TLSKeyFile  string `json:"tls.key.file"`
	TLSCAFile   string `json:"tls.ca.file"`
}

// MetricAgentConfig controls all metric collection behaviour.
type MetricAgentConfig struct {
	SystemLogLevel int `json:"system.log.level"`

	// Poll intervals (seconds)
	CPUMemoryPollSeconds   int `json:"cpu.memory.metric.poll.seconds"`
	DiskPollSeconds        int `json:"disk.metric.poll.seconds"`
	NetworkPollSeconds     int `json:"network.metric.poll.seconds"`
	ProcessPollSeconds     int `json:"process.metric.poll.seconds"`
	ServicePollSeconds     int `json:"service.metric.poll.seconds"`
	SystemInfoPollSeconds  int `json:"system.info.poll.seconds"`
	SystemLoadPollSeconds  int `json:"system.load.poll.seconds"`

	// Feature toggles
	CPUMemoryEnabled  bool `json:"cpu.memory.metric.enabled"`
	DiskEnabled       bool `json:"disk.metric.enabled"`
	NetworkEnabled    bool `json:"network.metric.enabled"`
	ProcessEnabled    bool `json:"process.metric.enabled"`
	ProcessConnEnabled bool `json:"process.connection.enabled"`
	ServiceEnabled    bool `json:"service.metric.enabled"`
	SystemInfoEnabled bool `json:"system.info.metric.enabled"`
	SystemLoadEnabled bool `json:"system.load.metric.enabled"`

	// Scoped collections (empty = collect all)
	MonitoredProcesses  []string `json:"processes"`
	MonitoredServices   []string `json:"services"`
	MonitoredDisks      []string `json:"disks"`
	MonitoredInterfaces []string `json:"interfaces"`

	// Plugin directories
	PluginDirectories []string `json:"plugin.directories"`

	CacheFlushSeconds int `json:"cache.flush.timer.seconds"`
}

// LogAgentConfig controls log collection.
type LogAgentConfig struct {
	SystemLogLevel int `json:"system.log.level"`

	// File tailing
	LogDirectories []LogDirectory `json:"log.dirs"`
	MultilineFiles []MultilineLogFile `json:"multiline.log.files"`
	MultilineEnabled bool `json:"multiline.log.enabled"`

	// Windows Event Log
	EventLogEnabled bool         `json:"event.log.source.enabled"`
	EventLogSources []EventLogSource `json:"event.log.sources"`

	// Behaviour
	ReadExistingEvents       bool `json:"read.existing.events"`
	WatchCreateEvents        bool `json:"watcher.create.event"`
	MaxWorkers               int  `json:"max.workers"`
	WorkerMaxQueueSize       int  `json:"worker.max.queue.size"`
	WorkerMaxPageSizeBytes   int  `json:"worker.max.page.size.bytes"`
	CacheMaxSizeBytes        int  `json:"cache.max.size.bytes"`
	CacheFlushSeconds        int  `json:"cache.flush.timer.seconds"`
	PositionWriteSeconds     int  `json:"log.position.write.timer.seconds"`
	IgnoreInvalidLogFile     bool `json:"ignore.invalid.log.file"`

	// Character escaping applied to log lines before shipping
	EscapeCharacters map[string]string `json:"escape.characters"`

	// Log-parser plugin directories
	ParserPluginDirectories []string `json:"parser.plugin.directories"`

	// Filters: drop lines matching these patterns
	LogFilters []string `json:"log.filters"`
}

type LogDirectory struct {
	Path    string `json:"path"`
	Pattern string `json:"pattern"` // glob, e.g. "*.log"
	Tag     string `json:"tag"`     // label added to every shipped record
}

type MultilineLogFile struct {
	Path           string `json:"path"`
	StartPattern   string `json:"start.pattern"`
}

type EventLogSource struct {
	Name   string `json:"name"`
	Levels []int  `json:"levels"`
	Events []int  `json:"events"` // empty = all events
}

// TraceAgentConfig wires into the embedded OTel collector.
type TraceAgentConfig struct {
	Enabled       bool   `json:"enabled"`
	OTLPEndpoint  string `json:"otlp.endpoint"` // listener, e.g. "0.0.0.0:4318"
	ExportEndpoint string `json:"export.endpoint"` // product OTLP endpoint
}

// FlowAgentConfig controls network flow (packet) collection.
type FlowAgentConfig struct {
	Enabled      bool     `json:"enabled"`
	Interfaces   []string `json:"interfaces"`
	SampleRate   int      `json:"sample.rate"`
	CaptureBytes int      `json:"capture.bytes"`
}

// ── Loader + hot-reload ───────────────────────────────────────────────────────

// Manager holds the current configuration and supports atomic reloads.
type Manager struct {
	mu       sync.RWMutex
	current  *Config
	filePath string
}

var defaultConfig = &Config{
	Agent: AgentConfig{
		SystemLogLevel:         2,
		AgentState:             "ENABLE",
		ProductHost:            "127.0.0.1",
		EventPublisherPort:     9441,
		EventSubscriberPort:    9440,
		MetricAgentEnabled:     true,
		LogAgentEnabled:        false,
		TraceAgentEnabled:      false,
		FlowAgentEnabled:       false,
		CacheFlushSeconds:      30,
		CacheMaxSizeMB:         1024,
		HealthPort:             8765,
		MetricAgentMemoryWarningMB:  200,
		MetricAgentMemoryCriticalMB: 500,
		MetricAgentCPUWarningPct:    50,
		MetricAgentCPUCriticalPct:   80,
		HTTPForwarderEnabled:        true,
		HTTPForwarderURL:            "http://localhost:8080",
	},
	MetricAgent: MetricAgentConfig{
		SystemLogLevel:        2,
		CPUMemoryPollSeconds:  300,
		DiskPollSeconds:       300,
		NetworkPollSeconds:    300,
		ProcessPollSeconds:    300,
		ServicePollSeconds:    300,
		SystemInfoPollSeconds: 3600,
		SystemLoadPollSeconds: 300,
		CPUMemoryEnabled:      true,
		DiskEnabled:           true,
		NetworkEnabled:        true,
		ProcessEnabled:        true,
		ServiceEnabled:        true,
		SystemInfoEnabled:     true,
		SystemLoadEnabled:     true,
		CacheFlushSeconds:     10,
	},
	LogAgent: LogAgentConfig{
		SystemLogLevel:           2,
		MultilineEnabled:         true,
		EventLogEnabled:          true,
		ReadExistingEvents:       true,
		WatchCreateEvents:        true,
		MaxWorkers:               2,
		WorkerMaxQueueSize:       50,
		WorkerMaxPageSizeBytes:   4096,
		CacheMaxSizeBytes:        10240,
		CacheFlushSeconds:        5,
		PositionWriteSeconds:     5,
		IgnoreInvalidLogFile:     true,
		EscapeCharacters:         map[string]string{"\n": " ", "\r": "", "\t": " "},
	},
	TraceAgent: TraceAgentConfig{
		Enabled:        false,
		OTLPEndpoint:   "0.0.0.0:4318",
		ExportEndpoint: "http://127.0.0.1:4317",
	},
	FlowAgent: FlowAgentConfig{
		Enabled:      false,
		SampleRate:   1,
		CaptureBytes: 128,
	},
}

// Load reads agent.json from disk, merges with defaults, validates.
func Load(path string) (*Config, error) {
	cfg := clone(defaultConfig)

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil // first run — use defaults
		}
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}

	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}

	if err := validate(cfg); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	return cfg, nil
}

// Reload atomically replaces the current config.  Called by the file watcher.
func (m *Manager) Reload() error {
	cfg, err := Load(m.filePath)
	if err != nil {
		return err
	}
	m.mu.Lock()
	m.current = cfg
	m.mu.Unlock()
	return nil
}

// Get returns the current config snapshot (safe for concurrent reads).
func (m *Manager) Get() *Config {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.current
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func validate(cfg *Config) error {
	if cfg.Agent.EventPublisherPort <= 0 || cfg.Agent.EventPublisherPort > 65535 {
		return fmt.Errorf("agent.neuroops.event.publisher.port must be 1-65535")
	}
	if cfg.Agent.EventSubscriberPort <= 0 || cfg.Agent.EventSubscriberPort > 65535 {
		return fmt.Errorf("agent.neuroops.event.subscriber.port must be 1-65535")
	}
	if cfg.Agent.ProductHost == "" {
		return fmt.Errorf("agent.neuroops.product.host must not be empty")
	}
	return nil
}

func clone(src *Config) *Config {
	data, _ := json.Marshal(src)
	dst := &Config{}
	_ = json.Unmarshal(data, dst)
	return dst
}
