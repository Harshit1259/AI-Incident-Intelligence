package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"ai-incident-platform/backend/internal/models"
)

type SchemaMappingStore struct {
	db *sql.DB
}

func NewSchemaMappingStore(db *sql.DB) *SchemaMappingStore {
	return &SchemaMappingStore{db: db}
}

// Create inserts a new schema mapping.
func (s *SchemaMappingStore) Create(m *models.SchemaMapping) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	fieldMapJSON, severityMapJSON, err := marshalMaps(m)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO schema_mappings
		  (id, tenant_id, source_type, name, enabled, field_map, severity_map,
		   default_signal_type, default_severity, title_template, sample_payload,
		   created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)`,
		m.ID, m.TenantID, m.SourceType, m.Name, m.Enabled,
		fieldMapJSON, severityMapJSON,
		m.DefaultSignalType, m.DefaultSeverity, m.TitleTemplate, m.SamplePayload,
		now, now,
	)
	return err
}

// Get returns a mapping by tenant + source_type.
func (s *SchemaMappingStore) Get(tenantID, sourceType string) (*models.SchemaMapping, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	row := s.db.QueryRowContext(ctx, `
		SELECT id, tenant_id, source_type, name, enabled, field_map, severity_map,
		       default_signal_type, default_severity, title_template,
		       COALESCE(sample_payload,''), created_at, updated_at
		FROM schema_mappings
		WHERE tenant_id = $1 AND source_type = $2`, tenantID, sourceType)
	return scanMapping(row)
}

// GetByID returns a mapping by primary key.
func (s *SchemaMappingStore) GetByID(id string) (*models.SchemaMapping, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	row := s.db.QueryRowContext(ctx, `
		SELECT id, tenant_id, source_type, name, enabled, field_map, severity_map,
		       default_signal_type, default_severity, title_template,
		       COALESCE(sample_payload,''), created_at, updated_at
		FROM schema_mappings
		WHERE id = $1`, id)
	return scanMapping(row)
}

// ListByTenant returns all mappings for a tenant.
func (s *SchemaMappingStore) ListByTenant(tenantID string) ([]models.SchemaMapping, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	rows, err := s.db.QueryContext(ctx, `
		SELECT id, tenant_id, source_type, name, enabled, field_map, severity_map,
		       default_signal_type, default_severity, title_template,
		       COALESCE(sample_payload,''), created_at, updated_at
		FROM schema_mappings
		WHERE tenant_id = $1
		ORDER BY name ASC`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.SchemaMapping
	for rows.Next() {
		m, err := scanMapping(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *m)
	}
	return out, rows.Err()
}

// ListEnabled returns all enabled mappings for a tenant (used to warm the mapper registry).
func (s *SchemaMappingStore) ListEnabled(tenantID string) ([]models.SchemaMapping, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	rows, err := s.db.QueryContext(ctx, `
		SELECT id, tenant_id, source_type, name, enabled, field_map, severity_map,
		       default_signal_type, default_severity, title_template,
		       COALESCE(sample_payload,''), created_at, updated_at
		FROM schema_mappings
		WHERE tenant_id = $1 AND enabled = true
		ORDER BY name ASC`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.SchemaMapping
	for rows.Next() {
		m, err := scanMapping(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *m)
	}
	return out, rows.Err()
}

// Update replaces all mutable fields of a mapping.
func (s *SchemaMappingStore) Update(m *models.SchemaMapping) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	fieldMapJSON, severityMapJSON, err := marshalMaps(m)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `
		UPDATE schema_mappings SET
		  name = $1, enabled = $2, field_map = $3, severity_map = $4,
		  default_signal_type = $5, default_severity = $6, title_template = $7,
		  sample_payload = $8, updated_at = NOW()
		WHERE id = $9 AND tenant_id = $10`,
		m.Name, m.Enabled, fieldMapJSON, severityMapJSON,
		m.DefaultSignalType, m.DefaultSeverity, m.TitleTemplate,
		m.SamplePayload, m.ID, m.TenantID,
	)
	return err
}

// Delete removes a mapping by id + tenant.
func (s *SchemaMappingStore) Delete(id, tenantID string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err := s.db.ExecContext(ctx, `DELETE FROM schema_mappings WHERE id = $1 AND tenant_id = $2`, id, tenantID)
	return err
}

// Stats returns usage stats for all mappings for a tenant.
func (s *SchemaMappingStore) Stats(tenantID string) ([]models.SchemaMappingStats, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	rows, err := s.db.QueryContext(ctx, `
		SELECT sm.source_type, sm.name, sm.enabled,
		       COUNT(e.id) AS events_ingested,
		       MAX(e.timestamp) AS last_ingested_at
		FROM schema_mappings sm
		LEFT JOIN events e
		       ON e.tenant_id = sm.tenant_id
		      AND e.ingest_schema = 'custom:' || sm.source_type
		WHERE sm.tenant_id = $1
		GROUP BY sm.source_type, sm.name, sm.enabled
		ORDER BY sm.name ASC`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.SchemaMappingStats
	for rows.Next() {
		var st models.SchemaMappingStats
		var lastAt sql.NullTime
		if err := rows.Scan(&st.SourceType, &st.Name, &st.Enabled, &st.EventsIngested, &lastAt); err != nil {
			return nil, err
		}
		if lastAt.Valid {
			t := lastAt.Time
			st.LastIngestedAt = &t
		}
		out = append(out, st)
	}
	return out, rows.Err()
}

// ── helpers ──────────────────────────────────────────────────────────────────

type rowScannableMapping interface {
	Scan(dest ...any) error
}

func scanMapping(r rowScannableMapping) (*models.SchemaMapping, error) {
	var m models.SchemaMapping
	var fieldMapJSON, severityMapJSON string
	err := r.Scan(
		&m.ID, &m.TenantID, &m.SourceType, &m.Name, &m.Enabled,
		&fieldMapJSON, &severityMapJSON,
		&m.DefaultSignalType, &m.DefaultSeverity, &m.TitleTemplate,
		&m.SamplePayload, &m.CreatedAt, &m.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	if err := unmarshalStringMap(fieldMapJSON, &m.FieldMap); err != nil {
		return nil, fmt.Errorf("field_map: %w", err)
	}
	if err := unmarshalStringMap(severityMapJSON, &m.SeverityMap); err != nil {
		return nil, fmt.Errorf("severity_map: %w", err)
	}
	return &m, nil
}

func marshalMaps(m *models.SchemaMapping) (fieldMapJSON, severityMapJSON string, err error) {
	fm, err := marshalStringMap(m.FieldMap)
	if err != nil {
		return "", "", err
	}
	sm, err := marshalStringMap(m.SeverityMap)
	if err != nil {
		return "", "", err
	}
	return fm, sm, nil
}
