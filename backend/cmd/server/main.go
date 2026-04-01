package main

import (
	"log"
	"net/http"

	"ai-incident-platform/backend/internal/handlers"
	"ai-incident-platform/backend/internal/routes"
	"ai-incident-platform/backend/internal/store"
)

func main() {
	mux := http.NewServeMux()

	eventStore := store.NewEventStore()
	eventHandler := handlers.NewEventHandler(eventStore)

	routes.RegisterRoutes(mux, eventHandler)

	log.Println("Backend server running on :8080")

	if err := http.ListenAndServe(":8080", mux); err != nil {
		log.Fatal(err)
	}
}
