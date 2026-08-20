package handlers

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"ai-incident-platform/backend/internal/api"
	"ai-incident-platform/backend/internal/middleware"
	"ai-incident-platform/backend/internal/models"
	"ai-incident-platform/backend/internal/services"
	"ai-incident-platform/backend/internal/store"
)

// AgentHandler handles HTTP requests for NeuroOps agent management and ingestion.
type AgentHandler struct {
	agentStore       *store.AgentStore
	enrollStore      *store.AgentEnrollmentStore
	ingestionService *services.AgentIngestionService
	secretKey        []byte // AES-256 key for encrypting/decrypting per-agent secrets
	devToken         string // legacy AGENT_INGEST_TOKEN; non-empty enables dev fallback
	isProd           bool   // when true, open-dev registration is rejected
}

// NewAgentHandler creates a new AgentHandler.
func NewAgentHandler(
	as *store.AgentStore,
	es *store.AgentEnrollmentStore,
	is *services.AgentIngestionService,
	secretKey []byte,
	devToken string,
) *AgentHandler {
	return &AgentHandler{
		agentStore:       as,
		enrollStore:      es,
		ingestionService: is,
		secretKey:        secretKey,
		devToken:         devToken,
	}
}

// SetProdMode marks the handler as running in production.
// When true, open-dev registration (no credential, no enrollment token) is rejected.
// Call this from main before serving when cfg.IsProd == true.
func (h *AgentHandler) SetProdMode(isProd bool) { h.isProd = isProd }

// ─── Registration ────────────────────────────────────────────────────────────

