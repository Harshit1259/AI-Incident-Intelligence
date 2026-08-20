package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"ai-incident-platform/backend/internal/models"
)

// TenantSettingsStore manages per-tenant multiplier and preference overrides.
type TenantSettingsStore struct {
	db *sql.DB
}

func NewTenantSettingsStore(db *sql.DB) *TenantSettingsStore {
	return &TenantSettingsStore{db: db}
}

func (s *TenantSettingsStore) Upsert(settings models.TenantSettings) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if settings.ID == "" {
		settings.ID = fmt.Sprintf("ts-%s", settings.TenantID)
	}

	sevJSON, _ := json.Marshal(settings.SeverityMultipliers)
	tierJSON, _ := json.Marshal(settings.TierMultipliers)
	prefsJSON, _ := json.Marshal(settings.MonthlyReportPrefs)

	_, err := s.db.ExecContext(ctx, 
		`INSERT INTO tenant_settings
		 (id, tenant_id, severity_multipliers_json, tier_multipliers_json,
		  confidence_mode, monthly_report_prefs_json, updated_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7)
		 ON CONFLICT (tenant_id) DO UPDATE SET
		   severity_multipliers_json  = EXCLUDED.severity_multipliers_json,
		   tier_multipliers_json      = EXCLUDED.tier_multipliers_json,
		   confidence_mode            = EXCLUDED.confidence_mode,
		   monthly_report_prefs_json  = EXCLUDED.monthly_report_prefs_json,
		   updated_at                 = EXCLUDED.updated_at`,
		settings.ID, settings.TenantID,
		string(sevJSON), string(tierJSON),
		settings.ConfidenceMode, string(prefsJSON),
		time.Now(),
	)
	return err
}

func (s *TenantSettingsStore) GetByTenant(tenantID string) (*models.TenantSettings, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var settings models.TenantSettings
	var sevJSON, tierJSON, prefsJSON string

	err := s.db.QueryRowContext(ctx, 
		`SELECT id, tenant_id, severity_multipliers_json, tier_multipliers_json,
		        confidence_mode, monthly_report_prefs_json, updated_at
		 FROM tenant_settings WHERE tenant_id = $1`, tenantID,
	).Scan(
		&settings.ID, &settings.TenantID,
		&sevJSON, &tierJSON,
		&settings.ConfidenceMode, &prefsJSON,
		&settings.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	settings.SeverityMultipliers = make(map[string]float64)
	settings.TierMultipliers = make(map[string]float64)
	settings.MonthlyReportPrefs = make(map[string]interface{})

	_ = json.Unmarshal([]byte(sevJSON), &settings.SeverityMultipliers)
	_ = json.Unmarshal([]byte(tierJSON), &settings.TierMultipliers)
	_ = json.Unmarshal([]byte(prefsJSON), &settings.MonthlyReportPrefs)

	return &settings, nil
}
