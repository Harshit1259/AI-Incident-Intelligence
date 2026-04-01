package models

type Incident struct {
	ID             string   `json:"id"`
	Service        string   `json:"service"`
	Severity       string   `json:"severity"`
	Status         string   `json:"status"`
	EventIDs       []string `json:"event_ids"`
	FirstEventTime string   `json:"first_event_time"`
	LastEventTime  string   `json:"last_event_time"`
	Title          string   `json:"title"`
}
