package services

// explain_service.go — Phase 1, Week 2
//
// BuildExplanation now tries the LLM-powered RCA pipeline first and falls
// back to the original template-based narrative when:
//   - No LLM client is injected (nil), or
//   - The API call fails.
//
// The function signature is unchanged so no callers need updating.

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"ai-incident-platform/backend/internal/llm"
	"ai-incident-platform/backend/internal/models"
)

// ExplainService wraps the LLM client for incident explanation.
type ExplainService struct {
	llmClient     *llm.Client
	knowledgeBase *KnowledgeBase
}

func NewExplainService(llmClient *llm.Client) *ExplainService {
	return &ExplainService{llmClient: llmClient}
}

// SetKnowledgeBase wires in the knowledge base for template narrative enrichment.
func (s *ExplainService) SetKnowledgeBase(kb *KnowledgeBase) {
	s.knowledgeBase = kb
}

// ─────────────────────────────────────────────────────
// LLM RCA response schema — extended with evidence graph fields
// ─────────────────────────────────────────────────────

// llmClaim mirrors one element of the "claims" array in the LLM response.
type llmClaim struct {
	Claim       string   `json:"claim"`
	Confidence  int      `json:"confidence"`
	Reasoning   string   `json:"reasoning"`
	FalsifiedBy string   `json:"falsified_by"`
	EvidenceIDs []string `json:"evidence_ids"`
}

// llmConflict mirrors one element of the "conflicting_signals" array.
type llmConflict struct {
	Signal     string `json:"signal"`
	Source     string `json:"source"`
	Strength   int    `json:"strength"`
	Resolution string `json:"resolution"`
}

type rcaResponse struct {
	RootCauseSummary       string        `json:"root_cause_summary"`
	RootCauseType          string        `json:"root_cause_type"`
	Confidence             int           `json:"confidence"`
	RiskScore              int           `json:"risk_score"`
	Reasoning              []string      `json:"reasoning"`
	RecommendedNextStep    string        `json:"recommended_next_step"`
	Narrative              string        `json:"narrative"`
	// Evidence graph fields (may be absent in older-style responses)
	Claims                 []llmClaim    `json:"claims"`
	ConflictingSignals     []llmConflict `json:"conflicting_signals"`
	RecommendationWhy      string        `json:"recommendation_why"`
	RecommendationPriority string        `json:"recommendation_priority"`
	RecommendationRisk     string        `json:"recommendation_risk"`
}

// ─────────────────────────────────────────────────────
// Public API
// ─────────────────────────────────────────────────────

// BuildExplanation returns the narrative string for the explain endpoint.
// It is the primary entry point used by ExplainHandler.
func BuildExplanation(detail models.IncidentDetail) string {
	// Stateless fallback — used when called without an ExplainService instance.
	return buildTemplateNarrative(detail)
}

// Explain returns the incident WITHOUT making a network call.
//
// It serves the two cheap, always-available tiers only:
//
//	knowledge_base — a pre-authored pattern matched, so a real cause is known.
//	observed       — nothing matched: return the evidence, assert no cause.
//
// It deliberately never calls the LLM. Reaching a model is an explicit act that
// costs money, takes up to 30 s, and produces a claim somebody is accountable
// for — so it belongs behind Analyze(), not behind a page load.
//
// On the observed tier the incident's RootCauseSummary is cleared from the
// response. Leaving it populated is how the old implementation presented a
// restated alert title as a diagnosis.
func (s *ExplainService) Explain(detail models.IncidentDetail) (models.IncidentDetail, string) {
	// 1. Knowledge base — deterministic, human-authored, no API call.
	if s.knowledgeBase != nil {
		incident := detail.Incident
		if entry := s.knowledgeBase.MatchByIncident(incident.Title, incident.RootCauseSummary, incident.Reasoning); entry != nil {
			narrative := fmt.Sprintf("%s. %s", entry.RootCause, entry.Impact)
			if entry.Prevention != "" {
				narrative += " " + entry.Prevention + "."
			}
			confidence := entry.BaseConfidence
			detail.Provenance = models.AnalysisProvenance{
				Source:                models.AnalysisSourceKB,
				HasCausalClaim:        true,
				RCAConfidence:         &confidence,
				CorrelationConfidence: detail.Incident.Confidence,
				KBEntryID:             entry.ID,
			}
			return detail, narrative
		}
	}

	// 2. No pattern matched — observation tier. Evidence only, no causal claim.
	return s.observed(detail, s.unavailableReason())
}

