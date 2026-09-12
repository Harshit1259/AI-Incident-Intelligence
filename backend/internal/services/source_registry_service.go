package services

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"time"

	"ai-incident-platform/backend/internal/models"
	"ai-incident-platform/backend/internal/store"
)

type SourceRegistryService struct {
	store *store.SourceRegistryStore
}

func NewSourceRegistryService(s *store.SourceRegistryStore) *SourceRegistryService {
	return &SourceRegistryService{store: s}
}

// SupportedSourceTypes are the integrations a customer can connect today
// (see plan.md for the ones removed for now).
var SupportedSourceTypes = map[string]bool{
	"otel": true, "prometheus": true, "grafana": true, "jaeger": true, "zabbix": true,
}

// ErrSourceNotFound is returned when a source does not exist for the tenant.
var ErrSourceNotFound = store.ErrSourceNotFound

// CreateSource registers a new ingest source for the given tenant.
// The returned SourceConnection includes the ingest token — this is the only
// time it is returned. A failed save is returned as an error: handing out a
// token for a source that was never stored produced tokens that never worked.
func (srs *SourceRegistryService) CreateSource(tenantID, name, sourceType string) (models.SourceConnection, error) {
	sourceID := fmt.Sprintf("source-%d", time.Now().UnixNano())
	token := randomHex(16)
	endpoint := endpointForSourceType(sourceType)

	source := models.SourceConnection{
		ID:          sourceID,
		TenantID:    tenantID,
		Name:        name,
		Type:        sourceType,
		Token:       token,
		Endpoint:    endpoint,
		Status:      "connected",
		CreatedAt:   time.Now().UTC(),
		LastEventAt: nil,
		LastError:   "",
		TotalEvents: 0,
	}

	if err := srs.store.Create(source, tenantID); err != nil {
		slog.Error("source_registry: failed to persist source", "source_id", sourceID, "tenant_id", tenantID, "error", err)
		return models.SourceConnection{}, fmt.Errorf("save source: %w", err)
	}

	return source, nil
}

// RotateToken issues a new ingest token for the tenant's source and returns
// the source with the new token. The old token stops working immediately.
func (srs *SourceRegistryService) RotateToken(tenantID, id string) (models.SourceConnection, error) {
	token := randomHex(16)
	if err := srs.store.RotateToken(tenantID, id, token); err != nil {
		return models.SourceConnection{}, err
	}
	src, err := srs.findForTenant(tenantID, id)
	if err != nil {
		return models.SourceConnection{}, err
	}
	src.Token = token
	return src, nil
}

// DeleteSource removes the tenant's source; its token stops working.
func (srs *SourceRegistryService) DeleteSource(tenantID, id string) error {
	return srs.store.Delete(tenantID, id)
}

// GetSource returns the tenant's source without its token.
func (srs *SourceRegistryService) GetSource(tenantID, id string) (models.SourceConnection, error) {
	src, err := srs.findForTenant(tenantID, id)
	if err != nil {
		return models.SourceConnection{}, err
	}
	src.Token = ""
	src.HealthScore = deriveHealthScore(src)
	return src, nil
}

func (srs *SourceRegistryService) findForTenant(tenantID, id string) (models.SourceConnection, error) {
	src, err := srs.store.FindByID(id)
	if err != nil {
		return models.SourceConnection{}, err
	}
	if src == nil || src.TenantID != tenantID {
		return models.SourceConnection{}, ErrSourceNotFound
	}
	return *src, nil
}

// ListSources returns all source connections for the given tenant.
// Ingest tokens are scrubbed from the response — they were shown only at creation time.
// A HealthScore is derived from status and error_count so the UI can show red/yellow/green.
func (srs *SourceRegistryService) ListSources(tenantID string) []models.SourceConnection {
	sources, err := srs.store.List(tenantID)
	if err != nil {
		slog.Error("source_registry: failed to list sources", "tenant_id", tenantID, "error", err)
		return []models.SourceConnection{}
	}
	if sources == nil {
		return []models.SourceConnection{}
	}
	for i := range sources {
		sources[i].Token = ""
		sources[i].HealthScore = deriveHealthScore(sources[i])
	}
	return sources
}

// deriveHealthScore returns a simple three-level health label.
func deriveHealthScore(src models.SourceConnection) string {
	switch {
	case src.Status == "error" || src.ErrorCount >= 5:
		return "error"
	case src.ErrorCount > 0:
		return "degraded"
	case src.LastEventAt == nil:
		return "waiting" // connected, but nothing received yet
	default:
		return "healthy"
	}
}

// FindByToken looks up a source by its ingest token and returns the source + tenantID.
// Used by public webhook ingest endpoints to bind an unauthenticated request to a tenant.
func (srs *SourceRegistryService) FindByToken(token string) (models.SourceConnection, string, bool) {
	src, tenantID, err := srs.store.FindByIngestToken(token)
	if err != nil {
		slog.Error("source_registry: token lookup error", "error", err)
		return models.SourceConnection{}, "", false
	}
	if src == nil {
		return models.SourceConnection{}, "", false
	}
	return *src, tenantID, true
}

func (srs *SourceRegistryService) RecordSuccess(sourceID string, eventCount int) {
	if err := srs.store.RecordSuccess(sourceID, eventCount); err != nil {
		slog.Error("source_registry: failed to record success", "source_id", sourceID, "error", err)
	}
}

func (srs *SourceRegistryService) RecordError(sourceID string, errorMessage string) {
	if err := srs.store.RecordError(sourceID, errorMessage); err != nil {
		slog.Error("source_registry: failed to record error", "source_id", sourceID, "error", err)
	}
}

// endpointForSourceType returns where a source of this type sends data.
// Only OpenTelemetry, Prometheus, Grafana, Jaeger and Zabbix are supported;
// unknown types have no endpoint (see plan.md).
func endpointForSourceType(sourceType string) string {
	switch sourceType {
	case "prometheus":
		return "/api/v1/ingest/prometheus"
	case "grafana":
		return "/api/v1/ingest/grafana"
	case "otel":
		return "/api/v1/otel"
	case "jaeger":
		return "/api/v1/otel/traces"
	case "zabbix":
		return "/api/v1/ingest/zabbix"
	default:
		return ""
	}
}

func randomHex(byteLength int) string {
	buffer := make([]byte, byteLength)
	_, _ = rand.Read(buffer)
	return hex.EncodeToString(buffer)
}
