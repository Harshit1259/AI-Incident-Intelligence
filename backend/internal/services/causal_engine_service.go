package services

// causal_engine_service.go — Tier 2 Feature 1: Causal AI
//
// Builds a Directed Acyclic Graph (DAG) of causal links for an incident.
// Each node is a discrete degradation signal at a service+time.
// Each edge is a directed "A caused B" link with an explicit confidence score.
//
// Confidence per edge = f(topology propagation factor, temporal proximity).
// Critical path = the sequence root → ... → leaf with maximum harmonic-mean confidence.
//
// Algorithm:
//   1. Build one node per service (earliest alert) + one change node if a deployment is linked.
//   2. Attempt topology-backed edges: for each topology edge (service A → service B),
//      if A's event preceded B's event in time, create a causal edge A→B.
//   3. Fall back to pure temporal ordering if no topology edges found.
//   4. Find critical path via DFS; build alternative chains from remaining root nodes.
//   5. Compute overall confidence as blended change+chain score.

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"ai-incident-platform/backend/internal/models"
	"ai-incident-platform/backend/internal/store"
)

// CausalEngineService builds causal DAGs for incidents.
type CausalEngineService struct {
	detailSvc        *IncidentDetailService
	topoStore        *store.TopologyStore     // may be nil; enables topology-backed edges when set
	businessImpactSvc *BusinessImpactService  // may be nil; enriches DAG with revenue loss
}

// NewCausalEngineService creates a CausalEngineService.
// topoStore may be nil — the service falls back to temporal-only analysis.
func NewCausalEngineService(ds *IncidentDetailService, ts *store.TopologyStore) *CausalEngineService {
	return &CausalEngineService{detailSvc: ds, topoStore: ts}
}

// SetBusinessImpactService wires in the business impact service so BuildCausalDAG
// can append revenue loss as a leaf impact node in the causal chain.
func (s *CausalEngineService) SetBusinessImpactService(bis *BusinessImpactService) {
	s.businessImpactSvc = bis
}