// Register handles POST /api/v1/agents/register.
//
// Production path: agent must present a valid, unused, non-expired enrollment
// token in X-Enrollment-Token.  On success the response carries a one-time
// AgentSecret the agent must use to sign all future ingest requests.
//
// Dev fallback: if the legacy AGENT_INGEST_TOKEN is set and the request carries
// a matching X-Agent-Token, the enrollment-token requirement is skipped (old
// behaviour, dev-only).  When neither token is configured the server is in open
// dev mode and registration proceeds without any credential check.
func (h *AgentHandler) Register(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		api.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var reg models.AgentRegistration
	if err := json.NewDecoder(r.Body).Decode(&reg); err != nil {
		api.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if reg.AgentID == "" {
		api.WriteError(w, http.StatusBadRequest, "agent_id is required")
		return
	}

	// Determine tenant and whether the caller is authenticated.
	tenantID, enrolledVia, authOK := h.resolveRegistrationAuth(r, reg.AgentID)
	if !authOK {
		api.WriteError(w, http.StatusUnauthorized,
			"registration requires a valid X-Enrollment-Token; obtain one from your admin via POST /api/v1/agents/enrollment-tokens")
		return
	}
	if tenantID == "" {
		tenantID = "default"
	}

	// Generate a fresh per-agent signing secret.
	rawSecret, err := middleware.GenerateAgentSecret()
	if err != nil {
		slog.ErrorContext(r.Context(), "agent_handler: secret gen failed", "agent_id", reg.AgentID, "error", err)
		api.WriteError(w, http.StatusInternalServerError, "failed to generate agent secret")
		return
	}

	secretEnc, err := middleware.EncryptAgentSecret(h.secretKey, rawSecret)
	if err != nil {
		slog.ErrorContext(r.Context(), "agent_handler: secret encrypt failed", "agent_id", reg.AgentID, "error", err)
		api.WriteError(w, http.StatusInternalServerError, "failed to encrypt agent secret")
		return
	}

	now := time.Now()
	agent := models.Agent{
		ID:           reg.AgentID,
		TenantID:     tenantID,
		Name:         reg.Name,
		HostIP:       reg.HostIP,
		OSType:       reg.OSType,
		Version:      reg.Version,
		Status:       "active",
		LastSeenAt:   now,
		RegisteredAt: now,
	}

	if err := h.agentStore.RegisterWithSecret(agent, secretEnc, enrolledVia); err != nil {
		slog.ErrorContext(r.Context(), "agent_handler: register failed", "agent_id", reg.AgentID, "error", err)
		api.WriteError(w, http.StatusInternalServerError, "failed to register agent")
		return
	}

	api.WriteJSON(w, http.StatusOK, models.AgentRegistrationResponse{
		Status:      "registered",
		AgentID:     reg.AgentID,
		AgentSecret: rawSecret, // shown exactly once; agent must store it securely
	})
}

// resolveRegistrationAuth validates the incoming registration credential.
// Returns (tenantID, enrolledVia, ok).
func (h *AgentHandler) resolveRegistrationAuth(r *http.Request, agentID string) (string, string, bool) {
	// JWT path: authenticated operator/admin registers on behalf of an agent.
	if claims, ok := middleware.ClaimsFromContext(r); ok {
		return claims.TenantID, "jwt:" + claims.UserID, true
	}

	// Enrollment token path (production).
	rawToken := r.Header.Get("X-Enrollment-Token")
	if rawToken != "" && h.enrollStore != nil {
		token, err := h.enrollStore.FindByRawToken(rawToken)
		if err != nil || token == nil {
			return "", "", false
		}
		if token.UsedAt != nil {
			slog.Warn("agent_handler: enrollment token already used", "token_id", token.ID, "used_by_agent_id", token.UsedByAgentID)
			return "", "", false
		}
		if time.Now().After(token.ExpiresAt) {
			slog.Warn("agent_handler: enrollment token expired", "token_id", token.ID, "expires_at", token.ExpiresAt)
			return "", "", false
		}
		// Consume the token atomically before responding.
		if err := h.enrollStore.MarkUsed(token.ID, agentID); err != nil {
			slog.Error("agent_handler: mark used failed for token", "token_id", token.ID, "error", err)
			return "", "", false
		}
		return token.TenantID, "enrollment:" + token.ID, true
	}

	// Legacy dev fallback: shared AGENT_INGEST_TOKEN.
	if h.devToken != "" && r.Header.Get("X-Agent-Token") == h.devToken {
		return "default", "legacy-dev-token", true
	}

	// Open dev mode: no token configured at all.
	// Blocked in production — operators must issue an enrollment token.
	if h.devToken == "" && h.enrollStore != nil && !h.isProd {
		slog.Warn("agent_handler: open-dev registration accepted — configure AGENT_INGEST_TOKEN or issue an enrollment token before deploying to production", "agent_id", agentID)
		return "default", "open-dev", true
	}

	return "", "", false
}

// ─── Ingest ──────────────────────────────────────────────────────────────────

// IngestBatch handles POST /api/v1/ingest/agent — batch event ingest from an agent.
//
// The HMAC middleware (AgentAuthenticator.Middleware) runs before this handler
// and injects a VerifiedAgent into the context.  Tenant is always taken from the
// registered agent record, never from the request body.
func (h *AgentHandler) IngestBatch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		api.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	// Derive tenant: verified agent context (HMAC path) → JWT → store lookup → default.
	tenantID := h.resolveTenantForIngest(r)

	var rawBody json.RawMessage
	if err := json.NewDecoder(r.Body).Decode(&rawBody); err != nil {
		api.WriteError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	var events []map[string]interface{}

	var batch models.AgentEventBatch
	if err := json.Unmarshal(rawBody, &batch); err == nil && len(batch.Events) > 0 {
		events = batch.Events
	} else {
		var single map[string]interface{}
		if err := json.Unmarshal(rawBody, &single); err != nil {
			api.WriteError(w, http.StatusBadRequest, "invalid event format")
			return
		}
		if _, ok := single["event.type"]; ok {
			events = []map[string]interface{}{single}
		} else if evts, ok := single["events"]; ok {
			if evtSlice, ok := evts.([]interface{}); ok {
				for _, e := range evtSlice {
					if em, ok := e.(map[string]interface{}); ok {
						events = append(events, em)
					}
				}
			}
		}
		if len(events) == 0 {
			api.WriteError(w, http.StatusBadRequest, "no events found in request body")
			return
		}
	}

	processed, alerts := h.ingestionService.ProcessBatch(events, tenantID)
	api.WriteJSON(w, http.StatusOK, map[string]int{"processed": processed, "alerts": alerts})
}

// resolveTenantForIngest returns the authoritative tenant ID for an ingest request.
func (h *AgentHandler) resolveTenantForIngest(r *http.Request) string {
	// HMAC-verified path: agent identity is cryptographically proven.
	if va, ok := middleware.AgentFromContext(r); ok {
		return va.TenantID
	}
	// JWT path (should not normally hit ingest, but handle gracefully).
	if claims, ok := middleware.ClaimsFromContext(r); ok {
		return claims.TenantID
	}
	// Dev open-mode fallback: try to resolve from agent.id in payload.
	return middleware.TenantFromRequest(r)
}

// ─── Secret rotation ─────────────────────────────────────────────────────────

// RotateSecret handles POST /api/v1/agents/{id}/rotate-secret.
// Requires a valid JWT (operator or admin).  Issues a fresh per-agent signing
// secret and immediately invalidates the old one.
func (h *AgentHandler) RotateSecret(w http.ResponseWriter, r *http.Request) {
	agentID := extractAgentIDBeforeAction(r.URL.Path, "/rotate-secret")
	if agentID == "" {
		api.WriteError(w, http.StatusBadRequest, "agent id is required")
		return
	}

	// Verify the agent belongs to the caller's tenant.
	claims, ok := middleware.ClaimsFromContext(r)
	if !ok {
		api.WriteError(w, http.StatusUnauthorized, "missing auth claims")
		return
	}
	agent, err := h.agentStore.GetAgentByID(agentID)
	if err != nil || agent == nil {
		api.WriteError(w, http.StatusNotFound, "agent not found")
		return
	}
	if agent.TenantID != claims.TenantID {
		api.WriteError(w, http.StatusForbidden, "agent does not belong to your tenant")
		return
	}

	rawSecret, err := middleware.GenerateAgentSecret()
	if err != nil {
		slog.ErrorContext(r.Context(), "agent_handler: rotate secret gen failed", "agent_id", agentID, "error", err)
		api.WriteError(w, http.StatusInternalServerError, "failed to generate secret")
		return
	}

	secretEnc, err := middleware.EncryptAgentSecret(h.secretKey, rawSecret)
	if err != nil {
		slog.ErrorContext(r.Context(), "agent_handler: rotate encrypt failed", "agent_id", agentID, "error", err)
		api.WriteError(w, http.StatusInternalServerError, "failed to encrypt secret")
		return
	}

	if err := h.agentStore.RotateSecret(agentID, secretEnc); err != nil {
		slog.ErrorContext(r.Context(), "agent_handler: rotate db failed", "agent_id", agentID, "error", err)
		api.WriteError(w, http.StatusInternalServerError, "failed to rotate secret")
		return
	}

	slog.InfoContext(r.Context(), "agent_handler: secret rotated", "agent_id", agentID, "user_id", claims.UserID)
	api.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"agent_id":    agentID,
		"agent_secret": rawSecret, // shown once; old secret is immediately invalid
		"rotated_at":  time.Now().UTC(),
	})
}

