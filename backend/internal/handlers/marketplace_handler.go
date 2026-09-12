package handlers

import (
	"encoding/json"
	"net/http"
	"strings"

	"ai-incident-platform/backend/internal/api"
	"ai-incident-platform/backend/internal/middleware"
	"ai-incident-platform/backend/internal/models"
	"ai-incident-platform/backend/internal/services"
)

// MarketplaceHandler serves the Integration Hub API.
//
// Routes (all require JWT auth, operator role or higher):
//   GET  /api/v1/marketplace               — catalog with per-tenant status
//   GET  /api/v1/marketplace/{id}          — integration detail + sample payload
//   GET  /api/v1/marketplace/{id}/sample   — raw sample payload JSON
//   POST /api/v1/marketplace/{id}/enable   — enable with config
//   POST /api/v1/marketplace/{id}/disable  — disable
//   POST /api/v1/marketplace/{id}/test     — test connection
//   POST /api/v1/marketplace/{id}/test-alert — send synthetic test alert
type MarketplaceHandler struct {
	svc *services.MarketplaceService
}

func NewMarketplaceHandler(svc *services.MarketplaceService) *MarketplaceHandler {
	return &MarketplaceHandler{svc: svc}
}

// Handle dispatches all /api/v1/marketplace* requests.
func (h *MarketplaceHandler) Handle(w http.ResponseWriter, r *http.Request) {
	claims, ok := middleware.ClaimsFromContext(r)
	if !ok {
		api.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	sub := strings.TrimPrefix(r.URL.Path, "/api/v1/marketplace")

	// /api/v1/marketplace — catalog list
	if sub == "" || sub == "/" {
		if r.Method != http.MethodGet {
			api.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		h.listCatalog(w, claims.TenantID)
		return
	}

	integrationID, action := marketplacePathParts(sub)
	if integrationID == "" {
		api.WriteError(w, http.StatusNotFound, "not found")
		return
	}

	switch action {
	case "", "/":
		if r.Method != http.MethodGet {
			api.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		h.getIntegration(w, claims.TenantID, integrationID)

	case "/sample":
		if r.Method != http.MethodGet {
			api.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		h.getSample(w, integrationID)

	case "/enable":
		if r.Method != http.MethodPost {
			api.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		h.enable(w, r, claims.TenantID, integrationID)

	case "/disable":
		if r.Method != http.MethodPost {
			api.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		h.disable(w, claims.TenantID, integrationID)

	case "/test":
		if r.Method != http.MethodPost {
			api.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		h.testConnection(w, claims.TenantID, integrationID)

	case "/test-alert":
		if r.Method != http.MethodPost {
			api.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		h.sendTestAlert(w, claims.TenantID, integrationID)

	default:
		api.WriteError(w, http.StatusNotFound, "not found")
	}
}

// ── Route handlers ────────────────────────────────────────────────────────────

func (h *MarketplaceHandler) listCatalog(w http.ResponseWriter, tenantID string) {
	entries, err := h.svc.ListCatalog(tenantID)
	if err != nil {
		api.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Build category summary for the UI overview panel.
	byCategory := map[string]int{}
	enabledCount := 0
	for _, e := range entries {
		byCategory[string(e.Category)]++
		if e.Enabled {
			enabledCount++
		}
	}

	api.WriteJSON(w, http.StatusOK, map[string]any{
		"integrations":    entries,
		"total":           len(entries),
		"enabled_count":   enabledCount,
		"by_category":     byCategory,
	})
}

func (h *MarketplaceHandler) getIntegration(w http.ResponseWriter, tenantID, integrationID string) {
	entry, err := h.svc.GetIntegration(tenantID, integrationID)
	if err != nil {
		api.WriteError(w, http.StatusNotFound, err.Error())
		return
	}
	api.WriteJSON(w, http.StatusOK, entry)
}

func (h *MarketplaceHandler) getSample(w http.ResponseWriter, integrationID string) {
	payload, err := h.svc.GetSamplePayload(integrationID)
	if err != nil {
		api.WriteError(w, http.StatusNotFound, err.Error())
		return
	}
	// Return as JSON — decode then re-encode to validate + pretty-print.
	var raw any
	if err := json.Unmarshal([]byte(payload), &raw); err != nil {
		// Fallback: return as raw string if not valid JSON.
		api.WriteJSON(w, http.StatusOK, map[string]string{"payload": payload})
		return
	}
	api.WriteJSON(w, http.StatusOK, map[string]any{
		"integration_id": integrationID,
		"payload":        raw,
	})
}

func (h *MarketplaceHandler) enable(w http.ResponseWriter, r *http.Request, tenantID, integrationID string) {
	var req models.EnableRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		api.WriteError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if req.AuthConfig == nil {
		req.AuthConfig = map[string]string{}
	}

	entry, err := h.svc.Enable(tenantID, integrationID, req)
	if err != nil {
		api.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	api.WriteJSON(w, http.StatusOK, entry)
}

func (h *MarketplaceHandler) disable(w http.ResponseWriter, tenantID, integrationID string) {
	if err := h.svc.Disable(tenantID, integrationID); err != nil {
		api.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	api.WriteJSON(w, http.StatusOK, map[string]string{
		"status":         "disabled",
		"integration_id": integrationID,
	})
}

func (h *MarketplaceHandler) testConnection(w http.ResponseWriter, tenantID, integrationID string) {
	result, err := h.svc.TestConnection(tenantID, integrationID)
	if err != nil {
		api.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	api.WriteJSON(w, http.StatusOK, result)
}

func (h *MarketplaceHandler) sendTestAlert(w http.ResponseWriter, tenantID, integrationID string) {
	n, err := h.svc.SendTestAlert(tenantID, integrationID)
	if err != nil {
		api.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	api.WriteJSON(w, http.StatusOK, map[string]any{
		"status":         "sent",
		"integration_id": integrationID,
		"events":         n,
		"message":        "Sample alert accepted by the real parser — the incident should appear within seconds.",
	})
}

// ── Helpers ───────────────────────────────────────────────────────────────────

// marketplacePathParts splits a sub-path like "/prometheus/enable" into ("prometheus", "/enable").
func marketplacePathParts(sub string) (id, action string) {
	sub = strings.TrimPrefix(sub, "/")
	if sub == "" {
		return "", ""
	}
	parts := strings.SplitN(sub, "/", 2)
	id = parts[0]
	if len(parts) > 1 {
		action = "/" + parts[1]
	}
	return id, action
}
