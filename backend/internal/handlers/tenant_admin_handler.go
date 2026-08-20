package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"ai-incident-platform/backend/internal/api"
	"ai-incident-platform/backend/internal/middleware"
	"ai-incident-platform/backend/internal/models"
	"ai-incident-platform/backend/internal/platform/cache"
	"ai-incident-platform/backend/internal/platform/queue"
	"ai-incident-platform/backend/internal/platform/ratelimit"
	"ai-incident-platform/backend/internal/store"
)

// TenantAdminHandler exposes the SaaS multi-tenancy management plane.
// All endpoints require admin role.
type TenantAdminHandler struct {
	ts        *store.TenantStore
	auditStore *store.AuditLogStore
	isolation  *middleware.TenantIsolation
	limiter    *ratelimit.TenantRateLimiter
	cachesMgr  *cache.Manager
	queueMgr   *queue.Manager
}

func NewTenantAdminHandler(
	ts *store.TenantStore,
	as *store.AuditLogStore,
	iso *middleware.TenantIsolation,
	lim *ratelimit.TenantRateLimiter,
	cm *cache.Manager,
	qm *queue.Manager,
) *TenantAdminHandler {
	return &TenantAdminHandler{
		ts: ts, auditStore: as, isolation: iso,
		limiter: lim, cachesMgr: cm, queueMgr: qm,
	}
}

