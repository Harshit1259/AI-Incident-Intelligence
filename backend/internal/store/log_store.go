package store

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"strings"
	"time"

	"ai-incident-platform/backend/internal/models"
)

// LogStore provides persistence for log entries (Log Explorer feature).
type LogStore struct {
	db *sql.DB
}

// NewLogStore creates a new LogStore.
func NewLogStore(db *sql.DB) *LogStore {
	return &LogStore{db: db}
}

// Insert stores a single log entry.
func (s *LogStore) Insert(entry models.LogEntry) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO log_entries (tenant_id, agent_id, host_ip, log_source, log_tag, event_type, event_category, message, raw_json, timestamp)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
		entry.TenantID,
		entry.AgentID,
		entry.HostIP,
		entry.LogSource,
		entry.LogTag,
		entry.EventType,
		entry.EventCategory,
		entry.Message,
		entry.RawJSON,
		entry.Timestamp,
	)
	if err != nil {
		log.Printf("log_store: insert error: %v", err)
	}
	return err
}

// InsertBatch stores multiple log entries efficiently using a single INSERT
// with multiple VALUE clauses.
func (s *LogStore) InsertBatch(entries []models.LogEntry) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if len(entries) == 0 {
		return nil
	}

	const cols = 10 // number of columns per row
	valueStrings := make([]string, 0, len(entries))
	args := make([]interface{}, 0, len(entries)*cols)

	for i, e := range entries {
		base := i * cols
		valueStrings = append(valueStrings, fmt.Sprintf(
			"($%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d)",
			base+1, base+2, base+3, base+4, base+5,
			base+6, base+7, base+8, base+9, base+10,
		))
		args = append(args,
			e.TenantID,
			e.AgentID,
			e.HostIP,
			e.LogSource,
			e.LogTag,
			e.EventType,
			e.EventCategory,
			e.Message,
			e.RawJSON,
			e.Timestamp,
		)
	}

	query := `INSERT INTO log_entries (tenant_id, agent_id, host_ip, log_source, log_tag, event_type, event_category, message, raw_json, timestamp)
VALUES ` + strings.Join(valueStrings, ",")

	_, err := s.db.ExecContext(ctx, query, args...)
	if err != nil {
		log.Printf("log_store: batch insert error (%d entries): %v", len(entries), err)
	}
	return err
}

// Query returns filtered log entries with pagination.
func (s *LogStore) Query(q models.LogQuery) (*models.LogQueryResponse, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	where, args := buildLogWhere(q)

	// Count total matching rows
	countQuery := "SELECT COUNT(*) FROM log_entries" + where
	var total int
	if err := s.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, fmt.Errorf("log_store: count query: %w", err)
	}

	// Fetch page
	limit := q.Limit
	if limit <= 0 {
		limit = 100
	}
	offset := q.Offset
	if offset < 0 {
		offset = 0
	}

	nextParam := len(args) + 1
	dataQuery := `SELECT id, tenant_id, agent_id, host_ip, log_source, log_tag, event_type, event_category, message, raw_json, timestamp
FROM log_entries` + where + fmt.Sprintf(" ORDER BY timestamp DESC LIMIT $%d OFFSET $%d", nextParam, nextParam+1)
	args = append(args, limit, offset)

	rows, err := s.db.QueryContext(ctx, dataQuery, args...)
	if err != nil {
		return nil, fmt.Errorf("log_store: query: %w", err)
	}
	defer rows.Close()

	entries := make([]models.LogEntry, 0)
	for rows.Next() {
		var e models.LogEntry
		if err := rows.Scan(
			&e.ID, &e.TenantID, &e.AgentID, &e.HostIP,
			&e.LogSource, &e.LogTag, &e.EventType, &e.EventCategory,
			&e.Message, &e.RawJSON, &e.Timestamp,
		); err != nil {
			return nil, fmt.Errorf("log_store: scan: %w", err)
		}
		entries = append(entries, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("log_store: rows: %w", err)
	}

	return &models.LogQueryResponse{
		Entries: entries,
		Total:   total,
		HasMore: offset+len(entries) < total,
		Query:   q,
	}, nil
}

