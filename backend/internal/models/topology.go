package models

import "time"

// ─── Node & Edge Catalog types ───────────────────────────────────────────────

// TopologyNode is the DB-backed representation of any node in the service graph:
// services, infra hosts, pods, deployments, teams, databases, queues, caches, externals.
type TopologyNode struct {
	ID               string            `json:"id"`
	TenantID         string            `json:"tenant_id"`
	NodeType         string            `json:"node_type"`         // service | infra | pod | deployment | team | database | queue | cache | external
	Name             string            `json:"name"`
	DisplayName      string            `json:"display_name"`
	Tier             string            `json:"tier"`              // critical | internal | external | infra
	OwnerTeam        string            `json:"owner_team"`
	IsCustomerFacing bool              `json:"is_customer_facing"`
	Version          string            `json:"version"`
	Environment      string            `json:"environment"`
	Metadata         map[string]string `json:"metadata,omitempty"`
	AutoDiscovered   bool              `json:"auto_discovered"`
	Health           *NodeHealthSnap   `json:"health,omitempty"`
	CreatedAt        time.Time         `json:"created_at"`
	UpdatedAt        time.Time         `json:"updated_at"`
}

// NodeHealthSnap is the latest health sample for a node, joined on read.
type NodeHealthSnap struct {
	Status        string    `json:"status"`
	CPUPct        float64   `json:"cpu_pct"`
	MemPct        float64   `json:"mem_pct"`
	ErrorRatePct  float64   `json:"error_rate_pct"`
	LatencyMsP99  float64   `json:"latency_ms_p99"`
	RecordedAt    time.Time `json:"recorded_at"`
}

