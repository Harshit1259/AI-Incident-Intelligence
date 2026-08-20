package services

// TopologyService builds an enriched evidence-linked topology graph for an incident.
// It extends the basic service-dependency graph with:
//   - Evidence ref nodes (events, changes, metrics) linked to the services that produced them
//   - Alert origin nodes showing which integration/source fired
//   - Change event nodes when a deployment is linked
//   - Health status and signal confidence on every edge

import (
	"fmt"
	"strings"
	"time"

	"ai-incident-platform/backend/internal/config"
	"ai-incident-platform/backend/internal/models"
)

// BuildTopologyGraph constructs the full topology graph for an incident.
// It is richer than buildGraph() in incident_detail_service.go:
//   - Nodes carry health/status/alert counts
//   - Edges carry confidence and evidence counts
//   - Change event and evidence nodes are added as first-class graph members
func BuildTopologyGraph(detail models.IncidentDetail) models.GraphData {
	incident := detail.Incident

	nodes := map[string]models.GraphNode{}
	var edges []models.GraphEdge

	// ── Root: incident service ─────────────────────────
	rootID := incident.Service
	if rootID == "" {
		rootID = "unknown"
	}

	nodes[rootID] = models.GraphNode{
		ID:            rootID,
		Label:         humanizeNodeLabel(rootID),
		NodeType:      "service",
		Severity:      incident.Severity,
		Status:        severityToStatus(incident.Severity),
		AlertCount:    incident.EventCount,
		ChangeLinked:  incident.WhatChangedType != "",
		EvidenceCount: len(detail.EvidenceRefs),
	}

	// ── Dependencies ───────────────────────────────────
	for _, dep := range config.ServiceDependencies[strings.ToLower(rootID)] {
		if dep == "" || dep == rootID {
			continue
		}
		if _, exists := nodes[dep]; !exists {
			nodes[dep] = models.GraphNode{
				ID:       dep,
				Label:    humanizeNodeLabel(dep),
				NodeType: "dependency",
				Status:   "unknown",
			}
		}
		edges = append(edges, models.GraphEdge{
			From:       rootID,
			To:         dep,
			Relation:   "depends_on",
			Weight:     0.8,
			Confidence: 90,
		})
	}

	// ── Impacted services ──────────────────────────────
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
		edges = append(edges, models.GraphEdge{
			From:       rootID,
			To:         svc,
			Relation:   "impacts",
			Weight:     0.7,
			Confidence: 75,
		})
	}

	// ── Alert origin node ──────────────────────────────
	// Aggregate events by source; add one origin node per distinct source.
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
			ID:         originID,
			Label:      fmt.Sprintf("Alert: %s (%d)", src, count),
			NodeType:   "alert_origin",
			AlertCount: count,
		}
		edges = append(edges, models.GraphEdge{
			From:          originID,
			To:            rootID,
			Relation:      "fired_on",
			Weight:        1.0,
			Confidence:    100,
			EvidenceCount: count,
		})
	}

	// ── Change event node ──────────────────────────────
	if detail.WhatChanged.Type != "" {
		chgSvc := detail.WhatChanged.Service
		if chgSvc == "" {
			chgSvc = rootID
		}
		changeID := fmt.Sprintf("change:%s:%s", chgSvc, detail.WhatChanged.Type)
		versionLabel := ""
		if detail.WhatChanged.Version != "" {
			versionLabel = " @" + detail.WhatChanged.Version
		}
		nodes[changeID] = models.GraphNode{
			ID:           changeID,
			Label:        fmt.Sprintf("%s%s", detail.WhatChanged.Type, versionLabel),
			NodeType:     "change",
			ChangeLinked: true,
		}
		confidence := incident.WhatChangedConfidence
		if confidence == 0 {
			confidence = 60 // default when not scored
		}
		edges = append(edges, models.GraphEdge{
			From:       changeID,
			To:         rootID,
			Relation:   "preceded",
			Weight:     float64(confidence) / 100.0,
			Confidence: confidence,
		})
		// If change was on a different service, add an edge from that service to root too
		if chgSvc != rootID {
			if _, exists := nodes[chgSvc]; !exists {
				nodes[chgSvc] = models.GraphNode{
					ID:           chgSvc,
					Label:        humanizeNodeLabel(chgSvc),
					NodeType:     "dependency",
					ChangeLinked: true,
				}
			}
			edges = append(edges, models.GraphEdge{
				From:       chgSvc,
				To:         rootID,
				Relation:   "change_propagated",
				Weight:     float64(confidence) / 100.0,
				Confidence: confidence,
			})
		}
	}

	// ── Evidence nodes (metrics/logs) ──────────────────
	evidenceByType := map[string]int{}
	for _, ref := range detail.EvidenceRefs {
		evidenceByType[ref.Type]++
	}
	for evType, count := range evidenceByType {
		if evType == "event" {
			continue // events already shown as alert origins
		}
		evID := fmt.Sprintf("evidence:%s", evType)
		nodes[evID] = models.GraphNode{
			ID:            evID,
			Label:         fmt.Sprintf("%s evidence (%d)", strings.Title(evType), count),
			NodeType:      "evidence",
			EvidenceCount: count,
		}
		edges = append(edges, models.GraphEdge{
			From:          evID,
			To:            rootID,
			Relation:      "supports",
			Weight:        0.6,
			Confidence:    70,
			EvidenceCount: count,
		})
	}

	// ── Flatten nodes map → slice ──────────────────────
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

// ─────────────────────────────────────────────────────
// Helpers
// ─────────────────────────────────────────────────────

func severityToStatus(severity string) string {
	switch strings.ToLower(severity) {
	case "critical":
		return "down"
	case "high":
		return "degraded"
	case "medium", "low":
		return "degraded"
	default:
		return "unknown"
	}
}

func humanizeNodeLabel(id string) string {
	parts := strings.FieldsFunc(id, func(r rune) bool { return r == '-' || r == '_' || r == '.' })
	for i, p := range parts {
		if len(p) > 0 {
			parts[i] = strings.ToUpper(p[:1]) + p[1:]
		}
	}
	return strings.Join(parts, " ")
}
