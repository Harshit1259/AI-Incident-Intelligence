package routes

import (
	"net/http"

	"ai-incident-platform/backend/internal/config"
	"ai-incident-platform/backend/internal/handlers"
	"ai-incident-platform/backend/internal/middleware"
	"ai-incident-platform/backend/internal/models"
	"ai-incident-platform/backend/internal/platform/edition"
	"ai-incident-platform/backend/internal/platform/ratelimit"
)

// EnableCORS wraps a handler with CORS headers for the given origin.
func EnableCORS(frontendOrigin string, handler http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		allowedOrigin := frontendOrigin
		if allowedOrigin == "" {
			allowedOrigin = "*"
		}

		w.Header().Set("Access-Control-Allow-Origin", allowedOrigin)
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Source-Token, X-Agent-Token, X-Enrollment-Token, X-Agent-ID, X-Timestamp, X-Nonce, X-Signature, X-Idempotency-Key, X-Datadog-Webhook-Token, X-Hub-Signature-256, X-Gitlab-Token, X-PagerDuty-Signature")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		handler(w, r)
	}
}

// RegisterRoutes wires HTTP routes into mux according to the active edition gate.
// Core routes are always registered. Each other module group is registered only
// when gate.Has(edition.<X>) returns true.
func RegisterRoutes(mux *http.ServeMux, cfg config.Config, h Handlers, gate edition.Gate) {
	withCORS := func(handler http.HandlerFunc) http.HandlerFunc {
		return EnableCORS(cfg.FrontendOrigin, handler)
	}

	withOps := func(handler http.HandlerFunc) http.Handler {
		return middleware.RequestID(middleware.RequestLogger(withCORS(handler)))
	}

	withAuth := func(handler http.HandlerFunc) http.Handler {
		// Apply tenant-aware middleware inside-out (outermost wrapper = last to execute):
		//   1. RequireAuth      — validates JWT, sets claims in context
		//   2. Isolation        — blocks suspended / disabled tenants (reads claims)
		//   3. DataResidency    — rejects cross-region requests (GDPR enforcement)
		//   4. TenantLimiter    — per-tenant query token-bucket rate limit (reads claims)
		//   5. MutationLimiter  — per-tenant mutation rate limit (POST/PUT/PATCH/DELETE only)
		//   6. handler          — actual business logic
		final := handler
		if h.TenantLimiter != nil {
			final = middleware.MutationRateLimit(h.TenantLimiter)(final)
			final = middleware.TenantRateLimit(h.TenantLimiter, ratelimit.CategoryQuery)(final)
		}
		if h.DataResidency != nil {
			final = h.DataResidency.Enforce(final)
		}
		if h.Isolation != nil {
			final = h.Isolation.Enforce(final)
		}
		return middleware.RequestID(middleware.RequestLogger(
			withCORS(middleware.RequireAuth(cfg.JWTSecret, final)),
		))
	}

	requireAdmin := middleware.RequireMinRole(models.RoleAdmin)
	requireOperator := middleware.RequireMinRole(models.RoleOperator)

	// ── Core — always active ──────────────────────────────────────────────────
	// Health and config schema are public endpoints required before credentials exist.
	mux.Handle("/api/v1/health", withOps(h.Health))
	mux.Handle("/api/v1/config/schema", withOps(handlers.HandleConfigSchema))

	// Agent install script — public, no auth required.
	// curl -fsSL http://<host>/install.sh | TENANT_ID=<id> sh
	mux.Handle("/install.sh", withOps(handlers.HandleInstallScript))

	registerAuthRoutes(mux, withOps, withAuth, h.Auth)

	registerIngestRoutes(mux, withOps, withAuth, h.IngestRateLimiter,
		h.Ingest,
		h.GitHub, h.GitLab, h.PagerDuty, h.Datadog,
		h.Slack,
		h.Status,
		h.ChangeIntelligence,
		h.OTel,
		h.SchemaRegistry,
	)

	registerIncidentRoutes(mux, withAuth, requireOperator,
		h.Event, h.Incident, h.Explain, h.Copilot, h.Activity,
		h.Source,
		h.Postmortem,
		h.BusinessImpact, h.Runbook, h.Dependency,
		h.Verification, h.ActionExecution,
		h.ChangeIntelligence,
		h.Workflow,
	)

	registerBillingRoutes(mux, withAuth, requireOperator, h.Billing, h.Onboarding)

	// ── Mobile Push Notifications — always active ─────────────────────────────
	if h.Push != nil {
		registerPushRoutes(mux, withAuth, h.Push)
	}

	// ── Alert Quality Governance — always active (core analytics) ─────────────
	registerAlertQualityRoutes(mux, withAuth, h.AlertQuality)

	// ── Noise Reduction Score — first-screen dashboard ────────────────────────
	if h.NoiseReduction != nil {
		mux.Handle("/api/v1/noise-reduction", withAuth(methodHandler(map[string]http.HandlerFunc{
			http.MethodGet: h.NoiseReduction.HandleGet,
		})))
	}

	// ── Multi-Region Data Residency — SaaS Feature 2 ─────────────────────────
	// Public: login page uses this to show "Data processed in EU (Frankfurt)".
	// Auth:   UI badge "Your data is stored in EU (Frankfurt)".
	if h.Region != nil {
		mux.Handle("/api/v1/region", withOps(h.Region.HandleDeploymentRegion))
		mux.Handle("/api/v1/region/tenant", withAuth(h.Region.HandleTenantRegion))
	}

	// ── Team Workflow Primitives — always active ───────────────────────────────
	registerWorkflowRoutes(mux, withAuth, withOps, h.Workflow)

	// ── AI Domain Memory — always active ──────────────────────────────────────
	registerDomainMemoryRoutes(mux, withAuth, h.DomainMemory)

	// ── SaaS Multi-Tenancy management plane — always active ───────────────────
	registerTenantAdminRoutes(mux, withAuth, h.TenantAdmin)

	// ── Customer Health Score & Churn Prevention — SaaS Feature 4 ────────────
	if h.HealthScore != nil {
		registerHealthScoreRoutes(mux, withAuth, h.HealthScore)
	}

	// ── Integration Hub / Marketplace — SaaS Feature 5 ───────────────────────
	if h.Marketplace != nil {
		registerMarketplaceRoutes(mux, withAuth, h.Marketplace)
	}

	// ── Enterprise ────────────────────────────────────────────────────────────
	if gate.Has(edition.Enterprise) {
		registerEnterpriseRoutes(mux, withAuth, requireAdmin, requireOperator,
			h.Policy, h.Config, h.Audit,
		)
		registerAdminRoutes(mux, withAuth, requireAdmin, requireOperator, h.TenantConfig, h.Import)
	}

	// ── Agent Automation ──────────────────────────────────────────────────────
	if gate.Has(edition.AgentAutomation) {
		registerAgentRoutes(mux, withOps, withAuth, requireOperator,
			h.Agent, h.Enrollment, h.LogExplorer, h.AgentAuth,
		)
	}

	// ── SaaS Ops Add-on ───────────────────────────────────────────────────────
	if gate.Has(edition.SaaSOpsAddOn) {
		registerAnalyticsRoutes(mux, withAuth, requireOperator,
			h.SLO, h.OnCall, h.Anomaly,
			h.EngineeringHealth, h.ROI, h.Digest,
		)
		registerGapRoutes(mux, withAuth, requireOperator,
			h.BusinessImpact, h.AlertFeedback, h.AutoResolve,
			h.Runbook, h.Dependency, h.WhatsApp, h.Compliance,
		)
		registerP3Routes(mux, withAuth, requireOperator,
			h.Topology, h.IncidentMemory, h.RiskExposure, h.AIStatus,
		)
		if h.PredictiveIncident != nil {
			registerPredictiveRoutes(mux, withAuth, requireOperator, h.PredictiveIncident)
		}
	}
}
