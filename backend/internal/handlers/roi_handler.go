package handlers

import (
	"net/http"
	"strconv"

	"ai-incident-platform/backend/internal/middleware"
	"ai-incident-platform/backend/internal/api"
	"ai-incident-platform/backend/internal/services"
)

type ROIHandler struct {
	roiService *services.ROIService
}

func NewROIHandler(rs *services.ROIService) *ROIHandler {
	return &ROIHandler{roiService: rs}
}

// HandleROI handles GET /api/v1/roi
func (h *ROIHandler) HandleROI(w http.ResponseWriter, r *http.Request) {
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

	dashboard, err := h.roiService.GetROI(tenantID, days)
	if err != nil {
		api.WriteError(w, http.StatusInternalServerError, "failed to compute ROI dashboard")
		return
	}

	api.WriteJSON(w, http.StatusOK, dashboard)
}
