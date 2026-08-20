package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

// TenantConfigStore provides persistence for tenant configuration.
type TenantConfigStore struct {
	db *sql.DB
}

// NewTenantConfigStore creates a new TenantConfigStore.
func NewTenantConfigStore(db *sql.DB) *TenantConfigStore {
	return &TenantConfigStore{db: db}
}

// Get returns the parsed settings for a tenant. Returns default settings if none stored.
func (s *TenantConfigStore) Get(tenantID string) (map[string]interface{}, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var settingsJSON string
	err := s.db.QueryRowContext(ctx, `
		SELECT settings_json FROM tenant_configs WHERE tenant_id = $1`, tenantID).Scan(&settingsJSON)
	if err != nil {
		if err == sql.ErrNoRows {
			return defaultTenantSettings(), nil
		}
		return nil, fmt.Errorf("tenant_config_store: get: %w", err)
	}

	var settings map[string]interface{}
	if err := json.Unmarshal([]byte(settingsJSON), &settings); err != nil {
		return defaultTenantSettings(), nil
	}
	return settings, nil
}

// Set upserts the settings for a tenant.
func (s *TenantConfigStore) Set(tenantID string, settings map[string]interface{}) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	settingsJSON, err := json.Marshal(settings)
	if err != nil {
		return fmt.Errorf("tenant_config_store: marshal: %w", err)
	}

	_, err = s.db.ExecContext(ctx, `
		INSERT INTO tenant_configs (id, tenant_id, settings_json, updated_at)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (tenant_id) DO UPDATE SET
			settings_json = EXCLUDED.settings_json,
			updated_at = EXCLUDED.updated_at`,
		fmt.Sprintf("cfg-%s", tenantID), tenantID, string(settingsJSON), time.Now(),
	)
	return err
}

func defaultTenantSettings() map[string]interface{} {
	return map[string]interface{}{
		"correlation_window_minutes":   5,
		"dedup_window_minutes":         5,
		"auto_resolve_enabled":         true,
		"default_severity_multipliers": map[string]interface{}{"critical": 4, "high": 3, "medium": 2, "low": 1},
		"default_tier_multipliers":     map[string]interface{}{"TIER_1": 4, "TIER_2": 2, "TIER_3": 1},
		"notification_channels":        []string{"slack"},
	}
}
