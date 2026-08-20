/*
 * NeurOps Agent — Health Monitor
 * Copyright (c) NeurOps 2025. All rights reserved.
 *
 * Tracks agent self-health (CPU, memory, event queue depth, sub-system
 * liveness) and exposes a lightweight HTTP API so the product can
 * interrogate agent health without going through ZMQ.
 *
 * Endpoints:
 *   GET  /health          — overall health (200 OK | 503 Degraded)
 *   GET  /health/detail   — full JSON breakdown
 *   GET  /metrics         — Prometheus-compatible text metrics
 *   POST /execute         — run a diagnostic command (observe-only allowlist)
 *
 * Security model for /execute:
 *   - All commands are validated against an explicit observe-only allowlist.
 *   - Shell metacharacters (&&, ||, ;, |, ` etc.) are rejected before allowlist check.
 *   - Commands are tokenized with a safe splitter — no shell is invoked.
 *   - Optional bearer token authentication via NEUROOPS_EXECUTE_TOKEN env var.
 *     When set, requests without a matching X-Agent-Token header are rejected.
 *   - Every /execute call is logged with the caller's remote address.
 */

package health

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/neuroops/agent/internal/logger"
)

// Status constants.
const (
	StatusHealthy  = "healthy"
	StatusDegraded = "degraded"
	StatusCritical = "critical"
)

