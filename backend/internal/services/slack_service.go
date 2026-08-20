package services

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
	"time"

	"ai-incident-platform/backend/internal/models"
)

// SlackService provides Slack API integration using net/http only (no SDK).
type SlackService struct {
	botToken      string
	signingSecret string
	httpClient    *http.Client
}

// SlashCommandResponse is returned to Slack after a slash command.
type SlashCommandResponse struct {
	ResponseType string `json:"response_type"` // "in_channel" or "ephemeral"
	Text         string `json:"text"`
}

func NewSlackService(botToken, signingSecret string) *SlackService {
	return &SlackService{
		botToken:      botToken,
		signingSecret: signingSecret,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// IsConfigured returns true when a bot token is available.
func (s *SlackService) IsConfigured() bool {
	return s.botToken != ""
}

// ─── Channel management ───────────────────────────────────────────────────────

// CreateIncidentChannel creates a Slack channel named inc-{service}-{shortID}.
// Returns the channel ID or "" on error.
func (s *SlackService) CreateIncidentChannel(incident models.Incident) (string, error) {
	if !s.IsConfigured() {
		return "", fmt.Errorf("slack: not configured")
	}

	channelName := buildChannelName(incident.Service, incident.ID)

	payload := map[string]interface{}{
		"name":      channelName,
		"is_private": false,
	}

	resp, err := s.apiCall("conversations.create", payload)
	if err != nil {
		return "", fmt.Errorf("slack: create channel: %w", err)
	}

	channelID, ok := resp["channel"].(map[string]interface{})
	if !ok {
		// Channel might already exist — try to look it up
		slog.Warn("slack: create channel response missing channel field", "channel_name", channelName)
		return "", fmt.Errorf("slack: unexpected response creating channel")
	}

	id, _ := channelID["id"].(string)
	return id, nil
}

// PostIncidentAlert posts a rich Block Kit message to a channel.
func (s *SlackService) PostIncidentAlert(channelID string, incident models.Incident, explanation string) error {
	if !s.IsConfigured() {
		return fmt.Errorf("slack: not configured")
	}

	sev := strings.ToUpper(incident.Severity)
	blocks := []map[string]interface{}{
		{
			"type": "header",
			"text": map[string]interface{}{
				"type": "plain_text",
				"text": fmt.Sprintf("🚨 %s INCIDENT — %s", sev, incident.Service),
			},
		},
		{
			"type": "section",
			"text": map[string]interface{}{
				"type": "mrkdwn",
				"text": fmt.Sprintf("*%s*", incident.Title),
			},
		},
		{
			"type": "section",
			"fields": []map[string]interface{}{
				{"type": "mrkdwn", "text": fmt.Sprintf("*Service:*\n%s", incident.Service)},
				{"type": "mrkdwn", "text": fmt.Sprintf("*Severity:*\n%s", incident.Severity)},
				{"type": "mrkdwn", "text": fmt.Sprintf("*Status:*\n%s", incident.Status)},
				{"type": "mrkdwn", "text": fmt.Sprintf("*Risk Score:*\n%d", incident.RiskScore)},
			},
		},
		{
			"type": "context",
			"elements": []map[string]interface{}{
				{
					"type": "mrkdwn",
					"text": fmt.Sprintf("AI Root Cause: %s", explanation),
				},
			},
		},
		{
			"type": "actions",
			"elements": []map[string]interface{}{
				{
					"type":      "button",
					"text":      map[string]interface{}{"type": "plain_text", "text": "Acknowledge"},
					"style":     "primary",
					"action_id": "ack_incident",
					"value":     incident.ID,
				},
				{
					"type":      "button",
					"text":      map[string]interface{}{"type": "plain_text", "text": "Resolve"},
					"style":     "danger",
					"action_id": "resolve_incident",
					"value":     incident.ID,
				},
			},
		},
	}

	payload := map[string]interface{}{
		"channel": channelID,
		"blocks":  blocks,
	}

	_, err := s.apiCall("chat.postMessage", payload)
	if err != nil {
		return fmt.Errorf("slack: post incident alert: %w", err)
	}
	return nil
}

// PostRCAUpdate posts an updated RCA message to the channel.
func (s *SlackService) PostRCAUpdate(channelID string, detail models.IncidentDetail, narrative string) error {
	if !s.IsConfigured() {
		return fmt.Errorf("slack: not configured")
	}

	blocks := []map[string]interface{}{
		{
			"type": "header",
			"text": map[string]interface{}{
				"type": "plain_text",
				"text": "RCA Update",
			},
		},
		{
			"type": "section",
			"text": map[string]interface{}{
				"type": "mrkdwn",
				"text": fmt.Sprintf("*Incident:* %s\n\n%s", detail.Incident.ID, narrative),
			},
		},
	}

	payload := map[string]interface{}{
		"channel": channelID,
		"blocks":  blocks,
	}

	_, err := s.apiCall("chat.postMessage", payload)
	if err != nil {
		return fmt.Errorf("slack: post rca update: %w", err)
	}
	return nil
}

// PostCommanderAssigned sends a Slack message when an incident commander is assigned.
// Uses the default incident channel (if known) or falls back to a general notification.
func (s *SlackService) PostCommanderAssigned(incident models.Incident, c models.IncidentCommander) error {
	if !s.IsConfigured() {
		return nil
	}
	channelName := "inc-" + incident.ID
	blocks := []map[string]interface{}{
		{
			"type": "section",
			"text": map[string]interface{}{
				"type": "mrkdwn",
				"text": fmt.Sprintf("👤 *Incident Commander Assigned*\n*Incident:* %s\n*Commander:* %s\n*Assigned by:* %s",
					incident.Title, c.UserName, c.AssignedBy),
			},
		},
	}
	payload := map[string]interface{}{
		"channel": channelName,
		"blocks":  blocks,
	}
	_, err := s.apiCall("chat.postMessage", payload)
	return err
}

// HandleSlashCommand processes a Slack slash command and returns a response.
// Supports: ack <incidentID>, resolve <incidentID>, assign <incidentID> <user>, status <incidentID>.
func (s *SlackService) HandleSlashCommand(command, text, userID string) SlashCommandResponse {
	parts := strings.Fields(strings.TrimSpace(text))
	if len(parts) < 1 {
		return SlashCommandResponse{
			ResponseType: "ephemeral",
			Text:         "Usage: /incident ack <incidentID> | /incident resolve <incidentID>",
		}
	}

	action := strings.ToLower(parts[0])
	var incidentID string
	if len(parts) >= 2 {
		incidentID = parts[1]
	}

	switch action {
	case "ack", "acknowledge":
		if incidentID == "" {
			return SlashCommandResponse{ResponseType: "ephemeral", Text: "Usage: ack <incidentID>"}
		}
		return SlashCommandResponse{
			ResponseType: "in_channel",
			Text:         fmt.Sprintf("Incident %s acknowledged by <@%s>", incidentID, userID),
		}
	case "resolve":
		if incidentID == "" {
			return SlashCommandResponse{ResponseType: "ephemeral", Text: "Usage: resolve <incidentID>"}
		}
		return SlashCommandResponse{
			ResponseType: "in_channel",
			Text:         fmt.Sprintf("Incident %s resolved by <@%s>", incidentID, userID),
		}
	default:
		return SlashCommandResponse{
			ResponseType: "ephemeral",
			Text:         fmt.Sprintf("Unknown action '%s'. Supported: ack, resolve", action),
		}
	}
}

// VerifyRequest validates the Slack signing secret for incoming webhooks.
// It computes HMAC-SHA256 of "v0:{timestamp}:{body}" and compares to the
// provided signature (with "v0=" prefix stripped).
func (s *SlackService) VerifyRequest(timestamp, signature string, body []byte) bool {
	if s.signingSecret == "" {
		return true // allow-all when not configured
	}

	sigBase := fmt.Sprintf("v0:%s:%s", timestamp, string(body))
	mac := hmac.New(sha256.New, []byte(s.signingSecret))
	mac.Write([]byte(sigBase))
	expected := "v0=" + hex.EncodeToString(mac.Sum(nil))

	return hmac.Equal([]byte(expected), []byte(signature))
}

// ─── Remediation approval ─────────────────────────────────────────────────────

// PostRemediationApprovalRequest sends a Slack Block Kit message to channelID with
// [Approve] and [Reject] buttons. The button values carry execID so the interaction
// callback knows which execution to approve/reject.
func (s *SlackService) PostRemediationApprovalRequest(channelID, incidentID, execID string, actionLabel, actionDesc, policyReason string) error {
	if !s.IsConfigured() {
		return fmt.Errorf("slack not configured")
	}

	payload := map[string]interface{}{
		"channel": channelID,
		"blocks": []map[string]interface{}{
			{
				"type": "header",
				"text": map[string]string{
					"type": "plain_text",
					"text": ":rotating_light: Remediation Approval Required",
				},
			},
			{
				"type": "section",
				"fields": []map[string]string{
					{"type": "mrkdwn", "text": fmt.Sprintf("*Incident:*\n%s", incidentID)},
					{"type": "mrkdwn", "text": fmt.Sprintf("*Action:*\n%s", actionLabel)},
				},
			},
			{
				"type": "section",
				"text": map[string]string{
					"type": "mrkdwn",
					"text": fmt.Sprintf("*Why manual approval?*\n%s\n\n*What will run:*\n```%s```", policyReason, truncateForSlack(actionDesc, 300)),
				},
			},
			{
				"type": "actions",
				"elements": []map[string]interface{}{
					{
						"type":      "button",
						"action_id": "remediation_approve",
						"text":      map[string]string{"type": "plain_text", "text": ":white_check_mark: Approve"},
						"style":     "primary",
						"value":     execID,
						"confirm": map[string]interface{}{
							"title":   map[string]string{"type": "plain_text", "text": "Run remediation?"},
							"text":    map[string]string{"type": "mrkdwn", "text": fmt.Sprintf("This will execute *%s* on the incident service. Are you sure?", actionLabel)},
							"confirm": map[string]string{"type": "plain_text", "text": "Yes, execute"},
							"deny":    map[string]string{"type": "plain_text", "text": "Cancel"},
						},
					},
					{
						"type":      "button",
						"action_id": "remediation_reject",
						"text":      map[string]string{"type": "plain_text", "text": ":x: Reject"},
						"style":     "danger",
						"value":     execID,
					},
				},
			},
		},
	}

	_, err := s.apiCall("chat.postMessage", payload)
	return err
}

// VerifyRequest is already defined; this helper is for internal use only.
func truncateForSlack(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "…"
}

// ─── Internal helpers ─────────────────────────────────────────────────────────

func (s *SlackService) apiCall(method string, payload interface{}) (map[string]interface{}, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal payload: %w", err)
	}

	url := "https://slack.com/api/" + method
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+s.botToken)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http call: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	ok, _ := result["ok"].(bool)
	if !ok {
		errMsg, _ := result["error"].(string)
		return nil, fmt.Errorf("slack API error: %s", errMsg)
	}

	return result, nil
}

// nonAlphanumRe matches characters that are not lowercase letters or digits.
var nonAlphanumRe = regexp.MustCompile(`[^a-z0-9]+`)

// buildChannelName produces inc-{service[:10]}-{incidentID[4:12]}, all lowercase.
func buildChannelName(service, incidentID string) string {
	svc := strings.ToLower(service)
	if len(svc) > 10 {
		svc = svc[:10]
	}
	svc = nonAlphanumRe.ReplaceAllString(svc, "-")
	svc = strings.Trim(svc, "-")

	// incidentID is like "inc-1234567890"; skip the "inc-" prefix.
	short := incidentID
	if strings.HasPrefix(short, "inc-") {
		short = short[4:]
	}
	if len(short) > 8 {
		short = short[:8]
	}

	return fmt.Sprintf("inc-%s-%s", svc, short)
}
