package services

import (
	"fmt"
	"time"

	"ai-incident-platform/backend/internal/models"
	"ai-incident-platform/backend/internal/store"
)

type ROIService struct {
	metricsStore  *store.IncidentMetricsStore
	incidentStore *store.IncidentStore
	hourlyCost    float64
}

func NewROIService(ms *store.IncidentMetricsStore, is *store.IncidentStore, hourlyCost float64) *ROIService {
	if hourlyCost <= 0 {
		hourlyCost = 150.0
	}
	return &ROIService{
		metricsStore:  ms,
		incidentStore: is,
		hourlyCost:    hourlyCost,
	}
}

// GetROI computes the ROI dashboard for a time period.
func (s *ROIService) GetROI(tenantID string, days int) (*models.ROIDashboard, error) {
	since := time.Now().Add(-time.Duration(days) * 24 * time.Hour)
	metrics, err := s.metricsStore.GetForPeriod(tenantID, since)
	if err != nil {
		return nil, fmt.Errorf("fetch incident metrics for ROI: %w", err)
	}

	dashboard := &models.ROIDashboard{
		TenantID:       tenantID,
		PeriodDays:     days,
		TotalIncidents: len(metrics),
		TopWins:        []models.ROIWin{},
	}

	if len(metrics) == 0 {
		return dashboard, nil
	}

	var autoResolved int
	var totalTTR float64
	var ttrCount int
	var totalToilMinutes int

	// Split metrics into first and second half for trend comparison
	midpoint := len(metrics) / 2

	var firstHalfTTR float64
	var firstHalfCount int
	var secondHalfTTR float64
	var secondHalfCount int

	for i, m := range metrics {
		if m.IsAutoResolved {
			autoResolved++
		}
		if m.TTRSeconds > 0 {
			totalTTR += float64(m.TTRSeconds)
			ttrCount++

			if i < midpoint {
				firstHalfTTR += float64(m.TTRSeconds)
				firstHalfCount++
			} else {
				secondHalfTTR += float64(m.TTRSeconds)
				secondHalfCount++
			}
		}
		totalToilMinutes += m.ToilMinutes
	}

	dashboard.AutoResolved = autoResolved

	// HoursSaved: 30 min per auto-resolved incident
	dashboard.HoursSaved = float64(autoResolved) * 0.5

	// EngineerTimeSaved includes toil reduction estimate
	dashboard.EngineerTimeSaved = dashboard.HoursSaved + float64(totalToilMinutes)/60*0.2

	// SRECostAvoided
	dashboard.SRECostAvoided = dashboard.HoursSaved * s.hourlyCost

	// CostPerIncident
	if ttrCount > 0 {
		avgTTR := totalTTR / float64(ttrCount)
		dashboard.CostPerIncident = avgTTR * s.hourlyCost / 3600
	}

	// MTTRReduction: compare first half vs second half
	if firstHalfCount > 0 && secondHalfCount > 0 {
		avgFirst := firstHalfTTR / float64(firstHalfCount)
		avgSecond := secondHalfTTR / float64(secondHalfCount)
		if avgFirst > 0 {
			dashboard.MTTRReduction = (avgFirst - avgSecond) / avgFirst * 100
		}
	}

	// MonthlyROI
	dashboard.MonthlyROI = dashboard.SRECostAvoided + dashboard.IncidentReduction*dashboard.CostPerIncident

	// Top wins
	if autoResolved > 0 {
		dashboard.TopWins = append(dashboard.TopWins, models.ROIWin{
			Label:       "Auto-Resolution",
			Value:       dashboard.HoursSaved,
			Description: fmt.Sprintf("%d incidents auto-resolved, saving %.1f hours", autoResolved, dashboard.HoursSaved),
		})
	}
	if dashboard.SRECostAvoided > 0 {
		dashboard.TopWins = append(dashboard.TopWins, models.ROIWin{
			Label:       "SRE Cost Savings",
			Value:       dashboard.SRECostAvoided,
			Description: fmt.Sprintf("$%.0f saved in SRE time", dashboard.SRECostAvoided),
		})
	}
	if dashboard.MTTRReduction > 0 {
		dashboard.TopWins = append(dashboard.TopWins, models.ROIWin{
			Label:       "Faster MTTR",
			Value:       dashboard.MTTRReduction,
			Description: fmt.Sprintf("%.1f%% reduction in mean time to resolve", dashboard.MTTRReduction),
		})
	}
	if totalToilMinutes > 0 {
		dashboard.TopWins = append(dashboard.TopWins, models.ROIWin{
			Label:       "Toil Reduction",
			Value:       float64(totalToilMinutes) / 60,
			Description: fmt.Sprintf("%.1f hours of toil tracked and reduced", float64(totalToilMinutes)/60),
		})
	}

	return dashboard, nil
}
