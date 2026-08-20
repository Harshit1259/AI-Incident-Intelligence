package handlers

import (
	"encoding/json"
	"net/http"
	"strings"

	"ai-incident-platform/backend/internal/middleware"
	"ai-incident-platform/backend/internal/services"
)

// IncidentMemoryHandler exposes incident learning and playbook endpoints.
type IncidentMemoryHandler struct {
	memoryService *services.IncidentMemoryService
}

func NewIncidentMemoryHandler(ms *services.IncidentMemoryService) *IncidentMemoryHandler {
	return &IncidentMemoryHandler{memoryService: ms}
}

// HandleGetMemory handles GET /api/v1/incidents/memory/{id}
func (h *IncidentMemoryHandler) HandleGetMemory(w http.ResponseWriter, r *http.Request) {
	claims, ok := middleware.ClaimsFromContext(r)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	incidentID := extractMemoryIncidentID(r.URL.Path)
	if incidentID == "" {
		http.Error(w, "missing incident id", http.StatusBadRequest)
		return
	}

	history, err := h.memoryService.GetPatternHistory(incidentID, claims.TenantID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(history)
}

// HandleRecordResolution handles POST /api/v1/incidents/memory/{id}/record
func (h *IncidentMemoryHandler) HandleRecordResolution(w http.ResponseWriter, r *http.Request) {
	claims, ok := middleware.ClaimsFromContext(r)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	incidentID := extractMemoryRecordID(r.URL.Path)
	if incidentID == "" {
		http.Error(w, "missing incident id", http.StatusBadRequest)
		return
	}

	var body struct {
		Note string `json:"note"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		body.Note = ""
	}

	if err := h.memoryService.RecordResolution(incidentID, claims.TenantID, body.Note); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]string{"status": "recorded"})
}

func extractMemoryIncidentID(path string) string {
	// /api/v1/incidents/memory/{id}
	p := strings.TrimPrefix(path, "/api/v1/incidents/memory/")
	p = strings.SplitN(p, "/", 2)[0] // stop at first slash (strips /record suffix if present)
	return strings.TrimSpace(p)
}

func extractMemoryRecordID(path string) string {
	// /api/v1/incidents/memory/{id}/record
	p := strings.TrimPrefix(path, "/api/v1/incidents/memory/")
	p = strings.TrimSuffix(p, "/record")
	return strings.TrimSpace(p)
}
