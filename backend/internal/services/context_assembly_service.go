package services

// ContextAssemblyService — Phase 1, Week 2
//
// Implements the context-window pipeline:
//   incident + events + change + pattern library → LLM prompt
//
// The assembled context is injected into both the Copilot and Explain services
// so that Claude receives rich, structured incident context before answering.

import (
	"fmt"
	"strings"

	"ai-incident-platform/backend/internal/models"
)

// AssembledContext is the fully-built prompt context passed to LLM services.
type AssembledContext struct {
	SystemPrompt string // Persona + output format instructions
	UserPrompt   string // Full incident context as the user turn
	Signatures   []Signature
}

// AssembleForRCA builds context for root-cause-analysis (explain endpoint).
func AssembleForRCA(detail models.IncidentDetail) AssembledContext {
	incident := detail.Incident

	// 1. Match failure signatures against all text signals
	corpus := buildCorpus(detail)
	matched := MatchSignatures(corpus)

	// 2. Build system prompt
	system := rcaSystemPrompt()

	// 3. Build user prompt with all structured context
	var sb strings.Builder

	sb.WriteString("## Incident Context\n\n")
	sb.WriteString(fmt.Sprintf("**ID:** %s\n", incident.ID))
	sb.WriteString(fmt.Sprintf("**Title:** %s\n", incident.Title))
	sb.WriteString(fmt.Sprintf("**Service:** %s\n", incident.Service))
	sb.WriteString(fmt.Sprintf("**Severity:** %s\n", incident.Severity))
	sb.WriteString(fmt.Sprintf("**Status:** %s\n", incident.Status))
	sb.WriteString(fmt.Sprintf("**Event Count:** %d\n", incident.EventCount))
	sb.WriteString(fmt.Sprintf("**First Event:** %s\n", incident.FirstEventTime))
	sb.WriteString(fmt.Sprintf("**Last Event:** %s\n\n", incident.LastEventTime))

	if incident.RootCauseSummary != "" {
		sb.WriteString(fmt.Sprintf("**Existing Root Cause Summary:** %s\n\n", incident.RootCauseSummary))
	}

	// Events section
	if len(detail.Events) > 0 {
		sb.WriteString("## Alert / Event Timeline\n\n")
		for i, te := range detail.Events {
			if i >= 10 {
				sb.WriteString(fmt.Sprintf("... and %d more events\n", len(detail.Events)-10))
				break
			}
			sb.WriteString(fmt.Sprintf("- [%s] (%s) %s: %s\n",
				te.Event.Timestamp.Format("15:04:05"),
				te.Event.Severity,
				te.Event.Service,
				firstNonEmpty(te.Event.Title, te.Event.Message),
			))
		}
		sb.WriteString("\n")
	}

	// Change context
	if detail.WhatChanged.Type != "" {
		sb.WriteString("## Recent Change\n\n")
		sb.WriteString(fmt.Sprintf("**Type:** %s\n", detail.WhatChanged.Type))
		sb.WriteString(fmt.Sprintf("**Service:** %s\n", detail.WhatChanged.Service))
		if detail.WhatChanged.Description != "" {
			sb.WriteString(fmt.Sprintf("**Description:** %s\n", detail.WhatChanged.Description))
		}
		if detail.WhatChanged.Timestamp != "" {
			sb.WriteString(fmt.Sprintf("**Timestamp:** %s\n", detail.WhatChanged.Timestamp))
		}
		sb.WriteString("\n")
	}

	// Matched patterns
	if len(matched) > 0 {
		sb.WriteString("## Matched Failure Patterns\n\n")
		for _, sig := range matched {
			sb.WriteString(fmt.Sprintf("- **[%s/%s]** Cause: %s | Remediation: %s\n",
				sig.Category, sig.ID, sig.Cause, sig.Remediation))
		}
		sb.WriteString("\n")
	}

	// Recurrence
	if incident.SeenBefore {
		sb.WriteString(fmt.Sprintf("## Recurrence\n\nThis pattern has been seen %d time(s) previously.\n\n",
			incident.RecurringCount))
	}

	sb.WriteString("## Task\n\n")
	sb.WriteString("Analyse the incident context above and return a structured JSON response:\n\n")
	sb.WriteString("```json\n")
	sb.WriteString(`{
  "root_cause_summary": "<1-2 sentence summary of root cause>",
  "root_cause_type": "<one of: database_issue|network_issue|memory_pressure|cpu_spike|deployment_regression|dependency_failure|configuration_error|timeout_cascade|unknown>",
  "confidence": <integer 0-100>,
  "risk_score": <integer 0-100>,
  "reasoning": ["<evidence point 1>", "<evidence point 2>", "<evidence point 3>"],
  "recommended_next_step": "<single most important immediate action>",
  "narrative": "<3-4 sentence human-readable incident narrative>",
  "claims": [
    {
      "claim": "<discrete assertion, e.g. 'Database connection pool exhaustion is causing timeouts'>",
      "confidence": <integer 0-100>,
      "reasoning": "<why you believe this — cite specific signals from the context>",
      "falsified_by": "<one concrete test that would prove this claim wrong>",
      "evidence_ids": ["<IDs of evidence items that support this claim>"]
    }
  ],
  "conflicting_signals": [
    {
      "signal": "<data point that contradicts the diagnosis>",
      "source": "<field name or signal type>",
      "strength": <integer 0-100>,
      "resolution": "<how this contradiction was resolved in the final diagnosis>"
    }
  ],
  "recommendation_why": "<explicit reasoning: why THIS action, not alternatives>",
  "recommendation_priority": "<immediate|soon|investigate>",
  "recommendation_risk": "<low|medium|high>"
}`)
	sb.WriteString("\n```\n")
	sb.WriteString("\nRespond with ONLY the JSON block. No preamble, no explanation.")

	return AssembledContext{
		SystemPrompt: system,
		UserPrompt:   sb.String(),
		Signatures:   matched,
	}
}

