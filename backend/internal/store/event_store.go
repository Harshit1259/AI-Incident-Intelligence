package store

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"log/slog"
	"strings"
	"time"

	"github.com/lib/pq"

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
		`SELECT id, source, COALESCE(type,''), service, severity, COALESCE(title,''), message, timestamp,
		        COALESCE(labels_json,'')
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
		var labelsJSON string
		err := rows.Scan(
			&event.ID,
			&event.Source,
			&event.Type,
			&event.Service,
			&event.Severity,
			&event.Title,
			&event.Message,
			&event.Timestamp,
			&labelsJSON,
		)
		if err != nil {
			return nil, err
		}
		// Unreadable labels must not fail the whole query; the event still stands.
		if err := unmarshalStringMap(labelsJSON, &event.Labels); err != nil {
			slog.Error("event_store: failed to unmarshal labels", "event_id", event.ID, "error", err)
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
		`SELECT id, COALESCE(source,''), COALESCE(type,''), COALESCE(service,''), COALESCE(severity,''), COALESCE(title,''), COALESCE(message,''), timestamp,
		        COALESCE(labels_json,'')
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
		var labelsJSON string
		err := rows.Scan(
			&event.ID,
			&event.Source,
			&event.Type,
			&event.Service,
			&event.Severity,
			&event.Title,
			&event.Message,
			&event.Timestamp,
			&labelsJSON,
		)
		if err != nil {
			return nil, err
		}
		// Unreadable labels must not fail the whole query; the event still stands.
		if err := unmarshalStringMap(labelsJSON, &event.Labels); err != nil {
			slog.Error("event_store: failed to unmarshal labels", "event_id", event.ID, "error", err)
		}

		events = append(events, event)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return events, nil
}

// muteEventColumns are the fields alert-mute matching reads.
const muteEventColumns = `id, COALESCE(tenant_id,'default'), COALESCE(source,''), COALESCE(service,''), COALESCE(resource,''),
	COALESCE(environment,''), COALESCE(severity,''), COALESCE(title,''), timestamp,
	COALESCE(labels_json,''), COALESCE(ingest_schema,'')`

func (eventStore *EventStore) queryMuteEvents(query string, args ...any) ([]models.Event, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	rows, err := eventStore.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	events := make([]models.Event, 0)
	for rows.Next() {
		var e models.Event
		var labelsJSON string
		if err := rows.Scan(&e.ID, &e.TenantID, &e.Source, &e.Service, &e.Resource, &e.Environment,
			&e.Severity, &e.Title, &e.Timestamp, &labelsJSON, &e.IngestSchema); err != nil {
			return nil, err
		}
		if err := unmarshalStringMap(labelsJSON, &e.Labels); err != nil {
			slog.Error("event_store: failed to unmarshal labels", "event_id", e.ID, "error", err)
		}
		events = append(events, e)
	}
	return events, rows.Err()
}

// GetTenantEventsByIDs returns the tenant's events with the given IDs,
// including the fields alert mutes match on. IDs from another tenant are
// silently skipped.
func (eventStore *EventStore) GetTenantEventsByIDs(tenantID string, eventIDs []string) ([]models.Event, error) {
	if len(eventIDs) == 0 {
		return []models.Event{}, nil
	}
	placeholders := make([]string, len(eventIDs))
	args := make([]any, 0, len(eventIDs)+1)
	args = append(args, tenantID)
	for i, id := range eventIDs {
		placeholders[i] = fmt.Sprintf("$%d", i+2)
		args = append(args, id)
	}
	return eventStore.queryMuteEvents(
		`SELECT `+muteEventColumns+`
		   FROM events
		  WHERE tenant_id = $1 AND id IN (`+strings.Join(placeholders, ", ")+`)
		  ORDER BY timestamp DESC`,
		args...,
	)
}

// RecentTenantEvents returns up to limit of the tenant's events since the
// given time, newest first, including the fields alert mutes match on.
// A non-empty service narrows the scan.
func (eventStore *EventStore) RecentTenantEvents(tenantID, service string, since time.Time, limit int) ([]models.Event, error) {
	return eventStore.queryMuteEvents(
		`SELECT `+muteEventColumns+`
		   FROM events
		  WHERE tenant_id = $1 AND timestamp >= $2
		    AND ($3 = '' OR LOWER(service) = LOWER($3))
		  ORDER BY timestamp DESC
		  LIMIT $4`,
		tenantID, since, service, limit,
	)
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

	// Labels are stored as JSON. A marshal failure must not lose the event —
	// the labels are context, the event is the record — so fall back to empty.
	labelsJSON, err := marshalStringMap(e.Labels)
	if err != nil {
		slog.Error("event_store: failed to marshal labels", "event_id", e.ID, "error", err)
		labelsJSON = ""
	}

	_, err = s.db.ExecContext(ctx, `
		INSERT INTO events
		  (id, tenant_id, source, service, resource, environment, severity, type,
		   title, message, timestamp, fingerprint, external_id,
		   trace_id, span_id, ingest_schema, attrs_json, labels_json, alert_status)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19)
		ON CONFLICT (id) DO UPDATE
		  SET fingerprint   = EXCLUDED.fingerprint,
		      title         = EXCLUDED.title,
		      trace_id      = EXCLUDED.trace_id,
		      span_id       = EXCLUDED.span_id,
		      ingest_schema = EXCLUDED.ingest_schema,
		      attrs_json    = EXCLUDED.attrs_json,
		      -- Correlation re-saves the event to backfill its fingerprint, and
		      -- that copy may carry no labels. Keep the ones already stored
		      -- rather than letting the second write erase them.
		      -- marshalStringMap renders a nil map as "{}", so both forms mean
		      -- "this write carried no labels".
		      labels_json   = CASE WHEN EXCLUDED.labels_json IN ('', '{}') THEN events.labels_json
		                           -- A recovery keeps the value the alert fired with.
		                           WHEN EXCLUDED.alert_status = 'resolved' AND events.labels_json NOT IN ('', '{}')
		                             THEN (EXCLUDED.labels_json::jsonb || COALESCE(
		                                    (SELECT jsonb_object_agg(key, value)
		                                       FROM jsonb_each(events.labels_json::jsonb)
		                                      WHERE key = ANY($20)), '{}'::jsonb))::text
		                           ELSE EXCLUDED.labels_json END,
		      -- Firing → resolved → firing again all arrive on the same event ID
		      -- (Alertmanager's fingerprint); a write without a status keeps it.
		      alert_status  = CASE WHEN EXCLUDED.alert_status = '' THEN events.alert_status
		                           ELSE EXCLUDED.alert_status END
		  -- Never let a write from one tenant take over another tenant's row.
		  -- (This used to SET tenant_id = EXCLUDED.tenant_id.)
		  WHERE events.tenant_id = EXCLUDED.tenant_id
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
		labelsJSON,
		e.AlertStatus,
		pq.Array(models.AlertValueLabels),
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
// event with excludeID (so the event itself does not self-suppress). Only the
// tenant's own events count: another tenant's identical alert is not a duplicate.
func (s *EventStore) FingerprintSeenInWindow(tenantID, fingerprint string, windowStart time.Time, excludeID string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if fingerprint == "" {
		return false
	}
	if tenantID == "" {
		tenantID = "default"
	}
	var count int
	err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM events
		WHERE fingerprint = $1
		  AND timestamp   >= $2
		  AND id         != $3
		  AND tenant_id   = $4
	`, fingerprint, windowStart, excludeID, tenantID).Scan(&count)
	if err != nil {
		log.Printf("event_store: fingerprint dedup check failed: %v", err)
		return false
	}
	return count > 0
}
