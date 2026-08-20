package handlers

import (
	"net/http"
	"strconv"

	"ai-incident-platform/backend/internal/middleware"
	"ai-incident-platform/backend/internal/api"
	"ai-incident-platform/backend/internal/services"
)

type EngineeringHealthHandler struct {
	healthService *services.EngineeringHealthService
}

func NewEngineeringHealthHandler(hs *services.EngineeringHealthService) *EngineeringHealthHandler {
	return &EngineeringHealthHandler{healthService: hs}
}

// HandleHealth handles GET /api/v1/engineering/health
func (h *EngineeringHealthHandler) HandleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	tenantID := middleware.TenantFromRequest(r)

	days := 30
	if d := r.URL.Query().Get("days"); d != "" {
		if parsed, err := strconv.Atoi(d); err == nil && parsed > 0 {
			days = parsed
		}
	}

	summary, err := h.healthService.GetSummary(tenantID, days)
	if err != nil {
		api.WriteError(w, http.StatusInternalServerError, "failed to compute engineering health")
		return
	}

	api.WriteJSON(w, http.StatusOK, summary)
}
