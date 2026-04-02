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

func (incidentHandler *IncidentHandler) UpdateIncidentStatus(responseWriter http.ResponseWriter, request *http.Request) {
	pathParts := strings.Split(strings.Trim(request.URL.Path, "/"), "/")
	if len(pathParts) < 5 {
		http.Error(responseWriter, "invalid request", http.StatusBadRequest)
		return
	}

	incidentID := pathParts[3]
	action := pathParts[4]

	incident, found := incidentHandler.incidentStore.GetIncidentByID(incidentID)
	if !found {
		http.Error(responseWriter, "incident not found", http.StatusNotFound)
		return
	}

	switch action {
	case "ack":
		incident.Status = "acknowledged"
	case "resolve":
		incident.Status = "resolved"
	case "reopen":
		incident.Status = "open"
	default:
		http.Error(responseWriter, "invalid action", http.StatusBadRequest)
		return
	}

	if err := incidentHandler.incidentStore.UpdateIncident(incident); err != nil {
		http.Error(responseWriter, "failed to update incident", http.StatusInternalServerError)
		return
	}

	responseWriter.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(responseWriter).Encode(incident)
}