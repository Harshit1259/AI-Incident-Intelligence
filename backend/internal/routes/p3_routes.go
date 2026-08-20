package routes

import (
	"net/http"
	"strings"

	"ai-incident-platform/backend/internal/handlers"
	"ai-incident-platform/backend/internal/middleware"
)

// registerP3Routes wires Phase 3 category-defining endpoints:
//   - Topology / evidence graph (incident-specific + tenant-wide live graph)
//   - Blast radius traversal
//   - Structured causal RCA
//   - Node / edge management and auto-discovery
//   - Incident memory & playbook learning
//   - Business risk / exposure dashboard
//   - AI provider status (public)
func registerP3Routes(
	mux *http.ServeMux,
	withAuth func(http.HandlerFunc) http.Handler,
	requireOperator func(http.HandlerFunc) http.HandlerFunc,
	topologyHandler *handlers.TopologyHandler,
	incidentMemoryHandler *handlers.IncidentMemoryHandler,
	riskExposureHandler *handlers.RiskExposureHandler,
	aiStatusHandler *handlers.AIStatusHandler,
) {
	// ── Topology: incident-specific evidence graph (backwards compat) ─────────
	// GET /api/v1/incidents/topology/{id}
	mux.Handle("/api/v1/incidents/topology/", withAuth(methodHandler(map[string]http.HandlerFunc{
		http.MethodGet: topologyHandler.HandleGetTopology,
	})))

	// ── Causal AI DAG — full causation chain with per-edge confidence ─────────
	// GET /api/v1/incidents/{id}/causal-dag
	// Returns: nodes → edges (directed, with confidence %) → primary chain narrative → revenue impact
	mux.Handle("/api/v1/incidents/causal-dag/", withAuth(methodHandler(map[string]http.HandlerFunc{
		http.MethodGet: topologyHandler.HandleGetCausalDAG,
	})))

	// ── Topology: live tenant-wide graph ──────────────────────────────────────
	// GET /api/v1/topology/graph
	mux.Handle("/api/v1/topology/graph", withAuth(methodHandler(map[string]http.HandlerFunc{
		http.MethodGet: topologyHandler.HandleGetLiveGraph,
	})))

	// ── Topology: blast radius (downstream impact traversal) ──────────────────
	// GET /api/v1/topology/blast-radius/{nodeID}
	mux.Handle("/api/v1/topology/blast-radius/", withAuth(methodHandler(map[string]http.HandlerFunc{
		http.MethodGet: topologyHandler.HandleGetBlastRadius,
	})))

	// ── Topology: causal RCA for an incident ─────────────────────────────────
	// GET /api/v1/topology/rca/{incidentID}
	mux.Handle("/api/v1/topology/rca/", withAuth(methodHandler(map[string]http.HandlerFunc{
		http.MethodGet: topologyHandler.HandleGetRCA,
	})))

	// ── Topology: node & edge management (operator+) ──────────────────────────
	// POST /api/v1/topology/nodes
	// POST /api/v1/topology/edges
	mux.Handle("/api/v1/topology/nodes", withAuth(requireOperator(topologyHandler.HandleUpsertNode)))
	mux.Handle("/api/v1/topology/edges", withAuth(requireOperator(topologyHandler.HandleUpsertEdge)))

	// ── Topology: auto-discovery from incident history (operator+) ────────────
	// POST /api/v1/topology/discover
	mux.Handle("/api/v1/topology/discover", withAuth(requireOperator(topologyHandler.HandleDiscover)))

	// ── Incident memory ───────────────────────────────────────────────────────
	// GET  /incidents/memory/{id}         — viewer readable (pattern history)
	// POST /incidents/memory/{id}/record  — operator+ (recording a resolution)
	mux.Handle("/api/v1/incidents/memory/", withAuth(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/record") {
			requireOperator(incidentMemoryHandler.HandleRecordResolution)(w, r)
			return
		}
		incidentMemoryHandler.HandleGetMemory(w, r)
	}))

	// ── Risk / exposure dashboard — viewer readable ───────────────────────────
	mux.Handle("/api/v1/risk/exposure", withAuth(methodHandler(map[string]http.HandlerFunc{
		http.MethodGet: riskExposureHandler.HandleGetExposure,
	})))
	mux.Handle("/api/v1/risk/at-risk-services", withAuth(methodHandler(map[string]http.HandlerFunc{
		http.MethodGet: riskExposureHandler.HandleGetAtRiskServices,
	})))

	// ── AI status — public (no JWT required) ─────────────────────────────────
	mux.Handle("/api/v1/ai/status", middleware.RequestID(middleware.RequestLogger(
		http.HandlerFunc(aiStatusHandler.HandleGetAIStatus),
	)))
}
