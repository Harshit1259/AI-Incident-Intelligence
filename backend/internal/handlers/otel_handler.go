package handlers

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"ai-incident-platform/backend/internal/api"
	"ai-incident-platform/backend/internal/ingest"
	"ai-incident-platform/backend/internal/middleware"
	"ai-incident-platform/backend/internal/services"
	"ai-incident-platform/backend/internal/store"
)

// OTelHandler handles OTLP/HTTP ingest (logs, metrics, traces) and custom
// source-type webhooks that are backed by a SchemaMapping.
//
// Endpoints:
//   POST /v1/otel/logs              — OTLP Logs JSON
//   POST /v1/otel/metrics           — OTLP Metrics JSON
//   POST /v1/otel/traces            — OTLP Traces JSON
//   POST /v1/ingest/custom/{type}   — custom source mapped via SchemaRegistry
type OTelHandler struct {
	eventStore         *store.EventStore
	correlationService *services.CorrelationService
	schemaRegistrySvc  *services.SchemaRegistryService
	mapperRegistry     *ingest.MapperRegistry
	deadLetterStore    *store.DeadLetterStore
	idempotencyStore   *store.IdempotencyStore
}

func NewOTelHandler(
	es *store.EventStore,
	cs *services.CorrelationService,
	srs *services.SchemaRegistryService,
	mr *ingest.MapperRegistry,
	dlq *store.DeadLetterStore,
	idm *store.IdempotencyStore,
) *OTelHandler {
	return &OTelHandler{
		eventStore:         es,
		correlationService: cs,
		schemaRegistrySvc:  srs,
		mapperRegistry:     mr,
		deadLetterStore:    dlq,
		idempotencyStore:   idm,
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

	// Idempotency check — fail safe on DB error.
	iKey := r.Header.Get("X-Idempotency-Key")
	if iKey != "" && h.idempotencyStore != nil {
		resp, seen, iErr := h.idempotencyStore.Check(iKey, "")
		if iErr != nil {
			slog.ErrorContext(r.Context(), "otel-ingest: idempotency check error", "error", iErr)
			api.WriteError(w, http.StatusServiceUnavailable, "service temporarily unavailable")
			return
		}
		if seen {
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("X-Idempotency-Replayed", "true")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(resp))
			return
		}
	}

	tenantID := h.resolveTenant(r)
	if tenantID == "" {
		api.WriteError(w, http.StatusUnauthorized, "missing or invalid credentials")
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, maxIngestBodyBytes))
	if err != nil {
		api.WriteError(w, http.StatusBadRequest, "failed to read request body")
		return
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

	mapper, ok := h.mapperRegistry.Get(sourceType)
	if !ok {
		api.WriteError(w, http.StatusBadRequest, fmt.Sprintf("unknown source type: %s", sourceType))
		return
	}

	events, err := mapper.Map(body, tenantID)
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

	resp := map[string]any{
		"status":        "accepted",
		"source_type":   sourceType,
		"events_queued": accepted,
	}
	respBytes, _ := json.Marshal(resp)

	if iKey != "" && h.idempotencyStore != nil {
		_ = h.idempotencyStore.Record(iKey, tenantID, sourceType, string(respBytes))
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	_, _ = w.Write(respBytes)
}

func (h *OTelHandler) resolveTenant(r *http.Request) string {
	if t, ok := middleware.TenantFromRequestStrict(r); ok {
		return t
	}
	// X-Source-Token fallback: token format "t_{tenantID}_{random}".
	token := r.Header.Get("X-Source-Token")
	if token == "" {
		return ""
	}
	parts := strings.SplitN(token, "_", 3)
	if len(parts) >= 2 && parts[0] == "t" {
		return parts[1]
	}
	return "default"
}
