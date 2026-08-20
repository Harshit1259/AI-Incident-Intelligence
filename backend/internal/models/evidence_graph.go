package models

// EvidenceGraph is attached to every AI output — explain, copilot, and RCA.
// It makes AI reasoning auditable by recording what data was used, how
// confident each claim is, what contradicts the diagnosis, why the
// recommendation was chosen, and what evidence would falsify it.
//
// The frontend renders this as an "Evidence" panel below every AI answer.

// EvidencedClaim is one discrete assertion the AI made, with its supporting
// evidence, an individual confidence score, and an explicit falsifiability test.
type EvidencedClaim struct {
	Claim       string       `json:"claim"`        // "Database latency is the primary cause"
	Confidence  int          `json:"confidence"`   // 0–100
	Reasoning   string       `json:"reasoning"`    // "The deployment happened 12 min before first alert"
	FalsifiedBy string       `json:"falsified_by"` // "Rolling back v2.3.1 within 5 min does not reduce errors"
	EvidenceIDs []string     `json:"evidence_ids"` // IDs from the sources_used list
	EvidenceRefs []EvidenceRef `json:"evidence_refs,omitempty"` // resolved refs (populated server-side)
}

// ConflictingSignal is data that contradicts the main diagnosis.
// Showing contradictions builds trust — it proves the system examined counter-evidence.
type ConflictingSignal struct {
	Signal     string `json:"signal"`     // "Only 1 event despite critical severity"
	Source     string `json:"source"`     // "event_count"
	Strength   int    `json:"strength"`   // 0–100: how strongly this contradicts the diagnosis
	Resolution string `json:"resolution"` // "A single failed health check can cascade via auto-scaling"
}

// EvidenceRecommendation is the recommended action enriched with explicit "why" reasoning.
type EvidenceRecommendation struct {
	Action    string `json:"action"`     // What to do: "Roll back payment-service to v2.3.0"
	Why       string `json:"why"`        // "The change timestamp directly precedes the first alert by 12 min"
	Priority  string `json:"priority"`   // immediate | soon | investigate
	RiskLevel string `json:"risk_level"` // low | medium | high
}

// EvidenceGraph wraps an AI output with full epistemic provenance so every
// claim can be traced back to real data and every recommendation can be
// challenged with a concrete falsifiability criterion.
type EvidenceGraph struct {
	// Data artifacts the AI received as input
	SourcesUsed []EvidenceRef `json:"sources_used"`

	// Per-claim assertions with individual confidence + falsifiability
	Claims []EvidencedClaim `json:"claims"`

	// Signals that point away from the diagnosis
	ConflictingSignals []ConflictingSignal `json:"conflicting_signals"`

	// The recommendation the AI made, with explicit justification
	Recommendation *EvidenceRecommendation `json:"recommendation,omitempty"`

	// Weighted average confidence across all claims
	OverallConfidence int `json:"overall_confidence"`

	// How this graph was built: "llm", "rule_based", or "hybrid"
	Method      string `json:"method"`
	GeneratedAt string `json:"generated_at"`
}
