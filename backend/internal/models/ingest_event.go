package models

import "time"

type IngestEvent struct {
	TenantID    string            `json:"tenant_id"`
	Source      string            `json:"source"`
	ExternalID  string            `json:"external_id"`
	Service     string            `json:"service"`
	Resource    string            `json:"resource"`
	Environment string            `json:"environment"`
	Severity    string            `json:"severity"`
	SignalType  string            `json:"signal_type"`
	Title       string            `json:"title"`
	Message     string            `json:"message"`
	Labels      map[string]string `json:"labels"`
	Timestamp   time.Time         `json:"timestamp"`
}
