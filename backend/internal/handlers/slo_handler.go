package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"ai-incident-platform/backend/internal/middleware"
	"ai-incident-platform/backend/internal/api"
	"ai-incident-platform/backend/internal/models"
	"ai-incident-platform/backend/internal/services"
	"ai-incident-platform/backend/internal/store"
)

type SLOHandler struct {
	sloService *services.SLOService
	sloStore   *store.SLOStore
}

func NewSLOHandler(ss *services.SLOService, st *store.SLOStore) *SLOHandler {
	return &SLOHandler{sloService: ss, sloStore: st}
}

// HandleSLOs dispatches GET/POST on /api/v1/slos
func (h *SLOHandler) HandleSLOs(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.listSLOs(w, r)
	case http.MethodPost:
		h.createSLO(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// HandleSLOByID dispatches GET/DELETE on /api/v1/slos/{id}
func (h *SLOHandler) HandleSLOByID(w http.ResponseWriter, r *http.Request) {
	id := extractSLOID(r.URL.Path)
	if id == "" {
		api.WriteError(w, http.StatusBadRequest, "slo id is required")
		return
	}

	// Check if this is a measurement route
	if strings.HasSuffix(r.URL.Path, "/measure") {
		if r.Method == http.MethodPost {
			h.recordMeasurement(w, r, id)
			return
		}
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	switch r.Method {
	case http.MethodGet:
		h.getSLO(w, r, id)
	case http.MethodDelete:
		h.deleteSLO(w, r, id)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (h *SLOHandler) listSLOs(w http.ResponseWriter, r *http.Request) {
	tenantID := middleware.TenantFromRequest(r)

	statuses, err := h.sloService.GetAllStatuses(tenantID)
	if err != nil {
		api.WriteError(w, http.StatusInternalServerError, "failed to fetch SLO statuses")
		return
	}

	api.WriteJSON(w, http.StatusOK, statuses)
}

func (h *SLOHandler) createSLO(w http.ResponseWriter, r *http.Request) {
	var req models.SLOCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		api.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Service == "" || req.Name == "" {
		api.WriteError(w, http.StatusBadRequest, "service and name are required")
		return
	}

	if req.TargetPercent <= 0 || req.TargetPercent > 100 {
		req.TargetPercent = 99.9
	}
	if req.WindowDays <= 0 {
		req.WindowDays = 30
	}
	if req.MetricType == "" {
		req.MetricType = "availability"
	}

	now := time.Now()
	def := models.SLODefinition{
		ID:            fmt.Sprintf("slo-%d", now.UnixNano()),
		TenantID:      middleware.TenantFromRequest(r),
		Service:       req.Service,
		Name:          req.Name,
		Description:   req.Description,
		TargetPercent: req.TargetPercent,
		WindowDays:    req.WindowDays,
		MetricType:    req.MetricType,
		CreatedAt:     now,
		UpdatedAt:     now,
	}

	if err := h.sloStore.CreateSLO(def); err != nil {
		api.WriteError(w, http.StatusInternalServerError, "failed to create SLO")
		return
	}

	api.WriteJSON(w, http.StatusCreated, def)
}

func (h *SLOHandler) getSLO(w http.ResponseWriter, r *http.Request, id string) {
	def, err := h.sloStore.GetSLOByID(id)
	if err != nil {
		api.WriteError(w, http.StatusInternalServerError, "failed to fetch SLO")
		return
	}
	if def == nil {
		api.WriteError(w, http.StatusNotFound, "SLO not found")
		return
	}

	status := h.sloService.ComputeStatus(*def)

	// Include measurements
	since := time.Now().Add(-time.Duration(def.WindowDays) * 24 * time.Hour)
	measurements, _ := h.sloStore.GetMeasurements(id, since)
	status.Measurements = measurements

	api.WriteJSON(w, http.StatusOK, status)
}

func (h *SLOHandler) deleteSLO(w http.ResponseWriter, r *http.Request, id string) {
	if err := h.sloStore.DeleteSLO(id); err != nil {
		api.WriteError(w, http.StatusInternalServerError, "failed to delete SLO")
		return
	}

	api.WriteJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (h *SLOHandler) recordMeasurement(w http.ResponseWriter, r *http.Request, sloID string) {
	var req struct {
		TotalRequests int64 `json:"total_requests"`
		GoodRequests  int64 `json:"good_requests"`
		BadMinutes    int   `json:"bad_minutes"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		api.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if err := h.sloService.RecordMeasurement(sloID, req.TotalRequests, req.GoodRequests, req.BadMinutes); err != nil {
		api.WriteError(w, http.StatusInternalServerError, "failed to record measurement")
		return
	}

	api.WriteJSON(w, http.StatusOK, map[string]string{"status": "recorded"})
}

func extractSLOID(path string) string {
	// /api/v1/slos/{id} or /api/v1/slos/{id}/measure
	trimmed := strings.Trim(path, "/")
	parts := strings.Split(trimmed, "/")
	// api/v1/slos/{id}
	if len(parts) >= 4 && parts[0] == "api" && parts[1] == "v1" && parts[2] == "slos" {
		return parts[3]
	}
	return ""
}
