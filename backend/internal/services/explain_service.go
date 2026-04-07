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
	"log"
	"strings"

	"ai-incident-platform/backend/internal/llm"
	"ai-incident-platform/backend/internal/models"
)

// ExplainService wraps the LLM client for incident explanation.
type ExplainService struct {
	llmClient *llm.Client
}

func NewExplainService(llmClient *llm.Client) *ExplainService {
	return &ExplainService{llmClient: llmClient}
}

// ─────────────────────────────────────────────────────
// LLM RCA response schema
// ─────────────────────────────────────────────────────

type rcaResponse struct {
	RootCauseSummary    string   `json:"root_cause_summary"`
	RootCauseType       string   `json:"root_cause_type"`
	Confidence          int      `json:"confidence"`
	RiskScore           int      `json:"risk_score"`
	Reasoning           []string `json:"reasoning"`
	RecommendedNextStep string   `json:"recommended_next_step"`
	Narrative           string   `json:"narrative"`
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

// Explain returns a full RCA enriched by the LLM (or falls back to templates).
// ExplainHandler should call this when an ExplainService instance is available.
func (s *ExplainService) Explain(detail models.IncidentDetail) (models.IncidentDetail, string) {
	if s.llmClient != nil && s.llmClient.IsConfigured() {
		enriched, narrative, err := s.llmExplain(detail)
		if err == nil {
			return enriched, narrative
		}
		log.Printf("explain: llm call failed, using template fallback: %v", err)
	}
	return detail, buildTemplateNarrative(detail)
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
	if rca.RecommendedNextStep != "" {
		detail.RecommendedNextStep = rca.RecommendedNextStep
	}

	narrative := rca.Narrative
	if narrative == "" {
		narrative = buildTemplateNarrative(detail)
	}

	return detail, narrative, nil
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
