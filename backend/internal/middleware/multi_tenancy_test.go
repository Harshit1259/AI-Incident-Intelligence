// Package middleware_test — multi-tenancy isolation integration tests.
//
// These tests validate the fundamental invariant of a multi-tenant SaaS system:
// Tenant A's JWT cannot be used to impersonate Tenant B, and one tenant's
// rate limits cannot affect another tenant.
//
// No database required — all tests use in-memory middleware state + httptest.
package middleware_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"ai-incident-platform/backend/internal/api"
	"ai-incident-platform/backend/internal/middleware"
	"ai-incident-platform/backend/internal/models"
	"ai-incident-platform/backend/internal/platform/ratelimit"
)

// ── Helpers ───────────────────────────────────────────────────────────────────

const mtSecret = "multi-tenancy-test-secret-32chars-ok"

func jwtForTenant(t *testing.T, tenantID, role string) string {
	t.Helper()
	tok, err := middleware.GenerateToken(models.TokenClaims{
		UserID:   "user-" + tenantID,
		TenantID: tenantID,
		Role:     role,
		Exp:      time.Now().Add(time.Hour).Unix(),
	}, mtSecret)
	if err != nil {
		t.Fatalf("GenerateToken: %v", err)
	}
	return tok
}

// requestWithClaims returns a request whose context contains the given claims.
func requestWithClaims(method string, claims models.TokenClaims) *http.Request {
	r := httptest.NewRequest(method, "/", nil)
	ctx := context.WithValue(r.Context(), models.ClaimsContextKey, claims)
	return r.WithContext(ctx)
}

// echoTenantHandler writes the resolved tenant ID so tests can assert on it.
var echoTenantHandler http.HandlerFunc = func(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, middleware.TenantFromRequest(r))
}

// stubTenantStore implements tenantStateReader without a database.
type stubTenantStore struct {
	states map[string]string
}

func (s *stubTenantStore) GetState(tenantID string) string {
	if state, ok := s.states[tenantID]; ok {
		return state
	}
	return "active"
}

// ── JWT tenant isolation ───────────────────────────────────────────────────────

// TestJWTClaimsTakePrecedenceOverQueryParam is the primary cross-tenant
// escalation test: tenant_id in the URL must not override a valid JWT.
func TestJWTClaimsTakePrecedenceOverQueryParam(t *testing.T) {
	protected := middleware.RequireAuth(mtSecret, echoTenantHandler)

	r := httptest.NewRequest(http.MethodGet, "/?tenant_id=tenant-b", nil)
	r.Header.Set("Authorization", "Bearer "+jwtForTenant(t, "tenant-a", models.RoleAdmin))

	rr := httptest.NewRecorder()
	protected(rr, r)

	if rr.Code != http.StatusOK {
		t.Fatalf("unexpected status %d", rr.Code)
	}
	if got := rr.Body.String(); got != "tenant-a" {
		t.Errorf("TenantFromRequest = %q; want tenant-a — JWT must override query param", got)
	}
}

// TestJWTClaimsTakePrecedenceOverHeader verifies that X-Tenant-ID header
// cannot override the JWT tenant claim.
func TestJWTClaimsTakePrecedenceOverHeader(t *testing.T) {
	protected := middleware.RequireAuth(mtSecret, echoTenantHandler)

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("Authorization", "Bearer "+jwtForTenant(t, "tenant-a", models.RoleAdmin))
	r.Header.Set("X-Tenant-ID", "tenant-evil")

	rr := httptest.NewRecorder()
	protected(rr, r)

	if rr.Code != http.StatusOK {
		t.Fatalf("unexpected status %d", rr.Code)
	}
	if got := rr.Body.String(); got != "tenant-a" {
		t.Errorf("TenantFromRequest = %q; want tenant-a — JWT must override X-Tenant-ID", got)
	}
}

// TestCrossTenantResourceDenied verifies that a valid token from tenant-B
// cannot access a resource that enforces tenant-A ownership.
func TestCrossTenantResourceDenied(t *testing.T) {
	tokB := jwtForTenant(t, "tenant-b", models.RoleAdmin)

	// Simulates a handler that enforces ownership.
	tenantAOnly := middleware.RequireAuth(mtSecret, func(w http.ResponseWriter, r *http.Request) {
		claims, ok := middleware.ClaimsFromContext(r)
		if !ok || claims.TenantID != "tenant-a" {
			api.WriteErrorCode(w, http.StatusForbidden, "wrong tenant", api.ErrCodeForbidden)
			return
		}
		w.WriteHeader(http.StatusOK)
	})

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("Authorization", "Bearer "+tokB)

	rr := httptest.NewRecorder()
	tenantAOnly(rr, r)

	if rr.Code != http.StatusForbidden {
		t.Errorf("tenant-B token on tenant-A resource: got %d, want 403", rr.Code)
	}
}

// ── Rate limit isolation ──────────────────────────────────────────────────────

// TestRateLimitPerTenantIsolation verifies that exhausting tenant-A's bucket
// does not affect tenant-B's requests.
func TestRateLimitPerTenantIsolation(t *testing.T) {
	loader := func(_ string, _ ratelimit.Category) int { return 2 }
	limiter := ratelimit.New(loader)

	// Exhaust tenant-A.
	limiter.Allow("tenant-a", ratelimit.CategoryQuery)
	limiter.Allow("tenant-a", ratelimit.CategoryQuery)

	if limiter.Allow("tenant-a", ratelimit.CategoryQuery) {
		t.Error("tenant-a: should be rate limited after exhausting 2-rpm bucket")
	}
	if !limiter.Allow("tenant-b", ratelimit.CategoryQuery) {
		t.Error("tenant-b: must not be affected by tenant-a's exhausted bucket")
	}
}

