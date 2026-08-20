package services

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"ai-incident-platform/backend/internal/models"
	"ai-incident-platform/backend/internal/store"
)

type BusinessImpactService struct {
	store         *store.BusinessImpactStore
	incidentStore *store.IncidentStore
	tenantCfgSvc  *TenantConfigService // optional, nil-safe
}

func NewBusinessImpactService(bis *store.BusinessImpactStore, is *store.IncidentStore) *BusinessImpactService {
	return &BusinessImpactService{
		store:         bis,
		incidentStore: is,
	}
}

// SetTenantConfigService wires in the config-layer resolver for runtime lookups.
func (s *BusinessImpactService) SetTenantConfigService(svc *TenantConfigService) {
	s.tenantCfgSvc = svc
}

// ── Impact calculation ───────────────────────────────────────────────────────

// CalculateImpact computes (or recomputes) the financial impact for an incident.
func (s *BusinessImpactService) CalculateImpact(incidentID string) (*models.BusinessImpactResponse, error) {
	incident, found := s.incidentStore.GetIncidentByID(incidentID)
	if !found {
		return nil, fmt.Errorf("incident not found: %s", incidentID)
	}

	// 1. Resolve profile via config-layer precedence chain
	profile, err := s.resolveProfile(incident.TenantID, incident.Service)
	if err != nil {
		return nil, fmt.Errorf("resolve profile: %w", err)
	}
	profileIsDefault := false
	if profile == nil {
		profile = s.defaultProfile(incident.Service)
		profileIsDefault = true
	}

	// 2. Resolve baseline via config-layer
	baseline, err := s.resolveBaseline(incident.TenantID, incident.Service, incident.RootCauseType)
	if err != nil {
		return nil, fmt.Errorf("resolve baseline: %w", err)
	}

	// 2b. Resolve tenant settings (for custom multipliers)
	settings, _ := s.resolveSettings(incident.TenantID)

	// 3. Compute durations
	actualMinutes := s.computeActualMinutes(incident)

	impactPct := severityToImpactPct(incident.Severity)

	// 4. Actual loss
	actualHours := actualMinutes / 60.0
	revenueLoss := profile.HourlyRevenue * impactPct * actualHours * profile.BusinessHourMultiplier * profile.PeakMultiplier

	var slaPenalty float64
	if actualMinutes > float64(profile.SLAThresholdMinutes) {
		slaPenalty = (actualMinutes - float64(profile.SLAThresholdMinutes)) * profile.SLAPenaltyPerMinute
	}

	affectedUsers := profile.UsersPerHour * impactPct
	productivityLoss := affectedUsers * profile.EmployeeCostPerHour * actualHours
	infraLoss := profile.InfraCostPerHour * actualHours
	// Apply severity × tier multipliers (customer override → system default)
	sevMult := s.getSeverityMultiplier(incident.Severity, settings)
	tierMult := s.getTierMultiplier(profile.Tier, settings)
	rawActual := revenueLoss + slaPenalty + productivityLoss + infraLoss
	totalActual := rawActual * sevMult * tierMult

	// 5. Counterfactual loss (2-phase escalation model)
	counterfactualMinutes := 60.0
	methodUsed := "static_fallback"
	if baseline != nil {
		counterfactualMinutes = baseline.AvgMTTRMinutes
		methodUsed = "historical_baseline"
	}

	phase1Minutes := math.Min(0.4*counterfactualMinutes, actualMinutes)
	phase2Minutes := counterfactualMinutes - phase1Minutes

	phase1Impact := impactPct
	phase2Impact := math.Min(impactPct*1.8, 1.0)

	phase1Loss := profile.HourlyRevenue * phase1Impact * (phase1Minutes / 60.0)
	phase2Loss := profile.HourlyRevenue * phase2Impact * (phase2Minutes / 60.0)
	counterfactualRevenue := (phase1Loss + phase2Loss) * profile.BusinessHourMultiplier * profile.PeakMultiplier

	var counterfactualSLA float64
	if counterfactualMinutes > float64(profile.SLAThresholdMinutes) {
		counterfactualSLA = (counterfactualMinutes - float64(profile.SLAThresholdMinutes)) * profile.SLAPenaltyPerMinute
	}

	counterfactualHours := counterfactualMinutes / 60.0
	counterfactualProductivity := affectedUsers * profile.EmployeeCostPerHour * counterfactualHours
	counterfactualInfra := profile.InfraCostPerHour * counterfactualHours
	rawCounterfactual := counterfactualRevenue + counterfactualSLA + counterfactualProductivity + counterfactualInfra
	totalCounterfactual := rawCounterfactual * sevMult * tierMult

	// 6. Avoided loss
	avoidedLoss := math.Max(0, totalCounterfactual-totalActual)

	// 7. Confidence
	confidenceLevel := "LOW"
	confidenceScore := 0.4
	if baseline != nil && baseline.SampleSize >= 20 {
		confidenceLevel = "HIGH"
		confidenceScore = 1.0
	} else if baseline != nil && baseline.SampleSize >= 5 {
		confidenceLevel = "MEDIUM"
		confidenceScore = 0.7
	}

	// 8. Breakdown
	breakdown := models.LossBreakdown{
		ActualDurationMinutes:  round2(actualMinutes),
		ActualRevenueLoss:      round2(revenueLoss),
		ActualSLAPenalty:       round2(slaPenalty),
		ActualProductivityLoss: round2(productivityLoss),
		ActualInfraLoss:        round2(infraLoss),
		TotalActualLoss:        round2(totalActual),

		CounterfactualDurationMinutes:  round2(counterfactualMinutes),
		Phase1Minutes:                  round2(phase1Minutes),
		Phase2Minutes:                  round2(phase2Minutes),
		Phase1Impact:                   round2(phase1Impact),
		Phase2Impact:                   round2(phase2Impact),
		CounterfactualRevenueLoss:      round2(counterfactualRevenue),
		CounterfactualSLAPenalty:       round2(counterfactualSLA),
		CounterfactualProductivityLoss: round2(counterfactualProductivity),
		CounterfactualInfraLoss:        round2(counterfactualInfra),
		TotalCounterfactualLoss:        round2(totalCounterfactual),

		AvoidedLoss:        round2(avoidedLoss),
		SeverityMultiplier: sevMult,
		TierMultiplier:     tierMult,
	}

	breakdownBytes, _ := json.Marshal(breakdown)

	// 8b. Risk projections (if unresolved for +15/30/60 min)
	projections := s.computeProjections(profile, impactPct, actualMinutes, incident.Severity, settings)

	// 9. Explanation
	explanation := s.buildExplanation(incident, profile, baseline, actualMinutes, totalActual, totalCounterfactual, avoidedLoss, profileIsDefault)

	// 10. Build and save estimate
	now := time.Now()
	estimate := models.FinancialEstimate{
		ID:                 fmt.Sprintf("est-%s", incidentID),
		TenantID:           incident.TenantID,
		IncidentID:         incidentID,
		Service:            incident.Service,
		ActualLoss:         round2(totalActual),
		CounterfactualLoss: round2(totalCounterfactual),
		AvoidedLoss:        round2(avoidedLoss),
		ConfidenceLevel:    confidenceLevel,
		ConfidenceScore:    confidenceScore,
		MethodUsed:         methodUsed,
		Currency:           profile.Currency,
		BreakdownJSON:      string(breakdownBytes),
		Explanation:        explanation,
		CreatedAt:          now,
	}

	if err := s.store.SaveEstimate(estimate); err != nil {
		return nil, fmt.Errorf("save estimate: %w", err)
	}

	return &models.BusinessImpactResponse{
		Estimate:    &estimate,
		Breakdown:   &breakdown,
		Profile:     profile,
		Baseline:    baseline,
		Incident:    &incident,
		Projections: projections,
	}, nil
}

