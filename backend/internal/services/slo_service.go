package services

import (
	"fmt"
	"time"

	"ai-incident-platform/backend/internal/models"
	"ai-incident-platform/backend/internal/store"
)

type SLOService struct {
	store *store.SLOStore
}

func NewSLOService(s *store.SLOStore) *SLOService {
	return &SLOService{store: s}
}

// ComputeStatus returns the current status for a single SLO definition.
func (s *SLOService) ComputeStatus(def models.SLODefinition) models.SLOStatus {
	latest, _ := s.store.GetLatestMeasurement(def.ID)

	status := models.SLOStatus{
		Definition:           def,
		CurrentPercent:       100.0,
		ErrorBudgetRemaining: 100.0,
		BurnRate:             0,
		TimeToBreachHours:    -1,
		Status:               "healthy",
	}

	if latest == nil {
		return status
	}

	// Current percent
	if latest.TotalRequests > 0 {
		status.CurrentPercent = float64(latest.GoodRequests) / float64(latest.TotalRequests) * 100
	}

	// Error budget calculation
	errorBudgetTotalMinutes := (100 - def.TargetPercent) * float64(def.WindowDays) * 24 * 60 / 100
	if errorBudgetTotalMinutes <= 0 {
		errorBudgetTotalMinutes = 1 // avoid division by zero
	}

	errorBudgetUsed := float64(latest.BadMinutes)
	remaining := (errorBudgetTotalMinutes - errorBudgetUsed) / errorBudgetTotalMinutes * 100
	if remaining < 0 {
		remaining = 0
	}
	status.ErrorBudgetRemaining = remaining

	// Burn rate: how fast budget is burning relative to linear expectation
	elapsedDays := time.Since(def.CreatedAt).Hours() / 24
	if elapsedDays < 1 {
		elapsedDays = 1
	}
	if elapsedDays > float64(def.WindowDays) {
		elapsedDays = float64(def.WindowDays)
	}

	expectedBudgetUsed := elapsedDays * (100 - def.TargetPercent) * 24 * 60 / 100 / float64(def.WindowDays)
	if expectedBudgetUsed > 0 {
		status.BurnRate = errorBudgetUsed / expectedBudgetUsed
	}

	// Time to breach
	if status.BurnRate > 1 && errorBudgetTotalMinutes > errorBudgetUsed {
		budgetPerMinuteRate := errorBudgetUsed / (elapsedDays * 24 * 60)
		if budgetPerMinuteRate > 0 {
			remainingMinutes := errorBudgetTotalMinutes - errorBudgetUsed
			status.TimeToBreachHours = remainingMinutes / budgetPerMinuteRate / 60
		}
	}

	// Status classification
	switch {
	case remaining <= 0:
		status.Status = "breached"
	case remaining < 10:
		status.Status = "critical"
	case remaining < 25:
		status.Status = "warning"
	default:
		status.Status = "healthy"
	}

	return status
}

// GetAllStatuses returns status for all SLOs in a tenant.
func (s *SLOService) GetAllStatuses(tenantID string) ([]models.SLOStatus, error) {
	defs, err := s.store.GetSLOs(tenantID)
	if err != nil {
		return nil, err
	}

	statuses := make([]models.SLOStatus, 0, len(defs))
	for _, def := range defs {
		st := s.ComputeStatus(def)
		statuses = append(statuses, st)
	}
	return statuses, nil
}

// RecordMeasurement records a data point for an SLO.
func (s *SLOService) RecordMeasurement(sloID string, total, good int64, badMinutes int) error {
	def, err := s.store.GetSLOByID(sloID)
	if err != nil {
		return err
	}
	if def == nil {
		return fmt.Errorf("slo not found: %s", sloID)
	}

	// Compute error budget remaining
	errorBudgetTotalMinutes := (100 - def.TargetPercent) * float64(def.WindowDays) * 24 * 60 / 100
	if errorBudgetTotalMinutes <= 0 {
		errorBudgetTotalMinutes = 1
	}

	remaining := (errorBudgetTotalMinutes - float64(badMinutes)) / errorBudgetTotalMinutes * 100
	if remaining < 0 {
		remaining = 0
	}

	m := models.SLOMeasurement{
		SLOID:                sloID,
		Timestamp:            time.Now(),
		TotalRequests:        total,
		GoodRequests:         good,
		BadMinutes:           badMinutes,
		ErrorBudgetRemaining: remaining,
	}

	return s.store.AddMeasurement(m)
}