// BuildCausalDAG constructs the full causal DAG for an incident.
func (s *CausalEngineService) BuildCausalDAG(incidentID, tenantID string) (*models.CausalDAG, error) {
	detail, found := s.detailSvc.GetIncidentDetail(incidentID)
	if !found {
		return nil, fmt.Errorf("incident %q not found", incidentID)
	}

	incident := detail.Incident

	// ── Step 1: Build per-service causal nodes ────────────────────────────
	// One node per service: the earliest alert seen on that service.
	// Keeps the chain clean — later alerts on the same service are effects, not causes.
	serviceFirst := map[string]models.CausalNode{}
	for _, te := range detail.Events {
		ev := te.Event
		svc := strings.TrimSpace(ev.Service)
		if svc == "" {
			svc = incident.Service
		}
		t := ev.Timestamp
		if t.IsZero() {
			t = incident.FirstEventTime
		}
		existing, exists := serviceFirst[svc]
		if !exists || t.Before(existing.Timestamp) {
			serviceFirst[svc] = models.CausalNode{
				ID:          "alert:" + svc,
				NodeType:    causalNodeType(ev.Severity),
				ServiceName: svc,
				EventID:     ev.ID,
				Label:       alertNodeLabel(svc, ev.Title, ev.Severity),
				Detail:      ev.Title,
				Severity:    ev.Severity,
				Timestamp:   t,
				Confidence:  severityToConfidence(ev.Severity),
			}
		}
	}

	// Sort alert nodes chronologically.
	alertNodes := make([]models.CausalNode, 0, len(serviceFirst))
	for _, n := range serviceFirst {
		alertNodes = append(alertNodes, n)
	}
	sort.Slice(alertNodes, func(i, j int) bool {
		return alertNodes[i].Timestamp.Before(alertNodes[j].Timestamp)
	})

	// ── Step 2: Change node (potential root cause) ────────────────────────
	allNodes := make([]models.CausalNode, 0, len(alertNodes)+1)
	var changeNode *models.CausalNode

	if detail.WhatChanged.Type != "" {
		ts, err := time.Parse(time.RFC3339, detail.WhatChanged.Timestamp)
		if err != nil || ts.IsZero() {
			// Fallback: place change 15 min before the first alert.
			if len(alertNodes) > 0 {
				ts = alertNodes[0].Timestamp.Add(-15 * time.Minute)
			} else {
				ts = incident.FirstEventTime.Add(-15 * time.Minute)
			}
		}
		conf := incident.WhatChangedConfidence
		if conf == 0 {
			conf = 70
		}
		svc := detail.WhatChanged.Service
		if svc == "" {
			svc = incident.Service
		}
		cn := models.CausalNode{
			ID:          "change:" + detail.WhatChanged.Type + ":" + svc,
			NodeType:    "change",
			ServiceName: svc,
			Label:       causalChangeLabel(detail.WhatChanged),
			Detail:      causalChangeDetail(detail.WhatChanged),
			Timestamp:   ts,
			IsRoot:      true,
			Confidence:  conf,
		}
		changeNode = &cn
		allNodes = append(allNodes, cn)
	}
	allNodes = append(allNodes, alertNodes...)

	// ── Step 3: Build causal edges ────────────────────────────────────────
	var edges []models.CausalEdge
	algorithm := "temporal_only"

	if s.topoStore != nil {
		topoEdges := s.buildTopologyEdges(tenantID, allNodes, changeNode)
		if len(topoEdges) > 0 {
			edges = topoEdges
			algorithm = "topology_temporal"
		}
	}
	if len(edges) == 0 {
		edges = s.buildTemporalEdges(allNodes, incident, changeNode)
	}

	// ── Step 4: Mark root / leaf nodes ───────────────────────────────────
	inDeg := map[string]int{}
	outDeg := map[string]int{}
	for _, e := range edges {
		outDeg[e.FromNodeID]++
		inDeg[e.ToNodeID]++
	}
	for i := range allNodes {
		allNodes[i].IsRoot = inDeg[allNodes[i].ID] == 0
		allNodes[i].IsLeaf = outDeg[allNodes[i].ID] == 0
	}
	// Change node is always root by definition.
	if changeNode != nil {
		for i := range allNodes {
			if allNodes[i].ID == changeNode.ID {
				allNodes[i].IsRoot = true
			}
		}
	}

	// ── Step 5: Find critical path and alternatives ───────────────────────
	primary := s.findCriticalPath(allNodes, edges)
	alts := s.findAlternativeChains(allNodes, edges, primary)

	// ── Step 6: Overall confidence ────────────────────────────────────────
	overall := primary.ChainConfidence
	if changeNode != nil && changeNode.Confidence > 0 {
		overall = (overall*2 + changeNode.Confidence) / 3
	}
	if overall > 98 {
		overall = 98
	}

	dag := &models.CausalDAG{
		IncidentID:         incidentID,
		Nodes:              allNodes,
		Edges:              edges,
		PrimaryCausalChain: primary,
		AlternativeChains:  alts,
		OverallConfidence:  overall,
		Algorithm:          algorithm,
		ComputedAt:         time.Now().Format(time.RFC3339),
	}

	// Enrich with business impact (revenue loss) as a leaf node.
	// This closes the causal chain: "deployment → spike → exhaustion → 503s → $X revenue loss"
	if s.businessImpactSvc != nil {
		if impact, _ := s.businessImpactSvc.GetOrCalculate(incidentID); impact != nil && impact.Breakdown != nil {
			revLoss := impact.Breakdown.ActualRevenueLoss
			dur := impact.Breakdown.ActualDurationMinutes

			// Map 0-1 float confidence score to 0-100 int.
			confInt := 70
			if impact.Estimate != nil {
				confInt = int(impact.Estimate.ConfidenceScore * 100)
			}

			label := fmt.Sprintf("Revenue loss: $%.0f over %.0f min", revLoss, dur)
			impactNode := models.CausalNode{
				ID:          "impact:" + incidentID,
				NodeType:    "impact",
				ServiceName: incident.Service,
				Label:       label,
				Detail:      fmt.Sprintf("Estimated %s severity revenue impact based on SRE cost model.", incident.Severity),
				Severity:    incident.Severity,
				Timestamp:   incident.LastEventTime,
				IsLeaf:      true,
				Confidence:  confInt,
			}
			dag.Nodes = append(dag.Nodes, impactNode)
			dag.BusinessImpact = &models.CausalImpactSummary{
				RevenueLoss:     revLoss,
				DurationMinutes: dur,
				Severity:        incident.Severity,
				Label:           label,
			}

			// Edge from the last symptom node in the primary chain → impact node.
			if len(primary.Nodes) > 0 {
				lastNode := primary.Nodes[len(primary.Nodes)-1]
				dag.Edges = append(dag.Edges, models.CausalEdge{
					FromNodeID: lastNode.ID,
					ToNodeID:   impactNode.ID,
					Confidence: confInt,
					Mechanism:  "business_impact",
					LagSeconds: 0,
					Evidence: fmt.Sprintf(
						"Customer-facing degradation on %s caused ~$%.0f revenue loss over %.0f minutes.",
						incident.Service, revLoss, dur,
					),
				})
				dag.PrimaryCausalChain.Nodes = append(dag.PrimaryCausalChain.Nodes, impactNode)
				dag.PrimaryCausalChain.Narrative += fmt.Sprintf(" →[%d%%]→ %s", confInt, label)
			}
		}
	}

	return dag, nil
}

