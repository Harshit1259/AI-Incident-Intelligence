package services

import (
	"fmt"
	"log/slog"
	"time"

	"ai-incident-platform/backend/internal/models"
	"ai-incident-platform/backend/internal/store"
)

// OnboardingService manages tenant onboarding progress.
type OnboardingService struct {
	store          *store.OnboardingStore
	billingService *BillingService
}

func NewOnboardingService(s *store.OnboardingStore, bs *BillingService) *OnboardingService {
	return &OnboardingService{
		store:          s,
		billingService: bs,
	}
}

// GetProgress returns the current onboarding state for a tenant.
// If no progress exists, it creates a default one.
func (s *OnboardingService) GetProgress(tenantID string) (*models.OnboardingProgress, error) {
	p, err := s.store.GetProgress(tenantID)
	if err != nil {
		return nil, err
	}
	if p != nil {
		return p, nil
	}

	// Create default progress
	now := time.Now()
	p = &models.OnboardingProgress{
		ID:             fmt.Sprintf("onb_%s_%d", tenantID, now.UnixNano()),
		TenantID:       tenantID,
		Step:           "signup",
		CompletedSteps: []string{},
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if err := s.store.UpsertProgress(*p); err != nil {
		return nil, err
	}
	return p, nil
}

// AdvanceStep moves the onboarding to the next step.
func (s *OnboardingService) AdvanceStep(tenantID, step string) error {
	p, err := s.GetProgress(tenantID)
	if err != nil {
		return err
	}

	// Add step to completed_steps if not already there
	found := false
	for _, cs := range p.CompletedSteps {
		if cs == step {
			found = true
			break
		}
	}
	if !found {
		p.CompletedSteps = append(p.CompletedSteps, step)
	}

	p.Step = step
	p.UpdatedAt = time.Now()

	if step == "complete" {
		p.AhaMomentReached = true
	}

	return s.store.UpsertProgress(*p)
}

// MarkMilestone records that a milestone was reached.
func (s *OnboardingService) MarkMilestone(tenantID, milestone string) error {
	p, err := s.GetProgress(tenantID)
	if err != nil {
		slog.Error("onboarding: failed to get progress for tenant", "tenant_id", tenantID, "error", err)
		return err
	}

	switch milestone {
	case "source_connected":
		p.FirstSourceConnected = true
	case "incident_created":
		p.FirstIncidentCreated = true
	case "rca_generated":
		p.FirstRCAGenerated = true
		p.AhaMomentReached = true
	default:
		return nil // unknown milestone, ignore
	}

	p.UpdatedAt = time.Now()
	return s.store.UpsertProgress(*p)
}
