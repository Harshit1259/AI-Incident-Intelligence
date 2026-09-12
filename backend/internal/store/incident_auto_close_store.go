package store

import (
	"context"
	"database/sql"
	"time"
)

// AutoClosePending is an incident whose alerts have all recovered and which
// closes automatically at ClosesAt unless an alert fires again first.
type AutoClosePending struct {
	IncidentID  string
	TenantID    string
	RecoveredAt time.Time
	ClosesAt    time.Time
	AlertCount  int
}

// IncidentAutoCloseStore backs auto-close on recovery. Every query that reads
// events or incidents is scoped by tenant.
type IncidentAutoCloseStore struct {
	db *sql.DB
}

func NewIncidentAutoCloseStore(db *sql.DB) *IncidentAutoCloseStore {
	return &IncidentAutoCloseStore{db: db}
}

// OpenIncidentsWithEvent returns the tenant's open or acknowledged incidents
// that contain the event.
func (s *IncidentAutoCloseStore) OpenIncidentsWithEvent(tenantID, eventID string) ([]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	rows, err := s.db.QueryContext(ctx,
		`SELECT i.id
		   FROM incident_events ie
		   JOIN incidents i ON i.id = ie.incident_id
		  WHERE ie.event_id = $1
		    AND COALESCE(i.tenant_id, 'default') = $2
		    AND i.status IN ('open', 'acknowledged')`,
		eventID, tenantID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	ids := make([]string, 0)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// AlertCounts returns how many alerts an incident holds and how many of them
// are not resolved. Alerts from sources that never send a resolve (empty status)
// count as not resolved, so such incidents are never auto-closed.
func (s *IncidentAutoCloseStore) AlertCounts(tenantID, incidentID string) (total, unresolved int, err error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	err = s.db.QueryRowContext(ctx,
		`SELECT COUNT(*),
		        COUNT(*) FILTER (WHERE e.alert_status <> 'resolved')
		   FROM incident_events ie
		   JOIN events e ON e.id = ie.event_id
		  WHERE ie.incident_id = $1
		    AND COALESCE(e.tenant_id, 'default') = $2`,
		incidentID, tenantID,
	).Scan(&total, &unresolved)
	return total, unresolved, err
}

// MarkRecovered starts (or restarts) an incident's quiet period.
func (s *IncidentAutoCloseStore) MarkRecovered(p AutoClosePending) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err := s.db.ExecContext(ctx,
		`INSERT INTO incident_auto_close (incident_id, tenant_id, recovered_at, closes_at, alert_count)
		 VALUES ($1, $2, $3, $4, $5)
		 ON CONFLICT (incident_id) DO UPDATE
		   SET recovered_at = EXCLUDED.recovered_at,
		       closes_at    = EXCLUDED.closes_at,
		       alert_count  = EXCLUDED.alert_count`,
		p.IncidentID, p.TenantID, p.RecoveredAt, p.ClosesAt, p.AlertCount,
	)
	return err
}

// ClearRecovered cancels a pending auto-close (an alert fired again).
func (s *IncidentAutoCloseStore) ClearRecovered(incidentID string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err := s.db.ExecContext(ctx, `DELETE FROM incident_auto_close WHERE incident_id = $1`, incidentID)
	return err
}

// Pending returns the incident's pending auto-close, or nil.
func (s *IncidentAutoCloseStore) Pending(incidentID string) (*AutoClosePending, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var p AutoClosePending
	err := s.db.QueryRowContext(ctx,
		`SELECT incident_id, tenant_id, recovered_at, closes_at, alert_count
		   FROM incident_auto_close WHERE incident_id = $1`, incidentID,
	).Scan(&p.IncidentID, &p.TenantID, &p.RecoveredAt, &p.ClosesAt, &p.AlertCount)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// ClaimDue removes and returns up to limit pending rows whose quiet period has
// ended. Removing them in one statement (skipping rows another server holds)
// means each incident is claimed by exactly one server.
func (s *IncidentAutoCloseStore) ClaimDue(now time.Time, limit int) ([]AutoClosePending, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	rows, err := s.db.QueryContext(ctx,
		`DELETE FROM incident_auto_close
		  WHERE incident_id IN (
		        SELECT incident_id FROM incident_auto_close
		         WHERE closes_at <= $1
		         ORDER BY closes_at
		         LIMIT $2
		         FOR UPDATE SKIP LOCKED)
		 RETURNING incident_id, tenant_id, recovered_at, closes_at, alert_count`,
		now, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	due := make([]AutoClosePending, 0)
	for rows.Next() {
		var p AutoClosePending
		if err := rows.Scan(&p.IncidentID, &p.TenantID, &p.RecoveredAt, &p.ClosesAt, &p.AlertCount); err != nil {
			return nil, err
		}
		due = append(due, p)
	}
	return due, rows.Err()
}

// IncidentIsOpen reports whether the tenant's incident is open or acknowledged.
func (s *IncidentAutoCloseStore) IncidentIsOpen(tenantID, incidentID string) (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var open bool
	err := s.db.QueryRowContext(ctx,
		`SELECT EXISTS (SELECT 1 FROM incidents
		                 WHERE id = $1 AND COALESCE(tenant_id, 'default') = $2
		                   AND status IN ('open', 'acknowledged'))`,
		incidentID, tenantID,
	).Scan(&open)
	return open, err
}

// OpenIncidentStatusesWithEvent returns incident ID → status for the tenant's
// open or acknowledged incidents that contain the event.
func (s *IncidentAutoCloseStore) OpenIncidentStatusesWithEvent(tenantID, eventID string) (map[string]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	rows, err := s.db.QueryContext(ctx,
		`SELECT i.id, i.status
		   FROM incident_events ie
		   JOIN incidents i ON i.id = ie.incident_id
		  WHERE ie.event_id = $1
		    AND COALESCE(i.tenant_id, 'default') = $2
		    AND i.status IN ('open', 'acknowledged')`,
		eventID, tenantID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := map[string]string{}
	for rows.Next() {
		var id, status string
		if err := rows.Scan(&id, &status); err != nil {
			return nil, err
		}
		out[id] = status
	}
	return out, rows.Err()
}
