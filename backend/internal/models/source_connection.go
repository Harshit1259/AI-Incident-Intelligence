package models

import "time"

type SourceConnection struct {
	ID          string     `json:"id"`
	TenantID    string     `json:"tenant_id"`
	Name        string     `json:"name"`
	Type        string     `json:"type"`
	Token       string     `json:"token,omitempty"` // returned only on create; omitted in list responses
	Endpoint    string     `json:"endpoint"`
	Status      string     `json:"status"`
	CreatedAt   time.Time  `json:"created_at"`
	LastEventAt *time.Time `json:"last_event_at"`
	LastError   string     `json:"last_error"`
	LastErrorAt *time.Time `json:"last_error_at"`
	ErrorCount  int        `json:"error_count"`
	TotalEvents int        `json:"total_events"`
	// HealthScore is derived at read-time: "healthy", "degraded", "error".
	HealthScore string `json:"health_score,omitempty"`
}
