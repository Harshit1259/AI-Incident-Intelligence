package services

// evidence_graph_service.go
//
// Builds an EvidenceGraph from structured incident data without calling an LLM.
// Used in two situations:
//   1. As the primary builder when no LLM is configured.
//   2. As a fallback enricher when the LLM response doesn't return the extended schema.
//
// The resulting graph has the same shape as the LLM-produced one so the frontend
// renders identically regardless of which path was taken.

import (
	"fmt"
	"strings"
	"time"

	"ai-incident-platform/backend/internal/models"
)

// BuildRuleBasedEvidenceGraph constructs a full EvidenceGraph from the incident
// detail without any LLM inference. Every claim is derived from real data artifacts.
func BuildRuleBasedEvidenceGraph(detail models.IncidentDetail) *models.EvidenceGraph {
	incident := detail.Incident
	refs := buildEvidenceRefs(detail) // existing helper in explain_service.go

	g := &models.EvidenceGraph{
		SourcesUsed: refs,
		Method:      "rule_based",
		GeneratedAt: time.Now().Format(time.RFC3339),
	}

	// ── Claims ────────────────────────────────────────────────────────────────
	g.Claims = buildRuleClaims(detail, refs)

	// ── Conflicting signals ───────────────────────────────────────────────────
	g.ConflictingSignals = detectConflicts(detail)

	// ── Recommendation with explicit "why" ────────────────────────────────────
	g.Recommendation = buildRuleRecommendation(detail)

	// ── Overall confidence ────────────────────────────────────────────────────
	g.OverallConfidence = computeGraphConfidence(g.Claims, incident.Confidence)

	return g
}

// ─── Claims ───────────────────────────────────────────────────────────────────

func buildRuleClaims(detail models.IncidentDetail, refs []models.EvidenceRef) []models.EvidencedClaim {
	incident := detail.Incident
	var claims []models.EvidencedClaim

	// Claim 1: root cause assertion
	if rcs := strings.TrimSpace(incident.RootCauseSummary); rcs != "" {
		evidIDs := evidenceIDsOf(refs, "event", "metric")
		conf := incident.Confidence
		if conf == 0 {
			conf = 55
		}
		claims = append(claims, models.EvidencedClaim{
			Claim:       rcs,
			Confidence:  conf,
			Reasoning:   buildRootCauseReasoning(detail),
			FalsifiedBy: buildFalsification(incident.RootCauseType, detail),
			EvidenceIDs: evidIDs,
		})
	}

	// Claim 2: change-linked deployment regression
	if detail.WhatChanged.Type != "" {
		conf := incident.WhatChangedConfidence
		if conf == 0 {
			conf = 65
		}
		svc := detail.WhatChanged.Service
		if svc == "" {
			svc = incident.Service
		}
		changeRef := findRefByType(refs, "change")
		ids := []string{}
		if changeRef != nil {
			ids = append(ids, changeRef.ID)
		}
		claims = append(claims, models.EvidencedClaim{
			Claim:       fmt.Sprintf("%s on %s preceded the incident", detail.WhatChanged.Type, svc),
			Confidence:  conf,
			Reasoning:   fmt.Sprintf("A %s was recorded on %s at %s, before the first alert.", detail.WhatChanged.Type, svc, detail.WhatChanged.Timestamp),
			FalsifiedBy: fmt.Sprintf("Reverting the %s on %s and observing that alert volume drops within 10 minutes would falsify the causal link.", detail.WhatChanged.Type, svc),
			EvidenceIDs: ids,
		})
	}

	// Claim 3: impact scope
	if incident.ImpactCount > 0 {
		services := strings.Join(incident.ImpactedServices, ", ")
		if services == "" {
			services = fmt.Sprintf("%d service(s)", incident.ImpactCount)
		}
		claims = append(claims, models.EvidencedClaim{
			Claim:       fmt.Sprintf("The failure propagated to %s", services),
			Confidence:  70,
			Reasoning:   fmt.Sprintf("%d correlated events across %d services confirm lateral impact.", incident.EventCount, incident.ImpactCount),
			FalsifiedBy: "If all other services recover independently without action on the root service, propagation was coincidental.",
			EvidenceIDs: evidenceIDsOf(refs, "event"),
		})
	}

	// Claim 4: recurrence pattern
	if incident.SeenBefore && incident.RecurringCount > 0 {
		claims = append(claims, models.EvidencedClaim{
			Claim:       fmt.Sprintf("This failure pattern has recurred %d time(s), indicating a systemic issue", incident.RecurringCount),
			Confidence:  80,
			Reasoning:   "Pattern matching against incident history confirms the same fingerprint has appeared before.",
			FalsifiedBy: "If the root service has been refactored since the last occurrence, this may be a distinct failure mode with superficial similarity.",
			EvidenceIDs: []string{},
		})
	}

	// Resolve EvidenceRefs into each claim for frontend convenience
	refByID := map[string]models.EvidenceRef{}
	for _, r := range refs {
		refByID[r.ID] = r
	}
	for i := range claims {
		for _, id := range claims[i].EvidenceIDs {
			if r, ok := refByID[id]; ok {
				claims[i].EvidenceRefs = append(claims[i].EvidenceRefs, r)
			}
		}
	}

	return claims
}

