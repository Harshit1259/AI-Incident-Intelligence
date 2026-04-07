package routes

import (
	"net/http"

	"ai-incident-platform/backend/internal/handlers"
	"ai-incident-platform/backend/internal/services"
	"ai-incident-platform/backend/internal/store"
)

func RegisterIngestRoutes(
	mux *http.ServeMux,
	eventStore *store.EventStore,
	correlationService *services.CorrelationService,
) {
	ingestHandler := handlers.NewIngestHandler(eventStore, correlationService)

	mux.HandleFunc("/api/v1/ingest/webhook", ingestHandler.GenericWebhook)
	mux.HandleFunc("/api/v1/ingest/prometheus", ingestHandler.PrometheusWebhook)
}
