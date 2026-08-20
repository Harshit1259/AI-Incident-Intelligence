package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// AuditLogEntry represents a single entry in the platform audit log.
type AuditLogEntry struct {
	ID           int64     `json:"id"`
	TenantID     string    `json:"tenant_id"`
	Actor        string    `json:"actor"`
	Action       string    `json:"action"`
	ResourceType string    `json:"resource_type"`
	ResourceID   string    `json:"resource_id"`
	DetailsJSON  string    `json:"details_json"`
	IPAddress    string    `json:"ip_address"`
	CreatedAt    time.Time `json:"created_at"`
}

// AuditLogStore provides persistence for the comprehensive audit trail.
type AuditLogStore struct {
	db *sql.DB
}

// NewAuditLogStore creates a new AuditLogStore.
func NewAuditLogStore(db *sql.DB) *AuditLogStore {
	return &AuditLogStore{db: db}
}

// Log inserts a new audit log entry.
func (s *AuditLogStore) Log(entry AuditLogEntry) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO platform_audit_log (tenant_id, actor, action, resource_type, resource_id, details_json, ip_address)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		entry.TenantID, entry.Actor, entry.Action, entry.ResourceType,
		entry.ResourceID, entry.DetailsJSON, entry.IPAddress,
	)
	return err
}

// Query returns audit log entries with filtering and pagination.
func (s *AuditLogStore) Query(tenantID, resourceType string, limit, offset int) ([]AuditLogEntry, int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if limit <= 0 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}

	where := " WHERE tenant_id = $1"
	args := []interface{}{tenantID}
	argIdx := 2

	if resourceType != "" {
		where += fmt.Sprintf(" AND resource_type = $%d", argIdx)
		args = append(args, resourceType)
		argIdx++
	}

	// Count total
	var total int
	countQuery := "SELECT COUNT(*) FROM platform_audit_log" + where
	if err := s.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("audit_log_store: count: %w", err)
	}

	// Fetch page
	dataQuery := fmt.Sprintf(`
		SELECT id, tenant_id, actor, action, resource_type, resource_id, details_json, ip_address, created_at
		FROM platform_audit_log%s
		ORDER BY created_at DESC
		LIMIT $%d OFFSET $%d`, where, argIdx, argIdx+1)
	args = append(args, limit, offset)

	rows, err := s.db.QueryContext(ctx, dataQuery, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("audit_log_store: query: %w", err)
	}
	defer rows.Close()

	var entries []AuditLogEntry
	for rows.Next() {
		var e AuditLogEntry
		if err := rows.Scan(&e.ID, &e.TenantID, &e.Actor, &e.Action, &e.ResourceType,
			&e.ResourceID, &e.DetailsJSON, &e.IPAddress, &e.CreatedAt); err != nil {
			return nil, 0, fmt.Errorf("audit_log_store: scan: %w", err)
		}
		entries = append(entries, e)
	}
	return entries, total, rows.Err()
}

// AuditSearchQuery contains all optional filters for a tenant-scoped audit search.
// TenantID is mandatory and always enforced.
type AuditSearchQuery struct {
	TenantID     string
	Actor        string
	Action       string
	ResourceType string
	ResourceID   string
	FromTime     *time.Time
	ToTime       *time.Time
	Limit        int
	Offset       int
}

// Search performs a tenant-scoped full-filter audit log search.
func (s *AuditLogStore) Search(q AuditSearchQuery) ([]AuditLogEntry, int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if q.Limit <= 0 {
		q.Limit = 50
	}
	if q.Offset < 0 {
		q.Offset = 0
	}

	where := " WHERE tenant_id = $1"
	args := []any{q.TenantID}
	idx := 2

	if q.Actor != "" {
		where += fmt.Sprintf(" AND actor ILIKE $%d", idx)
		args = append(args, "%"+q.Actor+"%")
		idx++
	}
	if q.Action != "" {
		where += fmt.Sprintf(" AND action ILIKE $%d", idx)
		args = append(args, "%"+q.Action+"%")
		idx++
	}
	if q.ResourceType != "" {
		where += fmt.Sprintf(" AND resource_type = $%d", idx)
		args = append(args, q.ResourceType)
		idx++
	}
	if q.ResourceID != "" {
		where += fmt.Sprintf(" AND resource_id = $%d", idx)
		args = append(args, q.ResourceID)
		idx++
	}
	if q.FromTime != nil {
		where += fmt.Sprintf(" AND created_at >= $%d", idx)
		args = append(args, *q.FromTime)
		idx++
	}
	if q.ToTime != nil {
		where += fmt.Sprintf(" AND created_at <= $%d", idx)
		args = append(args, *q.ToTime)
		idx++
	}

	var total int
	if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM platform_audit_log"+where, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("audit search count: %w", err)
	}

	dataQuery := fmt.Sprintf(`
		SELECT id, tenant_id, actor, action, resource_type, resource_id, details_json, ip_address, created_at
		FROM platform_audit_log%s ORDER BY created_at DESC LIMIT $%d OFFSET $%d`,
		where, idx, idx+1,
	)
	args = append(args, q.Limit, q.Offset)

	rows, err := s.db.QueryContext(ctx, dataQuery, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("audit search query: %w", err)
	}
	defer rows.Close()

	var entries []AuditLogEntry
	for rows.Next() {
		var e AuditLogEntry
		if err := rows.Scan(&e.ID, &e.TenantID, &e.Actor, &e.Action, &e.ResourceType,
			&e.ResourceID, &e.DetailsJSON, &e.IPAddress, &e.CreatedAt); err != nil {
			return nil, 0, fmt.Errorf("audit search scan: %w", err)
		}
		entries = append(entries, e)
	}
	if entries == nil {
		entries = []AuditLogEntry{}
	}
	return entries, total, rows.Err()
}
