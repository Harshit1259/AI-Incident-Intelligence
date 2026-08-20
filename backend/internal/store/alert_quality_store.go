package store

import (
	"context"
	"database/sql"
	"time"

	"ai-incident-platform/backend/internal/models"
)

// AlertQualityStore runs analytics queries for alert quality governance.
// All queries operate on the existing events and alert_feedback tables
// plus the new alert_rules table.
type AlertQualityStore struct {
	db *sql.DB
}

func NewAlertQualityStore(db *sql.DB) *AlertQualityStore {
	return &AlertQualityStore{db: db}
}

// UpsertRule registers or refreshes an alert rule by fingerprint.
func (s *AlertQualityStore) UpsertRule(tenantID string, r models.AlertRuleRegistration) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO alert_rules (id, tenant_id, name, source, service, team, severity, condition_expr, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,NOW())
		ON CONFLICT (id) DO UPDATE SET
			name           = EXCLUDED.name,
			source         = COALESCE(NULLIF(EXCLUDED.source,''), alert_rules.source),
			service        = COALESCE(NULLIF(EXCLUDED.service,''), alert_rules.service),
			team           = COALESCE(NULLIF(EXCLUDED.team,''), alert_rules.team),
			severity       = COALESCE(NULLIF(EXCLUDED.severity,''), alert_rules.severity),
			condition_expr = COALESCE(NULLIF(EXCLUDED.condition_expr,''), alert_rules.condition_expr),
			updated_at     = NOW()
	`, r.ID, tenantID, r.Name, r.Source, r.Service, r.Team, r.Severity, r.ConditionExpr)
	return err
}

// GetRule fetches a single rule by fingerprint.
func (s *AlertQualityStore) GetRule(tenantID, id string) (*models.AlertRule, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	row := s.db.QueryRowContext(ctx, `
		SELECT id, tenant_id, name, source, service, team, severity, condition_expr,
		       first_seen_at, last_fired_at, fire_count_7d, fire_count_30d,
		       fp_count, noise_count, is_active, updated_at
		FROM alert_rules
		WHERE id=$1 AND tenant_id=$2`, id, tenantID)
	return scanAlertRule(row.Scan)
}

func scanAlertRule(scan func(...any) error) (*models.AlertRule, error) {
	var r models.AlertRule
	var lastFired sql.NullTime
	if err := scan(
		&r.ID, &r.TenantID, &r.Name, &r.Source, &r.Service, &r.Team,
		&r.Severity, &r.ConditionExpr, &r.FirstSeenAt, &lastFired,
		&r.FireCount7d, &r.FireCount30d, &r.FPCount, &r.NoiseCount,
		&r.IsActive, &r.UpdatedAt,
	); err != nil {
		return nil, err
	}
	if lastFired.Valid {
		r.LastFiredAt = &lastFired.Time
	}
	return &r, nil
}

// -------------------------------------------------------------------
// Analytics queries
// -------------------------------------------------------------------

// NoisyAlertRawRow is an intermediate query result before scoring.
type NoisyAlertRawRow struct {
	Fingerprint string
	Title       string
	Source      string
	Service     string
	FireCount   int
	FPCount     int
	NoiseCount  int
	UsefulCount int
}

// QueryNoisyAlerts returns alerts that fired frequently in the last windowDays.
// Threshold: ≥ 5 fires in the window. The caller scores and classifies them.
func (s *AlertQualityStore) QueryNoisyAlerts(tenantID string, windowDays int) ([]NoisyAlertRawRow, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	since := time.Now().AddDate(0, 0, -windowDays)
	rows, err := s.db.QueryContext(ctx, `
		SELECT
			e.fingerprint,
			MAX(e.title)                                               AS title,
			MAX(e.source)                                              AS source,
			MAX(e.service)                                             AS service,
			COUNT(*)                                                   AS fire_count,
			COUNT(af.id) FILTER (WHERE af.feedback = 'false_positive') AS fp_count,
			COUNT(af.id) FILTER (WHERE af.feedback = 'noise')          AS noise_count,
			COUNT(af.id) FILTER (WHERE af.feedback = 'useful')         AS useful_count
		FROM events e
		LEFT JOIN alert_feedback af
			ON af.fingerprint = e.fingerprint AND af.tenant_id = e.tenant_id
		WHERE e.tenant_id = $1
		  AND e.fingerprint != ''
		  AND e.timestamp >= $2
		GROUP BY e.fingerprint
		HAVING COUNT(*) >= 5
		ORDER BY fire_count DESC
		LIMIT 50
	`, tenantID, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []NoisyAlertRawRow
	for rows.Next() {
		var r NoisyAlertRawRow
		if err := rows.Scan(&r.Fingerprint, &r.Title, &r.Source, &r.Service,
			&r.FireCount, &r.FPCount, &r.NoiseCount, &r.UsefulCount); err != nil {
			return nil, err
		}
		result = append(result, r)
	}
	return result, rows.Err()
}

// DuplicatePairRaw is a raw query result for co-firing pairs.
type DuplicatePairRaw struct {
	FP1, Title1, Service1 string
	FP2, Title2, Service2 string
	CoCount               int
}

// QueryDuplicatePairs finds alert fingerprint pairs that co-fire within 5 minutes.
func (s *AlertQualityStore) QueryDuplicatePairs(tenantID string, windowDays int) ([]DuplicatePairRaw, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	since := time.Now().AddDate(0, 0, -windowDays)
	rows, err := s.db.QueryContext(ctx, `
		SELECT
			a.fingerprint                                  AS fp1,
			MAX(a.title)                                   AS t1,
			MAX(a.service)                                 AS s1,
			b.fingerprint                                  AS fp2,
			MAX(b.title)                                   AS t2,
			MAX(b.service)                                 AS s2,
			COUNT(*)                                       AS co_count
		FROM events a
		JOIN events b
			ON  a.tenant_id  = b.tenant_id
			AND a.fingerprint < b.fingerprint
			AND ABS(EXTRACT(EPOCH FROM (a.timestamp - b.timestamp))) <= 300
			AND b.timestamp >= $2
		WHERE a.tenant_id = $1
		  AND a.fingerprint != ''
		  AND b.fingerprint != ''
		  AND a.timestamp >= $2
		GROUP BY a.fingerprint, b.fingerprint
		HAVING COUNT(*) >= 3
		ORDER BY co_count DESC
		LIMIT 30
	`, tenantID, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []DuplicatePairRaw
	for rows.Next() {
		var r DuplicatePairRaw
		if err := rows.Scan(&r.FP1, &r.Title1, &r.Service1,
			&r.FP2, &r.Title2, &r.Service2, &r.CoCount); err != nil {
			return nil, err
		}
		result = append(result, r)
	}
	return result, rows.Err()
}

// StaleRuleRaw is a raw query result for stale alert detection.
type StaleRuleRaw struct {
	Fingerprint string
	Title       string
	Source      string
	Service     string
	LastSeen    time.Time
	TotalCount  int
}

// QueryStaleRules returns fingerprints that last fired more than staleDays ago
// but have fired at least once before that (indicating a known rule going quiet).
func (s *AlertQualityStore) QueryStaleRules(tenantID string, staleDays int) ([]StaleRuleRaw, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cutoff := time.Now().AddDate(0, 0, -staleDays)
	rows, err := s.db.QueryContext(ctx, `
		SELECT
			e.fingerprint,
			MAX(e.title)      AS title,
			MAX(e.source)     AS source,
			MAX(e.service)    AS service,
			MAX(e.timestamp)  AS last_seen,
			COUNT(*)          AS total_count
		FROM events e
		WHERE e.tenant_id = $1
		  AND e.fingerprint != ''
		GROUP BY e.fingerprint
		HAVING MAX(e.timestamp) < $2
		   AND COUNT(*) >= 1
		ORDER BY last_seen ASC
		LIMIT 40
	`, tenantID, cutoff)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []StaleRuleRaw
	for rows.Next() {
		var r StaleRuleRaw
		if err := rows.Scan(&r.Fingerprint, &r.Title, &r.Source, &r.Service,
			&r.LastSeen, &r.TotalCount); err != nil {
			return nil, err
		}
		result = append(result, r)
	}
	return result, rows.Err()
}

// AlertDebtRaw is a raw row for team/service debt aggregation.
type AlertDebtRaw struct {
	Service            string
	TotalFires         int
	UniqueRules        int
	FPFeedbackCount    int
	NoiseFeedbackCount int
}

// QueryAlertDebtByService aggregates quality metrics grouped by service (used as team proxy).
func (s *AlertQualityStore) QueryAlertDebtByService(tenantID string, windowDays int) ([]AlertDebtRaw, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	since := time.Now().AddDate(0, 0, -windowDays)
	rows, err := s.db.QueryContext(ctx, `
		SELECT
			COALESCE(NULLIF(e.service,''), 'unknown')                  AS service,
			COUNT(*)                                                   AS total_fires,
			COUNT(DISTINCT e.fingerprint)                              AS unique_rules,
			COUNT(af.id) FILTER (WHERE af.feedback = 'false_positive') AS fp_count,
			COUNT(af.id) FILTER (WHERE af.feedback = 'noise')          AS noise_count
		FROM events e
		LEFT JOIN alert_feedback af
			ON af.fingerprint = e.fingerprint AND af.tenant_id = e.tenant_id
		WHERE e.tenant_id = $1
		  AND e.timestamp >= $2
		GROUP BY COALESCE(NULLIF(e.service,''), 'unknown')
		ORDER BY total_fires DESC
		LIMIT 20
	`, tenantID, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []AlertDebtRaw
	for rows.Next() {
		var r AlertDebtRaw
		if err := rows.Scan(&r.Service, &r.TotalFires, &r.UniqueRules,
			&r.FPFeedbackCount, &r.NoiseFeedbackCount); err != nil {
			return nil, err
		}
		result = append(result, r)
	}
	return result, rows.Err()
}

// CountTotalFiredInWindow returns the total alert fire count for a tenant in the window.
func (s *AlertQualityStore) CountTotalFiredInWindow(tenantID string, windowDays int) (int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	since := time.Now().AddDate(0, 0, -windowDays)
	var count int
	err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM events
		WHERE tenant_id=$1 AND timestamp>=$2 AND fingerprint!=''
	`, tenantID, since).Scan(&count)
	return count, err
}

// CountUniqueRulesInWindow returns distinct fingerprints seen in the window.
func (s *AlertQualityStore) CountUniqueRulesInWindow(tenantID string, windowDays int) (int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	since := time.Now().AddDate(0, 0, -windowDays)
	var count int
	err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(DISTINCT fingerprint) FROM events
		WHERE tenant_id=$1 AND timestamp>=$2 AND fingerprint!=''
	`, tenantID, since).Scan(&count)
	return count, err
}

// FingerprintFireCount returns how many times a fingerprint fired in the given window.
func (s *AlertQualityStore) FingerprintFireCount(tenantID, fingerprint string, windowDays int) (int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	since := time.Now().AddDate(0, 0, -windowDays)
	var count int
	err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM events
		WHERE tenant_id=$1 AND fingerprint=$2 AND timestamp>=$3
	`, tenantID, fingerprint, since).Scan(&count)
	return count, err
}
