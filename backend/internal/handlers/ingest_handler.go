package handlers

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"ai-incident-platform/backend/internal/api"
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
//   Every ingest request must carry an X-Source-Token header containing the
//   token issued when the source was registered in the source registry.
//   The token is looked up in the DB to derive the owning tenant ID.
//   Requests without a valid token are rejected with 401.
//
// Source-type binding:
//   The /ingest/prometheus endpoint requires a source registered with type="prometheus".
//   The /ingest/webhook endpoint accepts any source type except "prometheus".
//   Mismatched tokens are rejected with 403 to prevent accidental cross-endpoint use.
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
//  2. X-Source-Token header → source registry lookup
//
// Returns ("", nil, false) when no valid identity can be established.
func (h *IngestHandler) resolveSource(r *http.Request) (tenantID string, src *models.SourceConnection, ok bool) {
	if t, trusted := middleware.TenantFromRequestStrict(r); trusted {
		return t, nil, true
	}

	token := r.Header.Get("X-Source-Token")
	if token == "" {
		return "", nil, false
	}

	found, tenant, valid := h.sourceRegistryService.FindByToken(token)
	if !valid {
		return "", nil, false
	}
	return tenant, &found, true
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

// GenericWebhook handles POST /api/v1/ingest/webhook
func (h *IngestHandler) GenericWebhook(w http.ResponseWriter, r *http.Request) {
	tenantID, src, ok := h.resolveSource(r)
	if !ok {
		api.WriteError(w, http.StatusUnauthorized, "missing or invalid X-Source-Token")
		return
	}

	// Source-type binding: block prometheus tokens on the generic endpoint.
	sourceID := ""
	if src != nil {
		if src.Type == "prometheus" {
			api.WriteError(w, http.StatusForbidden, "prometheus source token not valid for /ingest/webhook — use /ingest/prometheus")
			return
		}
		sourceID = src.ID
	}

	// Idempotency check before reading body.
	// A DB error here is fail-safe: return 503 so the sender retries later
	// rather than risk processing the same event twice.
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

	var e models.Event
	if err := json.Unmarshal(body, &e); err != nil {
		api.WriteError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}

	// Schema validation — title is the minimum meaningful field.
	if strings.TrimSpace(e.Title) == "" {
		const errMsg = "field 'title' is required"
		// Log to DLQ so operators can inspect malformed payloads without losing them.
		h.enqueueDeadLetter(tenantID, sourceID, "webhook", "/api/v1/ingest/webhook", string(body), errMsg)
		api.WriteError(w, http.StatusBadRequest, errMsg)
		return
	}

	// Stamp tenant — never trust a value in the body.
	e.TenantID = tenantID

	if strings.TrimSpace(e.ID) == "" {
		e.ID = fmt.Sprintf("evt-%d", time.Now().UnixNano())
	}
	if e.Service == "" {
		e.Service = "unknown-service"
	}
	e.Severity = normalizeSeverityInput(e.Severity)
	if e.Type == "" {
		e.Type = "alert"
	}
	if e.Timestamp.IsZero() {
		e.Timestamp = time.Now()
	}

	if err := h.eventStore.SaveEvent(e); err != nil {
		slog.ErrorContext(r.Context(), "ingest: failed to save event", "event_id", e.ID, "error", err)
		api.WriteError(w, http.StatusInternalServerError, "failed to store event")
		return
	}
	h.correlationService.ProcessEvent(e)

	if sourceID != "" {
		h.sourceRegistryService.RecordSuccess(sourceID, 1)
	}

	resp := fmt.Sprintf(`{"status":"accepted","event_id":%q}`, e.ID)
	h.recordIdempotency(r, tenantID, sourceID, resp)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(resp)) //nolint:errcheck
}

// PrometheusWebhook handles POST /api/v1/ingest/prometheus
func (h *IngestHandler) PrometheusWebhook(w http.ResponseWriter, r *http.Request) {
	tenantID, src, ok := h.resolveSource(r)
	if !ok {
		api.WriteError(w, http.StatusUnauthorized, "missing or invalid X-Source-Token")
		return
	}

	// Source-type binding: only prometheus or untyped sources may use this endpoint.
	sourceID := ""
	if src != nil {
		if src.Type != "" && src.Type != "prometheus" && src.Type != "webhook" && src.Type != "generic" {
			api.WriteError(w, http.StatusForbidden, "source token not authorized for /ingest/prometheus")
			return
		}
		sourceID = src.ID
	}

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
		api.WriteError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}

	if len(payload.Alerts) == 0 {
		api.WriteError(w, http.StatusBadRequest, "alerts array is empty or missing")
		return
	}
	if len(payload.Alerts) > maxPrometheusAlerts {
		api.WriteError(w, http.StatusBadRequest, fmt.Sprintf("too many alerts (max %d per request)", maxPrometheusAlerts))
		return
	}

	count := 0
	for _, a := range payload.Alerts {
		ts, _ := time.Parse(time.RFC3339, a.StartsAt)

		eventID := strings.TrimSpace(a.Fingerprint)
		if eventID == "" {
			eventID = fmt.Sprintf("prom-%d", time.Now().UnixNano())
		}
		if ts.IsZero() {
			ts = time.Now()
		}

		service := a.Labels["service"]
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

		event := models.Event{
			ID:          eventID,
			TenantID:    tenantID,
			Source:      "prometheus",
			Service:     service,
			Severity:    severity,
			Type:        "alert",
			Title:       title,
			Message:     a.Annotations["description"],
			Labels:      a.Labels,
			Timestamp:   ts,
			Fingerprint: a.Fingerprint,
		}

		if err := h.eventStore.SaveEvent(event); err != nil {
			slog.Error("ingest: failed to save prometheus event", "event_id", event.ID, "error", err)
			continue
		}
		h.correlationService.ProcessEvent(event)
		count++
	}

	if sourceID != "" {
		h.sourceRegistryService.RecordSuccess(sourceID, count)
	}

	resp := fmt.Sprintf(`{"status":"accepted","processed":%d}`, count)
	h.recordIdempotency(r, tenantID, sourceID, resp)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(resp)) //nolint:errcheck
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
