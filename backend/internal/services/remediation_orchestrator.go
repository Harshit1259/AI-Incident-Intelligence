package services

// remediation_orchestrator.go — Feature 3: Auto-Remediation with Human-in-the-Loop
//
// Full closed-loop flow:
//   Incident created (high/critical)
//     → AI suggests top remediation action (BuildActions)
//     → PolicyService evaluates safety (8-check engine)
//     → If auto + confidence ≥ 90: execute on agent immediately
//     → If approval required: send Slack message with [Approve][Reject]
//     → On approval: execute on agent
//     → VerificationService runs before/after snapshot
//     → If verification passes: incident auto-closed + resolution stored in memory

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"ai-incident-platform/backend/internal/audit"
	"ai-incident-platform/backend/internal/models"
	"ai-incident-platform/backend/internal/security"
	"ai-incident-platform/backend/internal/store"
)

const autoExecuteMinConfidence = 90

// RemediationOrchestrator wires together the full closed-loop automation flow.
type RemediationOrchestrator struct {
	incidentStore        *store.IncidentStore
	actionExecutionStore *store.ActionExecutionStore
	agentStore           *store.AgentStore
	policyService        *PolicyService
	slackService         *SlackService
	verificationService  *VerificationService
	incidentMemoryService *IncidentMemoryService
	incidentService      *IncidentService
	httpClient           *http.Client
}

// NewRemediationOrchestrator creates the orchestrator. All deps may be nil-safe.
func NewRemediationOrchestrator(
	incidentStore *store.IncidentStore,
	execStore *store.ActionExecutionStore,
	agentStore *store.AgentStore,
	policySvc *PolicyService,
	slackSvc *SlackService,
	verifySvc *VerificationService,
	memorySvc *IncidentMemoryService,
	incidentSvc *IncidentService,
) *RemediationOrchestrator {
	return &RemediationOrchestrator{
		incidentStore:         incidentStore,
		actionExecutionStore:  execStore,
		agentStore:            agentStore,
		policyService:         policySvc,
		slackService:          slackSvc,
		verificationService:   verifySvc,
		incidentMemoryService: memorySvc,
		incidentService:       incidentSvc,
		httpClient:            &http.Client{Timeout: 35 * time.Second},
	}
}

