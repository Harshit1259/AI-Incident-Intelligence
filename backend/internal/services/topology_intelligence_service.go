package services

// TopologyIntelligenceService replaces the static config.ServiceDependencies map
// with a DB-backed, evidence-linked graph engine. It answers:
//   - What is the full live topology for a tenant?
//   - Which services are in the blast radius of a given node?
//   - What caused this incident, which dependency degraded first,
//     which upstreams/downstreams are impacted, and what is the per-evidence confidence?

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"ai-incident-platform/backend/internal/models"
	"ai-incident-platform/backend/internal/store"
)

const (
	defaultMaxDepth = 5
)

type TopologyIntelligenceService struct {
	topoStore     *store.TopologyStore
	incidentStore *store.IncidentStore
}

func NewTopologyIntelligenceService(
	ts *store.TopologyStore,
	is *store.IncidentStore,
) *TopologyIntelligenceService {
	return &TopologyIntelligenceService{topoStore: ts, incidentStore: is}
}

// ─── Live Graph ───────────────────────────────────────────────────────────────

// BuildLiveGraph returns the full persisted topology graph for a tenant,
// enriched with per-node health snapshots.
func (s *TopologyIntelligenceService) BuildLiveGraph(tenantID string) (*models.LiveTopologyGraph, error) {
	nodes, err := s.topoStore.GetNodes(tenantID)
	if err != nil {
		return nil, fmt.Errorf("topology: get nodes: %w", err)
	}
	edges, err := s.topoStore.GetEdges(tenantID)
	if err != nil {
		return nil, fmt.Errorf("topology: get edges: %w", err)
	}

	layerSet := map[string]bool{}
	for _, n := range nodes {
		layerSet[n.NodeType] = true
	}
	layers := make([]string, 0, len(layerSet))
	for l := range layerSet {
		layers = append(layers, l)
	}
	sort.Strings(layers)

	return &models.LiveTopologyGraph{
		TenantID:  tenantID,
		Nodes:     nodes,
		Edges:     edges,
		NodeCount: len(nodes),
		EdgeCount: len(edges),
		Layers:    layers,
		BuiltAt:   time.Now().Format(time.RFC3339),
	}, nil
}

// ─── Incident Topology Graph ──────────────────────────────────────────────────

