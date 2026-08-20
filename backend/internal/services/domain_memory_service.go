package services

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"ai-incident-platform/backend/internal/llm"
	"ai-incident-platform/backend/internal/models"
	"ai-incident-platform/backend/internal/store"
)

// DomainMemoryService is the intelligence layer for Feature 9.
// It orchestrates the three memory pillars (remediations, deploy signatures, runbook
// preferences) together with the existing resolution history to produce a full
// "what do we know about this incident" context.
type DomainMemoryService struct {
	domainStore   *store.DomainMemoryStore
	memoryStore   *store.IncidentMemoryStore
	incidentStore *store.IncidentStore
	llmClient     *llm.Client
}

func NewDomainMemoryService(
	ds *store.DomainMemoryStore,
	ms *store.IncidentMemoryStore,
	is *store.IncidentStore,
	lc *llm.Client,
) *DomainMemoryService {
	return &DomainMemoryService{domainStore: ds, memoryStore: ms, incidentStore: is, llmClient: lc}
}

// ── Full memory context ───────────────────────────────────────────────────────

// GetFullMemoryContext assembles all four memory pillars for a given incident.
func (s *DomainMemoryService) GetFullMemoryContext(incidentID, tenantID string) (*models.FullMemoryContext, error) {
	incident, found := s.incidentStore.GetIncidentByID(incidentID)
	if !found {
		return nil, fmt.Errorf("incident not found: %s", incidentID)
	}

	// Pillar 1: past incidents for this fingerprint/service
	pastIncidents, _ := s.memoryStore.GetPatternHistory(tenantID, incident.Fingerprint, incident.Service, 8)
	if pastIncidents == nil {
		pastIncidents = []models.ResolutionRecord{}
	}
	recurrenceCount := s.memoryStore.CountResolutions(tenantID, incident.Fingerprint, incident.Service)

	// Derive a simple frequency-based playbook from past actions
	suggestedPlaybook := buildPlaybook(pastIncidents)

	// Pillar 2: remediation patterns for this service
	remediations, _ := s.domainStore.FindRemediationsByService(tenantID, incident.Service, 5)
	if remediations == nil {
		remediations = []models.RemediationPattern{}
	}

	// Pillar 3: deploy signatures for this service
	deployRisks, _ := s.domainStore.FindDeploySignaturesByService(tenantID, incident.Service, 5)
	if deployRisks == nil {
		deployRisks = []models.DeploySignature{}
	}

	// Pillar 4: team runbook preferences for this service
	prefs, _ := s.domainStore.ListRunbookPreferences(tenantID, "", incident.Service)
	if prefs == nil {
		prefs = []models.RunbookPreference{}
	}

	// LLM narrative: synthesise "last time this happened, here's what to do"
	narrative := s.buildNarrative(incident, pastIncidents, remediations, deployRisks, prefs)

	return &models.FullMemoryContext{
		IncidentID:             incidentID,
		Service:                incident.Service,
		SimilarPastIncidents:   pastIncidents,
		RecurrenceCount:        recurrenceCount,
		SuggestedPlaybook:      suggestedPlaybook,
		RemediationSuggestions: remediations,
		DeployRisks:            deployRisks,
		RunbookPreferences:     prefs,
		LLMNarrative:           narrative,
		QueriedAt:              time.Now().Format(time.RFC3339),
	}, nil
}

// buildNarrative attempts LLM synthesis, falls back to a rule-based summary.
func (s *DomainMemoryService) buildNarrative(
	incident models.Incident,
	past []models.ResolutionRecord,
	remediations []models.RemediationPattern,
	deployRisks []models.DeploySignature,
	prefs []models.RunbookPreference,
) string {
	if s.llmClient != nil && s.llmClient.IsConfigured() && (len(past) > 0 || len(remediations) > 0) {
		if n, err := s.llmNarrative(incident, past, remediations, deployRisks, prefs); err == nil {
			return n
		}
	}
	return s.ruleBasedNarrative(incident, past, remediations, deployRisks, prefs)
}

