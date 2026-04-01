package models

type Event struct {
	ID        string `json:"id"`
	Source    string `json:"source"`
	Type      string `json:"type"`
	Service   string `json:"service"`
	Severity  string `json:"severity"`
	Message   string `json:"message"`
	Timestamp string `json:"timestamp"`
}