// ─── Topology-backed edge construction ───────────────────────────────────────

// buildTopologyEdges creates causal edges using the persisted topology graph.
// For each topology path (service A → service B), if A's event preceded B's event,
// the edge confidence is the geometric mean of topology propagation score and temporal proximity.
func (s *CausalEngineService) buildTopologyEdges(
	tenantID string,
	nodes []models.CausalNode,
	changeNode *models.CausalNode,
) []models.CausalEdge {
	nodeByService := map[string]*models.CausalNode{}
	for i := range nodes {
		nodeByService[nodes[i].ServiceName] = &nodes[i]
	}

	var edges []models.CausalEdge

	for i := range nodes {
		from := &nodes[i]
		fromDB, err := s.topoStore.GetNodeByName(tenantID, from.ServiceName)
		if err != nil || fromDB == nil {
			continue
		}
		hops, err := s.topoStore.GetDownstream(fromDB.ID, tenantID, 2)
		if err != nil {
			continue
		}
		for _, hop := range hops {
			toDBNode, err := s.topoStore.GetNodeByID(hop.NodeID)
			if err != nil || toDBNode == nil {
				continue
			}
			to, ok := nodeByService[toDBNode.Name]
			if !ok || to.ID == from.ID {
				continue
			}
			// Causality requires the cause to precede the effect.
			if !from.Timestamp.Before(to.Timestamp) {
				continue
			}
			lag := to.Timestamp.Sub(from.Timestamp)
			if lag > 30*time.Minute {
				continue // too wide a gap to be a direct causal link
			}
			conf := topologyEdgeConfidence(hop.PropagatedConfidence, lag)
			edges = append(edges, models.CausalEdge{
				FromNodeID: from.ID,
				ToNodeID:   to.ID,
				Confidence: conf,
				Mechanism:  edgeMechanism(hop.EdgeRelation),
				LagSeconds: int(lag.Seconds()),
				Evidence: fmt.Sprintf(
					"%s → %s via %s (topology-confirmed, propagation %d%%, lag %s)",
					from.ServiceName, to.ServiceName,
					hop.EdgeRelation, hop.PropagatedConfidence, causalFmtDuration(lag),
				),
			})
		}
	}

	// Change → impacted service edge.
	if changeNode != nil {
		if to, ok := nodeByService[changeNode.ServiceName]; ok && to.ID != changeNode.ID {
			lag := to.Timestamp.Sub(changeNode.Timestamp)
			if lag > 0 && lag < 60*time.Minute {
				conf := changeEdgeConfidence(lag, changeNode.Confidence)
				edges = append(edges, models.CausalEdge{
					FromNodeID: changeNode.ID,
					ToNodeID:   to.ID,
					Confidence: conf,
					Mechanism:  "config_change",
					LagSeconds: int(lag.Seconds()),
					Evidence: fmt.Sprintf(
						"%s on %s preceded the first alert by %s",
						changeNode.Label, changeNode.ServiceName, causalFmtDuration(lag),
					),
				})
			}
		}
	}

	return edges
}