// Analyze is the explicit, human-triggered (or creation-time) LLM path.
// It is the ONLY code path that may produce an llm-tier causal claim.
//
// actor is recorded on the provenance so the resulting analysis is an
// auditable artifact attributable to whoever requested it; pass "system" for
// the automatic creation-time call.
//
// On failure it degrades to the observation tier carrying the reason, so the
// caller can render a retry affordance rather than a fabricated answer.
func (s *ExplainService) Analyze(detail models.IncidentDetail, actor string) (models.IncidentDetail, string) {
	if s.llmClient == nil || !s.llmClient.IsConfigured() {
		return s.observed(detail, "AI analysis is not configured on this deployment")
	}

	enriched, narrative, err := s.llmExplain(detail)
	if err != nil {
		slog.Warn("analyze: llm call failed — degrading to observation tier",
			"incident_id", detail.Incident.ID, "error", err)
		// Fall back to the knowledge base before giving up on a causal claim.
		if kbDetail, kbNarrative := s.Explain(detail); kbDetail.Provenance.HasCausalClaim {
			return kbDetail, kbNarrative
		}
		return s.observed(detail, "AI analysis unavailable: "+err.Error())
	}

	// llmExplain has already written the model's confidence onto the incident.
	confidence := enriched.Incident.Confidence
	enriched.Provenance = models.AnalysisProvenance{
		Source:                models.AnalysisSourceLLM,
		HasCausalClaim:        true,
		RCAConfidence:         &confidence,
		CorrelationConfidence: detail.Incident.Confidence,
		Model:                 s.llmClient.ModelName(),
		AnalyzedAt:            time.Now().UTC().Format(time.RFC3339),
		AnalyzedBy:            actor,
	}
	return enriched, narrative
}

// observed builds the zero-claim tier: real evidence, no asserted cause.
func (s *ExplainService) observed(detail models.IncidentDetail, unavailable string) (models.IncidentDetail, string) {
	detail.EvidenceRefs = buildEvidenceRefs(detail)
	detail.EvidenceGraph = BuildRuleBasedEvidenceGraph(detail)
	detail.Provenance = models.ObservedProvenance(detail.Incident.Confidence, unavailable)

	// Strip the inference-shaped fields. What the correlation engine stored as
	// "root cause" on this path is the alert title — a symptom, not a cause —
	// and rendering it under a Root Cause heading is the false-end bug.
	detail.Incident.RootCauseSummary = ""
	detail.Summary.RootCauseSummary = ""
	detail.DecisionCard.Cause = ""

	return detail, buildObservationNarrative(detail)
}

// unavailableReason explains why no causal claim is present, so the UI can
// offer the right affordance: "Analyze" when a model is available, or an
// explanation when one is not.
func (s *ExplainService) unavailableReason() string {
	if s.llmClient == nil || !s.llmClient.IsConfigured() {
		return "No matching known pattern, and AI analysis is not configured on this deployment"
	}
	return "No matching known pattern yet — run AI analysis to investigate the cause"
}

// ─────────────────────────────────────────────────────
// LLM path
// ─────────────────────────────────────────────────────

func (s *ExplainService) llmExplain(detail models.IncidentDetail) (models.IncidentDetail, string, error) {
	ctx := AssembleForRCA(detail)

	rawText, err := s.llmClient.CompleteWithSystem(ctx.SystemPrompt, ctx.UserPrompt)
	if err != nil {
		return detail, "", err
	}

	jsonStr := extractJSON(rawText)

	var rca rcaResponse
	if err := json.Unmarshal([]byte(jsonStr), &rca); err != nil {
		return detail, "", fmt.Errorf("explain: parse rca json: %w — raw: %s", err, truncateCopilot(rawText, 200))
	}

	// Enrich the incident detail with LLM-produced RCA fields
	if rca.RootCauseSummary != "" {
		detail.Incident.RootCauseSummary = rca.RootCauseSummary
		detail.Summary.RootCauseSummary = rca.RootCauseSummary
	}
	if rca.RootCauseType != "" {
		detail.Incident.RootCauseType = rca.RootCauseType
		detail.Summary.RootCauseType = rca.RootCauseType
	}
	if rca.Confidence > 0 {
		detail.Incident.Confidence = rca.Confidence
		detail.Summary.Confidence = rca.Confidence
	}
	if rca.RiskScore > 0 {
		detail.Incident.RiskScore = rca.RiskScore
		detail.Summary.RiskScore = rca.RiskScore
	}
	if len(rca.Reasoning) > 0 {
		detail.Incident.Reasoning = rca.Reasoning
		detail.Evidence = rca.Reasoning
	}

	// Build evidence refs from actual timeline data, not LLM text.
	detail.EvidenceRefs = buildEvidenceRefs(detail)
	if rca.RecommendedNextStep != "" {
		detail.RecommendedNextStep = rca.RecommendedNextStep
	}

	// Build evidence graph — prefer LLM-provided claims; fall back to rule-based.
	detail.EvidenceGraph = buildLLMEvidenceGraph(rca, detail)

	narrative := rca.Narrative
	if narrative == "" {
		narrative = buildTemplateNarrative(detail)
	}

	return detail, narrative, nil
}

