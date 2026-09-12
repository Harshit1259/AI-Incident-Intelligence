package models

import "time"

type IncidentSummary struct {
	EventCount         int      `json:"event_count"`
	Service            string   `json:"service"`
	Severity           string   `json:"severity"`
	LatestEventTime    string   `json:"latest_event_time"`
	CorrelationScore   int      `json:"correlation_score"`
	CorrelationReason  string   `json:"correlation_reason"`
	CorrelationPattern string   `json:"correlation_pattern"`
	Confidence         int      `json:"confidence"`
	RiskScore          int      `json:"risk_score"`
	RootCauseSummary   string   `json:"root_cause_summary"`
	RootCauseType      string   `json:"root_cause_type"`
	ImpactedServices   []string `json:"impacted_services"`
	ImpactCount        int      `json:"impact_count"`

	SeenBefore        bool   `json:"seen_before"`
	RecurringCount    int    `json:"recurring_count"`
	SimilarIncidentID string `json:"similar_incident_id"`
	LastSeenAt        string `json:"last_seen_at"`
	PriorityScore     int    `json:"priority_score"`
}

type TimelineEvent struct {
	Event           Event  `json:"event"`
	SignalType      string `json:"signal_type"`
	StageType       string `json:"stage_type"`
	GapFromPrevious string `json:"gap_from_previous"`
	StoryLabel      string `json:"story_label"`
}

type DecisionCard struct {
	Title            string `json:"title"`
	Cause            string `json:"cause"`
	Confidence       int    `json:"confidence"`
	RiskScore        int    `json:"risk_score"`
	ImpactCount      int    `json:"impact_count"`
	WhatChangedLabel string `json:"what_changed_label"`
	Status           string `json:"status"`
	Severity         string `json:"severity"`

	SeenBefore     bool `json:"seen_before"`
	RecurringCount int  `json:"recurring_count"`
}

type WhatChanged struct {
	Type        string `json:"type"`
	Service     string `json:"service"`
	Version     string `json:"version"`
	Description string `json:"description"`
	Timestamp   string `json:"timestamp"`
}

type IncidentStatusAudit struct {
	ID             int    `json:"id"`
	IncidentID     string `json:"incident_id"`
	PreviousStatus string `json:"previous_status"`
	NewStatus      string `json:"new_status"`
	Note           string `json:"note"`
	ChangedBy      string `json:"changed_by"`
	ChangedAt      string `json:"changed_at"`
}

type GraphNode struct {
	ID           string `json:"id"`
	Label        string `json:"label"`
	NodeType     string `json:"node_type"`      // service | dependency | impacted_service | change | alert_origin | evidence
	Severity     string `json:"severity,omitempty"`
	Status       string `json:"status,omitempty"`       // healthy | degraded | down | unknown
	AlertCount   int    `json:"alert_count,omitempty"`
	ChangeLinked bool   `json:"change_linked,omitempty"` // true if a change event is linked to this node
	EvidenceCount int   `json:"evidence_count,omitempty"`
}

type GraphEdge struct {
	From          string  `json:"from"`
	To            string  `json:"to"`
	Relation      string  `json:"relation"`
	Weight        float64 `json:"weight,omitempty"`      // 0.0–1.0: signal strength
	Confidence    int     `json:"confidence,omitempty"`  // 0–100: how confident we are in this edge
	EvidenceCount int     `json:"evidence_count,omitempty"`
}

type GraphData struct {
	Nodes      []GraphNode `json:"nodes"`
	Edges      []GraphEdge `json:"edges"`
	RootNodeID string      `json:"root_node_id,omitempty"` // primary incident service
	BuiltAt    string      `json:"built_at,omitempty"`
}

type ContextLogEntry struct {
	Timestamp string `json:"timestamp"`
	Category  string `json:"category"`
	Message   string `json:"message"`
	Source    string `json:"source"`
}

type ContextMetrics struct {
	CPUPercent    float64 `json:"cpu_percent"`
	MemoryPercent float64 `json:"memory_percent"`
	DiskPercent   float64 `json:"disk_percent"`
	LoadAvg1      float64 `json:"load_avg_1"`
	CollectedAt   string  `json:"collected_at"`
}

// EvidenceRef is a pointer to a concrete data artifact (event, log, metric) that
// supports an RCA conclusion. Unlike free-text reasoning, these are verifiable.
type EvidenceRef struct {
	Type      string `json:"type"`      // "event" | "log" | "metric" | "change"
	ID        string `json:"id"`        // event ID, log entry index, metric name
	Timestamp string `json:"timestamp"` // RFC3339
	Label     string `json:"label"`     // human-readable short label
	Detail    string `json:"detail"`    // one-line content excerpt
}

type LifecycleEntry struct {
	Stage     string `json:"stage"`     // detected, acknowledged, investigating, mitigated, resolved
	Timestamp string `json:"timestamp"`
	Duration  string `json:"duration"`  // time since previous stage
	Actor     string `json:"actor"`     // who/what triggered this
}

type DiscoveredService struct {
	Service       string `json:"service"`
	IncidentCount int    `json:"incident_count"`
	MaxSeverity   string `json:"max_severity"`
	LastIncident  string `json:"last_incident"`
	SuggestedTier string `json:"suggested_tier"`
}