// GetImpact returns a previously computed estimate, or nil if none exists.
func (s *BusinessImpactService) GetImpact(incidentID string) (*models.BusinessImpactResponse, error) {
	estimate, err := s.store.GetEstimateByIncident(incidentID)
	if err != nil {
		return nil, err
	}
	if estimate == nil {
		return nil, nil
	}

	incident, found := s.incidentStore.GetIncidentByID(incidentID)
	if !found {
		return nil, fmt.Errorf("incident not found: %s", incidentID)
	}

	profile, err := s.resolveProfile(incident.TenantID, incident.Service)
	if err != nil {
		return nil, err
	}
	if profile == nil {
		profile = s.defaultProfile(incident.Service)
	}

	baseline, err := s.resolveBaseline(incident.TenantID, incident.Service, incident.RootCauseType)
	if err != nil {
		return nil, err
	}

	var breakdown models.LossBreakdown
	if estimate.BreakdownJSON != "" {
		_ = json.Unmarshal([]byte(estimate.BreakdownJSON), &breakdown)
	}

	actualMinutes := s.computeActualMinutes(incident)
	impactPct := severityToImpactPct(incident.Severity)
	settings, _ := s.resolveSettings(incident.TenantID)
	projections := s.computeProjections(profile, impactPct, actualMinutes, incident.Severity, settings)

	return &models.BusinessImpactResponse{
		Estimate:    estimate,
		Breakdown:   &breakdown,
		Profile:     profile,
		Baseline:    baseline,
		Incident:    &incident,
		Projections: projections,
	}, nil
}

