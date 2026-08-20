package routes

import (
	"net/http"

	"ai-incident-platform/backend/internal/handlers"
	"ai-incident-platform/backend/internal/middleware"
	"ai-incident-platform/backend/internal/platform/ratelimit"
)

// Handlers bundles every HTTP handler the application exposes.
// Handlers are grouped by the product edition they belong to.
// Pass the struct to RegisterRoutes — the edition gate controls which groups
// are actually wired. A handler in a disabled edition is never called.
type Handlers struct {
	// Ingest rate limiter — applied to all webhook ingest routes.
	IngestRateLimiter *middleware.WebhookRateLimiter

	// SaaS multi-tenancy enforcement — wired into every authenticated route.
	// Isolation blocks requests from suspended or disabled tenants.
	// TenantLimiter enforces per-tenant token-bucket rate limits (plan-based).
	// DataResidency rejects cross-region requests (403) to enforce GDPR.
	Isolation     *middleware.TenantIsolation
	TenantLimiter *ratelimit.TenantRateLimiter
	DataResidency *middleware.DataResidencyMiddleware

	// ── Core ──────────────────────────────────────────────────────────────────
	// Alert ingest, incident correlation and lifecycle, JWT auth, source
	// registry, status page, billing, and onboarding.
	// Always registered — required by every other edition.
	Health   http.HandlerFunc // DB-aware readiness probe
	Event    *handlers.EventHandler
	Incident *handlers.IncidentHandler
	Explain  *handlers.ExplainHandler
	Copilot  *handlers.CopilotHandler
	Activity *handlers.ActivityHandler
	Ingest   *handlers.IngestHandler
	Source   *handlers.SourceHandler
	Auth     *handlers.AuthHandler
	// Webhook ingest sources (Core)
	GitHub     *handlers.GitHubWebhookHandler
	GitLab     *handlers.GitLabWebhookHandler
	PagerDuty  *handlers.PagerDutyWebhookHandler
	Datadog    *handlers.DatadogWebhookHandler
	Slack      *handlers.SlackHandler
	Postmortem *handlers.PostMortemHandler
	Status     *handlers.StatusHandler
	// OTel-native ingest + schema registry (Core)
	OTel           *handlers.OTelHandler
	SchemaRegistry *handlers.SchemaRegistryHandler
	// Billing & onboarding (Core — SaaS backbone)
	Billing    *handlers.BillingHandler
	Onboarding *handlers.OnboardingHandler

	// ── Enterprise ────────────────────────────────────────────────────────────
	// Policy engine, immutable audit log, compliance, tenant config, admin tools.
	// Active when PLATFORM_EDITIONS contains "enterprise".
	Policy       *handlers.PolicyHandler
	Config       *handlers.ConfigHandler
	Audit        *handlers.AuditHandler
	TenantConfig *handlers.TenantConfigHandler
	Import       *handlers.ImportHandler

	// ── Agent Automation ──────────────────────────────────────────────────────
	// NeuroOps agent pipeline, plugin execution, log explorer, action execution.
	// Active when PLATFORM_EDITIONS contains "agent".
	Agent           *handlers.AgentHandler
	Enrollment      *handlers.EnrollmentHandler
	AgentAuth       *middleware.AgentAuthenticator
	LogExplorer     *handlers.LogExplorerHandler
	ActionExecution *handlers.ActionExecutionHandler
	Verification    *handlers.VerificationHandler

	// ── SaaS Ops Add-on ───────────────────────────────────────────────────────
	// Analytics, AI enrichment, risk, runbooks, and all intelligence features.
	// Active when PLATFORM_EDITIONS contains "saasops".
	SLO               *handlers.SLOHandler
	OnCall            *handlers.OnCallHandler
	Anomaly           *handlers.AnomalyHandler
	EngineeringHealth *handlers.EngineeringHealthHandler
	ROI               *handlers.ROIHandler
	Digest            *handlers.DigestHandler
	BusinessImpact    *handlers.BusinessImpactHandler
	AlertFeedback     *handlers.AlertFeedbackHandler
	AutoResolve       *handlers.AutoResolveHandler
	Runbook           *handlers.RunbookHandler
	Dependency        *handlers.DependencyHandler
	WhatsApp          *handlers.WhatsAppHandler
	Compliance        *handlers.ComplianceHandler
	Topology          *handlers.TopologyHandler
	IncidentMemory    *handlers.IncidentMemoryHandler
	RiskExposure      *handlers.RiskExposureHandler
	AIStatus          *handlers.AIStatusHandler
	ChangeIntelligence *handlers.ChangeIntelligenceHandler
	AlertQuality       *handlers.AlertQualityHandler

	// Team Workflow Primitives — Feature 8
	Workflow *handlers.WorkflowHandler

	// AI Domain Memory — Feature 9
	DomainMemory *handlers.DomainMemoryHandler

	// SaaS Multi-Tenancy — Feature B1
	TenantAdmin *handlers.TenantAdminHandler

	// Predictive Incidents — Feature 2
	PredictiveIncident *handlers.PredictiveIncidentHandler

	// Noise Reduction Score — Feature 5
	NoiseReduction *handlers.NoiseReductionHandler

	// Multi-Region Data Residency — SaaS Feature 2
	Region *handlers.RegionHandler

	// Customer Health Score & Churn Prevention — SaaS Feature 4
	HealthScore *handlers.HealthScoreHandler

	// Integration Hub / Marketplace — SaaS Feature 5
	Marketplace *handlers.MarketplaceHandler

	// Mobile Push Notifications — SaaS Feature 7
	Push *handlers.PushHandler
}
