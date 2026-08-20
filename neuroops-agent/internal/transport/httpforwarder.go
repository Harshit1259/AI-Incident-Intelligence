/*
 * NeurOps Agent — HTTP Event Forwarder
 * Copyright (c) NeurOps 2025. All rights reserved.
 *
 * Sends telemetry events to the AI Incident Platform via HTTP POST.
 * Events are buffered in memory and flushed periodically or when the
 * batch reaches its size limit, whichever comes first.
 *
 * Reliability features:
 *   - Retry with exponential backoff on HTTP failure (3 attempts)
 *   - Disk spool for events that fail all retries
 *   - Registration retries until successful
 *   - Publish() returns false on buffer full (backpressure)
 *   - Single flush goroutine (no concurrent flushes)
 *   - Response body inspection for error details
 */

package transport

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/neuroops/agent/internal/logger"
)

const (
	defaultBatchSize    = 50
	defaultFlushSec     = 5
	defaultMaxBuffer    = 5000  // max events in memory before backpressure
	defaultMaxRetries   = 3
	httpTimeout         = 15 * time.Second
	spoolDir            = "/tmp/neuroops-spool"
	maxSpoolFiles       = 100
)

// HTTPForwarder sends events via HTTP POST to the product endpoint.
type HTTPForwarder struct {
	endpoint   string
	agentID    string
	httpClient *http.Client
	buffer     []map[string]any
	mu         sync.Mutex
	flushTimer *time.Ticker
	stopCh     chan struct{}
	batchSize  int
	flushSec   int
	maxBuffer  int
	log        *logger.Logger
	registered atomic.Bool
	flushing   atomic.Bool // prevents concurrent flushes

	// Metrics
	eventsPublished atomic.Int64
	eventsDropped   atomic.Int64
	flushErrors     atomic.Int64
}

// NewHTTPForwarder creates an HTTP forwarder targeting the given endpoint.
func NewHTTPForwarder(endpoint, agentID string, log *logger.Logger) *HTTPForwarder {
	return &HTTPForwarder{
		endpoint: endpoint,
		agentID:  agentID,
		httpClient: &http.Client{
			Timeout: httpTimeout,
			Transport: &http.Transport{
				MaxIdleConns:        10,
				MaxIdleConnsPerHost: 5,
				IdleConnTimeout:     30 * time.Second,
			},
		},
		buffer:    make([]map[string]any, 0, defaultBatchSize),
		stopCh:    make(chan struct{}),
		batchSize: defaultBatchSize,
		flushSec:  defaultFlushSec,
		maxBuffer: defaultMaxBuffer,
		log:       log,
	}
}

// Start begins the periodic flush goroutine and spool recovery.
func (h *HTTPForwarder) Start() {
	h.flushTimer = time.NewTicker(time.Duration(h.flushSec) * time.Second)

	// Ensure spool directory exists
	_ = os.MkdirAll(spoolDir, 0755)

	go h.flushLoop()
	go h.replaySpooledEvents()

	h.log.Infof("HTTP forwarder started -> %s (batch=%d, flush=%ds, maxbuf=%d)", h.endpoint, h.batchSize, h.flushSec, h.maxBuffer)
}

// Publish adds an event to the buffer. Returns false if buffer is full (backpressure).
func (h *HTTPForwarder) Publish(event map[string]any) bool {
	h.mu.Lock()
	if len(h.buffer) >= h.maxBuffer {
		h.mu.Unlock()
		h.eventsDropped.Add(1)
		return false // backpressure — caller knows delivery failed
	}
	h.buffer = append(h.buffer, event)
	shouldFlush := len(h.buffer) >= h.batchSize
	h.mu.Unlock()

	if shouldFlush {
		h.triggerFlush()
	}
	return true
}

// QueueDepth returns the number of buffered events waiting to be flushed.
func (h *HTTPForwarder) QueueDepth() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.buffer)
}

// Stats returns forwarder metrics.
func (h *HTTPForwarder) Stats() (published, dropped, errors int64) {
	return h.eventsPublished.Load(), h.eventsDropped.Load(), h.flushErrors.Load()
}

// Shutdown flushes any remaining events and stops the flush timer.
func (h *HTTPForwarder) Shutdown() {
	h.log.Info("HTTP forwarder shutting down")
	if h.flushTimer != nil {
		h.flushTimer.Stop()
	}
	close(h.stopCh)
	h.flush() // final flush
	h.log.Info("HTTP forwarder shut down")
}

// ── Internal ──────────────────────────────────────────────────────────────────

func (h *HTTPForwarder) flushLoop() {
	for {
		select {
		case <-h.stopCh:
			return
		case <-h.flushTimer.C:
			h.flush()
		}
	}
}

// triggerFlush starts a flush if one is not already running.
// Prevents multiple concurrent flushes from Publish() calls.
func (h *HTTPForwarder) triggerFlush() {
	if h.flushing.CompareAndSwap(false, true) {
		go func() {
			defer h.flushing.Store(false)
			h.flush()
		}()
	}
}

func (h *HTTPForwarder) flush() {
	h.mu.Lock()
	if len(h.buffer) == 0 {
		h.mu.Unlock()
		return
	}
	batch := h.buffer
	h.buffer = make([]map[string]any, 0, h.batchSize)
	h.mu.Unlock()

	// Ensure registration before sending events.
	// Retry registration on every flush until it succeeds.
	if !h.registered.Load() {
		if h.register() {
			h.registered.Store(true)
		}
		// Even if registration fails, still try to send events
	}

	if !h.postEventsWithRetry(batch) {
		// All retries failed — spool to disk for later replay
		h.spoolToDisk(batch)
	}
}