// GetOrCalculate returns existing estimate or computes a new one.
func (s *BusinessImpactService) GetOrCalculate(incidentID string) (*models.BusinessImpactResponse, error) {
	resp, err := s.GetImpact(incidentID)
	if err != nil {
		return nil, err
	}
	if resp != nil {
		return resp, nil
	}
	return s.CalculateImpact(incidentID)
}

// ── Profile management ───────────────────────────────────────────────────────

func (s *BusinessImpactService) SaveProfile(p models.ServiceProfile) error {
	if p.Currency == "" {
		p.Currency = "USD"
	}
	now := time.Now()
	if p.CreatedAt.IsZero() {
		p.CreatedAt = now
	}
	p.UpdatedAt = now
	return s.store.UpsertProfile(p)
}

func (s *BusinessImpactService) GetProfiles(tenantID string) ([]models.ServiceProfile, error) {
	return s.store.GetProfiles(tenantID)
}

func (s *BusinessImpactService) DeleteProfile(id string) error {
	return s.store.DeleteProfile(id)
}

// ── Baseline management ──────────────────────────────────────────────────────

func (s *BusinessImpactService) SaveBaseline(b models.IncidentBaseline) error {
	now := time.Now()
	if b.CreatedAt.IsZero() {
		b.CreatedAt = now
	}
	b.UpdatedAt = now
	return s.store.UpsertBaseline(b)
}

func (s *BusinessImpactService) GetBaselines(tenantID string) ([]models.IncidentBaseline, error) {
	return s.store.GetBaselines(tenantID)
}

// ── Monthly rollup ───────────────────────────────────────────────────────────

func (s *BusinessImpactService) GenerateMonthlyRollup(tenantID string, year, month int) (*models.MonthlyReportResponse, error) {
	start := time.Date(year, time.Month(month), 1, 0, 0, 0, 0, time.UTC)
	end := start.AddDate(0, 1, 0)

	estimates, err := s.store.GetEstimatesForPeriod(tenantID, start, end)
	if err != nil {
		return nil, fmt.Errorf("get estimates for period: %w", err)
	}

	var totalActual, totalCounterfactual, totalAvoided, confidenceWeighted float64
	for _, e := range estimates {
		totalActual += e.ActualLoss
		totalCounterfactual += e.CounterfactualLoss
		totalAvoided += e.AvoidedLoss
		confidenceWeighted += e.AvoidedLoss * e.ConfidenceScore
	}

	// Top 5 incidents by avoided loss
	sorted := make([]models.FinancialEstimate, len(estimates))
	copy(sorted, estimates)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].AvoidedLoss > sorted[j].AvoidedLoss
	})
	topN := 5
	if len(sorted) < topN {
		topN = len(sorted)
	}
	type topIncident struct {
		IncidentID  string  `json:"incident_id"`
		AvoidedLoss float64 `json:"avoided_loss"`
		ActualLoss  float64 `json:"actual_loss"`
		Service     string  `json:"service"`
	}
	topIncidents := make([]topIncident, topN)
	for i := 0; i < topN; i++ {
		topIncidents[i] = topIncident{
			IncidentID:  sorted[i].IncidentID,
			AvoidedLoss: sorted[i].AvoidedLoss,
			ActualLoss:  sorted[i].ActualLoss,
			Service:     sorted[i].Service,
		}
	}
	topJSON, _ := json.Marshal(topIncidents)

	now := time.Now()
	rollup := models.MonthlyRollup{
		ID:                      fmt.Sprintf("rollup-%s-%d-%02d", tenantID, year, month),
		TenantID:                tenantID,
		Year:                    year,
		Month:                   month,
		TotalActualLoss:         round2(totalActual),
		TotalCounterfactualLoss: round2(totalCounterfactual),
		TotalAvoidedLoss:        round2(totalAvoided),
		ConfidenceWeightedLoss:  round2(confidenceWeighted),
		Currency:                "USD",
		TopIncidentsJSON:        string(topJSON),
		CreatedAt:               now,
		UpdatedAt:               now,
	}

	if err := s.store.UpsertRollup(rollup); err != nil {
		return nil, fmt.Errorf("save rollup: %w", err)
	}

	return &models.MonthlyReportResponse{
		Rollup:         &rollup,
		Estimates:      estimates,
		TotalIncidents: len(estimates),
	}, nil
}

