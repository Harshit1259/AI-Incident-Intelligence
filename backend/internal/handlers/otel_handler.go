package handlers

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"ai-incident-platform/backend/internal/api"
	"ai-incident-platform/backend/internal/ingest"
	"ai-incident-platform/backend/internal/middleware"
	"ai-incident-platform/backend/internal/models"
	"ai-incident-platform/backend/internal/services"
	"ai-incident-platform/backend/internal/store"
)

// OTelHandler handles OTLP/HTTP ingest (logs, metrics, traces) and custom
// source-type webhooks that are backed by a SchemaMapping.
//
// Endpoints:
//
//	POST /v1/otel/logs              — OTLP Logs JSON
//	POST /v1/otel/metrics           — OTLP Metrics JSON
//	POST /v1/otel/traces            — OTLP Traces JSON
//	POST /v1/ingest/custom/{type}   — custom source mapped via SchemaRegistry
type OTelHandler struct {
	eventStore            *store.EventStore
	correlationService    *services.CorrelationService
	schemaRegistrySvc     *services.SchemaRegistryService
	sourceRegistryService *services.SourceRegistryService
	mapperRegistry        *ingest.MapperRegistry
	deadLetterStore       *store.DeadLetterStore
	idempotencyStore      *store.IdempotencyStore
}

func NewOTelHandler(
	es *store.EventStore,
	cs *services.CorrelationService,
	srs *services.SchemaRegistryService,
	sources *services.SourceRegistryService,
	mr *ingest.MapperRegistry,
	dlq *store.DeadLetterStore,
	idm *store.IdempotencyStore,
) *OTelHandler {
	return &OTelHandler{
		eventStore:            es,
		correlationService:    cs,
		schemaRegistrySvc:     srs,
		sourceRegistryService: sources,
		mapperRegistry:        mr,
		deadLetterStore:       dlq,
		idempotencyStore:      idm,
	}
}

func (h *OTelHandler) HandleOTLPLogs(w http.ResponseWriter, r *http.Request) {
	h.handleOTelIngest(w, r, "otel-logs")
}

func (h *OTelHandler) HandleOTLPMetrics(w http.ResponseWriter, r *http.Request) {
	h.handleOTelIngest(w, r, "otel-metrics")
}

func (h *OTelHandler) HandleOTLPTraces(w http.ResponseWriter, r *http.Request) {
	h.handleOTelIngest(w, r, "otel-traces")
}

// HandleCustomIngest accepts a JSON payload for a user-defined source type.
// The source type is the last path segment: /v1/ingest/custom/{type}
func (h *OTelHandler) HandleCustomIngest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		api.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	parts := strings.Split(strings.TrimSuffix(r.URL.Path, "/"), "/")
	sourceType := parts[len(parts)-1]
	if sourceType == "" {
		api.WriteError(w, http.StatusBadRequest, "source type required in path")
		return
	}
	h.handleOTelIngest(w, r, "custom:"+sourceType)
}