// TriggerRemediation is called (asynchronously) when a new high/critical incident
// is created. It selects the top remediation action, evaluates the policy engine,
// then either auto-executes or sends a Slack approval request.
// rcaConfidence is the confidence in the CAUSE, from a completed analysis.
// It is a pointer because nil is meaningful: it means nothing has analysed this
// incident, which is different from an analysis that returned low confidence.
//
// Passing nil guarantees the action cannot auto-execute — it routes to human
// approval instead. That is deliberate: the previous implementation gated on
// incident.Confidence, which is seeded from severity (critical = 75) and grows
// +5 per correlated alert, so three merges on a critical incident cleared the
// 90 threshold and fired a remediation on the strength of a number that only
// ever encoded "this alert was severe and noisy".
func (o *RemediationOrchestrator) TriggerRemediation(incident models.Incident, rcaConfidence *int) {
	if incident.Severity != "critical" && incident.Severity != "high" {
		return
	}

	// ── Step 1: Build action suggestions ──────────────────────────────────────
	insight := buildInsight(incident.CorrelationPattern, incident, nil)
	actions := BuildActions(incident, insight)

	// Pick the first remediation action (highest-priority after sorting).
	var chosen *models.Action
	for i := range actions {
		if actions[i].Type == "remediation" {
			chosen = &actions[i]
			break
		}
	}
	if chosen == nil {
		return // no remediation action available
	}

	// ── Step 2: Policy evaluation ─────────────────────────────────────────────
	policyCtx := models.PolicyEvalContext{
		TenantID:         incident.TenantID,
		IncidentID:       incident.ID,
		ActionID:         chosen.ID,
		Service:          incident.Service,
		Environment:      "production",
		Severity:         incident.Severity,
		ImpactedServices: incident.ImpactCount,
	}

	decision := o.policyService.Evaluate(policyCtx)

	// ── Step 3: Create execution record ───────────────────────────────────────
	execStatus := "pending_approval"
	approvalStatus := "pending"
	execMode := decision.ExecutionMode

	// Auto-execution requires a real analysis to have produced a cause, and for
	// that analysis to be confident. An unanalysed incident (rcaConfidence nil)
	// can never auto-execute regardless of severity or alert volume.
	autoApproved := decision.Allowed &&
		decision.ExecutionMode == "auto" &&
		rcaConfidence != nil &&
		*rcaConfidence >= autoExecuteMinConfidence

	if decision.Allowed && decision.ExecutionMode == "auto" && rcaConfidence == nil {
		slog.Info("remediation: policy permits auto-execution but no RCA exists — routing to approval",
			"incident_id", incident.ID, "action_id", chosen.ID)
	}

	if autoApproved {
		execStatus = "planned"
		approvalStatus = ""
		execMode = "auto"
	}

	exec := models.ActionExecution{
		ID:               fmt.Sprintf("exec-%d", time.Now().UnixNano()),
		TenantID:         incident.TenantID,
		IncidentID:       incident.ID,
		ActionID:         chosen.ID,
		ActionLabel:      chosen.Label,
		ActionDesc:       chosen.Description,
		ChainPosition:    1,
		Status:           execStatus,
		ExecutionMode:    execMode,
		RequiresApproval: chosen.RequiresApproval,
		ApprovalStatus:   approvalStatus,
		CreatedAt:        time.Now(),
	}

	if err := o.actionExecutionStore.Create(exec); err != nil {
		slog.Error("orchestrator: failed to create execution record", "error", err)
		return
	}

	// ── Step 4a: Auto-execute ─────────────────────────────────────────────────
	if autoApproved {
		slog.Info("orchestrator: auto-executing action",
			"incident_id", incident.ID,
			"action_id", chosen.ID,
			"confidence", incident.Confidence,
		)
		o.runExecution(exec, incident, chosen, "system:orchestrator")
		return
	}

	// ── Step 4b: Request human approval via Slack ─────────────────────────────
	if !decision.Allowed {
		slog.Info("orchestrator: policy denied auto-execution, skipping approval request",
			"incident_id", incident.ID,
			"denial_code", decision.DenialCode,
		)
		return
	}

	channelID := o.incidentStore.GetSlackChannelID(incident.ID)
	if channelID == "" || o.slackService == nil || !o.slackService.IsConfigured() {
		slog.Info("orchestrator: no Slack channel for approval request", "incident_id", incident.ID)
		return
	}

	reason := decision.Reason
	if reason == "" {
		reason = fmt.Sprintf("Policy requires approval for %s risk %s actions.", chosen.RiskLevel, chosen.Type)
	}

	if err := o.slackService.PostRemediationApprovalRequest(
		channelID, incident.ID, exec.ID,
		chosen.Label, chosen.Description, reason,
	); err != nil {
		slog.Error("orchestrator: failed to post Slack approval request",
			"incident_id", incident.ID,
			"error", err,
		)
	}
}

// ExecuteApprovedAction is called by the Slack interaction handler when an operator
// clicks [Approve]. It runs the agent command, verifies, and auto-closes if safe.
func (o *RemediationOrchestrator) ExecuteApprovedAction(execID, approvedBy string) error {
	exec, err := o.actionExecutionStore.GetByID(execID)
	if err != nil || exec == nil {
		return fmt.Errorf("execution not found: %s", execID)
	}
	if exec.ApprovalStatus != "pending" {
		return fmt.Errorf("execution %s is not pending approval (status: %s)", execID, exec.ApprovalStatus)
	}

	// Mark approved.
	if err := o.actionExecutionStore.Approve(execID, approvedBy, time.Now()); err != nil {
		return fmt.Errorf("approve execution: %w", err)
	}

	incident, found := o.incidentStore.GetIncidentByID(exec.IncidentID)
	if !found {
		return fmt.Errorf("incident not found: %s", exec.IncidentID)
	}

	action := &models.Action{
		ID:          exec.ActionID,
		Label:       exec.ActionLabel,
		Description: exec.ActionDesc,
		Type:        "remediation",
		RiskLevel:   "medium",
	}

	o.runExecution(*exec, incident, action, approvedBy)
	return nil
}

