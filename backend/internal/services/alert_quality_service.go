package services

import (
	"fmt"
	"math"
	"time"

	"ai-incident-platform/backend/internal/models"
	"ai-incident-platform/backend/internal/store"
)

const (
	// highNoiseRateThreshold marks a rule for deletion recommendation.
	highNoiseRateThreshold = 0.6
	// highFPRateThreshold marks a rule for tuning recommendation.
	highFPRateThreshold = 0.4
	// staleDays is how many days of silence makes a rule stale.
	staleDays = 30
	// dupCoOccurrenceRateThreshold marks a pair as definitively duplicate.
	dupCoOccurrenceRateThreshold = 0.5
)

// AlertQualityService computes alert quality governance reports.
type AlertQualityService struct {
	aqStore        *store.AlertQualityStore
	feedbackStore  *store.AlertFeedbackStore
}

func NewAlertQualityService(aq *store.AlertQualityStore, fb *store.AlertFeedbackStore) *AlertQualityService {
	return &AlertQualityService{aqStore: aq, feedbackStore: fb}
}

// RegisterRule upserts an alert rule definition.
func (s *AlertQualityService) RegisterRule(tenantID string, r models.AlertRuleRegistration) error {
	if r.ID == "" {
		return fmt.Errorf("alert rule id (fingerprint) is required")
	}
	return s.aqStore.UpsertRule(tenantID, r)
}

// GetReport computes the full quality report for windowDays.
func (s *AlertQualityService) GetReport(tenantID string, windowDays int) (*models.AlertQualityReport, error) {
	if windowDays <= 0 || windowDays > 90 {
		windowDays = 30
	}

	noisy, err := s.GetNoisyAlerts(tenantID, windowDays)
	if err != nil {
		return nil, fmt.Errorf("noisy alerts: %w", err)
	}

	dups, err := s.GetDuplicates(tenantID, windowDays)
	if err != nil {
		return nil, fmt.Errorf("duplicates: %w", err)
	}

	stale, err := s.GetStaleRules(tenantID)
	if err != nil {
		return nil, fmt.Errorf("stale rules: %w", err)
	}

	debt, err := s.GetAlertDebt(tenantID, windowDays, noisy, stale, dups)
	if err != nil {
		return nil, fmt.Errorf("debt: %w", err)
	}

	totalFired, _ := s.aqStore.CountTotalFiredInWindow(tenantID, windowDays)
	uniqueRules, _ := s.aqStore.CountUniqueRulesInWindow(tenantID, windowDays)

	highFP := 0
	for _, n := range noisy {
		if n.FPRate >= highFPRateThreshold {
			highFP++
		}
	}

	totalRecs := len(noisy) + len(dups) + len(stale)
	noisePct := 0
	if totalFired > 0 {
		noiseTotal := 0
		for _, n := range noisy {
			noiseTotal += n.NoiseCount + n.FPCount
		}
		noisePct = min(int(math.Round(float64(noiseTotal)/float64(totalFired)*100)), 100)
	}

	return &models.AlertQualityReport{
		TenantID:    tenantID,
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		WindowDays:  windowDays,
		Summary: models.AlertQualitySummary{
			TotalFired30d:        totalFired,
			UniqueRules:          uniqueRules,
			NoisyCount:           len(noisy),
			DuplicatePairCount:   len(dups),
			StaleCount:           len(stale),
			HighFPCount:          highFP,
			TotalRecommendations: totalRecs,
			EstimatedNoisePct:    noisePct,
		},
		NoisyAlerts: noisy,
		Duplicates:  dups,
		StaleRules:  stale,
		AlertDebt:   debt,
	}, nil
}

