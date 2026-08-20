package models

import "time"

// ── Incident Commander ────────────────────────────────────────────────────────

// IncidentCommander records who owns the incident response at any given time.
// History is kept: every assignment creates a new row, the latest is "current".
type IncidentCommander struct {
	ID           string    `json:"id"`
	TenantID     string    `json:"tenant_id"`
	IncidentID   string    `json:"incident_id"`
	UserID       string    `json:"user_id"`   // internal user id or Slack/Teams ID
	UserName     string    `json:"user_name"` // display name
	UserEmail    string    `json:"user_email,omitempty"`
	AssignedBy   string    `json:"assigned_by"`
	Notes        string    `json:"notes,omitempty"`
	AssignedAt   time.Time `json:"assigned_at"`
	RelievedAt   *time.Time `json:"relieved_at,omitempty"` // nil = currently active
}

// ── Stakeholder Comms Templates ───────────────────────────────────────────────

// TemplateChannel identifies which channel a template targets.
// Values: slack | teams | email | status-page
type TemplateChannel = string

const (
	ChannelSlack      TemplateChannel = "slack"
	ChannelTeams      TemplateChannel = "teams"
	ChannelEmail      TemplateChannel = "email"
	ChannelStatusPage TemplateChannel = "status-page"
)

// StakeholderTemplate is a reusable message template with variable substitution.
// Variables use {{name}} syntax. Supported vars:
//   {{incident_id}} {{service}} {{severity}} {{status}} {{title}}
//   {{commander}} {{started_at}} {{duration_mins}} {{event_count}}
//   {{impacted_services}} {{environment}}
type StakeholderTemplate struct {
	ID          string    `json:"id"`
	TenantID    string    `json:"tenant_id"`
	Name        string    `json:"name"`
	Channel     TemplateChannel `json:"channel"`
	Subject     string    `json:"subject,omitempty"` // email subject / Teams card title
	Body        string    `json:"body"`              // message body with {{vars}}
	IsDefault   bool      `json:"is_default"`        // shown as suggestion in UI
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// RenderedComms is the result of applying a template to an incident.
type RenderedComms struct {
	TemplateID string `json:"template_id"`
	Subject    string `json:"subject"`
	Body       string `json:"body"`
	Channel    string `json:"channel"`
}

// ── Ticket Sync ───────────────────────────────────────────────────────────────

// TicketProvider identifies the external ticketing system.
// Values: jira | servicenow
type TicketProvider = string

const (
	ProviderJira        TicketProvider = "jira"
	ProviderServiceNow  TicketProvider = "servicenow"
)

// TicketSync records the external ticket linked to an incident.
type TicketSync struct {
	ID           string    `json:"id"`
	TenantID     string    `json:"tenant_id"`
	IncidentID   string    `json:"incident_id"`
	Provider     TicketProvider `json:"provider"`
	TicketKey    string    `json:"ticket_key"`  // e.g. "OPS-123" (JIRA) or "INC0012345" (SNOW)
	TicketURL    string    `json:"ticket_url"`
	TicketStatus string    `json:"ticket_status,omitempty"`
	CreatedBy    string    `json:"created_by"`
	CreatedAt    time.Time `json:"created_at"`
	SyncedAt     time.Time `json:"synced_at"`
}

// TicketCreateRequest carries the fields needed to open a ticket.
type TicketCreateRequest struct {
	IncidentID  string `json:"incident_id"`
	Provider    string `json:"provider"` // "jira" | "servicenow"
	Priority    string `json:"priority,omitempty"`
	Assignee    string `json:"assignee,omitempty"`
	Description string `json:"description,omitempty"`
}

// ── Status Communications ─────────────────────────────────────────────────────

// StatusUpdate is a customer-facing status communication attached to an incident.
type StatusUpdateStage = string

const (
	StageInvestigating StatusUpdateStage = "investigating"
	StageIdentified    StatusUpdateStage = "identified"
	StageMonitoring    StatusUpdateStage = "monitoring"
	StageResolved      StatusUpdateStage = "resolved"
)

type StatusCommunication struct {
	ID               string    `json:"id"`
	TenantID         string    `json:"tenant_id"`
	IncidentID       string    `json:"incident_id"`
	Stage            StatusUpdateStage `json:"stage"`
	Title            string    `json:"title"`
	Body             string    `json:"body"`
	AffectedServices []string  `json:"affected_services"`
	PublishedBy      string    `json:"published_by"`
	PublishedAt      time.Time `json:"published_at"`
}

// ── Executive Summary ─────────────────────────────────────────────────────────

// ExecSummary is an AI-generated executive-level incident brief.
type ExecSummary struct {
	IncidentID       string    `json:"incident_id"`
	GeneratedAt      time.Time `json:"generated_at"`
	BusinessImpact   string    `json:"business_impact"`
	TimelineNarrative string   `json:"timeline_narrative"`
	RootCause        string    `json:"root_cause"`
	Resolution       string    `json:"resolution"`
	LessonsLearned   []string  `json:"lessons_learned"`
	NextSteps        []string  `json:"next_steps"`
	RawMarkdown      string    `json:"raw_markdown"`
}
