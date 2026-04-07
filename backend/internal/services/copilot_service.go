package services

// CopilotService — Phase 1, Week 2
//
// Strategy:
//  1. Assemble full incident context via ContextAssemblyService.
//  2. Call Anthropic API with the assembled prompt.
//  3. Parse the structured JSON response.
//  4. Fall back to the original rule-based answers when:
//     - LLM is not configured (ANTHROPIC_API_KEY absent), or
//     - The API call fails (network error, timeout, etc.)
//
// This ensures the product is always usable, even in dev environments
// without an API key.

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"ai-incident-platform/backend/internal/llm"
	"ai-incident-platform/backend/internal/models"
)

// CopilotService answers operator questions about an incident.
type CopilotService struct {
	llmClient *llm.Client
}

func NewCopilotService(llmClient *llm.Client) *CopilotService {
	return &CopilotService{llmClient: llmClient}
}

// Answer answers the question using LLM when configured, falling back to
// rule-based answers otherwise.
func (s *CopilotService) Answer(detail models.IncidentDetail, question string) models.CopilotAnswer {
	if s.llmClient != nil && s.llmClient.IsConfigured() {
		answer, err := s.llmAnswer(detail, question)
		if err == nil {
			return answer
		}
		log.Printf("copilot: llm call failed, falling back to rule-based: %v", err)
	}
	return s.ruleBasedAnswer(detail, question)
}

// ─────────────────────────────────────────────────────
// LLM path
// ─────────────────────────────────────────────────────

// llmCopilotResponse mirrors the structured JSON the LLM returns.
type llmCopilotResponse struct {
	Intent             string   `json:"intent"`
	Answer             string   `json:"answer"`
	SuggestedFollowups []string `json:"suggested_followups"`
}

func (s *CopilotService) llmAnswer(detail models.IncidentDetail, question string) (models.CopilotAnswer, error) {
	ctx := AssembleForCopilot(detail, question)

	rawText, err := s.llmClient.CompleteWithSystem(ctx.SystemPrompt, ctx.UserPrompt)
	if err != nil {
		return models.CopilotAnswer{}, err
	}

	// Extract JSON from the response (model may wrap in markdown fences)
	jsonStr := extractJSON(rawText)

	var resp llmCopilotResponse
	if err := json.Unmarshal([]byte(jsonStr), &resp); err != nil {
		return models.CopilotAnswer{}, fmt.Errorf("copilot: parse llm json: %w — raw: %s", err, truncateCopilot(rawText, 200))
	}

	if resp.Answer == "" {
		return models.CopilotAnswer{}, fmt.Errorf("copilot: llm returned empty answer")
	}

	followups := resp.SuggestedFollowups
	if len(followups) == 0 {
		followups = defaultFollowups(resp.Intent)
	}

	return models.CopilotAnswer{
		Intent:             resp.Intent,
		Answer:             resp.Answer,
		SuggestedFollowups: followups,
	}, nil
}

// ─────────────────────────────────────────────────────
// Rule-based fallback (preserved from original implementation)
// ─────────────────────────────────────────────────────

func (s *CopilotService) ruleBasedAnswer(detail models.IncidentDetail, question string) models.CopilotAnswer {
	intent := classifyCopilotIntent(question)

	switch intent {
	case "why":
		return models.CopilotAnswer{
			Intent: "why",
			Answer: buildWhyAnswer(detail),
			SuggestedFollowups: []string{
				"What should I do first?",
				"What changed?",
				"Has this happened before?",
			},
		}
	case "first_action":
		return models.CopilotAnswer{
			Intent: "first_action",
			Answer: buildFirstActionAnswer(detail),
			SuggestedFollowups: []string{
				"Why is this happening?",
				"What changed?",
				"What else should I check?",
			},
		}
	case "change":
		return models.CopilotAnswer{
			Intent: "change",
			Answer: buildChangeAnswer(detail),
			SuggestedFollowups: []string{
				"Why is this happening?",
				"What should I do first?",
				"Has this happened before?",
			},
		}
	case "history":
		return models.CopilotAnswer{
			Intent: "history",
			Answer: buildHistoryAnswer(detail),
			SuggestedFollowups: []string{
				"What should I do first?",
				"Why is this happening?",
				"What changed?",
			},
		}
	default:
		return models.CopilotAnswer{
			Intent: "general",
			Answer: buildGeneralAnswer(detail),
			SuggestedFollowups: []string{
				"Why is this happening?",
				"What should I do first?",
				"What changed?",
			},
		}
	}
}

