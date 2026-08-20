package routes

import (
	"net/http"

	"ai-incident-platform/backend/internal/handlers"
)

func registerPredictiveRoutes(
	mux *http.ServeMux,
	withAuth func(http.HandlerFunc) http.Handler,
	requireOperator func(http.HandlerFunc) http.HandlerFunc,
	h *handlers.PredictiveIncidentHandler,
) {
	// List predictive incidents — viewer-readable
	mux.Handle("/api/v1/predictive-incidents", withAuth(h.HandleList))

	// Record a raw metric data point — operator write
	mux.Handle("/api/v1/predictive-incidents/metrics", withAuth(requireOperator(h.HandleRecordMetric)))

	// Trigger on-demand trend evaluation — operator
	mux.Handle("/api/v1/predictive-incidents/evaluate", withAuth(requireOperator(h.HandleEvaluate)))

	// Demo / simulation — operator
	mux.Handle("/api/v1/predictive-incidents/simulate", withAuth(requireOperator(h.HandleSimulate)))

	// Resolve or mark false alarm — operator
	// Route: /api/v1/predictive-incidents/{id}/resolve
	mux.Handle("/api/v1/predictive-incidents/", withAuth(requireOperator(h.HandleResolve)))
}
