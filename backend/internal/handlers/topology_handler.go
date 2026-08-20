package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"ai-incident-platform/backend/internal/api"
	"ai-incident-platform/backend/internal/middleware"
	"ai-incident-platform/backend/internal/models"
	"ai-incident-platform/backend/internal/services"
)

// TopologyHandler serves:
//   - GET  /api/v1/incidents/topology/{id}       — incident-specific evidence graph (backwards compat)
//   - GET  /api/v1/incidents/causal-dag/{id}     — full causal DAG with per-edge confidence + revenue
//   - GET  /api/v1/topology/graph                — full live tenant topology
//   - GET  /api/v1/topology/blast-radius/{node}  — downstream blast radius for a service
//   - GET  /api/v1/topology/rca/{incidentID}     — structured causal RCA
//   - POST /api/v1/topology/nodes                — upsert a topology node
//   - POST /api/v1/topology/edges                — upsert a topology edge
//   - POST /api/v1/topology/discover             — auto-discover nodes from incident history
type TopologyHandler struct {
	detailSvc    *services.IncidentDetailService
	intelSvc     *services.TopologyIntelligenceService
	causalEngine *services.CausalEngineService // may be nil; enables causal-dag endpoint
}

func NewTopologyHandler(
	ds *services.IncidentDetailService,
	is *services.TopologyIntelligenceService,
) *TopologyHandler {
	return &TopologyHandler{detailSvc: ds, intelSvc: is}
}

// SetCausalEngine wires in the causal engine for the /causal-dag endpoint.
func (h *TopologyHandler) SetCausalEngine(ce *services.CausalEngineService) {
	h.causalEngine = ce
}

// ─── Backwards-compat endpoint ────────────────────────────────────────────────

// HandleGetTopology handles GET /api/v1/incidents/topology/{id}
// Returns the enriched graph using the intelligence service when available,
// falling back to the legacy static-config builder.
func (h *TopologyHandler) HandleGetTopology(w http.ResponseWriter, r *http.Request) {
	claims, ok := middleware.ClaimsFromContext(r)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	tenantID := claims.TenantID

	incidentID := strings.TrimSpace(strings.TrimPrefix(r.URL.Path, "/api/v1/incidents/topology/"))
	if incidentID == "" || strings.Contains(incidentID, "/") {
		http.Error(w, "missing incident id", http.StatusBadRequest)
		return
	}

	detail, found := h.detailSvc.GetIncidentDetail(incidentID)
	if !found {
		http.Error(w, "incident not found", http.StatusNotFound)
		return
	}

	var graph models.GraphData
	if h.intelSvc != nil {
		graph = h.intelSvc.BuildIncidentTopologyGraph(detail, tenantID)
	} else {
		graph = services.BuildTopologyGraph(detail)
	}

	api.WriteJSON(w, http.StatusOK, graph)
}

// ─── Live Graph ───────────────────────────────────────────────────────────────

// HandleGetLiveGraph handles GET /api/v1/topology/graph
// Returns the full persisted multi-layer topology for the tenant.
func (h *TopologyHandler) HandleGetLiveGraph(w http.ResponseWriter, r *http.Request) {
	claims, ok := middleware.ClaimsFromContext(r)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	graph, err := h.intelSvc.BuildLiveGraph(claims.TenantID)
	if err != nil {
		http.Error(w, "failed to build topology graph: "+err.Error(), http.StatusInternalServerError)
		return
	}

	api.WriteJSON(w, http.StatusOK, graph)
}

// ─── Blast Radius ─────────────────────────────────────────────────────────────