// GetNoisyAlerts returns alerts with low signal-to-noise ratio.
func (s *AlertQualityService) GetNoisyAlerts(tenantID string, windowDays int) ([]models.NoisyAlertRule, error) {
	rows, err := s.aqStore.QueryNoisyAlerts(tenantID, windowDays)
	if err != nil {
		return nil, err
	}

	// Load team info from rule registry for enrichment (best-effort).
	result := make([]models.NoisyAlertRule, 0, len(rows))
	for _, r := range rows {
		fpRate := safeDivide(r.FPCount, r.FPCount+r.NoiseCount+r.UsefulCount)
		noiseRate := safeDivide(r.NoiseCount, r.FPCount+r.NoiseCount+r.UsefulCount)
		score := computeQualityScore(r.FireCount, r.FPCount, r.NoiseCount, r.UsefulCount)
		rec := noisyRecommendation(noiseRate, fpRate, r.FireCount, windowDays)

		// Look up team from registry (ignore not-found).
		team := ""
		if rule, err2 := s.aqStore.GetRule(tenantID, r.Fingerprint); err2 == nil && rule != nil {
			team = rule.Team
		}

		result = append(result, models.NoisyAlertRule{
			Fingerprint:    r.Fingerprint,
			Title:          r.Title,
			Source:         r.Source,
			Service:        r.Service,
			Team:           team,
			FireCount7d:    r.FireCount,
			FPCount:        r.FPCount,
			NoiseCount:     r.NoiseCount,
			UsefulCount:    r.UsefulCount,
			FPRate:         fpRate,
			NoiseRate:      noiseRate,
			QualityScore:   score,
			Recommendation: rec,
		})
	}
	return result, nil
}

// GetDuplicates returns pairs of alerts that consistently co-fire.
func (s *AlertQualityService) GetDuplicates(tenantID string, windowDays int) ([]models.DuplicatePair, error) {
	rows, err := s.aqStore.QueryDuplicatePairs(tenantID, windowDays)
	if err != nil {
		return nil, err
	}

	result := make([]models.DuplicatePair, 0, len(rows))
	for _, r := range rows {
		// Co-occurrence rate: how often they fire together relative to the
		// more frequent of the two. Approximate using fire counts.
		fp1Count, _ := s.aqStore.FingerprintFireCount(tenantID, r.FP1, windowDays)
		rate := 0.0
		if fp1Count > 0 {
			rate = math.Round(float64(r.CoCount)/float64(fp1Count)*100) / 100
			if rate > 1.0 {
				rate = 1.0
			}
		}

		rec := "add_dedup_window"
		if rate >= dupCoOccurrenceRateThreshold {
			rec = "merge"
		}

		result = append(result, models.DuplicatePair{
			Fingerprint1:      r.FP1,
			Title1:            r.Title1,
			Service1:          r.Service1,
			Fingerprint2:      r.FP2,
			Title2:            r.Title2,
			Service2:          r.Service2,
			CoOccurrenceCount: r.CoCount,
			CoOccurrenceRate:  rate,
			Recommendation:    rec,
		})
	}
	return result, nil
}

// GetStaleRules returns alert rules that haven't fired in staleDays.
func (s *AlertQualityService) GetStaleRules(tenantID string) ([]models.StaleRule, error) {
	rows, err := s.aqStore.QueryStaleRules(tenantID, staleDays)
	if err != nil {
		return nil, err
	}

	now := time.Now()
	result := make([]models.StaleRule, 0, len(rows))
	for _, r := range rows {
		days := int(now.Sub(r.LastSeen).Hours() / 24)
		rec := "review_condition"
		if days > 90 {
			rec = "archive"
		}
		result = append(result, models.StaleRule{
			Fingerprint:       r.Fingerprint,
			Title:             r.Title,
			Source:            r.Source,
			Service:           r.Service,
			LastSeen:          r.LastSeen,
			DaysSinceLastFire: days,
			TotalFireCount:    r.TotalCount,
			Recommendation:    rec,
		})
	}
	return result, nil
}

