package handlers

import (
	"encoding/json"
	"net/http"

	"ai-incident-platform/backend/internal/middleware"
	"ai-incident-platform/backend/internal/services"
)

// RiskExposureHandler serves the live business risk / blast-radius dashboard.
type RiskExposureHandler struct {
	riskService *services.RiskExposureService
}

func NewRiskExposureHandler(rs *services.RiskExposureService) *RiskExposureHandler {
	return &RiskExposureHandler{riskService: rs}
}

// HandleGetExposure handles GET /api/v1/risk/exposure
func (h *RiskExposureHandler) HandleGetExposure(w http.ResponseWriter, r *http.Request) {
	claims, ok := middleware.ClaimsFromContext(r)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	dashboard, err := h.riskService.GetExposureDashboard(claims.TenantID)
	if err != nil {
		http.Error(w, "failed to compute risk exposure: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(dashboard)
}

// HandleGetAtRiskServices handles GET /api/v1/risk/at-risk-services
func (h *RiskExposureHandler) HandleGetAtRiskServices(w http.ResponseWriter, r *http.Request) {
	claims, ok := middleware.ClaimsFromContext(r)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	dashboard, err := h.riskService.GetExposureDashboard(claims.TenantID)
	if err != nil {
		http.Error(w, "failed to compute risk exposure: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"at_risk_services": dashboard.AtRiskServices,
		"computed_at":      dashboard.ComputedAt,
	})
}
