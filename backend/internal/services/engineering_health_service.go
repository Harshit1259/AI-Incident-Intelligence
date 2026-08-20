package services

import (
	"fmt"
	"sort"
	"time"

	"ai-incident-platform/backend/internal/models"
	"ai-incident-platform/backend/internal/store"
)

type EngineeringHealthService struct {
	metricsStore  *store.IncidentMetricsStore
	oncallService *OnCallService
}

func NewEngineeringHealthService(ms *store.IncidentMetricsStore, os *OnCallService) *EngineeringHealthService {
	return &EngineeringHealthService{
		metricsStore:  ms,
		oncallService: os,
	}
}

// GetSummary computes engineering health metrics for a time period.
func (s *EngineeringHealthService) GetSummary(tenantID string, days int) (*models.EngineeringHealthSummary, error) {
	since := time.Now().Add(-time.Duration(days) * 24 * time.Hour)
	metrics, err := s.metricsStore.GetForPeriod(tenantID, since)
	if err != nil {
		return nil, fmt.Errorf("fetch incident metrics: %w", err)
	}

	summary := &models.EngineeringHealthSummary{
		TenantID:       tenantID,
		PeriodDays:     days,
		TotalIncidents: len(metrics),
	}

	if len(metrics) == 0 {
		summary.TopResponders = []models.ResponderStats{}
		summary.BurnoutRisk = []models.BurnoutFlag{}
		return summary, nil
	}

	// Compute MTTR/MTTA/MTTD averages
	var totalTTR, totalTTA, totalTTD float64
	var ttrCount, ttaCount, ttdCount int
	var autoResolved int
	var totalToilMinutes int

	responderMap := make(map[string]*models.ResponderStats)

	for _, m := range metrics {
		if m.TTRSeconds > 0 {
			totalTTR += float64(m.TTRSeconds)
			ttrCount++
		}
		if m.TTASeconds > 0 {
			totalTTA += float64(m.TTASeconds)
			ttaCount++
		}
		if m.TTDSeconds > 0 {
			totalTTD += float64(m.TTDSeconds)
			ttdCount++
		}
		if m.IsAutoResolved {
			autoResolved++
		}
		totalToilMinutes += m.ToilMinutes

		// Group by responder
		if m.Responder != "" {
			rs, exists := responderMap[m.Responder]
			if !exists {
				rs = &models.ResponderStats{Responder: m.Responder}
				responderMap[m.Responder] = rs
			}
			rs.IncidentCount++
			rs.ToilHours += float64(m.ToilMinutes) / 60
			if m.TTRSeconds > 0 {
				rs.AvgTTRSeconds = (rs.AvgTTRSeconds*float64(rs.IncidentCount-1) + float64(m.TTRSeconds)) / float64(rs.IncidentCount)
			}
			// Estimate on-call hours: TTR + 1 hour overhead per incident
			rs.OnCallHours += float64(m.TTRSeconds)/3600 + 1
		}
	}

	if ttrCount > 0 {
		summary.MTTR = totalTTR / float64(ttrCount)
	}
	if ttaCount > 0 {
		summary.MTTA = totalTTA / float64(ttaCount)
	}
	if ttdCount > 0 {
		summary.MTTD = totalTTD / float64(ttdCount)
	}

	summary.AutoResolved = autoResolved
	summary.TotalToilHours = float64(totalToilMinutes) / 60

	// Top responders sorted by incident count descending
	responders := make([]models.ResponderStats, 0, len(responderMap))
	for _, rs := range responderMap {
		responders = append(responders, *rs)
	}
	sort.Slice(responders, func(i, j int) bool {
		return responders[i].IncidentCount > responders[j].IncidentCount
	})
	summary.TopResponders = responders

	// Burnout flags
	var burnoutFlags []models.BurnoutFlag
	for _, rs := range responders {
		if rs.IncidentCount > 5 || rs.OnCallHours > 40 {
			burnoutFlags = append(burnoutFlags, models.BurnoutFlag{
				Responder: rs.Responder,
				Reason:    fmt.Sprintf("%d incidents, %.1f on-call hours in %d days", rs.IncidentCount, rs.OnCallHours, days),
				RiskLevel: "high",
			})
		} else if rs.IncidentCount > 3 || rs.OnCallHours > 30 {
			burnoutFlags = append(burnoutFlags, models.BurnoutFlag{
				Responder: rs.Responder,
				Reason:    fmt.Sprintf("%d incidents, %.1f on-call hours in %d days", rs.IncidentCount, rs.OnCallHours, days),
				RiskLevel: "medium",
			})
		}
	}
	if burnoutFlags == nil {
		burnoutFlags = []models.BurnoutFlag{}
	}
	summary.BurnoutRisk = burnoutFlags

	return summary, nil
}

// RecordIncidentMetrics records TTD/TTA/TTR when an incident status changes.
func (s *EngineeringHealthService) RecordIncidentMetrics(incident models.Incident, action string, responder string) error {
	now := time.Now()
	m := models.IncidentMetrics{
		IncidentID: incident.ID,
		TenantID:   incident.TenantID,
		Responder:  responder,
	}

	firstEventTime := incident.FirstEventTime

	switch action {
	case "detected":
		m.DetectedAt = &now
		if !firstEventTime.IsZero() {
			m.TTDSeconds = int(now.Sub(firstEventTime).Seconds())
		}
	case "ack":
		m.AcknowledgedAt = &now
		m.DetectedAt = &firstEventTime
		if !firstEventTime.IsZero() {
			m.TTASeconds = int(now.Sub(firstEventTime).Seconds())
		}
	case "resolve":
		m.ResolvedAt = &now
		m.DetectedAt = &firstEventTime
		if !firstEventTime.IsZero() {
			m.TTRSeconds = int(now.Sub(firstEventTime).Seconds())
			// Estimate toil: 10% of TTR as toil
			m.ToilMinutes = m.TTRSeconds / 60 / 10
			if m.ToilMinutes < 1 {
				m.ToilMinutes = 1
			}
		}
	}

	return s.metricsStore.Upsert(m)
}
