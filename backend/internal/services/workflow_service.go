package services

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"ai-incident-platform/backend/internal/llm"
	"ai-incident-platform/backend/internal/models"
	"ai-incident-platform/backend/internal/store"
)

// WorkflowService orchestrates all Team Workflow Primitives:
//   - Commander assignment + notifications
//   - Stakeholder template management + rendering
//   - JIRA / ServiceNow ticket creation + sync
//   - Executive summary generation
//   - Customer-facing status communications
type WorkflowService struct {
	workflowStore *store.WorkflowStore
	incidentStore *store.IncidentStore
	slackSvc      *SlackService
	teamsSvc      *TeamsService
	jiraSvc       *JiraService
	snowSvc       *ServiceNowService
	llmClient     *llm.Client
}

func NewWorkflowService(
	ws *store.WorkflowStore,
	is *store.IncidentStore,
	slack *SlackService,
	teams *TeamsService,
	jira *JiraService,
	snow *ServiceNowService,
	llmClient *llm.Client,
) *WorkflowService {
	return &WorkflowService{
		workflowStore: ws,
		incidentStore: is,
		slackSvc:      slack,
		teamsSvc:      teams,
		jiraSvc:       jira,
		snowSvc:       snow,
		llmClient:     llmClient,
	}
}

// ── Commander ─────────────────────────────────────────────────────────────────

// AssignCommander assigns a new incident commander, relieves the previous one,
// and sends Slack + Teams notifications.
func (svc *WorkflowService) AssignCommander(c *models.IncidentCommander) error {
	if c.ID == "" {
		c.ID = fmt.Sprintf("cmd-%d", time.Now().UnixNano())
	}
	if err := svc.workflowStore.AssignCommander(c); err != nil {
		return err
	}

	// Best-effort notifications — never block the assignment itself.
	incident := svc.loadIncident(c.TenantID, c.IncidentID)
	if incident != nil {
		if svc.slackSvc != nil && svc.slackSvc.IsConfigured() {
			go func() {
				if err := svc.slackSvc.PostCommanderAssigned(*incident, *c); err != nil {
					slog.Error("workflow: slack commander notify failed", "error", err)
				}
			}()
		}
		if svc.teamsSvc != nil && svc.teamsSvc.IsConfigured() {
			go func() {
				if err := svc.teamsSvc.PostCommanderAssigned(*incident, *c); err != nil {
					slog.Error("workflow: teams commander notify failed", "error", err)
				}
			}()
		}
	}
	return nil
}

func (svc *WorkflowService) CurrentCommander(tenantID, incidentID string) (*models.IncidentCommander, error) {
	return svc.workflowStore.CurrentCommander(tenantID, incidentID)
}

func (svc *WorkflowService) CommanderHistory(tenantID, incidentID string) ([]models.IncidentCommander, error) {
	return svc.workflowStore.CommanderHistory(tenantID, incidentID)
}

// ── Templates ─────────────────────────────────────────────────────────────────

func (svc *WorkflowService) CreateTemplate(t *models.StakeholderTemplate) error {
	if t.ID == "" {
		t.ID = fmt.Sprintf("tmpl-%d", time.Now().UnixNano())
	}
	return svc.workflowStore.CreateTemplate(t)
}

func (svc *WorkflowService) ListTemplates(tenantID string) ([]models.StakeholderTemplate, error) {
	return svc.workflowStore.ListTemplates(tenantID)
}

func (svc *WorkflowService) UpdateTemplate(t *models.StakeholderTemplate) error {
	return svc.workflowStore.UpdateTemplate(t)
}

func (svc *WorkflowService) DeleteTemplate(id, tenantID string) error {
	return svc.workflowStore.DeleteTemplate(id, tenantID)
}

