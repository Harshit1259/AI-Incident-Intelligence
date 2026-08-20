package services

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"ai-incident-platform/backend/internal/models"
)

// JiraService creates and syncs JIRA issues via the JIRA Cloud REST API v3.
// Authentication: Basic auth using email + API token (base64-encoded).
type JiraService struct {
	baseURL    string // e.g. "https://your-org.atlassian.net"
	email      string
	apiToken   string
	projectKey string // e.g. "OPS"
	httpClient *http.Client
}

func NewJiraService(baseURL, email, apiToken, projectKey string) *JiraService {
	return &JiraService{
		baseURL:    baseURL,
		email:      email,
		apiToken:   apiToken,
		projectKey: projectKey,
		httpClient: &http.Client{Timeout: 15 * time.Second},
	}
}

func (s *JiraService) IsConfigured() bool {
	return s.baseURL != "" && s.email != "" && s.apiToken != ""
}

// CreateIssue opens a JIRA issue for the incident and returns ticket key + URL.
func (s *JiraService) CreateIssue(incident models.Incident, description, priority string) (*models.TicketSync, error) {
	if !s.IsConfigured() {
		return nil, fmt.Errorf("jira: not configured")
	}

	jiraPriority := mapPriorityToJira(priority, incident.Severity)

	body := map[string]any{
		"fields": map[string]any{
			"project":   map[string]string{"key": s.projectKey},
			"summary":   fmt.Sprintf("[%s] %s", incident.Severity, incident.Title),
			"issuetype": map[string]string{"name": "Bug"},
			"priority":  map[string]string{"name": jiraPriority},
			"description": map[string]any{
				"type":    "doc",
				"version": 1,
				"content": []map[string]any{
					{
						"type": "paragraph",
						"content": []map[string]any{
							{"type": "text", "text": buildIssueDescription(incident, description)},
						},
					},
				},
			},
			"labels": []string{"aiops-incident", incident.Service},
		},
	}

	resp, err := s.apiPost("/rest/api/3/issue", body)
	if err != nil {
		return nil, fmt.Errorf("jira create issue: %w", err)
	}

	key, _ := resp["key"].(string)
	id, _ := resp["id"].(string)
	if key == "" && id == "" {
		return nil, fmt.Errorf("jira: unexpected response — no key in: %v", resp)
	}

	return &models.TicketSync{
		Provider:     models.ProviderJira,
		IncidentID:   incident.ID,
		TicketKey:    key,
		TicketURL:    fmt.Sprintf("%s/browse/%s", s.baseURL, key),
		TicketStatus: "open",
	}, nil
}

// GetIssueStatus fetches the current status of a JIRA issue.
func (s *JiraService) GetIssueStatus(ticketKey string) (string, error) {
	if !s.IsConfigured() {
		return "", fmt.Errorf("jira: not configured")
	}
	resp, err := s.apiGet(fmt.Sprintf("/rest/api/3/issue/%s?fields=status", ticketKey))
	if err != nil {
		return "", err
	}
	fields, _ := resp["fields"].(map[string]any)
	if fields == nil {
		return "", nil
	}
	status, _ := fields["status"].(map[string]any)
	name, _ := status["name"].(string)
	return name, nil
}

// ── internal ──────────────────────────────────────────────────────────────────

func (s *JiraService) authHeader() string {
	creds := s.email + ":" + s.apiToken
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(creds))
}

func (s *JiraService) apiPost(path string, payload any) (map[string]any, error) {
	b, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest(http.MethodPost, s.baseURL+path, bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", s.authHeader())
	return s.doRequest(req)
}

func (s *JiraService) apiGet(path string) (map[string]any, error) {
	req, err := http.NewRequest(http.MethodGet, s.baseURL+path, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", s.authHeader())
	req.Header.Set("Accept", "application/json")
	return s.doRequest(req)
}

func (s *JiraService) doRequest(req *http.Request) (map[string]any, error) {
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("jira API %s %d: %s", req.URL.Path, resp.StatusCode, string(body))
	}
	var result map[string]any
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("jira: decode response: %w", err)
	}
	return result, nil
}

func mapPriorityToJira(priority, severity string) string {
	if priority != "" {
		return priority
	}
	switch severity {
	case "critical":
		return "Highest"
	case "high":
		return "High"
	case "medium":
		return "Medium"
	default:
		return "Low"
	}
}

func buildIssueDescription(incident models.Incident, extra string) string {
	desc := fmt.Sprintf("Incident ID: %s\nService: %s\nSeverity: %s\nStatus: %s\nStarted: %s\n\nTitle: %s",
		incident.ID, incident.Service, incident.Severity, incident.Status,
		incident.FirstEventTime.Format(time.RFC3339), incident.Title)
	if incident.RootCauseSummary != "" {
		desc += "\n\nRoot Cause:\n" + incident.RootCauseSummary
	}
	if extra != "" {
		desc += "\n\nAdditional Notes:\n" + extra
	}
	desc += "\n\n[Auto-created by AIOps Platform]"
	return desc
}