// BuildIncidentTopologyGraph returns the enriched GraphData for one incident.
// It uses the persisted topology DB as the source for service relationships
// instead of the static config map, and falls back gracefully when no DB nodes exist.
func (s *TopologyIntelligenceService) BuildIncidentTopologyGraph(
	detail models.IncidentDetail,
	tenantID string,
) models.GraphData {
	incident := detail.Incident
	rootID := incident.Service
	if rootID == "" {
		rootID = "unknown"
	}

	nodes := map[string]models.GraphNode{}
	var edges []models.GraphEdge

	// ── Root service node ──────────────────────────────────────────────
	rootNode := models.GraphNode{
		ID:            rootID,
		Label:         humanizeNodeLabel(rootID),
		NodeType:      "service",
		Severity:      incident.Severity,
		Status:        severityToStatus(incident.Severity),
		AlertCount:    incident.EventCount,
		ChangeLinked:  incident.WhatChangedType != "",
		EvidenceCount: len(detail.EvidenceRefs),
	}
	// Enrich from DB if the node is persisted.
	if dbNode, _ := s.topoStore.GetNodeByName(tenantID, rootID); dbNode != nil {
		rootNode.Label = dbNode.DisplayName
		if dbNode.Health != nil {
			rootNode.Status = dbNode.Health.Status
		}
	}
	nodes[rootID] = rootNode

	// ── DB-backed downstream dependencies ─────────────────────────────
	dbRoot, _ := s.topoStore.GetNodeByName(tenantID, rootID)
	if dbRoot != nil {
		hops, _ := s.topoStore.GetDownstream(dbRoot.ID, tenantID, 3)
		for _, hop := range hops {
			dbN, _ := s.topoStore.GetNodeByID(hop.NodeID)
			if dbN == nil {
				continue
			}
			if _, exists := nodes[dbN.Name]; !exists {
				status := "unknown"
				if dbN.Health != nil {
					status = dbN.Health.Status
				}
				nodes[dbN.Name] = models.GraphNode{
					ID:       dbN.Name,
					Label:    dbN.DisplayName,
					NodeType: mapDBNodeType(dbN.NodeType, hop.Depth),
					Status:   status,
				}
			}
			edges = append(edges, models.GraphEdge{
				From:       rootID,
				To:         dbN.Name,
				Relation:   hop.EdgeRelation,
				Weight:     float64(hop.PropagatedConfidence) / 100.0,
				Confidence: hop.PropagatedConfidence,
			})
		}
	}

	// ── Impacted services from incident record ─────────────────────────
	for _, svc := range incident.ImpactedServices {
		if svc == "" || svc == rootID {
			continue
		}
		if _, exists := nodes[svc]; !exists {
			nodes[svc] = models.GraphNode{
				ID:       svc,
				Label:    humanizeNodeLabel(svc),
				NodeType: "impacted_service",
				Status:   "degraded",
			}
		}
		// Only add the edge if no DB edge already covers it.
		alreadyLinked := false
		for _, e := range edges {
			if e.To == svc {
				alreadyLinked = true
				break
			}
		}
		if !alreadyLinked {
			edges = append(edges, models.GraphEdge{
				From: rootID, To: svc,
				Relation: "impacts", Weight: 0.7, Confidence: 65,
			})
		}
	}

	// ── Alert origin nodes ─────────────────────────────────────────────
	sourceCounts := map[string]int{}
	for _, te := range detail.Events {
		src := strings.ToLower(strings.TrimSpace(te.Event.Source))
		if src == "" {
			src = "unknown-source"
		}
		sourceCounts[src]++
	}
	for src, count := range sourceCounts {
		originID := "origin:" + src
		nodes[originID] = models.GraphNode{
			ID: originID, Label: fmt.Sprintf("Alert: %s (%d)", src, count),
			NodeType: "alert_origin", AlertCount: count,
		}
		edges = append(edges, models.GraphEdge{
			From: originID, To: rootID, Relation: "fired_on",
			Weight: 1.0, Confidence: 100, EvidenceCount: count,
		})
	}

	// ── Change event node ──────────────────────────────────────────────
	if detail.WhatChanged.Type != "" {
		chgSvc := detail.WhatChanged.Service
		if chgSvc == "" {
			chgSvc = rootID
		}
		changeID := fmt.Sprintf("change:%s:%s", chgSvc, detail.WhatChanged.Type)
		vLabel := ""
		if detail.WhatChanged.Version != "" {
			vLabel = " @" + detail.WhatChanged.Version
		}
		nodes[changeID] = models.GraphNode{
			ID: changeID, Label: fmt.Sprintf("%s%s", detail.WhatChanged.Type, vLabel),
			NodeType: "change", ChangeLinked: true,
		}
		conf := incident.WhatChangedConfidence
		if conf == 0 {
			conf = 60
		}
		edges = append(edges, models.GraphEdge{
			From: changeID, To: rootID, Relation: "preceded",
			Weight: float64(conf) / 100.0, Confidence: conf,
		})
		if chgSvc != rootID {
			if _, exists := nodes[chgSvc]; !exists {
				nodes[chgSvc] = models.GraphNode{
					ID: chgSvc, Label: humanizeNodeLabel(chgSvc),
					NodeType: "dependency", ChangeLinked: true,
				}
			}
			edges = append(edges, models.GraphEdge{
				From: chgSvc, To: rootID, Relation: "change_propagated",
				Weight: float64(conf) / 100.0, Confidence: conf,
			})
		}
	}

	// ── Evidence nodes ─────────────────────────────────────────────────
	evidenceByType := map[string]int{}
	for _, ref := range detail.EvidenceRefs {
		evidenceByType[ref.Type]++
	}
	for evType, count := range evidenceByType {
		if evType == "event" {
			continue
		}
		evID := "evidence:" + evType
		label := fmt.Sprintf("%s evidence (%d)", strings.ToUpper(evType[:1])+evType[1:], count)
		nodes[evID] = models.GraphNode{
			ID: evID, Label: label,
			NodeType: "evidence", EvidenceCount: count,
		}
		conf := evidenceConfidence(evType, count)
		edges = append(edges, models.GraphEdge{
			From: evID, To: rootID, Relation: "supports",
			Weight: float64(conf) / 100.0, Confidence: conf, EvidenceCount: count,
		})
	}

	// ── Flatten nodes → slice ──────────────────────────────────────────
	nodeSlice := make([]models.GraphNode, 0, len(nodes))
	for _, n := range nodes {
		nodeSlice = append(nodeSlice, n)
	}

	return models.GraphData{
		Nodes:      nodeSlice,
		Edges:      edges,
		RootNodeID: rootID,
		BuiltAt:    time.Now().Format(time.RFC3339),
	}
}

