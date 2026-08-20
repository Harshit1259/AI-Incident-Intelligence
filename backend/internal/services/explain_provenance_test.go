package services

import (
	"strings"
	"testing"
	"time"

	"ai-incident-platform/backend/internal/models"
)

// buildUnmatchableDetail returns an incident whose text cannot match any
// knowledge-base entry, forcing the observation tier. The severity is critical
// so Confidence is seeded at 75 — the value that used to leak out as a fake
// "75% confident" root cause.
func buildUnmatchableDetail() models.IncidentDetail {
	now := time.Now()
	incident := models.Incident{
		ID:               "inc-test-1",
		Title:            "zzqqxx unmatchable alert token",
		Service:          "payments-api",
		Severity:         "critical",
		Status:           "open",
		FirstEventTime:   now.Add(-4 * time.Minute),
		LastEventTime:    now,
		EventCount:       3,
		Confidence:       75, // severity-seeded, NOT a measure of cause
		RootCauseSummary: "zzqqxx unmatchable alert token",
		Fingerprint:      "abc123def4567890",
	}
	return models.IncidentDetail{
		Incident: incident,
		Summary: models.IncidentSummary{
			Confidence:       75,
			RootCauseSummary: "zzqqxx unmatchable alert token",
			RecurringCount:   2,
		},
		DecisionCard: models.DecisionCard{Cause: "zzqqxx unmatchable alert token"},
	}
}

// TestObservationTier_AssertsNoCause is the regression test for the false-end
// bug: when nothing has analysed an incident, the response must not contain a
// root cause or an RCA confidence, because neither exists.
func TestObservationTier_AssertsNoCause(t *testing.T) {
	svc := NewExplainService(nil) // no LLM configured
	svc.SetKnowledgeBase(NewKnowledgeBase())

	detail, narrative := svc.Explain(buildUnmatchableDetail())

	if detail.Provenance.Source != models.AnalysisSourceObserved {
		t.Fatalf("expected source %q, got %q",
			models.AnalysisSourceObserved, detail.Provenance.Source)
	}
	if detail.Provenance.HasCausalClaim {
		t.Error("observation tier must not claim a cause")
	}
	if detail.Provenance.RCAConfidence != nil {
		t.Errorf("RCAConfidence must be nil when nothing analysed the incident, got %d",
			*detail.Provenance.RCAConfidence)
	}
	if detail.Incident.RootCauseSummary != "" {
		t.Errorf("root cause must be cleared on the observation tier, got %q",
			detail.Incident.RootCauseSummary)
	}
	if detail.Summary.RootCauseSummary != "" {
		t.Errorf("summary root cause must be cleared, got %q", detail.Summary.RootCauseSummary)
	}
	if detail.DecisionCard.Cause != "" {
		t.Errorf("decision card cause must be cleared, got %q", detail.DecisionCard.Cause)
	}

	// Correlation confidence is legitimate and must survive — it measures that
	// these alerts belong together, which we do know.
	if detail.Provenance.CorrelationConfidence != 75 {
		t.Errorf("correlation confidence should be preserved, got %d",
			detail.Provenance.CorrelationConfidence)
	}

	// The narrative may summarise observations but must never assert causation.
	lower := strings.ToLower(narrative)
	for _, banned := range []string{"root cause is", "caused by", "due to", "because of"} {
		if strings.Contains(lower, banned) {
			t.Errorf("observation narrative must not assert causation, found %q in: %s",
				banned, narrative)
		}
	}
	if !strings.Contains(lower, "no root-cause analysis has been run") {
		t.Errorf("narrative should state that no analysis has run, got: %s", narrative)
	}
}

// TestObservationTier_ReportsRealFacts confirms the tier is useful, not just
// safe: the observations it does report must be true and present.
func TestObservationTier_ReportsRealFacts(t *testing.T) {
	svc := NewExplainService(nil)
	svc.SetKnowledgeBase(NewKnowledgeBase())

	d := buildUnmatchableDetail()
	d.WhatChanged = models.WhatChanged{
		Type:        "deployment",
		Service:     "payments-api",
		Description: "v2.4.1 rollout",
	}
	_, narrative := svc.Explain(d)

	for _, want := range []string{"payments-api", "deployment", "v2.4.1 rollout"} {
		if !strings.Contains(narrative, want) {
			t.Errorf("narrative should report observed fact %q, got: %s", want, narrative)
		}
	}
	// Recurrence is a real, purely factual signal and should be surfaced.
	if !strings.Contains(narrative, "seen 2 time(s) before") {
		t.Errorf("narrative should report recurrence, got: %s", narrative)
	}
}

// TestAnalyze_WithoutLLM_DegradesHonestly confirms that asking for analysis
// when no model is configured returns the observation tier with a reason,
// rather than inventing an answer.
func TestAnalyze_WithoutLLM_DegradesHonestly(t *testing.T) {
	svc := NewExplainService(nil)
	svc.SetKnowledgeBase(NewKnowledgeBase())

	detail, _ := svc.Analyze(buildUnmatchableDetail(), "user-1")

	if detail.Provenance.HasCausalClaim {
		t.Error("analysis without a configured LLM must not produce a causal claim")
	}
	if detail.Provenance.RCAConfidence != nil {
		t.Error("RCAConfidence must stay nil when analysis could not run")
	}
	if detail.Provenance.Unavailable == "" {
		t.Error("provenance must explain why no analysis is present so the UI can offer a retry")
	}
}

// TestKnowledgeBaseTier_CarriesProvenance confirms a KB match is labelled as
// such, with the entry id, so an operator can inspect the matched pattern.
func TestKnowledgeBaseTier_CarriesProvenance(t *testing.T) {
	kb := NewKnowledgeBase()
	svc := NewExplainService(nil)
	svc.SetKnowledgeBase(kb)

	// Find a real entry and build an incident that matches its keywords, so the
	// test stays valid as KB content changes.
	entries := kb.entries
	if len(entries) == 0 {
		t.Skip("knowledge base is empty")
	}
	var target *KnowledgeEntry
	for i := range entries {
		if len(entries[i].Keywords) >= 2 {
			target = &entries[i]
			break
		}
	}
	if target == nil {
		t.Skip("no multi-keyword entry available")
	}

	d := buildUnmatchableDetail()
	d.Incident.Title = strings.Join(target.Keywords, " ")
	d.Incident.RootCauseSummary = d.Incident.Title

	detail, _ := svc.Explain(d)

	if detail.Provenance.Source != models.AnalysisSourceKB {
		t.Fatalf("expected a knowledge_base match for keywords %v, got source %q",
			target.Keywords, detail.Provenance.Source)
	}
	if !detail.Provenance.HasCausalClaim {
		t.Error("a knowledge-base match is a real causal claim")
	}
	if detail.Provenance.RCAConfidence == nil {
		t.Fatal("KB tier must carry an RCA confidence")
	}
	if detail.Provenance.KBEntryID == "" {
		t.Error("KB tier must name the matched entry so an operator can inspect it")
	}
}
