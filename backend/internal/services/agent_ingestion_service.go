package services

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"ai-incident-platform/backend/internal/models"
	"ai-incident-platform/backend/internal/store"
)

// AgentIngestionService processes batches of NeuroOps agent telemetry events.
type AgentIngestionService struct {
	agentStore         *store.AgentStore
	eventStore         *store.EventStore
	correlationService *CorrelationService
	anomalyService     *AnomalyService
	logStore           *store.LogStore
}

// NewAgentIngestionService creates a new ingestion service for agent events.
func NewAgentIngestionService(as *store.AgentStore, es *store.EventStore, cs *CorrelationService, ans *AnomalyService, ls *store.LogStore) *AgentIngestionService {
	return &AgentIngestionService{
		agentStore:         as,
		eventStore:         es,
		correlationService: cs,
		anomalyService:     ans,
		logStore:           ls,
	}
}

// ProcessBatch processes a batch of agent events.
// Returns (processed count, alerts created count).
func (s *AgentIngestionService) ProcessBatch(events []map[string]interface{}, tenantID string) (int, int) {
	processed := 0
	alerts := 0

	// Check agent status ONCE per batch — if stopped, reject the entire batch
	stoppedAgents := make(map[string]bool)
	agentNames := make(map[string]string)

	for _, raw := range events {
		eventType := getString(raw, "event.type")
		agentID := getString(raw, "agent.id")
		objectIP := getString(raw, "object.ip")

		if agentID == "" {
			continue
		}

		// Check stopped status (cached per batch)
		if _, checked := stoppedAgents[agentID]; !checked {
			agent, err := s.agentStore.GetAgentByID(agentID)
			if err == nil && agent != nil {
				stoppedAgents[agentID] = (agent.Status == "stopped")
				if agent.Name != "" {
					agentNames[agentID] = agent.Name
				}
			} else {
				stoppedAgents[agentID] = false
			}
		}

		// Update last_seen even if stopped (so we know agent is alive)
		_ = s.agentStore.UpdateLastSeen(agentID)

		// Skip ALL processing for stopped agents — no logs, no metrics, no incidents
		if stoppedAgents[agentID] {
			continue
		}

		// Determine service name
		serviceName := objectIP
		if serviceName == "" {
			serviceName = agentID
		}
		if name, ok := agentNames[agentID]; ok {
			serviceName = name
		}

		// Parse timestamp
		ts := time.Now()
		if tsVal, ok := raw["timestamp"]; ok {
			switch v := tsVal.(type) {
			case float64:
				ts = time.Unix(int64(v), 0)
			case int64:
				ts = time.Unix(v, 0)
			}
		}

		switch eventType {
		case "metric":
			alertCount := s.processMetricEvent(raw, agentID, serviceName, ts, tenantID)
			alerts += alertCount

		case "log":
			alertCount := s.processLogEvent(raw, agentID, serviceName, ts, tenantID)
			alerts += alertCount

		case "trace":
			s.processTraceEvent(raw, agentID, ts)

		default:
			log.Printf("agent_ingestion: unknown event.type %q from agent %s", eventType, agentID)
			continue
		}

		processed++
	}

	return processed, alerts
}

// processMetricEvent handles metric-type agent events, saves to agent_metrics,
// and checks thresholds for anomaly detection.
func (s *AgentIngestionService) processMetricEvent(raw map[string]interface{}, agentID, serviceName string, ts time.Time, tenantID string) int {
	metricType := getString(raw, "metric.type")
	if metricType == "" {
		metricType = "unknown"
	}

	// Serialize the entire event as the metric data
	dataBytes, err := json.Marshal(raw)
	if err != nil {
		log.Printf("agent_ingestion: failed to marshal metric data: %v", err)
		return 0
	}

	if err := s.agentStore.SaveMetric(agentID, metricType, ts, string(dataBytes)); err != nil {
		log.Printf("agent_ingestion: failed to save metric for agent %s: %v", agentID, err)
	}

	alerts := 0

	// Threshold checks
	if cpuUsed := getFloat(raw, "system.cpu.used.percent"); cpuUsed > 85 {
		if s.checkAnomaly(tenantID, serviceName, "system.cpu.used.percent", cpuUsed, 85, "rising") {
			alerts++
		}
	}

	if memUsed := getFloat(raw, "system.memory.used.percent"); memUsed > 90 {
		if s.checkAnomaly(tenantID, serviceName, "system.memory.used.percent", memUsed, 90, "rising") {
			alerts++
		}
	}

	if diskUsed := getFloat(raw, "disk.used.percent"); diskUsed > 90 {
		if s.checkAnomaly(tenantID, serviceName, "disk.used.percent", diskUsed, 90, "rising") {
			alerts++
		}
	}

	if load1 := getFloat(raw, "system.load.1"); load1 > 0 {
		cpuCount := getFloat(raw, "system.cpu.count")
		if cpuCount <= 0 {
			cpuCount = 4
		}
		threshold := cpuCount * 2
		if load1 > threshold {
			if s.checkAnomaly(tenantID, serviceName, "system.load.1", load1, threshold, "rising") {
				alerts++
			}
		}
	}

	return alerts
}