// ── Config-layer resolvers ────────────────────────────────────────────────────

// resolveProfile uses the TenantConfigService chain when available, otherwise
// falls back to the direct store call.
func (s *BusinessImpactService) resolveProfile(tenantID, serviceName string) (*models.ServiceProfile, error) {
	if s.tenantCfgSvc != nil {
		return s.tenantCfgSvc.GetServiceProfile(tenantID, serviceName)
	}
	return s.store.GetProfileByService(tenantID, serviceName)
}

// resolveBaseline uses the TenantConfigService chain when available.
func (s *BusinessImpactService) resolveBaseline(tenantID, serviceName, incidentType string) (*models.IncidentBaseline, error) {
	if s.tenantCfgSvc != nil {
		return s.tenantCfgSvc.GetBaseline(tenantID, serviceName, incidentType)
	}
	return s.store.GetBaseline(tenantID, serviceName, incidentType)
}

// resolveSettings fetches per-tenant multiplier overrides.
func (s *BusinessImpactService) resolveSettings(tenantID string) (*models.TenantSettings, error) {
	if s.tenantCfgSvc != nil {
		return s.tenantCfgSvc.GetSettings(tenantID)
	}
	return nil, nil
}

// getSeverityMultiplier returns the tenant override for this severity, or system default.
func (s *BusinessImpactService) getSeverityMultiplier(severity string, settings *models.TenantSettings) float64 {
	if settings != nil && len(settings.SeverityMultipliers) > 0 {
		if m, ok := settings.SeverityMultipliers[strings.ToLower(severity)]; ok {
			return m
		}
	}
	return severityMultiplier(severity)
}

// getTierMultiplier returns the tenant override for this tier, or system default.
func (s *BusinessImpactService) getTierMultiplier(tier string, settings *models.TenantSettings) float64 {
	if settings != nil && len(settings.TierMultipliers) > 0 {
		if m, ok := settings.TierMultipliers[strings.ToUpper(tier)]; ok {
			return m
		}
	}
	return tierMultiplier(tier)
}

// ── Internal helpers ─────────────────────────────────────────────────────────

func (s *BusinessImpactService) computeActualMinutes(incident models.Incident) float64 {
	startTime := incident.FirstEventTime
	if startTime.IsZero() {
		startTime = time.Now().Add(-1 * time.Hour)
	}

	var endTime time.Time
	if incident.Status == "resolved" && !incident.LastEventTime.IsZero() {
		endTime = incident.LastEventTime
	} else {
		endTime = time.Now()
	}

	minutes := endTime.Sub(startTime).Minutes()
	if minutes < 1 {
		minutes = 1
	}
	return minutes
}

func severityToImpactPct(severity string) float64 {
	switch strings.ToLower(severity) {
	case "critical":
		return 0.8
	case "high":
		return 0.5
	case "medium":
		return 0.2
	case "low":
		return 0.05
	default:
		return 0.2
	}
}