// RenderTemplate substitutes {{variable}} tokens in a template using incident data.
// Supported variables: incident_id, service, severity, status, title, commander,
// started_at, duration_mins, event_count, impacted_services, environment.
func (svc *WorkflowService) RenderTemplate(tenantID, templateID, incidentID string) (*models.RenderedComms, error) {
	tmpl, err := svc.workflowStore.GetTemplate(templateID)
	if err != nil {
		return nil, fmt.Errorf("template not found: %w", err)
	}
	incident := svc.loadIncident(tenantID, incidentID)
	if incident == nil {
		return nil, fmt.Errorf("incident %s not found", incidentID)
	}
	commander, _ := svc.workflowStore.CurrentCommander(tenantID, incidentID)
	commanderName := ""
	if commander != nil {
		commanderName = commander.UserName
	}

	vars := buildTemplateVars(*incident, commanderName)
	body := applyTemplateVars(tmpl.Body, vars)
	subject := applyTemplateVars(tmpl.Subject, vars)

	return &models.RenderedComms{
		TemplateID: tmpl.ID,
		Subject:    subject,
		Body:       body,
		Channel:    tmpl.Channel,
	}, nil
}

// ── Ticket Sync ───────────────────────────────────────────────────────────────

// CreateTicket opens a JIRA or ServiceNow ticket for an incident.
func (svc *WorkflowService) CreateTicket(tenantID string, req models.TicketCreateRequest) (*models.TicketSync, error) {
	incident := svc.loadIncident(tenantID, req.IncidentID)
	if incident == nil {
		return nil, fmt.Errorf("incident %s not found", req.IncidentID)
	}

	var ts *models.TicketSync
	var err error

	switch req.Provider {
	case models.ProviderJira:
		if svc.jiraSvc == nil || !svc.jiraSvc.IsConfigured() {
			return nil, fmt.Errorf("JIRA not configured (set JIRA_BASE_URL, JIRA_EMAIL, JIRA_API_TOKEN, JIRA_PROJECT_KEY)")
		}
		ts, err = svc.jiraSvc.CreateIssue(*incident, req.Description, req.Priority)
	case models.ProviderServiceNow:
		if svc.snowSvc == nil || !svc.snowSvc.IsConfigured() {
			return nil, fmt.Errorf("ServiceNow not configured (set SERVICENOW_URL, SERVICENOW_USER, SERVICENOW_PASSWORD)")
		}
		ts, err = svc.snowSvc.CreateIncident(*incident, req.Description)
	default:
		return nil, fmt.Errorf("unknown provider %q: use 'jira' or 'servicenow'", req.Provider)
	}
	if err != nil {
		return nil, err
	}

	ts.ID = fmt.Sprintf("ts-%d", time.Now().UnixNano())
	ts.TenantID = tenantID
	ts.CreatedBy = req.Assignee
	if err := svc.workflowStore.SaveTicketSync(ts); err != nil {
		slog.Error("workflow: save ticket sync failed", "error", err)
	}
	return ts, nil
}

// SyncTicketStatus re-fetches the ticket status from the provider and persists it.
func (svc *WorkflowService) SyncTicketStatus(tenantID, incidentID, provider string) error {
	syncs, err := svc.workflowStore.GetTicketSyncs(tenantID, incidentID)
	if err != nil {
		return err
	}
	for _, ts := range syncs {
		if ts.Provider != provider {
			continue
		}
		var status string
		switch provider {
		case models.ProviderJira:
			if svc.jiraSvc != nil {
				status, _ = svc.jiraSvc.GetIssueStatus(ts.TicketKey)
			}
		case models.ProviderServiceNow:
			if svc.snowSvc != nil {
				status, _ = svc.snowSvc.GetIncidentState(ts.TicketKey)
			}
		}
		if status != "" {
			_ = svc.workflowStore.UpdateTicketStatus(incidentID, provider, status)
		}
		return nil
	}
	return fmt.Errorf("no %s ticket for incident %s", provider, incidentID)
}

func (svc *WorkflowService) GetTicketSyncs(tenantID, incidentID string) ([]models.TicketSync, error) {
	return svc.workflowStore.GetTicketSyncs(tenantID, incidentID)
}

// ── Executive Summary ─────────────────────────────────────────────────────────