func (s *DomainMemoryService) llmNarrative(
	incident models.Incident,
	past []models.ResolutionRecord,
	remediations []models.RemediationPattern,
	deployRisks []models.DeploySignature,
	prefs []models.RunbookPreference,
) (string, error) {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Incident: %s | Service: %s | Severity: %s\n\n",
		incident.Title, incident.Service, incident.Severity))

	if len(past) > 0 {
		sb.WriteString("Past resolutions (most recent first):\n")
		for i, r := range past {
			if i >= 3 {
				break
			}
			sb.WriteString(fmt.Sprintf("- %s: TTR %ds, note: %q, actions: %v\n",
				r.ResolvedAt, r.TTRSeconds, r.ResolutionNote, r.ActionsTaken))
		}
	}
	if len(remediations) > 0 {
		sb.WriteString("\nProven remediation patterns:\n")
		for i, r := range remediations {
			if i >= 3 {
				break
			}
			sb.WriteString(fmt.Sprintf("- [%s] %s (success rate: %.0f%%): %s\n",
				r.RootCauseCategory, r.RemediationSummary, r.SuccessRate*100, strings.Join(r.RemediationSteps, "; ")))
		}
	}
	if len(deployRisks) > 0 {
		sb.WriteString("\nKnown deploy risk signals:\n")
		for i, d := range deployRisks {
			if i >= 2 {
				break
			}
			sb.WriteString(fmt.Sprintf("- %s (%d occurrences): %s\n", d.SignatureName, d.OccurrenceCount, d.Description))
		}
	}
	if len(prefs) > 0 {
		sb.WriteString("\nTeam preferences:\n")
		for i, p := range prefs {
			if i >= 3 {
				break
			}
			sb.WriteString(fmt.Sprintf("- [%s] %s: %s\n", p.TeamName, p.PreferenceKey, p.PreferenceValue))
		}
	}

	systemPrompt := `You are an SRE knowledge engine. Given an incident's history, proven remediations, deploy risk signals, and team preferences, produce a concise 2-3 sentence operational narrative in plain English. Focus on: what caused this before, what fixed it, and the single most important first action to take right now. Do not use bullet points — output continuous prose only.`

	return s.llmClient.CompleteWithSystem(systemPrompt, sb.String())
}

func (s *DomainMemoryService) ruleBasedNarrative(
	incident models.Incident,
	past []models.ResolutionRecord,
	remediations []models.RemediationPattern,
	deployRisks []models.DeploySignature,
	prefs []models.RunbookPreference,
) string {
	var parts []string

	if len(past) == 0 && len(remediations) == 0 {
		return fmt.Sprintf("No prior memory for service %q — this may be a new class of incident.", incident.Service)
	}

	if len(past) > 0 {
		best := past[0]
		note := best.ResolutionNote
		if note == "" && len(best.ActionsTaken) > 0 {
			note = strings.Join(best.ActionsTaken, " → ")
		}
		if note == "" {
			note = "resolved without notes"
		}
		parts = append(parts, fmt.Sprintf("Last time this happened (%s), it was resolved in %ds: %s.",
			best.ResolvedAt[:10], best.TTRSeconds, note))
	}

	if len(remediations) > 0 {
		best := remediations[0]
		parts = append(parts, fmt.Sprintf("Highest-confidence fix (%s, %.0f%% success rate): %s.",
			best.RootCauseCategory, best.SuccessRate*100, best.RemediationSummary))
	}

	if len(deployRisks) > 0 {
		parts = append(parts, fmt.Sprintf("Watch for deploy risk: %s (%d past occurrences).",
			deployRisks[0].SignatureName, deployRisks[0].OccurrenceCount))
	}

	if len(prefs) > 0 {
		p := prefs[0]
		parts = append(parts, fmt.Sprintf("Team %q prefers: %s → %s.", p.TeamName, p.PreferenceKey, p.PreferenceValue))
	}

	return strings.Join(parts, " ")
}

// ── Structured resolution recording ──────────────────────────────────────────

// RecordStructuredResolution saves a resolution to the legacy history AND optionally
// upserts a remediation pattern and/or a deploy signature.
func (s *DomainMemoryService) RecordStructuredResolution(incidentID, tenantID string, req models.DomainResolutionRequest) error {
	incident, found := s.incidentStore.GetIncidentByID(incidentID)
	if !found {
		return fmt.Errorf("incident not found: %s", incidentID)
	}

	// Delegate base resolution recording to the existing memory service
	if err := s.memoryStore.RecordResolution(
		tenantID, incident.Fingerprint, incident.Service, incidentID,
		int(time.Since(incident.FirstEventTime).Seconds()),
		req.RemediationSteps, req.ResolutionNote, time.Now(),
	); err != nil {
		return err
	}

	// Upsert a remediation pattern when enough data is provided
	if req.RemediationSummary != "" && len(req.RemediationSteps) > 0 {
		p := models.RemediationPattern{
			ID:                 newMemoryID(),
			TenantID:           tenantID,
			Service:            incident.Service,
			ErrorSignature:     req.ErrorSignature,
			RootCauseCategory:  req.RootCauseCategory,
			RemediationSummary: req.RemediationSummary,
			RemediationSteps:   req.RemediationSteps,
			SuccessCount:       boolToInt(req.WasSuccessful),
			FailureCount:       boolToInt(!req.WasSuccessful),
		}
		// Check for existing pattern with matching signature to accumulate counts
		existing, _ := s.domainStore.ListRemediationPatterns(tenantID, incident.Service)
		for _, e := range existing {
			if e.ErrorSignature == req.ErrorSignature && req.ErrorSignature != "" {
				p.ID = e.ID
				if req.WasSuccessful {
					p.SuccessCount = e.SuccessCount + 1
					p.FailureCount = e.FailureCount
				} else {
					p.SuccessCount = e.SuccessCount
					p.FailureCount = e.FailureCount + 1
				}
				break
			}
		}
		_ = s.domainStore.SaveRemediationPattern(p)
	}

	// Record a deploy signature if one is named
	if req.DeploySignatureName != "" {
		d := models.DeploySignature{
			ID:               newMemoryID(),
			TenantID:         tenantID,
			Service:          incident.Service,
			SignatureName:    req.DeploySignatureName,
			Indicators:       req.DeployIndicators,
			ImpactedServices: req.ImpactedServices,
			TypicalSeverity:  incident.Severity,
			OccurrenceCount:  1,
		}
		existing, _ := s.domainStore.ListDeploySignatures(tenantID, incident.Service)
		for _, e := range existing {
			if e.SignatureName == req.DeploySignatureName {
				_ = s.domainStore.BumpDeploySignatureOccurrence(tenantID, e.ID)
				d = models.DeploySignature{}
				break
			}
		}
		if d.ID != "" {
			_ = s.domainStore.SaveDeploySignature(d)
		}
	}

	return nil
}

