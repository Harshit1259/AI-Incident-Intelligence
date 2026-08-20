package main

import (
	"bufio"
	"context"
	"encoding/json"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"ai-incident-platform/backend/internal/audit"
	"ai-incident-platform/backend/internal/config"
	"ai-incident-platform/backend/internal/handlers"
	"ai-incident-platform/backend/internal/ingest"
	"ai-incident-platform/backend/internal/llm"
	"ai-incident-platform/backend/internal/middleware"
	"ai-incident-platform/backend/internal/models"
	"ai-incident-platform/backend/internal/modules"
	"ai-incident-platform/backend/internal/platform/edition"
	appLogger "ai-incident-platform/backend/internal/platform/logger"
	traceMiddleware "ai-incident-platform/backend/internal/platform/trace"
	tenantCache "ai-incident-platform/backend/internal/platform/cache"
	tenantCrypto "ai-incident-platform/backend/internal/platform/crypto"
	tenantQueue "ai-incident-platform/backend/internal/platform/queue"
	"ai-incident-platform/backend/internal/platform/ratelimit"
	"ai-incident-platform/backend/internal/routes"
	"ai-incident-platform/backend/internal/services"
	"ai-incident-platform/backend/internal/store"
)

// loadDotEnv reads a .env file and sets unset environment variables from it.
// Real environment variables always take precedence.
func loadDotEnv(path string) {
	f, err := os.Open(path)
	if err != nil {
		return // file absent — silently skip
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		idx := strings.IndexByte(line, '=')
		if idx <= 0 {
			continue
		}
		key := strings.TrimSpace(line[:idx])
		val := strings.TrimSpace(line[idx+1:])
		// Strip surrounding quotes if present
		if len(val) >= 2 && val[0] == '"' && val[len(val)-1] == '"' {
			val = val[1 : len(val)-1]
		}
		if key != "" && os.Getenv(key) == "" {
			os.Setenv(key, val)
		}
	}
}

