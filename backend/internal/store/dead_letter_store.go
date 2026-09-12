package store

import (
	"context"
	"database/sql"
	"time"
)

// DeadLetterEntry holds a webhook payload that failed processing after passing auth/validation.
type DeadLetterEntry struct {
	ID          int64
	TenantID    string
	SourceID    string
	SourceType  string
	Endpoint    string
	RawPayload  string
	Error       string
	RetryCount  int
	CreatedAt   time.Time
	LastRetryAt *time.Time
}

// DeadLetterStore persists and retrieves dead-letter entries from the DLQ table.
type DeadLetterStore struct {
	db *sql.DB
}

func NewDeadLetterStore(db *sql.DB) *DeadLetterStore {
	return &DeadLetterStore{db: db}
}

// Enqueue writes a failed payload to the dead-letter queue.
func (s *DeadLetterStore) Enqueue(e DeadLetterEntry) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO webhook_dead_letters
		    (tenant_id, source_id, source_type, endpoint, raw_payload, error, retry_count)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		e.TenantID, e.SourceID, e.SourceType, e.Endpoint,
		e.RawPayload, e.Error, e.RetryCount)
	return err
}

// List returns dead-letter entries for a tenant, newest first, up to limit rows.
func (s *DeadLetterStore) List(tenantID string, limit int) ([]DeadLetterEntry, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, tenant_id, source_id, source_type, endpoint,
		       raw_payload, error, retry_count, created_at, last_retry_at
		FROM webhook_dead_letters
		WHERE tenant_id = $1
		ORDER BY created_at DESC
		LIMIT $2`, tenantID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []DeadLetterEntry
	for rows.Next() {
		var e DeadLetterEntry
		var lastRetry sql.NullTime
		if err := rows.Scan(
			&e.ID, &e.TenantID, &e.SourceID, &e.SourceType, &e.Endpoint,
			&e.RawPayload, &e.Error, &e.RetryCount, &e.CreatedAt, &lastRetry,
		); err != nil {
			return nil, err
		}
		if lastRetry.Valid {
			t := lastRetry.Time
			e.LastRetryAt = &t
		}
		result = append(result, e)
	}
	return result, rows.Err()
}

// Count returns how many unresolved dead-letter entries exist for a tenant.
func (s *DeadLetterStore) Count(tenantID string) (int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var n int
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM webhook_dead_letters WHERE tenant_id = $1`, tenantID,
	).Scan(&n)
	return n, err
}

// DeadLetterGroup is a set of dead-letter entries that share the same origin
// and the same failure cause, collapsed into a single row with a count.
//
// This is what the notification feed reads. A misconfigured source can produce
// thousands of identical failures; listing them individually is unusable in a
// UI, and the only facts an operator needs are "which source", "what broke",
// "how many", "since when", and one sample payload as evidence.
type DeadLetterGroup struct {
	SourceID      string
	SourceType    string
	Endpoint      string
	Error         string
	Count         int
	FirstSeen     time.Time
	LastSeen      time.Time
	SamplePayload string
}

// maxSamplePayloadBytes caps the evidence payload returned per group.
// Raw payloads can be up to 1 MiB each; the feed only needs enough to
// recognise the shape of the message.
const maxSamplePayloadBytes = 2000

// ListGrouped returns dead-letter entries collapsed by (source, endpoint, error),
// most recent failure first, capped at limit groups.
//
// The sample payload is the newest payload in each group, truncated in SQL so a
// large blob never crosses the wire.
func (s *DeadLetterStore) ListGrouped(tenantID string, limit int) ([]DeadLetterGroup, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if limit <= 0 || limit > 200 {
		limit = 50
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT source_id,
		       source_type,
		       endpoint,
		       error,
		       COUNT(*)                                                   AS occurrences,
		       MIN(created_at)                                            AS first_seen,
		       MAX(created_at)                                            AS last_seen,
		       LEFT((ARRAY_AGG(raw_payload ORDER BY created_at DESC))[1], $3) AS sample
		FROM webhook_dead_letters
		WHERE tenant_id = $1
		GROUP BY source_id, source_type, endpoint, error
		ORDER BY MAX(created_at) DESC
		LIMIT $2`, tenantID, limit, maxSamplePayloadBytes)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	groups := make([]DeadLetterGroup, 0, limit)
	for rows.Next() {
		var g DeadLetterGroup
		var sample sql.NullString
		if err := rows.Scan(
			&g.SourceID, &g.SourceType, &g.Endpoint, &g.Error,
			&g.Count, &g.FirstSeen, &g.LastSeen, &sample,
		); err != nil {
			return nil, err
		}
		g.SamplePayload = sample.String
		groups = append(groups, g)
	}
	return groups, rows.Err()
}
