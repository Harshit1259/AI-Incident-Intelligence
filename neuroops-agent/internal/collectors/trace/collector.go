/*
 * NeurOps Agent — Trace Collector
 * Copyright (c) NeurOps 2025. All rights reserved.
 *
 * Runs an embedded HTTP server that accepts OpenTelemetry traces via OTLP/HTTP
 * on a local port (default 4318), then forwards them to the NeurOps product
 * OTLP endpoint and also publishes summary span events via ZMQ.
 *
 * Applications instrumented with any OTel SDK (Java, Python, Node.js, .NET,
 * Go, PHP) point their OTLP_EXPORTER_OTLP_ENDPOINT at this agent, and
 * NeurOps receives distributed traces automatically.
 *
 * Wire format: OTLP/HTTP JSON (protobuf also accepted).
 */

package trace

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/neuroops/agent/internal/config"
	"github.com/neuroops/agent/internal/logger"
)

// Publisher minimal interface for trace events.
type Publisher interface {
	Publish(event map[string]any) bool
}

// Collector is the embedded OTLP trace proxy.
type Collector struct {
	cfg      *config.TraceAgentConfig
	agentCfg *config.AgentConfig
	pub      Publisher
	log      *logger.Logger
	server   *http.Server
}

// New creates a trace Collector.
func New(cfg *config.TraceAgentConfig, agentCfg *config.AgentConfig, pub Publisher, log *logger.Logger) *Collector {
	return &Collector{
		cfg:      cfg,
		agentCfg: agentCfg,
		pub:      pub,
		log:      log,
	}
}

// Start begins listening for OTLP traces.
func (c *Collector) Start() error {
	if !c.cfg.Enabled {
		c.log.Info("Trace collector disabled")
		return nil
	}

	mux := http.NewServeMux()
	// OTLP HTTP trace endpoint
	mux.HandleFunc("/v1/traces", c.handleTraces)
	// OTLP HTTP metrics endpoint (pass-through)
	mux.HandleFunc("/v1/metrics", c.handleMetrics)
	// OTLP HTTP logs endpoint (pass-through)
	mux.HandleFunc("/v1/logs", c.handleLogs)

	c.server = &http.Server{
		Addr:         c.cfg.OTLPEndpoint,
		Handler:      mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
	}

	go func() {
		c.log.Infof("Trace collector listening on %s", c.cfg.OTLPEndpoint)
		if err := c.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			c.log.Errorf("Trace collector error: %v", err)
		}
	}()

	return nil
}

// Stop shuts down the OTLP listener.
func (c *Collector) Stop() {
	if c.server != nil {
		_ = c.server.Close()
		c.log.Info("Trace collector stopped")
	}
}

// ── Handlers ──────────────────────────────────────────────────────────────────

func (c *Collector) handleTraces(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 10<<20)) // 10 MB max
	if err != nil {
		http.Error(w, "failed to read body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	// Forward to product OTLP endpoint
	go c.forward("/v1/traces", r.Header.Get("Content-Type"), body)

	// Extract and publish summary span events via ZMQ
	go c.publishTraceEvents(body)

	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{}`))
}

func (c *Collector) handleMetrics(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 10<<20))
	if err != nil {
		http.Error(w, "failed to read body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()
	go c.forward("/v1/metrics", r.Header.Get("Content-Type"), body)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{}`))
}

func (c *Collector) handleLogs(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 10<<20))
	if err != nil {
		http.Error(w, "failed to read body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()
	go c.forward("/v1/logs", r.Header.Get("Content-Type"), body)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{}`))
}

// ── Internal ──────────────────────────────────────────────────────────────────

func (c *Collector) forward(path, contentType string, body []byte) {
	if c.cfg.ExportEndpoint == "" {
		return
	}

	url := c.cfg.ExportEndpoint + path
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		c.log.Warnf("Failed to build forward request to %s: %v", url, err)
		return
	}

	req.Header.Set("Content-Type", contentType)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		c.log.Warnf("Failed to forward to %s: %v", url, err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		c.log.Warnf("Product OTLP endpoint returned %d for %s", resp.StatusCode, path)
	}
}

func (c *Collector) publishTraceEvents(body []byte) {
	// Parse the OTLP JSON payload to extract span summaries.
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return // binary protobuf — skip ZMQ publishing
	}

	resourceSpans, ok := payload["resourceSpans"].([]any)
	if !ok {
		return
	}

	for _, rs := range resourceSpans {
		rsMap, ok := rs.(map[string]any)
		if !ok {
			continue
		}

		// Extract service name from resource attributes
		serviceName := c.extractServiceName(rsMap)

		scopeSpans, _ := rsMap["scopeSpans"].([]any)
		for _, ss := range scopeSpans {
			ssMap, ok := ss.(map[string]any)
			if !ok {
				continue
			}

			spans, _ := ssMap["spans"].([]any)
			for _, span := range spans {
				spanMap, ok := span.(map[string]any)
				if !ok {
					continue
				}
				c.publishSpan(serviceName, spanMap)
			}
		}
	}
}

func (c *Collector) publishSpan(serviceName string, span map[string]any) {
	event := map[string]any{
		"event.type":         "trace",
		"trace.service":      serviceName,
		"trace.span.id":      fmt.Sprintf("%v", span["spanId"]),
		"trace.trace.id":     fmt.Sprintf("%v", span["traceId"]),
		"trace.parent.id":    fmt.Sprintf("%v", span["parentSpanId"]),
		"trace.operation":    fmt.Sprintf("%v", span["name"]),
		"trace.kind":         fmt.Sprintf("%v", span["kind"]),
		"trace.status":       c.spanStatus(span),
		"trace.start.time":   span["startTimeUnixNano"],
		"trace.end.time":     span["endTimeUnixNano"],
		"agent.id":           c.agentCfg.AgentID,
		"timestamp":          time.Now().Unix(),
	}
	c.pub.Publish(event)
}

func (c *Collector) extractServiceName(rs map[string]any) string {
	resource, ok := rs["resource"].(map[string]any)
	if !ok {
		return "unknown"
	}
	attrs, _ := resource["attributes"].([]any)
	for _, attr := range attrs {
		a, ok := attr.(map[string]any)
		if !ok {
			continue
		}
		if a["key"] == "service.name" {
			if v, ok := a["value"].(map[string]any); ok {
				if sv, ok := v["stringValue"].(string); ok {
					return sv
				}
			}
		}
	}
	return "unknown"
}

func (c *Collector) spanStatus(span map[string]any) string {
	if status, ok := span["status"].(map[string]any); ok {
		if code, ok := status["code"].(string); ok {
			return code
		}
	}
	return "STATUS_CODE_UNSET"
}
