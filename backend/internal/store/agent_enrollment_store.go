package store

import (
	"context"
	"time"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"

	"ai-incident-platform/backend/internal/models"
)

// AgentEnrollmentStore manages single-use bootstrap tokens for agent registration.
type AgentEnrollmentStore struct {
	db *sql.DB
}

// NewAgentEnrollmentStore creates a new AgentEnrollmentStore.
func NewAgentEnrollmentStore(db *sql.DB) *AgentEnrollmentStore {
	return &AgentEnrollmentStore{db: db}
}

// hashToken returns hex(SHA-256(rawToken)) for safe storage.
func hashToken(rawToken string) string {
	sum := sha256.Sum256([]byte(rawToken))
	return hex.EncodeToString(sum[:])
}

// Create persists a new enrollment token. rawToken is hashed before storage.
func (s *AgentEnrollmentStore) Create(t models.EnrollmentToken, rawToken string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO agent_enrollment_tokens
			(id, tenant_id, label, token_hash, expires_at, created_by, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, t.ID, t.TenantID, t.Label, hashToken(rawToken), t.ExpiresAt, t.CreatedBy, t.CreatedAt)
	return err
}

// FindByRawToken looks up an enrollment token by its plaintext value.
// Returns (nil, nil) when no matching token exists.
func (s *AgentEnrollmentStore) FindByRawToken(rawToken string) (*models.EnrollmentToken, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	h := hashToken(rawToken)
	row := s.db.QueryRowContext(ctx, `
		SELECT id, tenant_id, label, used_at, used_by_agent_id, expires_at, created_by, created_at
		FROM agent_enrollment_tokens
		WHERE token_hash = $1
	`, h)

	var t models.EnrollmentToken
	var usedAt sql.NullTime
	var usedByAgentID string
	err := row.Scan(&t.ID, &t.TenantID, &t.Label, &usedAt, &usedByAgentID,
		&t.ExpiresAt, &t.CreatedBy, &t.CreatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	if usedAt.Valid {
		t.UsedAt = &usedAt.Time
	}
	t.UsedByAgentID = usedByAgentID
	return &t, nil
}

// MarkUsed stamps the token as consumed by the given agent.
func (s *AgentEnrollmentStore) MarkUsed(id, agentID string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err := s.db.ExecContext(ctx, `
		UPDATE agent_enrollment_tokens
		SET used_at = NOW(), used_by_agent_id = $2
		WHERE id = $1
	`, id, agentID)
	return err
}

// ListByTenant returns all enrollment tokens for a tenant, newest first.
func (s *AgentEnrollmentStore) ListByTenant(tenantID string) ([]models.EnrollmentToken, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	rows, err := s.db.QueryContext(ctx, `
		SELECT id, tenant_id, label, used_at, used_by_agent_id, expires_at, created_by, created_at
		FROM agent_enrollment_tokens
		WHERE tenant_id = $1
		ORDER BY created_at DESC LIMIT 200
	`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tokens []models.EnrollmentToken
	for rows.Next() {
		var t models.EnrollmentToken
		var usedAt sql.NullTime
		var usedByAgentID string
		if err := rows.Scan(&t.ID, &t.TenantID, &t.Label, &usedAt, &usedByAgentID,
			&t.ExpiresAt, &t.CreatedBy, &t.CreatedAt); err != nil {
			return nil, err
		}
		if usedAt.Valid {
			t.UsedAt = &usedAt.Time
		}
		t.UsedByAgentID = usedByAgentID
		tokens = append(tokens, t)
	}
	return tokens, rows.Err()
}

// Delete removes an enrollment token scoped to a tenant (prevents cross-tenant deletion).
func (s *AgentEnrollmentStore) Delete(id, tenantID string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err := s.db.ExecContext(ctx, `
		DELETE FROM agent_enrollment_tokens WHERE id = $1 AND tenant_id = $2
	`, id, tenantID)
	return err
}

// DeleteExpired removes tokens that have passed their expiry and were never used.
// Call periodically from a maintenance goroutine.
func (s *AgentEnrollmentStore) DeleteExpired() error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err := s.db.ExecContext(ctx, `
		DELETE FROM agent_enrollment_tokens
		WHERE expires_at < NOW() AND used_at IS NULL
	`)
	return err
}

