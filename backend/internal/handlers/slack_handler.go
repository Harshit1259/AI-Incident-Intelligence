package handlers

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"ai-incident-platform/backend/internal/api"
	"ai-incident-platform/backend/internal/services"
	"ai-incident-platform/backend/internal/store"
)

// SlackHandler handles Slack slash commands and interactive component callbacks.
type SlackHandler struct {
	slackService            *services.SlackService
	incidentStore           *store.IncidentStore
	remediationOrchestrator remediationDispatcher
}

// remediationDispatcher is the minimal interface the handler needs from RemediationOrchestrator.
type remediationDispatcher interface {
	ExecuteApprovedAction(execID, approvedBy string) error
	RejectAction(execID, rejectedBy, reason string) error
}

func NewSlackHandler(ss *services.SlackService, is *store.IncidentStore) *SlackHandler {
	return &SlackHandler{slackService: ss, incidentStore: is}
}

// SetRemediationOrchestrator wires the orchestrator for approve/reject callbacks.
func (h *SlackHandler) SetRemediationOrchestrator(o remediationDispatcher) {
	h.remediationOrchestrator = o
}

// HandleSlashCommand handles POST /api/v1/slack/command from Slack.
func (h *SlackHandler) HandleSlashCommand(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		api.WriteError(w, http.StatusBadRequest, "failed to read body")
		return
	}
	defer r.Body.Close()

	// Verify Slack signature.
	timestamp := r.Header.Get("X-Slack-Request-Timestamp")
	signature := r.Header.Get("X-Slack-Signature")
	if !h.slackService.VerifyRequest(timestamp, signature, body) {
		api.WriteError(w, http.StatusUnauthorized, "invalid signature")
		return
	}

	// Parse form fields from the body.
	if err := r.ParseForm(); err != nil {
		// Fall back to parsing the raw body.
		r.Body = io.NopCloser(strings.NewReader(string(body)))
	}

	// Re-parse form from body bytes since we already read the body.
	formValues := parseFormBody(string(body))
	command := formValues["command"]
	text := formValues["text"]
	userID := formValues["user_id"]

	// Parse the text: "{action} {incidentID}"
	parts := strings.Fields(strings.TrimSpace(text))
	action := ""
	incidentID := ""
	if len(parts) >= 1 {
		action = strings.ToLower(parts[0])
	}
	if len(parts) >= 2 {
		incidentID = parts[1]
	}

	resp := h.slackService.HandleSlashCommand(command, text, userID)

	// Side-effect: update incident status in DB.
	if incidentID != "" {
		switch action {
		case "ack", "acknowledge":
			if _, err := h.incidentStore.UpdateIncidentStatus(incidentID, "acknowledged"); err != nil {
				slog.ErrorContext(r.Context(), "slack_handler: failed to ack incident", "incident_id", incidentID, "error", err)
			}
		case "resolve":
			if _, err := h.incidentStore.UpdateIncidentStatus(incidentID, "resolved"); err != nil {
				slog.ErrorContext(r.Context(), "slack_handler: failed to resolve incident", "incident_id", incidentID, "error", err)
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
}

// HandleInteraction handles POST /api/v1/slack/interaction for button clicks.
func (h *SlackHandler) HandleInteraction(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		api.WriteError(w, http.StatusBadRequest, "failed to read body")
		return
	}
	defer r.Body.Close()

	// Verify Slack signature.
	timestamp := r.Header.Get("X-Slack-Request-Timestamp")
	signature := r.Header.Get("X-Slack-Signature")
	if !h.slackService.VerifyRequest(timestamp, signature, body) {
		api.WriteError(w, http.StatusUnauthorized, "invalid signature")
		return
	}

	// The payload is in the "payload" form field as JSON.
	formValues := parseFormBody(string(body))
	payloadStr := formValues["payload"]
	if payloadStr == "" {
		api.WriteError(w, http.StatusBadRequest, "missing payload field")
		return
	}

	var interactionPayload struct {
		Type    string `json:"type"`
		Actions []struct {
			ActionID string `json:"action_id"`
			Value    string `json:"value"`
		} `json:"actions"`
		User struct {
			ID string `json:"id"`
		} `json:"user"`
	}

	if err := json.Unmarshal([]byte(payloadStr), &interactionPayload); err != nil {
		api.WriteError(w, http.StatusBadRequest, "invalid payload JSON")
		return
	}

	for _, action := range interactionPayload.Actions {
		value := action.Value
		userID := interactionPayload.User.ID
		switch action.ActionID {
		case "ack_incident":
			if _, err := h.incidentStore.UpdateIncidentStatus(value, "acknowledged"); err != nil {
				slog.ErrorContext(r.Context(), "slack_handler: failed to ack incident", "incident_id", value, "error", err)
			} else {
				slog.InfoContext(r.Context(), "slack_handler: incident acknowledged", "incident_id", value, "user_id", userID)
			}
		case "resolve_incident":
			if _, err := h.incidentStore.UpdateIncidentStatus(value, "resolved"); err != nil {
				slog.ErrorContext(r.Context(), "slack_handler: failed to resolve incident", "incident_id", value, "error", err)
			} else {
				slog.InfoContext(r.Context(), "slack_handler: incident resolved", "incident_id", value, "user_id", userID)
			}
		case "remediation_approve":
			if h.remediationOrchestrator != nil {
				go func(execID, actor string) {
					if err := h.remediationOrchestrator.ExecuteApprovedAction(execID, "slack:"+actor); err != nil {
						slog.Error("slack_handler: remediation approve failed", "exec_id", execID, "error", err)
					}
				}(value, userID)
			}
		case "remediation_reject":
			if h.remediationOrchestrator != nil {
				if err := h.remediationOrchestrator.RejectAction(value, "slack:"+userID, "rejected via Slack"); err != nil {
					slog.ErrorContext(r.Context(), "slack_handler: remediation reject failed", "exec_id", value, "error", err)
				}
			}
		}
	}

	// Respond with 200 OK to acknowledge the interaction.
	w.WriteHeader(http.StatusOK)
}

// parseFormBody parses URL-encoded form data from a string body.
func parseFormBody(body string) map[string]string {
	result := make(map[string]string)
	for _, part := range strings.Split(body, "&") {
		kv := strings.SplitN(part, "=", 2)
		if len(kv) == 2 {
			key := urlDecode(kv[0])
			val := urlDecode(kv[1])
			result[key] = val
		}
	}
	return result
}

// urlDecode decodes URL-encoded strings (+→space, %XX→char).
func urlDecode(s string) string {
	result := strings.ReplaceAll(s, "+", " ")
	// Use net/url's QueryUnescape-equivalent inline.
	var buf strings.Builder
	i := 0
	for i < len(result) {
		if result[i] == '%' && i+2 < len(result) {
			hi := hexVal(result[i+1])
			lo := hexVal(result[i+2])
			if hi >= 0 && lo >= 0 {
				buf.WriteByte(byte(hi<<4 | lo))
				i += 3
				continue
			}
		}
		buf.WriteByte(result[i])
		i++
	}
	return buf.String()
}

func hexVal(c byte) int {
	switch {
	case c >= '0' && c <= '9':
		return int(c - '0')
	case c >= 'a' && c <= 'f':
		return int(c-'a') + 10
	case c >= 'A' && c <= 'F':
		return int(c-'A') + 10
	}
	return -1
}
