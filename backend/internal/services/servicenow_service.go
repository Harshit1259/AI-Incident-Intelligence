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

// ServiceNowService creates and syncs ServiceNow incident records via the Table API.
// Auth: Basic auth (user + password) or OAuth — Basic used here for simplicity.
type ServiceNowService struct {
	instanceURL string // e.g. "https://your-instance.service-now.com"
	username    string
	password    string
	httpClient  *http.Client
}

func NewServiceNowService(instanceURL, username, password string) *ServiceNowService {
	return &ServiceNowService{
		instanceURL: instanceURL,
		username:    username,
		password:    password,
		httpClient:  &http.Client{Timeout: 15 * time.Second},
	}
}

func (s *ServiceNowService) IsConfigured() bool {
	return s.instanceURL != "" && s.username != "" && s.password != ""
}

// CreateIncident creates a ServiceNow incident and returns ticket metadata.
func (s *ServiceNowService) CreateIncident(incident models.Incident, description string) (*models.TicketSync, error) {
	if !s.IsConfigured() {
		return nil, fmt.Errorf("servicenow: not configured")
	}

	urgency, impact := snowUrgencyImpact(incident.Severity)

	body := map[string]any{
		"short_description": fmt.Sprintf("[%s] %s — %s", incident.Severity, incident.Service, incident.Title),
		"description":       buildSNOWDescription(incident, description),
		"urgency":           urgency,
		"impact":            impact,
		"category":          "software",
		"caller_id":         "aiops-platform",
	}

	resp, err := s.apiPost("/api/now/table/incident", body)
	if err != nil {
		return nil, fmt.Errorf("servicenow create incident: %w", err)
	}

	result, _ := resp["result"].(map[string]any)
	if result == nil {
		return nil, fmt.Errorf("servicenow: no result in response")
	}
	number, _ := result["number"].(string)
	sysID, _ := result["sys_id"].(string)

	return &models.TicketSync{
		Provider:     models.ProviderServiceNow,
		IncidentID:   incident.ID,
		TicketKey:    number,
		TicketURL:    fmt.Sprintf("%s/incident.do?sys_id=%s", s.instanceURL, sysID),
		TicketStatus: "new",
	}, nil
}

// GetIncidentState fetches the current state label of a ServiceNow incident by number.
func (s *ServiceNowService) GetIncidentState(ticketNumber string) (string, error) {
	if !s.IsConfigured() {
		return "", fmt.Errorf("servicenow: not configured")
	}
	resp, err := s.apiGet(fmt.Sprintf(
		"/api/now/table/incident?sysparm_query=number=%s&sysparm_fields=state,sys_id&sysparm_limit=1",
		ticketNumber,
	))
	if err != nil {
		return "", err
	}
	results, _ := resp["result"].([]any)
	if len(results) == 0 {
		return "", nil
	}
	rec, _ := results[0].(map[string]any)
	state, _ := rec["state"].(string)
	return snowStateLabel(state), nil
}

// ── internal ──────────────────────────────────────────────────────────────────

func (s *ServiceNowService) apiPost(path string, payload any) (map[string]any, error) {
	b, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest(http.MethodPost, s.instanceURL+path, bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.SetBasicAuth(s.username, s.password)
	return s.doRequest(req)
}

func (s *ServiceNowService) apiGet(path string) (map[string]any, error) {
	req, err := http.NewRequest(http.MethodGet, s.instanceURL+path, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.SetBasicAuth(s.username, s.password)
	return s.doRequest(req)
}

func (s *ServiceNowService) doRequest(req *http.Request) (map[string]any, error) {
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("servicenow API %s %d: %s", req.URL.Path, resp.StatusCode, string(body))
	}
	var result map[string]any
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("servicenow: decode response: %w", err)
	}
	return result, nil
}

// snowUrgencyImpact maps our severity to ServiceNow urgency+impact (1=High, 3=Low).
func snowUrgencyImpact(severity string) (urgency, impact string) {
	switch severity {
	case "critical":
		return "1", "1"
	case "high":
		return "2", "2"
	case "medium":
		return "2", "3"
	default:
		return "3", "3"
	}
}

func snowStateLabel(state string) string {
	switch state {
	case "1":
		return "new"
	case "2":
		return "in_progress"
	case "3":
		return "on_hold"
	case "6":
		return "resolved"
	case "7":
		return "closed"
	default:
		return state
	}
}

func buildSNOWDescription(incident models.Incident, extra string) string {
	desc := fmt.Sprintf("Incident ID: %s\nService: %s\nSeverity: %s\nStarted: %s\n\n%s",
		incident.ID, incident.Service, incident.Severity,
		incident.FirstEventTime.Format(time.RFC3339), incident.Title)
	if incident.RootCauseSummary != "" {
		desc += "\n\nRoot Cause Summary:\n" + incident.RootCauseSummary
	}
	if extra != "" {
		desc += "\n\nNotes:\n" + extra
	}
	return desc
}
