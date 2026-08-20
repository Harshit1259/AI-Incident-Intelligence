package handlers

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"ai-incident-platform/backend/internal/api"
	"ai-incident-platform/backend/internal/middleware"
	"ai-incident-platform/backend/internal/models"
	"ai-incident-platform/backend/internal/services"
)

// DomainMemoryHandler exposes all four AI memory pillars via /api/v1/memory/...
type DomainMemoryHandler struct {
	svc *services.DomainMemoryService
}

func NewDomainMemoryHandler(svc *services.DomainMemoryService) *DomainMemoryHandler {
	return &DomainMemoryHandler{svc: svc}
}

// Handle is the top-level dispatcher for /api/v1/memory/...
func (h *DomainMemoryHandler) Handle(w http.ResponseWriter, r *http.Request) {
	claims, ok := middleware.ClaimsFromContext(r)
	if !ok {
		api.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	// Strip /api/v1/memory prefix and dispatch
	sub := strings.TrimPrefix(r.URL.Path, "/api/v1/memory")

	switch {
	case sub == "" || sub == "/":
		api.WriteJSON(w, http.StatusOK, map[string]string{"status": "ai memory active"})

	// Full context for an incident
	case strings.HasPrefix(sub, "/context/"):
		h.handleContext(w, r, claims.TenantID, strings.TrimPrefix(sub, "/context/"))

	// Structured resolution recording for an incident
	case strings.HasPrefix(sub, "/resolve/"):
		h.handleResolve(w, r, claims.TenantID, strings.TrimPrefix(sub, "/resolve/"))

	// Remediation patterns
	case strings.HasPrefix(sub, "/remediations"):
		h.handleRemediations(w, r, claims.TenantID, strings.TrimPrefix(sub, "/remediations"))

	// Deploy signatures
	case strings.HasPrefix(sub, "/deploy-signatures"):
		h.handleDeploySignatures(w, r, claims.TenantID, strings.TrimPrefix(sub, "/deploy-signatures"))

	// Runbook preferences
	case strings.HasPrefix(sub, "/runbook-preferences"):
		h.handleRunbookPreferences(w, r, claims.TenantID, strings.TrimPrefix(sub, "/runbook-preferences"))

	default:
		api.WriteError(w, http.StatusNotFound, "not found")
	}
}

// ── Full memory context ───────────────────────────────────────────────────────

func (h *DomainMemoryHandler) handleContext(w http.ResponseWriter, r *http.Request, tenantID, incidentID string) {
	incidentID = strings.Trim(incidentID, "/")
	if incidentID == "" {
		api.WriteError(w, http.StatusBadRequest, "incident ID required")
		return
	}
	if r.Method != http.MethodGet {
		api.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	ctx, err := h.svc.GetFullMemoryContext(incidentID, tenantID)
	if err != nil {
		api.WriteError(w, http.StatusNotFound, err.Error())
		return
	}
	api.WriteJSON(w, http.StatusOK, ctx)
}

// ── Structured resolution recording ──────────────────────────────────────────

func (h *DomainMemoryHandler) handleResolve(w http.ResponseWriter, r *http.Request, tenantID, incidentID string) {
	incidentID = strings.Trim(incidentID, "/")
	if incidentID == "" {
		api.WriteError(w, http.StatusBadRequest, "incident ID required")
		return
	}
	if r.Method != http.MethodPost {
		api.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var req models.DomainResolutionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		api.WriteError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if err := h.svc.RecordStructuredResolution(incidentID, tenantID, req); err != nil {
		api.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	api.WriteJSON(w, http.StatusCreated, map[string]string{"status": "recorded"})
}

// ── Remediation patterns ──────────────────────────────────────────────────────

func (h *DomainMemoryHandler) handleRemediations(w http.ResponseWriter, r *http.Request, tenantID, rest string) {
	id := memoryPathID(rest)

	// POST /remediations/{id}/outcome
	if id != "" && strings.HasSuffix(rest, "/outcome") {
		if r.Method != http.MethodPost {
			api.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		var body struct {
			Success bool `json:"success"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil && err != io.EOF {
			api.WriteError(w, http.StatusBadRequest, "invalid JSON body")
			return
		}
		if err := h.svc.MarkRemediationOutcome(tenantID, id, body.Success); err != nil {
			api.WriteError(w, http.StatusNotFound, err.Error())
			return
		}
		api.WriteJSON(w, http.StatusOK, map[string]string{"status": "updated"})
		return
	}

	// /remediations/{id}
	if id != "" {
		switch r.Method {
		case http.MethodPut:
			var p models.RemediationPattern
			if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
				api.WriteError(w, http.StatusBadRequest, "invalid JSON")
				return
			}
			if err := h.svc.UpdateRemediationPattern(tenantID, id, p); err != nil {
				api.WriteError(w, http.StatusNotFound, err.Error())
				return
			}
			api.WriteJSON(w, http.StatusOK, map[string]string{"status": "updated"})
		case http.MethodDelete:
			if err := h.svc.DeleteRemediationPattern(tenantID, id); err != nil {
				api.WriteError(w, http.StatusInternalServerError, err.Error())
				return
			}
			api.WriteJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
		default:
			api.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		}
		return
	}

	// /remediations  (list or create)
	switch r.Method {
	case http.MethodGet:
		service := r.URL.Query().Get("service")
		list, err := h.svc.ListRemediationPatterns(tenantID, service)
		if err != nil {
			api.WriteError(w, http.StatusInternalServerError, err.Error())
			return
		}
		api.WriteJSON(w, http.StatusOK, list)
	case http.MethodPost:
		var p models.RemediationPattern
		if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
			api.WriteError(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		created, err := h.svc.CreateRemediationPattern(tenantID, p)
		if err != nil {
			api.WriteError(w, http.StatusBadRequest, err.Error())
			return
		}
		api.WriteJSON(w, http.StatusCreated, created)
	default:
		api.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// ── Deploy signatures ─────────────────────────────────────────────────────────

func (h *DomainMemoryHandler) handleDeploySignatures(w http.ResponseWriter, r *http.Request, tenantID, rest string) {
	id := memoryPathID(rest)

	if id != "" {
		switch r.Method {
		case http.MethodPut:
			var d models.DeploySignature
			if err := json.NewDecoder(r.Body).Decode(&d); err != nil {
				api.WriteError(w, http.StatusBadRequest, "invalid JSON")
				return
			}
			if err := h.svc.UpdateDeploySignature(tenantID, id, d); err != nil {
				api.WriteError(w, http.StatusNotFound, err.Error())
				return
			}
			api.WriteJSON(w, http.StatusOK, map[string]string{"status": "updated"})
		case http.MethodDelete:
			if err := h.svc.DeleteDeploySignature(tenantID, id); err != nil {
				api.WriteError(w, http.StatusInternalServerError, err.Error())
				return
			}
			api.WriteJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
		default:
			api.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		}
		return
	}

	switch r.Method {
	case http.MethodGet:
		service := r.URL.Query().Get("service")
		list, err := h.svc.ListDeploySignatures(tenantID, service)
		if err != nil {
			api.WriteError(w, http.StatusInternalServerError, err.Error())
			return
		}
		api.WriteJSON(w, http.StatusOK, list)
	case http.MethodPost:
		var d models.DeploySignature
		if err := json.NewDecoder(r.Body).Decode(&d); err != nil {
			api.WriteError(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		created, err := h.svc.CreateDeploySignature(tenantID, d)
		if err != nil {
			api.WriteError(w, http.StatusBadRequest, err.Error())
			return
		}
		api.WriteJSON(w, http.StatusCreated, created)
	default:
		api.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// ── Runbook preferences ───────────────────────────────────────────────────────

func (h *DomainMemoryHandler) handleRunbookPreferences(w http.ResponseWriter, r *http.Request, tenantID, rest string) {
	id := memoryPathID(rest)

	if id != "" {
		switch r.Method {
		case http.MethodPut:
			var p models.RunbookPreference
			if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
				api.WriteError(w, http.StatusBadRequest, "invalid JSON")
				return
			}
			if err := h.svc.UpdateRunbookPreference(tenantID, id, p); err != nil {
				api.WriteError(w, http.StatusNotFound, err.Error())
				return
			}
			api.WriteJSON(w, http.StatusOK, map[string]string{"status": "updated"})
		case http.MethodDelete:
			if err := h.svc.DeleteRunbookPreference(tenantID, id); err != nil {
				api.WriteError(w, http.StatusInternalServerError, err.Error())
				return
			}
			api.WriteJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
		default:
			api.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		}
		return
	}

	switch r.Method {
	case http.MethodGet:
		team := r.URL.Query().Get("team")
		service := r.URL.Query().Get("service")
		list, err := h.svc.ListRunbookPreferences(tenantID, team, service)
		if err != nil {
			api.WriteError(w, http.StatusInternalServerError, err.Error())
			return
		}
		api.WriteJSON(w, http.StatusOK, list)
	case http.MethodPost:
		var p models.RunbookPreference
		if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
			api.WriteError(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		created, err := h.svc.CreateRunbookPreference(tenantID, p)
		if err != nil {
			api.WriteError(w, http.StatusBadRequest, err.Error())
			return
		}
		api.WriteJSON(w, http.StatusCreated, created)
	default:
		api.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// memoryPathID extracts the first path segment from a rest string like "/{id}/..."
func memoryPathID(rest string) string {
	rest = strings.Trim(rest, "/")
	if rest == "" {
		return ""
	}
	parts := strings.SplitN(rest, "/", 2)
	// Treat known collection names as non-IDs
	seg := parts[0]
	if seg == "outcome" || seg == "" {
		return ""
	}
	return seg
}
