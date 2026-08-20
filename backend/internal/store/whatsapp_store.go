package store

import (
	"context"
	"time"
	"database/sql"
	"encoding/json"

	"ai-incident-platform/backend/internal/models"
)

type WhatsAppStore struct {
	db *sql.DB
}

func NewWhatsAppStore(db *sql.DB) *WhatsAppStore {
	return &WhatsAppStore{db: db}
}

func (s *WhatsAppStore) Upsert(c models.WhatsAppConfig) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	recipientsJSON, err := json.Marshal(c.Recipients)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, 
		`INSERT INTO whatsapp_configs (id, tenant_id, phone_number_id, access_token, verify_token, notify_on, recipients_json, enabled)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
		 ON CONFLICT (id) DO UPDATE SET
		   phone_number_id = EXCLUDED.phone_number_id,
		   access_token = EXCLUDED.access_token,
		   verify_token = EXCLUDED.verify_token,
		   notify_on = EXCLUDED.notify_on,
		   recipients_json = EXCLUDED.recipients_json,
		   enabled = EXCLUDED.enabled`,
		c.ID, c.TenantID, c.PhoneNumberID, c.AccessToken, c.VerifyToken, c.NotifyOn, string(recipientsJSON), c.Enabled,
	)
	return err
}

func (s *WhatsAppStore) GetByTenant(tenantID string) (*models.WhatsAppConfig, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var c models.WhatsAppConfig
	var recipientsJSON string
	err := s.db.QueryRowContext(ctx, 
		`SELECT id, tenant_id, phone_number_id, access_token, verify_token, notify_on, recipients_json, enabled, created_at
		 FROM whatsapp_configs WHERE tenant_id = $1`, tenantID,
	).Scan(&c.ID, &c.TenantID, &c.PhoneNumberID, &c.AccessToken, &c.VerifyToken, &c.NotifyOn, &recipientsJSON, &c.Enabled, &c.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal([]byte(recipientsJSON), &c.Recipients); err != nil {
		c.Recipients = []string{}
	}
	return &c, nil
}
