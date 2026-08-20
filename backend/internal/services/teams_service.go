package services

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"ai-incident-platform/backend/internal/models"
)

// TeamsService posts notifications to Microsoft Teams via Incoming Webhooks.
// Outgoing webhooks from Teams (for bi-directional commands) arrive on
// POST /api/v1/teams/webhook and are handled by TeamsHandler.
type TeamsService struct {
	webhookURL string // Incoming Webhook URL configured in Teams
	httpClient *http.Client
}

func NewTeamsService(webhookURL string) *TeamsService {
	return &TeamsService{
		webhookURL: webhookURL,
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

func (s *TeamsService) IsConfigured() bool { return s.webhookURL != "" }

// PostIncidentAlert sends a new-incident notification card to Teams.
func (s *TeamsService) PostIncidentAlert(incident models.Incident, explanation string) error {
	if !s.IsConfigured() {
		return nil
	}

	severityColor := map[string]string{
		"critical": "FF0000",
		"high":     "FFA500",
		"medium":   "FFFF00",
		"low":      "00AA00",
	}
	color := severityColor[incident.Severity]
	if color == "" {
		color = "888888"
	}

	body := map[string]any{
		"@type":    "MessageCard",
		"@context": "http://schema.org/extensions",
		"themeColor": color,
		"summary":  fmt.Sprintf("Incident: %s", incident.Title),
		"sections": []map[string]any{
			{
				"activityTitle":    fmt.Sprintf("🚨 %s", incident.Title),
				"activitySubtitle": fmt.Sprintf("Severity: %s | Service: %s", incident.Severity, incident.Service),
				"activityText":     explanation,
				"facts": []map[string]string{
					{"name": "Incident ID", "value": incident.ID},
					{"name": "Status", "value": incident.Status},
					{"name": "Started", "value": incident.FirstEventTime.Format(time.RFC3339)},
				},
				"markdown": true,
			},
		},
		"potentialAction": []map[string]any{
			{
				"@type": "OpenUri",
				"name":  "View Incident",
				"targets": []map[string]string{
					{"os": "default", "uri": fmt.Sprintf("/incidents/%s", incident.ID)},
				},
			},
		},
	}
	return s.post(body)
}

// PostCommanderAssigned notifies the channel when a commander is assigned.
func (s *TeamsService) PostCommanderAssigned(incident models.Incident, commander models.IncidentCommander) error {
	if !s.IsConfigured() {
		return nil
	}
	body := map[string]any{
		"@type":    "MessageCard",
		"@context": "http://schema.org/extensions",
		"themeColor": "0078D4",
		"summary":  fmt.Sprintf("Commander assigned: %s", incident.Title),
		"sections": []map[string]any{
			{
				"activityTitle":    fmt.Sprintf("👤 Incident Commander Assigned"),
				"activitySubtitle": incident.Title,
				"facts": []map[string]string{
					{"name": "Commander", "value": commander.UserName},
					{"name": "Assigned by", "value": commander.AssignedBy},
					{"name": "Incident", "value": incident.ID},
				},
			},
		},
	}
	return s.post(body)
}

// PostStatusUpdate posts a customer-facing status communication to Teams.
func (s *TeamsService) PostStatusUpdate(sc models.StatusCommunication) error {
	if !s.IsConfigured() {
		return nil
	}
	stageColor := map[string]string{
		"investigating": "FFA500",
		"identified":    "FFFF00",
		"monitoring":    "0078D4",
		"resolved":      "00AA00",
	}
	color := stageColor[sc.Stage]
	if color == "" {
		color = "888888"
	}
	body := map[string]any{
		"@type":      "MessageCard",
		"@context":   "http://schema.org/extensions",
		"themeColor": color,
		"summary":    sc.Title,
		"sections": []map[string]any{
			{
				"activityTitle":    fmt.Sprintf("📢 %s", sc.Title),
				"activitySubtitle": fmt.Sprintf("Status: %s", sc.Stage),
				"activityText":     sc.Body,
			},
		},
	}
	return s.post(body)
}

// HandleOutgoingWebhook processes a text message from a Teams outgoing webhook.
// Teams sends the message in JSON; we return an allowed response card.
// Supported commands: "ack <incidentID>", "resolve <incidentID>", "status <incidentID>"
func (s *TeamsService) HandleOutgoingWebhook(text string) (string, error) {
	parts := splitCommand(text)
	if len(parts) < 2 {
		return "Usage: ack|resolve|status <incidentID>", nil
	}
	action := toLower(parts[0])
	id := parts[1]
	switch action {
	case "ack", "acknowledge":
		return fmt.Sprintf("✅ Incident %s acknowledged", id), nil
	case "resolve":
		return fmt.Sprintf("✅ Incident %s resolved", id), nil
	case "status":
		return fmt.Sprintf("ℹ️ Incident %s — use the AIOps portal for full details", id), nil
	default:
		return fmt.Sprintf("Unknown command: %s. Supported: ack, resolve, status", action), nil
	}
}

func (s *TeamsService) post(payload any) error {
	b, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodPost, s.webhookURL, bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return fmt.Errorf("teams webhook %d: %s", resp.StatusCode, string(body))
	}
	return nil
}

func splitCommand(text string) []string {
	var parts []string
	word := ""
	for _, r := range text {
		if r == ' ' || r == '\t' || r == '\n' {
			if word != "" {
				parts = append(parts, word)
				word = ""
			}
		} else {
			word += string(r)
		}
	}
	if word != "" {
		parts = append(parts, word)
	}
	return parts
}

func toLower(s string) string {
	result := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			c += 32
		}
		result[i] = c
	}
	return string(result)
}
