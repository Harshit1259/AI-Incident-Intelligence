package store

import (
	"context"
	"database/sql"
	"time"

	"ai-incident-platform/backend/internal/models"
)

// PushStore manages push notification subscriptions.
type PushStore struct {
	db *sql.DB
}

func NewPushStore(db *sql.DB) *PushStore {
	return &PushStore{db: db}
}

// Upsert registers or refreshes a push subscription.
func (s *PushStore) Upsert(sub models.PushSubscription) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO push_subscriptions
		    (user_id, tenant_id, platform, device_token, p256dh, auth_key, app_version)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (user_id, device_token)
		DO UPDATE SET
		    tenant_id   = EXCLUDED.tenant_id,
		    platform    = EXCLUDED.platform,
		    p256dh      = EXCLUDED.p256dh,
		    auth_key    = EXCLUDED.auth_key,
		    app_version = EXCLUDED.app_version,
		    updated_at  = NOW()
	`, sub.UserID, sub.TenantID, sub.Platform, sub.DeviceToken,
		sub.P256DH, sub.AuthKey, sub.AppVersion)
	return err
}

// Delete removes a specific subscription by ID (must belong to the given user).
func (s *PushStore) Delete(userID, subID string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err := s.db.ExecContext(ctx,
		`DELETE FROM push_subscriptions WHERE id = $1 AND user_id = $2`,
		subID, userID)
	return err
}

// GetByTenant returns all subscriptions for a tenant (used when broadcasting).
func (s *PushStore) GetByTenant(tenantID string) ([]models.PushSubscription, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	rows, err := s.db.QueryContext(ctx, `
		SELECT id, user_id, tenant_id, platform, device_token,
		       COALESCE(p256dh,''), COALESCE(auth_key,''), COALESCE(app_version,'1.0.0'), created_at
		FROM   push_subscriptions
		WHERE  tenant_id = $1
	`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []models.PushSubscription
	for rows.Next() {
		var s models.PushSubscription
		if err := rows.Scan(&s.ID, &s.UserID, &s.TenantID, &s.Platform,
			&s.DeviceToken, &s.P256DH, &s.AuthKey, &s.AppVersion, &s.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// GetByUser returns all subscriptions for a specific user.
func (s *PushStore) GetByUser(userID string) ([]models.PushSubscription, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	rows, err := s.db.QueryContext(ctx, `
		SELECT id, user_id, tenant_id, platform, device_token,
		       COALESCE(p256dh,''), COALESCE(auth_key,''), COALESCE(app_version,'1.0.0'), created_at
		FROM   push_subscriptions
		WHERE  user_id = $1
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []models.PushSubscription
	for rows.Next() {
		var s models.PushSubscription
		if err := rows.Scan(&s.ID, &s.UserID, &s.TenantID, &s.Platform,
			&s.DeviceToken, &s.P256DH, &s.AuthKey, &s.AppVersion, &s.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}
