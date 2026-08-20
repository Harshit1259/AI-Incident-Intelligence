package services

import (
	"fmt"
	"time"

	"ai-incident-platform/backend/internal/models"
	"ai-incident-platform/backend/internal/store"
)

type AnomalyService struct {
	store              *store.AnomalyStore
	incidentStore      *store.IncidentStore
	eventStore         *store.EventStore
	corrService        *CorrelationService
	predictiveService  *PredictiveIncidentService
}

func NewAnomalyService(as *store.AnomalyStore, is *store.IncidentStore, es *store.EventStore, cs *CorrelationService) *AnomalyService {
	return &AnomalyService{
		store:         as,
		incidentStore: is,
		eventStore:    es,
		corrService:   cs,
	}
}

// SetPredictiveService wires the predictive engine so every metric check
// also feeds the time-series buffer and triggers forward-looking analysis.
func (s *AnomalyService) SetPredictiveService(ps *PredictiveIncidentService) {
	s.predictiveService = ps
}

// CheckMetric evaluates a metric value against thresholds and trends.
// If anomalous, creates an alert and optionally fires a pre-failure event into correlation.
func (s *AnomalyService) CheckMetric(tenantID, service, metricName string, value, threshold float64, trend string) (*models.AnomalyAlert, error) {
	if value < threshold*0.85 {
		return nil, nil // not anomalous
	}

	severity := "warning"
	message := fmt.Sprintf("Metric %s on %s is approaching threshold: %.2f / %.2f", metricName, service, value, threshold)

	if value >= threshold {
		severity = "critical"
		message = fmt.Sprintf("Metric %s on %s has breached threshold: %.2f >= %.2f", metricName, service, value, threshold)
	}

	alert := models.AnomalyAlert{
		TenantID:     tenantID,
		Service:      service,
		MetricName:   metricName,
		CurrentValue: value,
		Threshold:    threshold,
		Trend:        trend,
		Severity:     severity,
		Message:      message,
		FiredAt:      time.Now(),
		Acknowledged: false,
	}

	if err := s.store.CreateAlert(alert); err != nil {
		return nil, fmt.Errorf("create anomaly alert: %w", err)
	}

	// Feed every metric value into the predictive time-series buffer.
	// The predictive engine evaluates whether the trend will breach threshold
	// before it actually fires — enabling forward warnings.
	if s.predictiveService != nil {
		_ = s.predictiveService.RecordMetric(tenantID, service, metricName, value)
		_, _ = s.predictiveService.EvaluatePrediction(tenantID, service, metricName, threshold, 0)
	}

	// If value >= threshold, also create an event and process through correlation
	if value >= threshold && s.corrService != nil {
		event := models.Event{
			ID:        fmt.Sprintf("anomaly-%d", time.Now().UnixNano()),
			TenantID:  tenantID,
			Source:    "anomaly_detection",
			Type:      "anomaly",
			Service:   service,
			Severity:  severity,
			Title:     fmt.Sprintf("Anomaly: %s breached on %s", metricName, service),
			Message:   message,
			Timestamp: time.Now(),
		}
		s.corrService.ProcessEvent(event)
	}

	return &alert, nil
}

// SimulateAnomaly creates a demo anomaly alert for testing.
func (s *AnomalyService) SimulateAnomaly(tenantID, service string) (*models.AnomalyAlert, error) {
	if service == "" {
		service = "api-gateway"
	}

	alert := models.AnomalyAlert{
		TenantID:     tenantID,
		Service:      service,
		MetricName:   "error_rate",
		CurrentValue: 15.5,
		Threshold:    10.0,
		Trend:        "rising",
		Severity:     "critical",
		Message:      fmt.Sprintf("Simulated anomaly: error_rate on %s spiked to 15.5%% (threshold: 10%%)", service),
		FiredAt:      time.Now(),
		Acknowledged: false,
	}

	if err := s.store.CreateAlert(alert); err != nil {
		return nil, fmt.Errorf("create simulated anomaly alert: %w", err)
	}

	return &alert, nil
}