// GenerateExecSummary uses the LLM to produce an executive summary for an incident.
func (svc *WorkflowService) GenerateExecSummary(tenantID, incidentID string) (*models.ExecSummary, error) {
	incident := svc.loadIncident(tenantID, incidentID)
	if incident == nil {
		return nil, fmt.Errorf("incident %s not found", incidentID)
	}

	commander, _ := svc.workflowStore.CurrentCommander(tenantID, incidentID)
	commanderName := "unassigned"
	if commander != nil {
		commanderName = commander.UserName
	}

	if svc.llmClient != nil && svc.llmClient.IsConfigured() {
		return svc.llmExecSummary(*incident, commanderName)
	}
	return svc.ruleBasedExecSummary(*incident, commanderName), nil
}

func (svc *WorkflowService) llmExecSummary(incident models.Incident, commanderName string) (*models.ExecSummary, error) {
	systemPrompt := `You are an SRE communication specialist. Generate a concise, executive-level incident summary.
Respond ONLY with valid JSON matching this schema:
{
  "business_impact": "string",
  "timeline_narrative": "string",
  "root_cause": "string",
  "resolution": "string",
  "lessons_learned": ["string"],
  "next_steps": ["string"],
  "raw_markdown": "string"
}`

	userPrompt := fmt.Sprintf(`Generate an executive summary for this incident:
ID: %s
Title: %s
Service: %s
Severity: %s
Status: %s
Commander: %s
Started: %s
Duration: ~%d minutes
Events: %d
Root Cause: %s
Impacted Services: %s`,
		incident.ID,
		incident.Title,
		incident.Service,
		incident.Severity,
		incident.Status,
		commanderName,
		incident.FirstEventTime.Format("2006-01-02 15:04 UTC"),
		int(time.Since(incident.FirstEventTime).Minutes()),
		incident.EventCount,
		incident.RootCauseSummary,
		strings.Join(incident.ImpactedServices, ", "),
	)

	rawText, err := svc.llmClient.CompleteWithSystem(systemPrompt, userPrompt)
	if err != nil {
		return svc.ruleBasedExecSummary(incident, commanderName), nil
	}

	jsonStr := extractJSON(rawText)
	var result struct {
		BusinessImpact    string   `json:"business_impact"`
		TimelineNarrative string   `json:"timeline_narrative"`
		RootCause         string   `json:"root_cause"`
		Resolution        string   `json:"resolution"`
		LessonsLearned    []string `json:"lessons_learned"`
		NextSteps         []string `json:"next_steps"`
		RawMarkdown       string   `json:"raw_markdown"`
	}
	if err := json.Unmarshal([]byte(jsonStr), &result); err != nil {
		return svc.ruleBasedExecSummary(incident, commanderName), nil
	}

	return &models.ExecSummary{
		IncidentID:        incident.ID,
		GeneratedAt:       time.Now().UTC(),
		BusinessImpact:    result.BusinessImpact,
		TimelineNarrative: result.TimelineNarrative,
		RootCause:         result.RootCause,
		Resolution:        result.Resolution,
		LessonsLearned:    result.LessonsLearned,
		NextSteps:         result.NextSteps,
		RawMarkdown:       result.RawMarkdown,
	}, nil
}

