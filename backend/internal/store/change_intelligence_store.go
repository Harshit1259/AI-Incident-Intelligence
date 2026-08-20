package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"ai-incident-platform/backend/internal/models"
)

// ChangeIntelligenceStore persists and queries enriched change events,
// feature flag changes, config baselines, and drift events.
type ChangeIntelligenceStore struct {
	db *sql.DB
}

// NewChangeIntelligenceStore creates a new ChangeIntelligenceStore.
func NewChangeIntelligenceStore(db *sql.DB) *ChangeIntelligenceStore {
	return &ChangeIntelligenceStore{db: db}
}

// ─────────────────────────────────────────────────────────────────────────────
// Change events (enriched from the changes table)
// ─────────────────────────────────────────────────────────────────────────────

// AddEnrichedChange inserts a fully-enriched change event into the changes table.
func (s *ChangeIntelligenceStore) AddEnrichedChange(c models.ChangeEvent) (int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	meta, _ := json.Marshal(c.Metadata)
	if c.Metadata == nil {
		meta = []byte("{}")
	}
	ts := c.Timestamp
	if ts.IsZero() {
		ts = time.Now()
	}

	var id int
	err := s.db.QueryRowContext(ctx, `
		INSERT INTO changes
			(service, type, version, description, timestamp,
			 tenant_id, environment, commit_sha, author, pr_number,
			 changed_files_count, change_source, metadata_json,
			 correlation_score, linked_incident_id)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)
		RETURNING id`,
		c.Service, c.Type, c.Version, c.Description, ts,
		c.TenantID, c.Environment, c.CommitSHA, c.Author, c.PRNumber,
		c.ChangedFilesCount, c.ChangeSource, string(meta),
		c.CorrelationScore, c.LinkedIncidentID,
	).Scan(&id)
	return id, err
}

// UpdateCorrelation sets correlation_score and linked_incident_id on a change row.
func (s *ChangeIntelligenceStore) UpdateCorrelation(changeID int, incidentID string, score int) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err := s.db.ExecContext(ctx, `
		UPDATE changes SET correlation_score = $1, linked_incident_id = $2
		WHERE id = $3`, score, incidentID, changeID)
	return err
}

