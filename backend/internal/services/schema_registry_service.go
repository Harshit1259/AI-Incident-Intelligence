package services

import (
	"fmt"
	"time"

	"ai-incident-platform/backend/internal/ingest"
	"ai-incident-platform/backend/internal/models"
	"ai-incident-platform/backend/internal/store"
)

// SchemaRegistryService manages custom source schema mappings and keeps the
// mapper registry in sync so ingest requests can use up-to-date mappers.
type SchemaRegistryService struct {
	store    *store.SchemaMappingStore
	registry *ingest.MapperRegistry
}

func NewSchemaRegistryService(s *store.SchemaMappingStore, r *ingest.MapperRegistry) *SchemaRegistryService {
	return &SchemaRegistryService{store: s, registry: r}
}

// Create validates and persists a new schema mapping, then registers it.
func (svc *SchemaRegistryService) Create(tenantID string, m *models.SchemaMapping) error {
	if err := validateMapping(m); err != nil {
		return err
	}
	m.TenantID = tenantID
	if m.ID == "" {
		m.ID = fmt.Sprintf("sm-%d", time.Now().UnixNano())
	}
	if err := svc.store.Create(m); err != nil {
		return err
	}
	if m.Enabled {
		svc.registry.Register(ingest.NewCustomMapper(*m))
	}
	return nil
}

// Update persists changes and refreshes the mapper registration.
func (svc *SchemaRegistryService) Update(tenantID string, m *models.SchemaMapping) error {
	if err := validateMapping(m); err != nil {
		return err
	}
	m.TenantID = tenantID
	if err := svc.store.Update(m); err != nil {
		return err
	}
	if m.Enabled {
		svc.registry.Register(ingest.NewCustomMapper(*m))
	} else {
		svc.registry.Deregister("custom:" + m.SourceType)
	}
	return nil
}

// Delete removes the mapping and deregisters the mapper.
func (svc *SchemaRegistryService) Delete(id, tenantID string) error {
	m, err := svc.store.GetByID(id)
	if err != nil {
		return err
	}
	if err := svc.store.Delete(id, tenantID); err != nil {
		return err
	}
	svc.registry.Deregister("custom:" + m.SourceType)
	return nil
}

// Get returns a mapping by source type.
func (svc *SchemaRegistryService) Get(tenantID, sourceType string) (*models.SchemaMapping, error) {
	return svc.store.Get(tenantID, sourceType)
}

// GetByID returns a mapping by primary key.
func (svc *SchemaRegistryService) GetByID(id string) (*models.SchemaMapping, error) {
	return svc.store.GetByID(id)
}

// List returns all mappings for a tenant.
func (svc *SchemaRegistryService) List(tenantID string) ([]models.SchemaMapping, error) {
	return svc.store.ListByTenant(tenantID)
}

// Stats returns usage statistics for all mappings.
func (svc *SchemaRegistryService) Stats(tenantID string) ([]models.SchemaMappingStats, error) {
	return svc.store.Stats(tenantID)
}

// WarmRegistry loads all enabled mappings from the DB and registers their mappers.
// Called once at startup so custom sources are immediately ready to handle requests.
func (svc *SchemaRegistryService) WarmRegistry(tenantID string) error {
	mappings, err := svc.store.ListEnabled(tenantID)
	if err != nil {
		return err
	}
	for _, m := range mappings {
		svc.registry.Register(ingest.NewCustomMapper(m))
	}
	return nil
}

// validateMapping checks required fields.
func validateMapping(m *models.SchemaMapping) error {
	if m.SourceType == "" {
		return fmt.Errorf("source_type is required")
	}
	if m.Name == "" {
		return fmt.Errorf("name is required")
	}
	if len(m.FieldMap) == 0 {
		return fmt.Errorf("field_map must have at least one entry")
	}
	validSignalTypes := map[string]bool{"alert": true, "log": true, "metric": true, "trace": true, "change": true}
	if m.DefaultSignalType != "" && !validSignalTypes[m.DefaultSignalType] {
		return fmt.Errorf("invalid default_signal_type: %q", m.DefaultSignalType)
	}
	validSeverities := map[string]bool{"critical": true, "high": true, "medium": true, "low": true}
	if m.DefaultSeverity != "" && !validSeverities[m.DefaultSeverity] {
		return fmt.Errorf("invalid default_severity: %q", m.DefaultSeverity)
	}
	for _, v := range m.SeverityMap {
		if !validSeverities[v] {
			return fmt.Errorf("severity_map value %q must be critical|high|medium|low", v)
		}
	}
	return nil
}