func (h *OTelHandler) handleOTelIngest(w http.ResponseWriter, r *http.Request, sourceType string) {
	if r.Method != http.MethodPost {
		api.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	// Resolve the tenant first: the idempotency key is scoped per tenant, and
	// an unauthenticated caller should be rejected before any database work.
	tenantID, src, authed := h.resolveSource(r)
	if !authed {
		api.WriteError(w, http.StatusUnauthorized, "missing or invalid source token (send X-Source-Token or Authorization: Bearer <token>)")
		return
	}

	// Source-type binding. src is nil for JWT-authenticated callers, who are
	// already trusted and are not bound to a single endpoint.
	if src != nil {
		if reason := authorizeSourceType(src.Type, sourceType); reason != "" {
			api.WriteError(w, http.StatusForbidden, reason)
			return
		}
	}

	// Idempotency check — fail safe on DB error.
	//
	// Check MUST use the same tenant Record writes with. This previously passed
	// an empty string while Record saved the real tenant, so the lookup could
	// never match its own row: every retry was processed again and the feature
	// was silently a no-op on all four routes that share this handler.
	iKey := r.Header.Get("X-Idempotency-Key")
	if iKey != "" && h.idempotencyStore != nil {
		resp, seen, iErr := h.idempotencyStore.Check(iKey, tenantID)
		if iErr != nil {
			slog.ErrorContext(r.Context(), "otel-ingest: idempotency check error", "error", iErr)
			api.WriteError(w, http.StatusServiceUnavailable, "service temporarily unavailable")
			return
		}
		if seen {
			w.Header().Set("X-Idempotency-Replayed", "true")
			if isProtobufRequest(r) {
				writeOTLPProtobufSuccess(w)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(resp))
			return
		}
	}

	body, status, err := readOTLPBody(r)
	if err != nil {
		api.WriteError(w, status, err.Error())
		return
	}
	if isProtobufRequest(r) {
		// OTLP/HTTP protobuf (the Collector's default and most SDKs'):
		// convert to OTLP JSON and use the same mappers.
		if body, err = otlpProtobufToJSON(sourceType, body); err != nil {
			api.WriteError(w, http.StatusBadRequest, err.Error())
			return
		}
	}

	// For custom types, lazy-load mapper from DB if not already registered.
	if strings.HasPrefix(sourceType, "custom:") {
		rawType := strings.TrimPrefix(sourceType, "custom:")
		if _, ok := h.mapperRegistry.Get(sourceType); !ok && h.schemaRegistrySvc != nil {
			if m, err := h.schemaRegistrySvc.Get(tenantID, rawType); err == nil && m.Enabled {
				h.mapperRegistry.Register(ingest.NewCustomMapper(*m))
			}
		}
	}

	accepted, err := h.mapAndProcess(tenantID, sourceType, body)
	if errors.Is(err, errUnknownSourceType) {
		api.WriteError(w, http.StatusBadRequest, fmt.Sprintf("unknown source type: %s", sourceType))
		return
	}
	if err != nil {
		slog.ErrorContext(r.Context(), "otel-ingest: map error", "source_type", sourceType, "error", err)
		if h.deadLetterStore != nil {
			_ = h.deadLetterStore.Enqueue(store.DeadLetterEntry{
				TenantID:   tenantID,
				SourceType: sourceType,
				RawPayload: string(body),
				Error:      err.Error(),
			})
		}
		api.WriteError(w, http.StatusBadRequest, fmt.Sprintf("mapping error: %v", err))
		return
	}

	// Record delivery against the source so its last-seen time and health stay
	// accurate — the same signal the notification feed reads to spot a source
	// that has gone silent.
	if src != nil && h.sourceRegistryService != nil {
		h.sourceRegistryService.RecordSuccess(src.ID, accepted)
	}

	resp := map[string]any{
		"status":        "accepted",
		"source_type":   sourceType,
		"events_queued": accepted,
	}
	respBytes, _ := json.Marshal(resp)

	if iKey != "" && h.idempotencyStore != nil {
		_ = h.idempotencyStore.Record(iKey, tenantID, sourceType, string(respBytes))
	}

	if isProtobufRequest(r) {
		writeOTLPProtobufSuccess(w)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	_, _ = w.Write(respBytes)
}

// isProtobufRequest reports an OTLP/HTTP protobuf request
// (Content-Type: application/x-protobuf).
func isProtobufRequest(r *http.Request) bool {
	return strings.Contains(strings.ToLower(r.Header.Get("Content-Type")), "protobuf")
}

// writeOTLPProtobufSuccess answers a protobuf request the way OTLP/HTTP
// requires: 200 with the same Content-Type. An empty body is a valid, empty
// Export*ServiceResponse (no partial success to report).
func writeOTLPProtobufSuccess(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/x-protobuf")
	w.WriteHeader(http.StatusOK)
}

// unspecialisedTypes may post to any ingest endpoint. A source registered as
// "generic" or "webhook" (or with no type at all, from before types existed)
// makes no claim about what it sends, so binding it to one endpoint would only
// break working setups. This mirrors how /ingest/prometheus already treats them.
var unspecialisedTypes = map[string]bool{"": true, "generic": true, "webhook": true}

var errUnknownSourceType = errors.New("unknown source type")

// mapAndProcess maps a payload with the mapper for sourceType, stores each
// resulting event and runs it through correlation. The caller has already
// established the tenant. It returns how many events were stored; an error
// means the payload was rejected (errUnknownSourceType, or a mapping error).
func (h *OTelHandler) mapAndProcess(tenantID, sourceType string, body []byte) (int, error) {
	mapper, ok := h.mapperRegistry.Get(sourceType)
	if !ok {
		return 0, errUnknownSourceType
	}
	events, err := mapper.Map(body, tenantID)
	if err != nil {
		return 0, err
	}
	accepted := 0
	for _, ie := range events {
		ev := ingest.IngestEventToEvent(ie)
		if err := h.eventStore.SaveEvent(ev); err != nil {
			slog.Error("otel-ingest: failed to save event", "event_id", ev.ID, "source_type", sourceType, "error", err)
			continue
		}
		if h.correlationService != nil {
			h.correlationService.ProcessEvent(ev)
		}
		accepted++
	}
	return accepted, nil
}

// maxOTLPDecompressedBytes caps a gzip body after decompression, so a small
// compressed request cannot expand into an unbounded one.
const maxOTLPDecompressedBytes = 8 << 20 // 8 MiB

// readOTLPBody reads an OTLP/HTTP request body, gunzipping it when
// Content-Encoding is gzip (the Collector's otlphttp default). The caller
// decides between JSON and protobuf from Content-Type.
// It returns the HTTP status to use when it fails.
func readOTLPBody(r *http.Request) ([]byte, int, error) {
	raw, err := io.ReadAll(io.LimitReader(r.Body, maxIngestBodyBytes+1))
	if err != nil {
		return nil, http.StatusBadRequest, errors.New("failed to read request body")
	}
	if len(raw) > maxIngestBodyBytes {
		return nil, http.StatusRequestEntityTooLarge, errors.New("request body too large (max 1 MiB)")
	}

	switch enc := strings.ToLower(strings.TrimSpace(r.Header.Get("Content-Encoding"))); enc {
	case "", "identity":
		return raw, 0, nil
	case "gzip":
		zr, err := gzip.NewReader(bytes.NewReader(raw))
		if err != nil {
			return nil, http.StatusBadRequest, errors.New("invalid gzip body")
		}
		defer zr.Close()
		out, err := io.ReadAll(io.LimitReader(zr, maxOTLPDecompressedBytes+1))
		if err != nil {
			return nil, http.StatusBadRequest, errors.New("invalid gzip body")
		}
		if len(out) > maxOTLPDecompressedBytes {
			return nil, http.StatusRequestEntityTooLarge, errors.New("decompressed body too large (max 8 MiB)")
		}
		return out, 0, nil
	default:
		return nil, http.StatusUnsupportedMediaType,
			fmt.Errorf("unsupported Content-Encoding %q; use gzip or none", enc)
	}
}

// authorizeSourceType enforces source-type binding for the OTLP and custom
// ingest endpoints, the same protection /ingest/prometheus has.
//
// Without it, a token issued for one integration could post to any other:
// a Datadog token could write OTLP traces, and a token for custom source "A"
// could impersonate custom source "B" — which matters because the source type
// selects the schema mapping used to interpret the payload.
//
// Rules, where `endpoint` is the handler's own identifier
// ("otel-logs", "otel-metrics", "otel-traces", "custom:<type>"):
//
//	unspecialised source            → allowed anywhere
//	"otel"                          → allowed on any of the three OTLP endpoints
//	"otel-logs"/-metrics/-traces    → only its own signal
//	"custom:<type>"                 → only the matching custom endpoint
//	anything else                   → rejected
//
// Returns "" when authorised, or a human-readable reason when not.
func authorizeSourceType(srcType, endpoint string) string {
	if unspecialisedTypes[srcType] {
		return ""
	}
	if srcType == endpoint {
		return ""
	}
	// A source registered simply as "otel" may use any OTLP signal endpoint.
	if srcType == "otel" && strings.HasPrefix(endpoint, "otel-") {
		return ""
	}
	// Jaeger has no alerting of its own; its traces reach us over OTLP.
	if srcType == "jaeger" && endpoint == "otel-traces" {
		return ""
	}
	return fmt.Sprintf("source token of type %q is not authorized for this endpoint (expects %q)", srcType, endpoint)
}

// resolveSource derives the tenant for an ingest request and, when the caller
// authenticated with a source token, the source record behind it.
//
// The previous implementation read the tenant straight out of the token string
// ("t_{tenantID}_{random}") without ever consulting the database. That format
// was never issued by anything — CreateSource mints randomHex(16) — so in
// practice ANY value starting with "t_" granted write access to the tenant
// named inside it, and any other non-empty value silently mapped to "default".
// A caller who could reach these routes could inject events into any tenant by
// guessing its name.
//
// Tokens are now verified against the source registry, exactly as the other
// public ingest endpoints do (see IngestHandler.resolveSource). An unknown
// token resolves to no tenant, and the request is rejected.
func (h *OTelHandler) resolveSource(r *http.Request) (tenantID string, src *models.SourceConnection, ok bool) {
	// Authenticated callers (a route mounted behind withAuth) carry trusted claims.
	if t, trusted := middleware.TenantFromRequestStrict(r); trusted {
		return t, nil, true
	}

	token := sourceTokenFromRequest(r)
	if token == "" || h.sourceRegistryService == nil {
		return "", nil, false
	}

	found, tenant, valid := h.sourceRegistryService.FindByToken(token)
	if !valid {
		return "", nil, false
	}
	return tenant, &found, true
}
