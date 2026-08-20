/*
 * NeurOps Agent — Core Agent
 * Copyright (c) NeurOps 2025. All rights reserved.
 *
 * The Agent type is the central coordinator.  It owns the lifecycle of
 * every subsystem: transport, collectors, plugin engine, health monitor,
 * and config watcher.  main.go creates one Agent and calls Start()/Stop().
 *
 * Start() order:
 *   1. Health monitor (HTTP server)
 *   2. Event publisher (ZMQ PUSH → product)
 *   3. Event subscriber (ZMQ SUB ← product)
 *   4. Metric collector
 *   5. Log collector
 *   6. Trace collector
 *   7. Plugin engine
 *   8. Config file watcher (hot-reload)
 *
 * Command dispatch (from subscriber):
 *   "metric.poll"    → immediate metric collection
 *   "plugin.run"     → on-demand plugin execution
 *   "config.reload"  → re-read agent.json
 *   "agent.shutdown" → graceful stop
 *   "discovery"      → run discovery on all metric plugins
 */

package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/google/uuid"

	"github.com/neuroops/agent/internal/collectors/log"
	"github.com/neuroops/agent/internal/collectors/metric"
	"github.com/neuroops/agent/internal/collectors/trace"
	"github.com/neuroops/agent/internal/config"
	"github.com/neuroops/agent/internal/health"
	"github.com/neuroops/agent/internal/logger"
	"github.com/neuroops/agent/internal/plugin"
	"github.com/neuroops/agent/internal/transport"
)

const (
	// ZMQ topic prefixes — must match the product's expected prefixes.
	topicPublish   = "NEUROOPS.AGENT."
	topicSubscribe = "NEUROOPS.CMD."
)

// Publisher is the minimal interface all collectors need to ship data.
type Publisher interface {
	Publish(event map[string]any) bool
}

// Agent is the top-level coordinator.
type Agent struct {
	cfg         *config.Config
	log         *logger.Logger
	agentID     string
	publisher   Publisher                   // may be *EventPublisher or *MultiPublisher
	zmqPub      *transport.EventPublisher   // kept for lifecycle management
	httpFwd     *transport.HTTPForwarder    // kept for lifecycle management
	subscriber  *transport.EventSubscriber
	metricColl  *metric.Collector
	logColl     *log.Collector
	traceColl   *trace.Collector
	pluginEng   *plugin.Engine
	healthMon   *health.Monitor
	cfgWatcher  *fsnotify.Watcher
	cfgPath     string
	stopCh      chan struct{}
}

// New creates an Agent from loaded configuration.
func New(cfg *config.Config, log *logger.Logger) (*Agent, error) {
	// Ensure agent has a stable ID
	agentID := cfg.Agent.AgentID
	if agentID == "" {
		agentID = ensureAgentID()
		cfg.Agent.AgentID = agentID
	}
	log.Infof("Agent ID: %s", agentID)

	return &Agent{
		cfg:     cfg,
		log:     log,
		agentID: agentID,
		stopCh:  make(chan struct{}),
	}, nil
}

