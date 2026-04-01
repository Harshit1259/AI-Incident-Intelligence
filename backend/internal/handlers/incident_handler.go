package handlers

import (
	"encoding/json"
	"net/http"

	"ai-incident-platform/backend/internal/store"
)

type IncidentHandler struct {
	incidentStore *store.IncidentStore
}

func NewIncidentHandler(incidentStore *store.IncidentStore) *IncidentHandler {
	return &IncidentHandler{
		incidentStore: incidentStore,
	}
}

func (incidentHandler *IncidentHandler) ListIncidents(responseWriter http.ResponseWriter, request *http.Request) {
	incidents := incidentHandler.incidentStore.GetIncidents()

	responseWriter.Header().Set("Content-Type", "application/json")
	responseWriter.WriteHeader(http.StatusOK)

	_ = json.NewEncoder(responseWriter).Encode(incidents)
}
