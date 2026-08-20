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

type AutoResolveHandler struct {
	svc *services.AutoResolveService
}

func NewAutoResolveHandler(svc *services.AutoResolveService) *AutoResolveHandler {
	return &AutoResolveHandler{svc: svc}
}

// HandleRules handles GET and POST /api/v1/auto-resolve/rules
func (h *AutoResolveHandler) HandleRules(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.listRules(w, r)
	case http.MethodPost:
		h.createRule(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (h *AutoResolveHandler) listRules(w http.ResponseWriter, r *http.Request) {
	tenantID := middleware.TenantFromRequest(r)
	limit, offset := parsePagination(r, 50, 1000)

	rules, err := h.svc.GetRules(tenantID, limit, offset)
	if err != nil {
		api.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if rules == nil {
		rules = []models.AutoResolveRule{}
	}

	api.WriteJSON(w, http.StatusOK, rules)
}

func (h *AutoResolveHandler) createRule(w http.ResponseWriter, r *http.Request) {
	var req models.AutoResolveRuleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		api.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	tenantID := middleware.TenantFromRequest(r)

	rule, err := h.svc.CreateRule(req, tenantID)
	if err != nil {
		api.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	api.WriteJSON(w, http.StatusCreated, rule)
}

// HandleRuleByID handles DELETE /api/v1/auto-resolve/rules/{id} and PUT /api/v1/auto-resolve/rules/{id}/toggle
func (h *AutoResolveHandler) HandleRuleByID(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path

	if strings.HasSuffix(path, "/toggle") && r.Method == http.MethodPut {
		h.toggleRule(w, r)
		return
	}

	if r.Method == http.MethodDelete {
		h.deleteRule(w, r)
		return
	}

	http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
}

func (h *AutoResolveHandler) deleteRule(w http.ResponseWriter, r *http.Request) {
	id := extractAutoResolveRuleID(r.URL.Path)
	if id == "" {
		api.WriteError(w, http.StatusBadRequest, "missing rule ID")
		return
	}

	if err := h.svc.DeleteRule(id); err != nil {
		api.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	api.WriteJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (h *AutoResolveHandler) toggleRule(w http.ResponseWriter, r *http.Request) {
	id := extractAutoResolveRuleID(r.URL.Path)
	if id == "" {
		api.WriteError(w, http.StatusBadRequest, "missing rule ID")
		return
	}

	var req struct {
		Enabled bool `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		api.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if err := h.svc.ToggleRule(id, req.Enabled); err != nil {
		api.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	api.WriteJSON(w, http.StatusOK, map[string]interface{}{"status": "updated", "enabled": req.Enabled})
}

func extractAutoResolveRuleID(path string) string {
	// Path: /api/v1/auto-resolve/rules/{id} or /api/v1/auto-resolve/rules/{id}/toggle
	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
	for i, p := range parts {
		if p == "rules" && i+1 < len(parts) {
			id := parts[i+1]
			if id == "toggle" {
				return ""
			}
			return id
		}
	}
	return ""
}
