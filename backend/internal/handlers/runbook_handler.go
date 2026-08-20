package handlers

import (
	"encoding/json"
	"net/http"
	"strings"

	"ai-incident-platform/backend/internal/middleware"
	"ai-incident-platform/backend/internal/api"
	"ai-incident-platform/backend/internal/models"
	"ai-incident-platform/backend/internal/services"
)

type RunbookHandler struct {
	runbookSvc  *services.RunbookService
	detailSvc   *services.IncidentDetailService
}

func NewRunbookHandler(rs *services.RunbookService, ds *services.IncidentDetailService) *RunbookHandler {
	return &RunbookHandler{runbookSvc: rs, detailSvc: ds}
}

// HandleRunbooks handles GET and POST /api/v1/runbooks
func (h *RunbookHandler) HandleRunbooks(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.listOrSearch(w, r)
	case http.MethodPost:
		h.create(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (h *RunbookHandler) listOrSearch(w http.ResponseWriter, r *http.Request) {
	tenantID := middleware.TenantFromRequest(r)

	service := r.URL.Query().Get("service")
	query := r.URL.Query().Get("q")

	var runbooks []models.Runbook
	var err error

	if query != "" {
		runbooks, err = h.runbookSvc.Search(tenantID, query)
	} else if service != "" {
		runbooks, err = h.runbookSvc.Search(tenantID, service)
	} else {
		runbooks, err = h.runbookSvc.Search(tenantID, "")
	}

	if err != nil {
		api.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if runbooks == nil {
		runbooks = []models.Runbook{}
	}

	api.WriteJSON(w, http.StatusOK, runbooks)
}

func (h *RunbookHandler) create(w http.ResponseWriter, r *http.Request) {
	var req models.RunbookCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		api.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	tenantID := middleware.TenantFromRequest(r)
	createdBy := r.Header.Get("X-User-ID")

	rb, err := h.runbookSvc.Create(req, tenantID, createdBy)
	if err != nil {
		api.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	api.WriteJSON(w, http.StatusCreated, rb)
}

// HandleRunbookByID handles GET, PUT, DELETE /api/v1/runbooks/{id}
func (h *RunbookHandler) HandleRunbookByID(w http.ResponseWriter, r *http.Request) {
	id := extractRunbookID(r.URL.Path)
	if id == "" {
		api.WriteError(w, http.StatusBadRequest, "missing runbook ID")
		return
	}

	switch r.Method {
	case http.MethodGet:
		rb, err := h.runbookSvc.GetByID(id)
		if err != nil {
			api.WriteError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if rb == nil {
			api.WriteError(w, http.StatusNotFound, "runbook not found")
			return
		}
		api.WriteJSON(w, http.StatusOK, rb)

	case http.MethodPut:
		var req models.RunbookCreateRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			api.WriteError(w, http.StatusBadRequest, "invalid request body")
			return
		}
		rb, err := h.runbookSvc.Update(id, req)
		if err != nil {
			api.WriteError(w, http.StatusInternalServerError, err.Error())
			return
		}
		api.WriteJSON(w, http.StatusOK, rb)

	case http.MethodDelete:
		if err := h.runbookSvc.Delete(id); err != nil {
			api.WriteError(w, http.StatusInternalServerError, err.Error())
			return
		}
		api.WriteJSON(w, http.StatusOK, map[string]string{"status": "deleted"})

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// HandleIncidentRunbooks handles GET /api/v1/incidents/{id}/runbooks
func (h *RunbookHandler) HandleIncidentRunbooks(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	incidentID := extractIncidentIDFromRunbookPath(r.URL.Path)
	if incidentID == "" {
		api.WriteError(w, http.StatusBadRequest, "missing incident ID")
		return
	}

	detail, found := h.detailSvc.GetIncidentDetail(incidentID)
	if !found {
		api.WriteError(w, http.StatusNotFound, "incident not found")
		return
	}

	incident := detail.Incident
	pattern := incident.Title + " " + incident.RootCauseSummary

	runbooks, err := h.runbookSvc.FindRelevant(incident.Service, incident.Severity, pattern)
	if err != nil {
		api.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if runbooks == nil {
		runbooks = []models.Runbook{}
	}

	api.WriteJSON(w, http.StatusOK, runbooks)
}

func extractRunbookID(path string) string {
	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
	for i, p := range parts {
		if p == "runbooks" && i+1 < len(parts) {
			return parts[i+1]
		}
	}
	return ""
}

func extractIncidentIDFromRunbookPath(path string) string {
	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
	for i, p := range parts {
		if p == "incidents" && i+1 < len(parts) {
			return parts[i+1]
		}
	}
	return ""
}