// AssembleForCopilot builds context for an interactive copilot question.
func AssembleForCopilot(detail models.IncidentDetail, question string) AssembledContext {
	incident := detail.Incident

	corpus := buildCorpus(detail)
	matched := MatchSignatures(corpus)

	system := copilotSystemPrompt()

	var sb strings.Builder

	sb.WriteString("## Incident\n\n")
	sb.WriteString(fmt.Sprintf("**%s** — %s %s (status: %s, %d events)\n\n",
		incident.Title, incident.Severity, incident.Service, incident.Status, incident.EventCount))

	if incident.RootCauseSummary != "" {
		sb.WriteString(fmt.Sprintf("Root cause: %s\n\n", incident.RootCauseSummary))
	}

	if detail.WhatChanged.Type != "" {
		sb.WriteString(fmt.Sprintf("Recent change: %s on %s — %s\n\n",
			detail.WhatChanged.Type, detail.WhatChanged.Service, detail.WhatChanged.Description))
	}

	if detail.PrimaryAction != nil {
		sb.WriteString(fmt.Sprintf("Recommended action: %s — %s (risk: %s)\n\n",
			detail.PrimaryAction.Label, detail.PrimaryAction.Description, detail.PrimaryAction.RiskLevel))
	}

	if len(matched) > 0 {
		sb.WriteString("Matched failure patterns: ")
		names := make([]string, 0, len(matched))
		for _, sig := range matched {
			names = append(names, sig.ID)
		}
		sb.WriteString(strings.Join(names, ", "))
		sb.WriteString("\n\n")
	}

	if incident.SeenBefore {
		sb.WriteString(fmt.Sprintf("Recurrence: seen %d time(s) previously.\n\n", incident.RecurringCount))
	}

	sb.WriteString("## Operator Question\n\n")
	sb.WriteString(question)
	sb.WriteString("\n\n")
	sb.WriteString("## Task\n\n")
	sb.WriteString("Return a structured JSON response:\n\n```json\n")
	sb.WriteString(`{
  "intent": "<why|first_action|change|history|general>",
  "answer": "<concise, actionable answer — 2-4 sentences>",
  "suggested_followups": ["<followup question 1>", "<followup question 2>", "<followup question 3>"],
  "answer_confidence": <integer 0-100>,
  "answer_reasoning": "<why this answer is correct — cite specific evidence>",
  "falsified_by": "<one concrete observation that would prove this answer wrong>",
  "conflicting_signals": [
    {
      "signal": "<data point that contradicts or complicates this answer>",
      "source": "<field name>",
      "strength": <integer 0-100>,
      "resolution": "<how you resolved this in the answer>"
    }
  ]
}`)
	sb.WriteString("\n```\nRespond with ONLY the JSON block.")

	return AssembledContext{
		SystemPrompt: system,
		UserPrompt:   sb.String(),
		Signatures:   matched,
	}
}

// ─────────────────────────────────────────────────────
// System prompts
// ─────────────────────────────────────────────────────

func rcaSystemPrompt() string {
	return `You are an expert SRE (Site Reliability Engineer) and incident analyst.
You have deep knowledge of distributed systems, databases, container orchestration,
and common failure patterns in production infrastructure.

Your job is to analyse incident data and provide an accurate, evidence-based
root cause analysis (RCA). Be concise, technically precise, and actionable.
Always base conclusions on the evidence provided — do not speculate beyond it.`
}

func copilotSystemPrompt() string {
	return `You are an AI incident copilot assisting an on-call engineer.
You have deep knowledge of distributed systems and production operations.
Your answers must be concise (2-4 sentences), technically precise, and immediately actionable.
The engineer is under time pressure — do not be verbose.`
}

// ─────────────────────────────────────────────────────
// Helpers
// ─────────────────────────────────────────────────────

// buildCorpus creates a single string from all incident text fields for
// signature matching.
func buildCorpus(detail models.IncidentDetail) string {
	parts := []string{
		detail.Incident.Title,
		detail.Incident.Service,
		detail.Incident.RootCauseSummary,
		detail.WhatChanged.Type,
		detail.WhatChanged.Description,
	}

	for _, te := range detail.Events {
		parts = append(parts, te.Event.Title, te.Event.Message)
	}

	for _, e := range detail.Evidence {
		parts = append(parts, e)
	}

	return strings.Join(parts, " ")
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
