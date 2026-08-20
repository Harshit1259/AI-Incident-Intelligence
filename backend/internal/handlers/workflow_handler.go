package handlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"ai-incident-platform/backend/internal/api"
	"ai-incident-platform/backend/internal/middleware"
	"ai-incident-platform/backend/internal/models"
	"ai-incident-platform/backend/internal/services"
)

// WorkflowHandler exposes all Team Workflow Primitive endpoints.
//
// Routes (under /api/v1/workflow):
//   POST   /api/v1/incidents/{id}/commander          — assign commander
//   GET    /api/v1/incidents/{id}/commander          — current commander
//   GET    /api/v1/incidents/{id}/commander/history  — assignment history
//   GET    /api/v1/workflow/templates                — list templates
//   POST   /api/v1/workflow/templates                — create template
//   PUT    /api/v1/workflow/templates/{id}           — update template
//   DELETE /api/v1/workflow/templates/{id}           — delete template
//   POST   /api/v1/workflow/templates/{id}/render    — render template for incident
//   POST   /api/v1/incidents/{id}/tickets            — create JIRA/SNOW ticket
//   GET    /api/v1/incidents/{id}/tickets            — list tickets for incident
//   POST   /api/v1/incidents/{id}/tickets/{provider}/sync — sync ticket status
//   POST   /api/v1/incidents/{id}/exec-summary       — generate executive summary
//   POST   /api/v1/incidents/{id}/status-comms       — publish status communication
//   GET    /api/v1/incidents/{id}/status-comms       — list status communications
//   POST   /api/v1/teams/webhook                     — Teams outgoing webhook
type WorkflowHandler struct {
	svc         *services.WorkflowService
	teamsSvc    *services.TeamsService
	incidentSvc *services.IncidentService
}

func NewWorkflowHandler(wf *services.WorkflowService, teams *services.TeamsService, inc *services.IncidentService) *WorkflowHandler {
	return &WorkflowHandler{svc: wf, teamsSvc: teams, incidentSvc: inc}
}

// ── Commander ─────────────────────────────────────────────────────────────────

