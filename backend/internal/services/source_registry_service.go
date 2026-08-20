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

// CreateSource registers a new ingest source for the given tenant.
// The returned SourceConnection includes the ingest token — this is the only time it is returned.
func (srs *SourceRegistryService) CreateSource(tenantID, name, sourceType string) models.SourceConnection {
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
	}

	return source
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

func endpointForSourceType(sourceType string) string {
	switch sourceType {
	case "prometheus":
		return "/api/v1/ingest/prometheus"
	default:
		return "/api/v1/ingest/webhook"
	}
}

func randomHex(byteLength int) string {
	buffer := make([]byte, byteLength)
	_, _ = rand.Read(buffer)
	return hex.EncodeToString(buffer)
}