// TopologyEdge is the DB-backed directed edge between two topology nodes.
type TopologyEdge struct {
	ID                string    `json:"id"`
	TenantID          string    `json:"tenant_id"`
	FromNodeID        string    `json:"from_node_id"`
	ToNodeID          string    `json:"to_node_id"`
	Relation          string    `json:"relation"`           // calls | depends_on | runs_on | deployed_by | owned_by | change_caused | routes_to
	Weight            float64   `json:"weight"`
	Confidence        int       `json:"confidence"`
	PropagationFactor float64   `json:"propagation_factor"` // 0.0–1.0: signal decay per hop
	LatencyMsP99      float64   `json:"latency_ms_p99"`
	ErrorRatePct      float64   `json:"error_rate_pct"`
	AutoDiscovered    bool      `json:"auto_discovered"`
	// Denormalized names for API convenience
	FromNodeName      string    `json:"from_node_name,omitempty"`
	ToNodeName        string    `json:"to_node_name,omitempty"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

// NodeHealthInput is used when pushing a health snapshot for a node.
type NodeHealthInput struct {
	NodeID        string  `json:"node_id"`
	Status        string  `json:"status"`
	CPUPct        float64 `json:"cpu_pct"`
	MemPct        float64 `json:"mem_pct"`
	ErrorRatePct  float64 `json:"error_rate_pct"`
	LatencyMsP99  float64 `json:"latency_ms_p99"`
}

// LiveTopologyGraph is the full, multi-layer tenant graph returned by the API.
type LiveTopologyGraph struct {
	TenantID  string          `json:"tenant_id"`
	Nodes     []TopologyNode  `json:"nodes"`
	Edges     []TopologyEdge  `json:"edges"`
	NodeCount int             `json:"node_count"`
	EdgeCount int             `json:"edge_count"`
	Layers    []string        `json:"layers"`   // which node_type values are present
	BuiltAt   string          `json:"built_at"`
}

// ─── Blast Radius types ───────────────────────────────────────────────────────

// TraversalHop is returned by the recursive DB traversal and represents one
// node reached during a blast-radius or upstream-propagation BFS.
type TraversalHop struct {
	NodeID               string   `json:"node_id"`
	Depth                int      `json:"depth"`
	PropagatedConfidence int      `json:"propagated_confidence"` // decays by propagation_factor per hop
	EdgeRelation         string   `json:"edge_relation"`
	Path                 []string `json:"path"` // node IDs from root to this hop
}

// BlastRadiusHop enriches a TraversalHop with full node details.
type BlastRadiusHop struct {
	Node                 TopologyNode `json:"node"`
	Depth                int          `json:"depth"`
	PropagatedConfidence int          `json:"confidence"`
	EdgeRelation         string       `json:"edge_relation"`
	Path                 []string     `json:"path"`
}

// BlastRadiusResult is the full downstream blast-radius for a service node.
type BlastRadiusResult struct {
	RootNode             TopologyNode     `json:"root_node"`
	TotalImpacted        int              `json:"total_impacted"`
	CustomerFacingCount  int              `json:"customer_facing_impacted"`
	CriticalTierCount    int              `json:"critical_tier_impacted"`
	Hops                 []BlastRadiusHop `json:"hops"`
	ComputedAt           string           `json:"computed_at"`
}

// ─── Causal RCA types ────────────────────────────────────────────────────────

// DegradationEvent is one step in the temporal degradation sequence.
// The earliest event is the causal origin.
type DegradationEvent struct {
	ServiceName string    `json:"service_name"`
	EventID     string    `json:"event_id"`
	Timestamp   time.Time `json:"timestamp"`
	Severity    string    `json:"severity"`
	Signal      string    `json:"signal"`    // alert | metric | log | change
	Description string    `json:"description"`
	IsOrigin    bool      `json:"is_origin"` // true = degraded first
}

// EvidenceNode is one piece of evidence with its individual confidence score.
type EvidenceNode struct {
	ID                 string `json:"id"`
	Type               string `json:"type"`       // change | alert | metric | log | topology
	Label              string `json:"label"`
	Detail             string `json:"detail"`
	Confidence         int    `json:"confidence"` // 0–100
	Timestamp          string `json:"timestamp"`
	SupportsConclusion string `json:"supports_conclusion"`
}

// RootCauseNode captures the causal trigger — what changed.
type RootCauseNode struct {
	Type        string `json:"type"`        // deployment | config_change | dependency_failure | traffic_surge | unknown
	Service     string `json:"service"`
	Description string `json:"description"`
	Version     string `json:"version,omitempty"`
	Timestamp   string `json:"timestamp,omitempty"`
	Confidence  int    `json:"confidence"`
}

// ImpactSummary is one impacted service in the upstream/downstream list.
type ImpactSummary struct {
	ServiceName          string `json:"service_name"`
	NodeType             string `json:"node_type"`
	IsCustomerFacing     bool   `json:"is_customer_facing"`
	Tier                 string `json:"tier"`
	OwnerTeam            string `json:"owner_team"`
	PropagatedConfidence int    `json:"confidence"`
	Depth                int    `json:"depth"`
}

// CausalNarrative is the structured human-readable RCA broken into sections.
type CausalNarrative struct {
	WhatHappened      string `json:"what_happened"`
	WhatChanged       string `json:"what_changed"`
	HowItPropagated   string `json:"how_it_propagated"`
	WhoIsImpacted     string `json:"who_is_impacted"`
	ConfidenceSummary string `json:"confidence_summary"`
}

// CausalRCAResult is the structured RCA output — the core deliverable.
// It answers: what changed, which dependency degraded first, which up/downstreams
// are impacted, and what confidence score is attached to each piece of evidence.
type CausalRCAResult struct {
	IncidentID        string             `json:"incident_id"`
	RootCause         *RootCauseNode     `json:"root_cause"`
	DegradationChain  []DegradationEvent `json:"degradation_chain"`
	UpstreamImpact    []ImpactSummary    `json:"upstream_impact"`
	DownstreamImpact  []ImpactSummary    `json:"downstream_impact"`
	EvidenceNodes     []EvidenceNode     `json:"evidence_nodes"`
	OverallConfidence int                `json:"overall_confidence"`
	Narrative         CausalNarrative    `json:"narrative"`
	ComputedAt        string             `json:"computed_at"`
}