// ─── CRUD / management ───────────────────────────────────────────────────────

// ListAgents handles GET /api/v1/agents.
func (h *AgentHandler) ListAgents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		api.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	tenantID := middleware.TenantFromRequest(r)
	limit, offset := parsePagination(r, 50, 1000)
	agents, err := h.agentStore.GetAgents(tenantID, limit, offset)
	if err != nil {
		slog.ErrorContext(r.Context(), "agent_handler: list agents error", "error", err)
		api.WriteError(w, http.StatusInternalServerError, "failed to list agents")
		return
	}
	if agents == nil {
		agents = []models.Agent{}
	}
	api.WriteJSON(w, http.StatusOK, agents)
}

// GetAgent handles GET /api/v1/agents/{id}.
func (h *AgentHandler) GetAgent(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		api.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	agentID := extractAgentID(r.URL.Path)
	if agentID == "" {
		api.WriteError(w, http.StatusBadRequest, "agent id is required")
		return
	}
	agent, err := h.agentStore.GetAgentByID(agentID)
	if err != nil {
		slog.ErrorContext(r.Context(), "agent_handler: get agent error", "error", err)
		api.WriteError(w, http.StatusInternalServerError, "failed to get agent")
		return
	}
	if agent == nil {
		api.WriteError(w, http.StatusNotFound, "agent not found")
		return
	}

	metricTypes := []string{"cpu_memory", "disk", "network_interface", "process", "system_info", "system_load"}
	recentMetrics := make(map[string][]map[string]interface{})
	for _, mt := range metricTypes {
		if metrics, err := h.agentStore.GetRecentMetrics(agentID, mt, 10); err == nil && len(metrics) > 0 {
			recentMetrics[mt] = metrics
		}
	}

	api.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"agent":          agent,
		"recent_metrics": recentMetrics,
	})
}

