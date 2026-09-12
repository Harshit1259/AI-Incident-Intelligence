package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"ai-incident-platform/backend/internal/api"
	"ai-incident-platform/backend/internal/ingest"
	"ai-incident-platform/backend/internal/middleware"
	"ai-incident-platform/backend/internal/models"
	"ai-incident-platform/backend/internal/services"
	"ai-incident-platform/backend/internal/store"
)

const (
	maxIngestBodyBytes   = 1 << 20 // 1 MiB — prevents payload bombing
	maxPrometheusAlerts  = 1000    // max alerts per Prometheus batch
	idempotencyKeyHeader = "X-Idempotency-Key"
)

// IngestHandler handles webhook ingestion from external monitoring systems.
//
// Tenant resolution for public (unauthenticated) ingest:
//   Every ingest request must carry the source token (X-Source-Token header,
//   or Authorization: Bearer <token>) issued when the source was registered.
//   The token is looked up in the DB to derive the owning tenant ID.
//   Requests without a valid token are rejected with 401.
//
// Source-type binding:
//   /ingest/prometheus accepts sources registered as "prometheus" or "grafana"
//   (older Grafana setups post there), /ingest/grafana only "grafana"; both
//   accept untyped sources. Other tokens are rejected with 403 to prevent
//   cross-endpoint use.
//   The generic /ingest/webhook endpoint was removed — see plan.md.
//
// Idempotency:
//   If the caller sends X-Idempotency-Key the response is deduplicated for 24 hours.
//   A duplicate key returns 200 with the original response; no event is re-processed.
type IngestHandler struct {
	eventStore            *store.EventStore
	correlationService    *services.CorrelationService
	sourceRegistryService *services.SourceRegistryService
	deadLetterStore       *store.DeadLetterStore
	idempotencyStore      *store.IdempotencyStore
	noter                 incidentNoter
}

func NewIngestHandler(
	es *store.EventStore,
	cs *services.CorrelationService,
	srs *services.SourceRegistryService,
	dlq *store.DeadLetterStore,
	idm *store.IdempotencyStore,
) *IngestHandler {
	return &IngestHandler{
		eventStore:            es,
		correlationService:    cs,
		sourceRegistryService: srs,
		deadLetterStore:       dlq,
		idempotencyStore:      idm,
	}
}

// resolveSource derives the tenant ID and source record from the request.
// Resolution order:
//  1. JWT claims (authenticated callers — no source record returned)
//  2. Source token (X-Source-Token or Authorization: Bearer) → source registry lookup
//
// Returns ("", nil, false) when no valid identity can be established.
func (h *IngestHandler) resolveSource(r *http.Request) (tenantID string, src *models.SourceConnection, ok bool) {
	if t, trusted := middleware.TenantFromRequestStrict(r); trusted {
		return t, nil, true
	}

	token := sourceTokenFromRequest(r)
	if token == "" {
		return "", nil, false
	}

	found, tenant, valid := h.sourceRegistryService.FindByToken(token)
	if !valid {
		return "", nil, false
	}
	return tenant, &found, true
}

// sourceTokenFromRequest returns the ingest source token from X-Source-Token
// or, failing that, from "Authorization: Bearer <token>". Grafana's webhook
// contact point and Alertmanager's http_config.authorization can set the
// Authorization header but not always a custom one.
func sourceTokenFromRequest(r *http.Request) string {
	if t := strings.TrimSpace(r.Header.Get("X-Source-Token")); t != "" {
		return t
	}
	scheme, cred, ok := strings.Cut(strings.TrimSpace(r.Header.Get("Authorization")), " ")
	if ok && strings.EqualFold(scheme, "Bearer") {
		return strings.TrimSpace(cred)
	}
	return ""
}

// checkIdempotency returns (cachedResponse, seen, err).
// When err != nil the caller MUST return 503 — never proceed with processing,
// or the same event can be ingested twice during a DB hiccup.
func (h *IngestHandler) checkIdempotency(r *http.Request, tenantID string) (string, bool, error) {
	key := r.Header.Get(idempotencyKeyHeader)
	if key == "" || h.idempotencyStore == nil {
		return "", false, nil
	}
	cached, seen, err := h.idempotencyStore.Check(key, tenantID)
	if err != nil {
		slog.Error("ingest: idempotency check error", "error", err)
		return "", false, err
	}
	return cached, seen, nil
}

// recordIdempotency saves the idempotency key after successful processing.
func (h *IngestHandler) recordIdempotency(r *http.Request, tenantID, sourceID, response string) {
	key := r.Header.Get(idempotencyKeyHeader)
	if key == "" || h.idempotencyStore == nil {
		return
	}
	if err := h.idempotencyStore.Record(key, tenantID, sourceID, response); err != nil {
		slog.Error("ingest: failed to record idempotency key", "error", err)
	}
}

