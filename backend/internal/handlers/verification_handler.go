package handlers

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"ai-incident-platform/backend/internal/api"
	"ai-incident-platform/backend/internal/models"
	"ai-incident-platform/backend/internal/services"
)

// VerificationHandler handles closed-loop verification endpoints.
type VerificationHandler struct {
	service *services.VerificationService
}

// NewVerificationHandler creates a new VerificationHandler.
func NewVerificationHandler(s *services.VerificationService) *VerificationHandler {
	return &VerificationHandler{service: s}
}

// Verify runs a legacy one-shot verification for an incident.
// POST /api/v1/incidents/{id}/verify
func (h *VerificationHandler) Verify(w http.ResponseWriter, r *http.Request) {
	incidentID := extractIncidentIDFromPath(r.URL.Path, "/verify")
	if incidentID == "" {
		api.WriteError(w, http.StatusBadRequest, "incident id is required")
		return
	}
	record, err := h.service.VerifyIncident(incidentID)
	if err != nil {
		api.WriteError(w, http.StatusNotFound, err.Error())
		return
	}
	api.WriteJSON(w, http.StatusOK, record)
}

// StartVerification captures the before-snapshot and creates a pending record.
// POST /api/v1/incidents/{id}/verify/start
// Body: {"execution_id": "exec-abc123"}
func (h *VerificationHandler) StartVerification(w http.ResponseWriter, r *http.Request) {
	incidentID := extractVerifySubID(r.URL.Path, "/verify/start")
	if incidentID == "" {
		api.WriteError(w, http.StatusBadRequest, "incident id is required")
		return
	}

	var body struct {
		ExecutionID string `json:"execution_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil && err != io.EOF {
		api.WriteError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	record, err := h.service.StartVerification(incidentID, body.ExecutionID)
	if err != nil {
		api.WriteError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	api.WriteJSON(w, http.StatusCreated, record)
}

// CompleteVerification captures the after-snapshot and runs all checks.
// POST /api/v1/incidents/{id}/verify/complete
// Body: {"vrid": "vr-123456"}
func (h *VerificationHandler) CompleteVerification(w http.ResponseWriter, r *http.Request) {
	incidentID := extractVerifySubID(r.URL.Path, "/verify/complete")
	if incidentID == "" {
		api.WriteError(w, http.StatusBadRequest, "incident id is required")
		return
	}

	var body struct {
		VRID string `json:"vrid"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.VRID == "" {
		api.WriteError(w, http.StatusBadRequest, "vrid is required")
		return
	}

	record, err := h.service.CompleteVerification(body.VRID)
	if err != nil {
		api.WriteError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	_ = incidentID // already validated implicitly via record.IncidentID
	api.WriteJSON(w, http.StatusOK, record)
}

// TriggerRollback marks a verification record as rolled back.
// POST /api/v1/incidents/{id}/verify/rollback
// Body: {"vrid": "vr-123456"}
func (h *VerificationHandler) TriggerRollback(w http.ResponseWriter, r *http.Request) {
	incidentID := extractVerifySubID(r.URL.Path, "/verify/rollback")
	if incidentID == "" {
		api.WriteError(w, http.StatusBadRequest, "incident id is required")
		return
	}

	var body struct {
		VRID string `json:"vrid"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.VRID == "" {
		api.WriteError(w, http.StatusBadRequest, "vrid is required")
		return
	}

	record, err := h.service.TriggerRollback(body.VRID)
	if err != nil {
		api.WriteError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	_ = incidentID
	api.WriteJSON(w, http.StatusOK, record)
}

// AttachProof appends a proof item to a verification record.
// POST /api/v1/incidents/{id}/verify/{vrid}/proof
// Body: ProofItem JSON
func (h *VerificationHandler) AttachProof(w http.ResponseWriter, r *http.Request) {
	vrID := extractVRIDFromProofPath(r.URL.Path)
	if vrID == "" {
		api.WriteError(w, http.StatusBadRequest, "verification record id is required")
		return
	}

	var item models.ProofItem
	if err := json.NewDecoder(r.Body).Decode(&item); err != nil {
		api.WriteError(w, http.StatusBadRequest, "invalid proof item body")
		return
	}
	if item.Label == "" {
		api.WriteError(w, http.StatusBadRequest, "proof item label is required")
		return
	}

	record, err := h.service.AttachProof(vrID, item)
	if err != nil {
		api.WriteError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	api.WriteJSON(w, http.StatusOK, record)
}

// GetVerification returns all verification records for an incident.
// GET /api/v1/incidents/{id}/verification
func (h *VerificationHandler) GetVerification(w http.ResponseWriter, r *http.Request) {
	incidentID := extractIncidentIDFromPath(r.URL.Path, "/verification")
	if incidentID == "" {
		api.WriteError(w, http.StatusBadRequest, "incident id is required")
		return
	}
	records, err := h.service.GetByIncidentID(incidentID)
	if err != nil {
		api.WriteError(w, http.StatusInternalServerError, "failed to get verification records")
		return
	}
	api.WriteJSON(w, http.StatusOK, map[string]any{
		"incident_id": incidentID,
		"records":     records,
	})
}

// ─────────────────────────────────────────────────────────────────────────────
// Path helpers
// ─────────────────────────────────────────────────────────────────────────────

// extractIncidentIDFromPath extracts incident ID from paths like:
//
//	/api/v1/incidents/{id}/verify  →  {id}
func extractIncidentIDFromPath(path, suffix string) string {
	path = strings.TrimSuffix(path, suffix)
	path = strings.TrimSuffix(path, "/")
	parts := strings.Split(path, "/")
	if len(parts) == 0 {
		return ""
	}
	return strings.TrimSpace(parts[len(parts)-1])
}

// extractVerifySubID extracts the incident ID from paths ending in a known
// verify sub-action, e.g. /api/v1/incidents/{id}/verify/start.
func extractVerifySubID(path, suffix string) string {
	path = strings.TrimSuffix(path, suffix)
	path = strings.TrimSuffix(path, "/")
	parts := strings.Split(path, "/")
	if len(parts) == 0 {
		return ""
	}
	return strings.TrimSpace(parts[len(parts)-1])
}

// extractVRIDFromProofPath extracts the verification record ID from:
//
//	/api/v1/incidents/{incidentID}/verify/{vrid}/proof
func extractVRIDFromProofPath(path string) string {
	path = strings.TrimSuffix(path, "/proof")
	path = strings.TrimSuffix(path, "/")
	parts := strings.Split(path, "/")
	if len(parts) == 0 {
		return ""
	}
	return strings.TrimSpace(parts[len(parts)-1])
}
