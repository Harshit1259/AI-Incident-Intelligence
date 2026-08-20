package models

import "time"

type PostMortem struct {
	ID                string    `json:"id"`
	IncidentID        string    `json:"incident_id"`
	TenantID          string    `json:"tenant_id"`
	Title             string    `json:"title"`
	Status            string    `json:"status"` // draft, review, published
	ExecutiveSummary  string    `json:"executive_summary"`
	TimelineNarrative string    `json:"timeline_narrative"`
	RootCause         string    `json:"root_cause"`
	Impact            string    `json:"impact"`
	Resolution        string    `json:"resolution"`
	ActionItems       []string  `json:"action_items"`
	Lessons           string    `json:"lessons"`
	GeneratedBy       string    `json:"generated_by"` // llm or manual
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

type PostMortemUpdateRequest struct {
	Title             string   `json:"title"`
	Status            string   `json:"status"`
	ExecutiveSummary  string   `json:"executive_summary"`
	TimelineNarrative string   `json:"timeline_narrative"`
	RootCause         string   `json:"root_cause"`
	Impact            string   `json:"impact"`
	Resolution        string   `json:"resolution"`
	ActionItems       []string `json:"action_items"`
	Lessons           string   `json:"lessons"`
}
