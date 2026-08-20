package middleware_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"ai-incident-platform/backend/internal/middleware"
	"ai-incident-platform/backend/internal/models"
)

func TestTenantFromRequest_JWTClaims(t *testing.T) {
	claims := models.TokenClaims{UserID: "u1", TenantID: "acme", Role: "operator"}
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	ctx := context.WithValue(req.Context(), models.ClaimsContextKey, claims)
	req = req.WithContext(ctx)

	got := middleware.TenantFromRequest(req)
	if got != "acme" {
		t.Fatalf("expected acme, got %s", got)
	}
}

func TestTenantFromRequest_XTenantHeader(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Tenant-ID", "beta-corp")

	got := middleware.TenantFromRequest(req)
	if got != "beta-corp" {
		t.Fatalf("expected beta-corp, got %s", got)
	}
}

func TestTenantFromRequest_QueryParam(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/?tenant_id=gamma-co", nil)

	got := middleware.TenantFromRequest(req)
	if got != "gamma-co" {
		t.Fatalf("expected gamma-co, got %s", got)
	}
}

func TestTenantFromRequest_DefaultFallback(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)

	got := middleware.TenantFromRequest(req)
	if got != "default" {
		t.Fatalf("expected default, got %s", got)
	}
}

func TestTenantFromRequest_JWTTakesPrecedenceOverHeader(t *testing.T) {
	claims := models.TokenClaims{UserID: "u1", TenantID: "jwt-tenant", Role: "operator"}
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Tenant-ID", "header-tenant")
	ctx := context.WithValue(req.Context(), models.ClaimsContextKey, claims)
	req = req.WithContext(ctx)

	got := middleware.TenantFromRequest(req)
	if got != "jwt-tenant" {
		t.Fatalf("expected JWT tenant jwt-tenant to win, got %s", got)
	}
}

func TestTenantFromRequestStrict_NoAuth(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)

	_, ok := middleware.TenantFromRequestStrict(req)
	if ok {
		t.Fatal("expected TenantFromRequestStrict to return false for unauthenticated request")
	}
}

func TestTenantFromRequestStrict_WithJWT(t *testing.T) {
	claims := models.TokenClaims{UserID: "u1", TenantID: "strict-co", Role: "admin"}
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	ctx := context.WithValue(req.Context(), models.ClaimsContextKey, claims)
	req = req.WithContext(ctx)

	tenantID, ok := middleware.TenantFromRequestStrict(req)
	if !ok {
		t.Fatal("expected TenantFromRequestStrict to return true with JWT")
	}
	if tenantID != "strict-co" {
		t.Fatalf("expected strict-co, got %s", tenantID)
	}
}
