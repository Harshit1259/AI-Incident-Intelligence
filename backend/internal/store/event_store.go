package store

import (
	"database/sql"
	"database/sql/driver"
	"fmt"
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
	_, err := eventStore.db.Exec(
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
	eventTime := event.Timestamp // ✅ FIXED

	windowStart := eventTime.Add(-window)
	windowEnd := eventTime.Add(window)

	row := eventStore.db.QueryRow(
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

func (eventStore *EventStore) GetEvents() ([]models.Event, error) {
	rows, err := eventStore.db.Query(
		`SELECT id, source, type, service, severity, message, timestamp
		 FROM events
		 ORDER BY timestamp DESC`,
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
		`SELECT id, source, type, service, severity, title, message, timestamp
		 FROM events
		 WHERE id IN (%s)
		 ORDER BY timestamp ASC`,
		strings.Join(placeholders, ", "),
	)

	rows, err := eventStore.db.Query(query, args...)
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

// SaveEvent persists an event including its fingerprint for deduplication.
// If the event already exists (same id), its fingerprint and title are updated
// so that a pre-computed fingerprint from ProcessEvent overwrites an empty one
// saved earlier by the ingest handler.
func (s *EventStore) SaveEvent(e models.Event) {
	_, _ = s.db.Exec(`
		INSERT INTO events (id, source, service, severity, title, message, timestamp, fingerprint)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
		ON CONFLICT (id) DO UPDATE
		  SET fingerprint = EXCLUDED.fingerprint,
		      title       = EXCLUDED.title
	`,
		e.ID,
		e.Source,
		e.Service,
		e.Severity,
		e.Title,
		e.Message,
		e.Timestamp,
		e.Fingerprint,
	)
}

// FingerprintSeenInWindow returns true if an event with the same fingerprint
// already exists in the database within [windowStart, now), excluding the
// event with excludeID (so the event itself does not self-suppress).
func (s *EventStore) FingerprintSeenInWindow(fingerprint string, windowStart time.Time, excludeID string) bool {
	if fingerprint == "" {
		return false
	}
	var count int
	err := s.db.QueryRow(`
		SELECT COUNT(*)
		FROM events
		WHERE fingerprint = $1
		  AND timestamp   >= $2
		  AND id         != $3
	`, fingerprint, windowStart, excludeID).Scan(&count)
	if err != nil {
		return false
	}
	return count > 0
}
