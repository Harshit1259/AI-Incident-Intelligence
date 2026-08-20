package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"ai-incident-platform/backend/internal/models"
)

// APIKeyStore manages API key metadata.
// Raw keys are never stored — only their SHA-256 hex hash.
type APIKeyStore struct {
	db *sql.DB
}

// NewAPIKeyStore returns a new APIKeyStore.
func NewAPIKeyStore(db *sql.DB) *APIKeyStore {
	return &APIKeyStore{db: db}
}

// Create inserts a new API key record using a pre-computed hash.
// Callers must compute key_hash = hex(sha256(rawKey)) before calling Create.
// The keyHash parameter is the hex-encoded SHA-256 of the raw key.
func (s *APIKeyStore) Create(k models.APIKey, keyHash string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	scopesJSON, err := marshalStringSlice(k.Scopes)
	if err != nil {
		return fmt.Errorf("api_key_store create: marshal scopes: %w", err)
	}
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO api_keys (
			id, tenant_id, name, key_hash, key_prefix,
			owner_type, owner_id, scopes_json,
			last_used_at, expires_at, revoked, revoked_at, revoked_by,
			created_by, created_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,NOW())`,
		k.ID, k.TenantID, k.Name, keyHash, k.KeyPrefix,
		string(k.OwnerType), k.OwnerID, scopesJSON,
		k.LastUsedAt, k.ExpiresAt, k.Revoked, k.RevokedAt, k.RevokedBy,
		k.CreatedBy,
	)
	return err
}

// GetByHash looks up a key by its SHA-256 hex hash.
// Returns sql.ErrNoRows if not found.
func (s *APIKeyStore) GetByHash(keyHash string) (*models.APIKey, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	row := s.db.QueryRowContext(ctx, `
		SELECT id, tenant_id, name, key_prefix, owner_type, owner_id,
		       scopes_json, last_used_at, expires_at, revoked, revoked_at, revoked_by,
		       created_by, created_at
		FROM api_keys
		WHERE key_hash = $1`,
		keyHash,
	)
	return scanAPIKey(row)
}

// GetByID returns an API key by its UUID scoped to tenantID.
func (s *APIKeyStore) GetByID(id, tenantID string) (*models.APIKey, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	row := s.db.QueryRowContext(ctx, `
		SELECT id, tenant_id, name, key_prefix, owner_type, owner_id,
		       scopes_json, last_used_at, expires_at, revoked, revoked_at, revoked_by,
		       created_by, created_at
		FROM api_keys
		WHERE id = $1 AND tenant_id = $2`,
		id, tenantID,
	)
	return scanAPIKey(row)
}

// ListByOwner returns all API keys for an owner (user or service account).
func (s *APIKeyStore) ListByOwner(tenantID, ownerID string) ([]models.APIKey, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	rows, err := s.db.QueryContext(ctx, `
		SELECT id, tenant_id, name, key_prefix, owner_type, owner_id,
		       scopes_json, last_used_at, expires_at, revoked, revoked_at, revoked_by,
		       created_by, created_at
		FROM api_keys
		WHERE tenant_id = $1 AND owner_id = $2
		ORDER BY created_at DESC LIMIT 200`,
		tenantID, ownerID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.APIKey
	for rows.Next() {
		k, err := scanAPIKey(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *k)
	}
	return out, rows.Err()
}

// ListByTenant returns all API keys for a tenant.
func (s *APIKeyStore) ListByTenant(tenantID string) ([]models.APIKey, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	rows, err := s.db.QueryContext(ctx, `
		SELECT id, tenant_id, name, key_prefix, owner_type, owner_id,
		       scopes_json, last_used_at, expires_at, revoked, revoked_at, revoked_by,
		       created_by, created_at
		FROM api_keys
		WHERE tenant_id = $1
		ORDER BY created_at DESC LIMIT 200`,
		tenantID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.APIKey
	for rows.Next() {
		k, err := scanAPIKey(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *k)
	}
	return out, rows.Err()
}

// Revoke marks an API key as revoked.
func (s *APIKeyStore) Revoke(id, tenantID, revokedBy string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	now := time.Now().UTC()
	_, err := s.db.ExecContext(ctx, `
		UPDATE api_keys SET revoked = true, revoked_at = $1, revoked_by = $2
		WHERE id = $3 AND tenant_id = $4`,
		now, revokedBy, id, tenantID,
	)
	return err
}

// UpdateLastUsed records the last time an API key was used.
func (s *APIKeyStore) UpdateLastUsed(id string, t time.Time) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err := s.db.ExecContext(ctx, 
		`UPDATE api_keys SET last_used_at = $1 WHERE id = $2`,
		t, id,
	)
	return err
}

// ── scan helper ───────────────────────────────────────────────────────────────

func scanAPIKey(row interface{ Scan(...interface{}) error }) (*models.APIKey, error) {
	var k models.APIKey
	var scopesJSON string
	var ownerType string
	err := row.Scan(
		&k.ID, &k.TenantID, &k.Name, &k.KeyPrefix,
		&ownerType, &k.OwnerID,
		&scopesJSON, &k.LastUsedAt, &k.ExpiresAt,
		&k.Revoked, &k.RevokedAt, &k.RevokedBy,
		&k.CreatedBy, &k.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	k.OwnerType = models.APIKeyOwnerType(ownerType)
	if err := unmarshalStringSlice(scopesJSON, &k.Scopes); err != nil {
		return nil, fmt.Errorf("api_key_store: unmarshal scopes: %w", err)
	}
	return &k, nil
}