// ─── Conflicting signals ──────────────────────────────────────────────────────

func detectConflicts(detail models.IncidentDetail) []models.ConflictingSignal {
	var conflicts []models.ConflictingSignal
	incident := detail.Incident

	// Low event count vs high severity
	if incident.EventCount == 1 && (incident.Severity == "critical" || incident.Severity == "high") {
		conflicts = append(conflicts, models.ConflictingSignal{
			Signal:     fmt.Sprintf("Only 1 event correlates despite %s severity", incident.Severity),
			Source:     "event_count",
			Strength:   55,
			Resolution: "A single health-check failure can trigger a critical alert before other signals arrive. Monitor for additional events over the next 5 minutes.",
		})
	}

	// High confidence root cause but no change linked
	if incident.Confidence > 70 && detail.WhatChanged.Type == "" {
		conflicts = append(conflicts, models.ConflictingSignal{
			Signal:     "High root-cause confidence but no deployment or config change is linked",
			Source:     "change_absence",
			Strength:   45,
			Resolution: "The confidence score reflects signal correlation strength, not change evidence. An infrastructure event (DNS, network) may be the cause.",
		})
	}

	// Change linked but low confidence
	if detail.WhatChanged.Type != "" && incident.WhatChangedConfidence > 0 && incident.WhatChangedConfidence < 50 {
		conflicts = append(conflicts, models.ConflictingSignal{
			Signal:     fmt.Sprintf("A change is linked but confidence in the causal connection is only %d%%", incident.WhatChangedConfidence),
			Source:     "change_confidence",
			Strength:   60,
			Resolution: "The change may be coincidental. Check if the change is on the critical path for the failing service.",
		})
	}

	// Multiple impacted services but root cause is a single-service issue
	if incident.ImpactCount > 3 && incident.RootCauseType != "" && !strings.Contains(incident.RootCauseType, "dependency") && !strings.Contains(incident.RootCauseType, "network") {
		conflicts = append(conflicts, models.ConflictingSignal{
			Signal:     fmt.Sprintf("%d services impacted but root cause type is %q — suggesting wider scope", incident.ImpactCount, incident.RootCauseType),
			Source:     "impact_scope",
			Strength:   50,
			Resolution: "Consider whether a shared dependency (database, service mesh, DNS) is the actual root cause rather than the single service.",
		})
	}

	// Memory metrics fine but memory root cause type
	if detail.ContextMetrics != nil && detail.ContextMetrics.MemoryPercent < 70 &&
		strings.Contains(strings.ToLower(incident.RootCauseType), "memory") {
		conflicts = append(conflicts, models.ConflictingSignal{
			Signal:     fmt.Sprintf("Root cause type is memory-related but current memory is only %.1f%%", detail.ContextMetrics.MemoryPercent),
			Source:     "memory_metric",
			Strength:   70,
			Resolution: "Memory may have spiked and partially recovered, or the root cause classification may need revision.",
		})
	}

	return conflicts
}

