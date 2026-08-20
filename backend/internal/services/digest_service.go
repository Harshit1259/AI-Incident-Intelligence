package services

import (
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	"ai-incident-platform/backend/internal/llm"
	"ai-incident-platform/backend/internal/models"
	"ai-incident-platform/backend/internal/store"
)

type DigestService struct {
	incidentStore *store.IncidentStore
	metricsStore  *store.IncidentMetricsStore
	llmClient     *llm.Client
}

func NewDigestService(is *store.IncidentStore, ms *store.IncidentMetricsStore, lc *llm.Client) *DigestService {
	return &DigestService{
		incidentStore: is,
		metricsStore:  ms,
		llmClient:     lc,
	}
}

// GenerateWeeklyDigest creates the weekly reliability digest.
func (s *DigestService) GenerateWeeklyDigest(tenantID string) (*models.WeeklyDigest, error) {
	now := time.Now()
	weekEnd := now
	weekStart := now.Add(-7 * 24 * time.Hour)
	prevWeekStart := weekStart.Add(-7 * 24 * time.Hour)

	// Fetch incidents for this week
	filter := models.IncidentListFilter{
		From:      &weekStart,
		To:        &weekEnd,
		Page:      1,
		PageSize:  100,
		SortBy:    "last_event_time",
		SortOrder: "desc",
	}

	result, err := s.incidentStore.ListIncidents(filter)
	if err != nil {
		return nil, fmt.Errorf("fetch incidents for digest: %w", err)
	}

	totalIncidents := result.Total
	var criticalCount int
	for _, item := range result.Items {
		if strings.ToLower(item.Severity) == "critical" {
			criticalCount++
		}
	}

	// Compute MTTR from metrics
	metrics, err := s.metricsStore.GetForPeriod(tenantID, weekStart)
	if err != nil {
		return nil, fmt.Errorf("fetch metrics for digest: %w", err)
	}

	var totalTTR float64
	var ttrCount int
	for _, m := range metrics {
		if m.TTRSeconds > 0 {
			totalTTR += float64(m.TTRSeconds)
			ttrCount++
		}
	}

	var mttr float64
	if ttrCount > 0 {
		mttr = totalTTR / float64(ttrCount)
	}

	// Previous week MTTR for trend
	prevMetrics, _ := s.metricsStore.GetForPeriod(tenantID, prevWeekStart)
	var prevTTR float64
	var prevTTRCount int
	for _, m := range prevMetrics {
		if m.DetectedAt != nil && m.DetectedAt.Before(weekStart) && m.TTRSeconds > 0 {
			prevTTR += float64(m.TTRSeconds)
			prevTTRCount++
		}
	}

	mttrTrend := "stable"
	if prevTTRCount > 0 {
		prevMTTR := prevTTR / float64(prevTTRCount)
		if mttr < prevMTTR*0.9 {
			mttrTrend = "improving"
		} else if mttr > prevMTTR*1.1 {
			mttrTrend = "degrading"
		}
	}

	// Reliability score: 100 - (criticalCount * 10) - (totalIncidents * 2), clamped 0-100
	reliabilityScore := 100.0 - float64(criticalCount)*10 - float64(totalIncidents)*2
	if reliabilityScore < 0 {
		reliabilityScore = 0
	}
	if reliabilityScore > 100 {
		reliabilityScore = 100
	}

	// Top recurring: group by service + root_cause_type
	type recurKey struct {
		service       string
		rootCauseType string
	}
	recurMap := make(map[recurKey]*models.RecurringIssue)
	for _, item := range result.Items {
		key := recurKey{service: item.Service, rootCauseType: item.RootCauseSummary}
		if ri, ok := recurMap[key]; ok {
			ri.Count++
			ri.LastSeen = item.LastEventTime.Format(time.RFC3339)
		} else {
			pattern := item.RootCauseSummary
			if pattern == "" {
				pattern = "unknown"
			}
			recurMap[key] = &models.RecurringIssue{
				Pattern:  pattern,
				Service:  item.Service,
				Count:    1,
				LastSeen: item.LastEventTime.Format(time.RFC3339),
			}
		}
	}

	recurring := make([]models.RecurringIssue, 0, len(recurMap))
	for _, ri := range recurMap {
		if ri.Count > 1 {
			recurring = append(recurring, *ri)
		}
	}
	sort.Slice(recurring, func(i, j int) bool {
		return recurring[i].Count > recurring[j].Count
	})
	if len(recurring) > 3 {
		recurring = recurring[:3]
	}

	// Highlights
	highlights := []string{}
	if criticalCount > 0 {
		highlights = append(highlights, fmt.Sprintf("%d critical incident(s) this week", criticalCount))
	}
	if totalIncidents > 0 {
		highlights = append(highlights, fmt.Sprintf("%d total incidents tracked", totalIncidents))
	}
	if mttr > 0 {
		highlights = append(highlights, fmt.Sprintf("Average resolution time: %.0f seconds (%.1f minutes)", mttr, mttr/60))
	}
	if mttrTrend == "improving" {
		highlights = append(highlights, "MTTR is improving compared to last week")
	} else if mttrTrend == "degrading" {
		highlights = append(highlights, "MTTR is degrading compared to last week - investigate bottlenecks")
	}

	// Try LLM for narrative if configured
	if s.llmClient != nil && s.llmClient.IsConfigured() && totalIncidents > 0 {
		prompt := fmt.Sprintf(
			"Generate a brief 2-3 sentence reliability digest summary for a team. "+
				"This week: %d incidents (%d critical), MTTR %.0f seconds, reliability score %.0f/100. "+
				"Trend: MTTR is %s. Keep it professional and actionable.",
			totalIncidents, criticalCount, mttr, reliabilityScore, mttrTrend,
		)
		narrative, err := s.llmClient.Complete(prompt)
		if err == nil && narrative != "" {
			highlights = append([]string{narrative}, highlights...)
		} else if err != nil {
			slog.Warn("digest: LLM narrative failed (falling back to rule-based)", "error", err)
		}
	}

	digest := &models.WeeklyDigest{
		TenantID:         tenantID,
		WeekStart:        weekStart,
		WeekEnd:          weekEnd,
		TotalIncidents:   totalIncidents,
		CriticalCount:    criticalCount,
		MTTRSeconds:      mttr,
		MTTRTrend:        mttrTrend,
		ReliabilityScore: reliabilityScore,
		TopRecurring:     recurring,
		Highlights:       highlights,
		GeneratedAt:      now,
	}

	return digest, nil
}
