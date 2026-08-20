package store

import (
	"context"
	"database/sql"
	"time"
)

// IdempotencyStore checks and records webhook idempotency keys.
// Keys expire after 24 hours; duplicate requests within that window return the
// cached response without reprocessing the payload.
type IdempotencyStore struct {
	db *sql.DB
}

func NewIdempotencyStore(db *sql.DB) *IdempotencyStore {
	return &IdempotencyStore{db: db}
}

// Check returns the cached response and true if the key was already processed.
// Returns ("", false, nil) when the key is unseen or has expired.
func (s *IdempotencyStore) Check(key, tenantID string) (response string, seen bool, err error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if key == "" {
		return "", false, nil
	}
	err = s.db.QueryRowContext(ctx, `
		SELECT response FROM webhook_idempotency_keys
		WHERE key = $1 AND tenant_id = $2 AND expires_at > NOW()`,
		key, tenantID).Scan(&response)
	if err == sql.ErrNoRows {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return response, true, nil
}

// Record saves a new idempotency key with a 24-hour TTL.
// Uses ON CONFLICT DO NOTHING so concurrent duplicate first-requests are safe.
func (s *IdempotencyStore) Record(key, tenantID, sourceID, response string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if key == "" {
		return nil
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO webhook_idempotency_keys (key, tenant_id, source_id, response, expires_at)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (key, tenant_id) DO NOTHING`,
		key, tenantID, sourceID, response, time.Now().Add(24*time.Hour))
	return err
}

// Cleanup removes expired keys. Call on a regular schedule (e.g. hourly).
func (s *IdempotencyStore) Cleanup() error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err := s.db.ExecContext(ctx, `DELETE FROM webhook_idempotency_keys WHERE expires_at < NOW()`)
	return err
}
