package main

import (
	"log"
	"net/http"

	"ai-incident-platform/backend/internal/handlers"
	"ai-incident-platform/backend/internal/routes"
	"ai-incident-platform/backend/internal/services"
	"ai-incident-platform/backend/internal/store"
)

func main() {
	mux := http.NewServeMux()

	eventStore := store.NewEventStore()
	incidentStore := store.NewIncidentStore()

	correlationService := services.NewCorrelationService(incidentStore)
	incidentDetailService := services.NewIncidentDetailService(incidentStore, eventStore)

	eventHandler := handlers.NewEventHandler(eventStore, correlationService)
	incidentHandler := handlers.NewIncidentHandler(incidentStore, incidentDetailService)

	routes.RegisterRoutes(mux, eventHandler, incidentHandler)

	log.Println("Backend server running on :8080")

	if err := http.ListenAndServe(":8080", mux); err != nil {
		log.Fatal(err)
	}
}
