package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"ai-incident-platform/backend/internal/middleware"
	"ai-incident-platform/backend/internal/api"
	"ai-incident-platform/backend/internal/models"
	"ai-incident-platform/backend/internal/services"
)

type BusinessImpactHandler struct {
	service *services.BusinessImpactService
}

func NewBusinessImpactHandler(s *services.BusinessImpactService) *BusinessImpactHandler {
	return &BusinessImpactHandler{service: s}
}

// ── Incident-level endpoints ─────────────────────────────────────────────────

// HandleGet handles GET /api/v1/incidents/{id}/business-impact
func (h *BusinessImpactHandler) HandleGet(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	incidentID := extractIncidentIDFromBizPath(r.URL.Path)
	if incidentID == "" {
		api.WriteError(w, http.StatusBadRequest, "missing incident ID")
		return
	}

	resp, err := h.service.GetOrCalculate(incidentID)
	if err != nil {
		api.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	api.WriteJSON(w, http.StatusOK, resp)
}

// HandleGenerate handles POST /api/v1/incidents/{id}/business-impact/generate
func (h *BusinessImpactHandler) HandleGenerate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	incidentID := extractIncidentIDFromBizPath(r.URL.Path)
	if incidentID == "" {
		api.WriteError(w, http.StatusBadRequest, "missing incident ID")
		return
	}

	resp, err := h.service.CalculateImpact(incidentID)
	if err != nil {
		api.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	api.WriteJSON(w, http.StatusOK, resp)
}

// ── Service profile endpoints ────────────────────────────────────────────────

// HandleProfiles handles GET and POST for /api/v1/business/profiles
func (h *BusinessImpactHandler) HandleProfiles(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		tenantID := middleware.TenantFromRequest(r)
		profiles, err := h.service.GetProfiles(tenantID)
		if err != nil {
			api.WriteError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if profiles == nil {
			profiles = []models.ServiceProfile{}
		}
		api.WriteJSON(w, http.StatusOK, profiles)

	case http.MethodPost:
		var profile models.ServiceProfile
		if err := json.NewDecoder(r.Body).Decode(&profile); err != nil {
			api.WriteError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
			return
		}
		if profile.ID == "" {
			api.WriteError(w, http.StatusBadRequest, "id is required")
			return
		}
		if profile.Service == "" {
			api.WriteError(w, http.StatusBadRequest, "service is required")
			return
		}
		if profile.TenantID == "" {
			profile.TenantID = middleware.TenantFromRequest(r)
		}
		if err := h.service.SaveProfile(profile); err != nil {
			api.WriteError(w, http.StatusInternalServerError, err.Error())
			return
		}
		api.WriteJSON(w, http.StatusOK, profile)

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// HandleDeleteProfile handles DELETE /api/v1/business/profiles/{id}
func (h *BusinessImpactHandler) HandleDeleteProfile(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	id := extractLastPathSegment(r.URL.Path)
	if id == "" {
		api.WriteError(w, http.StatusBadRequest, "missing profile ID")
		return
	}

	if err := h.service.DeleteProfile(id); err != nil {
		api.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	api.WriteJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// ── Baseline endpoints ───────────────────────────────────────────────────────

// HandleBaselines handles GET and POST for /api/v1/business/baselines
func (h *BusinessImpactHandler) HandleBaselines(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		tenantID := middleware.TenantFromRequest(r)
		baselines, err := h.service.GetBaselines(tenantID)
		if err != nil {
			api.WriteError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if baselines == nil {
			baselines = []models.IncidentBaseline{}
		}
		api.WriteJSON(w, http.StatusOK, baselines)

	case http.MethodPost:
		var baseline models.IncidentBaseline
		if err := json.NewDecoder(r.Body).Decode(&baseline); err != nil {
			api.WriteError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
			return
		}
		if baseline.ID == "" {
			api.WriteError(w, http.StatusBadRequest, "id is required")
			return
		}
		if baseline.TenantID == "" {
			baseline.TenantID = middleware.TenantFromRequest(r)
		}
		if err := h.service.SaveBaseline(baseline); err != nil {
			api.WriteError(w, http.StatusInternalServerError, err.Error())
			return
		}
		api.WriteJSON(w, http.StatusOK, baseline)

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// ── Monthly report endpoint ──────────────────────────────────────────────────

// HandleMonthlyReport handles GET /api/v1/business/report/monthly?year=2026&month=4
func (h *BusinessImpactHandler) HandleMonthlyReport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	tenantID := middleware.TenantFromRequest(r)

	yearStr := r.URL.Query().Get("year")
	monthStr := r.URL.Query().Get("month")
	if yearStr == "" || monthStr == "" {
		api.WriteError(w, http.StatusBadRequest, "year and month query params are required")
		return
	}

	year, err := strconv.Atoi(yearStr)
	if err != nil {
		api.WriteError(w, http.StatusBadRequest, "invalid year")
		return
	}
	month, err := strconv.Atoi(monthStr)
	if err != nil || month < 1 || month > 12 {
		api.WriteError(w, http.StatusBadRequest, "invalid month")
		return
	}

	resp, err := h.service.GenerateMonthlyRollup(tenantID, year, month)
	if err != nil {
		api.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	api.WriteJSON(w, http.StatusOK, resp)
}

// HandleGetLiveImpact handles GET /api/v1/incidents/{id}/live-impact
// Returns the real-time dollar breakdown: $/min rate, SLA countdown, running total.
// Accepts optional query param ?engineers=N to override the assumed engineer headcount.
func (h *BusinessImpactHandler) HandleGetLiveImpact(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		api.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	// Extract tenant from JWT (for audit) but impact calc uses incident's own tenant.
	_, ok := middleware.ClaimsFromContext(r)
	if !ok {
		api.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	incidentID := extractIncidentIDFromBizPath(r.URL.Path)
	if incidentID == "" {
		api.WriteError(w, http.StatusBadRequest, "missing incident ID")
		return
	}

	engineers := 0 // 0 = let service pick the default (2)
	if q := r.URL.Query().Get("engineers"); q != "" {
		if n, err := strconv.Atoi(q); err == nil && n > 0 {
			engineers = n
		}
	}

	live, err := h.service.ComputeLiveImpact(incidentID, engineers)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			api.WriteError(w, http.StatusNotFound, err.Error())
			return
		}
		api.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	api.WriteJSON(w, http.StatusOK, live)
}

// HandleGetLiveImpactFromBody handles POST /api/v1/incidents/{id}/live-impact
// for cases where the caller wants to pass { "engineer_count": N } in JSON body.
func (h *BusinessImpactHandler) HandleGetLiveImpactFromBody(w http.ResponseWriter, r *http.Request) {
	incidentID := extractIncidentIDFromBizPath(r.URL.Path)
	if incidentID == "" {
		api.WriteError(w, http.StatusBadRequest, "missing incident ID")
		return
	}

	var body struct {
		EngineerCount int `json:"engineer_count"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)

	live, err := h.service.ComputeLiveImpact(incidentID, body.EngineerCount)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			api.WriteError(w, http.StatusNotFound, err.Error())
			return
		}
		api.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	api.WriteJSON(w, http.StatusOK, live)
}

// ── Helpers ──────────────────────────────────────────────────────────────────

func extractIncidentIDFromBizPath(path string) string {
	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
	for i, p := range parts {
		if p == "incidents" && i+1 < len(parts) {
			return parts[i+1]
		}
	}
	return ""
}

func extractLastPathSegment(path string) string {
	path = strings.TrimSuffix(path, "/")
	parts := strings.Split(path, "/")
	if len(parts) == 0 {
		return ""
	}
	return parts[len(parts)-1]
}