// ─────────────────────────────────────────────────────
// Intent classification (rule-based)
// ─────────────────────────────────────────────────────

func classifyCopilotIntent(question string) string {
	normalized := strings.ToLower(strings.TrimSpace(question))

	switch {
	case containsAnyPhrase(normalized, []string{
		"why is this happening", "why happening", "why did this happen",
		"root cause", "what is the cause", "why",
	}):
		return "why"

	case containsAnyPhrase(normalized, []string{
		"what should i do first", "what do i do first", "recommended action",
		"best action", "next action", "first action",
	}):
		return "first_action"

	case containsAnyPhrase(normalized, []string{
		"what changed", "recent change", "did something change",
		"deployment", "config change",
	}):
		return "change"

	case containsAnyPhrase(normalized, []string{
		"has this happened before", "seen before", "similar incident",
		"did this happen before", "history",
	}):
		return "history"

	default:
		return "general"
	}
}

func containsAnyPhrase(text string, phrases []string) bool {
	for _, phrase := range phrases {
		if strings.Contains(text, phrase) {
			return true
		}
	}
	return false
}

// ─────────────────────────────────────────────────────
// Rule-based answer builders
// ─────────────────────────────────────────────────────

func buildWhyAnswer(detail models.IncidentDetail) string {
	incident := detail.Incident
	summary := detail.Summary

	serviceName := humanizeCopilotServiceName(incident.Service)
	rootCause := strings.TrimSpace(summary.RootCauseSummary)
	if rootCause == "" {
		rootCause = "the current correlated evidence points to active instability"
	}

	answerParts := []string{
		fmt.Sprintf("%s is in a %s state because %s.", serviceName, strings.ToLower(incident.Severity), rootCause),
	}

	if detail.WhatChanged.Type != "" {
		answerParts = append(answerParts,
			fmt.Sprintf("The incident also aligns with a recent %s on %s.", detail.WhatChanged.Type, humanizeCopilotServiceName(detail.WhatChanged.Service)),
		)
	}

	if summary.SeenBefore {
		answerParts = append(answerParts,
			fmt.Sprintf("This pattern has been seen before %s, which increases confidence in the diagnosis.", copilotPluralizeTimes(summary.RecurringCount)),
		)
	}

	if len(detail.Insight.WhyThisIsLikely) > 0 {
		answerParts = append(answerParts,
			fmt.Sprintf("Key evidence includes %s.", joinEvidence(detail.Insight.WhyThisIsLikely, 2)),
		)
	}

	return strings.Join(answerParts, " ")
}

func buildFirstActionAnswer(detail models.IncidentDetail) string {
	if detail.PrimaryAction != nil {
		action := detail.PrimaryAction
		approvalText := "This action does not require approval."
		if action.RequiresApproval {
			approvalText = "This action requires approval before execution."
		}
		return fmt.Sprintf(
			"Start with %s. %s Risk level is %s. %s",
			action.Label, action.Description, action.RiskLevel, approvalText,
		)
	}
	if len(detail.Actions) > 0 {
		action := detail.Actions[0]
		return fmt.Sprintf("Start with %s. %s", action.Label, action.Description)
	}
	return "There is no recommended action available yet. Start by reviewing the incident explanation, recent changes, and correlated events."
}