// ─── Temporal-only edge construction (fallback) ───────────────────────────────

// buildTemporalEdges builds a simple sequential chain when no topology data exists.
// The chain is: change → earliest_service → next_service → ..., ordered by time.
func (s *CausalEngineService) buildTemporalEdges(
	nodes []models.CausalNode,
	incident models.Incident,
	changeNode *models.CausalNode,
) []models.CausalEdge {
	var alertNodes []models.CausalNode
	for _, n := range nodes {
		if n.NodeType != "change" {
			alertNodes = append(alertNodes, n)
		}
	}
	sort.Slice(alertNodes, func(i, j int) bool {
		return alertNodes[i].Timestamp.Before(alertNodes[j].Timestamp)
	})

	var edges []models.CausalEdge

	// Change → first alert.
	if changeNode != nil && len(alertNodes) > 0 {
		lag := alertNodes[0].Timestamp.Sub(changeNode.Timestamp)
		if lag > 0 && lag < 60*time.Minute {
			conf := changeEdgeConfidence(lag, changeNode.Confidence)
			edges = append(edges, models.CausalEdge{
				FromNodeID: changeNode.ID,
				ToNodeID:   alertNodes[0].ID,
				Confidence: conf,
				Mechanism:  "config_change",
				LagSeconds: int(lag.Seconds()),
				Evidence: fmt.Sprintf(
					"%s preceded the first alert on %s by %s",
					changeNode.Label, alertNodes[0].ServiceName, causalFmtDuration(lag),
				),
			})
		}
	}

	// Sequential alert links.
	for i := 1; i < len(alertNodes); i++ {
		prev, curr := alertNodes[i-1], alertNodes[i]
		lag := curr.Timestamp.Sub(prev.Timestamp)
		if lag > 30*time.Minute {
			break // discontinuity — stop the chain
		}
		conf := temporalOnlyConf(lag)
		edges = append(edges, models.CausalEdge{
			FromNodeID: prev.ID,
			ToNodeID:   curr.ID,
			Confidence: conf,
			Mechanism:  "cascade",
			LagSeconds: int(lag.Seconds()),
			Evidence: fmt.Sprintf(
				"%s preceded %s by %s (temporal inference — no topology data available)",
				prev.ServiceName, curr.ServiceName, causalFmtDuration(lag),
			),
		})
	}

	return edges
}

// ─── Critical path (DFS) ──────────────────────────────────────────────────────

// findCriticalPath returns the root-to-leaf path with the highest harmonic-mean
// edge confidence (the harmonic mean penalises weak links in the chain).
func (s *CausalEngineService) findCriticalPath(
	nodes []models.CausalNode,
	edges []models.CausalEdge,
) models.CausalChain {
	if len(nodes) == 0 {
		return models.CausalChain{}
	}

	adj := map[string][]models.CausalEdge{}
	inDeg := map[string]int{}
	for _, e := range edges {
		adj[e.FromNodeID] = append(adj[e.FromNodeID], e)
		inDeg[e.ToNodeID]++
	}
	nodeByID := map[string]models.CausalNode{}
	for _, n := range nodes {
		nodeByID[n.ID] = n
	}

	var bestNodes []models.CausalNode
	var bestEdges []models.CausalEdge
	bestConf := -1

	var dfs func(id string, path []models.CausalNode, pathEdges []models.CausalEdge, depth int)
	dfs = func(id string, path []models.CausalNode, pathEdges []models.CausalEdge, depth int) {
		n, ok := nodeByID[id]
		if !ok || depth > 20 {
			return
		}
		path = append(path, n)
		outEdges := adj[id]
		if len(outEdges) == 0 { // leaf
			if len(path) < 2 {
				return // single node is not a chain
			}
			c := harmonicMean(pathEdges)
			if c > bestConf {
				bestConf = c
				bestNodes = append([]models.CausalNode{}, path...)
				bestEdges = append([]models.CausalEdge{}, pathEdges...)
			}
			return
		}
		for _, e := range outEdges {
			dfs(e.ToNodeID, path, append(pathEdges, e), depth+1)
		}
	}

	// Start from every root node.
	for _, n := range nodes {
		if inDeg[n.ID] == 0 {
			dfs(n.ID, nil, nil, 0)
		}
	}

	// If no multi-node path found, return all nodes in order as a trivial chain.
	if len(bestNodes) == 0 {
		bestNodes = append([]models.CausalNode{}, nodes...)
		bestConf = 50
	}
	if bestConf < 0 {
		bestConf = 0
	}

	return models.CausalChain{
		Nodes:           bestNodes,
		Edges:           bestEdges,
		ChainConfidence: bestConf,
		Narrative:       buildCausalNarrative(bestNodes, bestEdges),
	}
}

