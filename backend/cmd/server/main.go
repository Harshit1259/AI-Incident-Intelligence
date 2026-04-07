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
	"ai-incident-platform/backend/internal/llm"
	"ai-incident-platform/backend/internal/middleware"
	"ai-incident-platform/backend/internal/routes"
	"ai-incident-platform/backend/internal/services"
	"ai-incident-platform/backend/internal/store"
)

func main() {
	cfg := config.Load()

	// ── Database ──────────────────────────────────────
	db, err := store.NewDB(cfg)
	if err != nil {
		log.Fatalf("database initialization failed: %v", err)
	}
	defer db.Close()

	// ── Stores ────────────────────────────────────────
	devStore := store.NewDevStore(db)
	historyStore := store.NewIncidentStatusHistoryStore(db)
	changeStore := store.NewChangeStore(db)
	actionAuditStore := store.NewActionAuditStore(db)
	eventStore := store.NewEventStore(db)
	incidentStore := store.NewIncidentStore(db)
	userStore := store.NewUserStore(db)

	// Phase 2 stores
	postMortemStore := store.NewPostMortemStore(db)

	// Phase 3 stores
	sloStore := store.NewSLOStore(db)
	oncallStore := store.NewOnCallStore(db)
	anomalyStore := store.NewAnomalyStore(db)
	incidentMetricsStore := store.NewIncidentMetricsStore(db)

	// Phase 4 stores
	billingStore := store.NewBillingStore(db)
	onboardingStore := store.NewOnboardingStore(db)

	handlers.SetDefaultActionAuditStore(actionAuditStore)

	// ── LLM client (Anthropic) ────────────────────────
	llmClient := llm.New(cfg.AnthropicAPIKey, cfg.AnthropicModel)
	if llmClient.IsConfigured() {
		log.Printf("llm: Anthropic API configured (model=%s)", cfg.AnthropicModel)
	} else {
		log.Printf("llm: ANTHROPIC_API_KEY not set — copilot/explain will use rule-based fallback")
	}

	// ── Services ──────────────────────────────────────
	correlationService := services.NewCorrelationService(
		incidentStore,
		eventStore,
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
	copilotService := services.NewCopilotService(llmClient)
	explainService := services.NewExplainService(llmClient)
	demoService := services.NewDemoService(eventStore, correlationService, devStore)

	// Phase 2 services
	changeLinkerService := services.NewChangeLinkerService(incidentStore, changeStore)
	slackService := services.NewSlackService(cfg.SlackBotToken, cfg.SlackSigningSecret)
	postMortemService := services.NewPostMortemService(llmClient, postMortemStore, incidentDetailService)

	// Phase 3 services
	sloService := services.NewSLOService(sloStore)
	oncallService := services.NewOnCallService(oncallStore)
	anomalyService := services.NewAnomalyService(anomalyStore, incidentStore, eventStore, correlationService)
	engineeringHealthService := services.NewEngineeringHealthService(incidentMetricsStore, oncallService)
	roiService := services.NewROIService(incidentMetricsStore, incidentStore, cfg.SREHourlyCost)
	digestService := services.NewDigestService(incidentStore, incidentMetricsStore, llmClient)

	// Phase 4 services
	billingService := services.NewBillingService(billingStore)
	onboardingService := services.NewOnboardingService(onboardingStore, billingService)

	// Wire Slack into correlation service (for auto-channel creation on critical incidents).
	correlationService.SetSlackService(slackService)

	// Wire engineering health into correlation + incident service for metrics recording.
	correlationService.SetEngineeringHealthService(engineeringHealthService)
	incidentService.SetEngineeringHealthService(engineeringHealthService)

	// Wire onboarding into correlation service for milestone tracking.
	correlationService.SetOnboardingService(onboardingService)

	// Ensure starter plan exists for default tenant.
	if err := billingService.EnsureStarterPlan("default"); err != nil {
		log.Printf("billing: failed to ensure starter plan for default tenant: %v", err)
	}

	if cfg.SlackEnabled() {
		log.Printf("slack: integration configured")
	}

	// ── Handlers ──────────────────────────────────────
	mux := http.NewServeMux()

	authHandler := handlers.NewAuthHandler(userStore, cfg.JWTSecret)
	demoHandler := handlers.NewDemoHandler(demoService)
	devHandler := handlers.NewDevHandler(devStore)
	incidentHandler := handlers.NewIncidentHandler(incidentService)
	eventHandler := handlers.NewEventHandler(eventStore, correlationService)
	explainHandler := handlers.NewExplainHandler(incidentService, explainService)
	copilotHandler := handlers.NewCopilotHandler(incidentService, copilotService)
	activityHandler := handlers.NewActivityHandler(incidentService, actionAuditStore)
	ingestHandler := handlers.NewIngestHandler(eventStore, correlationService)
	sourceHandler := handlers.NewSourceHandler(sourceRegistryService, ingestHandler)

	// Phase 2 handlers
	githubHandler := handlers.NewGitHubWebhookHandler(changeLinkerService, cfg.GitHubWebhookSecret)
	gitlabHandler := handlers.NewGitLabWebhookHandler(changeLinkerService, cfg.GitLabWebhookToken)
	pagerdutyHandler := handlers.NewPagerDutyWebhookHandler(eventStore, correlationService)
	datadogHandler := handlers.NewDatadogWebhookHandler(eventStore, correlationService)
	slackHandler := handlers.NewSlackHandler(slackService, incidentStore)
	postmortemHandler := handlers.NewPostMortemHandler(postMortemService)
	statusHandler := handlers.NewStatusHandler(incidentStore)

	// Phase 3 handlers
	sloHandler := handlers.NewSLOHandler(sloService, sloStore)
	oncallHandler := handlers.NewOnCallHandler(oncallService, oncallStore)
	anomalyHandler := handlers.NewAnomalyHandler(anomalyService, anomalyStore)
	engineeringHealthHandler := handlers.NewEngineeringHealthHandler(engineeringHealthService)
	roiHandler := handlers.NewROIHandler(roiService)
	digestHandler := handlers.NewDigestHandler(digestService)

	// Phase 4 handlers
	billingHandler := handlers.NewBillingHandler(billingService)
	onboardingHandler := handlers.NewOnboardingHandler(onboardingService)

	// ── Routes ────────────────────────────────────────
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
		authHandler,
		// Phase 2
		githubHandler,
		gitlabHandler,
		pagerdutyHandler,
		datadogHandler,
		slackHandler,
		postmortemHandler,
		statusHandler,
		// Phase 3
		sloHandler,
		oncallHandler,
		anomalyHandler,
		engineeringHealthHandler,
		roiHandler,
		digestHandler,
		// Phase 4
		billingHandler,
		onboardingHandler,
	)

	// ── HTTP Server ───────────────────────────────────
	server := &http.Server{
		Addr:              cfg.HTTPAddress(),
		Handler:           middleware.Recovery(mux),
		ReadTimeout:       10 * time.Second,
		ReadHeaderTimeout: 5 * time.Second,
		WriteTimeout:      60 * time.Second, // increased for LLM calls (up to 30 s)
		IdleTimeout:       60 * time.Second,
	}

	go func() {
		log.Printf(
			"backend server starting addr=%s frontend_origin=%s llm_enabled=%t",
			cfg.HTTPAddress(),
			cfg.FrontendOrigin,
			cfg.LLMEnabled(),
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
	log.Println("server stopped gracefully")
}
