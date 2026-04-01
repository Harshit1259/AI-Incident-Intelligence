package models

type IncidentSummary struct {
	EventCount      int    `json:"event_count"`
	Service         string `json:"service"`
	Severity        string `json:"severity"`
	LatestEventTime string `json:"latest_event_time"`
}

type IncidentDetail struct {
	Incident Incident        `json:"incident"`
	Events   []Event         `json:"events"`
	Summary  IncidentSummary `json:"summary"`
	Insight  IncidentInsight `json:"insight"`
}
