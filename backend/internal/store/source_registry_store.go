package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"time"

	"ai-incident-platform/backend/internal/models"
	"ai-incident-platform/backend/internal/platform/crypto"
)

// SourceRegistryStore persists source connection registrations to PostgreSQL.
// When an Encryptor is provided and encryption is enabled, ingest tokens are
// stored as AES-256-GCM ciphertext; a SHA-256 hash of the plaintext token is
// kept separately for fast O(1) lookup without decrypting the entire table.
type SourceRegistryStore struct {
	db        *sql.DB
	encryptor *crypto.Encryptor // nil = encryption disabled
}

func NewSourceRegistryStore(db *sql.DB) *SourceRegistryStore {
	return &SourceRegistryStore{db: db}
}

// SetEncryptor attaches a field-level encryptor.
// Call this in main.go when MASTER_ENCRYPTION_KEY is configured.
func (s *SourceRegistryStore) SetEncryptor(e *crypto.Encryptor) {
	s.encryptor = e
}

// tokenHash returns the hex-encoded SHA-256 of the raw token — used as
// the lookup key when encryption is active.
func tokenHash(raw string) string {
	h := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(h[:])
}

// Create inserts a new source connection record.
// When the encryptor is active, the token is stored encrypted and the
// token_hash column is populated for lookup.
func (s *SourceRegistryStore) Create(src models.SourceConnection, tenantID string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	storedToken := src.Token
	hash := ""

	if s.encryptor != nil && s.encryptor.IsEnabled() {
		hash = tokenHash(src.Token)
		var err error
		storedToken, err = s.encryptor.EncryptString(tenantID, src.Token)
		if err != nil {
			return err
		}
	}

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO source_registry
		    (id, tenant_id, name, source_type, endpoint, status, event_count,
		     ingest_token, ingest_token_hash, created_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
		ON CONFLICT (id) DO NOTHING`,
		src.ID, tenantID, src.Name, src.Type, src.Endpoint, src.Status, src.TotalEvents,
		storedToken, hash, time.Now().UTC(),
	)
	return err
}

// scanSource reads all source_registry columns including the hardening fields.
func scanSource(row interface {
	Scan(dest ...any) error
}, src *models.SourceConnection) error {
	var lastEvent, lastErrorAt sql.NullTime
	err := row.Scan(
		&src.ID, &src.TenantID, &src.Name, &src.Type, &src.Endpoint,
		&src.Status, &lastEvent, &src.TotalEvents, &src.CreatedAt,
		&src.Token, &src.LastError, &lastErrorAt, &src.ErrorCount,
	)
	if err != nil {
		return err
	}
	if lastEvent.Valid {
		t := lastEvent.Time
		src.LastEventAt = &t
	}
	if lastErrorAt.Valid {
		t := lastErrorAt.Time
		src.LastErrorAt = &t
	}
	return nil
}

const sourceSelectCols = `
	SELECT id, tenant_id, name, source_type, endpoint, status,
	       last_event, event_count, created_at,
	       COALESCE(ingest_token, ''),
	       COALESCE(last_error, ''),
	       last_error_at,
	       COALESCE(error_count, 0)
	FROM source_registry`

// List returns all source connections for a tenant.
func (s *SourceRegistryStore) List(tenantID string) ([]models.SourceConnection, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	rows, err := s.db.QueryContext(ctx, sourceSelectCols+`
		WHERE tenant_id = $1
		ORDER BY created_at DESC LIMIT 200`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []models.SourceConnection
	for rows.Next() {
		var src models.SourceConnection
		if err := scanSource(rows, &src); err != nil {
			return nil, err
		}
		result = append(result, src)
	}
	return result, rows.Err()
}

// FindByID fetches a source by primary key.
func (s *SourceRegistryStore) FindByID(id string) (*models.SourceConnection, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	row := s.db.QueryRowContext(ctx, sourceSelectCols+` WHERE id = $1`, id)

	var src models.SourceConnection
	if err := scanSource(row, &src); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &src, nil
}

// FindByIngestToken resolves a source by its ingest token. When encryption is
// active, lookup is by token_hash. Falls back to plaintext lookup for
// pre-existing records that pre-date encryption (hash column is empty).
func (s *SourceRegistryStore) FindByIngestToken(token string) (*models.SourceConnection, string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if token == "" {
		return nil, "", nil
	}

	var row *sql.Row

	if s.encryptor != nil && s.encryptor.IsEnabled() {
		// Primary: lookup by hash — O(1) indexed, never exposes plaintext in WHERE clause
		hash := tokenHash(token)
		row = s.db.QueryRowContext(ctx, sourceSelectCols+` WHERE ingest_token_hash = $1`, hash)
	} else {
		row = s.db.QueryRowContext(ctx, sourceSelectCols+` WHERE ingest_token = $1`, token)
	}

	var src models.SourceConnection
	if err := scanSource(row, &src); err != nil {
		if err == sql.ErrNoRows {
			// Fallback for older records: plaintext token lookup (hash column empty)
			if s.encryptor != nil && s.encryptor.IsEnabled() {
				fallbackRow := s.db.QueryRowContext(ctx, sourceSelectCols+` WHERE ingest_token = $1 AND ingest_token_hash = ''`, token)
				if err2 := scanSource(fallbackRow, &src); err2 != nil {
					return nil, "", nil
				}
				return &src, src.TenantID, nil
			}
			return nil, "", nil
		}
		return nil, "", err
	}
	return &src, src.TenantID, nil
}

// RecordSuccess updates the status and event count after successful ingestion.
func (s *SourceRegistryStore) RecordSuccess(id string, eventCount int) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err := s.db.ExecContext(ctx, `
		UPDATE source_registry
		SET status      = 'healthy',
		    last_event  = NOW(),
		    event_count = event_count + $1,
		    error_count = 0,
		    last_error  = ''
		WHERE id = $2`, eventCount, id)
	return err
}

// RecordError marks the source status as error and persists the error message.
func (s *SourceRegistryStore) RecordError(id string, errMsg string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err := s.db.ExecContext(ctx, `
		UPDATE source_registry
		SET    status       = 'error',
		       last_error    = $1,
		       last_error_at = NOW(),
		       error_count   = error_count + 1
		WHERE  id = $2`, errMsg, id)
	return err
}

// ResetErrorCount clears the error counter when a source recovers.
func (s *SourceRegistryStore) ResetErrorCount(id string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err := s.db.ExecContext(ctx, `
		UPDATE source_registry
		SET error_count = 0, last_error = '', last_error_at = NULL
		WHERE id = $1`, id)
	return err
}

// ErrSourceNotFound is returned when a source does not exist for the tenant.
var ErrSourceNotFound = errors.New("source not found")

// RotateToken replaces a source's ingest token. The old token stops working
// immediately. Only the tenant that owns the source can rotate it.
func (s *SourceRegistryStore) RotateToken(tenantID, id, newToken string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	storedToken := newToken
	hash := ""
	if s.encryptor != nil && s.encryptor.IsEnabled() {
		hash = tokenHash(newToken)
		var err error
		storedToken, err = s.encryptor.EncryptString(tenantID, newToken)
		if err != nil {
			return err
		}
	}

	res, err := s.db.ExecContext(ctx, `
		UPDATE source_registry
		   SET ingest_token = $3, ingest_token_hash = $4
		 WHERE id = $1 AND tenant_id = $2`,
		id, tenantID, storedToken, hash)
	if err != nil {
		return err
	}
	if n, err := res.RowsAffected(); err == nil && n == 0 {
		return ErrSourceNotFound
	}
	return nil
}

// Delete removes a source; its token stops working immediately.
func (s *SourceRegistryStore) Delete(tenantID, id string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	res, err := s.db.ExecContext(ctx,
		`DELETE FROM source_registry WHERE id = $1 AND tenant_id = $2`, id, tenantID)
	if err != nil {
		return err
	}
	if n, err := res.RowsAffected(); err == nil && n == 0 {
		return ErrSourceNotFound
	}
	return nil
}
