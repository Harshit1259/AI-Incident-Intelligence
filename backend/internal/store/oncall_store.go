package store

import (
	"context"
	"database/sql"
	"time"

	"ai-incident-platform/backend/internal/models"
)

type OnCallStore struct {
	db *sql.DB
}

func NewOnCallStore(db *sql.DB) *OnCallStore {
	return &OnCallStore{db: db}
}

func (s *OnCallStore) CreateSchedule(sched models.OnCallSchedule) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	_, err = tx.ExecContext(ctx, 
		`INSERT INTO oncall_schedules (id, tenant_id, team_name, timezone, rotation_type, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		sched.ID, sched.TenantID, sched.TeamName, sched.Timezone, sched.RotationType, sched.CreatedAt)
	if err != nil {
		return err
	}

	for _, m := range sched.Members {
		_, err = tx.ExecContext(ctx, 
			`INSERT INTO oncall_members (schedule_id, user_name, user_email, position)
			 VALUES ($1, $2, $3, $4)`,
			sched.ID, m.UserName, m.UserEmail, m.Position)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (s *OnCallStore) GetSchedules(tenantID string) ([]models.OnCallSchedule, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	rows, err := s.db.QueryContext(ctx, 
		`SELECT id, tenant_id, team_name, timezone, rotation_type, created_at
		 FROM oncall_schedules WHERE tenant_id = $1 ORDER BY created_at DESC LIMIT 100`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var schedules []models.OnCallSchedule
	for rows.Next() {
		var sched models.OnCallSchedule
		if err := rows.Scan(&sched.ID, &sched.TenantID, &sched.TeamName, &sched.Timezone, &sched.RotationType, &sched.CreatedAt); err != nil {
			return nil, err
		}
		members, err := s.getMembersForSchedule(sched.ID)
		if err != nil {
			return nil, err
		}
		sched.Members = members
		schedules = append(schedules, sched)
	}
	return schedules, rows.Err()
}

func (s *OnCallStore) GetScheduleByID(id string) (*models.OnCallSchedule, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var sched models.OnCallSchedule
	err := s.db.QueryRowContext(ctx, 
		`SELECT id, tenant_id, team_name, timezone, rotation_type, created_at
		 FROM oncall_schedules WHERE id = $1`, id).Scan(
		&sched.ID, &sched.TenantID, &sched.TeamName, &sched.Timezone, &sched.RotationType, &sched.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	members, err := s.getMembersForSchedule(id)
	if err != nil {
		return nil, err
	}
	sched.Members = members
	return &sched, nil
}

func (s *OnCallStore) DeleteSchedule(id string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	_, err = tx.ExecContext(ctx, `DELETE FROM oncall_overrides WHERE schedule_id = $1`, id)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `DELETE FROM oncall_members WHERE schedule_id = $1`, id)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `DELETE FROM oncall_schedules WHERE id = $1`, id)
	if err != nil {
		return err
	}
	return tx.Commit()
}

func (s *OnCallStore) AddOverride(o models.OnCallOverride) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err := s.db.ExecContext(ctx, 
		`INSERT INTO oncall_overrides (schedule_id, original_user, override_user, start_time, end_time, reason)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		o.ScheduleID, o.OriginalUser, o.OverrideUser, o.StartTime, o.EndTime, o.Reason)
	return err
}

func (s *OnCallStore) GetActiveOverrides(scheduleID string, at time.Time) ([]models.OnCallOverride, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	rows, err := s.db.QueryContext(ctx, 
		`SELECT id, schedule_id, original_user, override_user, start_time, end_time, reason
		 FROM oncall_overrides WHERE schedule_id = $1 AND start_time <= $2 AND end_time > $2
		 ORDER BY start_time DESC`, scheduleID, at)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []models.OnCallOverride
	for rows.Next() {
		var o models.OnCallOverride
		if err := rows.Scan(&o.ID, &o.ScheduleID, &o.OriginalUser, &o.OverrideUser, &o.StartTime, &o.EndTime, &o.Reason); err != nil {
			return nil, err
		}
		results = append(results, o)
	}
	return results, rows.Err()
}

func (s *OnCallStore) getMembersForSchedule(scheduleID string) ([]models.OnCallMember, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	rows, err := s.db.QueryContext(ctx, 
		`SELECT id, schedule_id, user_name, user_email, position
		 FROM oncall_members WHERE schedule_id = $1 ORDER BY position ASC`, scheduleID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var members []models.OnCallMember
	for rows.Next() {
		var m models.OnCallMember
		if err := rows.Scan(&m.ID, &m.ScheduleID, &m.UserName, &m.UserEmail, &m.Position); err != nil {
			return nil, err
		}
		members = append(members, m)
	}
	return members, rows.Err()
}
