package models

import "time"

// Incident is the core incident record.
// All timestamp fields use time.Time; the API layer serialises them as RFC3339 via encoding/json.
type Incident struct {
	ID             string   `json:"id"`
	Service        string   `json:"service"`
	Severity       string   `json:"severity"`
	Status         string   `json:"status"`
	EventIDs       []string `json:"event_ids"`
	FirstEventTime time.Time `json:"first_event_time"`
	LastEventTime  time.Time `json:"last_event_time"`
	Title          string   `json:"title"`

	CorrelationPattern string `json:"correlation_pattern,omitempty"`
	CorrelationScore   int    `json:"correlation_score"`
	CorrelationReason  string `json:"correlation_reason,omitempty"`

	Confidence       int      `json:"confidence"`
	RiskScore        int      `json:"risk_score"`
	EventCount       int      `json:"event_count"`
	RootCauseSummary string   `json:"root_cause_summary,omitempty"`
	RootCauseType    string   `json:"root_cause_type,omitempty"`
	Reasoning        []string `json:"reasoning"`

	WhatChangedType        string     `json:"what_changed_type,omitempty"`
	WhatChangedService     string     `json:"what_changed_service,omitempty"`
	WhatChangedVersion     string     `json:"what_changed_version,omitempty"`
	WhatChangedDescription string     `json:"what_changed_description,omitempty"`
	WhatChangedTimestamp   *time.Time `json:"what_changed_timestamp,omitempty"`
	WhatChangedConfidence  int        `json:"what_changed_confidence,omitempty"`

	ImpactedServices []string `json:"impacted_services"`
	ImpactCount      int      `json:"impact_count"`

	SeenBefore        bool       `json:"seen_before"`
	RecurringCount    int        `json:"recurring_count"`
	SimilarIncidentID string     `json:"similar_incident_id,omitempty"`
	LastSeenAt        *time.Time `json:"last_seen_at,omitempty"`

	Fingerprint string `json:"fingerprint,omitempty"`
	TenantID    string `json:"tenant_id"`

	ParentIncidentID  string   `json:"parent_incident_id,omitempty"`
	MergedIncidentIDs []string `json:"merged_incident_ids"`
	IsMerged          bool     `json:"is_merged"`

	PriorityScore int `json:"priority_score"`
}
