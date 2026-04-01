package routes

import (
	"net/http"
	"strings"

	"ai-incident-platform/backend/internal/handlers"
)

func EnableCORS(handler http.HandlerFunc) http.HandlerFunc {
	return func(responseWriter http.ResponseWriter, request *http.Request) {
		responseWriter.Header().Set("Access-Control-Allow-Origin", "*")
		responseWriter.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		responseWriter.Header().Set("Access-Control-Allow-Headers", "Content-Type")

		if request.Method == http.MethodOptions {
			responseWriter.WriteHeader(http.StatusOK)
			return
		}

		handler(responseWriter, request)
	}
}

func RegisterRoutes(mux *http.ServeMux, eventHandler *handlers.EventHandler, incidentHandler *handlers.IncidentHandler) {
	mux.HandleFunc("/api/v1/health", EnableCORS(handlers.HealthHandler))

	mux.HandleFunc("/api/v1/events", EnableCORS(func(responseWriter http.ResponseWriter, request *http.Request) {
		switch request.Method {
		case http.MethodGet:
			eventHandler.ListEvents(responseWriter, request)
		case http.MethodPost:
			eventHandler.CreateEvent(responseWriter, request)
		default:
			http.Error(responseWriter, "method not allowed", http.StatusMethodNotAllowed)
		}
	}))

	mux.HandleFunc("/api/v1/incidents", EnableCORS(func(responseWriter http.ResponseWriter, request *http.Request) {
		switch request.Method {
		case http.MethodGet:
			eventHandler := incidentHandler
			eventHandler.ListIncidents(responseWriter, request)
		default:
			http.Error(responseWriter, "method not allowed", http.StatusMethodNotAllowed)
		}
	}))

	mux.HandleFunc("/api/v1/incidents/", EnableCORS(func(responseWriter http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet {
			http.Error(responseWriter, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		if !strings.HasPrefix(request.URL.Path, "/api/v1/incidents/") {
			http.Error(responseWriter, "not found", http.StatusNotFound)
			return
		}

		incidentHandler.GetIncidentDetail(responseWriter, request)
	}))
}