func (svc *WorkflowService) ruleBasedExecSummary(incident models.Incident, commanderName string) *models.ExecSummary {
	duration := int(time.Since(incident.FirstEventTime).Minutes())
	impacted := strings.Join(incident.ImpactedServices, ", ")
	if impacted == "" {
		impacted = incident.Service
	}

	sev := strings.ToUpper(incident.Severity[:1]) + incident.Severity[1:]
	businessImpact := fmt.Sprintf(
		"%s severity incident affecting %s. %d events observed over ~%d minutes.",
		sev, impacted, incident.EventCount, duration,
	)

	rootCause := incident.RootCauseSummary
	if rootCause == "" {
		rootCause = "Root cause analysis in progress."
	}

	var nextSteps []string
	if incident.Status != "resolved" {
		nextSteps = append(nextSteps, "Continue active investigation and remediation")
		nextSteps = append(nextSteps, "Update stakeholders with progress every 30 minutes")
	}
	nextSteps = append(nextSteps, "Document timeline and actions in postmortem")
	nextSteps = append(nextSteps, "Review monitoring coverage to detect this pattern earlier")

	markdown := fmt.Sprintf("## Incident %s — Executive Summary\n\n**Status:** %s\n**Commander:** %s\n**Duration:** ~%d minutes\n\n### Business Impact\n%s\n\n### Root Cause\n%s\n\n### Next Steps\n",
		incident.ID, incident.Status, commanderName, duration, businessImpact, rootCause)
	for _, s := range nextSteps {
		markdown += "- " + s + "\n"
	}

	return &models.ExecSummary{
		IncidentID:        incident.ID,
		GeneratedAt:       time.Now().UTC(),
		BusinessImpact:    businessImpact,
		TimelineNarrative: fmt.Sprintf("Incident started at %s and has been active for ~%d minutes.", incident.FirstEventTime.Format("15:04 UTC"), duration),
		RootCause:         rootCause,
		Resolution:        "Remediation in progress.",
		LessonsLearned:    []string{"Review alert thresholds for faster detection"},
		NextSteps:         nextSteps,
		RawMarkdown:       markdown,
	}
}

// ── Status Communications ─────────────────────────────────────────────────────

// PublishStatusUpdate persists a customer-facing update and notifies channels.
func (svc *WorkflowService) PublishStatusUpdate(sc *models.StatusCommunication) error {
	if sc.ID == "" {
		sc.ID = fmt.Sprintf("sc-%d", time.Now().UnixNano())
	}
	if err := svc.workflowStore.AddStatusCommunication(sc); err != nil {
		return err
	}

	// Notify via Teams (best-effort).
	if svc.teamsSvc != nil && svc.teamsSvc.IsConfigured() {
		go func() {
			if err := svc.teamsSvc.PostStatusUpdate(*sc); err != nil {
				slog.Error("workflow: teams status update failed", "error", err)
			}
		}()
	}
	return nil
}

func (svc *WorkflowService) GetStatusCommunications(tenantID, incidentID string) ([]models.StatusCommunication, error) {
	return svc.workflowStore.GetStatusCommunications(tenantID, incidentID)
}

// ── Slack bi-directional extension ───────────────────────────────────────────

// PostCommanderAssignedToSlack sends a Slack channel notification when a commander is assigned.
// Called by SlackService if it has access to WorkflowService — or invoked directly here.
func (svc *WorkflowService) NotifySlackCommanderAssigned(channelID string, incident models.Incident, c models.IncidentCommander) {
	if svc.slackSvc == nil || !svc.slackSvc.IsConfigured() {
		return
	}
	if err := svc.slackSvc.PostCommanderAssigned(incident, c); err != nil {
		slog.Error("workflow: slack commander assigned failed", "error", err)
	}
}

// ── internal helpers ──────────────────────────────────────────────────────────

func (svc *WorkflowService) loadIncident(tenantID, incidentID string) *models.Incident {
	incident, ok := svc.incidentStore.GetIncidentByID(incidentID)
	if !ok {
		return nil
	}
	if incident.TenantID != "" && incident.TenantID != tenantID {
		return nil
	}
	return &incident
}

func buildTemplateVars(incident models.Incident, commanderName string) map[string]string {
	duration := int(time.Since(incident.FirstEventTime).Minutes())
	impacted := strings.Join(incident.ImpactedServices, ", ")
	if impacted == "" {
		impacted = incident.Service
	}
	return map[string]string{
		"incident_id":       incident.ID,
		"service":           incident.Service,
		"severity":          incident.Severity,
		"status":            incident.Status,
		"title":             incident.Title,
		"commander":         commanderName,
		"started_at":        incident.FirstEventTime.Format("2006-01-02 15:04 UTC"),
		"duration_mins":     fmt.Sprintf("%d", duration),
		"event_count":       fmt.Sprintf("%d", incident.EventCount),
		"impacted_services": impacted,
		"environment":       "production",
	}
}

func applyTemplateVars(tmpl string, vars map[string]string) string {
	result := tmpl
	for k, v := range vars {
		result = strings.ReplaceAll(result, "{{"+k+"}}", v)
	}
	return result
}