// Handle dispatches all /api/v1/admin/tenants* requests.
func (h *TenantAdminHandler) Handle(w http.ResponseWriter, r *http.Request) {
	claims, ok := middleware.ClaimsFromContext(r)
	if !ok {
		api.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	sub := strings.TrimPrefix(r.URL.Path, "/api/v1/admin/tenants")

	// /api/v1/admin/audit/search
	if r.URL.Path == "/api/v1/admin/audit/search" || strings.HasPrefix(r.URL.Path, "/api/v1/admin/audit") {
		h.handleAuditSearch(w, r, claims.TenantID)
		return
	}

	// /api/v1/admin/tenants/queue-stats
	if sub == "/queue-stats" {
		if r.Method != http.MethodGet {
			api.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		api.WriteJSON(w, http.StatusOK, h.queueMgr.Stats())
		return
	}

	id, rest := adminPathID(sub)

	if id == "" {
		// /api/v1/admin/tenants
		switch r.Method {
		case http.MethodGet:
			h.listTenants(w, r)
		case http.MethodPost:
			h.createTenant(w, r, claims)
		default:
			api.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		}
		return
	}

	// /api/v1/admin/tenants/{id}/...
	switch rest {
	case "", "/":
		switch r.Method {
		case http.MethodGet:
			h.getTenant(w, r, id)
		case http.MethodPut:
			h.updateTenant(w, r, id, claims)
		default:
			api.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		}
	case "/state":
		if r.Method == http.MethodPost {
			h.setState(w, r, id, claims)
		} else {
			api.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		}
	case "/plan":
		if r.Method == http.MethodPost {
			h.setPlan(w, r, id, claims)
		} else {
			api.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		}
	case "/limits":
		switch r.Method {
		case http.MethodGet:
			h.getLimits(w, r, id)
		case http.MethodPut:
			h.setLimits(w, r, id, claims)
		default:
			api.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		}
	case "/usage":
		if r.Method == http.MethodGet {
			h.getUsage(w, r, id)
		} else {
			api.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		}
	case "/encryption":
		if r.Method == http.MethodPost {
			h.setEncryption(w, r, id, claims)
		} else {
			api.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		}
	default:
		api.WriteError(w, http.StatusNotFound, "not found")
	}
}

// ── Tenant CRUD ───────────────────────────────────────────────────────────────

func (h *TenantAdminHandler) listTenants(w http.ResponseWriter, _ *http.Request) {
	tenants, err := h.ts.List()
	if err != nil {
		api.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	api.WriteJSON(w, http.StatusOK, tenants)
}

func (h *TenantAdminHandler) getTenant(w http.ResponseWriter, _ *http.Request, id string) {
	t, err := h.ts.GetByID(id)
	if err != nil || t == nil {
		api.WriteError(w, http.StatusNotFound, "tenant not found")
		return
	}
	api.WriteJSON(w, http.StatusOK, t)
}

func (h *TenantAdminHandler) createTenant(w http.ResponseWriter, r *http.Request, claims models.TokenClaims) {
	var t models.TenantMaster
	if err := json.NewDecoder(r.Body).Decode(&t); err != nil {
		api.WriteError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if t.ID == "" || t.Name == "" {
		api.WriteError(w, http.StatusBadRequest, "id and name are required")
		return
	}
	if t.Slug == "" {
		t.Slug = strings.ToLower(strings.ReplaceAll(t.Name, " ", "-"))
	}
	if t.Plan == "" {
		t.Plan = string(models.PlanTrial)
	}
	if t.State == "" {
		t.State = string(models.TenantStateActive)
	}
	if t.AuditRetainDays == 0 {
		t.AuditRetainDays = 90
	}
	if err := h.ts.Upsert(t); err != nil {
		api.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	go h.auditStore.Log(store.AuditLogEntry{ //nolint:errcheck
		TenantID: claims.TenantID, Actor: claims.UserID,
		Action: "tenant.create", ResourceType: "tenant", ResourceID: t.ID,
	})
	api.WriteJSON(w, http.StatusCreated, t)
}

func (h *TenantAdminHandler) updateTenant(w http.ResponseWriter, r *http.Request, id string, claims models.TokenClaims) {
	var t models.TenantMaster
	if err := json.NewDecoder(r.Body).Decode(&t); err != nil {
		api.WriteError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	t.ID = id
	if err := h.ts.Upsert(t); err != nil {
		api.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	go h.auditStore.Log(store.AuditLogEntry{ //nolint:errcheck
		TenantID: claims.TenantID, Actor: claims.UserID,
		Action: "tenant.update", ResourceType: "tenant", ResourceID: id,
	})
	api.WriteJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

// ── Lifecycle state ───────────────────────────────────────────────────────────

func (h *TenantAdminHandler) setState(w http.ResponseWriter, r *http.Request, id string, claims models.TokenClaims) {
	var body struct {
		State  string `json:"state"`
		Reason string `json:"reason"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		api.WriteError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if body.State == "" {
		api.WriteError(w, http.StatusBadRequest, "state is required")
		return
	}
	if err := h.ts.SetState(id, body.State); err != nil {
		api.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	// Hard isolation side effects
	h.isolation.InvalidateCache(id)
	h.limiter.Evict(id)
	h.cachesMgr.Evict(id)
	if body.State == "suspended" || body.State == "disabled" {
		h.queueMgr.Purge(id)
	}
	go h.auditStore.Log(store.AuditLogEntry{ //nolint:errcheck
		TenantID: claims.TenantID, Actor: claims.UserID,
		Action: "tenant.state." + body.State, ResourceType: "tenant", ResourceID: id,
		DetailsJSON: `{"reason":"` + body.Reason + `"}`,
	})
	api.WriteJSON(w, http.StatusOK, map[string]string{"status": body.State})
}

// ── Plan ──────────────────────────────────────────────────────────────────────

func (h *TenantAdminHandler) setPlan(w http.ResponseWriter, r *http.Request, id string, claims models.TokenClaims) {
	var body struct {
		Plan string `json:"plan"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		api.WriteError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if err := h.ts.SetPlan(id, body.Plan); err != nil {
		api.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	// Evict rate-limit state so new plan defaults take effect immediately
	h.limiter.Evict(id)
	go h.auditStore.Log(store.AuditLogEntry{ //nolint:errcheck
		TenantID: claims.TenantID, Actor: claims.UserID,
		Action: "tenant.plan.change", ResourceType: "tenant", ResourceID: id,
		DetailsJSON: `{"plan":"` + body.Plan + `"}`,
	})
	api.WriteJSON(w, http.StatusOK, map[string]string{"plan": body.Plan})
}

// ── Rate limits ───────────────────────────────────────────────────────────────

func (h *TenantAdminHandler) getLimits(w http.ResponseWriter, _ *http.Request, id string) {
	profile, err := h.ts.GetEffectiveLimits(id)
	if err != nil {
		api.WriteError(w, http.StatusNotFound, "tenant not found")
		return
	}
	api.WriteJSON(w, http.StatusOK, profile)
}

func (h *TenantAdminHandler) setLimits(w http.ResponseWriter, r *http.Request, id string, claims models.TokenClaims) {
	var body struct {
		IngestPerMin int `json:"ingest_per_min"`
		QueryPerMin  int `json:"query_per_min"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		api.WriteError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if err := h.ts.SetRateLimits(id, body.IngestPerMin, body.QueryPerMin); err != nil {
		api.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.limiter.Evict(id) // force reload
	go h.auditStore.Log(store.AuditLogEntry{ //nolint:errcheck
		TenantID: claims.TenantID, Actor: claims.UserID,
		Action: "tenant.limits.update", ResourceType: "tenant", ResourceID: id,
	})
	api.WriteJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

// ── Usage ─────────────────────────────────────────────────────────────────────

func (h *TenantAdminHandler) getUsage(w http.ResponseWriter, _ *http.Request, id string) {
	usage, err := h.ts.GetUsage(id)
	if err != nil {
		api.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	// Append queue stats
	out := map[string]any{
		"tenant_id":      usage.TenantID,
		"incident_count": usage.IncidentCount,
		"event_count":    usage.EventCount,
		"user_count":     usage.UserCount,
		"queue_depth":    h.queueMgr.Depth(id),
		"queue_dropped":  h.queueMgr.DroppedCount(id),
		"cache_entries":  h.cachesMgr.Stats(id),
	}
	api.WriteJSON(w, http.StatusOK, out)
}

// ── Encryption toggle ─────────────────────────────────────────────────────────

func (h *TenantAdminHandler) setEncryption(w http.ResponseWriter, r *http.Request, id string, claims models.TokenClaims) {
	var body struct {
		Enabled bool `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		api.WriteError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if err := h.ts.SetEncryption(id, body.Enabled); err != nil {
		api.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	go h.auditStore.Log(store.AuditLogEntry{ //nolint:errcheck
		TenantID: claims.TenantID, Actor: claims.UserID,
		Action: "tenant.encryption.toggle", ResourceType: "tenant", ResourceID: id,
	})
	api.WriteJSON(w, http.StatusOK, map[string]bool{"encryption_enabled": body.Enabled})
}

// ── Tenant-scoped audit search ────────────────────────────────────────────────

func (h *TenantAdminHandler) handleAuditSearch(w http.ResponseWriter, r *http.Request, tenantID string) {
	if r.Method != http.MethodGet {
		api.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	q := r.URL.Query()
	sq := store.AuditSearchQuery{
		TenantID:     tenantID,
		Actor:        q.Get("actor"),
		Action:       q.Get("action"),
		ResourceType: q.Get("resource_type"),
		ResourceID:   q.Get("resource_id"),
	}
	if s := q.Get("from"); s != "" {
		if t, err := time.Parse(time.RFC3339, s); err == nil {
			sq.FromTime = &t
		}
	}
	if s := q.Get("to"); s != "" {
		if t, err := time.Parse(time.RFC3339, s); err == nil {
			sq.ToTime = &t
		}
	}
	sq.Limit, _ = strconv.Atoi(q.Get("limit"))
	sq.Offset, _ = strconv.Atoi(q.Get("offset"))

	entries, total, err := h.auditStore.Search(sq)
	if err != nil {
		api.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	api.WriteJSON(w, http.StatusOK, map[string]any{
		"entries": entries,
		"total":   total,
		"limit":   sq.Limit,
		"offset":  sq.Offset,
	})
}

// adminPathID extracts the first path segment from a sub-path like "/{id}/state".
func adminPathID(sub string) (id, rest string) {
	sub = strings.TrimPrefix(sub, "/")
	if sub == "" {
		return "", ""
	}
	parts := strings.SplitN(sub, "/", 2)
	id = parts[0]
	if len(parts) > 1 {
		rest = "/" + parts[1]
	}
	return id, rest
}
