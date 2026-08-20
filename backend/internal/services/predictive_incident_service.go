package services

import (
	"fmt"
	"math"
	"time"

	"ai-incident-platform/backend/internal/models"
	"ai-incident-platform/backend/internal/store"
)

const (
	defaultWindowMinutes  = 30  // predict breach within 30 min → create incident
	minDataPoints         = 6   // need at least 6 points for meaningful regression
	maxSeriesLookback     = 90  // minutes of history to fetch
	maxSeriesPoints       = 120 // max data points per query
	minBreachProbability  = 40.0
)

type PredictiveIncidentService struct {
	store       *store.PredictiveIncidentStore
	corrService *CorrelationService
}

func NewPredictiveIncidentService(s *store.PredictiveIncidentStore, cs *CorrelationService) *PredictiveIncidentService {
	return &PredictiveIncidentService{store: s, corrService: cs}
}

// RecordMetric persists a new data point for trend tracking.
func (s *PredictiveIncidentService) RecordMetric(tenantID, service, metricName string, value float64) error {
	return s.store.InsertMetricPoint(tenantID, service, metricName, value)
}

// EvaluatePrediction runs trend analysis on recent data and creates a predictive incident
// if the metric is on track to breach the given threshold within windowMinutes.
// Returns nil, nil when there is not enough data or the trend is not alarming.
func (s *PredictiveIncidentService) EvaluatePrediction(tenantID, service, metricName string, threshold float64, windowMinutes int) (*models.PredictiveIncident, error) {
	if windowMinutes <= 0 {
		windowMinutes = defaultWindowMinutes
	}

	since := time.Now().Add(-time.Duration(maxSeriesLookback) * time.Minute)
	pts, err := s.store.GetMetricSeries(tenantID, service, metricName, since, maxSeriesPoints)
	if err != nil {
		return nil, fmt.Errorf("get metric series: %w", err)
	}
	if len(pts) < minDataPoints {
		return nil, nil // not enough history
	}

	analysis := computeWeightedLinearRegression(pts)

	// We only predict when the metric is rising toward threshold.
	// Negative or flat slope → no breach risk.
	if analysis.Slope <= 0 {
		return nil, nil
	}

	lastValue := pts[len(pts)-1].Value
	minutesToBreach := (threshold - lastValue) / analysis.Slope

	// Already breached or too far in the future to be actionable
	if minutesToBreach < 0 || minutesToBreach > float64(windowMinutes*3) {
		return nil, nil
	}

	// 95% confidence interval: ±1.96 SE, converted to time uncertainty
	seMinutes := 0.0
	if analysis.Slope > 0 {
		seMinutes = analysis.StdError / analysis.Slope
	}
	halfWidth := 1.96 * seMinutes
	breachMin := int(math.Max(1, minutesToBreach-halfWidth))
	breachMax := int(minutesToBreach + halfWidth)
	if breachMax < breachMin {
		breachMax = breachMin + 5
	}

	// Breach probability combines R² (trend reliability) with proximity to window.
	// R²=1.0 and at edge of window → 99%. R²=0.5 and far out → low.
	proximity := 1.0 - (minutesToBreach / float64(windowMinutes*3))
	if proximity < 0 {
		proximity = 0
	}
	probability := analysis.RSquared * proximity * 100
	if probability > 99 {
		probability = 99
	}
	if probability < minBreachProbability {
		return nil, nil // not confident enough to raise alarm
	}

	// Dedup: don't create a second open prediction for same service+metric.
	existing, err := s.store.GetOpen(tenantID, service, metricName)
	if err != nil {
		return nil, fmt.Errorf("check existing prediction: %w", err)
	}
	if existing != nil {
		return existing, nil
	}

	msg := fmt.Sprintf("%.0f%% chance of SLO breach in %d–%d minutes (trend: +%.3f/min, R²=%.2f)",
		probability, breachMin, breachMax, analysis.Slope, analysis.RSquared)

	pred := &models.PredictiveIncident{
		ID:                 generateID(),
		TenantID:           tenantID,
		Service:            service,
		MetricName:         metricName,
		CurrentValue:       lastValue,
		SLOThreshold:       threshold,
		TrendSlope:         analysis.Slope,
		BreachProbability:  probability,
		ConfidenceInterval: 95,
		PredictedBreachMin: breachMin,
		PredictedBreachMax: breachMax,
		Status:             "open",
		Message:            msg,
		DataPointsUsed:     analysis.DataPoints,
		CreatedAt:          time.Now(),
	}

	if err := s.store.Create(pred); err != nil {
		return nil, fmt.Errorf("create predictive incident: %w", err)
	}

	// Feed into correlation pipeline so it appears as an incident event.
	if s.corrService != nil {
		event := models.Event{
			ID:        fmt.Sprintf("predict-%s", pred.ID),
			TenantID:  tenantID,
			Source:    "predictive_engine",
			Type:      "predictive_incident",
			Service:   service,
			Severity:  "warning",
			Title:     fmt.Sprintf("Predictive: %s may breach SLO in %d–%d min", metricName, breachMin, breachMax),
			Message:   msg,
			Timestamp: time.Now(),
		}
		s.corrService.ProcessEvent(event)
	}

	return pred, nil
}