// ─── Blast Radius ─────────────────────────────────────────────────────────────

// ComputeBlastRadius traverses downstream from the given service node and returns
// the full blast radius: total impacted count, customer-facing count, and ordered hops.
func (s *TopologyIntelligenceService) ComputeBlastRadius(
	serviceID, tenantID string,
) (*models.BlastRadiusResult, error) {
	// Resolve node — serviceID may be a name or DB id.
	root, err := s.topoStore.GetNodeByName(tenantID, serviceID)
	if err != nil || root == nil {
		root, err = s.topoStore.GetNodeByID(serviceID)
		if err != nil || root == nil {
			return nil, fmt.Errorf("topology: node %q not found for tenant %q", serviceID, tenantID)
		}
	}

	hops, err := s.topoStore.GetDownstream(root.ID, tenantID, defaultMaxDepth)
	if err != nil {
		return nil, fmt.Errorf("topology: downstream traversal: %w", err)
	}

	result := &models.BlastRadiusResult{
		RootNode:   *root,
		ComputedAt: time.Now().Format(time.RFC3339),
	}

	for _, hop := range hops {
		dbN, _ := s.topoStore.GetNodeByID(hop.NodeID)
		if dbN == nil {
			continue
		}
		result.TotalImpacted++
		if dbN.IsCustomerFacing {
			result.CustomerFacingCount++
		}
		if dbN.Tier == "critical" {
			result.CriticalTierCount++
		}
		result.Hops = append(result.Hops, models.BlastRadiusHop{
			Node:                 *dbN,
			Depth:                hop.Depth,
			PropagatedConfidence: hop.PropagatedConfidence,
			EdgeRelation:         hop.EdgeRelation,
			Path:                 hop.Path,
		})
	}

	// Sort hops: customer-facing and critical first, then by depth.
	sort.Slice(result.Hops, func(i, j int) bool {
		hi, hj := result.Hops[i], result.Hops[j]
		if hi.Node.IsCustomerFacing != hj.Node.IsCustomerFacing {
			return hi.Node.IsCustomerFacing
		}
		if hi.Node.Tier != hj.Node.Tier {
			return hi.Node.Tier == "critical"
		}
		return hi.Depth < hj.Depth
	})

	return result, nil
}

// ─── Causal RCA ───────────────────────────────────────────────────────────────

