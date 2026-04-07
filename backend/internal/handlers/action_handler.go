package handlers

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"ai-incident-platform/backend/internal/api"
	"ai-incident-platform/backend/internal/store"
)

type ActionHandler struct {
	auditStore *store.ActionAuditStore
}

func NewActionHandler(auditStore *store.ActionAuditStore) *ActionHandler {
	return &ActionHandler{auditStore: auditStore}
}

func (handler *ActionHandler) ExecuteAction(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		api.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	type request struct {
		ActionID   string `json:"action_id"`
		IncidentID string `json:"incident_id"`
		Approved   bool   `json:"approved"`
	}

	var req request

	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(&req); err != nil {
		api.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	req.ActionID = strings.TrimSpace(req.ActionID)
	req.IncidentID = strings.TrimSpace(req.IncidentID)

	if req.ActionID == "" {
		api.WriteError(w, http.StatusBadRequest, "action_id is required")
		return
	}

	if req.IncidentID == "" {
		api.WriteError(w, http.StatusBadRequest, "incident_id is required")
		return
	}

	executedAt := time.Now().UTC().Format(time.RFC3339)

	if handler.auditStore != nil {
		_ = handler.auditStore.AddAudit(store.ActionAudit{
			ActionID:   req.ActionID,
			IncidentID: req.IncidentID,
			Approved:   req.Approved,
			Status:     "executed",
			Message:    "action executed successfully",
			ExecutedAt: executedAt,
		})
	}

	result := map[string]interface{}{
		"action_id":    req.ActionID,
		"incident_id":  req.IncidentID,
		"approved":     req.Approved,
		"status":       "executed",
		"message":      "action executed successfully",
		"executed_at":  executedAt,
		"requires_ack": false,
	}

	api.WriteJSON(w, http.StatusOK, result)
}

func (handler *ActionHandler) GetActionAudit(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		api.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	incidentID := strings.TrimSpace(r.URL.Query().Get("incident_id"))
	if incidentID == "" {
		api.WriteError(w, http.StatusBadRequest, "incident_id is required")
		return
	}

	audits := []store.ActionAudit{}
	if handler.auditStore != nil {
		audits = handler.auditStore.GetAuditsByIncident(incidentID)
	}

	api.WriteJSON(w, http.StatusOK, audits)
}

var defaultActionAuditStore *store.ActionAuditStore

func SetDefaultActionAuditStore(auditStore *store.ActionAuditStore) {
	defaultActionAuditStore = auditStore
}

func ExecuteActionHandler(w http.ResponseWriter, r *http.Request) {
	handler := NewActionHandler(defaultActionAuditStore)
	handler.ExecuteAction(w, r)
}

func GetActionAuditHandler(w http.ResponseWriter, r *http.Request) {
	handler := NewActionHandler(defaultActionAuditStore)
	handler.GetActionAudit(w, r)
}