// Simulate creates a synthetic predictive incident for demo/testing purposes.
func (s *PredictiveIncidentService) Simulate(tenantID, service string) (*models.PredictiveIncident, error) {
	if service == "" {
		service = "api-gateway"
	}

	// Seed 12 synthetic rising data points every 5 min over the last 55 min.
	now := time.Now()
	for i := 0; i < 12; i++ {
		t := now.Add(time.Duration(-55+i*5) * time.Minute)
		value := 5.0 + float64(i)*0.7 + float64(i)*float64(i)*0.03
		_ = s.store.InsertMetricPointAt(tenantID, service, "error_rate", value, t)
	}

	return s.EvaluatePrediction(tenantID, service, "error_rate", 10.0, defaultWindowMinutes)
}

// ListPredictions returns predictive incidents for a tenant.
func (s *PredictiveIncidentService) ListPredictions(tenantID, statusFilter string, limit int) ([]models.PredictiveIncident, error) {
	return s.store.List(tenantID, statusFilter, limit)
}

// Resolve marks a prediction as resolved or false_alarm.
func (s *PredictiveIncidentService) Resolve(id, resolution string) error {
	if resolution != "resolved" && resolution != "false_alarm" {
		return fmt.Errorf("resolution must be 'resolved' or 'false_alarm'")
	}
	return s.store.Resolve(id, resolution)
}

// PurgeOldMetrics removes data points older than 2 hours (called by background worker).
func (s *PredictiveIncidentService) PurgeOldMetrics() error {
	return s.store.PurgeOldMetrics(time.Now().Add(-2 * time.Hour))
}

// computeWeightedLinearRegression performs exponentially-weighted least-squares regression.
// Recent points are weighted 2× more than the oldest point.
// Returns slope (change per minute), intercept, R², standard error.
func computeWeightedLinearRegression(pts []models.MetricDataPoint) models.TrendAnalysis {
	n := len(pts)
	if n < 2 {
		return models.TrendAnalysis{}
	}

	// Express x as minutes since the first point so numbers stay small.
	origin := pts[0].RecordedAt
	xs := make([]float64, n)
	for i, p := range pts {
		xs[i] = p.RecordedAt.Sub(origin).Minutes()
	}

	// Exponential weights: w_i = e^(i/n) so newest point gets e≈2.7× the oldest.
	ws := make([]float64, n)
	for i := range ws {
		ws[i] = math.Exp(float64(i) / float64(n))
	}

	// Weighted sums
	var sw, swx, swy, swxx, swxy float64
	for i := 0; i < n; i++ {
		w := ws[i]
		x := xs[i]
		y := pts[i].Value
		sw += w
		swx += w * x
		swy += w * y
		swxx += w * x * x
		swxy += w * x * y
	}

	denom := sw*swxx - swx*swx
	if math.Abs(denom) < 1e-12 {
		return models.TrendAnalysis{DataPoints: n}
	}

	slope := (sw*swxy - swx*swy) / denom
	intercept := (swy - slope*swx) / sw

	// Standard error of the regression (unweighted residuals for simplicity).
	var ssRes, ssTot float64
	yMean := swy / sw
	for i := 0; i < n; i++ {
		predicted := intercept + slope*xs[i]
		r := pts[i].Value - predicted
		ssRes += r * r
		d := pts[i].Value - yMean
		ssTot += d * d
	}

	rSquared := 0.0
	if ssTot > 0 {
		rSquared = 1.0 - ssRes/ssTot
		if rSquared < 0 {
			rSquared = 0
		}
	}

	stdError := 0.0
	if n > 2 {
		stdError = math.Sqrt(ssRes / float64(n-2))
	}

	return models.TrendAnalysis{
		Slope:      slope,
		Intercept:  intercept,
		RSquared:   rSquared,
		StdError:   stdError,
		DataPoints: n,
	}
}
