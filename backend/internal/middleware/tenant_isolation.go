package middleware

import (
	"net/http"
	"sync"
	"time"

	"ai-incident-platform/backend/internal/api"
	"ai-incident-platform/backend/internal/store"
)

const tenantStateTTL = 30 * time.Second

type cachedTenantState struct {
	state     string
	expiresAt time.Time
}

// tenantStateReader is the subset of store.TenantStore used by TenantIsolation.
// Extracted as an interface so tests can inject stubs without a real database.
type tenantStateReader interface {
	GetState(tenantID string) string
}

// TenantIsolation enforces hard tenant lifecycle boundaries.
// It caches state for 30 s to avoid a DB read on every request.
type TenantIsolation struct {
	ts    tenantStateReader
	cache sync.Map // tenantID -> *cachedTenantState
}

// NewTenantIsolation creates a TenantIsolation middleware backed by the tenant store.
func NewTenantIsolation(ts *store.TenantStore) *TenantIsolation {
	return &TenantIsolation{ts: ts}
}

// NewTenantIsolationFromReader creates a TenantIsolation from any tenantStateReader.
// Used in tests to inject stubs without a real database.
func NewTenantIsolationFromReader(ts tenantStateReader) *TenantIsolation {
	return &TenantIsolation{ts: ts}
}

// Enforce returns an http.Handler that rejects requests from suspended or disabled
// tenants before calling the wrapped handler. The tenant ID is read from JWT claims
// (set by RequireAuth). Requests without a tenant claim pass through unchanged.
func (ti *TenantIsolation) Enforce(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		claims, ok := ClaimsFromContext(r)
		if !ok || claims.TenantID == "" {
			next(w, r)
			return
		}

		state := ti.cachedState(claims.TenantID)
		switch state {
		case "suspended":
			api.WriteErrorCode(w, http.StatusForbidden,
				"tenant suspended", api.ErrCodeTenantSuspended)
			return
		case "disabled":
			api.WriteErrorCode(w, http.StatusForbidden,
				"account disabled — contact support", api.ErrCodeTenantDisabled)
			return
		}
		next(w, r)
	}
}

// InvalidateCache removes a tenant's cached state — call after a lifecycle change.
func (ti *TenantIsolation) InvalidateCache(tenantID string) {
	ti.cache.Delete(tenantID)
}

func (ti *TenantIsolation) cachedState(tenantID string) string {
	if v, ok := ti.cache.Load(tenantID); ok {
		cs := v.(*cachedTenantState)
		if time.Now().Before(cs.expiresAt) {
			return cs.state
		}
	}
	state := ti.ts.GetState(tenantID)
	ti.cache.Store(tenantID, &cachedTenantState{
		state:     state,
		expiresAt: time.Now().Add(tenantStateTTL),
	})
	return state
}
