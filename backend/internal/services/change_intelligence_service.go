package services

// change_intelligence_service.go — Phase 4 Change Intelligence
//
// Assembles a ChangeIntelligenceReport for any incident by combining:
//  1. Deployment & commit correlation   — what was deployed before the incident
//  2. Feature flag correlation          — what flags were toggled
//  3. Config drift detection            — what config values deviated from baseline
//  4. Infra change overlay              — infra/rollback changes in the window
//  5. "What changed first" timeline     — chronological causality view, scored

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"ai-incident-platform/backend/internal/models"
	"ai-incident-platform/backend/internal/store"
)

const (
	changeWindowBefore = 60 * time.Minute // look 60 min before incident start
	changeWindowAfter  = 15 * time.Minute // look 15 min after (late-detected changes)
)

// ChangeIntelligenceService builds change context for incidents.
type ChangeIntelligenceService struct {
	ciStore       *store.ChangeIntelligenceStore
	incidentStore *store.IncidentStore
}

// NewChangeIntelligenceService creates a new ChangeIntelligenceService.
func NewChangeIntelligenceService(
	ciStore *store.ChangeIntelligenceStore,
	is *store.IncidentStore,
) *ChangeIntelligenceService {
	return &ChangeIntelligenceService{
		ciStore:       ciStore,
		incidentStore: is,
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Public API
// ─────────────────────────────────────────────────────────────────────────────

// BuildReport assembles a full ChangeIntelligenceReport for an incident.
func (s *ChangeIntelligenceService) BuildReport(incident models.Incident) (*models.ChangeIntelligenceReport, error) {
	anchor := incident.FirstEventTime
	if anchor.IsZero() {
		anchor = time.Now()
	}

	windowStart := anchor.Add(-changeWindowBefore)
	windowEnd := anchor.Add(changeWindowAfter)
	tenantID := incident.TenantID
	if tenantID == "" {
		tenantID = "default"
	}

	// Load all change types from DB.
	allChanges, err := s.ciStore.GetChangesInWindow(tenantID, windowStart, windowEnd)
	if err != nil {
		allChanges = []models.ChangeEvent{}
	}

	flags, err := s.ciStore.GetRecentFlagChanges(tenantID, windowStart, windowEnd)
	if err != nil {
		flags = []models.FeatureFlagChange{}
	}

	drift, err := s.ciStore.GetRecentDrift(tenantID, windowStart, windowEnd)
	if err != nil {
		drift = []models.ConfigDriftEvent{}
	}

	// Partition changes by type.
	var deployments, infraChanges []models.ChangeEvent
	for _, c := range allChanges {
		switch strings.ToLower(c.Type) {
		case "deployment", "commit", "release", "rollback":
			deployments = append(deployments, c)
		case "infra", "config_change", "config", "terraform":
			infraChanges = append(infraChanges, c)
		}
	}

	// Build causality timeline.
	timeline := s.buildCausalityTimeline(allChanges, flags, drift, anchor, incident)

	// Find first change and primary correlation.
	var firstChange *models.ChangeTimelineEntry
	var primaryCorr *models.ChangeEvent
	bestScore := -1

	for i := range timeline {
		if timeline[i].IsFirstChange {
			firstChange = &timeline[i]
		}
		if timeline[i].IsPrimaryCorr {
			// Find the original change event for more detail.
			for j := range deployments {
				if fmt.Sprintf("%d", deployments[j].ID) == timeline[i].ChangeID {
					primaryCorr = &deployments[j]
					primaryCorr.CorrelationScore = timeline[i].CorrelationScore
				}
			}
		}
		if timeline[i].CorrelationScore > bestScore {
			bestScore = timeline[i].CorrelationScore
		}
	}

	// Persist correlation scores back to the DB (best-effort, no error propagation).
	for _, entry := range timeline {
		if entry.CorrelationScore < 30 {
			continue
		}
		// Only update changes with known numeric IDs.
		var id int
		fmt.Sscanf(entry.ChangeID, "%d", &id)
		if id > 0 && entry.EntryType != "feature_flag" && entry.EntryType != "config_drift" {
			_ = s.ciStore.UpdateCorrelation(id, incident.ID, entry.CorrelationScore)
		}
	}

	overallConf := computeOverallChangeConfidence(timeline, len(flags), len(drift))

	return &models.ChangeIntelligenceReport{
		IncidentID:         incident.ID,
		Service:            incident.Service,
		WindowStart:        windowStart,
		WindowEnd:          windowEnd,
		Deployments:        coalesceChanges(deployments),
		FeatureFlagChanges: coalesceFlags(flags),
		ConfigDrift:        coalesceConfigDrift(drift),
		InfraChanges:       coalesceChanges(infraChanges),
		CausalityTimeline:  timeline,
		FirstChange:        firstChange,
		PrimaryCorrelation: primaryCorr,
		OverallConfidence:  overallConf,
		GeneratedAt:        time.Now().Format(time.RFC3339),
	}, nil
}

// IngestFeatureFlag stores a feature flag change and links it to any open incident.
func (s *ChangeIntelligenceService) IngestFeatureFlag(tenantID string, f models.FeatureFlagChange) (*models.FeatureFlagChange, error) {
	if f.TenantID == "" {
		f.TenantID = tenantID
	}
	if f.Timestamp.IsZero() {
		f.Timestamp = time.Now()
	}
	if f.Environment == "" {
		f.Environment = "production"
	}
	if f.AffectedPct == 0 {
		f.AffectedPct = 100
	}

	// Auto-link to any open incident for a service whose name matches the flag key.
	if f.FlagKey != "" {
		svc := flagKeyToService(f.FlagKey)
		if incident := s.incidentStore.FindOpenIncidentForService(f.TenantID, svc, f.Timestamp.Add(-30*time.Minute)); incident != nil {
			score := scoreFlagCorrelation(f, *incident)
			f.LinkedIncidentID = incident.ID
			f.CorrelationScore = score
		}
	}

	id, err := s.ciStore.AddFeatureFlagChange(f)
	if err != nil {
		return nil, fmt.Errorf("change_intel: ingest flag: %w", err)
	}
	f.ID = id
	return &f, nil
}

// DetectConfigDrift compares currentConfig against stored baselines for a service,
// records drift events, and returns what drifted.
func (s *ChangeIntelligenceService) DetectConfigDrift(tenantID, service string, currentConfig map[string]string) ([]models.ConfigDriftEvent, error) {
	return s.ciStore.DetectAndRecordDrift(tenantID, service, currentConfig)
}

// GetRecentChanges returns enriched change events in a time window for the dashboard.
func (s *ChangeIntelligenceService) GetRecentChanges(tenantID string, since time.Time) ([]models.ChangeEvent, error) {
	return s.ciStore.GetRecentChanges(tenantID, since, time.Now(), 200)
}

// ─────────────────────────────────────────────────────────────────────────────
// Causality timeline construction
// ─────────────────────────────────────────────────────────────────────────────

func (s *ChangeIntelligenceService) buildCausalityTimeline(
	changes []models.ChangeEvent,
	flags []models.FeatureFlagChange,
	drift []models.ConfigDriftEvent,
	incidentAnchor time.Time,
	incident models.Incident,
) []models.ChangeTimelineEntry {
	var entries []models.ChangeTimelineEntry

	for _, c := range changes {
		lag := c.Timestamp.Sub(incidentAnchor)
		score := scoreChangeCorrelation(c, incident, lag)
		entries = append(entries, models.ChangeTimelineEntry{
			EntryType:        strings.ToLower(c.Type),
			ChangeID:         fmt.Sprintf("%d", c.ID),
			Service:          c.Service,
			Environment:      c.Environment,
			Title:            changeTitle(c),
			Description:      c.Description,
			Author:           c.Author,
			Version:          c.Version,
			Timestamp:        c.Timestamp,
			CorrelationScore: score,
			LagFromIncident:  formatLag(lag),
			LagSeconds:       int(lag.Seconds()),
			Metadata:         c.Metadata,
		})
	}

	for _, f := range flags {
		lag := f.Timestamp.Sub(incidentAnchor)
		score := scoreFlagCorrelation(f, incident)
		meta := map[string]string{
			"old_value":    f.OldValue,
			"new_value":    f.NewValue,
			"affected_pct": fmt.Sprintf("%d%%", f.AffectedPct),
		}
		entries = append(entries, models.ChangeTimelineEntry{
			EntryType:        "feature_flag",
			ChangeID:         fmt.Sprintf("ff-%d", f.ID),
			Service:          flagKeyToService(f.FlagKey),
			Environment:      f.Environment,
			Title:            fmt.Sprintf("Flag: %s → %s", f.FlagName, f.NewValue),
			Description:      fmt.Sprintf("Changed by %s; affects %d%% traffic", f.ChangedBy, f.AffectedPct),
			Author:           f.ChangedBy,
			Timestamp:        f.Timestamp,
			CorrelationScore: score,
			LagFromIncident:  formatLag(lag),
			LagSeconds:       int(lag.Seconds()),
			Metadata:         meta,
		})
	}

	for _, d := range drift {
		lag := d.DetectedAt.Sub(incidentAnchor)
		score := scoreDriftCorrelation(d, incident, lag)
		entries = append(entries, models.ChangeTimelineEntry{
			EntryType:        "config_drift",
			ChangeID:         fmt.Sprintf("cd-%d", d.ID),
			Service:          d.Service,
			Title:            fmt.Sprintf("Config drift: %s", d.ConfigKey),
			Description:      fmt.Sprintf("%s → %s", d.BaselineValue, d.CurrentValue),
			Timestamp:        d.DetectedAt,
			CorrelationScore: score,
			LagFromIncident:  formatLag(lag),
			LagSeconds:       int(lag.Seconds()),
			Metadata:         map[string]string{"severity": d.DriftSeverity},
		})
	}

	// Sort chronologically (ascending).
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Timestamp.Before(entries[j].Timestamp)
	})

	// Mark first change (earliest entry before incident start).
	var firstIdx = -1
	for i, e := range entries {
		if e.LagSeconds <= 0 { // before or at incident start
			firstIdx = i
			break
		}
	}
	if firstIdx >= 0 {
		entries[firstIdx].IsFirstChange = true
	}

	// Mark primary correlation (highest score, before incident).
	bestScore, bestIdx := -1, -1
	for i, e := range entries {
		if e.LagSeconds <= 0 && e.CorrelationScore > bestScore {
			bestScore = e.CorrelationScore
			bestIdx = i
		}
	}
	if bestIdx >= 0 {
		entries[bestIdx].IsPrimaryCorr = true
	}

	return entries
}

