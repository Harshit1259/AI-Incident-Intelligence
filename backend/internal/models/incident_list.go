package models

import "time"

type IncidentListFilter struct {
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
	ID               string `json:"id"`
	Service          string `json:"service"`
	Severity         string `json:"severity"`
	Status           string `json:"status"`
	FirstEventTime   string `json:"first_event_time"`
	LastEventTime    string `json:"last_event_time"`
	Title            string `json:"title"`
	EventCount       int    `json:"event_count"`
	Confidence       int    `json:"confidence"`
	RiskScore        int    `json:"risk_score"`
	ImpactCount      int    `json:"impact_count"`
	RootCauseSummary string `json:"root_cause_summary"`
	WhatChangedType  string `json:"what_changed_type"`
	HasWhatChanged   bool   `json:"has_what_changed"`

	SeenBefore        bool   `json:"seen_before"`
	RecurringCount    int    `json:"recurring_count"`
	SimilarIncidentID string `json:"similar_incident_id"`
}

type IncidentListResponse struct {
	Items    []IncidentListItem `json:"items"`
	Page     int                `json:"page"`
	PageSize int                `json:"page_size"`
	Total    int                `json:"total"`
	HasMore  bool               `json:"has_more"`
}
