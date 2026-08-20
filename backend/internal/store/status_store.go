package store

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"math"
	"time"

	"ai-incident-platform/backend/internal/models"
)

// StatusStore runs read-only (and subscription write) queries for the public status page.
// It queries the incidents table directly for minimal overhead on an unauthenticated endpoint.
type StatusStore struct {
	db *sql.DB
}

func NewStatusStore(db *sql.DB) *StatusStore {
	return &StatusStore{db: db}
}

// GetActiveIncidents returns all open/acknowledged incidents for a tenant.
func (s *StatusStore) GetActiveIncidents(tenantID string) ([]models.StatusActiveIncident, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	rows, err := s.db.QueryContext(ctx, `
		SELECT id, COALESCE(title,''), COALESCE(service,''), COALESCE(severity,''),
		       status, first_event_time
		FROM   incidents
		WHERE  tenant_id = $1 AND status IN ('open','acknowledged')
		ORDER  BY first_event_time DESC
		LIMIT  50
	`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []models.StatusActiveIncident
	for rows.Next() {
		var inc models.StatusActiveIncident
		if err := rows.Scan(&inc.ID, &inc.Title, &inc.Service, &inc.Severity,
			&inc.Status, &inc.StartedAt); err != nil {
			return nil, err
		}
		out = append(out, inc)
	}
	return out, rows.Err()
}

// GetResolvedHistory returns the last `limit` resolved incidents with duration.
func (s *StatusStore) GetResolvedHistory(tenantID string, limit int) ([]models.StatusHistoryEntry, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	rows, err := s.db.QueryContext(ctx, `
		SELECT id, COALESCE(title,''), COALESCE(service,''), COALESCE(severity,''),
		       first_event_time, last_event_time,
		       GREATEST(0, EXTRACT(EPOCH FROM (last_event_time - first_event_time))::bigint / 60)
		FROM   incidents
		WHERE  tenant_id = $1 AND status = 'resolved'
		ORDER  BY last_event_time DESC
		LIMIT  $2
	`, tenantID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []models.StatusHistoryEntry
	for rows.Next() {
		var e models.StatusHistoryEntry
		if err := rows.Scan(&e.ID, &e.Title, &e.Service, &e.Severity,
			&e.StartedAt, &e.ResolvedAt, &e.DurationMin); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// GetDistinctServices returns all service names that had incidents in the last 90 days.
func (s *StatusStore) GetDistinctServices(tenantID string) ([]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	rows, err := s.db.QueryContext(ctx, `
		SELECT DISTINCT COALESCE(service,'unknown')
		FROM   incidents
		WHERE  tenant_id = $1
		  AND  first_event_time > NOW() - INTERVAL '90 days'
		ORDER  BY 1
	`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var svcs []string
	for rows.Next() {
		var svc string
		if err := rows.Scan(&svc); err != nil {
			return nil, err
		}
		svcs = append(svcs, svc)
	}
	return svcs, rows.Err()
}

// GetUptimePerService computes 90-day uptime % per service.
// Downtime = sum of incident durations (open incidents count from start → now).
func (s *StatusStore) GetUptimePerService(tenantID string) (map[string]float64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	const totalSec = float64(90 * 24 * 3600)
	cutoff := time.Now().AddDate(0, 0, -90)

	rows, err := s.db.QueryContext(ctx, `
		SELECT COALESCE(service,'unknown'),
		       COALESCE(SUM(
		           GREATEST(0, EXTRACT(EPOCH FROM (
		               LEAST(
		                   CASE WHEN status = 'resolved' THEN last_event_time ELSE NOW() END,
		                   NOW()
		               ) - GREATEST(first_event_time, $2)
		           ))::bigint)
		       ), 0) AS downtime_sec
		FROM   incidents
		WHERE  tenant_id = $1
		  AND  first_event_time > $2
		GROUP  BY service
	`, tenantID, cutoff)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	stats := map[string]float64{}
	for rows.Next() {
		var svc string
		var down float64
		if err := rows.Scan(&svc, &down); err != nil {
			return nil, err
		}
		uptime := (totalSec - down) / totalSec * 100
		uptime = math.Max(0, math.Min(100, uptime))
		stats[svc] = math.Round(uptime*100) / 100 // 2 decimal places
	}
	return stats, rows.Err()
}

// GetMTTRMinutes returns the average resolution time in minutes over the last 30 days.
func (s *StatusStore) GetMTTRMinutes(tenantID string) (int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var mttr sql.NullFloat64
	err := s.db.QueryRowContext(ctx, `
		SELECT AVG(EXTRACT(EPOCH FROM (last_event_time - first_event_time))::bigint / 60)
		FROM   incidents
		WHERE  tenant_id = $1
		  AND  status = 'resolved'
		  AND  last_event_time > NOW() - INTERVAL '30 days'
	`, tenantID).Scan(&mttr)
	if err != nil {
		return 0, err
	}
	if !mttr.Valid {
		return 0, nil
	}
	return int(math.Round(mttr.Float64)), nil
}

// CountResolvedLast30 returns the number of incidents resolved in the last 30 days.
func (s *StatusStore) CountResolvedLast30(tenantID string) (int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var count int
	err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM   incidents
		WHERE  tenant_id = $1
		  AND  status = 'resolved'
		  AND  last_event_time > NOW() - INTERVAL '30 days'
	`, tenantID).Scan(&count)
	return count, err
}

// Subscribe upserts a subscription and returns the unsubscribe token.
func (s *StatusStore) Subscribe(tenantID, channel, target string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	token, err := randomStatusToken()
	if err != nil {
		return "", err
	}

	_, err = s.db.ExecContext(ctx, `
		INSERT INTO status_subscriptions (tenant_id, channel, target, token)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (tenant_id, channel, target)
		DO UPDATE SET token = EXCLUDED.token
	`, tenantID, channel, target, token)
	if err != nil {
		return "", err
	}
	return token, nil
}

// Unsubscribe deletes a subscription by its unique token.
func (s *StatusStore) Unsubscribe(token string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err := s.db.ExecContext(ctx, `DELETE FROM status_subscriptions WHERE token = $1`, token)
	return err
}

func randomStatusToken() (string, error) {
	b := make([]byte, 20)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