func main() {
	// Load .env from project root (works when running from repo root or backend dir)
	for _, candidate := range []string{".env", "../.env", "../../.env", "../../../.env"} {
		if _, err := os.Stat(candidate); err == nil {
			loadDotEnv(candidate)
			log.Printf("config: loaded env from %s", candidate)
			break
		}
	}

	cfg := config.Load()

	// Initialise structured logging immediately — all subsequent log output is JSON
	// in production (machine-parseable) and coloured text in development.
	appLogger.Init(cfg.IsProd)

	// Print the configuration summary immediately after loading.
	// Shows every env var, whether it is set, and any warnings for missing
	// required vars — giving operators a complete picture on every startup.
	config.PrintStartupSummary(cfg)

	// ── Edition gate ──────────────────────────────────────
	gate := edition.ParseEditions(cfg.PlatformEditions)
	if err := modules.ValidateDependencies(gate); err != nil {
		log.Fatalf("platform edition configuration error: %v", err)
	}
	log.Printf("platform: active modules — %s", modules.Summary(gate))
	handlers.SetEditionGate(gate)

	// ── Database ──────────────────────────────────────
	db, err := store.NewDB(cfg)
	if err != nil {
		log.Fatalf("database initialization failed: %v", err)
	}
	defer db.Close()

	// ── Stores ────────────────────────────────────────
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

	// Phase 3 (category-defining) stores
	incidentMemoryStore := store.NewIncidentMemoryStore(db)
	topologyStore := store.NewTopologyStore(db)
	predictiveIncidentStore := store.NewPredictiveIncidentStore(db)

	// Phase 4: Change Intelligence
	changeIntelligenceStore := store.NewChangeIntelligenceStore(db)

	// Alert Quality Governance
	alertQualityStore := store.NewAlertQualityStore(db)

	// NeuroOps agent stores
	agentStore := store.NewAgentStore(db)
	agentEnrollmentStore := store.NewAgentEnrollmentStore(db)

	// Log Explorer store
	logStore := store.NewLogStore(db)

	// Webhook hardening stores
	deadLetterStore := store.NewDeadLetterStore(db)
	idempotencyStore := store.NewIdempotencyStore(db)

	// Gap feature stores
	businessImpactStore := store.NewBusinessImpactStore(db)
	alertFeedbackStore := store.NewAlertFeedbackStore(db)
	autoResolveStore := store.NewAutoResolveStore(db)
	runbookStore := store.NewRunbookStore(db)
	dependencyStore := store.NewDependencyStore(db)
	whatsappStore := store.NewWhatsAppStore(db)

	// OTel / schema registry stores
	schemaMappingStore := store.NewSchemaMappingStore(db)

	// Team Workflow Primitives store
	workflowStore := store.NewWorkflowStore(db)

	// AI Domain Memory store (Feature 9)
	domainMemoryStore := store.NewDomainMemoryStore(db)

	// Enterprise feature stores
	verificationStore := store.NewVerificationStore(db)
	policyStore := store.NewPolicyStore(db)
	policyTrailStore := store.NewPolicyExecutionTrailStore(db)
	circuitBreakerStore := store.NewCircuitBreakerStore(db)
	actionExecutionStore := store.NewActionExecutionStore(db)
	tenantConfigStore := store.NewTenantConfigStore(db)
	auditLogStore := store.NewAuditLogStore(db)

	// Tenant configuration layer stores
	tenantStore := store.NewTenantStore(db)
	serviceCatalogStore := store.NewServiceCatalogStore(db)
	tenantSettingsStore := store.NewTenantSettingsStore(db)

	// Initialize audit logger
	audit.SetStore(auditLogStore)

	// ── SaaS Multi-Tenancy platform components ──────────────────────────────

	// Tenant lifecycle enforcement: blocks requests from suspended/disabled tenants.
	tenantIsolation := middleware.NewTenantIsolation(tenantStore)

	// Per-tenant token-bucket rate limiter (lazily loads plan defaults from DB).
	tenantLimiter := ratelimit.New(func(tenantID string, cat ratelimit.Category) int {
		profile, err := tenantStore.GetEffectiveLimits(tenantID)
		if err != nil {
			return 100 // safe fallback
		}
		if cat == ratelimit.CategoryIngest {
			return profile.IngestPerMin
		}
		return profile.QueryPerMin
	})

	// Per-tenant TTL cache (2000 entries / tenant) and per-tenant job queue.
	tenantCacheManager := tenantCache.NewManager(2000)

	// Queue is started with a no-op processor; the real processor is wired
	// after the correlation service is initialised (see SetProcessor call below).
	tenantQueueManager := tenantQueue.New(500, 8, func(job tenantQueue.Job) { _ = job })

	// Per-tenant AES-256-GCM field-level encryptor.
	// When MASTER_ENCRYPTION_KEY is set, sensitive fields (ingest tokens, agent
	// secrets) are stored encrypted using a per-tenant derived key.
	tenantEncryptor, err := tenantCrypto.New(cfg.MasterEncryptionKey)
	if err != nil {
		log.Fatalf("crypto: %v", err)
	}
	if tenantEncryptor.IsEnabled() {
		log.Printf("crypto: field-level encryption active (AES-256-GCM, per-tenant key derivation)")
	}

	handlers.SetDefaultActionAuditStore(actionAuditStore)
	handlers.SetDefaultAgentStore(agentStore)

	// ── LLM client (provider-agnostic, supports cloud/private/offline) ──
	llmClient := llm.NewWithOptions(cfg.LLMAPIKey, cfg.LLMModel, cfg.LLMProvider, cfg.LLMBaseURL, cfg.LLMDataMode)
	if llmClient.IsConfigured() {
		log.Printf("llm: provider=%s model=%s mode=%s", llmClient.ProviderName(), cfg.LLMModel, llmClient.DataModeName())
	} else {
		log.Printf("llm: not configured — copilot/explain will use rule-based fallback")
	}

	// ── Knowledge Base ────────────────────────────────
	kb := services.NewKnowledgeBase()
	log.Printf("knowledge base: %d entries loaded", kb.Count())

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
		logStore,
		agentStore,
	)

	incidentService := services.NewIncidentService(
		incidentStore,
		incidentDetailService,
		historyStore,
	)

	sourceRegistryStore := store.NewSourceRegistryStore(db)
	sourceRegistryStore.SetEncryptor(tenantEncryptor) // encrypt ingest tokens at rest
	sourceRegistryService := services.NewSourceRegistryService(sourceRegistryStore)
	copilotService := services.NewCopilotService(llmClient)
	explainService := services.NewExplainService(llmClient)

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

	// Phase 3 category-defining services
	incidentMemoryService := services.NewIncidentMemoryService(incidentMemoryStore, incidentStore)
	riskExposureService := services.NewRiskExposureService(incidentStore, incidentMetricsStore)
	topologyIntelligenceService := services.NewTopologyIntelligenceService(topologyStore, incidentStore)
	predictiveIncidentService := services.NewPredictiveIncidentService(predictiveIncidentStore, correlationService)
	// Wire predictive engine into anomaly service — every metric check feeds the trend buffer.
	anomalyService.SetPredictiveService(predictiveIncidentService)

	// Phase 4: Change Intelligence
	changeIntelligenceService := services.NewChangeIntelligenceService(changeIntelligenceStore, incidentStore)
	// Wire enriched store into the change linker so webhook events persist full metadata.
	changeLinkerService.SetChangeIntelligenceStore(changeIntelligenceStore)

	// Phase 4 services
	stripePriceIDs := map[string]string{
		"starter": cfg.StripePriceStarter,
		"growth":  cfg.StripePriceGrowth,
		"scale":   cfg.StripePriceScale,
	}
	billingService := services.NewBillingService(billingStore, cfg.StripeSecretKey, stripePriceIDs, cfg.FrontendOrigin)
	onboardingService := services.NewOnboardingService(onboardingStore, billingService)

	// Tenant configuration layer service (must be created before businessImpactService)
	tenantConfigService := services.NewTenantConfigService(
		tenantStore,
		serviceCatalogStore,
		tenantSettingsStore,
		businessImpactStore,
	)
	importService := services.NewImportService(serviceCatalogStore, businessImpactStore)

	// Gap feature services
	businessImpactService := services.NewBusinessImpactService(businessImpactStore, incidentStore)
	alertFeedbackService := services.NewAlertFeedbackService(alertFeedbackStore, eventStore)
	autoResolveService := services.NewAutoResolveService(autoResolveStore, incidentStore)
	runbookService := services.NewRunbookService(runbookStore)
	dependencyService := services.NewDependencyService(dependencyStore)
	whatsappService := services.NewWhatsAppService(whatsappStore)
	complianceService := services.NewComplianceService(incidentStore, incidentMetricsStore, sloService)

	// Team Workflow Primitives services
	jiraService := services.NewJiraService(cfg.JiraBaseURL, cfg.JiraEmail, cfg.JiraAPIToken, cfg.JiraProjectKey)
	snowService := services.NewServiceNowService(cfg.ServiceNowURL, cfg.ServiceNowUser, cfg.ServiceNowPassword)
	teamsService := services.NewTeamsService(cfg.TeamsWebhookURL)

	// OTel / schema registry service
	mapperRegistry := ingest.NewMapperRegistry()
	schemaRegistryService := services.NewSchemaRegistryService(schemaMappingStore, mapperRegistry)
	if err := schemaRegistryService.WarmRegistry("default"); err != nil {
		log.Printf("schema-registry: warm failed: %v", err)
	}

	// Team Workflow Primitives service
	workflowService := services.NewWorkflowService(
		workflowStore, incidentStore,
		slackService, teamsService,
		jiraService, snowService,
		llmClient,
	)

	// Enterprise services
	verificationService := services.NewVerificationService(verificationStore, logStore, agentStore, incidentStore)
	policyService := services.NewPolicyService(policyStore, policyTrailStore, circuitBreakerStore)

	// Feature 5: Noise Reduction Score — first-screen login dashboard
	noiseReductionService := services.NewNoiseReductionService(eventStore, incidentStore, cfg.SREHourlyCost)
	noiseReductionHandler := handlers.NewNoiseReductionHandler(noiseReductionService)

	// SaaS Feature 4: Customer Health Score & Churn Prevention
	healthScoreStore := store.NewHealthScoreStore(db)
	healthScoreService := services.NewHealthScoreService(healthScoreStore)
	healthScoreHandler := handlers.NewHealthScoreHandler(healthScoreService)

	// SaaS Feature 5: Integration Hub / Marketplace
	// Server base URL is derived from the configured frontend origin (same host, different port)
	// or defaults to localhost. The marketplace uses it to build inbound webhook URLs.
	marketplaceStore := store.NewMarketplaceStore(db)
	marketplaceService := services.NewMarketplaceService(
		marketplaceStore,
		sourceRegistryService,
		cfg.FrontendOrigin, // used as base URL hint; handlers build proper webhook paths
	)
	marketplaceService.SetEventProcessor(correlationService)
	marketplaceHandler := handlers.NewMarketplaceHandler(marketplaceService)

	// SaaS Feature 7: Mobile Push Notifications
	pushStore := store.NewPushStore(db)
	pushService := services.NewPushService(pushStore)
	pushHandler := handlers.NewPushHandler(pushStore)
	correlationService.SetPushService(pushService)

	// Feature 3: Auto-Remediation Orchestrator — closed-loop: incident → policy → agent → verify → close
	remediationOrchestrator := services.NewRemediationOrchestrator(
		incidentStore,
		actionExecutionStore,
		agentStore,
		policyService,
		slackService,
		verificationService,
		incidentMemoryService,
		incidentService,
	)
	correlationService.SetRemediationOrchestrator(remediationOrchestrator)

	// Wire action execution store into incident detail service
	incidentDetailService.SetActionExecutionStore(actionExecutionStore)

	// NeuroOps agent ingestion service
	agentIngestionService := services.NewAgentIngestionService(agentStore, eventStore, correlationService, anomalyService, logStore)

	// Wire knowledge base into services.
	correlationService.SetKnowledgeBase(kb)
	incidentDetailService.SetKnowledgeBase(kb)
	explainService.SetKnowledgeBase(kb)
	services.SetKnowledgeBaseForActions(kb)

	// Wire Slack into correlation service (for auto-channel creation on critical incidents).
	correlationService.SetSlackService(slackService)

	// Wire engineering health into correlation + incident service for metrics recording.
	correlationService.SetEngineeringHealthService(engineeringHealthService)
	incidentService.SetEngineeringHealthService(engineeringHealthService)

	// Wire business impact service for auto-baseline on resolve.
	incidentService.SetBusinessImpactService(businessImpactService)

	// Wire incident memory service for resolution pattern learning.
	incidentService.SetIncidentMemoryService(incidentMemoryService)

	// Feature 4: wire live dollar-impact into the incident detail view.
	incidentDetailService.SetBusinessImpactService(businessImpactService)

	// Wire tenant config service into business impact for DB-driven config lookups.
	businessImpactService.SetTenantConfigService(tenantConfigService)

	// Wire onboarding into correlation service for milestone tracking.
	correlationService.SetOnboardingService(onboardingService)

	// Wire gap feature services into correlation service.
	correlationService.SetAlertFeedbackService(alertFeedbackService)
	correlationService.SetAutoResolveService(autoResolveService)
	correlationService.SetWhatsAppService(whatsappService)

	// Ensure starter plan exists for default tenant.
	if err := billingService.EnsureStarterPlan("default"); err != nil {
		log.Printf("billing: failed to ensure starter plan for default tenant: %v", err)
	}

	if cfg.SlackBotToken != "" {
		log.Printf("slack: integration configured")
	}

	// ── Handlers ──────────────────────────────────────
	mux := http.NewServeMux()

	authHandler := handlers.NewAuthHandler(userStore, cfg.JWTSecret)
	authHandler.SetTenantProvisioner(tenantStore)
	authHandler.SetDeploymentRegion(cfg.DataRegion)
	authHandler.SetDemoEventProcessor(correlationService)

	// SaaS Feature 2: region-aware middleware + handler
	dataResidencyMiddleware := middleware.NewDataResidencyMiddleware(cfg.DataRegion, tenantStore)
	regionHandler := handlers.NewRegionHandler(cfg.DataRegion, tenantStore)
	incidentHandler := handlers.NewIncidentHandler(incidentService, incidentStore)
	eventHandler := handlers.NewEventHandler(eventStore, correlationService)
	explainHandler := handlers.NewExplainHandler(incidentService, explainService)
	copilotHandler := handlers.NewCopilotHandler(incidentService, copilotService)
	activityHandler := handlers.NewActivityHandler(incidentService, actionAuditStore)
	ingestHandler := handlers.NewIngestHandler(eventStore, correlationService, sourceRegistryService, deadLetterStore, idempotencyStore)
	sourceHandler := handlers.NewSourceHandler(sourceRegistryService, ingestHandler)

	// Phase 2 handlers
	githubHandler := handlers.NewGitHubWebhookHandler(changeLinkerService, cfg.GitHubWebhookSecret)
	gitlabHandler := handlers.NewGitLabWebhookHandler(changeLinkerService, cfg.GitLabWebhookToken)
	pagerdutyHandler := handlers.NewPagerDutyWebhookHandler(eventStore, correlationService, sourceRegistryService, cfg.PagerDutyWebhookSecret)
	datadogHandler := handlers.NewDatadogWebhookHandler(eventStore, correlationService, sourceRegistryService, cfg.DatadogWebhookSecret)
	slackHandler := handlers.NewSlackHandler(slackService, incidentStore)
	slackHandler.SetRemediationOrchestrator(remediationOrchestrator)
	// A completed AI analysis re-evaluates automation with a real RCA
	// confidence — the only way an action can reach auto-execute mode.
	explainHandler.SetRemediationOrchestrator(remediationOrchestrator)
	postmortemHandler := handlers.NewPostMortemHandler(postMortemService)
	statusStore := store.NewStatusStore(db)
	statusHandler := handlers.NewStatusHandler(statusStore)

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
	// Wire correlation service so the wizard's "Send Test Alert" runs through the full pipeline.
	onboardingHandler.SetEventProcessor(correlationService)

	// Gap feature handlers
	businessImpactHandler := handlers.NewBusinessImpactHandler(businessImpactService)
	alertFeedbackHandler := handlers.NewAlertFeedbackHandler(alertFeedbackService)
	autoResolveHandler := handlers.NewAutoResolveHandler(autoResolveService)
	runbookHandler := handlers.NewRunbookHandler(runbookService, incidentDetailService)
	dependencyHandler := handlers.NewDependencyHandler(dependencyService, incidentStore)
	whatsappHandler := handlers.NewWhatsAppHandler(whatsappService, incidentStore)
	complianceHandler := handlers.NewComplianceHandler(complianceService)

	// NeuroOps agent auth — derive AES-256 key and build the HMAC authenticator.
	agentSecretKey := middleware.DeriveAgentSecretKey(cfg.JWTSecret, cfg.AgentSecretKey)
	agentAuth := middleware.NewAgentAuthenticator(agentStore, agentSecretKey, cfg.AgentIngestToken, cfg.IsProd)

	agentHandler := handlers.NewAgentHandler(agentStore, agentEnrollmentStore, agentIngestionService, agentSecretKey, cfg.AgentIngestToken)
	agentHandler.SetProdMode(cfg.IsProd)
	enrollmentHandler := handlers.NewEnrollmentHandler(agentEnrollmentStore)

	// Log Explorer handler
	logExplorerHandler := handlers.NewLogExplorerHandler(logStore, agentStore)

	// Enterprise feature handlers
	verificationHandler := handlers.NewVerificationHandler(verificationService)
	policyHandler := handlers.NewPolicyHandler(policyService)
	configHandler := handlers.NewConfigHandler(tenantConfigStore)
	auditHandler := handlers.NewAuditHandler(auditLogStore)
	actionExecutionHandler := handlers.NewActionExecutionHandler(actionExecutionStore)

	// Tenant config layer handlers
	tenantConfigHandler := handlers.NewTenantConfigHandler(
		tenantStore,
		serviceCatalogStore,
		tenantSettingsStore,
		tenantConfigService,
	)
	importHandler := handlers.NewImportHandler(importService)

	// Phase 3 category-defining handlers
	topologyHandler := handlers.NewTopologyHandler(incidentDetailService, topologyIntelligenceService)
	// Wire the causal engine: builds full DAG from incident + topology store + business impact.
	causalEngineService := services.NewCausalEngineService(incidentDetailService, topologyStore)
	causalEngineService.SetBusinessImpactService(businessImpactService)
	topologyHandler.SetCausalEngine(causalEngineService)
	incidentMemoryHandler := handlers.NewIncidentMemoryHandler(incidentMemoryService)
	riskExposureHandler := handlers.NewRiskExposureHandler(riskExposureService)
	aiStatusHandler := handlers.NewAIStatusHandler(llmClient)
	predictiveIncidentHandler := handlers.NewPredictiveIncidentHandler(predictiveIncidentService)

	// OTel-native ingest + schema registry handlers
	otelHandler := handlers.NewOTelHandler(eventStore, correlationService, schemaRegistryService, mapperRegistry, deadLetterStore, idempotencyStore)
	schemaRegistryHandler := handlers.NewSchemaRegistryHandler(schemaRegistryService)

	// Phase 4: Change Intelligence handler
	changeIntelligenceHandler := handlers.NewChangeIntelligenceHandler(changeIntelligenceService, incidentDetailService)

	// Alert Quality Governance handler
	alertQualityService := services.NewAlertQualityService(alertQualityStore, alertFeedbackStore)
	alertQualityHandler := handlers.NewAlertQualityHandler(alertQualityService)

	// Team Workflow Primitives handler
	workflowHandler := handlers.NewWorkflowHandler(workflowService, teamsService, incidentService)

	// AI Domain Memory handler (Feature 9)
	domainMemoryService := services.NewDomainMemoryService(domainMemoryStore, incidentMemoryStore, incidentStore, llmClient)
	domainMemoryHandler := handlers.NewDomainMemoryHandler(domainMemoryService)

	// ── Wire queue processor (async event processing for buffered ingest) ──
	// The processor deserializes a models.Event from the job payload and runs
	// it through the same correlation pipeline as the synchronous ingest path.
	// Jobs are enqueued when ingest is buffered (e.g. during load spikes).
	tenantQueueManager.SetProcessor(func(job tenantQueue.Job) {
		var event models.Event
		if err := json.Unmarshal(job.Payload, &event); err != nil {
			log.Printf("queue: failed to deserialize event for tenant %s: %v", job.TenantID, err)
			return
		}
		if event.TenantID == "" {
			event.TenantID = job.TenantID
		}
		correlationService.ProcessEvent(event)
	})

	// ── TenantAdminHandler: SaaS multi-tenancy management plane ─────────
	// Tenant CRUD, lifecycle transitions, plan + rate-limit overrides,
	// per-tenant usage + queue stats, encryption toggle, audit search.
	tenantAdminHandler := handlers.NewTenantAdminHandler(
		tenantStore,
		auditLogStore,
		tenantIsolation,
		tenantLimiter,
		tenantCacheManager,
		tenantQueueManager,
	)

	// ── Webhook rate limiter (100 req/min per source token) ──────────────
	ingestRateLimiter := middleware.NewWebhookRateLimiter(100, time.Minute)

	// ── Routes ────────────────────────────────────────
	routes.RegisterRoutes(mux, cfg, routes.Handlers{
		IngestRateLimiter: ingestRateLimiter,
		// SaaS multi-tenancy enforcement on every authenticated route
		Isolation:     tenantIsolation,
		TenantLimiter: tenantLimiter,
		DataResidency: dataResidencyMiddleware,
		// Core
		Health:   handlers.HealthHandler(db),
		Event:    eventHandler,
		Incident: incidentHandler,
		Explain:  explainHandler,
		Copilot:  copilotHandler,
		Activity: activityHandler,
		Ingest:   ingestHandler,
		Source:   sourceHandler,
		Auth:     authHandler,
		// Webhook ingest
		GitHub:     githubHandler,
		GitLab:     gitlabHandler,
		PagerDuty:  pagerdutyHandler,
		Datadog:    datadogHandler,
		Slack:      slackHandler,
		Postmortem: postmortemHandler,
		Status:     statusHandler,
		// OTel-native ingest + schema registry
		OTel:           otelHandler,
		SchemaRegistry: schemaRegistryHandler,
		// Analytics
		SLO:               sloHandler,
		OnCall:            oncallHandler,
		Anomaly:           anomalyHandler,
		EngineeringHealth: engineeringHealthHandler,
		ROI:               roiHandler,
		Digest:            digestHandler,
		// Billing & onboarding
		Billing:    billingHandler,
		Onboarding: onboardingHandler,
		// Gap features
		BusinessImpact: businessImpactHandler,
		AlertFeedback:  alertFeedbackHandler,
		AutoResolve:    autoResolveHandler,
		Runbook:        runbookHandler,
		Dependency:     dependencyHandler,
		WhatsApp:       whatsappHandler,
		Compliance:     complianceHandler,
		// NeuroOps agents
		Agent:       agentHandler,
		Enrollment:  enrollmentHandler,
		AgentAuth:   agentAuth,
		LogExplorer: logExplorerHandler,
		// Enterprise
		Verification:    verificationHandler,
		Policy:          policyHandler,
		Config:          configHandler,
		Audit:           auditHandler,
		ActionExecution: actionExecutionHandler,
		// Tenant config layer
		TenantConfig: tenantConfigHandler,
		Import:       importHandler,
		// Phase 3 category-defining
		Topology:       topologyHandler,
		IncidentMemory: incidentMemoryHandler,
		RiskExposure:   riskExposureHandler,
		AIStatus:       aiStatusHandler,
		// Phase 4: Change Intelligence
		ChangeIntelligence: changeIntelligenceHandler,
		// Alert Quality Governance
		AlertQuality: alertQualityHandler,
		// Team Workflow Primitives
		Workflow: workflowHandler,
		// AI Domain Memory
		DomainMemory: domainMemoryHandler,
		// SaaS multi-tenancy management plane
		TenantAdmin: tenantAdminHandler,
		// Predictive Incidents — Feature 2
		PredictiveIncident: predictiveIncidentHandler,
		// Noise Reduction Score — Feature 5
		NoiseReduction: noiseReductionHandler,
		// Multi-Region Data Residency — SaaS Feature 2
		Region: regionHandler,
		// Customer Health Score & Churn Prevention — SaaS Feature 4
		HealthScore: healthScoreHandler,
		// Integration Hub / Marketplace — SaaS Feature 5
		Marketplace: marketplaceHandler,
		// Mobile Push Notifications — SaaS Feature 7
		Push: pushHandler,
	}, gate)

	// Platform metrics endpoint (no auth — for monitoring tools)
	mux.HandleFunc("/api/v1/platform/metrics", handlers.PlatformMetricsHandler(db))

	// ── Root context — cancels all background goroutines on shutdown ──────
	rootCtx, rootCancel := context.WithCancel(context.Background())
	var bgWg sync.WaitGroup

	// ── Idempotency key cleanup (hourly) ─────────────
	bgWg.Add(1)
	go func() {
		defer bgWg.Done()
		ticker := time.NewTicker(time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-rootCtx.Done():
				return
			case <-ticker.C:
				if err := idempotencyStore.Cleanup(); err != nil {
					slog.Error("idempotency cleanup failed", "error", err)
				}
			}
		}
	}()

	// ── Agent maintenance tasks (every 5 minutes) ────
	bgWg.Add(1)
	go func() {
		defer bgWg.Done()
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-rootCtx.Done():
				return
			case <-ticker.C:
				if err := agentStore.MarkInactive(time.Now().Add(-10 * time.Minute)); err != nil {
					slog.Error("agent maintenance: mark inactive failed", "error", err)
				}
				if err := agentEnrollmentStore.DeleteExpired(); err != nil {
					slog.Error("agent maintenance: enrollment token cleanup failed", "error", err)
				}
			}
		}
	}()

	// ── Predictive incident metric series cleanup (every 30 minutes) ────
	bgWg.Add(1)
	go func() {
		defer bgWg.Done()
		ticker := time.NewTicker(30 * time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-rootCtx.Done():
				return
			case <-ticker.C:
				if err := predictiveIncidentService.PurgeOldMetrics(); err != nil {
					slog.Error("predictive: metric series purge failed", "error", err)
				}
			}
		}
	}()

	// ── HTTP Server ───────────────────────────────────
	server := &http.Server{
		Addr:              cfg.HTTPAddress(),
		Handler:           traceMiddleware.Middleware(middleware.SecurityHeaders(middleware.Recovery(mux))),
		ReadTimeout:       10 * time.Second,
		ReadHeaderTimeout: 5 * time.Second,
		WriteTimeout:      60 * time.Second, // increased for LLM calls (up to 30 s)
		IdleTimeout:       60 * time.Second,
	}

	go func() {
		slog.Info("server starting",
			"addr", cfg.HTTPAddress(),
			"frontend_origin", cfg.FrontendOrigin,
			"llm_enabled", cfg.LLMEnabled(),
		)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("http server failed", "error", err)
			os.Exit(1)
		}
	}()

	stopSignal := make(chan os.Signal, 1)
	signal.Notify(stopSignal, syscall.SIGINT, syscall.SIGTERM)
	<-stopSignal

	slog.Info("shutdown signal received — draining requests")

	// Cancel background goroutines before draining HTTP.
	rootCancel()

	// Allow 30 s for in-flight requests (LLM calls can take up to 30 s).
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		slog.Error("graceful shutdown failed", "error", err)
		os.Exit(1)
	}

	// Wait for all background goroutines to finish cleanly.
	bgWg.Wait()
	slog.Info("server stopped gracefully")
}