// GetAlertDebt aggregates quality debt per service/team.
func (s *AlertQualityService) GetAlertDebt(
	tenantID string,
	windowDays int,
	noisy []models.NoisyAlertRule,
	stale []models.StaleRule,
	dups []models.DuplicatePair,
) ([]models.TeamAlertDebt, error) {
	rows, err := s.aqStore.QueryAlertDebtByService(tenantID, windowDays)
	if err != nil {
		return nil, err
	}

	// Build lookup maps for cross-referencing.
	noisyByService := map[string]int{}
	for _, n := range noisy {
		noisyByService[n.Service]++
	}
	staleByService := map[string]int{}
	for _, st := range stale {
		staleByService[st.Service]++
	}
	dupByService := map[string]int{}
	for _, d := range dups {
		dupByService[d.Service1]++
		dupByService[d.Service2]++
	}

	result := make([]models.TeamAlertDebt, 0, len(rows))
	for _, r := range rows {
		noisyC := noisyByService[r.Service]
		staleC := staleByService[r.Service]
		dupC := dupByService[r.Service]

		score := computeDebtScore(r.TotalFires, noisyC, staleC, dupC,
			r.FPFeedbackCount, r.NoiseFeedbackCount)

		rec := debtRecommendation(noisyC, staleC, dupC, r.FPFeedbackCount)

		result = append(result, models.TeamAlertDebt{
			Service:            r.Service,
			Team:               r.Service,
			TotalAlerts30d:     r.TotalFires,
			UniqueRules:        r.UniqueRules,
			NoisyCount:         noisyC,
			StaleCount:         staleC,
			DuplicateCount:     dupC,
			FPFeedbackCount:    r.FPFeedbackCount,
			NoiseFeedbackCount: r.NoiseFeedbackCount,
			DebtScore:          score,
			TopRecommendation:  rec,
		})
	}
	return result, nil
}

// -------------------------------------------------------------------
// Scoring helpers
// -------------------------------------------------------------------

// computeQualityScore converts feedback counts into a 0–100 score.
// 100 = all useful, 0 = all noise/FP.
func computeQualityScore(fires, fp, noise, useful int) int {
	total := fp + noise + useful
	if total == 0 {
		// No feedback: score from fire frequency alone
		// High fire count without feedback is suspicious.
		if fires >= 50 {
			return 40
		}
		return 70
	}
	badPct := safeDivide(fp+noise, total)
	return max(0, int(math.Round((1-badPct)*100)))
}

// noisyRecommendation returns a string action for a noisy alert.
func noisyRecommendation(noiseRate, fpRate float64, fires, windowDays int) string {
	firesPerDay := safeDivide(fires, windowDays)
	switch {
	case fpRate >= 0.7:
		return "delete"
	case noiseRate >= highNoiseRateThreshold:
		return "tune_threshold"
	case firesPerDay >= 20:
		return "consolidate"
	default:
		return "review"
	}
}

// computeDebtScore produces a 0–100 debt score (higher = worse).
func computeDebtScore(totalFires, noisyC, staleC, dupC, fpFeedback, noiseFeedback int) int {
	score := 0
	if totalFires > 0 {
		badFeedbackRatio := safeDivide(fpFeedback+noiseFeedback, totalFires)
		score += int(badFeedbackRatio * 40)
	}
	score += min(noisyC*5, 25)
	score += min(staleC*3, 15)
	score += min(dupC*4, 20)
	return min(score, 100)
}

// debtRecommendation returns the top action for a team's debt.
func debtRecommendation(noisyC, staleC, dupC, fpFeedback int) string {
	switch {
	case noisyC >= 3:
		return "tune_or_delete_noisy_rules"
	case dupC >= 2:
		return "merge_duplicate_alerts"
	case staleC >= 5:
		return "archive_stale_rules"
	case fpFeedback >= 5:
		return "review_false_positive_alerts"
	default:
		return "review_alert_inventory"
	}
}

func safeDivide(num, denom int) float64 {
	if denom == 0 {
		return 0.0
	}
	return float64(num) / float64(denom)
}
