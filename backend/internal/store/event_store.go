package store

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"log"
	"strings"
	"time"

	"ai-incident-platform/backend/internal/models"
)

type EventStore struct {
	db *sql.DB
}

func NewEventStore(db *sql.DB) *EventStore {
	return &EventStore{db: db}
}

func (eventStore *EventStore) AddEvent(event models.Event) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err := eventStore.db.ExecContext(ctx, 
		`INSERT INTO events (id, source, type, service, severity, message, timestamp)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		event.ID,
		event.Source,
		event.Type,
		event.Service,
		event.Severity,
		event.Message,
		event.Timestamp,
	)

	return err
}

func (eventStore *EventStore) FindRecentDuplicate(event models.Event, window time.Duration) (models.Event, bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	eventTime := event.Timestamp // ✅ FIXED

	windowStart := eventTime.Add(-window)
	windowEnd := eventTime.Add(window)

	row := eventStore.db.QueryRowContext(ctx, 
		`SELECT id, source, type, service, severity, message, timestamp
		 FROM events
		 WHERE LOWER(service) = LOWER($1)
		   AND LOWER(severity) = LOWER($2)
		   AND LOWER(message) = LOWER($3)
		   AND timestamp >= $4
		   AND timestamp <= $5
		 ORDER BY timestamp DESC
		 LIMIT 1`,
		strings.TrimSpace(event.Service),
		strings.TrimSpace(event.Severity),
		strings.TrimSpace(event.Message),
		windowStart, // ✅ FIXED (no string format)
		windowEnd,
	)

	var duplicate models.Event
	err := row.Scan(
		&duplicate.ID,
		&duplicate.Source,
		&duplicate.Type,
		&duplicate.Service,
		&duplicate.Severity,
		&duplicate.Message,
		&duplicate.Timestamp,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return models.Event{}, false, nil
		}
		return models.Event{}, false, err
	}

	return duplicate, true, nil
}

func (eventStore *EventStore) GetEvents(limit, offset int) ([]models.Event, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if limit <= 0 {
		limit = 50
	}
	if limit > 1000 {
		limit = 1000
	}
	if offset < 0 {
		offset = 0
	}

	rows, err := eventStore.db.QueryContext(ctx,
		`SELECT id, source, COALESCE(type,''), service, severity, COALESCE(title,''), message, timestamp
		 FROM events
		 ORDER BY timestamp DESC
		 LIMIT $1 OFFSET $2`,
		limit, offset,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	events := make([]models.Event, 0)

	for rows.Next() {
		var event models.Event
		err := rows.Scan(
			&event.ID,
			&event.Source,
			&event.Type,
			&event.Service,
			&event.Severity,
			&event.Title,
			&event.Message,
			&event.Timestamp,
		)
		if err != nil {
			return nil, err
		}

		events = append(events, event)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return events, nil
}

func (eventStore *EventStore) GetEventsByIDs(eventIDs []string) ([]models.Event, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if len(eventIDs) == 0 {
		return []models.Event{}, nil
	}

	// Build dynamic IN clause — avoids pq array type issues with ANY($1)
	placeholders := make([]string, len(eventIDs))
	args := make([]interface{}, len(eventIDs))
	for i, id := range eventIDs {
		placeholders[i] = fmt.Sprintf("$%d", i+1)
		args[i] = id
	}
	query := fmt.Sprintf(
		`SELECT id, COALESCE(source,''), COALESCE(type,''), COALESCE(service,''), COALESCE(severity,''), COALESCE(title,''), COALESCE(message,''), timestamp
		 FROM events
		 WHERE id IN (%s)
		 ORDER BY timestamp ASC`,
		strings.Join(placeholders, ", "),
	)

	rows, err := eventStore.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	events := make([]models.Event, 0)

	for rows.Next() {
		var event models.Event
		err := rows.Scan(
			&event.ID,
			&event.Source,
			&event.Type,
			&event.Service,
			&event.Severity,
			&event.Title,
			&event.Message,
			&event.Timestamp,
		)
		if err != nil {
			return nil, err
		}

		events = append(events, event)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return events, nil
}

type stringArray []string

func (array stringArray) Value() (driver.Value, error) {
	if len(array) == 0 {
		return "{}", nil
	}

	result := "{"
	for index, value := range array {
		if index > 0 {
			result += ","
		}
		result += `"` + strings.ReplaceAll(value, `"`, `\"`) + `"`
	}
	result += "}"

	return result, nil
}

func (array *stringArray) Scan(src interface{}) error {
	if src == nil {
		*array = []string{}
		return nil
	}

	var raw string

	switch value := src.(type) {
	case string:
		raw = value
	case []byte:
		raw = string(value)
	default:
		return fmt.Errorf("unsupported type for stringArray scan: %T", src)
	}

	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "{}" {
		*array = []string{}
		return nil
	}

	if strings.HasPrefix(raw, "{") && strings.HasSuffix(raw, "}") {
		raw = raw[1 : len(raw)-1]
	}

	if strings.TrimSpace(raw) == "" {
		*array = []string{}
		return nil
	}

	parts := strings.Split(raw, ",")
	result := make([]string, 0, len(parts))

	for _, part := range parts {
		cleaned := strings.TrimSpace(part)
		cleaned = strings.Trim(cleaned, `"`)
		cleaned = strings.ReplaceAll(cleaned, `\"`, `"`)
		if cleaned != "" {
			result = append(result, cleaned)
		}
	}

	*array = result
	return nil
}

