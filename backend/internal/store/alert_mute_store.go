package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"ai-incident-platform/backend/internal/models"
)

// ErrMuteNotActive is returned by Unmute when the mute does not exist for the
// tenant, has already expired, or was already unmuted.
var ErrMuteNotActive = errors.New("mute not found or no longer active")

// AlertMuteStore persists alert mutes and the log of alerts they stopped.
// Every query is scoped by tenant_id.
type AlertMuteStore struct {
	db *sql.DB
}

func NewAlertMuteStore(db *sql.DB) *AlertMuteStore {
	return &AlertMuteStore{db: db}
}

const alertMuteColumns = `id, tenant_id, alert_names_json, devices_json, service, environment,
	value_min, value_max, reason, incident_id, created_by, created_at, ends_at,
	unmuted_at, unmuted_by, match_count, last_matched_at`

func scanAlertMute(scan func(...any) error) (models.AlertMute, error) {
	var (
		m                    models.AlertMute
		namesJSON, devsJSON  string
		vMin, vMax           sql.NullFloat64
		unmutedAt, lastMatch sql.NullTime
	)
	if err := scan(&m.ID, &m.TenantID, &namesJSON, &devsJSON, &m.Service, &m.Environment,
		&vMin, &vMax, &m.Reason, &m.IncidentID, &m.CreatedBy, &m.CreatedAt, &m.EndsAt,
		&unmutedAt, &m.UnmutedBy, &m.MatchCount, &lastMatch); err != nil {
		return m, err
	}
	if err := unmarshalStringSlice(namesJSON, &m.AlertNames); err != nil {
		return m, err
	}
	if err := unmarshalStringSlice(devsJSON, &m.Devices); err != nil {
		return m, err
	}
	if vMin.Valid {
		m.ValueMin = &vMin.Float64
	}
	if vMax.Valid {
		m.ValueMax = &vMax.Float64
	}
	if unmutedAt.Valid {
		m.UnmutedAt = &unmutedAt.Time
	}
	if lastMatch.Valid {
		m.LastMatchedAt = &lastMatch.Time
	}
	return m, nil
}

func (s *AlertMuteStore) queryMutes(query string, args ...any) ([]models.AlertMute, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	mutes := make([]models.AlertMute, 0)
	for rows.Next() {
		m, err := scanAlertMute(rows.Scan)
		if err != nil {
			return nil, err
		}
		mutes = append(mutes, m)
	}
	return mutes, rows.Err()
}

// Create inserts a new mute.
func (s *AlertMuteStore) Create(m models.AlertMute) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	namesJSON, err := marshalStringSlice(m.AlertNames)
	if err != nil {
		return err
	}
	devsJSON, err := marshalStringSlice(m.Devices)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx,
		`INSERT INTO alert_mutes
		   (id, tenant_id, alert_names_json, devices_json, service, environment,
		    value_min, value_max, reason, incident_id, created_by, created_at, ends_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)`,
		m.ID, m.TenantID, namesJSON, devsJSON, m.Service, m.Environment,
		m.ValueMin, m.ValueMax, m.Reason, m.IncidentID, m.CreatedBy, m.CreatedAt, m.EndsAt,
	)
	return err
}

// ActiveMutes returns the tenant's mutes that are in force at now.
func (s *AlertMuteStore) ActiveMutes(tenantID string, now time.Time) ([]models.AlertMute, error) {
	return s.queryMutes(
		`SELECT `+alertMuteColumns+`
		   FROM alert_mutes
		  WHERE tenant_id = $1 AND unmuted_at IS NULL AND ends_at > $2
		  ORDER BY created_at ASC`,
		tenantID, now,
	)
}

// ListRecent returns the tenant's mutes that are active or ended after since,
// newest first.
func (s *AlertMuteStore) ListRecent(tenantID string, since time.Time) ([]models.AlertMute, error) {
	return s.queryMutes(
		`SELECT `+alertMuteColumns+`
		   FROM alert_mutes
		  WHERE tenant_id = $1 AND COALESCE(unmuted_at, ends_at) > $2
		  ORDER BY created_at DESC
		  LIMIT 200`,
		tenantID, since,
	)
}

// Unmute ends an active mute early.
func (s *AlertMuteStore) Unmute(tenantID, id, userID string, now time.Time) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	res, err := s.db.ExecContext(ctx,
		`UPDATE alert_mutes SET unmuted_at = $4, unmuted_by = $3
		  WHERE tenant_id = $1 AND id = $2 AND unmuted_at IS NULL AND ends_at > $4`,
		tenantID, id, userID, now,
	)
	if err != nil {
		return err
	}
	if n, err := res.RowsAffected(); err == nil && n == 0 {
		return ErrMuteNotActive
	}
	return nil
}

// RecordMatch writes a mute-log row and bumps the mute's counters in one
// transaction, so the drawer's count and log never disagree.
func (s *AlertMuteStore) RecordMatch(e models.AlertMuteLogEntry) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck

	if _, err := tx.ExecContext(ctx,
		`INSERT INTO alert_mute_log (tenant_id, mute_id, event_id, alert_name, device, service, value, received_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`,
		e.TenantID, e.MuteID, e.EventID, e.AlertName, e.Device, e.Service, e.Value, e.ReceivedAt,
	); err != nil {
		return fmt.Errorf("insert mute log: %w", err)
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE alert_mutes SET match_count = match_count + 1, last_matched_at = $3
		  WHERE tenant_id = $1 AND id = $2`,
		e.TenantID, e.MuteID, e.ReceivedAt,
	); err != nil {
		return fmt.Errorf("bump mute counter: %w", err)
	}
	return tx.Commit()
}

// ListLog returns the tenant's muted alerts, newest first, optionally for one mute.
func (s *AlertMuteStore) ListLog(tenantID, muteID string, limit, offset int) ([]models.AlertMuteLogEntry, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	rows, err := s.db.QueryContext(ctx,
		`SELECT id, tenant_id, mute_id, event_id, alert_name, device, service, value, received_at
		   FROM alert_mute_log
		  WHERE tenant_id = $1 AND ($2 = '' OR mute_id = $2)
		  ORDER BY received_at DESC, id DESC
		  LIMIT $3 OFFSET $4`,
		tenantID, muteID, limit, offset,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	entries := make([]models.AlertMuteLogEntry, 0)
	for rows.Next() {
		var e models.AlertMuteLogEntry
		if err := rows.Scan(&e.ID, &e.TenantID, &e.MuteID, &e.EventID, &e.AlertName,
			&e.Device, &e.Service, &e.Value, &e.ReceivedAt); err != nil {
			return nil, err
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

// CountLogSince returns how many alerts were muted for the tenant since t.
func (s *AlertMuteStore) CountLogSince(tenantID string, since time.Time) (int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var n int
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM alert_mute_log WHERE tenant_id = $1 AND received_at >= $2`,
		tenantID, since,
	).Scan(&n)
	return n, err
}