// ─── Recommendation ───────────────────────────────────────────────────────────

func buildRuleRecommendation(detail models.IncidentDetail) *models.EvidenceRecommendation {
	incident := detail.Incident

	if detail.PrimaryAction != nil {
		a := detail.PrimaryAction
		why := fmt.Sprintf("This action (%s) was selected because it directly addresses %s with %s risk.",
			a.Label, incident.RootCauseSummary, a.RiskLevel)
		if detail.WhatChanged.Type != "" {
			why = fmt.Sprintf("A %s on %s is linked to this incident. %s targets this change directly.",
				detail.WhatChanged.Type, detail.WhatChanged.Service, a.Label)
		}
		return &models.EvidenceRecommendation{
			Action:    a.Label + ": " + a.Description,
			Why:       why,
			Priority:  actionPriority(incident.Severity),
			RiskLevel: a.RiskLevel,
		}
	}

	if strings.TrimSpace(detail.RecommendedNextStep) != "" {
		return &models.EvidenceRecommendation{
			Action:    detail.RecommendedNextStep,
			Why:       fmt.Sprintf("Derived from the root cause: %s", incident.RootCauseSummary),
			Priority:  actionPriority(incident.Severity),
			RiskLevel: "medium",
		}
	}

	return &models.EvidenceRecommendation{
		Action:    "Investigate root service logs and recent deployments",
		Why:       "No specific action matched. Manual investigation is needed to confirm the root cause before acting.",
		Priority:  "investigate",
		RiskLevel: "low",
	}
}

// ─── Copilot evidence graph ───────────────────────────────────────────────────

