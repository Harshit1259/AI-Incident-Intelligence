package handlers

import (
	"net/http"
	"strconv"

	"ai-incident-platform/backend/internal/api"
	"ai-incident-platform/backend/internal/middleware"
	"ai-incident-platform/backend/internal/store"
)

// AuditHandler handles audit log query endpoints.
type AuditHandler struct {
	store *store.AuditLogStore
}

// NewAuditHandler creates a new AuditHandler.
func NewAuditHandler(s *store.AuditLogStore) *AuditHandler {
	return &AuditHandler{store: s}
}

// HandleAudit handles GET /api/v1/audit.
func (h *AuditHandler) HandleAudit(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		api.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	tenantID := middleware.TenantFromRequest(r)

	resourceType := r.URL.Query().Get("resource_type")

	limit := 50
	if v := r.URL.Query().Get("limit"); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil && parsed > 0 {
			limit = parsed
		}
	}

	offset := 0
	if v := r.URL.Query().Get("offset"); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil && parsed >= 0 {
			offset = parsed
		}
	}

	entries, total, err := h.store.Query(tenantID, resourceType, limit, offset)
	if err != nil {
		api.WriteError(w, http.StatusInternalServerError, "failed to query audit log")
		return
	}

	if entries == nil {
		entries = []store.AuditLogEntry{}
	}

	api.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"entries": entries,
		"total":   total,
		"limit":   limit,
		"offset":  offset,
	})
}
