package models

type Action struct {
	ID               string `json:"id"`
	Label            string `json:"label"`
	Description      string `json:"description"`
	Type             string `json:"type"`
	Severity         string `json:"severity"`
	RiskLevel        string `json:"risk_level"`
	RequiresApproval bool   `json:"requires_approval"`
}

type ActionExecutionResult struct {
	ActionID   string `json:"action_id"`
	IncidentID string `json:"incident_id"`
	Status     string `json:"status"`
	Message    string `json:"message"`
}