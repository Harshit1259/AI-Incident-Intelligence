package services

import (
	"fmt"
	"math"
	"time"

	"ai-incident-platform/backend/internal/models"
	"ai-incident-platform/backend/internal/store"
)

const (
	// avgAlertHandleMinutes is how long an engineer spends triaging one alert that
	// turns out to be noise (look at it, check context, decide to dismiss).
	avgAlertHandleMinutes = 5.0
)

// NoiseReductionService computes the real-time noise reduction dashboard.
type NoiseReductionService struct {
	eventStore    *store.EventStore
	incidentStore *store.IncidentStore
	sreHourlyCost float64
}

// NewNoiseReductionService creates the service.
// sreHourlyCost defaults to $150/hr if ≤ 0.
func NewNoiseReductionService(
	es *store.EventStore,
	is *store.IncidentStore,
	sreHourlyCost float64,
) *NoiseReductionService {
	if sreHourlyCost <= 0 {
		sreHourlyCost = 150.0
	}
	return &NoiseReductionService{
		eventStore:    es,
		incidentStore: is,
		sreHourlyCost: sreHourlyCost,
	}
}

// ComputeScore computes the full noise reduction funnel for the given tenant and window.
// windowDays defaults to 30 when ≤ 0.
func (s *NoiseReductionService) ComputeScore(tenantID string, windowDays int) (*models.NoiseReductionScore, error) {
	if windowDays <= 0 {
		windowDays = 30
	}
	since := time.Now().Add(-time.Duration(windowDays) * 24 * time.Hour)

	// ── Raw funnel counts ────────────────────────────────────────────────────
	rawAlerts, err := s.eventStore.CountEventsInPeriod(tenantID, since)
	if err != nil {
		return nil, fmt.Errorf("count raw alerts: %w", err)
	}

	afterDedup, err := s.eventStore.CountUniqueFingerprints(tenantID, since)
	if err != nil {
		return nil, fmt.Errorf("count unique fingerprints: %w", err)
	}

	incidents, err := s.incidentStore.CountIncidentsInPeriod(tenantID, since)
	if err != nil {
		return nil, fmt.Errorf("count incidents: %w", err)
	}

	confirmed, err := s.incidentStore.CountConfirmedIncidents(tenantID, since)
	if err != nil {
		return nil, fmt.Errorf("count confirmed incidents: %w", err)
	}

	// ── Derived metrics ──────────────────────────────────────────────────────
	dedupReductionPct := pctReduction(rawAlerts, afterDedup)
	correlationReductionPct := pctReduction(afterDedup, incidents)
	confirmationRate := safeRatio(confirmed, incidents) * 100
	noiseAvoided := max(0, rawAlerts-confirmed)
	noiseReductionPct := safeRatio(noiseAvoided, rawAlerts) * 100
	onCallHoursSaved := round2f(float64(noiseAvoided) * avgAlertHandleMinutes / 60.0)
	engineerCostSaved := round2f(onCallHoursSaved * s.sreHourlyCost)
	grade := noiseGrade(noiseReductionPct)

	// ── Funnel lines for the UI ───────────────────────────────────────────────
	funnelLines := buildFunnelLines(rawAlerts, afterDedup, incidents, confirmed,
		dedupReductionPct, correlationReductionPct)

	savingsLines := []string{
		fmt.Sprintf("Noise you didn't page on-call for: %s alerts", fmtCount(noiseAvoided)),
		fmt.Sprintf("On-call hours saved: ~%.0f hours", onCallHoursSaved),
		fmt.Sprintf("Engineer cost saved: ~$%.0f", engineerCostSaved),
	}

	return &models.NoiseReductionScore{
		TenantID:   tenantID,
		WindowDays: windowDays,
		ComputedAt: time.Now(),

		RawAlertsReceived:       rawAlerts,
		AfterDedup:              afterDedup,
		DedupReductionPct:       round2f(dedupReductionPct),
		AfterCorrelation:        incidents,
		CorrelationReductionPct: round2f(correlationReductionPct),
		TrueIncidents:           confirmed,
		ConfirmationRate:        round2f(confirmationRate),

		NoiseAvoided:      noiseAvoided,
		OnCallHoursSaved:  onCallHoursSaved,
		EngineerCostSaved: engineerCostSaved,

		NoiseReductionPct:   round2f(noiseReductionPct),
		NoiseReductionGrade: grade,

		FunnelLines:  funnelLines,
		SavingsLines: savingsLines,
	}, nil
}

// ─── Helpers ────────────────────────────────────────────────────────────────

func pctReduction(before, after int) float64 {
	if before == 0 {
		return 0
	}
	return math.Max(0, float64(before-after)/float64(before)*100)
}

func safeRatio(num, denom int) float64 {
	if denom == 0 {
		return 0
	}
	return float64(num) / float64(denom)
}

func round2f(v float64) float64 {
	return math.Round(v*100) / 100
}

func noiseGrade(reductionPct float64) string {
	switch {
	case reductionPct >= 90:
		return "A"
	case reductionPct >= 75:
		return "B"
	case reductionPct >= 50:
		return "C"
	default:
		return "D"
	}
}

func fmtCount(n int) string {
	if n >= 1000 {
		return fmt.Sprintf("%d,%03d", n/1000, n%1000)
	}
	return fmt.Sprintf("%d", n)
}

func buildFunnelLines(raw, afterDedup, incidents, confirmed int, dedupPct, corrPct float64) []models.FunnelLine {
	pctStr := func(pct float64) string {
		return fmt.Sprintf("(-%s%%)", fmtPct(pct))
	}

	return []models.FunnelLine{
		{
			Label: "Raw alerts received",
			Count: raw,
		},
		{
			Label:    "After deduplication",
			Count:    afterDedup,
			Delta:    pctStr(dedupPct),
			DeltaPct: -dedupPct,
		},
		{
			Label:    "After correlation",
			Count:    incidents,
			Delta:    pctStr(corrPct),
			DeltaPct: -corrPct,
		},
		{
			Label:     "True incidents (confirmed)",
			Count:     confirmed,
			Highlight: true,
		},
	}
}

func fmtPct(pct float64) string {
	return fmt.Sprintf("%.1f", pct)
}
