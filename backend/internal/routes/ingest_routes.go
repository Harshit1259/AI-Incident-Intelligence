package routes

import (
	"net/http"

	"ai-incident-platform/backend/internal/handlers"
	"ai-incident-platform/backend/internal/middleware"
)

func registerIngestRoutes(
	mux *http.ServeMux,
	withOps func(http.HandlerFunc) http.Handler,
	withAuth func(http.HandlerFunc) http.Handler,
	rateLimiter *middleware.WebhookRateLimiter,
	ingestHandler *handlers.IngestHandler,
	statusHandler *handlers.StatusHandler,
	changeIntelligenceHandler *handlers.ChangeIntelligenceHandler,
	otelHandler *handlers.OTelHandler,
) {
	// rl wraps a HandlerFunc with rate limiting then the standard ops middleware chain.
	rl := func(h http.HandlerFunc) http.Handler {
		return withOps(rateLimiter.Middleware(h))
	}

	// Supported alert sources: Prometheus/Alertmanager here (Grafana sources
	// set up before /ingest/grafana existed also post here); Grafana, Zabbix,
	// and OpenTelemetry and Jaeger (via OTLP) below.
	// Every other integration was removed for now — see plan.md.
	mux.Handle("/api/v1/ingest/prometheus", rl(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			ingestHandler.PrometheusWebhook(w, r)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	}))

	// Grafana Alerting webhook contact point.
	mux.Handle("/api/v1/ingest/grafana", rl(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		ingestHandler.GrafanaWebhook(w, r)
	}))

	// Zabbix webhook media type (our script) — problems, recoveries, updates.
	mux.Handle("/api/v1/ingest/zabbix", rl(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		ingestHandler.ZabbixWebhook(w, r)
	}))

	// Dead-letter queue — auth required (operator visibility into failed payloads)
	mux.Handle("/api/v1/ingest/dlq", withAuth(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		ingestHandler.ListDeadLetters(w, r)
	}))

	// Status page — public, no auth.
	// Exact match handles GET /api/v1/status; prefix match handles tenant sub-paths,
	// /subscribe, and /unsubscribe/{token}.
	mux.Handle("/api/v1/status", withOps(statusHandler.Handle))
	mux.Handle("/api/v1/status/", withOps(statusHandler.Handle))

	// ── Change Intelligence ingest ─────────────────────────────────────────────
	// Feature flag toggles — operator-authenticated; no rate limit needed here
	// as these come from internal CI/CD pipelines, not external actors.
	mux.Handle("/api/v1/ingest/feature-flags", withAuth(methodHandler(map[string]http.HandlerFunc{
		http.MethodPost: changeIntelligenceHandler.HandleIngestFeatureFlag,
	})))

	// Config drift snapshot — agent or CI/CD posts current config; service
	// compares against stored baseline and records any deviations.
	mux.Handle("/api/v1/ingest/config-drift", withAuth(methodHandler(map[string]http.HandlerFunc{
		http.MethodPost: changeIntelligenceHandler.HandleConfigDrift,
	})))

	// Recent changes dashboard (read-only).
	mux.Handle("/api/v1/change-intelligence/recent", withAuth(methodHandler(map[string]http.HandlerFunc{
		http.MethodGet: changeIntelligenceHandler.HandleGetRecent,
	})))

	// ── OTel-native ingest (OTLP/HTTP) ────────────────────────────────────────
	// These endpoints accept OTLP/JSON payloads from OTel SDKs and Collectors.
	// No JWT required — callers identify via X-Source-Token.
	mux.Handle("/api/v1/otel/logs", rl(otelHandler.HandleOTLPLogs))
	mux.Handle("/api/v1/otel/metrics", rl(otelHandler.HandleOTLPMetrics))
	mux.Handle("/api/v1/otel/traces", rl(otelHandler.HandleOTLPTraces))

	// Custom sources (/api/v1/ingest/custom/{type}) and the schema registry
	// (/api/schema-registry) are switched off for now — only the five
	// supported integrations are offered. The handlers remain; see plan.md.
}