// GenerateCausalRCA produces the structured RCA output for an incident:
//   - What changed (the causal trigger)
//   - Which dependency degraded first (temporal ordering of alerts)
//   - Which upstreams/downstreams are impacted (graph traversal)
//   - Confidence per evidence node (scored by signal type + timing)
func (s *TopologyIntelligenceService) GenerateCausalRCA(
	detail models.IncidentDetail,
	tenantID string,
) (*models.CausalRCAResult, error) {
	incident := detail.Incident
	rca := &models.CausalRCAResult{
		IncidentID: incident.ID,
		ComputedAt: time.Now().Format(time.RFC3339),
	}

	// ── 1. What changed ───────────────────────────────────────────────
	rca.RootCause = buildRootCause(detail)

	// ── 2. Degradation chain (temporal ordering) ──────────────────────
	rca.DegradationChain = buildDegradationChain(detail)

	// ── 3. Upstream / downstream impact from graph ─────────────────────
	rootService := incident.Service
	dbRoot, _ := s.topoStore.GetNodeByName(tenantID, rootService)
	if dbRoot != nil {
		down, _ := s.topoStore.GetDownstream(dbRoot.ID, tenantID, defaultMaxDepth)
		for _, hop := range down {
			n, _ := s.topoStore.GetNodeByID(hop.NodeID)
			if n == nil {
				continue
			}
			rca.DownstreamImpact = append(rca.DownstreamImpact, models.ImpactSummary{
				ServiceName:          n.Name,
				NodeType:             n.NodeType,
				IsCustomerFacing:     n.IsCustomerFacing,
				Tier:                 n.Tier,
				OwnerTeam:            n.OwnerTeam,
				PropagatedConfidence: hop.PropagatedConfidence,
				Depth:                hop.Depth,
			})
		}

		up, _ := s.topoStore.GetUpstream(dbRoot.ID, tenantID, defaultMaxDepth)
		for _, hop := range up {
			n, _ := s.topoStore.GetNodeByID(hop.NodeID)
			if n == nil {
				continue
			}
			rca.UpstreamImpact = append(rca.UpstreamImpact, models.ImpactSummary{
				ServiceName:          n.Name,
				NodeType:             n.NodeType,
				IsCustomerFacing:     n.IsCustomerFacing,
				Tier:                 n.Tier,
				OwnerTeam:            n.OwnerTeam,
				PropagatedConfidence: hop.PropagatedConfidence,
				Depth:                hop.Depth,
			})
		}
	} else {
		// Fallback: use incident's impacted_services as downstream proxies.
		for _, svc := range incident.ImpactedServices {
			if svc == "" || svc == rootService {
				continue
			}
			rca.DownstreamImpact = append(rca.DownstreamImpact, models.ImpactSummary{
				ServiceName: svc, NodeType: "service",
				PropagatedConfidence: 55, Depth: 1,
				Tier: "internal",
			})
		}
	}

	// ── 4. Evidence nodes with individual confidence scores ────────────
	rca.EvidenceNodes = buildEvidenceNodes(detail, rca.DegradationChain)

	// ── 5. Overall confidence (weighted average) ───────────────────────
	rca.OverallConfidence = computeOverallConfidence(rca.EvidenceNodes, rca.RootCause)

	// ── 6. Human-readable narrative ────────────────────────────────────
	rca.Narrative = buildRCANarrative(incident, rca)

	return rca, nil
}

// AutoDiscover mines incident history to populate the topology graph.
func (s *TopologyIntelligenceService) AutoDiscover(tenantID string) (int, error) {
	return s.topoStore.AutoDiscoverFromIncidents(tenantID)
}

// ─── RCA helpers ──────────────────────────────────────────────────────────────

