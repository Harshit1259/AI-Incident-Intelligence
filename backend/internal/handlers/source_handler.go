package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"ai-incident-platform/backend/internal/api"
	"ai-incident-platform/backend/internal/middleware"
	"ai-incident-platform/backend/internal/models"
	"ai-incident-platform/backend/internal/services"
)

type SourceHandler struct {
	sourceRegistryService *services.SourceRegistryService
	ingestHandler         *IngestHandler
	sampleIngester        *MarketplaceSampleIngester
}

func NewSourceHandler(
	sourceRegistryService *services.SourceRegistryService,
	ingestHandler *IngestHandler,
) *SourceHandler {
	return &SourceHandler{
		sourceRegistryService: sourceRegistryService,
		ingestHandler:         ingestHandler,
	}
}

func (sourceHandler *SourceHandler) ListSources(w http.ResponseWriter, r *http.Request) {
	tenantID := middleware.TenantFromRequest(r)
	sources := sourceHandler.sourceRegistryService.ListSources(tenantID)
	api.WriteJSON(w, http.StatusOK, map[string]interface{}{"items": sources})
}

func (sourceHandler *SourceHandler) CreateSource(w http.ResponseWriter, r *http.Request) {
	var requestBody struct {
		Name string `json:"name"`
		Type string `json:"type"`
	}

	if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
		api.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	name := strings.TrimSpace(requestBody.Name)
	if name == "" {
		api.WriteError(w, http.StatusBadRequest, "name is required")
		return
	}
	if len(name) > 100 {
		api.WriteError(w, http.StatusBadRequest, "name must be 100 characters or fewer")
		return
	}
	sourceType := strings.ToLower(strings.TrimSpace(requestBody.Type))
	if !services.SupportedSourceTypes[sourceType] {
		api.WriteError(w, http.StatusBadRequest,
			"type must be one of: otel, prometheus, grafana, jaeger, zabbix")
		return
	}

	tenantID := middleware.TenantFromRequest(r)
	source, err := sourceHandler.sourceRegistryService.CreateSource(tenantID, name, sourceType)
	if err != nil {
		api.WriteError(w, http.StatusInternalServerError, "could not create the source")
		return
	}
	api.WriteJSON(w, http.StatusCreated, source)
}

// HandleSourceByID serves DELETE /api/v1/sources/{id} and
// POST /api/v1/sources/{id}/rotate (issues a new token; the old one stops working).
func (sourceHandler *SourceHandler) HandleSourceByID(w http.ResponseWriter, r *http.Request) {
	rest := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v1/sources/"), "/")
	id, action, _ := strings.Cut(rest, "/")
	if id == "" {
		api.WriteError(w, http.StatusNotFound, "not found")
		return
	}
	tenantID := middleware.TenantFromRequest(r)

	switch {
	case action == "rotate" && r.Method == http.MethodPost:
		src, err := sourceHandler.sourceRegistryService.RotateToken(tenantID, id)
		if errors.Is(err, services.ErrSourceNotFound) {
			api.WriteError(w, http.StatusNotFound, "source not found")
			return
		}
		if err != nil {
			api.WriteError(w, http.StatusInternalServerError, "could not rotate the token")
			return
		}
		api.WriteJSON(w, http.StatusOK, src)
	case action == "" && r.Method == http.MethodDelete:
		err := sourceHandler.sourceRegistryService.DeleteSource(tenantID, id)
		if errors.Is(err, services.ErrSourceNotFound) {
			api.WriteError(w, http.StatusNotFound, "source not found")
			return
		}
		if err != nil {
			api.WriteError(w, http.StatusInternalServerError, "could not delete the source")
			return
		}
		api.WriteJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
	default:
		api.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (sourceHandler *SourceHandler) ListSourceHealth(w http.ResponseWriter, r *http.Request) {
	tenantID := middleware.TenantFromRequest(r)
	sources := sourceHandler.sourceRegistryService.ListSources(tenantID)

	// Augment each source with a staleness flag: no event in the last 30 minutes
	// on an otherwise healthy source signals a silent failure.
	threshold := time.Now().Add(-30 * time.Minute)
	type healthItem struct {
		models.SourceConnection
		Stale bool `json:"stale"`
	}
	items := make([]healthItem, len(sources))
	for i, src := range sources {
		stale := src.LastEventAt != nil && src.LastEventAt.Before(threshold) && src.Status == "healthy"
		items[i] = healthItem{SourceConnection: src, Stale: stale}
	}

	api.WriteJSON(w, http.StatusOK, map[string]interface{}{"items": items})
}

// SendTestEvent sends the integration's sample payload for this source
// through the real parser and pipeline, so a successful test means real data
// from that tool will be accepted.
func (sourceHandler *SourceHandler) SendTestEvent(w http.ResponseWriter, r *http.Request) {
	var requestBody struct {
		SourceID string `json:"source_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
		api.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	tenantID := middleware.TenantFromRequest(r)
	source, err := sourceHandler.sourceRegistryService.GetSource(tenantID, requestBody.SourceID)
	if err != nil {
		api.WriteError(w, http.StatusNotFound, "source not found")
		return
	}
	if sourceHandler.sampleIngester == nil {
		api.WriteError(w, http.StatusServiceUnavailable, "test events are not configured")
		return
	}
	payload, ok := services.IntegrationSamplePayload(source.Type)
	if !ok {
		api.WriteError(w, http.StatusBadRequest,
			fmt.Sprintf("test events are not available for %q sources", source.Type))
		return
	}
	processed, err := sourceHandler.sampleIngester.IngestSample(tenantID, source.ID, source.Type, []byte(payload))
	if err != nil {
		api.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	api.WriteJSON(w, http.StatusOK, map[string]any{"status": "ok", "processed": processed})
}

// SetSampleIngester wires the real parsers used by SendTestEvent.
func (sourceHandler *SourceHandler) SetSampleIngester(si *MarketplaceSampleIngester) {
	sourceHandler.sampleIngester = si
}
