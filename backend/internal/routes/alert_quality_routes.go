package routes

import (
	"net/http"

	"ai-incident-platform/backend/internal/handlers"
)

func registerAlertQualityRoutes(
	mux *http.ServeMux,
	withAuth func(http.HandlerFunc) http.Handler,
	h *handlers.AlertQualityHandler,
) {
	mux.Handle("/api/v1/alert-quality/report", withAuth(methodHandler(map[string]http.HandlerFunc{
		http.MethodGet: h.HandleReport,
	})))

	mux.Handle("/api/v1/alert-quality/noisy", withAuth(methodHandler(map[string]http.HandlerFunc{
		http.MethodGet: h.HandleNoisy,
	})))

	mux.Handle("/api/v1/alert-quality/duplicates", withAuth(methodHandler(map[string]http.HandlerFunc{
		http.MethodGet: h.HandleDuplicates,
	})))

	mux.Handle("/api/v1/alert-quality/stale", withAuth(methodHandler(map[string]http.HandlerFunc{
		http.MethodGet: h.HandleStale,
	})))

	mux.Handle("/api/v1/alert-quality/debt", withAuth(methodHandler(map[string]http.HandlerFunc{
		http.MethodGet: h.HandleDebt,
	})))

	mux.Handle("/api/v1/alert-quality/rules", withAuth(methodHandler(map[string]http.HandlerFunc{
		http.MethodPost: h.HandleRegisterRule,
	})))
}
