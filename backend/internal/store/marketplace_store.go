package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"ai-incident-platform/backend/internal/models"
)

// MarketplaceStore persists per-tenant integration configurations.
// The integration catalog itself lives in services/marketplace_catalog.go.
type MarketplaceStore struct {
	db *sql.DB
}

func NewMarketplaceStore(db *sql.DB) *MarketplaceStore {
	return &MarketplaceStore{db: db}
}

// Upsert creates or updates a tenant's integration configuration.
func (s *MarketplaceStore) Upsert(cfg models.MarketplaceConfig) error {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	authJSON, _ := json.Marshal(cfg.AuthConfig)
	routingJSON, _ := json.Marshal(cfg.RoutingConfig)
	now := time.Now()

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO marketplace_configs
		    (tenant_id, integration_id, enabled, auth_config, routing_config,
		     source_id, last_tested_at, test_status, test_message, enabled_at,
		     created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
		ON CONFLICT (tenant_id, integration_id) DO UPDATE SET
		    enabled        = EXCLUDED.enabled,
		    auth_config    = EXCLUDED.auth_config,
		    routing_config = EXCLUDED.routing_config,
		    source_id      = EXCLUDED.source_id,
		    last_tested_at = EXCLUDED.last_tested_at,
		    test_status    = EXCLUDED.test_status,
		    test_message   = EXCLUDED.test_message,
		    enabled_at     = EXCLUDED.enabled_at,
		    updated_at     = EXCLUDED.updated_at`,
		cfg.TenantID, cfg.IntegrationID, cfg.Enabled,
		string(authJSON), string(routingJSON),
		cfg.SourceID, cfg.LastTestedAt, cfg.TestStatus, cfg.TestMessage,
		cfg.EnabledAt, now, now,
	)
	return err
}

// Get returns the config for a specific tenant+integration pair.
// Returns nil (no error) if no config exists yet.
func (s *MarketplaceStore) Get(tenantID, integrationID string) (*models.MarketplaceConfig, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var cfg models.MarketplaceConfig
	var authJSON, routingJSON string
	var lastTestedAt, enabledAt sql.NullTime

	err := s.db.QueryRowContext(ctx, `
		SELECT tenant_id, integration_id, enabled,
		       auth_config, routing_config, source_id,
		       last_tested_at, test_status, test_message,
		       enabled_at, created_at, updated_at
		FROM marketplace_configs
		WHERE tenant_id=$1 AND integration_id=$2`,
		tenantID, integrationID,
	).Scan(
		&cfg.TenantID, &cfg.IntegrationID, &cfg.Enabled,
		&authJSON, &routingJSON, &cfg.SourceID,
		&lastTestedAt, &cfg.TestStatus, &cfg.TestMessage,
		&enabledAt, &cfg.CreatedAt, &cfg.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	_ = json.Unmarshal([]byte(authJSON), &cfg.AuthConfig)
	_ = json.Unmarshal([]byte(routingJSON), &cfg.RoutingConfig)
	if lastTestedAt.Valid {
		t := lastTestedAt.Time
		cfg.LastTestedAt = &t
	}
	if enabledAt.Valid {
		t := enabledAt.Time
		cfg.EnabledAt = &t
	}
	return &cfg, nil
}

// ListEnabled returns all enabled integration configs for a tenant.
func (s *MarketplaceStore) ListEnabled(tenantID string) ([]models.MarketplaceConfig, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	rows, err := s.db.QueryContext(ctx, `
		SELECT tenant_id, integration_id, enabled,
		       auth_config, routing_config, source_id,
		       last_tested_at, test_status, test_message,
		       enabled_at, created_at, updated_at
		FROM marketplace_configs
		WHERE tenant_id=$1
		ORDER BY integration_id`,
		tenantID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var configs []models.MarketplaceConfig
	for rows.Next() {
		var cfg models.MarketplaceConfig
		var authJSON, routingJSON string
		var lastTestedAt, enabledAt sql.NullTime

		if err := rows.Scan(
			&cfg.TenantID, &cfg.IntegrationID, &cfg.Enabled,
			&authJSON, &routingJSON, &cfg.SourceID,
			&lastTestedAt, &cfg.TestStatus, &cfg.TestMessage,
			&enabledAt, &cfg.CreatedAt, &cfg.UpdatedAt,
		); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(authJSON), &cfg.AuthConfig)
		_ = json.Unmarshal([]byte(routingJSON), &cfg.RoutingConfig)
		if lastTestedAt.Valid {
			t := lastTestedAt.Time
			cfg.LastTestedAt = &t
		}
		if enabledAt.Valid {
			t := enabledAt.Time
			cfg.EnabledAt = &t
		}
		configs = append(configs, cfg)
	}
	return configs, rows.Err()
}

// UpdateTestResult updates only the test-related columns after a connection test.
func (s *MarketplaceStore) UpdateTestResult(tenantID, integrationID, status, message string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	now := time.Now()
	_, err := s.db.ExecContext(ctx, `
		UPDATE marketplace_configs
		SET test_status=$3, test_message=$4, last_tested_at=$5, updated_at=$5
		WHERE tenant_id=$1 AND integration_id=$2`,
		tenantID, integrationID, status, message, now,
	)
	return err
}
