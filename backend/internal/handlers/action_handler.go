package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"ai-incident-platform/backend/internal/api"
	"ai-incident-platform/backend/internal/audit"
	"ai-incident-platform/backend/internal/middleware"
	"ai-incident-platform/backend/internal/security"
	"ai-incident-platform/backend/internal/store"
)

type ActionHandler struct {
	auditStore *store.ActionAuditStore
	agentStore *store.AgentStore
	httpClient *http.Client
}

func NewActionHandler(auditStore *store.ActionAuditStore, agentStore *store.AgentStore) *ActionHandler {
	return &ActionHandler{
		auditStore: auditStore,
		agentStore: agentStore,
		httpClient: &http.Client{Timeout: 35 * time.Second},
	}
}

func (handler *ActionHandler) ExecuteAction(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		api.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	type request struct {
		ActionID    string `json:"action_id"`
		IncidentID  string `json:"incident_id"`
		Label       string `json:"label"`
		Description string `json:"description"`
		ActionType  string `json:"type"`
		RiskLevel   string `json:"risk_level"`
		Approved    bool   `json:"approved"`
	}

	var req request
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		api.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	req.ActionID = strings.TrimSpace(req.ActionID)
	req.IncidentID = strings.TrimSpace(req.IncidentID)

	if req.ActionID == "" {
		api.WriteError(w, http.StatusBadRequest, "action_id is required")
		return
	}
	if req.IncidentID == "" {
		api.WriteError(w, http.StatusBadRequest, "incident_id is required")
		return
	}

	// Extract caller identity from JWT for audit attribution.
	actorID := ""
	tenantID := ""
	if claims, ok := middleware.ClaimsFromContext(r); ok {
		actorID = claims.UserID
		tenantID = claims.TenantID
	}
	sourceIP := extractSourceIP(r)

	label := strings.TrimSpace(req.Label)
	if label == "" {
		label = req.ActionID
	}
	desc := strings.TrimSpace(req.Description)
	if desc == "" {
		desc = "Action executed"
	}

	// Determine execution mode for audit before any command extraction.
	execMode := string(security.ClassifyActionMode(req.ActionType))

	// Extract a candidate shell command from the action label/description.
	rawCommand := extractCommand(label, desc)

	executedAt := time.Now().UTC().Format(time.RFC3339)
	agentOutput := ""
	execStatus := "executed"
	finalCommand := ""
	policyReason := "no executable command in action description"

	if rawCommand != "" {
		// Apply command policy: injection check + allowlist + approval enforcement.
		decision := security.EvaluateCommand(rawCommand, req.ActionType, req.Approved)
		policyReason = decision.Reason

		if !decision.Allowed {
			// Policy blocked the command. Log for security audit and return early.
			slog.WarnContext(r.Context(), "action: policy blocked command", "command", rawCommand, "action_id", req.ActionID, "reason", decision.Reason)

			handler.recordAudit(store.ActionAudit{
				ActionID:      req.ActionID,
				IncidentID:    req.IncidentID,
				Approved:      req.Approved,
				Status:        "policy_blocked",
				Message:       fmt.Sprintf("Command blocked by policy: %s", decision.Reason),
				ExecutedAt:    executedAt,
				ActorID:       actorID,
				TenantID:      tenantID,
				SourceIP:      sourceIP,
				Command:       rawCommand,
				ActionType:    req.ActionType,
				RiskLevel:     req.RiskLevel,
				ExecutionMode: execMode,
				PolicyReason:  decision.Reason,
			})

			audit.Log(tenantID, actorID, "action.policy_blocked", "action", req.ActionID, map[string]interface{}{
				"incident_id":  req.IncidentID,
				"action_type":  req.ActionType,
				"risk_level":   req.RiskLevel,
				"command":      rawCommand,
				"policy_reason": decision.Reason,
				"source_ip":    sourceIP,
			})

			api.WriteError(w, http.StatusForbidden, decision.Reason)
			return
		}

		finalCommand = decision.SanitizedCmd

		// Forward the validated command to the agent for execution.
		agentHost := handler.findActiveAgentHost(tenantID)
		if agentHost != "" {
			output, err := handler.executeOnAgent(agentHost, finalCommand)
			if err != nil {
				agentOutput = fmt.Sprintf("Agent execution failed: %v", err)
				execStatus = "error"
				slog.ErrorContext(r.Context(), "action: agent execute failed", "agent_host", agentHost, "error", err)
			} else {
				agentOutput = output
				execStatus = "executed"
				slog.InfoContext(r.Context(), "action: executed command on agent", "actor_id", actorID, "command", finalCommand, "agent_host", agentHost, "incident_id", req.IncidentID)
			}
		} else {
			agentOutput = "No active agent available for remote execution"
			execStatus = "no_agent"
		}
	} else {
		agentOutput = "This action requires manual execution — no executable command detected"
		execStatus = "manual"
	}

	auditMsg := fmt.Sprintf("%s — %s", label, desc)
	if agentOutput != "" {
		maxOut := agentOutput
		if len(maxOut) > 500 {
			maxOut = maxOut[:500] + "... (truncated)"
		}
		auditMsg = fmt.Sprintf("%s\n\nOutput:\n%s", auditMsg, maxOut)
	}

	handler.recordAudit(store.ActionAudit{
		ActionID:      req.ActionID,
		IncidentID:    req.IncidentID,
		Approved:      req.Approved,
		Status:        execStatus,
		Message:       auditMsg,
		ExecutedAt:    executedAt,
		ActorID:       actorID,
		TenantID:      tenantID,
		SourceIP:      sourceIP,
		Command:       finalCommand,
		ActionType:    req.ActionType,
		RiskLevel:     req.RiskLevel,
		ExecutionMode: execMode,
		PolicyReason:  policyReason,
	})

	// Emit to platform audit log for cross-system compliance queries.
	audit.Log(tenantID, actorID, "action.executed", "action", req.ActionID, map[string]interface{}{
		"incident_id":    req.IncidentID,
		"action_type":    req.ActionType,
		"risk_level":     req.RiskLevel,
		"execution_mode": execMode,
		"command":        finalCommand,
		"status":         execStatus,
		"approved":       req.Approved,
		"source_ip":      sourceIP,
	})

	result := map[string]interface{}{
		"action_id":      req.ActionID,
		"incident_id":    req.IncidentID,
		"approved":       req.Approved,
		"status":         execStatus,
		"message":        auditMsg,
		"output":         agentOutput,
		"command":        finalCommand,
		"execution_mode": execMode,
		"executed_at":    executedAt,
	}

	api.WriteJSON(w, http.StatusOK, result)
}

