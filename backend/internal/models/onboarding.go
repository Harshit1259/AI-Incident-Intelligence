package models

import "time"

// OnboardingProgress tracks a tenant's onboarding state.
type OnboardingProgress struct {
	ID                   string    `json:"id"`
	TenantID             string    `json:"tenant_id"`
	Step                 string    `json:"step"` // signup, connect_source, first_alert, first_rca, explore, complete
	CompletedSteps       []string  `json:"completed_steps"`
	FirstSourceConnected bool      `json:"first_source_connected"`
	FirstIncidentCreated bool      `json:"first_incident_created"`
	FirstRCAGenerated    bool      `json:"first_rca_generated"`
	AhaMomentReached     bool      `json:"aha_moment_reached"`
	CreatedAt            time.Time `json:"created_at"`
	UpdatedAt            time.Time `json:"updated_at"`
}

// OnboardingStepUpdate is the JSON body for POST /api/v1/onboarding/step.
type OnboardingStepUpdate struct {
	Step string `json:"step"`
}

// WizardConfig is returned by GET /api/v1/onboarding/wizard-config.
// Bundles all URLs, one-click commands, and current progress for the setup wizard.
type WizardConfig struct {
	TenantID        string              `json:"tenant_id"`
	BaseURL         string              `json:"base_url"`
	WebhookURL      string              `json:"webhook_url"`
	PrometheusURL   string              `json:"prometheus_url"`
	GitHubURL       string              `json:"github_url"`
	AgentInstallCmd string              `json:"agent_install_cmd"`
	Progress        *OnboardingProgress `json:"progress"`
}
