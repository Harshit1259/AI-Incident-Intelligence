package middleware

import (
	"net/http"

	"ai-incident-platform/backend/internal/api"
	"ai-incident-platform/backend/internal/platform/ratelimit"
)

// TenantRateLimit returns middleware that enforces per-tenant token-bucket
// rate limits. The tenant is resolved from the JWT claims set by RequireAuth,
// so this middleware must appear downstream of RequireAuth in the chain.
//
// When the tenant limit is exceeded the handler receives a 429 with a
// Retry-After header instead of calling the wrapped handler.
//
// "default" tenants (dev/single-tenant mode) are never limited.
func TenantRateLimit(limiter *ratelimit.TenantRateLimiter, cat ratelimit.Category) func(http.HandlerFunc) http.HandlerFunc {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			tenantID := TenantFromRequest(r)

			// Skip limiting for the default/dev sentinel
			if tenantID != "" && tenantID != "default" {
				if !limiter.Allow(tenantID, cat) {
					w.Header().Set("Retry-After", "60")
					api.WriteErrorCode(w, http.StatusTooManyRequests,
						"rate limit exceeded — retry after 60 seconds", api.ErrCodeRateLimitExceeded)
					return
				}
			}

			next(w, r)
		}
	}
}

// MutationRateLimit returns middleware that enforces per-tenant rate limits
// only on state-changing requests (POST, PUT, PATCH, DELETE). GET, HEAD, and
// OPTIONS pass through without consuming tokens, so read-heavy workloads are
// unaffected. Apply this inside withAuth, downstream of RequireAuth.
func MutationRateLimit(limiter *ratelimit.TenantRateLimiter) func(http.HandlerFunc) http.HandlerFunc {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			switch r.Method {
			case http.MethodGet, http.MethodHead, http.MethodOptions:
				// Read-only — skip mutation bucket entirely.
				next(w, r)
				return
			}

			tenantID := TenantFromRequest(r)
			if tenantID != "" && tenantID != "default" {
				if !limiter.Allow(tenantID, ratelimit.CategoryMutation) {
					w.Header().Set("Retry-After", "60")
					api.WriteErrorCode(w, http.StatusTooManyRequests,
						"mutation rate limit exceeded — retry after 60 seconds",
						api.ErrCodeMutationRateLimitExceeded)
					return
				}
			}

			next(w, r)
		}
	}
}