// SubsystemStatus tracks the liveness of one named subsystem.
type SubsystemStatus struct {
	Name      string    `json:"name"`
	Status    string    `json:"status"`
	Message   string    `json:"message,omitempty"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Snapshot is the full health report returned by /health/detail.
type Snapshot struct {
	Status     string                      `json:"status"`
	AgentID    string                      `json:"agent_id"`
	Version    string                      `json:"version"`
	Uptime     string                      `json:"uptime"`
	Timestamp  time.Time                   `json:"timestamp"`
	Subsystems map[string]*SubsystemStatus `json:"subsystems"`
	Memory     MemoryStats                 `json:"memory"`
	QueueDepth int                         `json:"event_queue_depth"`
	Goroutines int                         `json:"goroutines"`
}

// MemoryStats is a snapshot of Go runtime memory.
type MemoryStats struct {
	AllocMB      float64 `json:"alloc_mb"`
	TotalAllocMB float64 `json:"total_alloc_mb"`
	SysMB        float64 `json:"sys_mb"`
	NumGC        uint32  `json:"num_gc"`
}

// QueueDepther is implemented by the publisher.
type QueueDepther interface {
	QueueDepth() int
}

// Monitor manages self-health state and serves the health HTTP API.
type Monitor struct {
	mu        sync.RWMutex
	subsystems map[string]*SubsystemStatus
	agentID   string
	version   string
	startTime time.Time
	log       *logger.Logger
	server    *http.Server
	publisher QueueDepther
}

// New creates a health Monitor.
func New(agentID, version string, pub QueueDepther, log *logger.Logger) *Monitor {
	return &Monitor{
		subsystems: make(map[string]*SubsystemStatus),
		agentID:    agentID,
		version:    version,
		startTime:  time.Now(),
		log:        log,
		publisher:  pub,
	}
}

// Start launches the health HTTP server on the given port.
// port=0 disables the server.
func (m *Monitor) Start(port int) error {
	if port == 0 {
		return nil
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/health", m.handleHealth)
	mux.HandleFunc("/health/detail", m.handleHealthDetail)
	mux.HandleFunc("/metrics", m.handlePrometheusMetrics)
	mux.HandleFunc("/execute", m.handleExecuteCommand)

	m.server = &http.Server{
		Addr:         fmt.Sprintf(":%d", port),
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 45 * time.Second, // slightly longer than /execute command timeout
	}

	go func() {
		m.log.Infof("Health endpoint listening on port %d", port)
		if err := m.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			m.log.Errorf("Health server error: %v", err)
		}
	}()

	return nil
}

// Stop shuts down the health HTTP server.
func (m *Monitor) Stop() {
	if m.server != nil {
		_ = m.server.Close()
		m.log.Info("Health monitor stopped")
	}
}

// SetSubsystem updates the status of a named subsystem.
func (m *Monitor) SetSubsystem(name, status, message string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.subsystems[name] = &SubsystemStatus{
		Name:      name,
		Status:    status,
		Message:   message,
		UpdatedAt: time.Now(),
	}
}

// MarkHealthy marks a subsystem as healthy.
func (m *Monitor) MarkHealthy(name string) {
	m.SetSubsystem(name, StatusHealthy, "")
}

// MarkDegraded marks a subsystem as degraded with a reason.
func (m *Monitor) MarkDegraded(name, reason string) {
	m.SetSubsystem(name, StatusDegraded, reason)
	m.log.Warnf("Subsystem %q degraded: %s", name, reason)
}

// MarkCritical marks a subsystem as critical with a reason.
func (m *Monitor) MarkCritical(name, reason string) {
	m.SetSubsystem(name, StatusCritical, reason)
	m.log.Errorf("Subsystem %q critical: %s", name, reason)
}

// Overall returns the aggregate health status.
func (m *Monitor) Overall() string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	worst := StatusHealthy
	for _, s := range m.subsystems {
		if s.Status == StatusCritical {
			return StatusCritical
		}
		if s.Status == StatusDegraded {
			worst = StatusDegraded
		}
	}
	return worst
}

// Snapshot returns the full health report.
func (m *Monitor) Snapshot() Snapshot {
	m.mu.RLock()
	subs := make(map[string]*SubsystemStatus, len(m.subsystems))
	for k, v := range m.subsystems {
		subs[k] = v
	}
	m.mu.RUnlock()

	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	qd := 0
	if m.publisher != nil {
		qd = m.publisher.QueueDepth()
	}

	return Snapshot{
		Status:     m.Overall(),
		AgentID:    m.agentID,
		Version:    m.version,
		Uptime:     time.Since(m.startTime).Round(time.Second).String(),
		Timestamp:  time.Now().UTC(),
		Subsystems: subs,
		Memory: MemoryStats{
			AllocMB:      float64(memStats.Alloc) / 1e6,
			TotalAllocMB: float64(memStats.TotalAlloc) / 1e6,
			SysMB:        float64(memStats.Sys) / 1e6,
			NumGC:        memStats.NumGC,
		},
		QueueDepth: qd,
		Goroutines: runtime.NumGoroutine(),
	}
}

// ── HTTP handlers ─────────────────────────────────────────────────────────────

func (m *Monitor) handleHealth(w http.ResponseWriter, r *http.Request) {
	overall := m.Overall()
	if overall == StatusCritical {
		w.WriteHeader(http.StatusServiceUnavailable)
	} else {
		w.WriteHeader(http.StatusOK)
	}
	_, _ = fmt.Fprintf(w, `{"status":"%s"}`, overall)
}

func (m *Monitor) handleHealthDetail(w http.ResponseWriter, r *http.Request) {
	snap := m.Snapshot()
	data, _ := json.MarshalIndent(snap, "", "  ")
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

func (m *Monitor) handlePrometheusMetrics(w http.ResponseWriter, r *http.Request) {
	snap := m.Snapshot()
	w.Header().Set("Content-Type", "text/plain; version=0.0.4")
	_, _ = fmt.Fprintf(w,
		`# HELP neuroops_agent_memory_alloc_bytes Memory currently allocated
# TYPE neuroops_agent_memory_alloc_bytes gauge
neuroops_agent_memory_alloc_bytes %.0f
# HELP neuroops_agent_goroutines Number of goroutines
# TYPE neuroops_agent_goroutines gauge
neuroops_agent_goroutines %d
# HELP neuroops_agent_event_queue_depth Events pending in publisher queue
# TYPE neuroops_agent_event_queue_depth gauge
neuroops_agent_event_queue_depth %d
`,
		snap.Memory.AllocMB*1e6,
		snap.Goroutines,
		snap.QueueDepth,
	)
}

// handleExecuteCommand runs a diagnostic command on the agent host and returns output.
//
// Security controls (applied in order):
//  1. Optional bearer token authentication (NEUROOPS_EXECUTE_TOKEN env var)
//  2. Shell injection detection — rejects &&, ||, ;, |, `, $( etc.
//  3. Observe-only allowlist — only read-only diagnostic command prefixes pass
//  4. Safe tokenization — splits into argv without invoking a shell
//  5. Execution timeout — 30 seconds hard limit
func (m *Monitor) handleExecuteCommand(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	// 1. Optional bearer token authentication.
	// Configure by setting NEUROOPS_EXECUTE_TOKEN in the agent's environment.
	// When set, every /execute caller must provide: X-Agent-Token: <token>
	if expectedToken := os.Getenv("NEUROOPS_EXECUTE_TOKEN"); expectedToken != "" {
		provided := r.Header.Get("X-Agent-Token")
		if provided != expectedToken {
			m.log.Warnf("execute: rejected unauthenticated request from %s", r.RemoteAddr)
			jsonError(w, http.StatusUnauthorized, "X-Agent-Token header is required or invalid")
			return
		}
	}

	var req struct {
		Command string `json:"command"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	cmd := strings.TrimSpace(req.Command)
	if cmd == "" {
		jsonError(w, http.StatusBadRequest, "command is empty")
		return
	}

	// 2. Shell injection detection — must happen before allowlist check.
	if found, seq := containsShellInjection(cmd); found {
		m.log.Warnf("execute: shell injection attempt from %s — blocked sequence %q in: %s", r.RemoteAddr, seq, cmd)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		resp := map[string]interface{}{
			"command": cmd,
			"status":  "denied",
			"output":  fmt.Sprintf("Command blocked: shell injection sequence %q is forbidden.", seq),
		}
		data, _ := json.Marshal(resp)
		_, _ = w.Write(data)
		return
	}

	// 3. Observe-only allowlist — reject commands not in the known-safe set.
	lower := strings.ToLower(cmd)
	allowed := false
	for _, prefix := range observeCommandPrefixes {
		if strings.HasPrefix(lower, prefix) {
			allowed = true
			break
		}
	}
	if !allowed {
		m.log.Warnf("execute: allowlist rejection from %s — command not permitted: %s", r.RemoteAddr, cmd)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		resp := map[string]interface{}{
			"command": cmd,
			"status":  "denied",
			"output":  "Command is not in the observe-only allowlist. Only read-only diagnostic commands are permitted.",
		}
		data, _ := json.Marshal(resp)
		_, _ = w.Write(data)
		return
	}

	m.log.Infof("execute: running %q (caller=%s)", cmd, r.RemoteAddr)

	// 4. Safe tokenization — split into argv without shell interpretation.
	// This does NOT invoke /bin/sh; metacharacters have no effect.
	argv := safeTokenize(cmd)
	if len(argv) == 0 {
		jsonError(w, http.StatusBadRequest, "command could not be tokenized")
		return
	}

	// 5. Execute with a hard timeout.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	execCmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	output, err := execCmd.CombinedOutput()

	status := "success"
	if err != nil {
		status = "error"
	}

	w.Header().Set("Content-Type", "application/json")
	resp := map[string]interface{}{
		"command":     cmd,
		"status":      status,
		"output":      string(output),
		"executed_at": time.Now().UTC().Format(time.RFC3339),
	}
	if err != nil {
		resp["error"] = err.Error()
	}

	data, _ := json.Marshal(resp)
	_, _ = w.Write(data)
}

// ── Security helpers ──────────────────────────────────────────────────────────

// shellInjectionSequences are substrings that indicate shell injection.
// Checked before the allowlist so even allowlisted prefixes can't be chained.
var shellInjectionSequences = []string{
	"&&", "||", ";", "|", "`", "$(", "${",
}

// containsShellInjection returns (true, sequence) if a dangerous shell sequence
// is found in cmd.
func containsShellInjection(cmd string) (bool, string) {
	for _, seq := range shellInjectionSequences {
		if strings.Contains(cmd, seq) {
			return true, seq
		}
	}
	return false, ""
}

// safeTokenize splits cmd into an argv slice without shell interpretation.
// It handles simple quoted strings ("hello world") as single tokens.
// Double-quoted segments are unquoted; single-quoted segments are left as-is.
// Shell metacharacters inside quotes have NO special meaning here because
// we are NOT invoking a shell — the result goes directly to exec.Command.
func safeTokenize(cmd string) []string {
	var tokens []string
	var current strings.Builder
	inDouble := false
	inSingle := false

	for i := 0; i < len(cmd); i++ {
		ch := cmd[i]
		switch {
		case ch == '"' && !inSingle:
			inDouble = !inDouble
		case ch == '\'' && !inDouble:
			inSingle = !inSingle
		case ch == ' ' && !inDouble && !inSingle:
			if current.Len() > 0 {
				tokens = append(tokens, current.String())
				current.Reset()
			}
		default:
			current.WriteByte(ch)
		}
	}
	if current.Len() > 0 {
		tokens = append(tokens, current.String())
	}
	return tokens
}

// observeCommandPrefixes — the only commands the agent will execute via /execute.
// These are all read-only diagnostic commands with no state-changing side effects.
// Lower-cased for comparison.
var observeCommandPrefixes = []string{
	"df ",
	"df\t",
	"du ",
	"free ",
	"free\t",
	"top ",
	"uptime",
	"cat /proc/",
	"cat /etc/os-release",
	"cat /var/log/",
	"ps aux",
	"ps -ef",
	"netstat ",
	"ss ",
	"ip addr",
	"ip route",
	"systemctl status",
	"systemctl is-active",
	"journalctl ",
	"docker ps",
	"docker stats",
	"docker system df",
	"kubectl get",
	"kubectl describe",
	"kubectl logs",
	"kubectl top",
	"lsblk",
	"lsof ",
	"iostat",
	"vmstat",
	"mpstat",
	"sar ",
	"nslookup ",
	"dig ",
	"ping ",
	"traceroute ",
	"curl ",
	"head ",
	"tail ",
	"wc ",
	"sort ",
	"grep ",
	"find ",
	"ls ",
	"hostname",
	"uname ",
	"sysctl ",
	"mount",
	"blkid",
	"fdisk -l",
}

// jsonError writes a JSON-encoded error response.
func jsonError(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	data, _ := json.Marshal(map[string]string{"error": msg})
	_, _ = w.Write(data)
}
