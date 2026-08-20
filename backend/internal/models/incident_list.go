package models

import "time"

type IncidentListFilter struct {
	TenantID  string // required for tenant-scoped queries; defaults to "default"
	Status    string
	Severity  string
	Service   string
	Search    string
	From      *time.Time
	To        *time.Time
	Page      int
	PageSize  int
	SortBy    string
	SortOrder string
}

type IncidentListItem struct {
	ID               string    `json:"id"`
	Service          string    `json:"service"`
	Severity         string    `json:"severity"`
	Status           string    `json:"status"`
	FirstEventTime   time.Time `json:"first_event_time"`
	LastEventTime    time.Time `json:"last_event_time"`
	Title            string    `json:"title"`
	EventCount       int       `json:"event_count"`
	Confidence       int       `json:"confidence"`
	RiskScore        int       `json:"risk_score"`
	ImpactCount      int       `json:"impact_count"`
	RootCauseSummary string    `json:"root_cause_summary"`
	WhatChangedType  string    `json:"what_changed_type"`
	HasWhatChanged   bool      `json:"has_what_changed"`

	SeenBefore        bool   `json:"seen_before"`
	RecurringCount    int    `json:"recurring_count"`
	SimilarIncidentID string `json:"similar_incident_id"`
	PriorityScore     int    `json:"priority_score"`
}

type IncidentListResponse struct {
	Items    []IncidentListItem `json:"items"`
	Page     int                `json:"page"`
	PageSize int                `json:"page_size"`
	Total    int                `json:"total"`
	HasMore  bool               `json:"has_more"`
}
