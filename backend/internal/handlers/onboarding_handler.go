package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"ai-incident-platform/backend/internal/api"
	"ai-incident-platform/backend/internal/middleware"
	"ai-incident-platform/backend/internal/models"
	"ai-incident-platform/backend/internal/services"
)

// onboardingEventProcessor is the minimal interface needed to inject a synthetic
// test event into the correlation pipeline from the onboarding wizard.
type onboardingEventProcessor interface {
	ProcessEvent(event models.Event) bool
}

// OnboardingHandler exposes onboarding progress endpoints.
type OnboardingHandler struct {
	onboardingService *services.OnboardingService
	eventProc         onboardingEventProcessor
}

func NewOnboardingHandler(os *services.OnboardingService) *OnboardingHandler {
	return &OnboardingHandler{onboardingService: os}
}

// SetEventProcessor wires in the correlation service so the wizard's
// "Send Test Alert" button can fire a real event through the full pipeline.
func (h *OnboardingHandler) SetEventProcessor(ep onboardingEventProcessor) {
	h.eventProc = ep
}

// HandleProgress handles GET /api/v1/onboarding/progress.
func (h *OnboardingHandler) HandleProgress(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		api.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	claims, ok := middleware.ClaimsFromContext(r)
	if !ok {
		api.WriteError(w, http.StatusUnauthorized, "no auth claims")
		return
	}

	progress, err := h.onboardingService.GetProgress(claims.TenantID)
	if err != nil {
		api.WriteError(w, http.StatusInternalServerError, "failed to get onboarding progress")
		return
	}

	api.WriteJSON(w, http.StatusOK, progress)
}

// HandleStep handles POST /api/v1/onboarding/step.
func (h *OnboardingHandler) HandleStep(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		api.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	claims, ok := middleware.ClaimsFromContext(r)
	if !ok {
		api.WriteError(w, http.StatusUnauthorized, "no auth claims")
		return
	}

	var req models.OnboardingStepUpdate
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		api.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Step == "" {
		api.WriteError(w, http.StatusBadRequest, "step is required")
		return
	}

	if err := h.onboardingService.AdvanceStep(claims.TenantID, req.Step); err != nil {
		api.WriteError(w, http.StatusInternalServerError, "failed to advance step")
		return
	}

	api.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// HandleWizardConfig handles GET /api/v1/onboarding/wizard-config.
// Returns all webhook URLs, the agent install command, and current progress
// in a single call so the wizard can render without multiple round-trips.
func (h *OnboardingHandler) HandleWizardConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		api.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	claims, ok := middleware.ClaimsFromContext(r)
	if !ok {
		api.WriteError(w, http.StatusUnauthorized, "no auth claims")
		return
	}

	progress, err := h.onboardingService.GetProgress(claims.TenantID)
	if err != nil {
		api.WriteError(w, http.StatusInternalServerError, "failed to get onboarding progress")
		return
	}

	scheme := "http"
	if proto := r.Header.Get("X-Forwarded-Proto"); proto == "https" {
		scheme = "https"
	}
	baseURL := scheme + "://" + r.Host

	cfg := &models.WizardConfig{
		TenantID:      claims.TenantID,
		BaseURL:       baseURL,
		PrometheusURL: baseURL + "/api/v1/ingest/prometheus",
		OTelURL:       baseURL + "/api/v1/otel",
		AgentInstallCmd: fmt.Sprintf(
			"curl -fsSL %s/install.sh | TENANT_ID=%s SERVER_URL=%s sh",
			baseURL, claims.TenantID, baseURL,
		),
		Progress: progress,
	}

	api.WriteJSON(w, http.StatusOK, cfg)
}

// HandleSendTest handles POST /api/v1/onboarding/send-test.
// Injects a synthetic high-severity event through the full correlation pipeline
// so the new user sees their first incident without needing an external tool.
func (h *OnboardingHandler) HandleSendTest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		api.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	claims, ok := middleware.ClaimsFromContext(r)
	if !ok {
		api.WriteError(w, http.StatusUnauthorized, "no auth claims")
		return
	}

	if h.eventProc == nil {
		api.WriteError(w, http.StatusServiceUnavailable, "event processor not configured")
		return
	}

	now := time.Now()
	event := models.Event{
		ID:          fmt.Sprintf("test-%d", now.UnixNano()),
		TenantID:    claims.TenantID,
		Source:      "onboarding-wizard",
		ExternalID:  fmt.Sprintf("test-evt-%d", now.UnixNano()),
		Service:     "checkout-api",
		Resource:    "checkout-api",
		Environment: "production",
		Severity:    "high",
		Type:        "error_rate",
		Title:       "[Test] High error rate on checkout-api",
		Message:     "Test event from NeuroOps onboarding wizard. Error rate exceeded 5% on checkout-api (23 errors in 5 min).",
		Labels:      map[string]string{"source": "onboarding", "synthetic": "true"},
		Timestamp:   now,
		IngestSchema: "webhook",
	}

	go func() { h.eventProc.ProcessEvent(event) }()

	_ = h.onboardingService.AdvanceStep(claims.TenantID, "first_alert")

	api.WriteJSON(w, http.StatusOK, map[string]string{
		"status":  "sent",
		"message": "Test alert sent — your first incident should appear within a few seconds.",
	})
}

// HandleMilestone handles POST /api/v1/onboarding/milestone.
func (h *OnboardingHandler) HandleMilestone(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		api.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	claims, ok := middleware.ClaimsFromContext(r)
	if !ok {
		api.WriteError(w, http.StatusUnauthorized, "no auth claims")
		return
	}

	var body struct {
		Milestone string `json:"milestone"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		api.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if body.Milestone == "" {
		api.WriteError(w, http.StatusBadRequest, "milestone is required")
		return
	}

	if err := h.onboardingService.MarkMilestone(claims.TenantID, body.Milestone); err != nil {
		api.WriteError(w, http.StatusInternalServerError, "failed to mark milestone")
		return
	}

	api.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
