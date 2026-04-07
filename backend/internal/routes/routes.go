package routes

import (
	"net/http"
	"strings"

	"ai-incident-platform/backend/internal/config"
	"ai-incident-platform/backend/internal/handlers"
	"ai-incident-platform/backend/internal/middleware"
)

func EnableCORS(frontendOrigin string, handler http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		allowedOrigin := frontendOrigin
		if allowedOrigin == "" {
			allowedOrigin = "*"
		}

		w.Header().Set("Access-Control-Allow-Origin", allowedOrigin)
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Source-Token")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		handler(w, r)
	}
}

func RegisterRoutes(
	mux *http.ServeMux,
	cfg config.Config,
	eventHandler *handlers.EventHandler,
	incidentHandler *handlers.IncidentHandler,
	explainHandler *handlers.ExplainHandler,
	copilotHandler *handlers.CopilotHandler,
	activityHandler *handlers.ActivityHandler,
	demoHandler *handlers.DemoHandler,
	devHandler *handlers.DevHandler,
	ingestHandler *handlers.IngestHandler,
	sourceHandler *handlers.SourceHandler,
	authHandler *handlers.AuthHandler,
	// Phase 2 handlers
	githubHandler *handlers.GitHubWebhookHandler,
	gitlabHandler *handlers.GitLabWebhookHandler,
	pagerdutyHandler *handlers.PagerDutyWebhookHandler,
	datadogHandler *handlers.DatadogWebhookHandler,
	slackHandler *handlers.SlackHandler,
	postmortemHandler *handlers.PostMortemHandler,
	statusHandler *handlers.StatusHandler,
	// Phase 3 handlers
	sloHandler *handlers.SLOHandler,
	oncallHandler *handlers.OnCallHandler,
	anomalyHandler *handlers.AnomalyHandler,
	engineeringHealthHandler *handlers.EngineeringHealthHandler,
	roiHandler *handlers.ROIHandler,
	digestHandler *handlers.DigestHandler,
	// Phase 4 handlers
	billingHandler *handlers.BillingHandler,
	onboardingHandler *handlers.OnboardingHandler,
) {
	withCORS := func(handler http.HandlerFunc) http.HandlerFunc {
		return EnableCORS(cfg.FrontendOrigin, handler)
	}

	withOps := func(handler http.HandlerFunc) http.Handler {
		return middleware.RequestID(middleware.RequestLogger(withCORS(handler)))
	}

	// withAuth wraps a handler with CORS + ops middleware + JWT validation.
	// Routes registered with withAuth require a valid Bearer token.
	withAuth := func(handler http.HandlerFunc) http.Handler {
		return middleware.RequestID(middleware.RequestLogger(
			withCORS(middleware.RequireAuth(cfg.JWTSecret, handler)),
		))
	}

	// ── Public routes ────────────────────────────────────────────────────────

	// Health
	mux.Handle("/api/v1/health", withOps(handlers.HealthHandler))

	// Auth — no JWT required
	mux.Handle("/api/v1/auth/login", withOps(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			authHandler.Login(w, r)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	}))

	mux.Handle("/api/v1/auth/register", withOps(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			authHandler.Register(w, r)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	}))

	// Auth — requires JWT
	mux.Handle("/api/v1/auth/me", withAuth(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			authHandler.Me(w, r)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	}))

	// ── Ingest — public (called by external monitoring systems) ──────────────

	mux.Handle("/api/v1/ingest/webhook", withOps(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			ingestHandler.GenericWebhook(w, r)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	}))

	mux.Handle("/api/v1/ingest/prometheus", withOps(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			ingestHandler.PrometheusWebhook(w, r)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	}))

	// Phase 2: ingest routes for external services.
	mux.Handle("/api/v1/ingest/github", withOps(githubHandler.Handle))
	mux.Handle("/api/v1/ingest/gitlab", withOps(gitlabHandler.Handle))
	mux.Handle("/api/v1/ingest/pagerduty", withOps(pagerdutyHandler.Handle))
	mux.Handle("/api/v1/ingest/datadog", withOps(datadogHandler.Handle))

	// ── Slack routes (verified by signing secret, not JWT) ───────────────────
	mux.Handle("/api/v1/slack/command", withOps(slackHandler.HandleSlashCommand))
	mux.Handle("/api/v1/slack/interaction", withOps(slackHandler.HandleInteraction))

	// ── Status page — public, no auth ────────────────────────────────────────
	mux.Handle("/api/v1/status", withOps(statusHandler.GetStatus))
	mux.Handle("/api/v1/status/", withOps(statusHandler.GetStatus))

	// ── Protected API routes — require valid JWT ──────────────────────────────

	mux.Handle("/api/v1/events", withAuth(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			eventHandler.ListEvents(w, r)
		case http.MethodPost:
			eventHandler.CreateEvent(w, r)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	}))

	mux.Handle("/api/v1/incidents", withAuth(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			incidentHandler.ListIncidents(w, r)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	}))

	mux.Handle("/api/v1/incidents/explain/", withAuth(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			explainHandler.Explain(w, r)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	}))

	mux.Handle("/api/v1/incidents/copilot/", withAuth(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			copilotHandler.Ask(w, r)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	}))

	mux.Handle("/api/v1/incidents/activity/", withAuth(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			activityHandler.List(w, r)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	}))

	// Post-mortem routes — must come BEFORE the generic /api/v1/incidents/ catch-all.
	mux.Handle("/api/v1/incidents/postmortem/", withAuth(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		switch {
		case strings.HasSuffix(path, "/generate") && r.Method == http.MethodPost:
			postmortemHandler.Generate(w, r)
		case strings.HasSuffix(path, "/postmortem") && r.Method == http.MethodGet:
			postmortemHandler.Get(w, r)
		case strings.HasSuffix(path, "/postmortem") && r.Method == http.MethodPut:
			postmortemHandler.Update(w, r)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	}))

	mux.Handle("/api/v1/incidents/", withAuth(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path

		isActionRoute := strings.HasSuffix(path, "/ack") ||
			strings.HasSuffix(path, "/resolve") ||
			strings.HasSuffix(path, "/reopen")

		isPostmortemRoute := strings.HasSuffix(path, "/postmortem") ||
			strings.HasSuffix(path, "/postmortem/generate")

		switch {
		case isPostmortemRoute && r.Method == http.MethodGet:
			postmortemHandler.Get(w, r)
		case isPostmortemRoute && r.Method == http.MethodPut:
			postmortemHandler.Update(w, r)
		case strings.HasSuffix(path, "/postmortem/generate") && r.Method == http.MethodPost:
			postmortemHandler.Generate(w, r)
		case isActionRoute && r.Method == http.MethodPost:
			incidentHandler.UpdateIncidentStatus(w, r)
		case !isActionRoute && !isPostmortemRoute && r.Method == http.MethodGet:
			incidentHandler.GetIncidentDetail(w, r)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	}))

	mux.Handle("/api/v1/actions/execute", withAuth(handlers.ExecuteActionHandler))
	mux.Handle("/api/v1/actions/audit", withAuth(handlers.GetActionAuditHandler))

	mux.Handle("/api/v1/sources", withAuth(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			sourceHandler.ListSources(w, r)
		case http.MethodPost:
			sourceHandler.CreateSource(w, r)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	}))

	mux.Handle("/api/v1/sources/health", withAuth(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			sourceHandler.ListSourceHealth(w, r)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	}))

	mux.Handle("/api/v1/sources/test", withAuth(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			sourceHandler.SendTestEvent(w, r)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	}))

	// ── Phase 3: SLO routes ─────────────────────────────────────────────────
	mux.Handle("/api/v1/slos", withAuth(sloHandler.HandleSLOs))
	mux.Handle("/api/v1/slos/", withAuth(sloHandler.HandleSLOByID))

	// ── Phase 3: On-Call routes ──────────────────────────────────────────────
	mux.Handle("/api/v1/oncall", withAuth(oncallHandler.HandleOnCall))
	mux.Handle("/api/v1/oncall/", withAuth(oncallHandler.HandleOnCallByID))

	// ── Phase 3: Anomaly routes ──────────────────────────────────────────────
	mux.Handle("/api/v1/anomalies", withAuth(anomalyHandler.HandleAnomalies))
	mux.Handle("/api/v1/anomalies/check", withAuth(anomalyHandler.HandleCheckMetric))
	mux.Handle("/api/v1/anomalies/simulate", withAuth(anomalyHandler.HandleSimulate))
	mux.Handle("/api/v1/anomalies/", withAuth(anomalyHandler.HandleAckAlert))

	// ── Phase 3: Engineering Health route ────────────────────────────────────
	mux.Handle("/api/v1/engineering/health", withAuth(engineeringHealthHandler.HandleHealth))

	// ── Phase 3: ROI route ───────────────────────────────────────────────────
	mux.Handle("/api/v1/roi", withAuth(roiHandler.HandleROI))

	// ── Phase 3: Digest routes ───────────────────────────────────────────────
	mux.Handle("/api/v1/digest/weekly", withAuth(digestHandler.HandleWeeklyDigest))
	mux.Handle("/api/v1/digest/send", withAuth(digestHandler.HandleSendDigest))

	// ── Phase 4: Billing routes ─────────────────────────────────────────────
	mux.Handle("/api/v1/billing/plans", withAuth(billingHandler.HandlePlans))
	mux.Handle("/api/v1/billing/subscription", withAuth(billingHandler.HandleSubscription))
	mux.Handle("/api/v1/billing/checkout", withAuth(billingHandler.HandleCheckout))
	mux.Handle("/api/v1/billing/usage", withAuth(billingHandler.HandleUsage))

	// ── Phase 4: Onboarding routes ──────────────────────────────────────────
	mux.Handle("/api/v1/onboarding/progress", withAuth(onboardingHandler.HandleProgress))
	mux.Handle("/api/v1/onboarding/step", withAuth(onboardingHandler.HandleStep))
	mux.Handle("/api/v1/onboarding/milestone", withAuth(onboardingHandler.HandleMilestone))

	// ── Demo / Dev — keep public for quick testing ────────────────────────────
	mux.Handle("/api/v1/demo/scenario", withOps(demoHandler.RunScenario))
	mux.Handle("/api/v1/dev/reset", withOps(devHandler.Reset))
}