// ─────────────────────────────────────────────────────────────────────────────
// Correlation scoring
// ─────────────────────────────────────────────────────────────────────────────

// scoreChangeCorrelation returns 0-100 for a change-to-incident correlation.
func scoreChangeCorrelation(c models.ChangeEvent, incident models.Incident, lag time.Duration) int {
	score := 0

	// Temporal proximity (max 50): only changes before the incident count fully.
	absSec := math.Abs(lag.Seconds())
	switch {
	case lag <= 0 && absSec <= 120:   // within 2 min before
		score += 50
	case lag <= 0 && absSec <= 300:   // 2-5 min before
		score += 45
	case lag <= 0 && absSec <= 600:   // 5-10 min before
		score += 38
	case lag <= 0 && absSec <= 900:   // 10-15 min before
		score += 28
	case lag <= 0 && absSec <= 1800:  // 15-30 min before
		score += 18
	case lag <= 0 && absSec <= 3600:  // 30-60 min before
		score += 8
	case lag > 0 && lag <= 5*time.Minute: // just after — may be a related deploy
		score += 5
	}

	// Service match (max 25).
	incSvc := strings.ToLower(strings.TrimSpace(incident.Service))
	chSvc := strings.ToLower(strings.TrimSpace(c.Service))
	switch {
	case strings.EqualFold(incSvc, chSvc):
		score += 25
	case strings.HasPrefix(incSvc, chSvc) || strings.HasPrefix(chSvc, incSvc):
		score += 15
	case strings.Contains(incSvc, chSvc) || strings.Contains(chSvc, incSvc):
		score += 8
	}

	// Change type risk weight (max 15).
	switch strings.ToLower(c.Type) {
	case "deployment":
		score += 15
	case "rollback":
		score += 14 // rollbacks often follow broken deploys
	case "release":
		score += 12
	case "infra", "terraform":
		score += 10
	case "config_change", "config":
		score += 9
	case "commit":
		score += 6
	}

	// Environment boost (max 10): production changes are higher risk.
	if strings.ToLower(c.Environment) == "production" || strings.ToLower(c.Environment) == "prod" {
		score += 10
	}

	if score > 100 {
		score = 100
	}
	return score
}

