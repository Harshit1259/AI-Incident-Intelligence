package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Validation runs before any database work, so a nil service is enough.
func TestCreateSourceValidation(t *testing.T) {
	h := NewSourceHandler(nil, nil)
	tests := []struct {
		name, body, want string
	}{
		{"missing name", `{"type":"prometheus"}`, "name is required"},
		{"removed integration", `{"name":"x","type":"datadog"}`, "type must be one of"},
		{"generic no longer allowed", `{"name":"x","type":"generic"}`, "type must be one of"},
		{"empty type", `{"name":"x"}`, "type must be one of"},
		{"name too long", `{"name":"` + strings.Repeat("a", 101) + `","type":"otel"}`, "100 characters"},
		{"bad json", `{`, "invalid request body"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/v1/sources", strings.NewReader(tt.body))
			rec := httptest.NewRecorder()
			h.CreateSource(rec, req)
			if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), tt.want) {
				t.Fatalf("got %d %s, want 400 containing %q", rec.Code, rec.Body.String(), tt.want)
			}
		})
	}
}

func TestHandleSourceByIDRouting(t *testing.T) {
	h := NewSourceHandler(nil, nil)
	for _, tc := range []struct {
		method, path string
		want         int
	}{
		{http.MethodGet, "/api/v1/sources/source-1", http.StatusMethodNotAllowed},
		{http.MethodPost, "/api/v1/sources/source-1", http.StatusMethodNotAllowed},
		{http.MethodDelete, "/api/v1/sources/source-1/rotate", http.StatusMethodNotAllowed},
		{http.MethodPost, "/api/v1/sources/", http.StatusNotFound},
	} {
		req := httptest.NewRequest(tc.method, tc.path, nil)
		rec := httptest.NewRecorder()
		h.HandleSourceByID(rec, req)
		if rec.Code != tc.want {
			t.Errorf("%s %s = %d, want %d", tc.method, tc.path, rec.Code, tc.want)
		}
	}
}
