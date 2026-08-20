package services

import (
	"fmt"
	"time"

	"ai-incident-platform/backend/internal/models"
	"ai-incident-platform/backend/internal/store"
)

// HealthScoreService computes the 4-pillar customer health score for each tenant.
//
// Pillars (each 0-25 pts, total 0-100):
//   1. Incident Recency     — active users encounter and handle incidents regularly
//   2. Integration Depth    — more connected sources = stickier product
//   3. Alert Volume Trend   — growing alert volume means the platform is expanding in value
//   4. AI Feature Adoption  — automation, RCA, and copilot usage = deep product investment
type HealthScoreService struct {
	store *store.HealthScoreStore
}

func NewHealthScoreService(s *store.HealthScoreStore) *HealthScoreService {
	return &HealthScoreService{store: s}
}

// ComputeScore computes and persists the health score for a single tenant.
func (svc *HealthScoreService) ComputeScore(tenantID string) (*models.TenantHealthScore, error) {
	raw, err := svc.store.GetRawSignals(tenantID)
	if err != nil {
		return nil, fmt.Errorf("health score: raw signals: %w", err)
	}

	incidentSignal := scoreIncidentRecency(raw.LastIncidentTime)
	integrationSignal := scoreIntegrations(raw.IntegrationCount)
	trendSignal := scoreAlertTrend(raw.EventsThisWeek, raw.EventsLastWeek)
	aiSignal := scoreAIAdoption(raw.AutoRemediationCount, raw.ExplainUseCount, raw.CopilotUseCount)

	total := incidentSignal.Score + integrationSignal.Score + trendSignal.Score + aiSignal.Score

	result := &models.TenantHealthScore{
		TenantID:   tenantID,
		TenantName: raw.TenantName,
		Score:      total,
		MaxScore:   100,
		ChurnRisk:  churnRiskFromScore(total),
		Signals:    []models.HealthSignal{incidentSignal, integrationSignal, trendSignal, aiSignal},
		Actions:    recommendActions(incidentSignal, integrationSignal, trendSignal, aiSignal, total),
		ComputedAt: time.Now(),
	}

	// Persist snapshot asynchronously — failure here never blocks the caller.
	go func() { _ = svc.store.SaveSnapshot(*result) }()

	return result, nil
}

// ComputeAllScores computes health scores for every active tenant.
func (svc *HealthScoreService) ComputeAllScores() ([]models.TenantHealthScore, error) {
	ids, err := svc.store.GetAllTenantIDs()
	if err != nil {
		return nil, fmt.Errorf("health score: list tenants: %w", err)
	}

	results := make([]models.TenantHealthScore, 0, len(ids))
	for _, id := range ids {
		score, err := svc.ComputeScore(id)
		if err != nil {
			continue // don't let one broken tenant abort the whole list
		}
		results = append(results, *score)
	}
	return results, nil
}

// LogAction records a CSM action taken on a tenant.
func (svc *HealthScoreService) LogAction(a models.HealthAction) error {
	return svc.store.LogAction(a)
}

// ListActions returns all CSM actions for a tenant.
func (svc *HealthScoreService) ListActions(tenantID string) ([]models.HealthAction, error) {
	return svc.store.ListActions(tenantID)
}

// ── Pillar 1: Incident Recency (0-25 pts) ────────────────────────────────────

func scoreIncidentRecency(lastIncident *time.Time) models.HealthSignal {
	sig := models.HealthSignal{
		Name:     "Incident Activity",
		MaxScore: 25,
	}

	if lastIncident == nil {
		sig.Score = 0
		sig.Value = "No incidents recorded"
		sig.Status = models.HealthSignalPoor
		return sig
	}

	days := time.Since(*lastIncident).Hours() / 24
	switch {
	case days < 1:
		sig.Score = 25
		sig.Value = "Active today"
		sig.Status = models.HealthSignalGood
	case days < 4:
		sig.Score = 20
		sig.Value = fmt.Sprintf("%.0f days ago", days)
		sig.Status = models.HealthSignalGood
	case days < 8:
		sig.Score = 15
		sig.Value = fmt.Sprintf("%.0f days ago", days)
		sig.Status = models.HealthSignalWarning
	case days < 15:
		sig.Score = 8
		sig.Value = fmt.Sprintf("%.0f days ago", days)
		sig.Status = models.HealthSignalWarning
	case days < 31:
		sig.Score = 3
		sig.Value = fmt.Sprintf("%.0f days ago", days)
		sig.Status = models.HealthSignalPoor
	default:
		sig.Score = 0
		sig.Value = fmt.Sprintf("%.0f+ days ago", days)
		sig.Status = models.HealthSignalPoor
	}

	return sig
}

// ── Pillar 2: Integration Depth (0-25 pts) ───────────────────────────────────

func scoreIntegrations(count int) models.HealthSignal {
	sig := models.HealthSignal{
		Name:     "Integrations Connected",
		MaxScore: 25,
	}

	switch {
	case count >= 5:
		sig.Score = 25
		sig.Status = models.HealthSignalGood
	case count == 4:
		sig.Score = 20
		sig.Status = models.HealthSignalGood
	case count == 3:
		sig.Score = 15
		sig.Status = models.HealthSignalGood
	case count == 2:
		sig.Score = 10
		sig.Status = models.HealthSignalWarning
	case count == 1:
		sig.Score = 5
		sig.Status = models.HealthSignalWarning
	default:
		sig.Score = 0
		sig.Status = models.HealthSignalPoor
	}

	sig.Value = fmt.Sprintf("%d/%d integrations", count, 5)
	return sig
}

// ── Pillar 3: Alert Volume Trend (0-25 pts) ──────────────────────────────────

