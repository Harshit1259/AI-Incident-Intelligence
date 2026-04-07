package models

type ActionAudit struct {
	ID         string `json:"id"`
	ActionID   string `json:"action_id"`
	IncidentID string `json:"incident_id"`
	Status     string `json:"status"`
	Message    string `json:"message"`
	ExecutedAt string `json:"executed_at"`
	ExecutedBy string `json:"executed_by"`
}