func (s *BusinessImpactService) defaultProfile(service string) *models.ServiceProfile {
	svc := strings.ToLower(service)
	p := models.ServiceProfile{
		Service:                service,
		CostModelType:          "mixed",
		BusinessHourMultiplier: 1.0,
		PeakMultiplier:         1.0,
		ConfidenceMode:         "balanced",
		Currency:               "USD",
	}

	switch {
	case containsAny(svc, "payment", "checkout", "order"):
		p.Tier = "TIER_0"
		p.HourlyRevenue = 10000
		p.UsersPerHour = 20000
		p.SLAPenaltyPerMinute = 200
		p.SLAThresholdMinutes = 5
		p.EmployeeCostPerHour = 25
		p.InfraCostPerHour = 100
	case containsAny(svc, "api", "service", "gateway"):
		p.Tier = "TIER_1"
		p.HourlyRevenue = 2000
		p.UsersPerHour = 5000
		p.SLAPenaltyPerMinute = 50
		p.SLAThresholdMinutes = 10
		p.EmployeeCostPerHour = 25
		p.InfraCostPerHour = 50
	case containsAny(svc, "worker", "cron", "batch"):
		p.Tier = "TIER_3"
		p.HourlyRevenue = 0
		p.UsersPerHour = 100
		p.SLAPenaltyPerMinute = 0
		p.SLAThresholdMinutes = 60
		p.EmployeeCostPerHour = 25
		p.InfraCostPerHour = 10
	default:
		p.Tier = "TIER_2"
		p.HourlyRevenue = 500
		p.UsersPerHour = 1000
		p.SLAPenaltyPerMinute = 10
		p.SLAThresholdMinutes = 15
		p.EmployeeCostPerHour = 25
		p.InfraCostPerHour = 20
	}

	return &p
}

func containsAny(s string, substrs ...string) bool {
	for _, sub := range substrs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}

func (s *BusinessImpactService) buildExplanation(
	incident models.Incident,
	profile *models.ServiceProfile,
	baseline *models.IncidentBaseline,
	actualMinutes, totalActual, totalCounterfactual, avoidedLoss float64,
	profileIsDefault bool,
) string {
	var b strings.Builder

	durationStr := fmt.Sprintf("%.0f minutes", actualMinutes)
	if actualMinutes >= 120 {
		durationStr = fmt.Sprintf("%.1f hours", actualMinutes/60.0)
	}

	fmt.Fprintf(&b, "This incident on %s lasted %s.", incident.Service, durationStr)

	if profile.HourlyRevenue > 0 {
		fmt.Fprintf(&b, " Based on a $%.0f/hr revenue profile, actual loss was $%.0f.", profile.HourlyRevenue, totalActual)
	} else {
		fmt.Fprintf(&b, " Actual loss was $%.0f.", totalActual)
	}

	if baseline != nil {
		fmt.Fprintf(&b, " Without intervention (historical baseline: %.0f min avg MTTR from %d similar incidents), the loss would have been $%.0f.",
			baseline.AvgMTTRMinutes, baseline.SampleSize, totalCounterfactual)
	} else {
		fmt.Fprintf(&b, " Without intervention (estimated 60 min MTTR baseline), the loss would have been $%.0f.", totalCounterfactual)
	}

	if avoidedLoss > 0 {
		fmt.Fprintf(&b, " Your team's response saved $%.0f.", avoidedLoss)
	}

	if profileIsDefault {
		fmt.Fprintf(&b, " No business profile configured for this service. Using %s defaults. Configure a profile for accurate estimates.", profile.Tier)
	}

	return b.String()
}

func severityMultiplier(severity string) float64 {
	switch strings.ToLower(severity) {
	case "critical":
		return 1.0
	case "high":
		return 0.7 // not using older naming "major"
	case "medium":
		return 0.4
	case "low":
		return 0.2
	default:
		return 0.4
	}
}

func tierMultiplier(tier string) float64 {
	switch strings.ToUpper(tier) {
	case "TIER_0":
		return 1.5
	case "TIER_1":
		return 1.2
	case "TIER_2":
		return 1.0
	case "TIER_3":
		return 0.7
	default:
		return 1.0
	}
}

// computeProjections returns "if unresolved for X more minutes" risk projections.
func (s *BusinessImpactService) computeProjections(
	profile *models.ServiceProfile,
	impactPct float64,
	actualMinutes float64,
	severity string,
	settings *models.TenantSettings,
) []models.RiskProjection {
	projections := []models.RiskProjection{}
	sevMult := s.getSeverityMultiplier(severity, settings)
	tierMult := s.getTierMultiplier(profile.Tier, settings)

	for _, extra := range []float64{15, 30, 60} {
		totalMin := actualMinutes + extra
		totalHours := totalMin / 60.0

		escalatedImpact := math.Min(impactPct*(1.0+extra/60.0*0.5), 1.0)

		rev := profile.HourlyRevenue * escalatedImpact * totalHours * profile.BusinessHourMultiplier * profile.PeakMultiplier
		var sla float64
		if totalMin > float64(profile.SLAThresholdMinutes) {
			sla = (totalMin - float64(profile.SLAThresholdMinutes)) * profile.SLAPenaltyPerMinute
		}
		prod := profile.UsersPerHour * escalatedImpact * profile.EmployeeCostPerHour * totalHours
		infra := profile.InfraCostPerHour * totalHours

		total := (rev + sla + prod + infra) * sevMult * tierMult

		projections = append(projections, models.RiskProjection{
			MinutesExtra:  extra,
			Label:         fmt.Sprintf("+%.0f min", extra),
			EstimatedLoss: round2(total),
		})
	}

	return projections
}