// BuildCopilotEvidenceGraph creates an EvidenceGraph scoped to a single copilot answer.
func BuildCopilotEvidenceGraph(detail models.IncidentDetail, intent, answer, answerReasoning string, confidence int) *models.EvidenceGraph {
	refs := buildEvidenceRefs(detail)
	incident := detail.Incident

	claim := models.EvidencedClaim{
		Claim:       summarizeCopilotAnswer(answer),
		Confidence:  confidence,
		Reasoning:   answerReasoning,
		FalsifiedBy: copilotFalsification(intent, detail),
		EvidenceIDs: evidenceIDsOf(refs, "event", "change", "metric"),
	}
	refByID := map[string]models.EvidenceRef{}
	for _, r := range refs {
		refByID[r.ID] = r
	}
	for _, id := range claim.EvidenceIDs {
		if r, ok := refByID[id]; ok {
			claim.EvidenceRefs = append(claim.EvidenceRefs, r)
		}
	}

	g := &models.EvidenceGraph{
		SourcesUsed:        refs,
		Claims:             []models.EvidencedClaim{claim},
		ConflictingSignals: detectConflicts(detail),
		OverallConfidence:  confidence,
		Method:             "rule_based",
		GeneratedAt:        time.Now().Format(time.RFC3339),
	}

	// Surface the primary action as the recommendation.
	if detail.PrimaryAction != nil && (intent == "first_action" || intent == "general") {
		g.Recommendation = &models.EvidenceRecommendation{
			Action:    detail.PrimaryAction.Label,
			Why:       fmt.Sprintf("Highest-priority action for %s severity %s incident", incident.Severity, incident.RootCauseType),
			Priority:  actionPriority(incident.Severity),
			RiskLevel: detail.PrimaryAction.RiskLevel,
		}
	}

	return g
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

func buildRootCauseReasoning(detail models.IncidentDetail) string {
	incident := detail.Incident
	parts := []string{}

	if incident.EventCount > 0 {
		parts = append(parts, fmt.Sprintf("%d correlated events on %s", incident.EventCount, incident.Service))
	}
	if detail.WhatChanged.Type != "" {
		parts = append(parts, fmt.Sprintf("a %s on %s at %s", detail.WhatChanged.Type, detail.WhatChanged.Service, detail.WhatChanged.Timestamp))
	}
	if len(incident.Reasoning) > 0 {
		parts = append(parts, incident.Reasoning[0])
	}

	if len(parts) == 0 {
		return "Based on correlated alert signals and pattern matching."
	}
	return "Supported by: " + strings.Join(parts, "; ") + "."
}

func buildFalsification(rcaType string, detail models.IncidentDetail) string {
	switch strings.ToLower(rcaType) {
	case "database_issue":
		return "This claim is falsified if database response times remain normal (< 100 ms P95) throughout the incident window."
	case "memory_pressure":
		return "This claim is falsified if memory utilization stays below 80% on all instances while alerts continue."
	case "cpu_spike":
		return "This claim is falsified if CPU drops below 70% without manual intervention and alerts persist."
	case "deployment_regression":
		svc := detail.WhatChanged.Service
		if svc == "" {
			svc = detail.Incident.Service
		}
		return fmt.Sprintf("This claim is falsified if reverting the deployment on %s does not reduce error rate within 10 minutes.", svc)
	case "network_issue":
		return "This claim is falsified if network latency and packet loss return to baseline without any routing or infrastructure changes."
	case "dependency_failure":
		return "This claim is falsified if the identified upstream dependency is healthy and its SLAs are being met during the incident window."
	case "timeout_cascade":
		return "This claim is falsified if reducing downstream timeout values stops the cascading failure."
	case "configuration_error":
		return "This claim is falsified if rolling back the configuration change fully restores service health."
	default:
		return "This claim is falsified if the service recovers fully without any action targeting this specific root cause."
	}
}

func copilotFalsification(intent string, detail models.IncidentDetail) string {
	switch intent {
	case "why":
		return buildFalsification(detail.Incident.RootCauseType, detail)
	case "first_action":
		if detail.PrimaryAction != nil {
			return fmt.Sprintf("This recommendation is falsified if executing %q does not reduce alert volume within 15 minutes.", detail.PrimaryAction.Label)
		}
		return "This recommendation is falsified if following the suggested step does not reduce alert volume within 15 minutes."
	case "change":
		if detail.WhatChanged.Type != "" {
			return fmt.Sprintf("The causal link to the %s is falsified if reverting it does not affect incident severity.", detail.WhatChanged.Type)
		}
		return "Falsified if no change is found in the deployment history for the relevant time window."
	case "history":
		return "The recurrence pattern is falsified if the root service has been fully refactored since the last occurrence."
	default:
		return "This assessment is falsified by any evidence that contradicts the stated diagnosis."
	}
}

func summarizeCopilotAnswer(answer string) string {
	// Extract first sentence as the claim label.
	answer = strings.TrimSpace(answer)
	if i := strings.IndexByte(answer, '.'); i > 0 && i < 120 {
		return answer[:i+1]
	}
	if len(answer) > 120 {
		return answer[:120] + "…"
	}
	return answer
}

func actionPriority(severity string) string {
	switch strings.ToLower(severity) {
	case "critical":
		return "immediate"
	case "high":
		return "immediate"
	case "medium":
		return "soon"
	default:
		return "investigate"
	}
}

func evidenceIDsOf(refs []models.EvidenceRef, types ...string) []string {
	typeSet := map[string]bool{}
	for _, t := range types {
		typeSet[t] = true
	}
	ids := []string{}
	for _, r := range refs {
		if typeSet[r.Type] {
			ids = append(ids, r.ID)
		}
	}
	return ids
}

func findRefByType(refs []models.EvidenceRef, refType string) *models.EvidenceRef {
	for i, r := range refs {
		if r.Type == refType {
			return &refs[i]
		}
	}
	return nil
}

func computeGraphConfidence(claims []models.EvidencedClaim, incidentConf int) int {
	if len(claims) == 0 {
		if incidentConf > 0 {
			return incidentConf
		}
		return 40
	}
	sum := 0
	for _, c := range claims {
		sum += c.Confidence
	}
	avg := sum / len(claims)
	// Blend with the incident-level confidence.
	if incidentConf > 0 {
		return (avg*2 + incidentConf) / 3
	}
	return avg
}
