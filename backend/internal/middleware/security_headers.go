package middleware

import "net/http"

// SecurityHeaders wraps a handler and sets security-relevant HTTP response
// headers on every reply. Placing this at the outermost layer of the handler
// chain guarantees coverage for all routes including 404s and panics.
//
// Headers applied:
//   - X-Content-Type-Options: nosniff        — stop MIME-type sniffing
//   - X-Frame-Options: DENY                  — block clickjacking
//   - Referrer-Policy: strict-origin-when-cross-origin
//   - Permissions-Policy: geolocation=(), microphone=(), camera=()
//   - Content-Security-Policy: default-src 'none' — API only; no resources served
//   - Strict-Transport-Security              — enforce HTTPS (1 year + subdomains)
//   - Cache-Control: no-store                — prevent caching of API responses
//   - X-XSS-Protection: 0                   — disable legacy mode; use CSP instead
//
// SOC2 / OWASP reference: A02 (Cryptographic Failures), A05 (Security Misconfiguration)
func SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()

		// Prevent MIME sniffing — browsers must honour Content-Type as-is.
		h.Set("X-Content-Type-Options", "nosniff")

		// Deny embedding in frames — eliminates clickjacking surface.
		h.Set("X-Frame-Options", "DENY")

		// Control how much referrer info is sent in cross-origin requests.
		h.Set("Referrer-Policy", "strict-origin-when-cross-origin")

		// Disable browser features the API never uses.
		h.Set("Permissions-Policy", "geolocation=(), microphone=(), camera=()")

		// This is a pure JSON API — no resources should be loaded from it.
		// 'none' blocks all content types, which is correct for an API server.
		h.Set("Content-Security-Policy", "default-src 'none'")

		// Instruct clients to only reach this origin via HTTPS for 1 year.
		// Browsers ignore this header over plain HTTP, so it is safe to set
		// unconditionally; it takes effect the first time the client connects
		// over HTTPS (e.g., via the TLS-terminating load balancer).
		h.Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")

		// Prevent caches (proxy, browser) from storing API responses.
		h.Set("Cache-Control", "no-store")

		// Disable the legacy XSS auditor — modern browsers no longer use it
		// and enabling it can introduce its own vulnerabilities. CSP is the
		// correct defence instead.
		h.Set("X-XSS-Protection", "0")

		next.ServeHTTP(w, r)
	})
}
