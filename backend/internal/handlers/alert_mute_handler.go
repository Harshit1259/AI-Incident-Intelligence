package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"ai-incident-platform/backend/internal/api"
	"ai-incident-platform/backend/internal/middleware"
	"ai-incident-platform/backend/internal/models"
	"ai-incident-platform/backend/internal/services"
)

// AlertMuteHandler serves /api/v1/mutes. Route registration decides who may
// call what: reads are open to any signed-in user, writes are admin-only.
type AlertMuteHandler struct {
	svc *services.AlertMuteService
}

func NewAlertMuteHandler(svc *services.AlertMuteService) *AlertMuteHandler {
	return &AlertMuteHandler{svc: svc}
}

// HandleList handles GET /api/v1/mutes.
func (h *AlertMuteHandler) HandleList(w http.ResponseWriter, r *http.Request) {
	list, err := h.svc.List(middleware.TenantFromRequest(r))
	if err != nil {
		api.WriteError(w, http.StatusInternalServerError, "could not load mutes")
		return
	}
	api.WriteJSON(w, http.StatusOK, list)
}

// HandleCreate handles POST /api/v1/mutes.
func (h *AlertMuteHandler) HandleCreate(w http.ResponseWriter, r *http.Request) {
	var req models.AlertMuteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		api.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	tenantID, userID := tenantAndUser(r)
	mute, err := h.svc.Create(req, tenantID, userID)
	if err != nil {
		writeMuteError(w, err)
		return
	}
	api.WriteJSON(w, http.StatusCreated, mute)
}

// HandlePreview handles POST /api/v1/mutes/preview.
func (h *AlertMuteHandler) HandlePreview(w http.ResponseWriter, r *http.Request) {
	var req models.AlertMuteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		api.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	preview, err := h.svc.Preview(req, middleware.TenantFromRequest(r))
	if err != nil {
		writeMuteError(w, err)
		return
	}
	api.WriteJSON(w, http.StatusOK, preview)
}

// HandleSuggest handles GET /api/v1/mutes/suggest?event_id=… or ?incident_id=….
func (h *AlertMuteHandler) HandleSuggest(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	sg, err := h.svc.Suggest(middleware.TenantFromRequest(r),
		strings.TrimSpace(q.Get("event_id")), strings.TrimSpace(q.Get("incident_id")))
	if err != nil {
		writeMuteError(w, err)
		return
	}
	api.WriteJSON(w, http.StatusOK, sg)
}

// HandleLog handles GET /api/v1/mutes/log?mute_id=…&limit=…&offset=….
func (h *AlertMuteHandler) HandleLog(w http.ResponseWriter, r *http.Request) {
	limit, offset := parsePagination(r, 50, 200)
	entries, err := h.svc.Log(middleware.TenantFromRequest(r),
		strings.TrimSpace(r.URL.Query().Get("mute_id")), limit, offset)
	if err != nil {
		api.WriteError(w, http.StatusInternalServerError, "could not load muted alerts")
		return
	}
	api.WriteJSON(w, http.StatusOK, map[string]any{"items": entries})
}

// HandleUnmute handles POST /api/v1/mutes/{id}/unmute.
func (h *AlertMuteHandler) HandleUnmute(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		api.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	rest := strings.TrimPrefix(r.URL.Path, "/api/v1/mutes/")
	id, action, ok := strings.Cut(rest, "/")
	if !ok || action != "unmute" || id == "" {
		api.WriteError(w, http.StatusNotFound, "not found")
		return
	}
	tenantID, userID := tenantAndUser(r)
	if err := h.svc.Unmute(tenantID, id, userID); err != nil {
		writeMuteError(w, err)
		return
	}
	api.WriteJSON(w, http.StatusOK, map[string]string{"status": models.MuteStatusUnmuted})
}

// tenantAndUser reads both from the verified JWT, never from request headers.
func tenantAndUser(r *http.Request) (string, string) {
	tenantID := middleware.TenantFromRequest(r)
	userID := ""
	if claims, ok := middleware.ClaimsFromContext(r); ok {
		userID = claims.UserID
	}
	return tenantID, userID
}

func writeMuteError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, services.ErrInvalidMute):
		api.WriteError(w, http.StatusBadRequest, strings.TrimPrefix(err.Error(), services.ErrInvalidMute.Error()+": "))
	case errors.Is(err, services.ErrMuteSourceNotFound):
		api.WriteError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, services.ErrMuteNotActive):
		api.WriteError(w, http.StatusNotFound, err.Error())
	default:
		api.WriteError(w, http.StatusInternalServerError, "could not complete the request")
	}
}