// RejectAction records a rejection from the Slack interaction callback.
func (o *RemediationOrchestrator) RejectAction(execID, rejectedBy, reason string) error {
	exec, err := o.actionExecutionStore.GetByID(execID)
	if err != nil || exec == nil {
		return fmt.Errorf("execution not found: %s", execID)
	}
	if exec.ApprovalStatus != "pending" {
		return fmt.Errorf("execution %s is not pending approval", execID)
	}
	return o.actionExecutionStore.Reject(execID, rejectedBy, reason)
}

// ─── Core execution + verification + close loop ───────────────────────────────

// runExecution executes the action on the agent, runs verification, and on success:
//  1. auto-closes the incident
//  2. records the resolution in incident memory for future auto-approval
func (o *RemediationOrchestrator) runExecution(exec models.ActionExecution, incident models.Incident, action *models.Action, actor string) {
	now := time.Now()
	exec.StartedAt = &now
	exec.Status = "running"
	_ = o.actionExecutionStore.Update(exec)

	// ── Before-snapshot ───────────────────────────────────────────────────────
	var vrID string
	if o.verificationService != nil {
		vr, err := o.verificationService.StartVerification(incident.ID, exec.ID)
		if err == nil && vr != nil {
			vrID = vr.ID
		}
	}

	// ── Agent execution ───────────────────────────────────────────────────────
	rawCmd := extractOrchestratorCommand(action.Label, action.Description)
	agentOutput := "No executable command detected — action requires manual execution."
	execStatus := "manual"

	if rawCmd != "" {
		decision := security.EvaluateCommand(rawCmd, action.Type, true)
		if !decision.Allowed {
			agentOutput = fmt.Sprintf("Command blocked by security policy: %s", decision.Reason)
			execStatus = "policy_blocked"
		} else {
			agentHost := o.findActiveAgentHost(incident.TenantID)
			if agentHost != "" {
				output, err := o.callAgent(agentHost, decision.SanitizedCmd)
				if err != nil {
					agentOutput = fmt.Sprintf("Agent execution failed: %v", err)
					execStatus = "error"
				} else {
					agentOutput = output
					execStatus = "executed"
				}
			} else {
				agentOutput = "No active agent available — action queued for manual execution."
				execStatus = "no_agent"
			}
		}
	}

	// ── Update execution record ───────────────────────────────────────────────
	completed := time.Now()
	exec.CompletedAt = &completed
	exec.Status = execStatus
	exec.ResultOutput = agentOutput
	_ = o.actionExecutionStore.Update(exec)

	audit.Log(incident.TenantID, actor, "remediation.executed", "execution", exec.ID, map[string]interface{}{
		"incident_id":   incident.ID,
		"action_id":     action.ID,
		"status":        execStatus,
		"execution_mode": exec.ExecutionMode,
	})

	// ── After-snapshot + verification ─────────────────────────────────────────
	if vrID == "" || o.verificationService == nil {
		return
	}

	vr, err := o.verificationService.CompleteVerification(vrID)
	if err != nil || vr == nil {
		return
	}

	// ── Auto-close if verification passed ─────────────────────────────────────
	if vr.AutoCloseEligible && o.incidentService != nil {
		_, err := o.incidentService.UpdateIncidentStatus(incident.ID, "resolve")
		if err != nil {
			slog.Error("orchestrator: auto-close failed", "incident_id", incident.ID, "error", err)
		} else {
			slog.Info("orchestrator: incident auto-closed after successful verification",
				"incident_id", incident.ID,
				"action_id", action.ID,
			)
		}
	}

	// ── Store resolution in Incident Memory ───────────────────────────────────
	if vr.Result != "not_resolved" && o.incidentMemoryService != nil {
		note := fmt.Sprintf("Auto-remediated via %q (%s). Verification: %s.", action.Label, exec.ExecutionMode, vr.Result)
		_ = o.incidentMemoryService.RecordResolution(incident.ID, incident.TenantID, note)
	}
}