// ComputeLiveImpact returns the real-time dollar breakdown for an open incident.
// engineerCount defaults to 2 when ≤ 0.  Never uses a cached estimate — always
// recomputes from the current elapsed time so the numbers tick up in real time.
func (s *BusinessImpactService) ComputeLiveImpact(incidentID string, engineerCount int) (*models.LiveBusinessImpact, error) {
	incident, found := s.incidentStore.GetIncidentByID(incidentID)
	if !found {
		return nil, fmt.Errorf("incident not found: %s", incidentID)
	}

	profile, err := s.resolveProfile(incident.TenantID, incident.Service)
	if err != nil {
		return nil, err
	}
	if profile == nil {
		profile = s.defaultProfile(incident.Service)
	}

	settings, _ := s.resolveSettings(incident.TenantID)
	durationMinutes := s.computeActualMinutes(incident)
	impactPct := severityToImpactPct(incident.Severity)
	sevMult := s.getSeverityMultiplier(incident.Severity, settings)
	tierMult := s.getTierMultiplier(profile.Tier, settings)

	// Revenue
	revenuePerMinute := profile.HourlyRevenue * impactPct *
		profile.BusinessHourMultiplier * profile.PeakMultiplier *
		sevMult * tierMult / 60.0
	totalRevenueLoss := round2(revenuePerMinute * durationMinutes)

	// Customer exposure — users per hour × impact %
	affectedSessions := int(profile.UsersPerHour * impactPct)

	// SLA exposure
	slaThreshold := float64(profile.SLAThresholdMinutes)
	slaBreachIn := slaThreshold - durationMinutes
	slaAlreadyBreached := slaBreachIn <= 0
	var slaPenaltyAccrued float64
	if slaAlreadyBreached {
		slaPenaltyAccrued = round2(-slaBreachIn * profile.SLAPenaltyPerMinute)
	}

	// Engineering cost (N engineers × elapsed hours × hourly rate)
	if engineerCount <= 0 {
		engineerCount = 2
	}
	engineeringCost := round2(float64(engineerCount) * durationMinutes / 60.0 * profile.EmployeeCostPerHour)

	// Infra + productivity
	infraLoss := round2(profile.InfraCostPerHour * durationMinutes / 60.0)
	productivityLoss := round2(profile.UsersPerHour * impactPct * profile.EmployeeCostPerHour * durationMinutes / 60.0)

	totalCost := round2(totalRevenueLoss + slaPenaltyAccrued + engineeringCost + infraLoss + productivityLoss)

	// Projections
	projections := s.computeProjections(profile, impactPct, durationMinutes, incident.Severity, settings)

	// Narrative lines — the human-readable panel shown in the UI
	lines := buildLiveNarrativeLines(incident, profile, revenuePerMinute, affectedSessions,
		slaAlreadyBreached, slaBreachIn, slaPenaltyAccrued, engineerCount, durationMinutes,
		engineeringCost, totalCost)

	return &models.LiveBusinessImpact{
		DurationMinutes:       round2(durationMinutes),
		RevenuePerMinute:      round2(revenuePerMinute),
		TotalRevenueLoss:      totalRevenueLoss,
		AffectedSessions:      affectedSessions,
		SLATierLabel:          tierToSLALabel(profile.Tier),
		SLABreachInMinutes:    round2(slaBreachIn),
		SLAAlreadyBreached:    slaAlreadyBreached,
		SLAPenaltyAccrued:     slaPenaltyAccrued,
		SLAPenaltyPerMin:      profile.SLAPenaltyPerMinute,
		EngineerCount:         engineerCount,
		EngineeringCostSoFar:  engineeringCost,
		InfraCostSoFar:        infraLoss,
		ProductivityLossSoFar: productivityLoss,
		TotalCostSoFar:        totalCost,
		Projections:           projections,
		Currency:              profile.Currency,
		ComputedAt:            time.Now(),
		NarrativeLines:        lines,
	}, nil
}

