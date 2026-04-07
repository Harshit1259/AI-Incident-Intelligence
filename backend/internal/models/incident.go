package models

type Incident struct {
	ID              string   `json:"id"`
	Service         string   `json:"service"`
	Severity        string   `json:"severity"`
	Status          string   `json:"status"`
	EventIDs        []string `json:"event_ids"`
	FirstEventTime  string   `json:"first_event_time"`
	LastEventTime   string   `json:"last_event_time"`
	Title           string   `json:"title"`

	CorrelationPattern string `json:"correlation_pattern"`
	CorrelationScore   int    `json:"correlation_score"`
	CorrelationReason  string `json:"correlation_reason"`

	Confidence       int      `json:"confidence"`
	RiskScore        int      `json:"risk_score"`
	EventCount       int      `json:"event_count"`
	RootCauseSummary string   `json:"root_cause_summary"`
	RootCauseType    string   `json:"root_cause_type"`
	Reasoning        []string `json:"reasoning"`

	WhatChangedType        string `json:"what_changed_type"`
	WhatChangedService     string `json:"what_changed_service"`
	WhatChangedVersion     string `json:"what_changed_version"`
	WhatChangedDescription string `json:"what_changed_description"`
	WhatChangedTimestamp   string `json:"what_changed_timestamp"`

	ImpactedServices []string `json:"impacted_services"`
	ImpactCount      int      `json:"impact_count"`

	SeenBefore        bool   `json:"seen_before"`
	RecurringCount    int    `json:"recurring_count"`
	SimilarIncidentID string `json:"similar_incident_id"`
	LastSeenAt        string `json:"last_seen_at"`
}