// executeOnAgent sends a validated command to the agent's /execute endpoint.
func (handler *ActionHandler) executeOnAgent(agentHost, command string) (string, error) {
	url := fmt.Sprintf("http://%s:8765/execute", agentHost)

	body, _ := json.Marshal(map[string]string{"command": command})
	resp, err := handler.httpClient.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("connect to agent: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read agent response: %w", err)
	}

	var result struct {
		Status  string `json:"status"`
		Output  string `json:"output"`
		Error   string `json:"error"`
		Command string `json:"command"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", fmt.Errorf("parse agent response: %w (body: %.200s)", err, respBody)
	}

	if result.Status == "denied" {
		// Agent's own allowlist also rejected it — defense in depth working as intended.
		return result.Output, nil
	}
	if result.Error != "" {
		return fmt.Sprintf("%s\n(exit error: %s)", result.Output, result.Error), nil
	}
	return result.Output, nil
}

// findActiveAgentHost returns the host of the first reachable agent scoped to tenantID.
func (handler *ActionHandler) findActiveAgentHost(tenantID string) string {
	testClient := &http.Client{Timeout: 2 * time.Second}

	for _, host := range []string{"localhost", "127.0.0.1"} {
		if checkAgentHealth(testClient, host) {
			return host
		}
	}

	if handler.agentStore != nil {
		agents, err := handler.agentStore.GetAgents(tenantID, 500, 0)
		if err == nil {
			for _, a := range agents {
				if a.Status == "active" && a.HostIP != "" {
					if checkAgentHealth(testClient, a.HostIP) {
						return a.HostIP
					}
				}
			}
		}
	}

	return ""
}

func checkAgentHealth(client *http.Client, host string) bool {
	resp, err := client.Get(fmt.Sprintf("http://%s:8765/health", host))
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

// extractCommand pulls a single shell command from action label/description.
// It only extracts the first command (stops at newlines) so that multi-step
// instructional descriptions don't accidentally include chained operators.
func extractCommand(label, desc string) string {
	for _, text := range []string{label, desc} {
		lower := strings.ToLower(text)

		for _, prefix := range []string{
			"df ", "du ", "free ", "top ", "uptime", "ps aux", "ps -ef",
			"netstat ", "ss ", "ip addr", "ip route",
			"systemctl status", "systemctl is-active",
			"docker ps", "docker stats", "docker system",
			"kubectl get", "kubectl describe", "kubectl logs", "kubectl top",
			"lsblk", "iostat", "vmstat", "mpstat",
			"cat /proc/", "cat /var/log/",
			"find ", "ls ", "tail ", "head ", "grep ",
			"hostname", "uname ", "mount", "blkid",
			"curl ", "journalctl ",
		} {
			idx := strings.Index(lower, prefix)
			if idx < 0 {
				continue
			}
			cmd := strings.TrimSpace(text[idx:])
			// Stop at the first newline — descriptions are multi-step human instructions,
			// we only extract the first command to avoid picking up chained operators.
			if nlIdx := strings.IndexAny(cmd, "\n\r"); nlIdx > 0 {
				cmd = cmd[:nlIdx]
			}
			if len(cmd) > 200 {
				cmd = cmd[:200]
			}
			return cmd
		}

		// Pattern: "Check disk: df -h" — extract after ": "
		if colonIdx := strings.Index(text, ": "); colonIdx >= 0 {
			after := strings.TrimSpace(text[colonIdx+2:])
			for _, prefix := range []string{
				"df", "du", "free", "top", "ps", "systemctl",
				"docker", "kubectl", "cat", "ls", "tail", "find",
				"curl", "journalctl",
			} {
				if strings.HasPrefix(strings.ToLower(after), prefix) {
					cmd := after
					if nlIdx := strings.IndexAny(cmd, "\n\r"); nlIdx > 0 {
						cmd = cmd[:nlIdx]
					}
					if len(cmd) > 200 {
						cmd = cmd[:200]
					}
					return cmd
				}
			}
		}
	}
	return ""
}

// recordAudit writes to the action audit store, logging errors but not failing the request.
func (handler *ActionHandler) recordAudit(audit store.ActionAudit) {
	if handler.auditStore == nil {
		return
	}
	if err := handler.auditStore.AddAudit(audit); err != nil {
		slog.Error("action: failed to record audit", "action_id", audit.ActionID, "error", err)
	}
}

// extractSourceIP gets the caller's IP from X-Forwarded-For or RemoteAddr.
func extractSourceIP(r *http.Request) string {
	if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
		// X-Forwarded-For may contain a list; take the first entry.
		parts := strings.SplitN(fwd, ",", 2)
		return strings.TrimSpace(parts[0])
	}
	// RemoteAddr is host:port; strip the port.
	addr := r.RemoteAddr
	if colon := strings.LastIndex(addr, ":"); colon >= 0 {
		return addr[:colon]
	}
	return addr
}

func (handler *ActionHandler) GetActionAudit(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		api.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	incidentID := strings.TrimSpace(r.URL.Query().Get("incident_id"))
	if incidentID == "" {
		api.WriteError(w, http.StatusBadRequest, "incident_id is required")
		return
	}

	audits := []store.ActionAudit{}
	if handler.auditStore != nil {
		tenantID := middleware.TenantFromRequest(r)
		audits = handler.auditStore.GetAuditsByIncident(tenantID, incidentID)
	}

	api.WriteJSON(w, http.StatusOK, audits)
}

var defaultActionAuditStore *store.ActionAuditStore
var defaultAgentStore *store.AgentStore

func SetDefaultActionAuditStore(auditStore *store.ActionAuditStore) {
	defaultActionAuditStore = auditStore
}

func SetDefaultAgentStore(agentStore *store.AgentStore) {
	defaultAgentStore = agentStore
}

func ExecuteActionHandler(w http.ResponseWriter, r *http.Request) {
	handler := NewActionHandler(defaultActionAuditStore, defaultAgentStore)
	handler.ExecuteAction(w, r)
}

func GetActionAuditHandler(w http.ResponseWriter, r *http.Request) {
	handler := NewActionHandler(defaultActionAuditStore, defaultAgentStore)
	handler.GetActionAudit(w, r)
}