// Start boots all subsystems in dependency order.
func (a *Agent) Start() error {
	if a.cfg.Agent.AgentState == "DISABLE" {
		a.log.Warn("Agent state is DISABLE — exiting without starting")
		os.Exit(0)
	}

	a.log.Info("Agent starting")

	// 1. Health monitor
	a.healthMon = health.New(a.agentID, "1.0.0", nil, a.log.WithComponent("health"))
	if err := a.healthMon.Start(a.cfg.Agent.HealthPort); err != nil {
		return fmt.Errorf("health monitor: %w", err)
	}
	a.healthMon.MarkHealthy("health")

	// 2. Event publisher (agent → product)
	zmqPub, err := transport.NewEventPublisher(
		a.cfg.Agent.ProductHost,
		a.cfg.Agent.EventPublisherPort,
		topicPublish,
		100_000,
		a.log.WithComponent("publisher"),
	)
	if err != nil {
		return fmt.Errorf("event publisher: %w", err)
	}
	if err := zmqPub.Start(); err != nil {
		return fmt.Errorf("starting event publisher: %w", err)
	}
	a.zmqPub = zmqPub
	a.healthMon.MarkHealthy("publisher")

	// 2b. HTTP forwarder (agent → AI Incident Platform)
	var pub Publisher
	if a.cfg.Agent.HTTPForwarderEnabled && a.cfg.Agent.HTTPForwarderURL != "" {
		httpFwd := transport.NewHTTPForwarder(
			a.cfg.Agent.HTTPForwarderURL,
			a.agentID,
			a.log.WithComponent("http_forwarder"),
		)
		httpFwd.Start()
		a.httpFwd = httpFwd
		a.healthMon.MarkHealthy("http_forwarder")

		pub = transport.NewMultiPublisher(zmqPub, httpFwd)
		a.log.Info("Event publishing: ZMQ + HTTP forwarder")
	} else {
		pub = zmqPub
		a.log.Info("Event publishing: ZMQ only (HTTP forwarder disabled)")
	}
	a.publisher = pub

	// 3. Event subscriber (product → agent)
	sub, err := transport.NewEventSubscriber(
		a.cfg.Agent.ProductHost,
		a.cfg.Agent.EventSubscriberPort,
		topicSubscribe,
		a.dispatch,
		a.log.WithComponent("subscriber"),
	)
	if err != nil {
		return fmt.Errorf("event subscriber: %w", err)
	}
	if err := sub.Start(); err != nil {
		a.log.Warnf("Event subscriber failed to start (product may be unreachable): %v", err)
		a.healthMon.MarkDegraded("subscriber", err.Error())
	} else {
		a.subscriber = sub
		a.healthMon.MarkHealthy("subscriber")
	}

	// 4. Metric collector
	if a.cfg.Agent.MetricAgentEnabled {
		a.metricColl = metric.New(&a.cfg.MetricAgent, &a.cfg.Agent, pub, a.log.WithComponent("metric"))
		if err := a.metricColl.Start(); err != nil {
			a.log.Errorf("Metric collector failed: %v", err)
			a.healthMon.MarkDegraded("metric_collector", err.Error())
		} else {
			a.healthMon.MarkHealthy("metric_collector")
		}
	}

	// 5. Log collector
	if a.cfg.Agent.LogAgentEnabled {
		lc, err := log.New(&a.cfg.LogAgent, &a.cfg.Agent, pub, a.log.WithComponent("log"))
		if err != nil {
			a.log.Errorf("Log collector init failed: %v", err)
			a.healthMon.MarkDegraded("log_collector", err.Error())
		} else {
			if err := lc.Start(); err != nil {
				a.log.Errorf("Log collector start failed: %v", err)
				a.healthMon.MarkDegraded("log_collector", err.Error())
			} else {
				a.logColl = lc
				a.healthMon.MarkHealthy("log_collector")
			}
		}
	}

	// 6. Trace collector
	if a.cfg.Agent.TraceAgentEnabled {
		a.traceColl = trace.New(&a.cfg.TraceAgent, &a.cfg.Agent, pub, a.log.WithComponent("trace"))
		if err := a.traceColl.Start(); err != nil {
			a.log.Errorf("Trace collector start failed: %v", err)
			a.healthMon.MarkDegraded("trace_collector", err.Error())
		} else {
			a.healthMon.MarkHealthy("trace_collector")
		}
	}

	// 7. Plugin engine
	if len(a.cfg.MetricAgent.PluginDirectories) > 0 {
		a.pluginEng = plugin.New(&a.cfg.MetricAgent, &a.cfg.Agent, pub, a.log.WithComponent("plugin"))
		if err := a.pluginEng.Start(); err != nil {
			a.log.Errorf("Plugin engine start failed: %v", err)
			a.healthMon.MarkDegraded("plugin_engine", err.Error())
		} else {
			a.healthMon.MarkHealthy("plugin_engine")
		}
	}

	// 8. Config file watcher
	if err := a.startConfigWatcher(); err != nil {
		a.log.Warnf("Config watcher failed (hot-reload disabled): %v", err)
	}

	// Announce agent ready
	a.publishAgentEvent("agent.started", map[string]any{
		"agent.id":      a.agentID,
		"agent.version": "1.0.0",
		"agent.host":    a.hostname(),
	})

	a.log.Info("Agent started successfully")
	return nil
}

