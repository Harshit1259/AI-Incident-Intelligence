package models

import "time"

// CausalNode is one node in the incident causal DAG.
// Each node represents a discrete state change or degradation signal at a
// specific service and moment in time — e.g., "CPU spike on payment-service at 14:33".
type CausalNode struct {
	ID          string    `json:"id"`          // stable: "alert:{service}" or "change:{type}:{service}"
	NodeType    string    `json:"node_type"`   // "change" | "alert" | "symptom" | "impact"
	ServiceName string    `json:"service_name"`
	EventID     string    `json:"event_id,omitempty"`
	Label       string    `json:"label"`       // concise display text
	Detail      string    `json:"detail"`      // longer description
	Severity    string    `json:"severity,omitempty"`
	Timestamp   time.Time `json:"timestamp"`
	IsRoot      bool      `json:"is_root"` // true for the originating cause (no incoming edges)
	IsLeaf      bool      `json:"is_leaf"` // true for terminal symptoms (no outgoing edges)
	Confidence  int       `json:"confidence"` // 0-100: confidence this signal is correctly classified
}

// CausalEdge is a directed "A caused B" link in the DAG.
// Confidence reflects how certain we are that A directly caused B,
// derived from topology propagation factors and temporal proximity.
type CausalEdge struct {
	FromNodeID string `json:"from_node_id"`
	ToNodeID   string `json:"to_node_id"`
	Confidence int    `json:"confidence"` // 0-100: probability this edge is causal
	// Mechanism: "dependency_call" | "resource_pressure" | "config_change" | "cascade" | "topology"
	Mechanism  string `json:"mechanism"`
	LagSeconds int    `json:"lag_seconds"` // positive: seconds between cause and effect
	Evidence   string `json:"evidence"`    // human-readable explanation of why this edge exists
}

// CausalChain is the linearized critical path through the DAG.
// It is the primary answer to "what caused this incident, step by step."
type CausalChain struct {
	Nodes           []CausalNode `json:"nodes"`
	Edges           []CausalEdge `json:"edges"`
	ChainConfidence int          `json:"chain_confidence"` // harmonic mean of edge confidences
	Narrative       string       `json:"narrative"`        // "A →[87%]→ B →[73%]→ C" one-liner
}

// CausalImpactSummary holds the financial tail of the causal chain.
// It is appended as a leaf node so the chain reads:
// "deployment → CPU spike → DB exhaustion → 503s → $12,400 revenue loss"
type CausalImpactSummary struct {
	RevenueLoss     float64 `json:"revenue_loss"`      // USD
	DurationMinutes float64 `json:"duration_minutes"`
	AffectedUsers   int     `json:"affected_users"`
	Severity        string  `json:"severity"`
	Label           string  `json:"label"` // "Revenue loss: $12,400 over 47 min"
}

// CausalDAG is the full directed acyclic graph for an incident.
// It answers: what is every plausible causal pathway, which path is most
// likely the true cause, and what is the confidence of each causal link?
//
// The PrimaryCausalChain is the critical path — the highest-confidence sequence
// from root cause to the terminal symptom. AlternativeChains capture secondary
// branches that did not appear in the primary path.
type CausalDAG struct {
	IncidentID         string               `json:"incident_id"`
	Nodes              []CausalNode         `json:"nodes"`
	Edges              []CausalEdge         `json:"edges"`
	PrimaryCausalChain CausalChain          `json:"primary_causal_chain"`
	AlternativeChains  []CausalChain        `json:"alternative_chains,omitempty"`
	OverallConfidence  int                  `json:"overall_confidence"`
	BusinessImpact     *CausalImpactSummary `json:"business_impact,omitempty"`
	// Algorithm: "topology_temporal" when DB topology is used; "temporal_only" when not.
	Algorithm  string `json:"algorithm"`
	ComputedAt string `json:"computed_at"`
}
