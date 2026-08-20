package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"ai-incident-platform/backend/internal/api"
	"ai-incident-platform/backend/internal/middleware"
	"ai-incident-platform/backend/internal/models"
	"ai-incident-platform/backend/internal/services"
)

type PredictiveIncidentHandler struct {
	svc *services.PredictiveIncidentService
}

func NewPredictiveIncidentHandler(svc *services.PredictiveIncidentService) *PredictiveIncidentHandler {
	return &PredictiveIncidentHandler{svc: svc}
}

// HandleList serves GET /api/v1/predictive-incidents
func (h *PredictiveIncidentHandler) HandleList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		api.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	tenantID := middleware.TenantFromRequest(r)
	statusFilter := r.URL.Query().Get("status")
	limit := 50
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 {
			limit = n
		}
	}

	predictions, err := h.svc.ListPredictions(tenantID, statusFilter, limit)
	if err != nil {
		api.WriteError(w, http.StatusInternalServerError, "failed to fetch predictive incidents")
		return
	}

	if predictions == nil {
		predictions = []models.PredictiveIncident{}
	}
	api.WriteJSON(w, http.StatusOK, predictions)
}

// HandleEvaluate serves POST /api/v1/predictive-incidents/evaluate
// Triggers on-demand trend analysis for a service+metric and returns the prediction (if any).
func (h *PredictiveIncidentHandler) HandleEvaluate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		api.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var req models.PredictiveEvaluateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		api.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Service == "" || req.MetricName == "" {
		api.WriteError(w, http.StatusBadRequest, "service and metric_name are required")
		return
	}
	if req.Threshold <= 0 {
		api.WriteError(w, http.StatusBadRequest, "threshold must be a positive number")
		return
	}

	tenantID := middleware.TenantFromRequest(r)
	pred, err := h.svc.EvaluatePrediction(tenantID, req.Service, req.MetricName, req.Threshold, req.WindowMinutes)
	if err != nil {
		api.WriteError(w, http.StatusInternalServerError, "trend evaluation failed")
		return
	}

	if pred == nil {
		api.WriteJSON(w, http.StatusOK, map[string]string{
			"status":  "safe",
			"message": "no breach predicted within the configured window",
		})
		return
	}

	api.WriteJSON(w, http.StatusCreated, pred)
}

// HandleRecordMetric serves POST /api/v1/predictive-incidents/metrics
// Accepts a single metric data point for trend tracking.
func (h *PredictiveIncidentHandler) HandleRecordMetric(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		api.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var req models.MetricRecordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		api.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Service == "" || req.MetricName == "" {
		api.WriteError(w, http.StatusBadRequest, "service and metric_name are required")
		return
	}

	tenantID := middleware.TenantFromRequest(r)
	if err := h.svc.RecordMetric(tenantID, req.Service, req.MetricName, req.Value); err != nil {
		api.WriteError(w, http.StatusInternalServerError, "failed to record metric")
		return
	}

	api.WriteJSON(w, http.StatusCreated, map[string]string{"status": "recorded"})
}

// HandleSimulate serves POST /api/v1/predictive-incidents/simulate
// Seeds synthetic rising metric data and runs a prediction — useful for demos.
func (h *PredictiveIncidentHandler) HandleSimulate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		api.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var req struct {
		Service string `json:"service"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req) // body optional

	tenantID := middleware.TenantFromRequest(r)
	pred, err := h.svc.Simulate(tenantID, req.Service)
	if err != nil {
		api.WriteError(w, http.StatusInternalServerError, "simulation failed")
		return
	}

	if pred == nil {
		api.WriteJSON(w, http.StatusOK, map[string]string{
			"status":  "safe",
			"message": "simulation did not produce a prediction (data may already exist)",
		})
		return
	}

	api.WriteJSON(w, http.StatusCreated, pred)
}

// HandleResolve serves POST /api/v1/predictive-incidents/{id}/resolve
func (h *PredictiveIncidentHandler) HandleResolve(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		api.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	id := extractPredictiveID(r.URL.Path)
	if id == "" {
		api.WriteError(w, http.StatusBadRequest, "invalid predictive incident id")
		return
	}

	var req struct {
		Resolution string `json:"resolution"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Resolution == "" {
		req.Resolution = "resolved"
	}

	if err := h.svc.Resolve(id, req.Resolution); err != nil {
		api.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	api.WriteJSON(w, http.StatusOK, map[string]string{"status": req.Resolution})
}

// extractPredictiveID parses /api/v1/predictive-incidents/{id}/resolve → id.
func extractPredictiveID(path string) string {
	// path: /api/v1/predictive-incidents/{id}/resolve
	trimmed := strings.Trim(path, "/")
	parts := strings.Split(trimmed, "/")
	// parts: ["api","v1","predictive-incidents","{id}","resolve"]
	if len(parts) >= 4 && parts[2] == "predictive-incidents" {
		return parts[3]
	}
	return ""
}