func buildRootCause(detail models.IncidentDetail) *models.RootCauseNode {
	wc := detail.WhatChanged
	incident := detail.Incident
	if wc.Type == "" {
		return &models.RootCauseNode{
			Type: "unknown", Service: incident.Service,
			Description: "No deployment or configuration change detected in the relevant window.",
			Confidence:  25,
		}
	}

	rcType := "deployment"
	switch strings.ToLower(wc.Type) {
	case "config", "config_change", "configuration":
		rcType = "config_change"
	case "scale", "scaling":
		rcType = "traffic_surge"
	}

	conf := incident.WhatChangedConfidence
	if conf == 0 {
		conf = 60
	}

	desc := fmt.Sprintf("%s on %s", wc.Type, wc.Service)
	if wc.Version != "" {
		desc += fmt.Sprintf(" (version %s)", wc.Version)
	}
	if wc.Description != "" {
		desc = wc.Description
	}

	return &models.RootCauseNode{
		Type:        rcType,
		Service:     wc.Service,
		Description: desc,
		Version:     wc.Version,
		Timestamp:   wc.Timestamp,
		Confidence:  conf,
	}
}

func buildDegradationChain(detail models.IncidentDetail) []models.DegradationEvent {
	type rawEvent struct {
		service string
		t       time.Time
		id      string
		sev     string
		desc    string
	}

	var raw []rawEvent
	for _, te := range detail.Events {
		ev := te.Event
		svc := ev.Service
		if svc == "" {
			svc = detail.Incident.Service
		}
		t := ev.Timestamp
		if t.IsZero() {
			t = detail.Incident.FirstEventTime
		}
		raw = append(raw, rawEvent{
			service: svc, t: t, id: ev.ID,
			sev: ev.Severity, desc: ev.Title,
		})
	}

	// Sort chronologically.
	sort.Slice(raw, func(i, j int) bool { return raw[i].t.Before(raw[j].t) })

	// One DegradationEvent per service (first alert for that service).
	seen := map[string]bool{}
	var chain []models.DegradationEvent
	for idx, r := range raw {
		if seen[r.service] {
			continue
		}
		seen[r.service] = true
		chain = append(chain, models.DegradationEvent{
			ServiceName: r.service,
			EventID:     r.id,
			Timestamp:   r.t,
			Severity:    r.sev,
			Signal:      "alert",
			Description: r.desc,
			IsOrigin:    idx == 0,
		})
	}

	// Mark the first entry as the causal origin.
	if len(chain) > 0 {
		chain[0].IsOrigin = true
	}
	return chain
}

func buildEvidenceNodes(detail models.IncidentDetail, chain []models.DegradationEvent) []models.EvidenceNode {
	var ev []models.EvidenceNode
	firstAlert := time.Time{}
	if len(chain) > 0 {
		firstAlert = chain[0].Timestamp
	}

	// Change evidence — strongest signal.
	if detail.WhatChanged.Type != "" {
		conf := detail.Incident.WhatChangedConfidence
		if conf == 0 {
			conf = 65
		}
		// Boost if change happened < 1 h before first alert.
		if !firstAlert.IsZero() {
			if ts, err := time.Parse(time.RFC3339, detail.WhatChanged.Timestamp); err == nil {
				if firstAlert.Sub(ts) < time.Hour && firstAlert.After(ts) {
					conf = min(conf+15, 95)
				}
			}
		}
		ev = append(ev, models.EvidenceNode{
			ID: "change-" + detail.WhatChanged.Type,
			Type: "change", Label: "Deployment / Change Event",
			Detail:             detail.WhatChanged.Type + " on " + detail.WhatChanged.Service,
			Confidence:         conf,
			Timestamp:          detail.WhatChanged.Timestamp,
			SupportsConclusion: "Root cause: " + detail.WhatChanged.Type + " preceded the incident",
		})
	}

	// Alert evidence — one node per source bucket.
	sourceCounts := map[string]int{}
	for _, te := range detail.Events {
		src := te.Event.Source
		if src == "" {
			src = "unknown"
		}
		sourceCounts[src]++
	}
	for src, count := range sourceCounts {
		conf := min(70+count*2, 90)
		ev = append(ev, models.EvidenceNode{
			ID: "alert-" + src, Type: "alert",
			Label:              fmt.Sprintf("Alerts from %s (%d)", src, count),
			Detail:             fmt.Sprintf("%d alerts fired on %s", count, detail.Incident.Service),
			Confidence:         conf,
			Timestamp:          firstAlert.Format(time.RFC3339),
			SupportsConclusion: "Correlated alerts confirm service degradation",
		})
	}

	// EvidenceRef signals (metric, log, trace, etc.).
	typeCount := map[string]int{}
	for _, ref := range detail.EvidenceRefs {
		typeCount[ref.Type]++
	}
	for evType, count := range typeCount {
		if evType == "event" {
			continue
		}
		conf := evidenceConfidence(evType, count)
		label := strings.ToUpper(evType[:1]) + evType[1:] + " signals"
		ev = append(ev, models.EvidenceNode{
			ID: "evidence-" + evType, Type: evType,
			Label:              fmt.Sprintf("%s (%d)", label, count),
			Detail:             fmt.Sprintf("%d %s evidence items collected", count, evType),
			Confidence:         conf,
			Timestamp:          firstAlert.Format(time.RFC3339),
			SupportsConclusion: "Confirms service state during the incident window",
		})
	}

	// Topology evidence — if we have graph data, that's structural evidence.
	if detail.WhatChanged.Type != "" && len(detail.Incident.ImpactedServices) > 0 {
		ev = append(ev, models.EvidenceNode{
			ID: "topology-propagation", Type: "topology",
			Label:              "Topology Propagation",
			Detail:             fmt.Sprintf("Failure propagated from %s to %d downstream services", detail.Incident.Service, len(detail.Incident.ImpactedServices)),
			Confidence:         72,
			Timestamp:          firstAlert.Format(time.RFC3339),
			SupportsConclusion: "Graph traversal confirms blast radius boundaries",
		})
	}

	return ev
}