type IncidentDetail struct {
	Incident             Incident              `json:"incident"`
	Events               []TimelineEvent       `json:"events"`
	Summary              IncidentSummary       `json:"summary"`
	Insight              IncidentInsight       `json:"insight"`
	Impact               ImpactAnalysis        `json:"impact"`
	Actions              []Action              `json:"actions"`
	PrimaryAction        *Action               `json:"primary_action"`
	DecisionCard         DecisionCard          `json:"decision_card"`
	WhatChanged          WhatChanged           `json:"what_changed"`
	Narrative            string                `json:"narrative"`
	StatusAudit          []IncidentStatusAudit `json:"status_audit"`
	Graph                GraphData             `json:"graph"`
	Evidence             []string              `json:"evidence"`
	EvidenceRefs         []EvidenceRef         `json:"evidence_refs"`
	RecommendedNextStep  string                `json:"recommended_next_step"`
	ResolutionSteps      []string              `json:"resolution_steps"`
	OccurrenceTimes      []string              `json:"occurrence_times"`
	ContextLogs          []ContextLogEntry     `json:"context_logs"`
	ContextMetrics       *ContextMetrics       `json:"context_metrics"`
	Lifecycle            []LifecycleEntry      `json:"lifecycle"`
	ActionExecutions     []ActionExecution     `json:"action_executions"`

	// Phase 3 additions
	TopologyGraph    *GraphData            `json:"topology_graph,omitempty"`  // enriched evidence-linked topology
	PatternHistory   *PatternHistory       `json:"pattern_history,omitempty"` // incident memory: what fixed it before
	EvidenceGraph    *EvidenceGraph        `json:"evidence_graph,omitempty"`  // AI reasoning provenance: sources, per-claim confidence, contradictions

	// Feature 4: real-time dollar breakdown — $/min rate, SLA countdown, total running cost
	LiveBusinessImpact *LiveBusinessImpact `json:"live_business_impact,omitempty"`

	// Analysis provenance — tells the client which tier produced the causal
	// claim (if any). Always populated; see AnalysisProvenance.
	Provenance AnalysisProvenance `json:"provenance"`

	// AutoCloseAt is set while every alert in the incident has recovered and
	// the incident will close automatically at this time unless one fires again.
	AutoCloseAt *time.Time `json:"auto_close_at,omitempty"`
}

// PatternHistory summarises past resolutions for this incident's fingerprint/service.
type PatternHistory struct {
	Fingerprint       string             `json:"fingerprint"`
	Service           string             `json:"service"`
	RecurrenceCount   int                `json:"recurrence_count"`
	SuggestedPlaybook []string           `json:"suggested_playbook"` // ordered action steps from past resolutions
	PastResolutions   []ResolutionRecord `json:"past_resolutions"`
}

// ResolutionRecord captures how an incident was resolved in the past.
type ResolutionRecord struct {
	IncidentID     string   `json:"incident_id"`
	ResolvedAt     string   `json:"resolved_at"`
	TTRSeconds     int      `json:"ttr_seconds"`
	ActionsTaken   []string `json:"actions_taken"`
	ResolutionNote string   `json:"resolution_note"`
}

// ─────────────────────────────────────────────────────────────────────────────
// Analysis provenance
//
// The platform can describe an incident at three different levels of epistemic
// strength, and the client must be able to tell them apart:
//
//   observed       — facts the system collected (alerts, changes, recurrence).
//                    Always true. Carries NO causal claim.
//   knowledge_base — a pre-authored pattern matched. Deterministic, human-written.
//   llm            — a model reasoned over the assembled evidence.
//
// A narrative built from templates is "observed": it may summarise, but it must
// never assert a cause. Only knowledge_base and llm may populate RCAConfidence.
// ─────────────────────────────────────────────────────────────────────────────

const (
	AnalysisSourceObserved = "observed"
	AnalysisSourceKB       = "knowledge_base"
	AnalysisSourceLLM      = "llm"
)

// AnalysisProvenance records who or what produced the causal claim on an
// incident, and when. It is attached to every explain/analyze response so the
// UI can badge the answer rather than presenting all three tiers identically.
type AnalysisProvenance struct {
	// Source is one of AnalysisSourceObserved | AnalysisSourceKB | AnalysisSourceLLM.
	Source string `json:"source"`

	// HasCausalClaim is false for the observed tier. When false the client must
	// not render a "Root cause" heading or a confidence percentage.
	HasCausalClaim bool `json:"has_causal_claim"`

	// RCAConfidence is how confident we are in the CAUSE. It is deliberately a
	// pointer: nil means "nothing analysed this", which is different from zero.
	// Never derived from severity. Only set by the KB or LLM tiers.
	RCAConfidence *int `json:"rca_confidence"`

	// CorrelationConfidence is how confident we are that these alerts belong
	// together. This is what the +5-per-merge score legitimately measures, and
	// it is meaningful on every tier including observed.
	CorrelationConfidence int `json:"correlation_confidence"`

	// Model, AnalyzedAt and AnalyzedBy are populated on the llm tier so the
	// analysis is an auditable artifact rather than an anonymous assertion.
	Model      string `json:"model,omitempty"`
	AnalyzedAt string `json:"analyzed_at,omitempty"`
	AnalyzedBy string `json:"analyzed_by,omitempty"`

	// KBEntryID is set on the knowledge_base tier so an operator can inspect
	// the exact pattern that matched.
	KBEntryID string `json:"kb_entry_id,omitempty"`

	// Unavailable explains why no analysis exists, so the UI can offer a retry
	// instead of a blank panel (e.g. "LLM unreachable: context deadline exceeded").
	Unavailable string `json:"unavailable,omitempty"`
}

// ObservedProvenance builds the zero-claim provenance for the observation tier.
func ObservedProvenance(correlationConfidence int, unavailable string) AnalysisProvenance {
	return AnalysisProvenance{
		Source:                AnalysisSourceObserved,
		HasCausalClaim:        false,
		RCAConfidence:         nil,
		CorrelationConfidence: correlationConfidence,
		Unavailable:           unavailable,
	}
}