// SaveEvent persists an event including OTel-native fields.
// ON CONFLICT updates mutable fields so a re-ingested event refreshes its data.
func (s *EventStore) SaveEvent(e models.Event) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	tenantID := e.TenantID
	if tenantID == "" {
		tenantID = "default"
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO events
		  (id, tenant_id, source, service, resource, environment, severity, type,
		   title, message, timestamp, fingerprint, external_id,
		   trace_id, span_id, ingest_schema, attrs_json)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17)
		ON CONFLICT (id) DO UPDATE
		  SET fingerprint   = EXCLUDED.fingerprint,
		      title         = EXCLUDED.title,
		      tenant_id     = EXCLUDED.tenant_id,
		      trace_id      = EXCLUDED.trace_id,
		      span_id       = EXCLUDED.span_id,
		      ingest_schema = EXCLUDED.ingest_schema,
		      attrs_json    = EXCLUDED.attrs_json
	`,
		e.ID,
		tenantID,
		e.Source,
		e.Service,
		e.Resource,
		e.Environment,
		e.Severity,
		e.Type,
		e.Title,
		e.Message,
		e.Timestamp,
		e.Fingerprint,
		e.ExternalID,
		e.TraceID,
		e.SpanID,
		e.IngestSchema,
		e.AttrsJSON,
	)
	if err != nil {
		return fmt.Errorf("event_store: save event %s: %w", e.ID, err)
	}
	return nil
}

// CountEventsInPeriod returns the total number of events ingested for a tenant
// since the given time — the "raw alerts received" figure for noise dashboards.
func (s *EventStore) CountEventsInPeriod(tenantID string, since time.Time) (int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	var count int
	err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM events
		WHERE tenant_id = $1 AND timestamp >= $2
	`, tenantID, since).Scan(&count)
	return count, err
}

// CountUniqueFingerprints returns the number of distinct alert fingerprints seen
// for a tenant since the given time — approximates "after deduplication" because
// each fingerprint represents one unique alert pattern regardless of how many
// times it fired.
func (s *EventStore) CountUniqueFingerprints(tenantID string, since time.Time) (int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	var count int
	err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(DISTINCT fingerprint) FROM events
		WHERE tenant_id = $1 AND timestamp >= $2 AND fingerprint != ''
	`, tenantID, since).Scan(&count)
	return count, err
}

// FingerprintSeenInWindow returns true if an event with the same fingerprint
// already exists in the database within [windowStart, now), excluding the
// event with excludeID (so the event itself does not self-suppress).
func (s *EventStore) FingerprintSeenInWindow(fingerprint string, windowStart time.Time, excludeID string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if fingerprint == "" {
		return false
	}
	var count int
	err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM events
		WHERE fingerprint = $1
		  AND timestamp   >= $2
		  AND id         != $3
	`, fingerprint, windowStart, excludeID).Scan(&count)
	if err != nil {
		log.Printf("event_store: fingerprint dedup check failed: %v", err)
		return false
	}
	return count > 0
}
