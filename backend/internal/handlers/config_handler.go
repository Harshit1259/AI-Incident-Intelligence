package handlers

import (
	"encoding/json"
	"net/http"

	"ai-incident-platform/backend/internal/api"
	"ai-incident-platform/backend/internal/audit"
	"ai-incident-platform/backend/internal/middleware"
	"ai-incident-platform/backend/internal/store"
)

// ConfigHandler handles tenant configuration endpoints.
type ConfigHandler struct {
	store *store.TenantConfigStore
}

// NewConfigHandler creates a new ConfigHandler.
func NewConfigHandler(s *store.TenantConfigStore) *ConfigHandler {
	return &ConfigHandler{store: s}
}

// HandleConfig handles GET /api/v1/config and PUT /api/v1/config.
func (h *ConfigHandler) HandleConfig(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.getConfig(w, r)
	case http.MethodPut:
		h.updateConfig(w, r)
	default:
		api.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (h *ConfigHandler) getConfig(w http.ResponseWriter, r *http.Request) {
	tenantID := middleware.TenantFromRequest(r)

	settings, err := h.store.Get(tenantID)
	if err != nil {
		api.WriteError(w, http.StatusInternalServerError, "failed to get config")
		return
	}

	api.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"tenant_id": tenantID,
		"settings":  settings,
	})
}

func (h *ConfigHandler) updateConfig(w http.ResponseWriter, r *http.Request) {
	tenantID := middleware.TenantFromRequest(r)
	actor := "system"
	claims, ok := middleware.ClaimsFromContext(r)
	if ok {
		if claims.TenantID != "" {
			tenantID = claims.TenantID
		}
		if claims.UserID != "" {
			actor = claims.UserID
		}
	}

	var settings map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&settings); err != nil {
		api.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if err := h.store.Set(tenantID, settings); err != nil {
		api.WriteError(w, http.StatusInternalServerError, "failed to update config")
		return
	}

	audit.Log(tenantID, actor, "config.updated", "config", tenantID, settings)

	api.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"tenant_id": tenantID,
		"settings":  settings,
		"status":    "updated",
	})
}