// enqueueDeadLetter persists a failed payload so it is not silently dropped.
func (h *IngestHandler) enqueueDeadLetter(tenantID, sourceID, sourceType, endpoint, rawPayload, errMsg string) {
	if h.deadLetterStore == nil {
		return
	}
	if err := h.deadLetterStore.Enqueue(store.DeadLetterEntry{
		TenantID:   tenantID,
		SourceID:   sourceID,
		SourceType: sourceType,
		Endpoint:   endpoint,
		RawPayload: rawPayload,
		Error:      errMsg,
	}); err != nil {
		slog.Error("ingest: failed to enqueue dead letter", "error", err)
	}
}

// PrometheusWebhook handles POST /api/v1/ingest/prometheus
//
// Grafana sources may also post here (the setup guide pointed Grafana at this
// endpoint before /ingest/grafana existed); their payloads get the Grafana
// mapping.
func (h *IngestHandler) PrometheusWebhook(w http.ResponseWriter, r *http.Request) {
	h.serveAlertWebhook(w, r, "/ingest/prometheus",
		[]string{"prometheus", "grafana", "webhook", "generic"},
		func(body []byte, tenantID string, src *models.SourceConnection) (int, error) {
			if src != nil && src.Type == "grafana" {
				return h.ingestGrafanaPayload(body, tenantID, src.ID, src.Name)
			}
			return h.ingestAlertmanagerPayload(body, tenantID, sourceIDOf(src))
		})
}

func sourceIDOf(src *models.SourceConnection) string {
	if src == nil {
		return ""
	}
	return src.ID
}

// serveAlertWebhook is the request handling shared by the alert webhooks:
// source token → tenant, source-type binding (untyped sources are allowed
// everywhere), idempotency, body size limit, then ingest.
func (h *IngestHandler) serveAlertWebhook(w http.ResponseWriter, r *http.Request, endpoint string, allowedTypes []string,
	ingestFn func(body []byte, tenantID string, src *models.SourceConnection) (int, error)) {
	tenantID, src, ok := h.resolveSource(r)
	if !ok {
		api.WriteError(w, http.StatusUnauthorized, "missing or invalid source token (send X-Source-Token or Authorization: Bearer <token>)")
		return
	}
	if src != nil && src.Type != "" && !containsType(allowedTypes, src.Type) {
		api.WriteError(w, http.StatusForbidden, "source token not authorized for "+endpoint)
		return
	}
	sourceID := sourceIDOf(src)

	// Idempotency check — fail safe on DB error.
	cached, seen, err := h.checkIdempotency(r, tenantID)
	if err != nil {
		api.WriteError(w, http.StatusServiceUnavailable, "service temporarily unavailable")
		return
	}
	if seen {
		w.Header().Set("X-Idempotent-Replay", "true")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(cached)) //nolint:errcheck
		return
	}

	// Enforce body size limit.
	r.Body = http.MaxBytesReader(w, r.Body, maxIngestBodyBytes)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		api.WriteError(w, http.StatusRequestEntityTooLarge, "request body too large (max 1 MiB)")
		return
	}

	count, err := ingestFn(body, tenantID, src)
	if err != nil {
		api.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	resp := fmt.Sprintf(`{"status":"accepted","processed":%d}`, count)
	h.recordIdempotency(r, tenantID, sourceID, resp)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(resp)) //nolint:errcheck
}

func containsType(types []string, t string) bool {
	for _, v := range types {
		if v == t {
			return true
		}
	}
	return false
}

