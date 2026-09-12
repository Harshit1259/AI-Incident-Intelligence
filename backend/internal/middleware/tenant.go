package middleware

import (
	"net/http"
)

// TenantFromRequest extracts the active tenant ID for a request.
//
// Resolution order:
//  1. JWT claims (set by RequireAuth — authoritative for authenticated routes)
//  2. X-Tenant-ID header (for service-to-service calls carrying an explicit tenant)
//  3. tenant_id query parameter (admin override — only useful when JWT has no tenant)
//  4. "default" sentinel (single-tenant / dev mode)
//
// Callers that require a real tenant (multi-tenant deployments) should call
// TenantFromRequestStrict and reject 401 when it returns ("", false).
func TenantFromRequest(r *http.Request) string {
	if claims, ok := ClaimsFromContext(r); ok && claims.TenantID != "" {
		return claims.TenantID
	}
	if h := r.Header.Get("X-Tenant-ID"); h != "" {
		return h
	}
	if q := r.URL.Query().Get("tenant_id"); q != "" {
		return q
	}
	return "default"
}

// TenantFromRequestStrict returns the tenant ID and true only when the tenant
// comes from verified JWT claims. Returns ("", false) otherwise.
//
// Request headers are never trusted here. It used to accept an X-Tenant-ID
// header too, and the public ingest endpoints (which have no auth middleware)
// call this before checking the source token — so any caller could write
// alerts into any tenant by naming it in a header.
func TenantFromRequestStrict(r *http.Request) (string, bool) {
	if claims, ok := ClaimsFromContext(r); ok && claims.TenantID != "" {
		return claims.TenantID, true
	}
	return "", false
}