func scoreFlagCorrelation(f models.FeatureFlagChange, incident models.Incident) int {
	score := 0
	lag := f.Timestamp.Sub(incident.FirstEventTime)
	absSec := math.Abs(lag.Seconds())

	// Temporal proximity (max 50).
	if lag <= 0 {
		switch {
		case absSec <= 300:
			score += 50
		case absSec <= 900:
			score += 38
		case absSec <= 1800:
			score += 22
		case absSec <= 3600:
			score += 10
		}
	}

	// Flag impact (affected_pct).
	if f.AffectedPct >= 100 {
		score += 20
	} else if f.AffectedPct >= 50 {
		score += 14
	} else if f.AffectedPct >= 10 {
		score += 8
	}

	// Environment match.
	if strings.ToLower(f.Environment) == "production" {
		score += 10
	}

	// Enable/disable change is riskier than value change.
	if (f.OldValue == "false" && f.NewValue == "true") ||
		(f.OldValue == "true" && f.NewValue == "false") {
		score += 10
	}

	if f.AffectedPct > 0 {
		score += 10 // has explicit impact data
	}

	if score > 100 {
		score = 100
	}
	return score
}

func scoreDriftCorrelation(d models.ConfigDriftEvent, incident models.Incident, lag time.Duration) int {
	score := 0
	absSec := math.Abs(lag.Seconds())

	// Temporal proximity (max 45).
	if lag <= 0 {
		switch {
		case absSec <= 600:
			score += 45
		case absSec <= 1800:
			score += 28
		case absSec <= 3600:
			score += 12
		}
	}

	// Service match (max 25).
	if strings.EqualFold(d.Service, incident.Service) {
		score += 25
	} else if strings.Contains(strings.ToLower(incident.Service), strings.ToLower(d.Service)) {
		score += 12
	}

	// Severity weight (max 20).
	switch d.DriftSeverity {
	case "critical":
		score += 20
	case "high":
		score += 14
	case "medium":
		score += 8
	case "low":
		score += 3
	}

	if score > 100 {
		score = 100
	}
	return score
}

