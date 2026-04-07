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
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-Source-Token")

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
) {
	withCORS := func(handler http.HandlerFunc) http.HandlerFunc {
		return EnableCORS(cfg.FrontendOrigin, handler)
	}

	withOpsMiddleware := func(handler http.HandlerFunc) http.Handler {
		return middleware.RequestID(middleware.RequestLogger(withCORS(handler)))
	}

	mux.Handle("/api/v1/health", withOpsMiddleware(handlers.HealthHandler))

	mux.Handle("/api/v1/events", withOpsMiddleware(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			eventHandler.ListEvents(w, r)
		case http.MethodPost:
			eventHandler.CreateEvent(w, r)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	}))

	mux.Handle("/api/v1/incidents", withOpsMiddleware(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			incidentHandler.ListIncidents(w, r)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	}))

	mux.Handle("/api/v1/incidents/explain/", withOpsMiddleware(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			explainHandler.Explain(w, r)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	}))

	mux.Handle("/api/v1/incidents/copilot/", withOpsMiddleware(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			copilotHandler.Ask(w, r)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	}))

	mux.Handle("/api/v1/incidents/activity/", withOpsMiddleware(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			activityHandler.List(w, r)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	}))

	mux.Handle("/api/v1/incidents/", withOpsMiddleware(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		isActionRoute := strings.HasSuffix(path, "/ack") ||
			strings.HasSuffix(path, "/resolve") ||
			strings.HasSuffix(path, "/reopen")

		switch {
		case isActionRoute && r.Method == http.MethodPost:
			incidentHandler.UpdateIncidentStatus(w, r)
		case !isActionRoute && r.Method == http.MethodGet:
			incidentHandler.GetIncidentDetail(w, r)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	}))

	mux.Handle("/api/v1/actions/execute", withOpsMiddleware(handlers.ExecuteActionHandler))
	mux.Handle("/api/v1/actions/audit", withOpsMiddleware(handlers.GetActionAuditHandler))
	mux.Handle("/api/v1/demo/scenario", withOpsMiddleware(demoHandler.RunScenario))
	mux.Handle("/api/v1/dev/reset", withOpsMiddleware(devHandler.Reset))

	mux.Handle("/api/v1/ingest/webhook", withOpsMiddleware(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			ingestHandler.GenericWebhook(w, r)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	}))

	mux.Handle("/api/v1/ingest/prometheus", withOpsMiddleware(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			ingestHandler.PrometheusWebhook(w, r)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	}))

	mux.Handle("/api/v1/sources", withOpsMiddleware(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			sourceHandler.ListSources(w, r)
		case http.MethodPost:
			sourceHandler.CreateSource(w, r)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	}))

	mux.Handle("/api/v1/sources/health", withOpsMiddleware(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			sourceHandler.ListSourceHealth(w, r)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	}))

	mux.Handle("/api/v1/sources/test", withOpsMiddleware(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			sourceHandler.SendTestEvent(w, r)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	}))
}
