package models

import "time"

// ServiceStatus is the live status of one monitored service.
type ServiceStatus struct {
	Name        string  `json:"name"`
	Status      string  `json:"status"`          // "operational", "degraded", "outage"
	UptimePct   float64 `json:"uptime_90d"`      // 0.0 – 100.0
	ActiveCount int     `json:"active_incidents"`
}

// StatusActiveIncident is a live (open/acknowledged) incident for the status page.
type StatusActiveIncident struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Service   string    `json:"service"`
	Severity  string    `json:"severity"`
	Status    string    `json:"status"`
	StartedAt time.Time `json:"started_at"`
}

// StatusHistoryEntry is a resolved incident shown in the 30-day history section.
type StatusHistoryEntry struct {
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	Service     string    `json:"service"`
	Severity    string    `json:"severity"`
	StartedAt   time.Time `json:"started_at"`
	ResolvedAt  time.Time `json:"resolved_at"`
	DurationMin int       `json:"duration_minutes"`
}

// UptimeSummary holds the three headline KPIs shown at the top of the status page.
type UptimeSummary struct {
	Uptime90d      float64 `json:"uptime_90d"`       // overall platform uptime %
	MTTRMinutes    int     `json:"mttr_minutes"`     // average resolution time
	ResolvedLast30 int     `json:"resolved_last_30"` // incidents resolved in last 30 d
}

// StatusPageFull is the complete public status page API response.
type StatusPageFull struct {
	Tenant          string                 `json:"tenant"`
	OverallStatus   string                 `json:"status"` // "operational","degraded","outage"
	Services        []ServiceStatus        `json:"services"`
	ActiveIncidents []StatusActiveIncident `json:"active_incidents"`
	History         []StatusHistoryEntry   `json:"history"`
	UptimeSummary   UptimeSummary          `json:"uptime_summary"`
	UpdatedAt       time.Time              `json:"updated_at"`
}

// StatusSubscription is one row in status_subscriptions.
type StatusSubscription struct {
	ID        string    `json:"id"`
	TenantID  string    `json:"tenant_id"`
	Channel   string    `json:"channel"` // "email" or "slack"
	Target    string    `json:"target"`
	Token     string    `json:"token"`
	Verified  bool      `json:"verified"`
	CreatedAt time.Time `json:"created_at"`
}

// SubscribeRequest is the JSON body for POST /api/v1/status/subscribe.
type SubscribeRequest struct {
	Channel  string `json:"channel"`   // "email" or "slack"
	Target   string `json:"target"`    // email address or Slack webhook URL
	TenantID string `json:"tenant_id"` // defaults to "default" if empty
}