// findAlternativeChains finds significant paths not covered by the primary chain.
// Limited to 2 alternatives to keep the response compact.
func (s *CausalEngineService) findAlternativeChains(
	nodes []models.CausalNode,
	edges []models.CausalEdge,
	primary models.CausalChain,
) []models.CausalChain {
	primarySet := map[string]bool{}
	for _, n := range primary.Nodes {
		primarySet[n.ID] = true
	}

	adj := map[string][]models.CausalEdge{}
	inDeg := map[string]int{}
	for _, e := range edges {
		adj[e.FromNodeID] = append(adj[e.FromNodeID], e)
		inDeg[e.ToNodeID]++
	}
	nodeByID := map[string]models.CausalNode{}
	for _, n := range nodes {
		nodeByID[n.ID] = n
	}

	var alts []models.CausalChain
	for _, n := range nodes {
		if primarySet[n.ID] || inDeg[n.ID] != 0 || len(adj[n.ID]) == 0 {
			continue
		}
		// Greedily follow the highest-confidence edge at each step.
		var chainNodes []models.CausalNode
		var chainEdges []models.CausalEdge
		cur := n.ID
		for steps := 0; steps < 10; steps++ {
			cn, ok := nodeByID[cur]
			if !ok {
				break
			}
			chainNodes = append(chainNodes, cn)
			best := bestOutEdge(adj[cur])
			if best == nil {
				break
			}
			chainEdges = append(chainEdges, *best)
			cur = best.ToNodeID
		}
		if len(chainNodes) >= 2 {
			alts = append(alts, models.CausalChain{
				Nodes:           chainNodes,
				Edges:           chainEdges,
				ChainConfidence: harmonicMean(chainEdges),
				Narrative:       buildCausalNarrative(chainNodes, chainEdges),
			})
		}
		if len(alts) >= 2 {
			break
		}
	}
	return alts
}

// ─── Confidence math ──────────────────────────────────────────────────────────

// topologyEdgeConfidence blends structural propagation score with temporal proximity.
// propConf is the already-computed propagated confidence from the topology store (0-100).
func topologyEdgeConfidence(propConf int, lag time.Duration) int {
	ts := causalTemporalScore(lag)
	return (propConf + int(ts)) / 2
}

// changeEdgeConfidence scores a deployment → first-alert causal link.
func changeEdgeConfidence(lag time.Duration, changeConf int) int {
	ts := causalTemporalScore(lag)
	return (changeConf + int(ts)) / 2
}

// temporalOnlyConf returns a penalised score (65% of temporal score) when no
// topology evidence supports the edge — pure temporal ordering is weaker evidence.
func temporalOnlyConf(lag time.Duration) int {
	return int(causalTemporalScore(lag) * 0.65)
}

// causalTemporalScore converts lag into a 0-100 confidence score.
// Shorter lag → higher confidence that cause preceded effect.
func causalTemporalScore(lag time.Duration) float64 {
	switch {
	case lag <= 60*time.Second:
		return 95
	case lag <= 5*time.Minute:
		return 87
	case lag <= 10*time.Minute:
		return 76
	case lag <= 15*time.Minute:
		return 62
	case lag <= 30*time.Minute:
		return 44
	default:
		return 25
	}
}

