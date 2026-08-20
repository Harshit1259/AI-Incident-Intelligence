package handlers

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"ai-incident-platform/backend/internal/api"
	"ai-incident-platform/backend/internal/middleware"
	"ai-incident-platform/backend/internal/models"
	"ai-incident-platform/backend/internal/services"
)

// SchemaRegistryHandler exposes CRUD for custom source schema mappings.
//
// Routes (all under /api/schema-registry):
//   GET    /api/schema-registry           — list all mappings for tenant
//   POST   /api/schema-registry           — create a new mapping
//   GET    /api/schema-registry/stats     — usage stats per source type
//   GET    /api/schema-registry/{id}      — get one mapping
//   PUT    /api/schema-registry/{id}      — update mapping
//   DELETE /api/schema-registry/{id}      — delete mapping
type SchemaRegistryHandler struct {
	svc *services.SchemaRegistryService
}

func NewSchemaRegistryHandler(svc *services.SchemaRegistryService) *SchemaRegistryHandler {
	return &SchemaRegistryHandler{svc: svc}
}

func (h *SchemaRegistryHandler) Handle(w http.ResponseWriter, r *http.Request) {
	tenantID := middleware.TenantFromRequest(r)

	path := strings.TrimPrefix(r.URL.Path, "/api/schema-registry")
	path = strings.TrimSuffix(path, "/")

	switch {
	case path == "" || path == "/":
		switch r.Method {
		case http.MethodGet:
			h.list(w, r, tenantID)
		case http.MethodPost:
			h.create(w, r, tenantID)
		default:
			api.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		}
	case path == "/stats":
		if r.Method != http.MethodGet {
			api.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		h.stats(w, r, tenantID)
	default:
		id := strings.TrimPrefix(path, "/")
		switch r.Method {
		case http.MethodGet:
			h.getOne(w, r, id)
		case http.MethodPut:
			h.update(w, r, tenantID, id)
		case http.MethodDelete:
			h.delete(w, r, tenantID, id)
		default:
			api.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		}
	}
}

func (h *SchemaRegistryHandler) list(w http.ResponseWriter, _ *http.Request, tenantID string) {
	mappings, err := h.svc.List(tenantID)
	if err != nil {
		api.WriteError(w, http.StatusInternalServerError, "failed to list schema mappings")
		return
	}
	if mappings == nil {
		mappings = []models.SchemaMapping{}
	}
	api.WriteJSON(w, http.StatusOK, mappings)
}

func (h *SchemaRegistryHandler) create(w http.ResponseWriter, r *http.Request, tenantID string) {
	var m models.SchemaMapping
	if err := json.NewDecoder(r.Body).Decode(&m); err != nil {
		api.WriteError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	m.ID = ""
	if err := h.svc.Create(tenantID, &m); err != nil {
		api.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	api.WriteJSON(w, http.StatusCreated, m)
}

func (h *SchemaRegistryHandler) getOne(w http.ResponseWriter, _ *http.Request, id string) {
	m, err := h.svc.GetByID(id)
	if err != nil {
		if err == sql.ErrNoRows {
			api.WriteError(w, http.StatusNotFound, "schema mapping not found")
			return
		}
		api.WriteError(w, http.StatusInternalServerError, "failed to get schema mapping")
		return
	}
	api.WriteJSON(w, http.StatusOK, m)
}

func (h *SchemaRegistryHandler) update(w http.ResponseWriter, r *http.Request, tenantID, id string) {
	var m models.SchemaMapping
	if err := json.NewDecoder(r.Body).Decode(&m); err != nil {
		api.WriteError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	m.ID = id
	m.UpdatedAt = time.Now().UTC()
	if err := h.svc.Update(tenantID, &m); err != nil {
		api.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	api.WriteJSON(w, http.StatusOK, m)
}

func (h *SchemaRegistryHandler) delete(w http.ResponseWriter, r *http.Request, tenantID, id string) {
	if err := h.svc.Delete(id, tenantID); err != nil {
		if err == sql.ErrNoRows {
			api.WriteError(w, http.StatusNotFound, "schema mapping not found")
			return
		}
		api.WriteError(w, http.StatusInternalServerError, "failed to delete schema mapping")
		return
	}
	api.WriteJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (h *SchemaRegistryHandler) stats(w http.ResponseWriter, _ *http.Request, tenantID string) {
	stats, err := h.svc.Stats(tenantID)
	if err != nil {
		api.WriteError(w, http.StatusInternalServerError, "failed to get schema mapping stats")
		return
	}
	if stats == nil {
		stats = []models.SchemaMappingStats{}
	}
	api.WriteJSON(w, http.StatusOK, stats)
}