func (h *WorkflowHandler) HandleCommander(w http.ResponseWriter, r *http.Request) {
	tenantID := middleware.TenantFromRequest(r)
	incidentID := wfSegment(r.URL.Path, "incidents", 1)
	if incidentID == "" {
		api.WriteError(w, http.StatusBadRequest, "incident ID required")
		return
	}

	switch {
	case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/history"):
		h.commanderHistory(w, r, tenantID, incidentID)
	case r.Method == http.MethodGet:
		h.currentCommander(w, r, tenantID, incidentID)
	case r.Method == http.MethodPost:
		h.assignCommander(w, r, tenantID, incidentID)
	default:
		api.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (h *WorkflowHandler) assignCommander(w http.ResponseWriter, r *http.Request, tenantID, incidentID string) {
	var body struct {
		UserID    string `json:"user_id"`
		UserName  string `json:"user_name"`
		UserEmail string `json:"user_email"`
		Notes     string `json:"notes"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		api.WriteError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if body.UserName == "" {
		api.WriteError(w, http.StatusBadRequest, "user_name is required")
		return
	}

	claims, _ := middleware.ClaimsFromContext(r)
	assignedBy := claims.UserID
	if assignedBy == "" {
		assignedBy = "system"
	}

	c := &models.IncidentCommander{
		ID:         fmt.Sprintf("cmd-%d", time.Now().UnixNano()),
		TenantID:   tenantID,
		IncidentID: incidentID,
		UserID:     body.UserID,
		UserName:   body.UserName,
		UserEmail:  body.UserEmail,
		AssignedBy: assignedBy,
		Notes:      body.Notes,
	}

	if err := h.svc.AssignCommander(c); err != nil {
		api.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	api.WriteJSON(w, http.StatusCreated, c)
}

func (h *WorkflowHandler) currentCommander(w http.ResponseWriter, r *http.Request, tenantID, incidentID string) {
	c, err := h.svc.CurrentCommander(tenantID, incidentID)
	if err != nil {
		api.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if c == nil {
		api.WriteJSON(w, http.StatusOK, map[string]string{"status": "unassigned"})
		return
	}
	api.WriteJSON(w, http.StatusOK, c)
}

func (h *WorkflowHandler) commanderHistory(w http.ResponseWriter, r *http.Request, tenantID, incidentID string) {
	history, err := h.svc.CommanderHistory(tenantID, incidentID)
	if err != nil {
		api.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if history == nil {
		history = []models.IncidentCommander{}
	}
	api.WriteJSON(w, http.StatusOK, history)
}

// ── Templates ─────────────────────────────────────────────────────────────────

func (h *WorkflowHandler) HandleTemplates(w http.ResponseWriter, r *http.Request) {
	tenantID := middleware.TenantFromRequest(r)
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/workflow/templates")
	path = strings.TrimSuffix(path, "/")

	switch {
	case path == "" || path == "/":
		switch r.Method {
		case http.MethodGet:
			h.listTemplates(w, r, tenantID)
		case http.MethodPost:
			h.createTemplate(w, r, tenantID)
		default:
			api.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		}
	case strings.HasSuffix(path, "/render"):
		id := strings.TrimSuffix(strings.TrimPrefix(path, "/"), "/render")
		if r.Method != http.MethodPost {
			api.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		h.renderTemplate(w, r, tenantID, id)
	default:
		id := strings.TrimPrefix(path, "/")
		switch r.Method {
		case http.MethodPut:
			h.updateTemplate(w, r, tenantID, id)
		case http.MethodDelete:
			h.deleteTemplate(w, r, tenantID, id)
		default:
			api.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		}
	}
}

func (h *WorkflowHandler) listTemplates(w http.ResponseWriter, _ *http.Request, tenantID string) {
	templates, err := h.svc.ListTemplates(tenantID)
	if err != nil {
		api.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if templates == nil {
		templates = []models.StakeholderTemplate{}
	}
	api.WriteJSON(w, http.StatusOK, templates)
}

func (h *WorkflowHandler) createTemplate(w http.ResponseWriter, r *http.Request, tenantID string) {
	var t models.StakeholderTemplate
	if err := json.NewDecoder(r.Body).Decode(&t); err != nil {
		api.WriteError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	t.TenantID = tenantID
	t.ID = ""
	if err := h.svc.CreateTemplate(&t); err != nil {
		api.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	api.WriteJSON(w, http.StatusCreated, t)
}

func (h *WorkflowHandler) updateTemplate(w http.ResponseWriter, r *http.Request, tenantID, id string) {
	var t models.StakeholderTemplate
	if err := json.NewDecoder(r.Body).Decode(&t); err != nil {
		api.WriteError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	t.ID = id
	t.TenantID = tenantID
	if err := h.svc.UpdateTemplate(&t); err != nil {
		api.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	api.WriteJSON(w, http.StatusOK, t)
}

func (h *WorkflowHandler) deleteTemplate(w http.ResponseWriter, r *http.Request, tenantID, id string) {
	if err := h.svc.DeleteTemplate(id, tenantID); err != nil {
		if err == sql.ErrNoRows {
			api.WriteError(w, http.StatusNotFound, "template not found")
			return
		}
		api.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	api.WriteJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (h *WorkflowHandler) renderTemplate(w http.ResponseWriter, r *http.Request, tenantID, templateID string) {
	var body struct {
		IncidentID string `json:"incident_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		api.WriteError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	rendered, err := h.svc.RenderTemplate(tenantID, templateID, body.IncidentID)
	if err != nil {
		api.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	api.WriteJSON(w, http.StatusOK, rendered)
}

// ── Tickets ───────────────────────────────────────────────────────────────────

func (h *WorkflowHandler) HandleTickets(w http.ResponseWriter, r *http.Request) {
	tenantID := middleware.TenantFromRequest(r)
	incidentID := wfSegment(r.URL.Path, "incidents", 1)

	// /api/v1/incidents/{id}/tickets/{provider}/sync
	if strings.HasSuffix(r.URL.Path, "/sync") && r.Method == http.MethodPost {
		provider := wfSegment(r.URL.Path, "tickets", 1)
		if err := h.svc.SyncTicketStatus(tenantID, incidentID, provider); err != nil {
			api.WriteError(w, http.StatusBadRequest, err.Error())
			return
		}
		api.WriteJSON(w, http.StatusOK, map[string]string{"status": "synced"})
		return
	}

	switch r.Method {
	case http.MethodGet:
		syncs, err := h.svc.GetTicketSyncs(tenantID, incidentID)
		if err != nil {
			api.WriteError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if syncs == nil {
			syncs = []models.TicketSync{}
		}
		api.WriteJSON(w, http.StatusOK, syncs)
	case http.MethodPost:
		var req models.TicketCreateRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			api.WriteError(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		req.IncidentID = incidentID
		ts, err := h.svc.CreateTicket(tenantID, req)
		if err != nil {
			api.WriteError(w, http.StatusBadRequest, err.Error())
			return
		}
		api.WriteJSON(w, http.StatusCreated, ts)
	default:
		api.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// ── Executive Summary ─────────────────────────────────────────────────────────

func (h *WorkflowHandler) HandleExecSummary(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		api.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	tenantID := middleware.TenantFromRequest(r)
	incidentID := wfSegment(r.URL.Path, "incidents", 1)
	summary, err := h.svc.GenerateExecSummary(tenantID, incidentID)
	if err != nil {
		api.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	api.WriteJSON(w, http.StatusOK, summary)
}

// ── Status Communications ─────────────────────────────────────────────────────

func (h *WorkflowHandler) HandleStatusComms(w http.ResponseWriter, r *http.Request) {
	tenantID := middleware.TenantFromRequest(r)
	incidentID := wfSegment(r.URL.Path, "incidents", 1)

	switch r.Method {
	case http.MethodGet:
		comms, err := h.svc.GetStatusCommunications(tenantID, incidentID)
		if err != nil {
			api.WriteError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if comms == nil {
			comms = []models.StatusCommunication{}
		}
		api.WriteJSON(w, http.StatusOK, comms)
	case http.MethodPost:
		var sc models.StatusCommunication
		if err := json.NewDecoder(r.Body).Decode(&sc); err != nil {
			api.WriteError(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		sc.IncidentID = incidentID
		sc.TenantID = tenantID
		claims, _ := middleware.ClaimsFromContext(r)
		if sc.PublishedBy == "" {
			sc.PublishedBy = claims.UserID
		}
		if err := h.svc.PublishStatusUpdate(&sc); err != nil {
			api.WriteError(w, http.StatusInternalServerError, err.Error())
			return
		}
		api.WriteJSON(w, http.StatusCreated, sc)
	default:
		api.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// ── Teams outgoing webhook ────────────────────────────────────────────────────

func (h *WorkflowHandler) HandleTeamsWebhook(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		api.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var body struct {
		Text string `json:"text"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		api.WriteError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if h.teamsSvc == nil {
		api.WriteError(w, http.StatusServiceUnavailable, "Teams not configured")
		return
	}
	replyText, err := h.teamsSvc.HandleOutgoingWebhook(body.Text)
	if err != nil {
		api.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	// Teams expects a JSON card or a plain text response.
	api.WriteJSON(w, http.StatusOK, map[string]string{
		"type": "message",
		"text": replyText,
	})
}

// ── path helper ───────────────────────────────────────────────────────────────

// wfSegment returns the path segment that comes N positions after the key segment.
// e.g. wfSegment("/api/v1/incidents/INC123/commander", "incidents", 1) → "INC123"
func wfSegment(path, key string, offset int) string {
	parts := strings.Split(path, "/")
	for i, p := range parts {
		if p == key && i+offset < len(parts) {
			return parts[i+offset]
		}
	}
	return ""
}
