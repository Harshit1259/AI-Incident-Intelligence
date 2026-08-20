package services

import (
	"testing"

	"ai-incident-platform/backend/internal/models"
)

func TestComputeEventFingerprint_SameEventSameFingerprint(t *testing.T) {
	event := models.Event{
		Source:   "prometheus",
		Service:  "payments-api",
		Severity: "critical",
		Title:    "Database connection pool exhausted",
	}
	fp1 := computeEventFingerprint(event)
	fp2 := computeEventFingerprint(event)
	if fp1 != fp2 {
		t.Errorf("same event produced different fingerprints: %q vs %q", fp1, fp2)
	}
}

func TestComputeEventFingerprint_DifferentEventsDifferent(t *testing.T) {
	event1 := models.Event{
		Source:   "prometheus",
		Service:  "payments-api",
		Severity: "critical",
		Title:    "Database connection pool exhausted",
	}
	event2 := models.Event{
		Source:   "prometheus",
		Service:  "checkout-api",
		Severity: "high",
		Title:    "Memory usage above threshold",
	}
	fp1 := computeEventFingerprint(event1)
	fp2 := computeEventFingerprint(event2)
	if fp1 == fp2 {
		t.Errorf("different events produced the same fingerprint: %q", fp1)
	}
}

func TestComputeEventFingerprint_NormalizationStripsInstanceTokens(t *testing.T) {
	// Two events with different pod names / IPs but same semantic alert
	event1 := models.Event{
		Source:   "prometheus",
		Service:  "payments-api",
		Severity: "critical",
		Title:    "High latency on pod abc123def456-xyz 10.0.0.1",
	}
	event2 := models.Event{
		Source:   "prometheus",
		Service:  "payments-api",
		Severity: "critical",
		Title:    "High latency on pod def789ghi012-uvw 10.0.0.2",
	}
	fp1 := computeEventFingerprint(event1)
	fp2 := computeEventFingerprint(event2)
	if fp1 != fp2 {
		t.Errorf("events differing only by instance tokens should have same fingerprint: %q vs %q", fp1, fp2)
	}
}

func TestComputeEventFingerprint_CaseInsensitive(t *testing.T) {
	event1 := models.Event{
		Source:   "Prometheus",
		Service:  "Payments-API",
		Severity: "CRITICAL",
		Title:    "Database Down",
	}
	event2 := models.Event{
		Source:   "prometheus",
		Service:  "payments-api",
		Severity: "critical",
		Title:    "database down",
	}
	fp1 := computeEventFingerprint(event1)
	fp2 := computeEventFingerprint(event2)
	if fp1 != fp2 {
		t.Errorf("case-insensitive events should have same fingerprint: %q vs %q", fp1, fp2)
	}
}

func TestComputeEventFingerprint_UsesMessageWhenTitleEmpty(t *testing.T) {
	event := models.Event{
		Source:   "webhook",
		Service:  "api-gateway",
		Severity: "high",
		Title:    "",
		Message:  "Connection refused to database",
	}
	fp := computeEventFingerprint(event)
	if fp == "" {
		t.Error("fingerprint should not be empty when title is empty but message exists")
	}
	if len(fp) != 16 {
		t.Errorf("fingerprint should be 16 hex chars, got %d: %q", len(fp), fp)
	}
}

func TestComputePriorityScore_CriticalHighest(t *testing.T) {
	critical := models.Incident{Severity: "critical", EventCount: 1, RiskScore: 50}
	low := models.Incident{Severity: "low", EventCount: 1, RiskScore: 50}

	critScore := computePriorityScore(critical)
	lowScore := computePriorityScore(low)

	if critScore <= lowScore {
		t.Errorf("critical (%d) should score higher than low (%d)", critScore, lowScore)
	}
}

func TestComputePriorityScore_CappedAt100(t *testing.T) {
	incident := models.Incident{
		Severity:    "critical",
		EventCount:  100,
		RiskScore:   100,
		SeenBefore:  true,
		ImpactCount: 50,
	}
	score := computePriorityScore(incident)
	if score > 100 {
		t.Errorf("priority score should be capped at 100, got %d", score)
	}
}

func TestComputePriorityScore_RecurrenceBonus(t *testing.T) {
	base := models.Incident{Severity: "medium", EventCount: 1, RiskScore: 50}
	recurring := models.Incident{Severity: "medium", EventCount: 1, RiskScore: 50, SeenBefore: true}

	baseScore := computePriorityScore(base)
	recurringScore := computePriorityScore(recurring)

	if recurringScore <= baseScore {
		t.Errorf("recurring incident (%d) should score higher than base (%d)", recurringScore, baseScore)
	}
	if recurringScore-baseScore != 10 {
		t.Errorf("recurrence bonus should be 10, got %d", recurringScore-baseScore)
	}
}

func TestSeverityWeight_AllLevels(t *testing.T) {
	tests := []struct {
		severity string
		want     int
	}{
		{"critical", 4},
		{"high", 3},
		{"medium", 2},
		{"low", 1},
		{"unknown", 0},
		{"", 0},
	}
	for _, tt := range tests {
		got := severityWeight(tt.severity)
		if got != tt.want {
			t.Errorf("severityWeight(%q) = %d, want %d", tt.severity, got, tt.want)
		}
	}
}

func TestSeverityWeight_CaseInsensitive(t *testing.T) {
	if severityWeight("CRITICAL") != severityWeight("critical") {
		t.Error("severityWeight should be case-insensitive")
	}
}