// DeleteAgent handles DELETE /api/v1/agents/{id}.
func (h *AgentHandler) DeleteAgent(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		api.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	agentID := extractAgentID(r.URL.Path)
	if agentID == "" {
		api.WriteError(w, http.StatusBadRequest, "agent id is required")
		return
	}
	if err := h.agentStore.DeleteAgent(agentID); err != nil {
		slog.ErrorContext(r.Context(), "agent_handler: delete agent error", "error", err)
		api.WriteError(w, http.StatusInternalServerError, "failed to delete agent")
		return
	}
	api.WriteJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// StartAgent handles POST /api/v1/agents/{id}/start.
func (h *AgentHandler) StartAgent(w http.ResponseWriter, r *http.Request) {
	agentID := extractAgentIDBeforeAction(r.URL.Path, "/start")
	if agentID == "" {
		api.WriteError(w, http.StatusBadRequest, "agent id is required")
		return
	}
	if err := h.agentStore.UpdateStatus(agentID, "active"); err != nil {
		api.WriteError(w, http.StatusInternalServerError, "failed to start agent")
		return
	}
	api.WriteJSON(w, http.StatusOK, map[string]string{"status": "active", "agent_id": agentID})
}

// StopAgent handles POST /api/v1/agents/{id}/stop.
func (h *AgentHandler) StopAgent(w http.ResponseWriter, r *http.Request) {
	agentID := extractAgentIDBeforeAction(r.URL.Path, "/stop")
	if agentID == "" {
		api.WriteError(w, http.StatusBadRequest, "agent id is required")
		return
	}
	if err := h.agentStore.UpdateStatus(agentID, "stopped"); err != nil {
		api.WriteError(w, http.StatusInternalServerError, "failed to stop agent")
		return
	}
	api.WriteJSON(w, http.StatusOK, map[string]string{"status": "stopped", "agent_id": agentID})
}

// GetAgentMetrics handles GET /api/v1/agents/{id}/metrics.
func (h *AgentHandler) GetAgentMetrics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		api.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	path := strings.TrimSuffix(r.URL.Path, "/metrics")
	agentID := extractAgentID(path)
	if agentID == "" {
		api.WriteError(w, http.StatusBadRequest, "agent id is required")
		return
	}

	metricType := r.URL.Query().Get("type")
	limit := 50
	if l, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && l > 0 {
		limit = l
	}

	metrics, err := h.agentStore.GetRecentMetrics(agentID, metricType, limit)
	if err != nil {
		api.WriteError(w, http.StatusInternalServerError, "failed to get metrics")
		return
	}
	if metrics == nil {
		metrics = []map[string]interface{}{}
	}
	api.WriteJSON(w, http.StatusOK, metrics)
}

// HandleAgentByID dispatches sub-paths and HTTP methods for /api/v1/agents/{id}/...
func (h *AgentHandler) HandleAgentByID(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	switch {
	case strings.HasSuffix(path, "/metrics"):
		h.GetAgentMetrics(w, r)
	case strings.HasSuffix(path, "/start") && r.Method == http.MethodPost:
		h.StartAgent(w, r)
	case strings.HasSuffix(path, "/stop") && r.Method == http.MethodPost:
		h.StopAgent(w, r)
	case strings.HasSuffix(path, "/rotate-secret") && r.Method == http.MethodPost:
		h.RotateSecret(w, r)
	default:
		switch r.Method {
		case http.MethodGet:
			h.GetAgent(w, r)
		case http.MethodDelete:
			h.DeleteAgent(w, r)
		default:
			api.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		}
	}
}

// ─── URL helpers ─────────────────────────────────────────────────────────────

func extractAgentIDBeforeAction(path, action string) string {
	return extractAgentID(strings.TrimSuffix(path, action))
}

func extractAgentID(path string) string {
	path = strings.TrimSuffix(path, "/")
	parts := strings.Split(path, "/")
	if len(parts) < 1 {
		return ""
	}
	return parts[len(parts)-1]
}