func buildChangeAnswer(detail models.IncidentDetail) string {
	if detail.WhatChanged.Type == "" {
		return "No recent deployment, config, or infrastructure change is currently linked to this incident."
	}

	serviceName := humanizeCopilotServiceName(detail.WhatChanged.Service)
	answer := fmt.Sprintf("A recent %s is linked to this incident on %s.", detail.WhatChanged.Type, serviceName)

	if strings.TrimSpace(detail.WhatChanged.Description) != "" {
		answer += " " + detail.WhatChanged.Description + "."
	}
	if strings.TrimSpace(detail.WhatChanged.Timestamp) != "" {
		answer += " It was recorded at " + detail.WhatChanged.Timestamp + "."
	}
	return answer
}

func buildHistoryAnswer(detail models.IncidentDetail) string {
	if !detail.Summary.SeenBefore {
		return "This incident pattern has not been seen before in the recent history window."
	}

	answer := fmt.Sprintf("Yes. This pattern has been seen before %s.", copilotPluralizeTimes(detail.Summary.RecurringCount))

	if strings.TrimSpace(detail.Summary.SimilarIncidentID) != "" {
		answer += fmt.Sprintf(" The closest similar incident is %s.", detail.Summary.SimilarIncidentID)
	}
	if strings.TrimSpace(detail.Summary.LastSeenAt) != "" {
		answer += fmt.Sprintf(" It was last seen at %s.", detail.Summary.LastSeenAt)
	}
	return answer
}

func buildGeneralAnswer(detail models.IncidentDetail) string {
	serviceName := humanizeCopilotServiceName(detail.Incident.Service)
	rootCause := strings.TrimSpace(detail.Summary.RootCauseSummary)
	if rootCause == "" {
		rootCause = "the incident needs more evidence for a precise cause"
	}

	answer := fmt.Sprintf(
		"%s is currently in a %s incident state. The most likely cause is %s.",
		serviceName, strings.ToLower(detail.Incident.Severity), rootCause,
	)

	if detail.PrimaryAction != nil {
		answer += fmt.Sprintf(" The best first step is %s.", detail.PrimaryAction.Label)
	}
	return answer
}

// ─────────────────────────────────────────────────────
// Shared helpers
// ─────────────────────────────────────────────────────

func joinEvidence(values []string, limit int) string {
	if len(values) == 0 {
		return ""
	}
	if len(values) <= limit {
		return strings.Join(values, "; ")
	}
	return strings.Join(values[:limit], "; ")
}

func humanizeCopilotServiceName(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "This service"
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

func copilotPluralizeTimes(value int) string {
	if value == 1 {
		return "1 time"
	}
	return fmt.Sprintf("%d times", value)
}

func defaultFollowups(intent string) []string {
	switch intent {
	case "why":
		return []string{"What should I do first?", "What changed?", "Has this happened before?"}
	case "first_action":
		return []string{"Why is this happening?", "What changed?", "What else should I check?"}
	case "change":
		return []string{"Why is this happening?", "What should I do first?", "Has this happened before?"}
	case "history":
		return []string{"What should I do first?", "Why is this happening?", "What changed?"}
	default:
		return []string{"Why is this happening?", "What should I do first?", "What changed?"}
	}
}

// extractJSON pulls out the first JSON block from an LLM response.
// The model sometimes wraps output in ```json ... ``` fences.
func extractJSON(text string) string {
	// Try to find ```json ... ``` fence
	if idx := strings.Index(text, "```json"); idx != -1 {
		rest := text[idx+7:]
		if end := strings.Index(rest, "```"); end != -1 {
			return strings.TrimSpace(rest[:end])
		}
	}
	// Try generic ``` fence
	if idx := strings.Index(text, "```"); idx != -1 {
		rest := text[idx+3:]
		if end := strings.Index(rest, "```"); end != -1 {
			return strings.TrimSpace(rest[:end])
		}
	}
	// Assume raw JSON
	return strings.TrimSpace(text)
}

func truncateCopilot(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
