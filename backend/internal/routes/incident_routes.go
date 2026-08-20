package routes

import (
	"net/http"
	"strings"

	"ai-incident-platform/backend/internal/handlers"
)

func registerIncidentRoutes(
	mux *http.ServeMux,
	withAuth func(http.HandlerFunc) http.Handler,
	requireOperator func(http.HandlerFunc) http.HandlerFunc,
	eventHandler *handlers.EventHandler,
	incidentHandler *handlers.IncidentHandler,
	explainHandler *handlers.ExplainHandler,
	copilotHandler *handlers.CopilotHandler,
	activityHandler *handlers.ActivityHandler,
	sourceHandler *handlers.SourceHandler,
	postmortemHandler *handlers.PostMortemHandler,
	businessImpactHandler *handlers.BusinessImpactHandler,
	runbookHandler *handlers.RunbookHandler,
	dependencyHandler *handlers.DependencyHandler,
	verificationHandler *handlers.VerificationHandler,
	actionExecutionHandler *handlers.ActionExecutionHandler,
	changeIntelligenceHandler *handlers.ChangeIntelligenceHandler,
	workflowHandler *handlers.WorkflowHandler,
) {
	// ── Events ────────────────────────────────────────────────────────────────
	// Viewers can list; operators can create.
	mux.Handle("/api/v1/events", withAuth(methodHandler(map[string]http.HandlerFunc{
		http.MethodGet:  eventHandler.ListEvents,
		http.MethodPost: requireOperator(eventHandler.CreateEvent),
	})))

	// ── Incident list ─────────────────────────────────────────────────────────
	mux.Handle("/api/v1/incidents", withAuth(methodHandler(map[string]http.HandlerFunc{
		http.MethodGet: incidentHandler.ListIncidents,
	})))

	// ── Incident status counts (must be before the /incidents/ subtree) ───────
	mux.Handle("/api/v1/incidents/counts", withAuth(methodHandler(map[string]http.HandlerFunc{
		http.MethodGet: incidentHandler.GetStatusCounts,
	})))

	// ── Pattern-A routes: /incidents/{subresource}/{id} ───────────────────────
	// These handlers parse the incident ID from the tail of the URL, so the
	// subresource name must come first in the path.

	// AI explain — viewer readable
	mux.Handle("/api/v1/incidents/explain/", withAuth(methodHandler(map[string]http.HandlerFunc{
		http.MethodGet: explainHandler.Explain,
	})))

	// Copilot — operator+ (triggers AI calls)
	mux.Handle("/api/v1/incidents/copilot/", withAuth(methodHandler(map[string]http.HandlerFunc{
		http.MethodPost: requireOperator(copilotHandler.Ask),
	})))

	// Activity timeline — viewer readable
	mux.Handle("/api/v1/incidents/activity/", withAuth(methodHandler(map[string]http.HandlerFunc{
		http.MethodGet: activityHandler.List,
	})))

	// ── Execution lifecycle: approve / reject / rollback ──────────────────────
	// Separate domain from /incidents/; lives at /executions/{id}/{action}.
	mux.Handle("/api/v1/executions/", withAuth(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		switch {
		case strings.HasSuffix(r.URL.Path, "/approve"):
			requireOperator(actionExecutionHandler.HandleApprove)(w, r)
		case strings.HasSuffix(r.URL.Path, "/reject"):
			requireOperator(actionExecutionHandler.HandleReject)(w, r)
		case strings.HasSuffix(r.URL.Path, "/rollback"):
			requireOperator(actionExecutionHandler.HandleRollback)(w, r)
		default:
			http.Error(w, "not found", http.StatusNotFound)
		}
	}))

	// ── Incident detail & sub-resources: /incidents/{id}[/{sub}] ─────────────
	// Must be registered AFTER the more-specific pattern-A routes above so that
	// Go's ServeMux longest-prefix rule routes explain/copilot/activity correctly.
	mux.Handle("/api/v1/incidents/", withAuth(func(w http.ResponseWriter, r *http.Request) {
		_, sub, ok := incidentSubPath(r.URL.Path)
		if !ok {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}

		switch sub {
		case "":
			methodHandler(map[string]http.HandlerFunc{
				http.MethodGet: incidentHandler.GetIncidentDetail,
			})(w, r)

		case "ack", "resolve", "reopen":
			methodHandler(map[string]http.HandlerFunc{
				http.MethodPost: requireOperator(incidentHandler.UpdateIncidentStatus),
			})(w, r)

		// Explicit AI analysis. POST because it spends an LLM call and produces
		// a causal claim attributed to the caller — operator role required, so a
		// viewer cannot incur cost or author an RCA.
		case "analyze":
			methodHandler(map[string]http.HandlerFunc{
				http.MethodPost: requireOperator(explainHandler.Analyze),
			})(w, r)

		case "postmortem":
			methodHandler(map[string]http.HandlerFunc{
				http.MethodGet: postmortemHandler.Get,
				http.MethodPut: requireOperator(postmortemHandler.Update),
			})(w, r)

		case "postmortem/generate":
			methodHandler(map[string]http.HandlerFunc{
				http.MethodPost: requireOperator(postmortemHandler.Generate),
			})(w, r)

		case "business-impact":
			methodHandler(map[string]http.HandlerFunc{
				http.MethodGet: businessImpactHandler.HandleGet,
			})(w, r)

		case "business-impact/generate":
			methodHandler(map[string]http.HandlerFunc{
				http.MethodPost: requireOperator(businessImpactHandler.HandleGenerate),
			})(w, r)

		case "live-impact":
			methodHandler(map[string]http.HandlerFunc{
				http.MethodGet:  businessImpactHandler.HandleGetLiveImpact,
				http.MethodPost: businessImpactHandler.HandleGetLiveImpactFromBody,
			})(w, r)

		case "runbooks":
			methodHandler(map[string]http.HandlerFunc{
				http.MethodGet: runbookHandler.HandleIncidentRunbooks,
			})(w, r)

		case "attribution":
			methodHandler(map[string]http.HandlerFunc{
				http.MethodGet: dependencyHandler.HandleAttribution,
			})(w, r)

		case "change-intelligence":
			methodHandler(map[string]http.HandlerFunc{
				http.MethodGet: changeIntelligenceHandler.HandleIncidentReport,
			})(w, r)

		case "verify":
			methodHandler(map[string]http.HandlerFunc{
				http.MethodPost: requireOperator(verificationHandler.Verify),
			})(w, r)

		case "verify/start":
			methodHandler(map[string]http.HandlerFunc{
				http.MethodPost: requireOperator(verificationHandler.StartVerification),
			})(w, r)

		case "verify/complete":
			methodHandler(map[string]http.HandlerFunc{
				http.MethodPost: requireOperator(verificationHandler.CompleteVerification),
			})(w, r)

		case "verify/rollback":
			methodHandler(map[string]http.HandlerFunc{
				http.MethodPost: requireOperator(verificationHandler.TriggerRollback),
			})(w, r)

		case "verification":
			methodHandler(map[string]http.HandlerFunc{
				http.MethodGet: verificationHandler.GetVerification,
			})(w, r)

		case "executions":
			methodHandler(map[string]http.HandlerFunc{
				http.MethodGet:  actionExecutionHandler.HandleGetExecutions,
				http.MethodPost: requireOperator(actionExecutionHandler.HandleRecordExecution),
			})(w, r)

		default:
			// verify/{vrid}/proof — attach proof item to a verification record
			if strings.HasPrefix(sub, "verify/") && strings.HasSuffix(sub, "/proof") {
				methodHandler(map[string]http.HandlerFunc{
					http.MethodPost: requireOperator(verificationHandler.AttachProof),
				})(w, r)
				return
			}
			// workflow sub-resources: commander, tickets, exec-summary, status-comms
			if workflowHandler != nil {
				switch {
				case strings.HasPrefix(sub, "commander"):
					workflowHandler.HandleCommander(w, r)
					return
				case strings.HasPrefix(sub, "tickets"):
					workflowHandler.HandleTickets(w, r)
					return
				case strings.HasPrefix(sub, "exec-summary"):
					workflowHandler.HandleExecSummary(w, r)
					return
				case strings.HasPrefix(sub, "status-comms"):
					workflowHandler.HandleStatusComms(w, r)
					return
				}
			}
			http.Error(w, "not found", http.StatusNotFound)
		}
	}))

	// ── Sources ───────────────────────────────────────────────────────────────
	mux.Handle("/api/v1/sources", withAuth(methodHandler(map[string]http.HandlerFunc{
		http.MethodGet:  sourceHandler.ListSources,
		http.MethodPost: requireOperator(sourceHandler.CreateSource),
	})))

	mux.Handle("/api/v1/sources/health", withAuth(methodHandler(map[string]http.HandlerFunc{
		http.MethodGet: sourceHandler.ListSourceHealth,
	})))

	mux.Handle("/api/v1/sources/test", withAuth(methodHandler(map[string]http.HandlerFunc{
		http.MethodPost: requireOperator(sourceHandler.SendTestEvent),
	})))

	// ── Service discovery ─────────────────────────────────────────────────────
	mux.Handle("/api/v1/services/discovered", withAuth(methodHandler(map[string]http.HandlerFunc{
		http.MethodGet: incidentHandler.DiscoveredServices,
	})))

	// ── Action audit ──────────────────────────────────────────────────────────
	mux.Handle("/api/v1/actions/execute", withAuth(requireOperator(handlers.ExecuteActionHandler)))
	mux.Handle("/api/v1/actions/audit", withAuth(requireOperator(handlers.GetActionAuditHandler)))
}