// GetRecentChanges returns all change events for a tenant within [since, until].
func (s *ChangeIntelligenceStore) GetRecentChanges(tenantID string, since, until time.Time, limit int) ([]models.ChangeEvent, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if limit <= 0 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, COALESCE(tenant_id,'default'), service, type, version, description,
		       timestamp, COALESCE(environment,'production'), COALESCE(commit_sha,''),
		       COALESCE(author,''), COALESCE(pr_number,''),
		       COALESCE(changed_files_count,0), COALESCE(change_source,'webhook'),
		       COALESCE(metadata_json,'{}'), COALESCE(correlation_score,0),
		       COALESCE(linked_incident_id,'')
		FROM changes
		WHERE (tenant_id = $1 OR tenant_id = 'default')
		  AND timestamp >= $2 AND timestamp <= $3
		ORDER BY timestamp DESC
		LIMIT $4`,
		tenantID, since, until, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanChangeEvents(rows)
}

// GetChangesForService returns change events for a specific service within [since, until].
func (s *ChangeIntelligenceStore) GetChangesForService(tenantID, service string, since, until time.Time) ([]models.ChangeEvent, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	rows, err := s.db.QueryContext(ctx, `
		SELECT id, COALESCE(tenant_id,'default'), service, type, version, description,
		       timestamp, COALESCE(environment,'production'), COALESCE(commit_sha,''),
		       COALESCE(author,''), COALESCE(pr_number,''),
		       COALESCE(changed_files_count,0), COALESCE(change_source,'webhook'),
		       COALESCE(metadata_json,'{}'), COALESCE(correlation_score,0),
		       COALESCE(linked_incident_id,'')
		FROM changes
		WHERE (tenant_id = $1 OR tenant_id = 'default')
		  AND service = $2
		  AND timestamp >= $3 AND timestamp <= $4
		ORDER BY timestamp ASC`,
		tenantID, service, since, until)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanChangeEvents(rows)
}

// GetChangesInWindow returns all changes in a time window, for any service.
// Used by the causality timeline to find what changed first across all services.
func (s *ChangeIntelligenceStore) GetChangesInWindow(tenantID string, since, until time.Time) ([]models.ChangeEvent, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	rows, err := s.db.QueryContext(ctx, `
		SELECT id, COALESCE(tenant_id,'default'), service, type, version, description,
		       timestamp, COALESCE(environment,'production'), COALESCE(commit_sha,''),
		       COALESCE(author,''), COALESCE(pr_number,''),
		       COALESCE(changed_files_count,0), COALESCE(change_source,'webhook'),
		       COALESCE(metadata_json,'{}'), COALESCE(correlation_score,0),
		       COALESCE(linked_incident_id,'')
		FROM changes
		WHERE (tenant_id = $1 OR tenant_id = 'default')
		  AND timestamp >= $2 AND timestamp <= $3
		ORDER BY timestamp ASC
		LIMIT 200`,
		tenantID, since, until)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanChangeEvents(rows)
}

func scanChangeEvents(rows *sql.Rows) ([]models.ChangeEvent, error) {
	var out []models.ChangeEvent
	for rows.Next() {
		var c models.ChangeEvent
		var metaJSON string
		if err := rows.Scan(
			&c.ID, &c.TenantID, &c.Service, &c.Type, &c.Version, &c.Description,
			&c.Timestamp, &c.Environment, &c.CommitSHA,
			&c.Author, &c.PRNumber,
			&c.ChangedFilesCount, &c.ChangeSource,
			&metaJSON, &c.CorrelationScore,
			&c.LinkedIncidentID,
		); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(metaJSON), &c.Metadata)
		if c.Metadata == nil {
			c.Metadata = map[string]string{}
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// ─────────────────────────────────────────────────────────────────────────────
// Feature flag changes
// ─────────────────────────────────────────────────────────────────────────────

// AddFeatureFlagChange persists a feature flag toggle event.
func (s *ChangeIntelligenceStore) AddFeatureFlagChange(f models.FeatureFlagChange) (int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	ts := f.Timestamp
	if ts.IsZero() {
		ts = time.Now()
	}
	var id int
	err := s.db.QueryRowContext(ctx, `
		INSERT INTO feature_flag_changes
			(tenant_id, flag_name, flag_key, environment,
			 old_value, new_value, changed_by, affected_pct, timestamp,
			 linked_incident_id, correlation_score)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
		RETURNING id`,
		f.TenantID, f.FlagName, f.FlagKey, f.Environment,
		f.OldValue, f.NewValue, f.ChangedBy, f.AffectedPct, ts,
		f.LinkedIncidentID, f.CorrelationScore,
	).Scan(&id)
	return id, err
}

// GetRecentFlagChanges returns all feature flag changes in [since, until].
func (s *ChangeIntelligenceStore) GetRecentFlagChanges(tenantID string, since, until time.Time) ([]models.FeatureFlagChange, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	rows, err := s.db.QueryContext(ctx, `
		SELECT id, tenant_id, flag_name, flag_key, environment,
		       old_value, new_value, changed_by, affected_pct, timestamp,
		       linked_incident_id, correlation_score
		FROM feature_flag_changes
		WHERE (tenant_id = $1 OR tenant_id = 'default')
		  AND timestamp >= $2 AND timestamp <= $3
		ORDER BY timestamp ASC`,
		tenantID, since, until)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []models.FeatureFlagChange
	for rows.Next() {
		var f models.FeatureFlagChange
		if err := rows.Scan(
			&f.ID, &f.TenantID, &f.FlagName, &f.FlagKey, &f.Environment,
			&f.OldValue, &f.NewValue, &f.ChangedBy, &f.AffectedPct, &f.Timestamp,
			&f.LinkedIncidentID, &f.CorrelationScore,
		); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

// UpdateFlagCorrelation links a flag change to an incident.
func (s *ChangeIntelligenceStore) UpdateFlagCorrelation(flagID int, incidentID string, score int) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err := s.db.ExecContext(ctx, `
		UPDATE feature_flag_changes
		SET linked_incident_id = $1, correlation_score = $2
		WHERE id = $3`, incidentID, score, flagID)
	return err
}

// ─────────────────────────────────────────────────────────────────────────────
// Config baselines and drift
// ─────────────────────────────────────────────────────────────────────────────

// UpsertConfigBaseline stores or updates the known-good value for a config key.
func (s *ChangeIntelligenceStore) UpsertConfigBaseline(tenantID, service, key, value string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO config_baselines (tenant_id, service, config_key, baseline_value, updated_at)
		VALUES ($1, $2, $3, $4, NOW())
		ON CONFLICT (tenant_id, service, config_key)
		DO UPDATE SET baseline_value = $4, updated_at = NOW()`,
		tenantID, service, key, value)
	return err
}