// ── CRUD delegation methods ───────────────────────────────────────────────────

func (s *DomainMemoryService) ListRemediationPatterns(tenantID, service string) ([]models.RemediationPattern, error) {
	return s.domainStore.ListRemediationPatterns(tenantID, service)
}

func (s *DomainMemoryService) CreateRemediationPattern(tenantID string, p models.RemediationPattern) (models.RemediationPattern, error) {
	p.ID = newMemoryID()
	p.TenantID = tenantID
	if p.SuccessCount == 0 && p.FailureCount == 0 {
		p.SuccessCount = 1
	}
	return p, s.domainStore.SaveRemediationPattern(p)
}

func (s *DomainMemoryService) UpdateRemediationPattern(tenantID, id string, p models.RemediationPattern) error {
	if _, ok := s.domainStore.GetRemediationPattern(tenantID, id); !ok {
		return fmt.Errorf("remediation pattern not found")
	}
	return s.domainStore.UpdateRemediationPattern(tenantID, id, p)
}

func (s *DomainMemoryService) DeleteRemediationPattern(tenantID, id string) error {
	return s.domainStore.DeleteRemediationPattern(tenantID, id)
}

func (s *DomainMemoryService) MarkRemediationOutcome(tenantID, id string, success bool) error {
	if _, ok := s.domainStore.GetRemediationPattern(tenantID, id); !ok {
		return fmt.Errorf("remediation pattern not found")
	}
	return s.domainStore.MarkRemediationOutcome(tenantID, id, success)
}

func (s *DomainMemoryService) ListDeploySignatures(tenantID, service string) ([]models.DeploySignature, error) {
	return s.domainStore.ListDeploySignatures(tenantID, service)
}

func (s *DomainMemoryService) CreateDeploySignature(tenantID string, d models.DeploySignature) (models.DeploySignature, error) {
	d.ID = newMemoryID()
	d.TenantID = tenantID
	if d.OccurrenceCount == 0 {
		d.OccurrenceCount = 1
	}
	return d, s.domainStore.SaveDeploySignature(d)
}

func (s *DomainMemoryService) UpdateDeploySignature(tenantID, id string, d models.DeploySignature) error {
	if _, ok := s.domainStore.GetDeploySignature(tenantID, id); !ok {
		return fmt.Errorf("deploy signature not found")
	}
	return s.domainStore.UpdateDeploySignature(tenantID, id, d)
}

func (s *DomainMemoryService) DeleteDeploySignature(tenantID, id string) error {
	return s.domainStore.DeleteDeploySignature(tenantID, id)
}

func (s *DomainMemoryService) ListRunbookPreferences(tenantID, team, service string) ([]models.RunbookPreference, error) {
	return s.domainStore.ListRunbookPreferences(tenantID, team, service)
}

func (s *DomainMemoryService) CreateRunbookPreference(tenantID string, p models.RunbookPreference) (models.RunbookPreference, error) {
	p.ID = newMemoryID()
	p.TenantID = tenantID
	if p.PreferenceKey == "" {
		return p, fmt.Errorf("preference_key is required")
	}
	return p, s.domainStore.SaveRunbookPreference(p)
}

func (s *DomainMemoryService) UpdateRunbookPreference(tenantID, id string, p models.RunbookPreference) error {
	if _, ok := s.domainStore.GetRunbookPreference(tenantID, id); !ok {
		return fmt.Errorf("runbook preference not found")
	}
	return s.domainStore.UpdateRunbookPreference(tenantID, id, p)
}

func (s *DomainMemoryService) DeleteRunbookPreference(tenantID, id string) error {
	return s.domainStore.DeleteRunbookPreference(tenantID, id)
}

// ── helpers ───────────────────────────────────────────────────────────────────

func newMemoryID() string {
	b := make([]byte, 10)
	_, _ = rand.Read(b)
	return "mem_" + hex.EncodeToString(b)
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
