package handlers

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"ai-incident-platform/backend/internal/api"
	"ai-incident-platform/backend/internal/middleware"
	"ai-incident-platform/backend/internal/models"
	"ai-incident-platform/backend/internal/store"
)

// EnrollmentHandler manages the lifecycle of agent enrollment tokens.
// All routes require a valid JWT — creating tokens is an operator-level action.
type EnrollmentHandler struct {
	enrollmentStore *store.AgentEnrollmentStore
}

// NewEnrollmentHandler creates a new EnrollmentHandler.
func NewEnrollmentHandler(es *store.AgentEnrollmentStore) *EnrollmentHandler {
	return &EnrollmentHandler{enrollmentStore: es}
}

// HandleCollection dispatches GET (list) and POST (create) for /api/v1/agents/enrollment-tokens.
func (h *EnrollmentHandler) HandleCollection(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.list(w, r)
	case http.MethodPost:
		h.create(w, r)
	default:
		api.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// HandleItem dispatches DELETE for /api/v1/agents/enrollment-tokens/{id}.
func (h *EnrollmentHandler) HandleItem(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodDelete:
		h.revoke(w, r)
	default:
		api.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (h *EnrollmentHandler) create(w http.ResponseWriter, r *http.Request) {
	claims, ok := middleware.ClaimsFromContext(r)
	if !ok {
		api.WriteError(w, http.StatusUnauthorized, "missing auth claims")
		return
	}

	var req models.CreateEnrollmentTokenRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		api.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	ttl := req.TTLHours
	if ttl <= 0 {
		ttl = 24
	}
	if ttl > 720 { // cap at 30 days
		ttl = 720
	}

	// Generate raw token — 32 bytes → 64 hex chars prefixed with "enroll_".
	rawBytes := make([]byte, 32)
	if _, err := rand.Read(rawBytes); err != nil {
		slog.ErrorContext(r.Context(), "enrollment_handler: rand read failed", "error", err)
		api.WriteError(w, http.StatusInternalServerError, "failed to generate token")
		return
	}
	rawToken := "enroll_" + hex.EncodeToString(rawBytes)

	// Build a short unique ID from the first 8 bytes of the token.
	tokenID := "etok_" + hex.EncodeToString(rawBytes[:8])

	token := models.EnrollmentToken{
		ID:        tokenID,
		TenantID:  claims.TenantID,
		Label:     req.Label,
		ExpiresAt: time.Now().Add(time.Duration(ttl) * time.Hour),
		CreatedBy: claims.UserID,
		CreatedAt: time.Now(),
	}

	if err := h.enrollmentStore.Create(token, rawToken); err != nil {
		slog.ErrorContext(r.Context(), "enrollment_handler: create token failed", "error", err)
		api.WriteError(w, http.StatusInternalServerError, "failed to create enrollment token")
		return
	}

	// Attach the raw token to the response — this is the only time it is visible.
	token.RawToken = rawToken
	api.WriteJSON(w, http.StatusCreated, token)
}

func (h *EnrollmentHandler) list(w http.ResponseWriter, r *http.Request) {
	claims, ok := middleware.ClaimsFromContext(r)
	if !ok {
		api.WriteError(w, http.StatusUnauthorized, "missing auth claims")
		return
	}

	tokens, err := h.enrollmentStore.ListByTenant(claims.TenantID)
	if err != nil {
		slog.ErrorContext(r.Context(), "enrollment_handler: list failed", "error", err)
		api.WriteError(w, http.StatusInternalServerError, "failed to list enrollment tokens")
		return
	}
	if tokens == nil {
		tokens = []models.EnrollmentToken{}
	}
	api.WriteJSON(w, http.StatusOK, tokens)
}

func (h *EnrollmentHandler) revoke(w http.ResponseWriter, r *http.Request) {
	claims, ok := middleware.ClaimsFromContext(r)
	if !ok {
		api.WriteError(w, http.StatusUnauthorized, "missing auth claims")
		return
	}

	id := extractEnrollmentTokenID(r.URL.Path)
	if id == "" {
		api.WriteError(w, http.StatusBadRequest, "enrollment token id is required")
		return
	}

	if err := h.enrollmentStore.Delete(id, claims.TenantID); err != nil {
		slog.ErrorContext(r.Context(), "enrollment_handler: delete failed", "token_id", id, "error", err)
		api.WriteError(w, http.StatusInternalServerError, "failed to revoke enrollment token")
		return
	}

	api.WriteJSON(w, http.StatusOK, map[string]string{"status": "revoked", "id": id})
}

// extractEnrollmentTokenID extracts the token ID from /api/v1/agents/enrollment-tokens/{id}.
func extractEnrollmentTokenID(path string) string {
	path = strings.TrimSuffix(path, "/")
	parts := strings.Split(path, "/")
	if len(parts) < 1 {
		return ""
	}
	return parts[len(parts)-1]
}