func scoreAlertTrend(thisWeek, lastWeek int) models.HealthSignal {
	sig := models.HealthSignal{
		Name:     "Alert Volume Trend",
		MaxScore: 25,
	}

	if thisWeek == 0 && lastWeek == 0 {
		sig.Score = 0
		sig.Value = "No alert activity"
		sig.Status = models.HealthSignalPoor
		return sig
	}

	var changePct float64
	if lastWeek == 0 {
		// New this week with no prior baseline — treat as positive signal.
		changePct = 100
	} else {
		changePct = float64(thisWeek-lastWeek) / float64(lastWeek) * 100
	}

	switch {
	case changePct >= 20:
		sig.Score = 25
		sig.Value = fmt.Sprintf("+%.0f%% this week", changePct)
		sig.Status = models.HealthSignalGood
	case changePct >= 0:
		sig.Score = 18
		sig.Value = fmt.Sprintf("+%.0f%% this week", changePct)
		sig.Status = models.HealthSignalGood
	case changePct >= -20:
		sig.Score = 12
		sig.Value = fmt.Sprintf("%.0f%% this week", changePct)
		sig.Status = models.HealthSignalWarning
	case changePct >= -40:
		sig.Score = 5
		sig.Value = fmt.Sprintf("%.0f%% this week", changePct)
		sig.Status = models.HealthSignalWarning
	default:
		sig.Score = 0
		sig.Value = fmt.Sprintf("%.0f%% this week (possible churn signal)", changePct)
		sig.Status = models.HealthSignalPoor
	}

	return sig
}

// ── Pillar 4: AI Feature Adoption (0-25 pts) ─────────────────────────────────
// Sub-signals: auto-remediation (10 pts), RCA/explain (8 pts), copilot (7 pts)

func scoreAIAdoption(remediations, explains, copilots int) models.HealthSignal {
	sig := models.HealthSignal{
		Name:     "AI Feature Adoption",
		MaxScore: 25,
	}

	var score int
	var parts []string

	// Auto-remediation (0-10 pts)
	switch {
	case remediations >= 3:
		score += 10
		parts = append(parts, "automation active")
	case remediations >= 1:
		score += 6
		parts = append(parts, "automation used")
	}

	// RCA / explain (0-8 pts)
	switch {
	case explains >= 3:
		score += 8
		parts = append(parts, "RCA used")
	case explains >= 1:
		score += 4
		parts = append(parts, "RCA used once")
	}

	// Copilot (0-7 pts)
	switch {
	case copilots >= 3:
		score += 7
		parts = append(parts, "copilot active")
	case copilots >= 1:
		score += 3
		parts = append(parts, "copilot used once")
	}

	sig.Score = score
	if len(parts) == 0 {
		sig.Value = "No AI features used in last 14 days"
		sig.Status = models.HealthSignalPoor
	} else {
		sig.Value = joinParts(parts)
		if score >= 18 {
			sig.Status = models.HealthSignalGood
		} else {
			sig.Status = models.HealthSignalWarning
		}
	}

	return sig
}

// ── Churn Risk ────────────────────────────────────────────────────────────────

func churnRiskFromScore(score int) models.ChurnRisk {
	switch {
	case score >= 76:
		return models.ChurnRiskLow
	case score >= 51:
		return models.ChurnRiskMedium
	case score >= 26:
		return models.ChurnRiskHigh
	default:
		return models.ChurnRiskCritical
	}
}

// ── Recommended Actions ───────────────────────────────────────────────────────

func recommendActions(
	incident, integration, trend, ai models.HealthSignal,
	total int,
) []models.RecommendedAction {
	var actions []models.RecommendedAction

	priority := func(score, max int) string {
		if float64(score)/float64(max) < 0.4 {
			return "urgent"
		}
		return "normal"
	}

	if integration.Score <= 10 {
		actions = append(actions, models.RecommendedAction{
			Type:     "call",
			Priority: priority(integration.Score, integration.MaxScore),
			Label:    "Schedule integration workshop call",
			Reason:   fmt.Sprintf("Only %s — low integration count reduces stickiness", integration.Value),
		})
	}

	if ai.Score <= 8 {
		actions = append(actions, models.RecommendedAction{
			Type:     "email",
			Priority: priority(ai.Score, ai.MaxScore),
			Label:    "Send AI features getting-started guide",
			Reason:   "AI features (RCA, copilot, automation) not yet adopted — key differentiator for retention",
		})
	}

	if trend.Score <= 5 {
		actions = append(actions, models.RecommendedAction{
			Type:     "review",
			Priority: priority(trend.Score, trend.MaxScore),
			Label:    "Review alert routing configuration with customer",
			Reason:   fmt.Sprintf("Alert volume: %s — significant decline is an early churn signal", trend.Value),
		})
	}

	if incident.Score <= 3 {
		actions = append(actions, models.RecommendedAction{
			Type:     "call",
			Priority: priority(incident.Score, incident.MaxScore),
			Label:    "Check-in call: confirm team is still using the platform",
			Reason:   fmt.Sprintf("Last incident: %s — low engagement is a retention risk", incident.Value),
		})
	}

	if total <= 25 {
		actions = append(actions, models.RecommendedAction{
			Type:     "call",
			Priority: "urgent",
			Label:    "Schedule urgent CSM call — critical churn risk",
			Reason:   fmt.Sprintf("Health score %d/100 is critically low across all dimensions", total),
		})
	}

	return actions
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func joinParts(parts []string) string {
	if len(parts) == 0 {
		return ""
	}
	out := parts[0]
	for _, p := range parts[1:] {
		out += ", " + p
	}
	return out
}