func tierToSLALabel(tier string) string {
	switch strings.ToUpper(tier) {
	case "TIER_0":
		return "Platinum"
	case "TIER_1":
		return "Gold"
	case "TIER_2":
		return "Silver"
	case "TIER_3":
		return "Bronze"
	default:
		return "Standard"
	}
}

func buildLiveNarrativeLines(
	incident models.Incident,
	profile *models.ServiceProfile,
	revenuePerMin float64,
	sessions int,
	breached bool,
	breachIn, penaltyAccrued float64,
	engineers int,
	durationMin, engCost, totalCost float64,
) []string {
	lines := []string{
		fmt.Sprintf("Incident: %s %s %s", incident.Service, incident.Severity, incident.RootCauseType),
	}
	if revenuePerMin > 0 {
		lines = append(lines, fmt.Sprintf("Business Impact: $%.0f/minute revenue loss", revenuePerMin))
	}
	if sessions > 0 {
		lines = append(lines, fmt.Sprintf("Affected customers: ~%d active sessions", sessions))
	}
	if breached {
		lines = append(lines, fmt.Sprintf(
			"SLA exposure: %s SLA breached %.0f min ago ($%.0f penalty accrued @ $%.0f/min)",
			tierToSLALabel(profile.Tier), -breachIn, penaltyAccrued, profile.SLAPenaltyPerMinute,
		))
	} else {
		lines = append(lines, fmt.Sprintf(
			"SLA exposure: Breaching %s SLA ($%.0f/min penalty) in %.0f minutes",
			tierToSLALabel(profile.Tier), profile.SLAPenaltyPerMinute, breachIn,
		))
	}
	lines = append(lines, fmt.Sprintf(
		"Engineering cost so far: %d engineers × %.0f minutes = $%.0f",
		engineers, durationMin, engCost,
	))
	lines = append(lines, fmt.Sprintf("Total current cost: $%.0f", totalCost))
	return lines
}

// AutoGenerateBaseline computes baseline from resolved incidents for a service.
func (s *BusinessImpactService) AutoGenerateBaseline(tenantID, service string) (*models.IncidentBaseline, error) {
	incidents, err := s.incidentStore.GetIncidents()
	if err != nil {
		return nil, err
	}

	var durations []float64
	for _, inc := range incidents {
		if inc.Status != "resolved" || !strings.EqualFold(inc.Service, service) {
			continue
		}
		if inc.TenantID != tenantID && tenantID != "" {
			continue
		}
		startT := inc.FirstEventTime
		endT := inc.LastEventTime
		if startT.IsZero() || endT.IsZero() {
			continue
		}
		dur := endT.Sub(startT).Minutes()
		if dur > 0 {
			durations = append(durations, dur)
		}
	}

	if len(durations) == 0 {
		return nil, nil
	}

	sort.Float64s(durations)

	// Compute stats
	var sum float64
	for _, d := range durations {
		sum += d
	}
	avg := sum / float64(len(durations))

	var median float64
	n := len(durations)
	if n%2 == 0 {
		median = (durations[n/2-1] + durations[n/2]) / 2
	} else {
		median = durations[n/2]
	}

	p90Idx := int(math.Ceil(float64(n)*0.9)) - 1
	if p90Idx < 0 {
		p90Idx = 0
	}
	if p90Idx >= n {
		p90Idx = n - 1
	}
	p90 := durations[p90Idx]

	baseline := models.IncidentBaseline{
		ID:                fmt.Sprintf("bl-%s-%d", service, time.Now().UnixNano()),
		TenantID:          tenantID,
		Service:           service,
		IncidentType:      "auto",
		AvgMTTRMinutes:    round2(avg),
		MedianMTTRMinutes: round2(median),
		P90MTTRMinutes:    round2(p90),
		SampleSize:        n,
		CreatedAt:         time.Now(),
		UpdatedAt:         time.Now(),
	}

	if err := s.store.UpsertBaseline(baseline); err != nil {
		return nil, err
	}

	return &baseline, nil
}

func round2(v float64) float64 {
	return math.Round(v*100) / 100
}