// ─── Agent helpers (mirrors ActionHandler's private methods) ─────────────────

func (o *RemediationOrchestrator) findActiveAgentHost(tenantID string) string {
	testClient := &http.Client{Timeout: 2 * time.Second}
	for _, host := range []string{"localhost", "127.0.0.1"} {
		if orchestratorCheckAgentHealth(testClient, host) {
			return host
		}
	}
	if o.agentStore != nil {
		agents, err := o.agentStore.GetAgents(tenantID, 500, 0)
		if err == nil {
			for _, a := range agents {
				if a.Status == "active" && a.HostIP != "" {
					if orchestratorCheckAgentHealth(testClient, a.HostIP) {
						return a.HostIP
					}
				}
			}
		}
	}
	return ""
}

func (o *RemediationOrchestrator) callAgent(agentHost, command string) (string, error) {
	url := fmt.Sprintf("http://%s:8765/execute", agentHost)
	body, _ := json.Marshal(map[string]string{"command": command})
	resp, err := o.httpClient.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("connect to agent: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read agent response: %w", err)
	}

	var result struct {
		Status string `json:"status"`
		Output string `json:"output"`
		Error  string `json:"error"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return string(respBody), nil // return raw on parse error
	}
	if result.Error != "" {
		return fmt.Sprintf("%s\n(error: %s)", result.Output, result.Error), nil
	}
	return result.Output, nil
}

func orchestratorCheckAgentHealth(client *http.Client, host string) bool {
	resp, err := client.Get(fmt.Sprintf("http://%s:8765/health", host))
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

// extractOrchestratorCommand pulls the first shell command from an action's
// label or description. Intentionally conservative — only diagnostic prefixes.
func extractOrchestratorCommand(label, desc string) string {
	import_strings_lower := func(s string) string {
		result := ""
		for _, c := range s {
			if c >= 'A' && c <= 'Z' {
				result += string(c + 32)
			} else {
				result += string(c)
			}
		}
		return result
	}

	safePrefix := []string{
		"systemctl restart ", "systemctl stop ", "systemctl start ",
		"kubectl rollout restart ", "kubectl scale ",
		"df ", "free ", "top ", "uptime", "ps aux",
		"journalctl ", "curl ", "grep ", "tail ", "cat /var/log/",
	}

	for _, text := range []string{label, desc} {
		lower := import_strings_lower(text)
		for _, prefix := range safePrefix {
			if idx := indexStr(lower, prefix); idx >= 0 {
				cmd := trimStr(text[idx:])
				if nl := indexAny(cmd, "\n\r"); nl > 0 {
					cmd = cmd[:nl]
				}
				if len(cmd) > 200 {
					cmd = cmd[:200]
				}
				return cmd
			}
		}
	}
	return ""
}

// string helpers to avoid importing strings (already imported via services package conventions)
func indexStr(s, sub string) int {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

func indexAny(s, chars string) int {
	for i, c := range s {
		for _, ch := range chars {
			if c == ch {
				return i
			}
		}
	}
	return -1
}

func trimStr(s string) string {
	start := 0
	for start < len(s) && (s[start] == ' ' || s[start] == '\t') {
		start++
	}
	end := len(s)
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t') {
		end--
	}
	return s[start:end]
}