func computeOverallConfidence(evNodes []models.EvidenceNode, rc *models.RootCauseNode) int {
	if len(evNodes) == 0 {
		if rc != nil {
			return rc.Confidence
		}
		return 30
	}
	sum, weight := 0, 0
	for _, n := range evNodes {
		w := 1
		switch n.Type {
		case "change":
			w = 3
		case "topology":
			w = 2
		case "alert":
			w = 2
		}
		sum += n.Confidence * w
		weight += w
	}
	if weight == 0 {
		return 30
	}
	avg := sum / weight
	if rc != nil {
		avg = (avg*3 + rc.Confidence) / 4
	}
	if avg > 98 {
		return 98
	}
	return avg
}

func buildRCANarrative(incident models.Incident, rca *models.CausalRCAResult) models.CausalNarrative {
	// What happened
	whatHappened := fmt.Sprintf(
		"%s incident on %s (severity: %s). %d events correlated, %d services impacted.",
		incident.RootCauseType, incident.Service, incident.Severity,
		incident.EventCount, incident.ImpactCount,
	)
	if whatHappened == " incident on  (severity: ). 0 events correlated, 0 services impacted." {
		whatHappened = "An incident was detected. See event details for more information."
	}

	// What changed
	whatChanged := "No deployment or configuration change was detected in the relevant window."
	if rca.RootCause != nil && rca.RootCause.Type != "unknown" {
		whatChanged = fmt.Sprintf(
			"%s on service %q (confidence: %d%%). %s",
			rca.RootCause.Type, rca.RootCause.Service,
			rca.RootCause.Confidence, rca.RootCause.Description,
		)
		if rca.RootCause.Timestamp != "" {
			whatChanged += fmt.Sprintf(" Detected at %s.", rca.RootCause.Timestamp)
		}
	}

	// How it propagated
	howProp := "No topology graph data available — propagation path unknown."
	if len(rca.DegradationChain) > 1 {
		origin := rca.DegradationChain[0].ServiceName
		others := make([]string, 0, len(rca.DegradationChain)-1)
		for _, d := range rca.DegradationChain[1:] {
			others = append(others, d.ServiceName)
		}
		howProp = fmt.Sprintf(
			"Degradation originated in %q and propagated to: %s.",
			origin, strings.Join(others, ", "),
		)
	} else if len(rca.DegradationChain) == 1 {
		howProp = fmt.Sprintf("Degradation was isolated to %q — no lateral spread detected.", rca.DegradationChain[0].ServiceName)
	}
	if len(rca.DownstreamImpact) > 0 {
		names := make([]string, 0, min(3, len(rca.DownstreamImpact)))
		for _, d := range rca.DownstreamImpact {
			names = append(names, d.ServiceName)
			if len(names) == 3 {
				break
			}
		}
		extra := ""
		if len(rca.DownstreamImpact) > 3 {
			extra = fmt.Sprintf(" and %d more", len(rca.DownstreamImpact)-3)
		}
		howProp += fmt.Sprintf(" Blast radius includes: %s%s.", strings.Join(names, ", "), extra)
	}

	// Who is impacted
	whoImpacted := "No downstream impact confirmed."
	customerFacing := 0
	for _, d := range rca.DownstreamImpact {
		if d.IsCustomerFacing {
			customerFacing++
		}
	}
	total := len(rca.DownstreamImpact)
	if total > 0 {
		whoImpacted = fmt.Sprintf(
			"%d downstream service(s) affected, %d of which are customer-facing.",
			total, customerFacing,
		)
	}
	for _, d := range rca.UpstreamImpact {
		if d.IsCustomerFacing {
			customerFacing++
		}
	}
	if len(rca.UpstreamImpact) > 0 {
		whoImpacted += fmt.Sprintf(" %d upstream caller(s) also impacted.", len(rca.UpstreamImpact))
	}

	// Confidence summary
	confSummary := fmt.Sprintf(
		"Overall RCA confidence: %d%%. Based on %d evidence nodes (%s).",
		rca.OverallConfidence, len(rca.EvidenceNodes),
		describeEvidence(rca.EvidenceNodes),
	)

	return models.CausalNarrative{
		WhatHappened:      whatHappened,
		WhatChanged:       whatChanged,
		HowItPropagated:   howProp,
		WhoIsImpacted:     whoImpacted,
		ConfidenceSummary: confSummary,
	}
}