// ─────────────────────────────────────────────────────────────────────────────
// Helpers
// ─────────────────────────────────────────────────────────────────────────────

func computeOverallChangeConfidence(timeline []models.ChangeTimelineEntry, flagCount, driftCount int) int {
	if len(timeline) == 0 {
		return 0
	}
	bestScore := 0
	for _, e := range timeline {
		if e.CorrelationScore > bestScore {
			bestScore = e.CorrelationScore
		}
	}
	conf := bestScore
	if flagCount > 0 {
		conf = min(100, conf+5)
	}
	if driftCount > 0 {
		conf = min(100, conf+5)
	}
	return conf
}

func formatLag(d time.Duration) string {
	neg := d < 0
	if neg {
		d = -d
	}
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	s := int(d.Seconds()) % 60

	var sb strings.Builder
	if neg {
		sb.WriteString("-")
	} else {
		sb.WriteString("+")
	}
	if h > 0 {
		sb.WriteString(fmt.Sprintf("%dh", h))
	}
	if m > 0 {
		sb.WriteString(fmt.Sprintf("%dm", m))
	}
	if s > 0 || (h == 0 && m == 0) {
		sb.WriteString(fmt.Sprintf("%ds", s))
	}
	return sb.String()
}

func changeTitle(c models.ChangeEvent) string {
	svc := c.Service
	if svc == "" {
		svc = "unknown"
	}
	switch strings.ToLower(c.Type) {
	case "deployment":
		if c.Version != "" {
			return fmt.Sprintf("Deployed %s@%s", svc, c.Version)
		}
		return fmt.Sprintf("Deployment: %s", svc)
	case "rollback":
		return fmt.Sprintf("Rollback: %s → %s", svc, c.Version)
	case "release":
		return fmt.Sprintf("Release %s (%s)", c.Version, svc)
	case "commit":
		if c.CommitSHA != "" {
			return fmt.Sprintf("Commit %s on %s", c.CommitSHA[:min(len(c.CommitSHA), 8)], svc)
		}
		return fmt.Sprintf("Commit on %s", svc)
	case "infra", "terraform":
		return fmt.Sprintf("Infra change: %s", svc)
	case "config_change", "config":
		return fmt.Sprintf("Config change: %s", svc)
	default:
		t := c.Type
		if len(t) > 0 {
			t = strings.ToUpper(t[:1]) + strings.ToLower(t[1:])
		}
		return fmt.Sprintf("%s: %s", t, svc)
	}
}

func flagKeyToService(key string) string {
	if key == "" {
		return "unknown"
	}
	parts := strings.SplitN(key, ".", 2)
	return parts[0]
}

// coalesceChanges returns an empty slice instead of nil.
func coalesceChanges(cc []models.ChangeEvent) []models.ChangeEvent {
	if cc == nil {
		return []models.ChangeEvent{}
	}
	return cc
}

func coalesceFlags(ff []models.FeatureFlagChange) []models.FeatureFlagChange {
	if ff == nil {
		return []models.FeatureFlagChange{}
	}
	return ff
}

func coalesceConfigDrift(dd []models.ConfigDriftEvent) []models.ConfigDriftEvent {
	if dd == nil {
		return []models.ConfigDriftEvent{}
	}
	return dd
}