// harmonicMean returns the harmonic mean of edge confidences (0 when any edge is 0).
// The harmonic mean heavily penalises weak links — the chain is only as strong as its weakest edge.
func harmonicMean(edges []models.CausalEdge) int {
	if len(edges) == 0 {
		return 0
	}
	var sumRecip float64
	for _, e := range edges {
		if e.Confidence <= 0 {
			return 0
		}
		sumRecip += 1.0 / float64(e.Confidence)
	}
	return int(float64(len(edges)) / sumRecip)
}

// ─── Narrative builder ────────────────────────────────────────────────────────

func buildCausalNarrative(nodes []models.CausalNode, edges []models.CausalEdge) string {
	if len(nodes) == 0 {
		return "No causal chain established."
	}
	var sb strings.Builder
	for i, n := range nodes {
		sb.WriteString(n.Label)
		if !n.Timestamp.IsZero() {
			sb.WriteString(fmt.Sprintf(" (%s)", n.Timestamp.UTC().Format("15:04:05")))
		}
		if i < len(edges) {
			sb.WriteString(fmt.Sprintf(" →[%d%%]→ ", edges[i].Confidence))
		}
	}
	return sb.String()
}

// ─── Label / type helpers ─────────────────────────────────────────────────────

func causalNodeType(severity string) string {
	switch strings.ToLower(severity) {
	case "critical", "high":
		return "symptom"
	default:
		return "alert"
	}
}

func alertNodeLabel(svc, title, severity string) string {
	if title != "" && len(title) < 80 {
		return title
	}
	h := causalHumanizeSvc(svc)
	switch strings.ToLower(severity) {
	case "critical":
		return h + ": critical failure"
	case "high":
		return h + ": high-severity alert"
	case "medium":
		return h + ": degraded"
	default:
		return h + ": alert"
	}
}

func causalChangeLabel(wc models.WhatChanged) string {
	svc := wc.Service
	if svc == "" {
		svc = "service"
	}
	if wc.Version != "" {
		return fmt.Sprintf("Deployed %s@%s", svc, wc.Version)
	}
	t := wc.Type
	if len(t) > 0 {
		t = strings.ToUpper(t[:1]) + strings.ToLower(t[1:])
	}
	return fmt.Sprintf("%s on %s", t, svc)
}

func causalChangeDetail(wc models.WhatChanged) string {
	if wc.Description != "" {
		return wc.Description
	}
	if wc.Version != "" {
		return fmt.Sprintf("Version %s deployed to %s at %s", wc.Version, wc.Service, wc.Timestamp)
	}
	return fmt.Sprintf("%s change on %s", wc.Type, wc.Service)
}

func edgeMechanism(relation string) string {
	switch strings.ToLower(relation) {
	case "calls", "routes_to":
		return "dependency_call"
	case "depends_on":
		return "dependency_call"
	case "runs_on":
		return "resource_pressure"
	case "change_caused":
		return "config_change"
	default:
		return "cascade"
	}
}

func severityToConfidence(severity string) int {
	switch strings.ToLower(severity) {
	case "critical":
		return 90
	case "high":
		return 80
	case "medium":
		return 70
	default:
		return 60
	}
}

func causalHumanizeSvc(svc string) string {
	parts := strings.FieldsFunc(svc, func(r rune) bool { return r == '-' || r == '_' || r == '.' })
	for i, p := range parts {
		if len(p) > 0 {
			parts[i] = strings.ToUpper(p[:1]) + p[1:]
		}
	}
	return strings.Join(parts, " ")
}

func causalFmtDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm%ds", int(d.Minutes()), int(d.Seconds())%60)
	}
	return fmt.Sprintf("%dh%dm", int(d.Hours()), int(d.Minutes())%60)
}

func bestOutEdge(out []models.CausalEdge) *models.CausalEdge {
	if len(out) == 0 {
		return nil
	}
	best := &out[0]
	for i := 1; i < len(out); i++ {
		if out[i].Confidence > best.Confidence {
			best = &out[i]
		}
	}
	return best
}