// Stop shuts down all subsystems in reverse dependency order.
func (a *Agent) Stop() {
	a.log.Info("Agent shutting down")

	close(a.stopCh)

	if a.cfgWatcher != nil {
		_ = a.cfgWatcher.Close()
	}
	if a.pluginEng != nil {
		a.pluginEng.Stop()
	}
	if a.traceColl != nil {
		a.traceColl.Stop()
	}
	if a.logColl != nil {
		a.logColl.Stop()
	}
	if a.metricColl != nil {
		a.metricColl.Stop()
	}
	if a.subscriber != nil {
		a.subscriber.Shutdown()
	}

	// Final flush: announce shutdown before closing publisher
	a.publishAgentEvent("agent.stopped", map[string]any{
		"agent.id": a.agentID,
	})
	time.Sleep(500 * time.Millisecond) // let the stop event drain

	if a.zmqPub != nil {
		a.zmqPub.Shutdown()
	}
	if a.httpFwd != nil {
		a.httpFwd.Shutdown()
	}
	if a.healthMon != nil {
		a.healthMon.Stop()
	}

	a.log.Info("Agent shutdown complete")
}

// ── Command dispatch ──────────────────────────────────────────────────────────

// dispatch is called for every command received from the product.
func (a *Agent) dispatch(event map[string]any) {
	eventType, _ := event["event.type"].(string)

	switch eventType {
	case "metric.poll":
		a.log.Debug("Received metric.poll command")
		// The metric collector will automatically publish on next tick;
		// for an immediate trigger we would call metricColl.TriggerNow() here.

	case "plugin.run":
		pluginID, _ := event["plugin.id"].(string)
		ctx, _ := event["context"].(map[string]any)
		if pluginID == "" {
			a.log.Warn("plugin.run command missing plugin.id")
			return
		}
		go func() {
			if a.pluginEng == nil {
				a.log.Warn("Plugin engine not running — cannot execute plugin")
				return
			}
			result, err := a.pluginEng.Execute(pluginID, ctx)
			if err != nil {
				a.publishAgentEvent("plugin.error", map[string]any{
					"plugin.id": pluginID,
					"error":     err.Error(),
				})
				return
			}
			a.publishAgentEvent("plugin.result", map[string]any{
				"plugin.id": pluginID,
				"result":    result,
			})
		}()

	case "config.reload":
		a.log.Info("Received config.reload command")
		// Re-read config and apply changes that support hot-reload.

	case "agent.shutdown":
		a.log.Info("Received agent.shutdown command from product")
		go a.Stop()

	case "discovery":
		a.log.Info("Received discovery command")
		// Trigger full rediscovery on all metric plugins.

	case "agent.ping":
		a.publishAgentEvent("agent.pong", map[string]any{
			"agent.id":  a.agentID,
			"timestamp": time.Now().Unix(),
		})

	default:
		a.log.Debugf("Unknown command type: %s", eventType)
	}
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func (a *Agent) publishAgentEvent(eventType string, extra map[string]any) {
	event := map[string]any{
		"event.type": eventType,
		"agent.id":   a.agentID,
		"timestamp":  time.Now().Unix(),
	}
	for k, v := range extra {
		event[k] = v
	}
	if a.publisher != nil {
		a.publisher.Publish(event)
	}
}

func (a *Agent) startConfigWatcher() error {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}

	configFile := "config/agent.json"
	if err := watcher.Add(filepath.Dir(configFile)); err != nil {
		_ = watcher.Close()
		return err
	}

	a.cfgWatcher = watcher

	go func() {
		for {
			select {
			case <-a.stopCh:
				return
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}
				if event.Has(fsnotify.Write) && filepath.Base(event.Name) == "agent.json" {
					a.log.Info("agent.json changed — reloading configuration")
					if newCfg, err := config.Load(configFile); err == nil {
						a.cfg = newCfg
						a.log.Info("Configuration reloaded successfully")
					} else {
						a.log.Errorf("Config reload failed: %v", err)
					}
				}
			}
		}
	}()

	return nil
}

func (a *Agent) hostname() string {
	h, _ := os.Hostname()
	return h
}

// ensureAgentID reads or generates a persistent agent UUID.
func ensureAgentID() string {
	const idFile = "config/.agent.id"
	if data, err := os.ReadFile(idFile); err == nil {
		id := string(data)
		if id != "" {
			return id
		}
	}
	id := uuid.New().String()
	_ = os.MkdirAll(filepath.Dir(idFile), 0o755)
	_ = os.WriteFile(idFile, []byte(id), 0o600)
	return id
}