// ─────────────────────────────────────────────────────
// Evidence ref builder — derives pointers to actual data artifacts
// ─────────────────────────────────────────────────────

// buildEvidenceRefs converts actual timeline events, context logs, and change
// records into structured EvidenceRef items. These are concrete data pointers
// that support RCA conclusions — not LLM-generated text.
func buildEvidenceRefs(detail models.IncidentDetail) []models.EvidenceRef {
	refs := make([]models.EvidenceRef, 0, len(detail.Events)+len(detail.ContextLogs)+1)

	// Timeline events — take up to the first 5 most significant events
	for i, te := range detail.Events {
		if i >= 5 {
			break
		}
		label := strings.TrimSpace(te.Event.Title)
		if label == "" {
			label = strings.TrimSpace(te.Event.Message)
		}
		if len(label) > 80 {
			label = label[:80] + "…"
		}
		ts := te.Event.Timestamp.Format("2006-01-02T15:04:05Z")
		refs = append(refs, models.EvidenceRef{
			Type:      "event",
			ID:        te.Event.ID,
			Timestamp: ts,
			Label:     fmt.Sprintf("[%s] %s on %s", strings.ToUpper(te.Event.Severity), te.Event.Source, te.Event.Service),
			Detail:    label,
		})
	}

	// Context logs — take up to 3 error/warn log entries
	errCount := 0
	for _, log := range detail.ContextLogs {
		if errCount >= 3 {
			break
		}
		cat := strings.ToLower(log.Category)
		if cat != "error" && cat != "warn" && cat != "critical" {
			continue
		}
		detail2 := strings.TrimSpace(log.Message)
		if len(detail2) > 100 {
			detail2 = detail2[:100] + "…"
		}
		refs = append(refs, models.EvidenceRef{
			Type:      "log",
			ID:        fmt.Sprintf("log-%d", errCount),
			Timestamp: log.Timestamp,
			Label:     fmt.Sprintf("[%s] %s", strings.ToUpper(cat), log.Source),
			Detail:    detail2,
		})
		errCount++
	}

	// Change record — if a deployment/config change was linked, it is direct evidence
	if detail.WhatChanged.Type != "" {
		detail3 := fmt.Sprintf("%s on %s", detail.WhatChanged.Type, detail.WhatChanged.Service)
		if detail.WhatChanged.Description != "" {
			detail3 += ": " + detail.WhatChanged.Description
		}
		if len(detail3) > 120 {
			detail3 = detail3[:120] + "…"
		}
		refs = append(refs, models.EvidenceRef{
			Type:      "change",
			ID:        "change-link",
			Timestamp: detail.WhatChanged.Timestamp,
			Label:     fmt.Sprintf("Change: %s @ %s", detail.WhatChanged.Type, detail.WhatChanged.Service),
			Detail:    detail3,
		})
	}

	// Context metrics — if CPU/memory/disk thresholds are breached, cite them
	if detail.ContextMetrics != nil {
		m := detail.ContextMetrics
		if m.CPUPercent > 80 {
			refs = append(refs, models.EvidenceRef{
				Type:      "metric",
				ID:        "metric-cpu",
				Timestamp: m.CollectedAt,
				Label:     "High CPU utilization",
				Detail:    fmt.Sprintf("CPU at %.1f%% (threshold: 80%%)", m.CPUPercent),
			})
		}
		if m.MemoryPercent > 85 {
			refs = append(refs, models.EvidenceRef{
				Type:      "metric",
				ID:        "metric-mem",
				Timestamp: m.CollectedAt,
				Label:     "High memory utilization",
				Detail:    fmt.Sprintf("Memory at %.1f%% (threshold: 85%%)", m.MemoryPercent),
			})
		}
		if m.DiskPercent > 90 {
			refs = append(refs, models.EvidenceRef{
				Type:      "metric",
				ID:        "metric-disk",
				Timestamp: m.CollectedAt,
				Label:     "Disk space critical",
				Detail:    fmt.Sprintf("Disk at %.1f%% (threshold: 90%%)", m.DiskPercent),
			})
		}
	}

	return refs
}

