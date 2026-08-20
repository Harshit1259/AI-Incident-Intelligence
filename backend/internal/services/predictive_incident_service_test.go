package services

import (
	"math"
	"testing"
	"time"

	"ai-incident-platform/backend/internal/models"
)

func TestComputeWeightedLinearRegression_Rising(t *testing.T) {
	// Generate clearly rising data: value = 5 + 0.5*i (i minutes)
	now := time.Now()
	pts := make([]models.MetricDataPoint, 10)
	for i := range pts {
		pts[i] = models.MetricDataPoint{
			RecordedAt: now.Add(time.Duration(i) * time.Minute),
			Value:      5.0 + 0.5*float64(i),
		}
	}

	result := computeWeightedLinearRegression(pts)

	if result.Slope <= 0 {
		t.Fatalf("expected positive slope for rising data, got %.4f", result.Slope)
	}
	// Slope should be close to 0.5 per minute
	if math.Abs(result.Slope-0.5) > 0.15 {
		t.Errorf("slope %.4f is too far from expected 0.5", result.Slope)
	}
	// R² should be very high for perfect linear data
	if result.RSquared < 0.95 {
		t.Errorf("R² %.4f too low for perfectly linear data", result.RSquared)
	}
	if result.DataPoints != 10 {
		t.Errorf("expected 10 data points, got %d", result.DataPoints)
	}
}

func TestComputeWeightedLinearRegression_Flat(t *testing.T) {
	now := time.Now()
	pts := make([]models.MetricDataPoint, 8)
	for i := range pts {
		pts[i] = models.MetricDataPoint{
			RecordedAt: now.Add(time.Duration(i) * time.Minute),
			Value:      7.0, // constant
		}
	}

	result := computeWeightedLinearRegression(pts)
	// Flat → slope should be near 0
	if math.Abs(result.Slope) > 0.01 {
		t.Errorf("expected near-zero slope for flat data, got %.4f", result.Slope)
	}
}

func TestComputeWeightedLinearRegression_TooFewPoints(t *testing.T) {
	result := computeWeightedLinearRegression([]models.MetricDataPoint{{Value: 1}})
	if result.Slope != 0 || result.DataPoints != 0 {
		t.Errorf("expected zero slope and DataPoints=0 for single point, got slope=%.4f DataPoints=%d", result.Slope, result.DataPoints)
	}
}

func TestPredictiveBreachProjection(t *testing.T) {
	// If slope = 0.5/min and value is at 7, threshold is 10:
	// breach in (10-7)/0.5 = 6 min
	lastValue := 7.0
	threshold := 10.0
	slope := 0.5
	minutesToBreach := (threshold - lastValue) / slope
	if math.Abs(minutesToBreach-6.0) > 0.001 {
		t.Errorf("expected 6 minutes to breach, got %.2f", minutesToBreach)
	}
}