// ingestAlertmanagerPayload parses an Alertmanager-format body (Prometheus
// Alertmanager or Grafana Alerting), stores each alert and runs it through
// correlation. The caller has already established the tenant. It returns how
// many alerts were stored; an error means the body was rejected as a whole.
func (h *IngestHandler) ingestAlertmanagerPayload(body []byte, tenantID, sourceID string) (int, error) {
	var payload struct {
		Alerts []struct {
			Status      string            `json:"status"`
			Labels      map[string]string `json:"labels"`
			Annotations map[string]string `json:"annotations"`
			StartsAt    string            `json:"startsAt"`
			Fingerprint string            `json:"fingerprint"`
		} `json:"alerts"`
	}

	if err := json.Unmarshal(body, &payload); err != nil {
		return 0, fmt.Errorf("invalid JSON: %w", err)
	}

	if len(payload.Alerts) == 0 {
		return 0, errors.New("alerts array is empty or missing")
	}
	if len(payload.Alerts) > maxPrometheusAlerts {
		return 0, fmt.Errorf("too many alerts (max %d per request)", maxPrometheusAlerts)
	}

	count := 0
	for _, a := range payload.Alerts {
		ts, _ := time.Parse(time.RFC3339, a.StartsAt)

		// The event ID is Alertmanager's fingerprint scoped to the tenant, so
		// firing, re-notify and resolved messages for one alert update one row
		// — and two tenants with identical labels never share a row.
		eventID := fmt.Sprintf("prom-%d", time.Now().UnixNano())
		if fp := strings.TrimSpace(a.Fingerprint); fp != "" {
			eventID = "am-" + tenantID + "-" + fp
		}
		status := "firing"
		if strings.EqualFold(strings.TrimSpace(a.Status), "resolved") {
			status = "resolved"
		}
		if ts.IsZero() {
			ts = time.Now()
		}

		// A service label that names the exporter (kube-prometheus-stack sets
		// e.g. "kube-state-metrics") is replaced with the workload it is about.
		service, derivedFrom := ingest.DeriveAlertService(a.Labels)
		if service == "" {
			service = a.Labels["alertname"]
		}
		if service == "" {
			service = "unknown-service"
		}

		severity := normalizeSeverityInput(a.Labels["severity"])

		title := a.Annotations["summary"]
		if title == "" {
			title = a.Labels["alertname"]
		}

		// Keep two annotations with the event by copying them into labels:
		//  - value: Alertmanager does not send the metric value; customers who
		//    want value ranges on mutes add `value: "{{ $value }}"` to rules.
		//  - runbook_url: shown as a link on the alert in the incident.
		labels := a.Labels
		extra := map[string]string{
			models.PrometheusValueLabel:   strings.TrimSpace(a.Annotations["value"]),
			models.PrometheusRunbookLabel: strings.TrimSpace(a.Annotations["runbook_url"]),
		}
		if derivedFrom != "" {
			extra[ingest.LabelServiceOriginal] = a.Labels["service"]
			extra[ingest.LabelServiceDerivedFrom] = derivedFrom
		}
		copied := false
		for key, v := range extra {
			if v == "" {
				continue
			}
			if !copied { // never mutate the decoded payload's map
				labels = make(map[string]string, len(a.Labels)+len(extra))
				for k, lv := range a.Labels {
					labels[k] = lv
				}
				copied = true
			}
			labels[key] = v
		}

		event := models.Event{
			ID:           eventID,
			TenantID:     tenantID,
			Source:       "prometheus",
			Service:      service,
			Severity:     severity,
			Type:         "alert",
			Title:        title,
			Message:      a.Annotations["description"],
			Labels:       labels,
			Timestamp:    ts,
			Fingerprint:  a.Fingerprint,
			IngestSchema: "prometheus",
			AlertStatus:  status,
		}

		if err := h.eventStore.SaveEvent(event); err != nil {
			slog.Error("ingest: failed to save prometheus event", "event_id", event.ID, "error", err)
			continue
		}
		// A resolved message is not a new occurrence: it must not merge into
		// an incident or bump its count. It only feeds auto-close.
		if status == "resolved" {
			h.correlationService.ProcessResolved(event)
		} else {
			h.correlationService.ProcessEvent(event)
		}
		count++
	}

	if sourceID != "" {
		h.sourceRegistryService.RecordSuccess(sourceID, count)
	}
	return count, nil
}

// SendTestAlert runs a synthetic Alertmanager payload through the same parsing
// and pipeline as a real webhook, for a source the caller already owns. It
// skips the token check: the caller is authenticated and the source was looked
// up within their tenant (ingest tokens may be stored hashed, so the plaintext
// token is not available to replay).
func (h *IngestHandler) SendTestAlert(tenantID, sourceID string, body []byte) (int, error) {
	return h.ingestAlertmanagerPayload(body, tenantID, sourceID)
}

// ListDeadLetters handles GET /api/v1/ingest/dlq
func (h *IngestHandler) ListDeadLetters(w http.ResponseWriter, r *http.Request) {
	tenantID := middleware.TenantFromRequest(r)
	if h.deadLetterStore == nil {
		api.WriteJSON(w, http.StatusOK, map[string]interface{}{"items": []interface{}{}})
		return
	}
	entries, err := h.deadLetterStore.List(tenantID, 100)
	if err != nil {
		api.WriteError(w, http.StatusInternalServerError, "failed to fetch dead-letter queue")
		return
	}
	if entries == nil {
		entries = []store.DeadLetterEntry{}
	}
	api.WriteJSON(w, http.StatusOK, map[string]interface{}{"items": entries})
}

// normalizeSeverityInput maps any caller-supplied severity string to the
// platform's canonical set: critical / high / medium / low / info.
// Unknown values default to "medium" rather than being stored verbatim.
func normalizeSeverityInput(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "critical", "crit", "fatal", "p0", "emergency":
		return "critical"
	case "high", "error", "err", "p1", "major":
		return "high"
	case "warning", "warn", "medium", "med", "p2", "moderate":
		return "medium"
	case "low", "minor", "p3":
		return "low"
	case "info", "information", "informational", "notice", "p4", "debug":
		return "info"
	default:
		return "medium"
	}
}
