package handlers

import (
	"encoding/json"
	"net/http"
	"strings"

	"ai-incident-platform/backend/internal/api"
	"ai-incident-platform/backend/internal/middleware"
	"ai-incident-platform/backend/internal/models"
	"ai-incident-platform/backend/internal/services"
	"ai-incident-platform/backend/internal/store"
)

type DependencyHandler struct {
	svc           *services.DependencyService
	incidentStore *store.IncidentStore
}

func NewDependencyHandler(svc *services.DependencyService, is *store.IncidentStore) *DependencyHandler {
	return &DependencyHandler{svc: svc, incidentStore: is}
}

// HandleDependencies handles GET and POST /api/v1/dependencies
func (h *DependencyHandler) HandleDependencies(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.listCatalog(w, r)
	case http.MethodPost:
		h.addEntry(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (h *DependencyHandler) listCatalog(w http.ResponseWriter, r *http.Request) {
	tenantID := middleware.TenantFromRequest(r)

	entries, err := h.svc.GetCatalog(tenantID)
	if err != nil {
		api.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if entries == nil {
		entries = []models.DependencyCatalogEntry{}
	}

	api.WriteJSON(w, http.StatusOK, entries)
}

func (h *DependencyHandler) addEntry(w http.ResponseWriter, r *http.Request) {
	var entry models.DependencyCatalogEntry
	if err := json.NewDecoder(r.Body).Decode(&entry); err != nil {
		api.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if entry.Service == "" || entry.DependencyName == "" {
		api.WriteError(w, http.StatusBadRequest, "service and dependency_name are required")
		return
	}

	// Always stamp with the request tenant — never trust body-provided tenant.
	entry.TenantID = middleware.TenantFromRequest(r)

	if err := h.svc.AddEntry(entry); err != nil {
		api.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	api.WriteJSON(w, http.StatusCreated, entry)
}

// HandleDependencyByID handles DELETE /api/v1/dependencies/{id}
func (h *DependencyHandler) HandleDependencyByID(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	id := extractDependencyID(r.URL.Path)
	if id == "" {
		api.WriteError(w, http.StatusBadRequest, "missing dependency ID")
		return
	}

	if err := h.svc.DeleteEntry(id); err != nil {
		api.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	api.WriteJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// HandleAttribution handles GET /api/v1/incidents/{id}/attribution
func (h *DependencyHandler) HandleAttribution(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	incidentID := extractIncidentIDFromAttrPath(r.URL.Path)
	if incidentID == "" {
		api.WriteError(w, http.StatusBadRequest, "missing incident ID")
		return
	}

	incident, found := h.incidentStore.GetIncidentByID(incidentID)
	if !found {
		api.WriteError(w, http.StatusNotFound, "incident not found")
		return
	}

	attr, err := h.svc.AttributeIncident(incident)
	if err != nil {
		api.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	api.WriteJSON(w, http.StatusOK, attr)
}

func extractDependencyID(path string) string {
	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
	for i, p := range parts {
		if p == "dependencies" && i+1 < len(parts) {
			return parts[i+1]
		}
	}
	return ""
}

func extractIncidentIDFromAttrPath(path string) string {
	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
	for i, p := range parts {
		if p == "incidents" && i+1 < len(parts) {
			return parts[i+1]
		}
	}
	return ""
}
