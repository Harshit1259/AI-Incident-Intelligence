package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// The public ingest endpoints have no auth middleware; the source token is the
// only proof of tenant. A request that names a tenant in a header but carries
// no token must be rejected before anything is stored.
func TestIngestRejectsTenantHeaderWithoutToken(t *testing.T) {
	prom := NewIngestHandler(nil, nil, nil, nil, nil)
	otel := NewOTelHandler(nil, nil, nil, nil, nil, nil, nil)

	cases := []struct {
		name    string
		path    string
		body    string
		handler http.HandlerFunc
	}{
		{"prometheus", "/api/v1/ingest/prometheus", `{"alerts":[{"labels":{"alertname":"X"}}]}`, prom.PrometheusWebhook},
		{"grafana", "/api/v1/ingest/grafana", `{"alerts":[{"labels":{"alertname":"X"}}]}`, prom.GrafanaWebhook},
		{"otel metrics", "/api/v1/otel/metrics", `{"resourceMetrics":[]}`, otel.HandleOTLPMetrics},
		{"otel traces", "/api/v1/otel/traces", `{"resourceSpans":[]}`, otel.HandleOTLPTraces},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, tc.path, strings.NewReader(tc.body))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("X-Tenant-ID", "victim-corp")
			rec := httptest.NewRecorder()
			tc.handler(rec, req)
			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d, want 401 (tenant header must not replace the source token)", rec.Code)
			}
		})
	}
}

func TestSourceTokenFromRequest(t *testing.T) {
	tests := []struct {
		name, xSource, auth, want string
	}{
		{"X-Source-Token", "tok-1", "", "tok-1"},
		{"Bearer (Grafana / Alertmanager authorization)", "", "Bearer tok-2", "tok-2"},
		{"bearer is case-insensitive", "", "bearer tok-3", "tok-3"},
		{"X-Source-Token wins", "tok-1", "Bearer tok-2", "tok-1"},
		{"Basic is ignored", "", "Basic dXNlcjpwYXNz", ""},
		{"nothing", "", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/", nil)
			if tt.xSource != "" {
				req.Header.Set("X-Source-Token", tt.xSource)
			}
			if tt.auth != "" {
				req.Header.Set("Authorization", tt.auth)
			}
			if got := sourceTokenFromRequest(req); got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}