// GetStats returns aggregate counts for the log explorer dashboard.
func (s *LogStore) GetStats(tenantID string, since time.Time) (*models.LogStats, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	stats := &models.LogStats{
		ByCategory: make(map[string]int),
		ByHost:     make([]models.HostLogCount, 0),
		ByTag:      make([]models.TagLogCount, 0),
	}

	// Total + by category
	rows, err := s.db.QueryContext(ctx, 
		`SELECT event_category, COUNT(*) FROM log_entries WHERE tenant_id=$1 AND timestamp >= $2 GROUP BY event_category`,
		tenantID, since,
	)
	if err != nil {
		return nil, fmt.Errorf("log_store: stats by_category: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var cat string
		var cnt int
		if err := rows.Scan(&cat, &cnt); err != nil {
			return nil, err
		}
		stats.ByCategory[cat] = cnt
		stats.TotalLogs += cnt
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// By host (top 20)
	hostRows, err := s.db.QueryContext(ctx, 
		`SELECT host_ip, COUNT(*) AS cnt FROM log_entries WHERE tenant_id=$1 AND timestamp >= $2 GROUP BY host_ip ORDER BY cnt DESC LIMIT 20`,
		tenantID, since,
	)
	if err != nil {
		return nil, fmt.Errorf("log_store: stats by_host: %w", err)
	}
	defer hostRows.Close()

	for hostRows.Next() {
		var h models.HostLogCount
		if err := hostRows.Scan(&h.HostIP, &h.Count); err != nil {
			return nil, err
		}
		stats.ByHost = append(stats.ByHost, h)
	}
	if err := hostRows.Err(); err != nil {
		return nil, err
	}

	// By tag (top 20)
	tagRows, err := s.db.QueryContext(ctx, 
		`SELECT log_tag, COUNT(*) AS cnt FROM log_entries WHERE tenant_id=$1 AND timestamp >= $2 GROUP BY log_tag ORDER BY cnt DESC LIMIT 20`,
		tenantID, since,
	)
	if err != nil {
		return nil, fmt.Errorf("log_store: stats by_tag: %w", err)
	}
	defer tagRows.Close()

	for tagRows.Next() {
		var t models.TagLogCount
		if err := tagRows.Scan(&t.Tag, &t.Count); err != nil {
			return nil, err
		}
		stats.ByTag = append(stats.ByTag, t)
	}
	if err := tagRows.Err(); err != nil {
		return nil, err
	}

	// Recent errors (last 1 hour)
	oneHourAgo := time.Now().Add(-1 * time.Hour)
	if err := s.db.QueryRowContext(ctx, 
		`SELECT COUNT(*) FROM log_entries WHERE tenant_id=$1 AND event_category='error' AND timestamp >= $2`,
		tenantID, oneHourAgo,
	).Scan(&stats.RecentErrors); err != nil {
		return nil, fmt.Errorf("log_store: stats recent_errors: %w", err)
	}

	return stats, nil
}

// GetNewLogsSince returns log entries with id > sinceID for the given tenant, up to limit.
func (s *LogStore) GetNewLogsSince(tenantID string, sinceID int64, limit int) ([]models.LogEntry, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.QueryContext(ctx, 
		`SELECT id, tenant_id, agent_id, host_ip, log_source, log_tag, event_type, event_category, message, raw_json, timestamp
		FROM log_entries WHERE tenant_id=$1 AND id > $2 ORDER BY id ASC LIMIT $3`,
		tenantID, sinceID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []models.LogEntry
	for rows.Next() {
		var e models.LogEntry
		if err := rows.Scan(
			&e.ID, &e.TenantID, &e.AgentID, &e.HostIP,
			&e.LogSource, &e.LogTag, &e.EventType, &e.EventCategory,
			&e.Message, &e.RawJSON, &e.Timestamp,
		); err != nil {
			return nil, err
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

// buildLogWhere constructs a parameterized WHERE clause from a LogQuery.
func buildLogWhere(q models.LogQuery) (string, []interface{}) {
	var conditions []string
	var args []interface{}
	paramIdx := 1

	add := func(cond string, val interface{}) {
		conditions = append(conditions, fmt.Sprintf(cond, paramIdx))
		args = append(args, val)
		paramIdx++
	}

	if q.TenantID != "" {
		add("tenant_id = $%d", q.TenantID)
	}
	if q.AgentID != "" {
		add("agent_id = $%d", q.AgentID)
	}
	if q.HostIP != "" {
		add("host_ip = $%d", q.HostIP)
	}
	if q.Category != "" {
		add("event_category = $%d", q.Category)
	}
	if q.Tag != "" {
		add("log_tag = $%d", q.Tag)
	}
	if q.Search != "" {
		add("message ILIKE '%%' || $%d || '%%'", q.Search)
	}
	if q.From != nil {
		add("timestamp >= $%d", *q.From)
	}
	if q.To != nil {
		add("timestamp <= $%d", *q.To)
	}

	if len(conditions) == 0 {
		return "", nil
	}
	return " WHERE " + strings.Join(conditions, " AND "), args
}
