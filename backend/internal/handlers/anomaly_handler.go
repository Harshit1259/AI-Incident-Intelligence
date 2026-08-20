package handlers

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"

	"ai-incident-platform/backend/internal/api"
	"ai-incident-platform/backend/internal/middleware"
	"ai-incident-platform/backend/internal/services"
	"ai-incident-platform/backend/internal/store"
)

type AnomalyHandler struct {
	anomalyService *services.AnomalyService
	anomalyStore   *store.AnomalyStore
}

func NewAnomalyHandler(as *services.AnomalyService, st *store.AnomalyStore) *AnomalyHandler {
	return &AnomalyHandler{anomalyService: as, anomalyStore: st}
}

// HandleAnomalies dispatches GET on /api/v1/anomalies
func (h *AnomalyHandler) HandleAnomalies(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	tenantID := middleware.TenantFromRequest(r)

	limit := 50
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 {
			limit = parsed
		}
	}

	alerts, err := h.anomalyStore.GetAlerts(tenantID, limit)
	if err != nil {
		api.WriteError(w, http.StatusInternalServerError, "failed to fetch anomaly alerts")
		return
	}

	api.WriteJSON(w, http.StatusOK, alerts)
}

// HandleCheckMetric handles POST /api/v1/anomalies/check
func (h *AnomalyHandler) HandleCheckMetric(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Service   string  `json:"service"`
		Metric    string  `json:"metric"`
		Value     float64 `json:"value"`
		Threshold float64 `json:"threshold"`
		Trend     string  `json:"trend"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		api.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Service == "" || req.Metric == "" {
		api.WriteError(w, http.StatusBadRequest, "service and metric are required")
		return
	}

	alert, err := h.anomalyService.CheckMetric(middleware.TenantFromRequest(r), req.Service, req.Metric, req.Value, req.Threshold, req.Trend)
	if err != nil {
		api.WriteError(w, http.StatusInternalServerError, "failed to check metric")
		return
	}

	if alert == nil {
		api.WriteJSON(w, http.StatusOK, map[string]string{"status": "normal"})
		return
	}

	api.WriteJSON(w, http.StatusOK, alert)
}

// HandleAckAlert handles POST /api/v1/anomalies/{id}/ack
func (h *AnomalyHandler) HandleAckAlert(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	id := extractAnomalyID(r.URL.Path)
	if id <= 0 {
		api.WriteError(w, http.StatusBadRequest, "invalid alert id")
		return
	}

	if err := h.anomalyStore.AcknowledgeAlert(id); err != nil {
		api.WriteError(w, http.StatusInternalServerError, "failed to acknowledge alert")
		return
	}

	api.WriteJSON(w, http.StatusOK, map[string]string{"status": "acknowledged"})
}

// HandleSimulate handles POST /api/v1/anomalies/simulate
func (h *AnomalyHandler) HandleSimulate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Service string `json:"service"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && err != io.EOF {
		api.WriteError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	alert, err := h.anomalyService.SimulateAnomaly(middleware.TenantFromRequest(r), req.Service)
	if err != nil {
		api.WriteError(w, http.StatusInternalServerError, "failed to simulate anomaly")
		return
	}

	api.WriteJSON(w, http.StatusCreated, alert)
}

func extractAnomalyID(path string) int {
	// /api/v1/anomalies/{id}/ack
	trimmed := strings.Trim(path, "/")
	parts := strings.Split(trimmed, "/")
	if len(parts) >= 4 && parts[0] == "api" && parts[1] == "v1" && parts[2] == "anomalies" {
		id, err := strconv.Atoi(parts[3])
		if err != nil {
			return 0
		}
		return id
	}
	return 0
}
