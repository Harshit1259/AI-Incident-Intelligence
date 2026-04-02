package handlers

import (
	"encoding/json"
	"net/http"
	"strings"

	"ai-incident-platform/backend/internal/services"
	"ai-incident-platform/backend/internal/store"
)

type IncidentHandler struct {
	incidentStore         *store.IncidentStore
	incidentDetailService *services.IncidentDetailService
}

func NewIncidentHandler(
	incidentStore *store.IncidentStore,
	incidentDetailService *services.IncidentDetailService,
) *IncidentHandler {
	return &IncidentHandler{
		incidentStore:         incidentStore,
		incidentDetailService: incidentDetailService,
	}
}

func (incidentHandler *IncidentHandler) ListIncidents(responseWriter http.ResponseWriter, request *http.Request) {
	incidents, err := incidentHandler.incidentStore.GetIncidents()
	if err != nil {
		http.Error(responseWriter, "failed to fetch incidents", http.StatusInternalServerError)
		return
	}

	responseWriter.Header().Set("Content-Type", "application/json")
	responseWriter.WriteHeader(http.StatusOK)

	_ = json.NewEncoder(responseWriter).Encode(incidents)
}

func (incidentHandler *IncidentHandler) GetIncidentDetail(responseWriter http.ResponseWriter, request *http.Request) {
	pathParts := strings.Split(strings.Trim(request.URL.Path, "/"), "/")
	if len(pathParts) < 4 {
		http.Error(responseWriter, "incident id is required", http.StatusBadRequest)
		return
	}

	incidentID := pathParts[3]

	detail, found := incidentHandler.incidentDetailService.GetIncidentDetail(incidentID)
	if !found {
		http.Error(responseWriter, "incident not found", http.StatusNotFound)
		return
	}

	responseWriter.Header().Set("Content-Type", "application/json")
	responseWriter.WriteHeader(http.StatusOK)

	_ = json.NewEncoder(responseWriter).Encode(detail)
}
