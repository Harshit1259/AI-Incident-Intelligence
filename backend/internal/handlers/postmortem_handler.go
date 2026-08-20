package handlers

import (
	"encoding/json"
	"net/http"
	"strings"

	"ai-incident-platform/backend/internal/api"
	"ai-incident-platform/backend/internal/models"
	"ai-incident-platform/backend/internal/services"
)

// PostMortemHandler handles post-mortem CRUD operations.
type PostMortemHandler struct {
	pmService *services.PostMortemService
}

func NewPostMortemHandler(ps *services.PostMortemService) *PostMortemHandler {
	return &PostMortemHandler{pmService: ps}
}

// Get handles GET /api/v1/incidents/{id}/postmortem
// Returns the existing post-mortem or generates one on demand.
func (h *PostMortemHandler) Get(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	incidentID := extractIncidentIDFromPostmortemPath(r.URL.Path)
	if incidentID == "" {
		api.WriteError(w, http.StatusBadRequest, "missing incident ID")
		return
	}

	pm, err := h.pmService.GetOrCreate(incidentID)
	if err != nil {
		api.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	api.WriteJSON(w, http.StatusOK, pm)
}

// Generate handles POST /api/v1/incidents/{id}/postmortem/generate
// (Re)generates the post-mortem via LLM.
func (h *PostMortemHandler) Generate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	incidentID := extractIncidentIDFromGeneratePath(r.URL.Path)
	if incidentID == "" {
		api.WriteError(w, http.StatusBadRequest, "missing incident ID")
		return
	}

	pm, err := h.pmService.Generate(incidentID)
	if err != nil {
		api.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	api.WriteJSON(w, http.StatusOK, pm)
}

// Update handles PUT /api/v1/incidents/{id}/postmortem
// Saves user edits to an existing post-mortem.
func (h *PostMortemHandler) Update(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	incidentID := extractIncidentIDFromPostmortemPath(r.URL.Path)
	if incidentID == "" {
		api.WriteError(w, http.StatusBadRequest, "missing incident ID")
		return
	}

	var req models.PostMortemUpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		api.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	pm, err := h.pmService.Update(incidentID, req)
	if err != nil {
		api.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	api.WriteJSON(w, http.StatusOK, pm)
}

// ─── path helpers ─────────────────────────────────────────────────────────────

// extractIncidentIDFromPostmortemPath extracts the incident ID from
// paths like /api/v1/incidents/{id}/postmortem
func extractIncidentIDFromPostmortemPath(path string) string {
	// Path: /api/v1/incidents/{id}/postmortem
	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
	// parts: ["api", "v1", "incidents", "{id}", "postmortem"]
	for i, p := range parts {
		if p == "incidents" && i+1 < len(parts) {
			return parts[i+1]
		}
	}
	return ""
}

// extractIncidentIDFromGeneratePath extracts the incident ID from
// paths like /api/v1/incidents/{id}/postmortem/generate
func extractIncidentIDFromGeneratePath(path string) string {
	return extractIncidentIDFromPostmortemPath(path)
}
