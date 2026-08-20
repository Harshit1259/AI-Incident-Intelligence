package handlers

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"ai-incident-platform/backend/internal/api"
	"ai-incident-platform/backend/internal/middleware"
	"ai-incident-platform/backend/internal/models"
	"ai-incident-platform/backend/internal/store"
)

// ActionExecutionHandler handles action execution lifecycle endpoints.
type ActionExecutionHandler struct {
	store *store.ActionExecutionStore
}

// NewActionExecutionHandler creates a new ActionExecutionHandler.
func NewActionExecutionHandler(s *store.ActionExecutionStore) *ActionExecutionHandler {
	return &ActionExecutionHandler{store: s}
}

// HandleGetExecutions handles GET /api/v1/incidents/{id}/executions.
func (h *ActionExecutionHandler) HandleGetExecutions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		api.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	incidentID := extractIncidentIDForExec(r.URL.Path)
	if incidentID == "" {
		api.WriteError(w, http.StatusBadRequest, "incident id is required")
		return
	}

	executions, err := h.store.GetByIncidentID(incidentID)
	if err != nil {
		api.WriteError(w, http.StatusInternalServerError, "failed to get executions")
		return
	}

	if executions == nil {
		executions = []models.ActionExecution{}
	}

	api.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"incident_id": incidentID,
		"executions":  executions,
	})
}

// HandleRecordExecution handles POST /api/v1/incidents/{id}/executions.
// Records a new action execution for an incident. If the action requires
// approval, the execution is created in pending_approval status.
func (h *ActionExecutionHandler) HandleRecordExecution(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		api.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	incidentID := extractIncidentIDForExec(r.URL.Path)
	if incidentID == "" {
		api.WriteError(w, http.StatusBadRequest, "incident id is required")
		return
	}

	var body struct {
		ActionID         string `json:"action_id"`
		ActionLabel      string `json:"action_label"`
		ActionDesc       string `json:"action_desc"`
		RequiresApproval bool   `json:"requires_approval"`
		ExecutionMode    string `json:"execution_mode"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		api.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if body.ActionID == "" {
		api.WriteError(w, http.StatusBadRequest, "action_id is required")
		return
	}

	claims, _ := middleware.ClaimsFromContext(r)
	tenantID := middleware.TenantFromRequest(r)
	if claims.TenantID != "" {
		tenantID = claims.TenantID
	}

	execMode := body.ExecutionMode
	if execMode == "" {
		execMode = "manual"
	}

	status := "planned"
	approvalStatus := ""
	if body.RequiresApproval {
		status = "pending_approval"
		approvalStatus = "pending"
	}

	exec := models.ActionExecution{
		ID:               fmt.Sprintf("exec-%d", time.Now().UnixNano()),
		TenantID:         tenantID,
		IncidentID:       incidentID,
		ActionID:         body.ActionID,
		ActionLabel:      body.ActionLabel,
		ActionDesc:       body.ActionDesc,
		ChainPosition:    0,
		Status:           status,
		ExecutionMode:    execMode,
		RequiresApproval: body.RequiresApproval,
		ApprovalStatus:   approvalStatus,
		CreatedAt:        time.Now(),
	}

	if err := h.store.Create(exec); err != nil {
		api.WriteError(w, http.StatusInternalServerError, "failed to record execution")
		return
	}

	api.WriteJSON(w, http.StatusCreated, exec)
}

// HandleApprove handles POST /api/v1/executions/{execId}/approve.
func (h *ActionExecutionHandler) HandleApprove(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		api.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	execID := extractExecID(r.URL.Path, "/approve")
	if execID == "" {
		api.WriteError(w, http.StatusBadRequest, "execution id is required")
		return
	}

	exec, err := h.store.GetByID(execID)
	if err != nil || exec == nil {
		api.WriteError(w, http.StatusNotFound, "execution not found")
		return
	}
	if exec.ApprovalStatus != "pending" {
		api.WriteError(w, http.StatusConflict, "execution is not pending approval")
		return
	}

	claims, _ := middleware.ClaimsFromContext(r)
	approver := claims.UserID
	if approver == "" {
		approver = "unknown"
	}

	if err := h.store.Approve(execID, approver, time.Now()); err != nil {
		api.WriteError(w, http.StatusInternalServerError, "failed to approve execution")
		return
	}

	api.WriteJSON(w, http.StatusOK, map[string]string{
		"status":      "approved",
		"approved_by": approver,
	})
}

// HandleReject handles POST /api/v1/executions/{execId}/reject.
func (h *ActionExecutionHandler) HandleReject(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		api.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	execID := extractExecID(r.URL.Path, "/reject")
	if execID == "" {
		api.WriteError(w, http.StatusBadRequest, "execution id is required")
		return
	}

	exec, err := h.store.GetByID(execID)
	if err != nil || exec == nil {
		api.WriteError(w, http.StatusNotFound, "execution not found")
		return
	}
	if exec.ApprovalStatus != "pending" {
		api.WriteError(w, http.StatusConflict, "execution is not pending approval")
		return
	}

	var body struct {
		Reason string `json:"reason"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil && err != io.EOF {
		api.WriteError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	claims, _ := middleware.ClaimsFromContext(r)
	rejector := claims.UserID
	if rejector == "" {
		rejector = "unknown"
	}

	if err := h.store.Reject(execID, rejector, body.Reason); err != nil {
		api.WriteError(w, http.StatusInternalServerError, "failed to reject execution")
		return
	}

	api.WriteJSON(w, http.StatusOK, map[string]string{"status": "rejected"})
}

// HandleRollback handles POST /api/v1/executions/{execId}/rollback.
func (h *ActionExecutionHandler) HandleRollback(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		api.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	execID := extractExecID(r.URL.Path, "/rollback")
	if execID == "" {
		api.WriteError(w, http.StatusBadRequest, "execution id is required")
		return
	}

	exec, err := h.store.GetByID(execID)
	if err != nil || exec == nil {
		api.WriteError(w, http.StatusNotFound, "execution not found")
		return
	}
	if exec.Status == "rolled_back" {
		api.WriteError(w, http.StatusConflict, "execution already rolled back")
		return
	}

	var body struct {
		Reason      string `json:"reason"`
		RollbackRef string `json:"rollback_ref"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil && err != io.EOF {
		api.WriteError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	claims, _ := middleware.ClaimsFromContext(r)
	actor := claims.UserID
	if actor == "" {
		actor = "unknown"
	}

	if err := h.store.Rollback(execID, actor, body.Reason, body.RollbackRef); err != nil {
		api.WriteError(w, http.StatusInternalServerError, "failed to record rollback")
		return
	}

	api.WriteJSON(w, http.StatusOK, map[string]string{
		"status":       "rolled_back",
		"rolled_back_by": actor,
	})
}

// ─────────────────────────────────────────────────────
// Path helpers
// ─────────────────────────────────────────────────────

func extractIncidentIDForExec(path string) string {
	path = strings.TrimSuffix(path, "/executions")
	path = strings.TrimSuffix(path, "/")
	parts := strings.Split(path, "/")
	if len(parts) == 0 {
		return ""
	}
	return strings.TrimSpace(parts[len(parts)-1])
}

// extractExecID parses /api/v1/executions/{id}/approve → id.
func extractExecID(path, suffix string) string {
	path = strings.TrimSuffix(path, suffix)
	path = strings.TrimSuffix(path, "/")
	parts := strings.Split(path, "/")
	if len(parts) == 0 {
		return ""
	}
	return strings.TrimSpace(parts[len(parts)-1])
}