// TestMutationRateLimitSkipsGET verifies the mutation limiter is method-aware:
// GET requests bypass it even after the bucket is exhausted.
func TestMutationRateLimitSkipsGET(t *testing.T) {
	loader := func(_ string, cat ratelimit.Category) int {
		if cat == ratelimit.CategoryMutation {
			return 1 // 1 mutation per minute
		}
		return 1000
	}
	limiter := ratelimit.New(loader)

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
	wrapped := middleware.MutationRateLimit(limiter)(inner)

	claims := models.TokenClaims{TenantID: "tenant-x", Role: models.RoleAdmin}

	makeReq := func(method string) *http.Request {
		return requestWithClaims(method, claims)
	}

	// GET never consumes mutation tokens.
	for i := 0; i < 5; i++ {
		rr := httptest.NewRecorder()
		wrapped(rr, makeReq(http.MethodGet))
		if rr.Code != http.StatusOK {
			t.Errorf("GET #%d: got %d, want 200", i+1, rr.Code)
		}
	}

	// First POST consumes the single mutation token.
	rr1 := httptest.NewRecorder()
	wrapped(rr1, makeReq(http.MethodPost))
	if rr1.Code != http.StatusOK {
		t.Errorf("POST #1: got %d, want 200", rr1.Code)
	}

	// Second POST must be rate-limited.
	rr2 := httptest.NewRecorder()
	wrapped(rr2, makeReq(http.MethodPost))
	if rr2.Code != http.StatusTooManyRequests {
		t.Errorf("POST #2 after exhaustion: got %d, want 429", rr2.Code)
	}

	// GET after exhaustion must still pass.
	rr3 := httptest.NewRecorder()
	wrapped(rr3, makeReq(http.MethodGet))
	if rr3.Code != http.StatusOK {
		t.Errorf("GET after exhaustion: got %d, want 200", rr3.Code)
	}
}

// ── Tenant lifecycle isolation ────────────────────────────────────────────────

// TestTenantIsolationSuspended verifies a suspended tenant gets 403 with
// the TENANT_SUSPENDED error code.
func TestTenantIsolationSuspended(t *testing.T) {
	ts := &stubTenantStore{states: map[string]string{"tenant-bad": "suspended"}}
	isolation := middleware.NewTenantIsolationFromReader(ts)
	handler := isolation.Enforce(okHandler)

	r := requestWithClaims(http.MethodGet, models.TokenClaims{
		UserID: "u", TenantID: "tenant-bad", Role: models.RoleAdmin,
	})
	rr := httptest.NewRecorder()
	handler(rr, r)

	if rr.Code != http.StatusForbidden {
		t.Errorf("suspended: got %d, want 403", rr.Code)
	}
	var body api.ErrorResponse
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Code != api.ErrCodeTenantSuspended {
		t.Errorf("Code: got %q, want %q", body.Code, api.ErrCodeTenantSuspended)
	}
}

// TestTenantIsolationDisabled verifies a disabled tenant gets the right code.
func TestTenantIsolationDisabled(t *testing.T) {
	ts := &stubTenantStore{states: map[string]string{"tenant-off": "disabled"}}
	isolation := middleware.NewTenantIsolationFromReader(ts)
	handler := isolation.Enforce(okHandler)

	r := requestWithClaims(http.MethodGet, models.TokenClaims{
		UserID: "u", TenantID: "tenant-off", Role: models.RoleAdmin,
	})
	rr := httptest.NewRecorder()
	handler(rr, r)

	if rr.Code != http.StatusForbidden {
		t.Errorf("disabled: got %d, want 403", rr.Code)
	}
	var body api.ErrorResponse
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Code != api.ErrCodeTenantDisabled {
		t.Errorf("Code: got %q, want %q", body.Code, api.ErrCodeTenantDisabled)
	}
}

// TestTenantIsolationActive verifies an active tenant passes through normally.
func TestTenantIsolationActive(t *testing.T) {
	ts := &stubTenantStore{states: map[string]string{"tenant-ok": "active"}}
	isolation := middleware.NewTenantIsolationFromReader(ts)
	handler := isolation.Enforce(okHandler)

	r := requestWithClaims(http.MethodGet, models.TokenClaims{
		UserID: "u", TenantID: "tenant-ok", Role: models.RoleAdmin,
	})
	rr := httptest.NewRecorder()
	handler(rr, r)

	if rr.Code != http.StatusOK {
		t.Errorf("active: got %d, want 200", rr.Code)
	}
}

// TestTenantACannotAffectTenantBIsolation verifies that suspending tenant-A
// does not block tenant-B.
func TestTenantACannotAffectTenantBIsolation(t *testing.T) {
	ts := &stubTenantStore{states: map[string]string{"tenant-a": "suspended", "tenant-b": "active"}}
	isolation := middleware.NewTenantIsolationFromReader(ts)
	handler := isolation.Enforce(okHandler)

	rA := requestWithClaims(http.MethodGet, models.TokenClaims{TenantID: "tenant-a", Role: models.RoleAdmin})
	rrA := httptest.NewRecorder()
	handler(rrA, rA)
	if rrA.Code != http.StatusForbidden {
		t.Errorf("tenant-a suspended: got %d, want 403", rrA.Code)
	}

	rB := requestWithClaims(http.MethodGet, models.TokenClaims{TenantID: "tenant-b", Role: models.RoleAdmin})
	rrB := httptest.NewRecorder()
	handler(rrB, rB)
	if rrB.Code != http.StatusOK {
		t.Errorf("tenant-b active: got %d, want 200 (must not be blocked by tenant-a)", rrB.Code)
	}
}