// ─── Shared helpers ───────────────────────────────────────────────────────────

// mapDBNodeType maps a DB node_type + traversal depth to the GraphNode node_type
// used by the frontend visualization.
func mapDBNodeType(dbType string, depth int) string {
	switch dbType {
	case "service":
		if depth == 1 {
			return "dependency"
		}
		return "impacted_service"
	case "database", "cache", "queue":
		return "dependency"
	case "infra", "pod":
		return "infra"
	case "team":
		return "owner"
	default:
		return "dependency"
	}
}

func evidenceConfidence(evType string, count int) int {
	base := 0
	switch evType {
	case "change":
		base = 85
	case "metric":
		base = 70 + min(count*3, 20)
	case "log":
		base = 65 + min(count*2, 15)
	case "trace":
		base = 72
	default:
		base = 60
	}
	if base > 95 {
		return 95
	}
	return base
}

func describeEvidence(nodes []models.EvidenceNode) string {
	types := map[string]bool{}
	for _, n := range nodes {
		types[n.Type] = true
	}
	list := make([]string, 0, len(types))
	for t := range types {
		list = append(list, t)
	}
	sort.Strings(list)
	return strings.Join(list, ", ")
}

// ─── Store delegate wrappers ──────────────────────────────────────────────────

// UpsertNode delegates to the underlying topology store.
func (s *TopologyIntelligenceService) UpsertNode(n models.TopologyNode) error {
	return s.topoStore.UpsertNode(n)
}

// UpsertEdge delegates to the underlying topology store.
func (s *TopologyIntelligenceService) UpsertEdge(e models.TopologyEdge) error {
	return s.topoStore.UpsertEdge(e)
}