// ─────────────────────────────────────────────────────
// Template fallback (original implementation, preserved)
// ─────────────────────────────────────────────────────

func buildTemplateNarrative(detail models.IncidentDetail) string {
	incident := detail.Incident
	summary := detail.Summary

	parts := make([]string, 0)

	serviceName := humanizeServiceName(incident.Service)
	if strings.TrimSpace(serviceName) == "" {
		serviceName = "This service"
	}

	severityLabel := strings.ToLower(strings.TrimSpace(incident.Severity))
	if severityLabel == "" {
		severityLabel = "active"
	}

	parts = append(parts, fmt.Sprintf("%s is experiencing a %s incident", serviceName, severityLabel))

	if strings.TrimSpace(summary.RootCauseSummary) != "" {
		parts = append(parts, sentenceCase(summary.RootCauseSummary))
	}

	if detail.WhatChanged.Type != "" {
		changeService := humanizeServiceName(detail.WhatChanged.Service)
		if strings.TrimSpace(changeService) == "" {
			changeService = serviceName
		}
		parts = append(parts, fmt.Sprintf("This started after a %s on %s", detail.WhatChanged.Type, changeService))
	}

	if summary.ImpactCount > 1 {
		parts = append(parts, fmt.Sprintf("It is affecting %d services", summary.ImpactCount))
	} else if summary.ImpactCount == 1 {
		parts = append(parts, "It is affecting 1 service")
	}

	if summary.SeenBefore {
		parts = append(parts, fmt.Sprintf("This issue has been seen before %s", pluralizeTimes(summary.RecurringCount)))
	}

	return strings.Join(parts, ". ") + "."
}

// ─────────────────────────────────────────────────────
// String helpers (shared with copilot_service.go within package)
// ─────────────────────────────────────────────────────

func humanizeServiceName(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}
	parts := strings.FieldsFunc(trimmed, func(r rune) bool { return r == '-' || r == '_' })
	for i, part := range parts {
		if len(part) == 0 {
			continue
		}
		parts[i] = strings.ToUpper(part[:1]) + strings.ToLower(part[1:])
	}
	return strings.Join(parts, " ")
}

func sentenceCase(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}
	return strings.ToUpper(trimmed[:1]) + trimmed[1:]
}

func pluralizeTimes(value int) string {
	if value == 1 {
		return "1 time"
	}
	return fmt.Sprintf("%d times", value)
}

// ─────────────────────────────────────────────────────
// LLM evidence graph builder
// ─────────────────────────────────────────────────────

// buildLLMEvidenceGraph converts the LLM-parsed rcaResponse into a models.EvidenceGraph.
// If the LLM response didn't include the extended evidence-graph fields (old-style response),
// it falls back to the rule-based builder so the graph is always populated.
func buildLLMEvidenceGraph(rca rcaResponse, detail models.IncidentDetail) *models.EvidenceGraph {
	refs := buildEvidenceRefs(detail)

	// If the LLM didn't return claims, use the rule-based builder.
	if len(rca.Claims) == 0 {
		g := BuildRuleBasedEvidenceGraph(detail)
		g.Method = "hybrid" // LLM powered summary, rule-based graph
		return g
	}

	// Convert LLM claims to model claims.
	refByID := map[string]models.EvidenceRef{}
	for _, r := range refs {
		refByID[r.ID] = r
	}

	claims := make([]models.EvidencedClaim, 0, len(rca.Claims))
	for _, c := range rca.Claims {
		mc := models.EvidencedClaim{
			Claim:       c.Claim,
			Confidence:  c.Confidence,
			Reasoning:   c.Reasoning,
			FalsifiedBy: c.FalsifiedBy,
			EvidenceIDs: c.EvidenceIDs,
		}
		for _, id := range c.EvidenceIDs {
			if r, ok := refByID[id]; ok {
				mc.EvidenceRefs = append(mc.EvidenceRefs, r)
			}
		}
		claims = append(claims, mc)
	}

	// Convert LLM conflicting signals.
	conflicts := make([]models.ConflictingSignal, 0, len(rca.ConflictingSignals))
	for _, cs := range rca.ConflictingSignals {
		conflicts = append(conflicts, models.ConflictingSignal{
			Signal:     cs.Signal,
			Source:     cs.Source,
			Strength:   cs.Strength,
			Resolution: cs.Resolution,
		})
	}

	// If the LLM didn't return any conflicts, detect them rule-based.
	if len(conflicts) == 0 {
		conflicts = detectConflicts(detail)
	}

	// Build recommendation from LLM fields.
	var rec *models.EvidenceRecommendation
	if rca.RecommendedNextStep != "" {
		priority := rca.RecommendationPriority
		if priority == "" {
			priority = actionPriority(detail.Incident.Severity)
		}
		riskLevel := rca.RecommendationRisk
		if riskLevel == "" {
			riskLevel = "medium"
		}
		why := rca.RecommendationWhy
		if why == "" {
			why = "See root cause summary for reasoning."
		}
		rec = &models.EvidenceRecommendation{
			Action:    rca.RecommendedNextStep,
			Why:       why,
			Priority:  priority,
			RiskLevel: riskLevel,
		}
	}

	g := &models.EvidenceGraph{
		SourcesUsed:        refs,
		Claims:             claims,
		ConflictingSignals: conflicts,
		Recommendation:     rec,
		Method:             "llm",
		GeneratedAt:        time.Now().Format(time.RFC3339),
	}
	g.OverallConfidence = computeGraphConfidence(claims, rca.Confidence)
	return g
}

