package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"ai-incident-platform/backend/internal/middleware"
)

func TestSecurityHeaders_AllHeadersPresent(t *testing.T) {
	handler := middleware.SecurityHeaders(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/", nil))

	required := map[string]string{
		"X-Content-Type-Options":    "nosniff",
		"X-Frame-Options":           "DENY",
		"Content-Security-Policy":   "default-src 'none'",
		"Referrer-Policy":           "strict-origin-when-cross-origin",
		"Cache-Control":             "no-store",
		"X-XSS-Protection":          "0",
	}

	for header, want := range required {
		got := rr.Header().Get(header)
		if got != want {
			t.Errorf("%s: got %q, want %q", header, got, want)
		}
	}

	// HSTS must be present and contain max-age
	hsts := rr.Header().Get("Strict-Transport-Security")
	if hsts == "" {
		t.Error("Strict-Transport-Security header missing")
	}
	if len(hsts) < 10 {
		t.Errorf("Strict-Transport-Security looks too short: %q", hsts)
	}
}

func TestSecurityHeaders_PresentOn404(t *testing.T) {
	mux := http.NewServeMux()
	handler := middleware.SecurityHeaders(mux) // mux with no routes → always 404

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/nonexistent", nil))

	if rr.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Error("X-Content-Type-Options missing on 404 response")
	}
	if rr.Header().Get("X-Frame-Options") != "DENY" {
		t.Error("X-Frame-Options missing on 404 response")
	}
}

func TestSecurityHeaders_PresentOn500(t *testing.T) {
	handler := middleware.SecurityHeaders(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/", nil))

	if rr.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Error("X-Content-Type-Options missing on 500 response")
	}
}
