package models

type SourceConnection struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Type        string `json:"type"`
	Token       string `json:"token"`
	Endpoint    string `json:"endpoint"`
	Status      string `json:"status"`
	CreatedAt   string `json:"created_at"`
	LastEventAt string `json:"last_event_at"`
	LastError   string `json:"last_error"`
	TotalEvents int    `json:"total_events"`
}