// GetConfigBaselines returns all baseline values for a service.
func (s *ChangeIntelligenceStore) GetConfigBaselines(tenantID, service string) (map[string]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	rows, err := s.db.QueryContext(ctx, `
		SELECT config_key, baseline_value
		FROM config_baselines
		WHERE tenant_id = $1 AND service = $2`,
		tenantID, service)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	baselines := map[string]string{}
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, err
		}
		baselines[k] = v
	}
	return baselines, rows.Err()
}

// RecordDrift persists a detected drift event and returns its ID.
func (s *ChangeIntelligenceStore) RecordDrift(d models.ConfigDriftEvent) (int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var id int
	err := s.db.QueryRowContext(ctx, `
		INSERT INTO config_drift_events
			(tenant_id, service, config_key, baseline_value, current_value,
			 drift_severity, detected_at, linked_incident_id)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
		RETURNING id`,
		d.TenantID, d.Service, d.ConfigKey, d.BaselineValue, d.CurrentValue,
		d.DriftSeverity, d.DetectedAt, d.LinkedIncidentID,
	).Scan(&id)
	return id, err
}

// GetRecentDrift returns all drift events in [since, until].
func (s *ChangeIntelligenceStore) GetRecentDrift(tenantID string, since, until time.Time) ([]models.ConfigDriftEvent, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	rows, err := s.db.QueryContext(ctx, `
		SELECT id, tenant_id, service, config_key, baseline_value, current_value,
		       drift_severity, detected_at, linked_incident_id
		FROM config_drift_events
		WHERE (tenant_id = $1 OR tenant_id = 'default')
		  AND detected_at >= $2 AND detected_at <= $3
		ORDER BY detected_at ASC`,
		tenantID, since, until)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []models.ConfigDriftEvent
	for rows.Next() {
		var d models.ConfigDriftEvent
		if err := rows.Scan(
			&d.ID, &d.TenantID, &d.Service, &d.ConfigKey,
			&d.BaselineValue, &d.CurrentValue,
			&d.DriftSeverity, &d.DetectedAt, &d.LinkedIncidentID,
		); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// DetectAndRecordDrift compares currentConfig against stored baselines for a service
// and records any deviating keys as drift events. Returns the events recorded.
func (s *ChangeIntelligenceStore) DetectAndRecordDrift(tenantID, service string, currentConfig map[string]string) ([]models.ConfigDriftEvent, error) {
	baselines, err := s.GetConfigBaselines(tenantID, service)
	if err != nil {
		return nil, fmt.Errorf("change_intel: read baselines: %w", err)
	}

	now := time.Now()
	var drifted []models.ConfigDriftEvent

	for key, current := range currentConfig {
		baseline, exists := baselines[key]
		if !exists {
			// New key — store it as the baseline; not a drift.
			_ = s.UpsertConfigBaseline(tenantID, service, key, current)
			continue
		}
		if baseline == current {
			continue
		}

		severity := classifyDriftSeverity(key, baseline, current)
		d := models.ConfigDriftEvent{
			TenantID:      tenantID,
			Service:       service,
			ConfigKey:     key,
			BaselineValue: baseline,
			CurrentValue:  current,
			DriftSeverity: severity,
			DetectedAt:    now,
		}
		if id, err := s.RecordDrift(d); err == nil {
			d.ID = id
		}
		drifted = append(drifted, d)
	}
	return drifted, nil
}

// classifyDriftSeverity assigns a severity level based on key name heuristics.
func classifyDriftSeverity(key, _, _ string) string {
	lower := strings.ToLower(key)
	switch {
	case containsAnyKW(lower, []string{"secret", "password", "token", "key", "cert", "tls"}):
		return "critical"
	case containsAnyKW(lower, []string{"replica", "shard", "memory", "cpu", "timeout", "limit", "rate"}):
		return "high"
	case containsAnyKW(lower, []string{"url", "host", "port", "endpoint", "feature", "flag", "enable", "disable"}):
		return "medium"
	default:
		return "low"
	}
}

func containsAnyKW(s string, kws []string) bool {
	for _, kw := range kws {
		if strings.Contains(s, kw) {
			return true
		}
	}
	return false
}
