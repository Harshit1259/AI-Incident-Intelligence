package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"ai-incident-platform/backend/internal/config"
	"ai-incident-platform/backend/internal/handlers"
	"ai-incident-platform/backend/internal/routes"
	"ai-incident-platform/backend/internal/services"
	"ai-incident-platform/backend/internal/store"
)

func main() {
	cfg := config.Load()

	db, err := store.NewDB(cfg)
	if err != nil {
		log.Fatalf("database initialization failed: %v", err)
	}
	defer db.Close()

	mux := http.NewServeMux()

	devStore := store.NewDevStore(db)
	historyStore := store.NewIncidentStatusHistoryStore(db)
	changeStore := store.NewChangeStore(db)
	actionAuditStore := store.NewActionAuditStore(db)
	handlers.SetDefaultActionAuditStore(actionAuditStore)
	eventStore := store.NewEventStore(db)
	incidentStore := store.NewIncidentStore(db)

	correlationService := services.NewCorrelationService(
		incidentStore,
		changeStore,
	)

	incidentDetailService := services.NewIncidentDetailService(
		incidentStore,
		eventStore,
		historyStore,
	)

	incidentService := services.NewIncidentService(
		incidentStore,
		incidentDetailService,
		historyStore,
	)

	sourceRegistryService := services.NewSourceRegistryService()
	copilotService := services.NewCopilotService()
	demoService := services.NewDemoService(eventStore, correlationService, devStore)

	demoHandler := handlers.NewDemoHandler(demoService)
	devHandler := handlers.NewDevHandler(devStore)
	incidentHandler := handlers.NewIncidentHandler(incidentService)
	eventHandler := handlers.NewEventHandler(eventStore, correlationService)
	explainHandler := handlers.NewExplainHandler(incidentService)
	copilotHandler := handlers.NewCopilotHandler(incidentService, copilotService)
	activityHandler := handlers.NewActivityHandler(incidentService, actionAuditStore)
	ingestHandler := handlers.NewIngestHandler(eventStore, correlationService)
	sourceHandler := handlers.NewSourceHandler(sourceRegistryService, ingestHandler)

	routes.RegisterRoutes(
		mux,
		cfg,
		eventHandler,
		incidentHandler,
		explainHandler,
		copilotHandler,
		activityHandler,
		demoHandler,
		devHandler,
		ingestHandler,
		sourceHandler,
	)

	server := &http.Server{
		Addr:              cfg.HTTPAddress(),
		Handler:           mux,
		ReadTimeout:       10 * time.Second,
		ReadHeaderTimeout: 5 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	go func() {
		log.Printf(
			"backend server starting addr=%s frontend_origin=%s postgres_configured=%t",
			cfg.HTTPAddress(),
			cfg.FrontendOrigin,
			cfg.PostgresDSN != "",
		)

		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("http server failed: %v", err)
		}
	}()

	stopSignal := make(chan os.Signal, 1)
	signal.Notify(stopSignal, syscall.SIGINT, syscall.SIGTERM)
	<-stopSignal

	shutdownContext, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := server.Shutdown(shutdownContext); err != nil {
		log.Fatalf("graceful shutdown failed: %v", err)
	}
}	