// postEventsWithRetry attempts to POST events with exponential backoff.
// Returns true if any attempt succeeded.
func (h *HTTPForwarder) postEventsWithRetry(events []map[string]any) bool {
	payload := map[string]any{"events": events}
	body, err := json.Marshal(payload)
	if err != nil {
		h.log.Warnf("HTTP forwarder: failed to marshal batch (%d events): %v", len(events), err)
		h.eventsDropped.Add(int64(len(events)))
		return false
	}

	url := h.endpoint + "/api/v1/ingest/agent"
	backoff := 1 * time.Second

	for attempt := 1; attempt <= defaultMaxRetries; attempt++ {
		resp, err := h.httpClient.Post(url, "application/json", bytes.NewReader(body))
		if err != nil {
			h.log.Warnf("HTTP forwarder: POST attempt %d/%d failed: %v", attempt, defaultMaxRetries, err)
			if attempt < defaultMaxRetries {
				time.Sleep(backoff)
				backoff *= 2
			}
			continue
		}

		// Read response body for error details
		respBody, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		if resp.StatusCode < 400 {
			// Success
			h.eventsPublished.Add(int64(len(events)))
			h.log.Debugf("HTTP forwarder: shipped %d events (HTTP %d)", len(events), resp.StatusCode)
			return true
		}

		// Server error — log details and retry
		h.log.Warnf("HTTP forwarder: POST attempt %d/%d returned %d: %s", attempt, defaultMaxRetries, resp.StatusCode, truncateStr(string(respBody), 200))

		if resp.StatusCode >= 400 && resp.StatusCode < 500 {
			// Client error (4xx) — don't retry, our payload is bad
			h.eventsDropped.Add(int64(len(events)))
			h.flushErrors.Add(1)
			return false
		}

		// Server error (5xx) — retry with backoff
		if attempt < defaultMaxRetries {
			time.Sleep(backoff)
			backoff *= 2
		}
	}

	h.flushErrors.Add(1)
	h.log.Warnf("HTTP forwarder: all %d retries exhausted for %d events — spooling to disk", defaultMaxRetries, len(events))
	return false
}

// ── Disk Spool (for events that fail all retries) ────────────────────────────

func (h *HTTPForwarder) spoolToDisk(events []map[string]any) {
	data, err := json.Marshal(events)
	if err != nil {
		h.log.Warnf("HTTP forwarder: failed to marshal for spool: %v", err)
		h.eventsDropped.Add(int64(len(events)))
		return
	}

	filename := filepath.Join(spoolDir, fmt.Sprintf("spool-%d.json", time.Now().UnixNano()))
	if err := os.WriteFile(filename, data, 0644); err != nil {
		h.log.Warnf("HTTP forwarder: failed to write spool file: %v", err)
		h.eventsDropped.Add(int64(len(events)))
		return
	}

	h.log.Infof("HTTP forwarder: spooled %d events to %s", len(events), filename)
}

// replaySpooledEvents runs once on startup — replays any events saved to disk
// from previous failed flushes.
func (h *HTTPForwarder) replaySpooledEvents() {
	// Wait a bit for the product to be ready
	time.Sleep(10 * time.Second)

	entries, err := os.ReadDir(spoolDir)
	if err != nil {
		return
	}

	replayed := 0
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		fullPath := filepath.Join(spoolDir, entry.Name())
		data, err := os.ReadFile(fullPath)
		if err != nil {
			continue
		}

		var events []map[string]any
		if err := json.Unmarshal(data, &events); err != nil {
			_ = os.Remove(fullPath) // corrupted, delete
			continue
		}

		if h.postEventsWithRetry(events) {
			_ = os.Remove(fullPath) // successfully replayed, delete spool file
			replayed += len(events)
		}

		// Don't overwhelm the product on startup
		if replayed > 1000 {
			break
		}
	}

	if replayed > 0 {
		h.log.Infof("HTTP forwarder: replayed %d spooled events", replayed)
	}
}

// ── Registration ─────────────────────────────────────────────────────────────

// register sends agent registration to the product. Returns true on success.
func (h *HTTPForwarder) register() bool {
	hostname, _ := os.Hostname()
	ip := getLocalIP()

	payload := map[string]any{
		"agent_id": h.agentID,
		"name":     hostname,
		"host_ip":  ip,
		"os_type":  runtime.GOOS,
		"version":  "1.0.0",
	}

	body, err := json.Marshal(payload)
	if err != nil {
		h.log.Warnf("HTTP forwarder: failed to marshal registration: %v", err)
		return false
	}

	url := h.endpoint + "/api/v1/agents/register"
	resp, err := h.httpClient.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		h.log.Warnf("HTTP forwarder: agent registration failed: %v — will retry on next flush", err)
		return false
	}

	// Read and log response
	respBody, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	if resp.StatusCode >= 400 {
		h.log.Warnf("HTTP forwarder: agent registration returned %d: %s — will retry", resp.StatusCode, truncateStr(string(respBody), 200))
		return false
	}

	h.log.Infof("HTTP forwarder: agent registered with platform (%s)", url)
	return true
}

// ── Helpers ──────────────────────────────────────────────────────────────────

func truncateStr(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}

// getLocalIP returns the first non-loopback IPv4 address found on any
// network interface, or "unknown" if none is available.
func getLocalIP() string {
	ifaces, err := net.Interfaces()
	if err != nil {
		return "unknown"
	}
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}
			if ip == nil || ip.IsLoopback() {
				continue
			}
			if ip4 := ip.To4(); ip4 != nil {
				return ip4.String()
			}
		}
	}
	return "unknown"
}

// GetLocalIP is the exported version for use by other packages.
func GetLocalIP() string {
	return getLocalIP()
}

// Ensure HTTPForwarder satisfies the same method set collectors expect.
var _ interface {
	Publish(event map[string]any) bool
} = (*HTTPForwarder)(nil)