// checkAnomaly delegates to the anomaly service and returns true if an alert was created.
func (s *AgentIngestionService) checkAnomaly(tenantID, service, metricName string, value, threshold float64, trend string) bool {
	if s.anomalyService == nil {
		return false
	}
	alert, err := s.anomalyService.CheckMetric(tenantID, service, metricName, value, threshold, trend)
	if err != nil {
		log.Printf("agent_ingestion: anomaly check error for %s/%s: %v", service, metricName, err)
		return false
	}
	return alert != nil
}

// processLogEvent handles log-type agent events. It always stores the log
// entry in the log_entries table, then pattern-matches for errors and creates
// internal events that flow through correlation.
func (s *AgentIngestionService) processLogEvent(raw map[string]interface{}, agentID, serviceName string, ts time.Time, tenantID string) int {
	logMessage := getString(raw, "log.message")
	logSource := getString(raw, "log.source")
	logTag := getString(raw, "log.tag")

	if logMessage == "" {
		return 0
	}

	// Always store the log entry in the log_entries table.
	if s.logStore != nil {
		rawJSON := "{}"
		if b, err := json.Marshal(raw); err == nil {
			rawJSON = string(b)
		}
		entry := models.LogEntry{
			TenantID:      tenantID,
			AgentID:       agentID,
			HostIP:        getString(raw, "object.ip"),
			LogSource:     logSource,
			LogTag:        logTag,
			EventType:     "log",
			EventCategory: getString(raw, "event.category"),
			Message:       logMessage,
			RawJSON:       rawJSON,
			Timestamp:     ts,
		}
		if entry.EventCategory == "" {
			entry.EventCategory = "info"
		}
		if err := s.logStore.Insert(entry); err != nil {
			log.Printf("agent_ingestion: failed to store log entry: %v", err)
		}
	}

	severity := matchLogSeverity(logMessage)
	if severity == "" {
		// No error pattern matched — log was still stored above.
		return 0
	}

	// Extract process name from syslog-format messages for a better service name.
	// Format: "timestamp hostname process[pid]: message"
	svcName := extractServiceFromLog(logMessage, serviceName)

	// Build title from the actual error, not the full syslog line
	title := buildLogTitle(severity, svcName, logMessage)

	event := models.Event{
		ID:        fmt.Sprintf("agent-log-%d", time.Now().UnixNano()),
		Source:    "neuroops-agent",
		Type:      "log",
		Service:   svcName,
		Severity:  severity,
		Title:     title,
		Message:   logMessage,
		Timestamp: ts,
	}

	// Add metadata as labels
	if logSource != "" || logTag != "" || agentID != "" {
		event.Labels = map[string]string{}
		if logSource != "" {
			event.Labels["log.source"] = logSource
		}
		if logTag != "" {
			event.Labels["log.tag"] = logTag
		}
		if agentID != "" {
			event.Labels["agent.id"] = agentID
		}
	}

	if err := s.eventStore.SaveEvent(event); err != nil {
		log.Printf("ERROR: agent_ingestion: failed to save log event %s: %v", event.ID, err)
		return 0
	}

	if s.correlationService != nil {
		s.correlationService.ProcessEvent(event)
	}

	return 1
}

// processTraceEvent handles trace-type events (basic storage for now).
func (s *AgentIngestionService) processTraceEvent(raw map[string]interface{}, agentID string, ts time.Time) {
	dataBytes, err := json.Marshal(raw)
	if err != nil {
		return
	}
	if err := s.agentStore.SaveMetric(agentID, "trace", ts, string(dataBytes)); err != nil {
		log.Printf("agent_ingestion: failed to save trace for agent %s: %v", agentID, err)
	}
}

