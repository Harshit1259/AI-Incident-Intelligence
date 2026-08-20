package models

import "time"

// Agent represents a registered NeuroOps agent.
type Agent struct {
	ID           string            `json:"id"`
	TenantID     string            `json:"tenant_id"`
	Name         string            `json:"name"`
	HostIP       string            `json:"host_ip"`
	OSType       string            `json:"os_type"`
	Version      string            `json:"version"`
	Status       string            `json:"status"` // active, inactive, error, stopped
	LastSeenAt   time.Time         `json:"last_seen_at"`
	RegisteredAt time.Time         `json:"registered_at"`
	Metadata     map[string]string `json:"metadata,omitempty"`
}

// AgentEvent is the universal envelope for all NeuroOps agent telemetry.
type AgentEvent struct {
	EventType  string                 `json:"event.type"`
	AgentID    string                 `json:"agent.id"`
	Timestamp  float64                `json:"timestamp"`
	ObjectType string                 `json:"object.type"`
	ObjectIP   string                 `json:"object.ip"`
	Extra      map[string]interface{} `json:"-"`
}

// AgentEventBatch is what the HTTP endpoint accepts.
type AgentEventBatch struct {
	Events []map[string]interface{} `json:"events"`
}

// AgentRegistration is the request body for agent registration.
type AgentRegistration struct {
	AgentID  string `json:"agent_id"`
	TenantID string `json:"tenant_id"` // informational only; authoritative tenant comes from enrollment token
	Name     string `json:"name"`
	HostIP   string `json:"host_ip"`
	OSType   string `json:"os_type"`
	Version  string `json:"version"`
}

// AgentRegistrationResponse is returned once on successful registration.
// AgentSecret is the raw HMAC signing secret — shown exactly once, never stored in plaintext.
type AgentRegistrationResponse struct {
	Status      string `json:"status"`
	AgentID     string `json:"agent_id"`
	AgentSecret string `json:"agent_secret"` // store this securely; it cannot be recovered
}

// EnrollmentToken is a single-use, time-limited bootstrap credential an admin
// creates so an agent can call /agents/register exactly once.
type EnrollmentToken struct {
	ID            string     `json:"id"`
	TenantID      string     `json:"tenant_id"`
	Label         string     `json:"label"`
	ExpiresAt     time.Time  `json:"expires_at"`
	UsedAt        *time.Time `json:"used_at,omitempty"`
	UsedByAgentID string     `json:"used_by_agent_id,omitempty"`
	CreatedBy     string     `json:"created_by"`
	CreatedAt     time.Time  `json:"created_at"`
	// RawToken is populated only on creation and never persisted.
	RawToken string `json:"token,omitempty"`
}

// CreateEnrollmentTokenRequest is the request body for creating an enrollment token.
type CreateEnrollmentTokenRequest struct {
	Label    string `json:"label"`
	TTLHours int    `json:"ttl_hours"` // default 24
}