// HandleGetBlastRadius handles GET /api/v1/topology/blast-radius/{nodeID}
// nodeID may be a service name (URL-encoded) or a DB node ID.
func (h *TopologyHandler) HandleGetBlastRadius(w http.ResponseWriter, r *http.Request) {
	claims, ok := middleware.ClaimsFromContext(r)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	nodeID := strings.TrimPrefix(r.URL.Path, "/api/v1/topology/blast-radius/")
	nodeID = strings.TrimSpace(nodeID)
	if nodeID == "" {
		http.Error(w, "missing node id", http.StatusBadRequest)
		return
	}

	result, err := h.intelSvc.ComputeBlastRadius(nodeID, claims.TenantID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	writeJSON(w, http.StatusOK, result)
}

// ─── Causal RCA ───────────────────────────────────────────────────────────────

// HandleGetRCA handles GET /api/v1/topology/rca/{incidentID}
// Returns the structured causal RCA: what changed, degradation chain, impact, per-evidence confidence.
func (h *TopologyHandler) HandleGetRCA(w http.ResponseWriter, r *http.Request) {
	claims, ok := middleware.ClaimsFromContext(r)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	incidentID := strings.TrimPrefix(r.URL.Path, "/api/v1/topology/rca/")
	incidentID = strings.TrimSpace(incidentID)
	if incidentID == "" {
		http.Error(w, "missing incident id", http.StatusBadRequest)
		return
	}

	detail, found := h.detailSvc.GetIncidentDetail(incidentID)
	if !found {
		http.Error(w, "incident not found", http.StatusNotFound)
		return
	}

	rca, err := h.intelSvc.GenerateCausalRCA(detail, claims.TenantID)
	if err != nil {
		http.Error(w, "rca computation failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, rca)
}

// ─── Causal DAG ───────────────────────────────────────────────────────────────

// HandleGetCausalDAG handles GET /api/v1/incidents/causal-dag/{id}
// Returns the full causal DAG: every node, every directed edge, per-edge confidence scores,
// the primary critical-path chain with narrative, and optional revenue loss leaf node.
func (h *TopologyHandler) HandleGetCausalDAG(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		api.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	claims, ok := middleware.ClaimsFromContext(r)
	if !ok {
		api.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	// Path: /api/v1/incidents/causal-dag/{id}
	incidentID := strings.TrimSpace(strings.TrimPrefix(r.URL.Path, "/api/v1/incidents/causal-dag/"))
	if incidentID == "" || strings.Contains(incidentID, "/") {
		api.WriteError(w, http.StatusBadRequest, "missing incident id")
		return
	}

	if h.causalEngine == nil {
		api.WriteError(w, http.StatusServiceUnavailable, "causal engine not configured")
		return
	}

	dag, err := h.causalEngine.BuildCausalDAG(incidentID, claims.TenantID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			api.WriteError(w, http.StatusNotFound, err.Error())
			return
		}
		api.WriteError(w, http.StatusInternalServerError, "causal dag computation failed")
		return
	}

	api.WriteJSON(w, http.StatusOK, dag)
}

// ─── Node Management ──────────────────────────────────────────────────────────

// HandleUpsertNode handles POST /api/v1/topology/nodes
// Caller provides a partial TopologyNode; the handler fills in tenant ID and generates an ID.
func (h *TopologyHandler) HandleUpsertNode(w http.ResponseWriter, r *http.Request) {
	claims, ok := middleware.ClaimsFromContext(r)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	var node models.TopologyNode
	if err := json.NewDecoder(r.Body).Decode(&node); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if node.Name == "" {
		http.Error(w, "name is required", http.StatusBadRequest)
		return
	}

	node.TenantID = claims.TenantID
	if node.ID == "" {
		node.ID = fmt.Sprintf("%s:%s", claims.TenantID, node.Name)
	}
	if node.NodeType == "" {
		node.NodeType = "service"
	}
	if node.Tier == "" {
		node.Tier = "internal"
	}
	if node.Environment == "" {
		node.Environment = "production"
	}
	if node.DisplayName == "" {
		node.DisplayName = node.Name
	}

	if err := h.intelSvc.UpsertNode(node); err != nil {
		http.Error(w, "failed to upsert node: "+err.Error(), http.StatusInternalServerError)
		return
	}

	api.WriteJSON(w, http.StatusOK, map[string]string{"id": node.ID, "status": "ok"})
}

// HandleUpsertEdge handles POST /api/v1/topology/edges
func (h *TopologyHandler) HandleUpsertEdge(w http.ResponseWriter, r *http.Request) {
	claims, ok := middleware.ClaimsFromContext(r)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	var edge models.TopologyEdge
	if err := json.NewDecoder(r.Body).Decode(&edge); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if edge.FromNodeID == "" || edge.ToNodeID == "" {
		http.Error(w, "from_node_id and to_node_id are required", http.StatusBadRequest)
		return
	}

	edge.TenantID = claims.TenantID
	if edge.ID == "" {
		edge.ID = fmt.Sprintf("%s:%s->%s:%s", claims.TenantID, edge.FromNodeID, edge.ToNodeID, edge.Relation)
	}
	if edge.Relation == "" {
		edge.Relation = "depends_on"
	}
	if edge.Weight == 0 {
		edge.Weight = 1.0
	}
	if edge.PropagationFactor == 0 {
		edge.PropagationFactor = 0.7
	}
	if edge.Confidence == 0 {
		edge.Confidence = 70
	}

	if err := h.intelSvc.UpsertEdge(edge); err != nil {
		http.Error(w, "failed to upsert edge: "+err.Error(), http.StatusInternalServerError)
		return
	}

	api.WriteJSON(w, http.StatusOK, map[string]string{"id": edge.ID, "status": "ok"})
}

// ─── Auto-discovery ───────────────────────────────────────────────────────────

// HandleDiscover handles POST /api/v1/topology/discover
// Mines incident history to auto-populate the topology graph.
func (h *TopologyHandler) HandleDiscover(w http.ResponseWriter, r *http.Request) {
	claims, ok := middleware.ClaimsFromContext(r)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	count, err := h.intelSvc.AutoDiscover(claims.TenantID)
	if err != nil {
		http.Error(w, "auto-discovery failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	api.WriteJSON(w, http.StatusOK, map[string]any{
		"nodes_discovered": count,
		"discovered_at":    time.Now().Format(time.RFC3339),
	})
}

