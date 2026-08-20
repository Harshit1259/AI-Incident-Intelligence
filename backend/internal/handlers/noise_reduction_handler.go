package handlers

import (
	"net/http"
	"strconv"

	"ai-incident-platform/backend/internal/api"
	"ai-incident-platform/backend/internal/middleware"
	"ai-incident-platform/backend/internal/services"
)

// NoiseReductionHandler serves GET /api/v1/noise-reduction — the first-screen
// dashboard that proves noise reduction value in real numbers.
type NoiseReductionHandler struct {
	svc *services.NoiseReductionService
}

func NewNoiseReductionHandler(svc *services.NoiseReductionService) *NoiseReductionHandler {
	return &NoiseReductionHandler{svc: svc}
}

// HandleGet handles GET /api/v1/noise-reduction?days=30
func (h *NoiseReductionHandler) HandleGet(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		api.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	claims, ok := middleware.ClaimsFromContext(r)
	if !ok {
		api.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	days := 30
	if q := r.URL.Query().Get("days"); q != "" {
		if n, err := strconv.Atoi(q); err == nil && n > 0 && n <= 90 {
			days = n
		}
	}

	score, err := h.svc.ComputeScore(claims.TenantID, days)
	if err != nil {
		api.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	api.WriteJSON(w, http.StatusOK, score)
}
