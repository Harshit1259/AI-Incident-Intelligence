package store

import (
	"context"
	"time"
	"database/sql"
	"encoding/json"

	"ai-incident-platform/backend/internal/models"
)

// OnboardingStore handles onboarding_progress persistence.
type OnboardingStore struct {
	db *sql.DB
}

func NewOnboardingStore(db *sql.DB) *OnboardingStore {
	return &OnboardingStore{db: db}
}

// GetProgress returns the onboarding progress for a tenant. Returns nil if none exists.
func (s *OnboardingStore) GetProgress(tenantID string) (*models.OnboardingProgress, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	row := s.db.QueryRowContext(ctx, 
		`SELECT id, tenant_id, step, completed_steps, first_source_connected,
		        first_incident_created, first_rca_generated, aha_moment_reached,
		        created_at, updated_at
		 FROM onboarding_progress WHERE tenant_id = $1`, tenantID,
	)

	var p models.OnboardingProgress
	var completedStepsJSON string
	err := row.Scan(
		&p.ID, &p.TenantID, &p.Step, &completedStepsJSON,
		&p.FirstSourceConnected, &p.FirstIncidentCreated,
		&p.FirstRCAGenerated, &p.AhaMomentReached,
		&p.CreatedAt, &p.UpdatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	if err := json.Unmarshal([]byte(completedStepsJSON), &p.CompletedSteps); err != nil {
		p.CompletedSteps = []string{}
	}

	return &p, nil
}

// UpsertProgress creates or updates onboarding progress.
func (s *OnboardingStore) UpsertProgress(p models.OnboardingProgress) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	completedJSON, err := json.Marshal(p.CompletedSteps)
	if err != nil {
		return err
	}
	if string(completedJSON) == "null" {
		completedJSON = []byte("[]")
	}

	_, err = s.db.ExecContext(ctx, 
		`INSERT INTO onboarding_progress (id, tenant_id, step, completed_steps,
		    first_source_connected, first_incident_created, first_rca_generated,
		    aha_moment_reached, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		 ON CONFLICT (tenant_id) DO UPDATE SET
		   step = EXCLUDED.step,
		   completed_steps = EXCLUDED.completed_steps,
		   first_source_connected = EXCLUDED.first_source_connected,
		   first_incident_created = EXCLUDED.first_incident_created,
		   first_rca_generated = EXCLUDED.first_rca_generated,
		   aha_moment_reached = EXCLUDED.aha_moment_reached,
		   updated_at = EXCLUDED.updated_at`,
		p.ID, p.TenantID, p.Step, string(completedJSON),
		p.FirstSourceConnected, p.FirstIncidentCreated, p.FirstRCAGenerated,
		p.AhaMomentReached, p.CreatedAt, p.UpdatedAt,
	)
	return err
}
