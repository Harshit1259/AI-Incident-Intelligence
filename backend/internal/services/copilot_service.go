package services

import (
	"fmt"
	"strings"

	"ai-incident-platform/backend/internal/models"
)

type CopilotService struct{}

func NewCopilotService() *CopilotService {
	return &CopilotService{}
}

func (copilotService *CopilotService) Answer(detail models.IncidentDetail, question string) models.CopilotAnswer {
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

func classifyCopilotIntent(question string) string {
	normalized := strings.ToLower(strings.TrimSpace(question))

	switch {
	case containsAnyPhrase(normalized, []string{
		"why is this happening",
		"why happening",
		"why did this happen",
		"root cause",
		"what is the cause",
		"why",
	}):
		return "why"

	case containsAnyPhrase(normalized, []string{
		"what should i do first",
		"what do i do first",
		"recommended action",
		"best action",
		"next action",
		"first action",
	}):
		return "first_action"

	case containsAnyPhrase(normalized, []string{
		"what changed",
		"recent change",
		"did something change",
		"deployment",
		"config change",
	}):
		return "change"

	case containsAnyPhrase(normalized, []string{
		"has this happened before",
		"seen before",
		"similar incident",
		"did this happen before",
		"history",
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
			action.Label,
			action.Description,
			action.RiskLevel,
			approvalText,
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
	answer := fmt.Sprintf(
		"A recent %s is linked to this incident on %s.",
		detail.WhatChanged.Type,
		serviceName,
	)

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

	answer := fmt.Sprintf(
		"Yes. This pattern has been seen before %s.",
		copilotPluralizeTimes(detail.Summary.RecurringCount),
	)

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
		serviceName,
		strings.ToLower(detail.Incident.Severity),
		rootCause,
	)

	if detail.PrimaryAction != nil {
		answer += fmt.Sprintf(" The best first step is %s.", detail.PrimaryAction.Label)
	}

	return answer
}

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

	parts := strings.FieldsFunc(trimmed, splitCopilotServiceName)
	for index, part := range parts {
		if len(part) == 0 {
			continue
		}
		parts[index] = strings.ToUpper(part[:1]) + strings.ToLower(part[1:])
	}

	return strings.Join(parts, " ")
}

func splitCopilotServiceName(r rune) bool {
	if r == '-' {
		return true
	}
	if r == '_' {
		return true
	}
	return false
}

func copilotPluralizeTimes(value int) string {
	if value == 1 {
		return "1 time"
	}
	return fmt.Sprintf("%d times", value)
}
