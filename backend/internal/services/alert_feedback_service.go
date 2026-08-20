package services

import (
	"fmt"
	"log/slog"
	"time"

	"ai-incident-platform/backend/internal/models"
	"ai-incident-platform/backend/internal/store"
)

type AlertFeedbackService struct {
	store      *store.AlertFeedbackStore
	eventStore *store.EventStore
}

func NewAlertFeedbackService(s *store.AlertFeedbackStore, es *store.EventStore) *AlertFeedbackService {
	return &AlertFeedbackService{store: s, eventStore: es}
}

func (s *AlertFeedbackService) SubmitFeedback(req models.AlertFeedbackRequest, tenantID, userID string) error {
	// Look up event to get fingerprint
	fingerprint := ""
	if req.EventID != "" {
		events, err := s.eventStore.GetEventsByIDs([]string{req.EventID})
		if err == nil && len(events) > 0 {
			fingerprint = events[0].Fingerprint
		}
	}

	feedback := models.AlertFeedback{
		EventID:     req.EventID,
		IncidentID:  req.IncidentID,
		TenantID:    tenantID,
		Feedback:    req.Feedback,
		Fingerprint: fingerprint,
		Reason:      req.Reason,
		CreatedBy:   userID,
		CreatedAt:   time.Now(),
	}

	if err := s.store.Create(feedback); err != nil {
		return fmt.Errorf("save feedback: %w", err)
	}

	slog.Info("alert_feedback: feedback submitted", "feedback", req.Feedback, "event_id", req.EventID, "fingerprint", fingerprint, "tenant_id", tenantID)
	return nil
}

func (s *AlertFeedbackService) GetStats(tenantID string) (*models.AlertFeedbackStats, error) {
	allFeedback, err := s.store.GetByTenant(tenantID, 10000)
	if err != nil {
		return nil, err
	}

	stats := &models.AlertFeedbackStats{
		TotalFeedback: len(allFeedback),
	}

	// Count by type
	suppressedFingerprints := make(map[string]int)
	for _, f := range allFeedback {
		switch f.Feedback {
		case "useful":
			stats.UsefulCount++
		case "noise":
			stats.NoiseCount++
			suppressedFingerprints[f.Fingerprint]++
		}
	}

	// Count suppressed (fingerprints with >= 5 noise)
	for _, count := range suppressedFingerprints {
		if count >= 5 {
			stats.SuppressedCount++
		}
	}

	// Top noisy fingerprints
	topNoisy, err := s.store.CountNoisyFingerprints(tenantID)
	if err != nil {
		slog.Error("alert_feedback: error getting noisy fingerprints", "error", err)
		topNoisy = []models.NoisyAlert{}
	}
	stats.TopNoisy = topNoisy

	return stats, nil
}

func (s *AlertFeedbackService) IsSuppressed(fingerprint string) bool {
	return s.store.IsSuppressed(fingerprint)
}

// GetSourceQuality returns per-integration-source quality statistics derived
// from the alert feedback store. Noise ratio and quality score let operators
// quickly identify low-quality integrations for tuning or suppression.
func (s *AlertFeedbackService) GetSourceQuality(tenantID string) ([]models.SourceQualityStat, error) {
	rows, err := s.store.GetSourceFeedbackCounts(tenantID)
	if err != nil {
		return nil, err
	}
	return rows, nil
}
