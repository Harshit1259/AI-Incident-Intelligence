package services

import (
	"fmt"
	"strings"
	"time"

	"ai-incident-platform/backend/internal/models"
	"ai-incident-platform/backend/internal/store"
)

type ComplianceService struct {
	incidentStore *store.IncidentStore
	metricsStore  *store.IncidentMetricsStore
	sloService    *SLOService
}

func NewComplianceService(
	is *store.IncidentStore,
	ms *store.IncidentMetricsStore,
	ss *SLOService,
) *ComplianceService {
	return &ComplianceService{
		incidentStore: is,
		metricsStore:  ms,
		sloService:    ss,
	}
}

func (s *ComplianceService) GenerateReport(tenantID string, days int) (*models.ComplianceReport, error) {
	if days <= 0 {
		days = 30
	}

	now := time.Now()
	periodStart := now.AddDate(0, 0, -days)

	// Get incidents for the period
	incidents, err := s.incidentStore.GetIncidents()
	if err != nil {
		return nil, fmt.Errorf("compliance: get incidents: %w", err)
	}

	// Filter incidents by time window
	var periodIncidents []models.Incident
	for _, inc := range incidents {
		if inc.FirstEventTime.After(periodStart) {
			periodIncidents = append(periodIncidents, inc)
		}
	}

	// Get metrics for the period
	metrics, err := s.metricsStore.GetForPeriod(tenantID, periodStart)
	if err != nil {
		return nil, fmt.Errorf("compliance: get metrics: %w", err)
	}

	// Compute MTTR and MTTA
	var totalTTR, totalTTA float64
	var ttrCount, ttaCount int
	for _, m := range metrics {
		if m.TTRSeconds > 0 {
			totalTTR += float64(m.TTRSeconds)
			ttrCount++
		}
		if m.TTASeconds > 0 {
			totalTTA += float64(m.TTASeconds)
			ttaCount++
		}
	}

	var mttr, mtta float64
	if ttrCount > 0 {
		mttr = totalTTR / float64(ttrCount)
	}
	if ttaCount > 0 {
		mtta = totalTTA / float64(ttaCount)
	}

	// Get SLO statuses
	sloStatuses, err := s.sloService.GetAllStatuses(tenantID)
	if err != nil {
		sloStatuses = []models.SLOStatus{}
	}

	var sloLines []models.SLOComplianceLine
	breachedCount := 0
	totalBadMinutes := 0.0

	for _, st := range sloStatuses {
		compliant := st.Status != "breached"
		if !compliant {
			breachedCount++
		}

		errorBudgetUsed := 100.0 - st.ErrorBudgetRemaining
		if errorBudgetUsed < 0 {
			errorBudgetUsed = 0
		}

		// Accumulate bad minutes from measurements for uptime calculation
		if st.ErrorBudgetRemaining < 100 {
			totalBadMinutes += (100 - st.ErrorBudgetRemaining) / 100 * float64(st.Definition.WindowDays) * 24 * 60
		}

		sloLines = append(sloLines, models.SLOComplianceLine{
			SLOName:         st.Definition.Name,
			Service:         st.Definition.Service,
			Target:          st.Definition.TargetPercent,
			Actual:          st.CurrentPercent,
			Compliant:       compliant,
			ErrorBudgetUsed: errorBudgetUsed,
		})
	}

	// Compute uptime percent
	totalMinutes := float64(days) * 24 * 60
	uptimePercent := 100.0
	if totalMinutes > 0 && totalBadMinutes > 0 {
		uptimePercent = (totalMinutes - totalBadMinutes) / totalMinutes * 100
		if uptimePercent < 0 {
			uptimePercent = 0
		}
	}

	return &models.ComplianceReport{
		TenantID:       tenantID,
		PeriodStart:    periodStart,
		PeriodEnd:      now,
		TotalIncidents: len(periodIncidents),
		MTTR:           mttr,
		MTTA:           mtta,
		SLOCompliance:  sloLines,
		UptimePercent:  uptimePercent,
		BreachedSLOs:   breachedCount,
		GeneratedAt:    now,
	}, nil
}

func (s *ComplianceService) ExportCSV(tenantID string, days int) (string, error) {
	report, err := s.GenerateReport(tenantID, days)
	if err != nil {
		return "", err
	}

	var b strings.Builder
	b.WriteString("SLO Name,Service,Target %,Actual %,Compliant,Error Budget Used %\n")

	for _, line := range report.SLOCompliance {
		compliantStr := "No"
		if line.Compliant {
			compliantStr = "Yes"
		}
		b.WriteString(fmt.Sprintf("%s,%s,%.2f,%.2f,%s,%.2f\n",
			csvEscape(line.SLOName),
			csvEscape(line.Service),
			line.Target,
			line.Actual,
			compliantStr,
			line.ErrorBudgetUsed,
		))
	}

	return b.String(), nil
}

func csvEscape(s string) string {
	if strings.ContainsAny(s, ",\"\n") {
		return "\"" + strings.ReplaceAll(s, "\"", "\"\"") + "\""
	}
	return s
}
