package services

// RiskExposureService provides a live aggregate view of risk across all open incidents.
// It answers: "How exposed are we RIGHT NOW?" — unlike the ROI service (historical)
// and the business impact service (per-incident).

import (
	"fmt"
	"strings"
	"time"

	"ai-incident-platform/backend/internal/models"
	"ai-incident-platform/backend/internal/store"
)

// RiskExposureService computes live risk dashboards.
type RiskExposureService struct {
	incidentStore *store.IncidentStore
	metricsStore  *store.IncidentMetricsStore
}

func NewRiskExposureService(is *store.IncidentStore, ms *store.IncidentMetricsStore) *RiskExposureService {
	return &RiskExposureService{incidentStore: is, metricsStore: ms}
}

// GetExposureDashboard aggregates all open incidents into a live risk snapshot.
func (s *RiskExposureService) GetExposureDashboard(tenantID string) (*models.RiskExposureDashboard, error) {
	openIncidents, err := s.incidentStore.ListOpenIncidentsByTenant(tenantID)
	if err != nil {
		return nil, fmt.Errorf("list open incidents: %w", err)
	}

	dashboard := &models.RiskExposureDashboard{
		TenantID:       tenantID,
		ComputedAt:     time.Now().Format(time.RFC3339),
		AtRiskServices: []models.AtRiskService{},
		ExposureTrend:  []models.ExposurePoint{},
	}

	// Aggregate counts and blast radius
	impactedServicesSet := map[string]bool{}
	serviceMap := map[string]*models.AtRiskService{}

	for i := range openIncidents {
		inc := openIncidents[i]

		switch strings.ToLower(inc.Severity) {
		case "critical":
			dashboard.CriticalCount++
			dashboard.EstimatedImpactUSD += 5000 // rough estimate per critical open incident
			if inc.Status == "open" {
				dashboard.UnackedCritical++
			}
		case "high":
			dashboard.HighCount++
			dashboard.EstimatedImpactUSD += 1500
		case "medium":
			dashboard.MediumCount++
			dashboard.EstimatedImpactUSD += 300
		case "low":
			dashboard.LowCount++
			dashboard.EstimatedImpactUSD += 50
		}

		// Collect blast radius
		for _, svc := range inc.ImpactedServices {
			impactedServicesSet[svc] = true
		}
		impactedServicesSet[inc.Service] = true

		// Build per-service risk summary
		entry, exists := serviceMap[inc.Service]
		if !exists {
			entry = &models.AtRiskService{Service: inc.Service}
			serviceMap[inc.Service] = entry
		}
		entry.OpenIncidents++
		if severityWeight(inc.Severity) > severityWeight(entry.MaxSeverity) {
			entry.MaxSeverity = inc.Severity
		}
		entry.RiskScore = clampInt(
			entry.RiskScore+inc.RiskScore/entry.OpenIncidents, 0, 100,
		)
	}

	dashboard.TotalOpenIncidents = len(openIncidents)
	dashboard.TotalBlastRadius = len(impactedServicesSet)

	// Fetch 7-day metrics for MTTR trend
	since7d := time.Now().Add(-7 * 24 * time.Hour)
	metrics7d, err := s.metricsStore.GetForPeriod(tenantID, since7d)
	if err == nil {
		var totalTTR float64
		var ttrCount int
		for _, m := range metrics7d {
			if m.TTRSeconds > 0 {
				totalTTR += float64(m.TTRSeconds)
				ttrCount++
			}
		}
		if ttrCount > 0 {
			dashboard.MTTRTrendMinutes = totalTTR / float64(ttrCount) / 60
		}
	}

	// Flatten service map
	for _, svc := range serviceMap {
		dashboard.AtRiskServices = append(dashboard.AtRiskServices, *svc)
	}

	// Build 7-day exposure trend
	var m7d []models.IncidentMetrics
	if err == nil {
		m7d = metrics7d
	}
	dashboard.ExposureTrend = buildExposureTrend(m7d, openIncidents)

	return dashboard, nil
}

// buildExposureTrend produces a 7-day daily open/new/resolved series.
func buildExposureTrend(metrics []models.IncidentMetrics, open []models.Incident) []models.ExposurePoint {
	points := make([]models.ExposurePoint, 7)
	now := time.Now()

	for i := 6; i >= 0; i-- {
		day := now.AddDate(0, 0, -i)
		points[6-i] = models.ExposurePoint{
			Date: day.Format("2006-01-02"),
		}
	}

	// Count new and resolved per day from metrics
	for _, m := range metrics {
		dayStr := m.DetectedAt.Format("2006-01-02")
		for j := range points {
			if points[j].Date == dayStr {
				points[j].New++
				if m.ResolvedAt != nil {
					dayResolved := m.ResolvedAt.Format("2006-01-02")
					for k := range points {
						if points[k].Date == dayResolved {
							points[k].Resolved++
						}
					}
				}
			}
		}
	}

	// Open count per day = running sum
	runningOpen := len(open)
	for i := len(points) - 1; i >= 0; i-- {
		runningOpen = runningOpen - points[i].New + points[i].Resolved
		if runningOpen < 0 {
			runningOpen = 0
		}
		points[i].Open = runningOpen + points[i].New
	}

	return points
}