// matchLogSeverity returns the severity if the log message matches a real error
// pattern. Ignores informational messages that happen to contain the word "error"
// in benign contexts (like log file names or status reports).
func matchLogSeverity(message string) string {
	lower := strings.ToLower(message)

	// Skip benign lines — agent internal logs, system noise, informational contexts
	for _, skip := range []string{
		"0 error", "errors=0", "error=0", "errors: 0", "error_count=0",
		"no error", "without error", "error.log", "error_log",
		"loglevel=error", "level=error", "level\":\"error",
		"apparmor=", "audit(", "session opened", "session closed",
		"systemd[", "started session", "new session", "removed session",
		// Agent internal logs — not real application errors
		"| core |", "| trace |", "| info |", "| debug |",
		"pkg/collector/", "pkg/trace/", "pkg/aggregator/",
		"comp/core/", "comp/metadata/", "comp/logs/",
		"neuroops-agent", "agent[", "trace-loader[",
		"corechecks/", "forwarder/", "autodiscovery/",
		// System informational / desktop noise
		"logrotate", "cron[", "anacron[", "snapd[",
		"dbus-daemon[", "networkmanager[", "resolved[",
		"polkitd[", "gdm-", "gnome-", "gvfsd",
		"bluetooth", "pulseaudio", "pipewire",
		"gsd-", "gsd_", "media-keys", "power-manager",
		"tracker-", "evolution-", "colord",
		"couldn't lock screen", "gdbus.error",
		"wlp0s20f3", "wlan0", "association with", "deauthenticated from",
		"4way_handshake", "handshake_timeout",
		// Agent internal / collector noise
		"| warn |", "| warning |",
		"disk.go:", "diskv2/",
	} {
		if strings.Contains(lower, skip) {
			return ""
		}
	}

	// Critical — genuine panics and kills
	for _, pattern := range []string{
		"kernel panic", "panic:", "fatal error:", "out of memory:",
		"killed process", "oom-killer", "oom_kill",
	} {
		if strings.Contains(lower, pattern) {
			return "critical"
		}
	}

	// High — real application/service errors
	for _, pattern := range []string{
		"segfault", "core dumped", "stack trace",
		"unhandled exception", "traceback (most recent",
		"connection refused", "connection reset by peer",
		"permission denied", "access denied",
		"disk full", "no space left on device",
		"authentication failure", "failed password",
	} {
		if strings.Contains(lower, pattern) {
			return "high"
		}
	}

	// Medium — operational warnings worth tracking
	for _, pattern := range []string{
		"timeout", "timed out", "deadline exceeded",
		"too many open files", "resource temporarily unavailable",
		"service failed", "failed to start",
	} {
		if strings.Contains(lower, pattern) {
			return "medium"
		}
	}

	// No actionable pattern
	return ""
}

// getString safely extracts a string value from the event map.
func getString(m map[string]interface{}, key string) string {
	v, ok := m[key]
	if !ok {
		return ""
	}
	s, ok := v.(string)
	if !ok {
		return fmt.Sprintf("%v", v)
	}
	return s
}

// extractServiceFromLog extracts the most specific service name from a log line.
// Priority: (1) service name after ERROR:/FATAL:/CRITICAL: prefix
//
//	(2) process[pid] from syslog format
//	(3) fallback (hostname/agent name)
func extractServiceFromLog(message, fallback string) string {
	// 1. Look for "SEVERITY: service-name:" pattern — most specific
	// e.g., "FATAL: payments-api: connection refused..."
	for _, prefix := range []string{"FATAL: ", "ERROR: ", "CRITICAL: ", "WARN: ", "WARNING: "} {
		idx := strings.Index(strings.ToUpper(message), strings.ToUpper(prefix))
		if idx < 0 {
			continue
		}
		after := message[idx+len(prefix):]
		// Take the next word before ":"
		colonIdx := strings.Index(after, ":")
		if colonIdx > 0 && colonIdx <= 40 {
			svc := strings.TrimSpace(after[:colonIdx])
			// Validate: looks like a service name (not a path, not a number)
			if len(svc) >= 2 && len(svc) <= 40 && !strings.Contains(svc, "/") && !strings.Contains(svc, " ") {
				return svc
			}
		}
	}

	// 2. Try syslog process[pid] format
	parts := strings.Fields(message)
	for _, p := range parts {
		if strings.Contains(p, "[") {
			procName := p
			if idx := strings.Index(procName, "["); idx > 0 {
				procName = procName[:idx]
			}
			procName = strings.TrimPrefix(procName, "/usr/sbin/")
			procName = strings.TrimPrefix(procName, "/usr/bin/")
			if procName != "" && len(procName) > 1 {
				sysProcs := map[string]bool{
					"kernel": true, "systemd": true, "sshd": true, "cron": true,
					"sudo": true, "su": true, "login": true, "dbus": true,
				}
				if !sysProcs[strings.ToLower(procName)] {
					return procName
				}
			}
		}
	}

	return fallback
}

// buildLogTitle creates a human-readable incident title from a log error.
func buildLogTitle(severity, service, message string) string {
	// Strip the syslog timestamp/hostname prefix to get the actual message
	msg := message
	// Find the ": " after process[pid] and take everything after it
	if idx := strings.Index(msg, "]: "); idx >= 0 {
		msg = msg[idx+3:]
	} else if idx := strings.Index(msg, ": "); idx >= 0 && idx < 80 {
		msg = msg[idx+2:]
	}
	msg = strings.TrimSpace(msg)
	if len(msg) > 100 {
		msg = msg[:100]
	}
	if msg == "" {
		msg = message
		if len(msg) > 80 {
			msg = msg[:80]
		}
	}
	return fmt.Sprintf("[%s] %s: %s", strings.ToUpper(severity), service, msg)
}

// getFloat safely extracts a float64 value from the event map.
func getFloat(m map[string]interface{}, key string) float64 {
	v, ok := m[key]
	if !ok {
		return 0
	}
	switch n := v.(type) {
	case float64:
		return n
	case int:
		return float64(n)
	case int64:
		return float64(n)
	default:
		return 0
	}
}
