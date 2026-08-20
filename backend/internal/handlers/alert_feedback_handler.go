package handlers

import (
	"encoding/json"
	"net/http"

	"ai-incident-platform/backend/internal/middleware"
	"ai-incident-platform/backend/internal/api"
	"ai-incident-platform/backend/internal/models"
	"ai-incident-platform/backend/internal/services"
)

type AlertFeedbackHandler struct {
	svc *services.AlertFeedbackService
}

func NewAlertFeedbackHandler(svc *services.AlertFeedbackService) *AlertFeedbackHandler {
	return &AlertFeedbackHandler{svc: svc}
}

// HandleSubmit handles POST /api/v1/alerts/feedback
func (h *AlertFeedbackHandler) HandleSubmit(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req models.AlertFeedbackRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		api.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Feedback == "" {
		api.WriteError(w, http.StatusBadRequest, "feedback field is required")
		return
	}

	tenantID := middleware.TenantFromRequest(r)
	userID := r.Header.Get("X-User-ID")

	if err := h.svc.SubmitFeedback(req, tenantID, userID); err != nil {
		api.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	api.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// HandleStats handles GET /api/v1/alerts/feedback/stats
func (h *AlertFeedbackHandler) HandleStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	tenantID := middleware.TenantFromRequest(r)

	stats, err := h.svc.GetStats(tenantID)
	if err != nil {
		api.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	api.WriteJSON(w, http.StatusOK, stats)
}

// HandleSourceQuality handles GET /api/v1/alerts/feedback/source-quality
func (h *AlertFeedbackHandler) HandleSourceQuality(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	tenantID := middleware.TenantFromRequest(r)

	stats, err := h.svc.GetSourceQuality(tenantID)
	if err != nil {
		api.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if stats == nil {
		stats = []models.SourceQualityStat{}
	}

	api.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"tenant_id": tenantID,
		"sources":   stats,
	})
}

// HandleSuppressed handles GET /api/v1/alerts/suppressed
func (h *AlertFeedbackHandler) HandleSuppressed(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	tenantID := middleware.TenantFromRequest(r)

	stats, err := h.svc.GetStats(tenantID)
	if err != nil {
		api.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Return only the suppressed fingerprints from the noisy alerts that have >= 5 noise feedbacks
	var suppressed []models.NoisyAlert
	for _, n := range stats.TopNoisy {
		if n.NoiseCount >= 5 {
			suppressed = append(suppressed, n)
		}
	}

	api.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"suppressed_count":        stats.SuppressedCount,
		"suppressed_fingerprints": suppressed,
	})
}
