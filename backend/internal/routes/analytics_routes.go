package routes

import (
	"net/http"

	"ai-incident-platform/backend/internal/handlers"
)

func registerAnalyticsRoutes(
	mux *http.ServeMux,
	withAuth func(http.HandlerFunc) http.Handler,
	requireOperator func(http.HandlerFunc) http.HandlerFunc,
	sloHandler *handlers.SLOHandler,
	oncallHandler *handlers.OnCallHandler,
	anomalyHandler *handlers.AnomalyHandler,
	engineeringHealthHandler *handlers.EngineeringHealthHandler,
	roiHandler *handlers.ROIHandler,
	digestHandler *handlers.DigestHandler,
) {
	// SLO — viewer reads, operator writes
	mux.Handle("/api/v1/slos", withAuth(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			sloHandler.HandleSLOs(w, r)
		default:
			requireOperator(sloHandler.HandleSLOs)(w, r)
		}
	}))
	mux.Handle("/api/v1/slos/", withAuth(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			sloHandler.HandleSLOByID(w, r)
		} else {
			requireOperator(sloHandler.HandleSLOByID)(w, r)
		}
	}))

	// On-call — viewer reads, operator writes
	mux.Handle("/api/v1/oncall", withAuth(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			oncallHandler.HandleOnCall(w, r)
		} else {
			requireOperator(oncallHandler.HandleOnCall)(w, r)
		}
	}))
	mux.Handle("/api/v1/oncall/", withAuth(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			oncallHandler.HandleOnCallByID(w, r)
		} else {
			requireOperator(oncallHandler.HandleOnCallByID)(w, r)
		}
	}))

	// Anomaly detection — viewer reads, operator writes / simulates
	mux.Handle("/api/v1/anomalies", withAuth(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			anomalyHandler.HandleAnomalies(w, r)
		} else {
			requireOperator(anomalyHandler.HandleAnomalies)(w, r)
		}
	}))
	mux.Handle("/api/v1/anomalies/check", withAuth(anomalyHandler.HandleCheckMetric))
	mux.Handle("/api/v1/anomalies/simulate", withAuth(requireOperator(anomalyHandler.HandleSimulate)))
	mux.Handle("/api/v1/anomalies/", withAuth(requireOperator(anomalyHandler.HandleAckAlert)))

	// Engineering health — viewer readable
	mux.Handle("/api/v1/engineering/health", withAuth(engineeringHealthHandler.HandleHealth))

	// ROI — viewer readable
	mux.Handle("/api/v1/roi", withAuth(roiHandler.HandleROI))

	// Weekly digest — read: operator+; send: operator+
	mux.Handle("/api/v1/digest/weekly", withAuth(digestHandler.HandleWeeklyDigest))
	mux.Handle("/api/v1/digest/send", withAuth(requireOperator(digestHandler.HandleSendDigest)))
}