// ─────────────────────────────────────────────────────
// Observation tier narrative
// ─────────────────────────────────────────────────────

// buildObservationNarrative describes what the system OBSERVED, without
// asserting why it happened.
//
// Every sentence here must be a fact the platform actually collected: how many
// alerts correlated, which services they spanned, what changed nearby, whether
// this fingerprint has been seen before. That is genuinely most of what an
// on-call engineer gathers in their first five minutes, and none of it is a
// guess — so it is safe to show when no analysis exists.
//
// It must never contain the words "root cause", "caused by", or "due to".
func buildObservationNarrative(detail models.IncidentDetail) string {
	incident := detail.Incident
	var sb strings.Builder

	// 1. What correlated.
	eventCount := len(detail.Events)
	if eventCount == 0 {
		eventCount = incident.EventCount
	}
	if eventCount > 1 {
		sb.WriteString(fmt.Sprintf(
			"%d alerts correlated into this incident on %s over %s.",
			eventCount, incident.Service, incidentSpan(incident)))
	} else {
		sb.WriteString(fmt.Sprintf(
			"A single %s alert on %s.", strings.ToLower(incident.Severity), incident.Service))
	}

	// 2. Blast radius — only when we actually observed more than one service.
	if len(incident.ImpactedServices) > 1 {
		sb.WriteString(fmt.Sprintf(
			" Alerts spanned %d services: %s.",
			len(incident.ImpactedServices), strings.Join(incident.ImpactedServices, ", ")))
	}

	// 3. Temporal coincidence with a change. Stated as timing, NOT causation —
	//    the engineer draws the inference, the platform only reports the fact.
	if detail.WhatChanged.Type != "" {
		desc := detail.WhatChanged.Description
		if desc == "" {
			desc = detail.WhatChanged.Type
		}
		sb.WriteString(fmt.Sprintf(
			" A %s to %s was recorded near the start of this incident: %s.",
			detail.WhatChanged.Type, detail.WhatChanged.Service, desc))
	}

	// 4. Recurrence — a strong, purely factual signal.
	if detail.Summary.RecurringCount > 0 {
		sb.WriteString(fmt.Sprintf(
			" This alert signature has been seen %d time(s) before on this service.",
			detail.Summary.RecurringCount))
		if detail.PatternHistory != nil && len(detail.PatternHistory.SuggestedPlaybook) > 0 {
			sb.WriteString(fmt.Sprintf(
				" It was previously resolved by: %s.",
				strings.Join(detail.PatternHistory.SuggestedPlaybook, "; ")))
		}
	}

	// 5. Supporting telemetry counts — what is available to look at.
	if n := len(detail.ContextLogs); n > 0 {
		sb.WriteString(fmt.Sprintf(" %d related log lines were captured.", n))
	}

	// 6. State the epistemic position plainly. This is the sentence that
	//    replaces the fabricated root cause.
	sb.WriteString(" No root-cause analysis has been run for this incident yet.")

	return sb.String()
}

// incidentSpan renders the wall-clock duration the alerts arrived over.
func incidentSpan(incident models.Incident) string {
	d := incident.LastEventTime.Sub(incident.FirstEventTime)
	switch {
	case d < time.Minute:
		return "under a minute"
	case d < time.Hour:
		return fmt.Sprintf("%d minutes", int(d.Minutes()))
	default:
		return fmt.Sprintf("%.1f hours", d.Hours())
	}
}
