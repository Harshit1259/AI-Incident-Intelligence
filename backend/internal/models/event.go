package models

import "time"

type Event struct {
	ID          string            `json:"id"`
	Source      string            `json:"source"`
	ExternalID  string            `json:"external_id"`
	Service     string            `json:"service"`
	Resource    string            `json:"resource"`
	Environment string            `json:"environment"`
	Severity    string            `json:"severity"`
	Type        string            `json:"type"`
	Title       string            `json:"title"`
	Message     string            `json:"message"`
	Labels      map[string]string `json:"labels"`
	Timestamp   time.Time         `json:"timestamp"`
}
