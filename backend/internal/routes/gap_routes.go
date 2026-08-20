package routes

import (
	"net/http"

	"ai-incident-platform/backend/internal/handlers"
)

func registerGapRoutes(
	mux *http.ServeMux,
	withAuth func(http.HandlerFunc) http.Handler,
	requireOperator func(http.HandlerFunc) http.HandlerFunc,
	businessImpactHandler *handlers.BusinessImpactHandler,
	alertFeedbackHandler *handlers.AlertFeedbackHandler,
	autoResolveHandler *handlers.AutoResolveHandler,
	runbookHandler *handlers.RunbookHandler,
	dependencyHandler *handlers.DependencyHandler,
	whatsappHandler *handlers.WhatsAppHandler,
	complianceHandler *handlers.ComplianceHandler,
) {
	// Business impact profiles — operator writes, no public reads here
	mux.Handle("/api/v1/business/profiles/", withAuth(requireOperator(businessImpactHandler.HandleDeleteProfile)))
	mux.Handle("/api/v1/business/profiles", withAuth(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			businessImpactHandler.HandleProfiles(w, r)
		} else {
			requireOperator(businessImpactHandler.HandleProfiles)(w, r)
		}
	}))
	mux.Handle("/api/v1/business/baselines", withAuth(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			businessImpactHandler.HandleBaselines(w, r)
		} else {
			requireOperator(businessImpactHandler.HandleBaselines)(w, r)
		}
	}))
	mux.Handle("/api/v1/business/report/monthly", withAuth(businessImpactHandler.HandleMonthlyReport))

	// Alert feedback — operator+ (affects suppression / quality scoring)
	mux.Handle("/api/v1/alerts/feedback/source-quality", withAuth(requireOperator(alertFeedbackHandler.HandleSourceQuality)))
	mux.Handle("/api/v1/alerts/feedback/stats", withAuth(alertFeedbackHandler.HandleStats)) // read — viewer ok
	mux.Handle("/api/v1/alerts/feedback", withAuth(requireOperator(alertFeedbackHandler.HandleSubmit)))
	mux.Handle("/api/v1/alerts/suppressed", withAuth(requireOperator(alertFeedbackHandler.HandleSuppressed)))

	// Auto-resolve rules — operator writes
	mux.Handle("/api/v1/auto-resolve/rules/", withAuth(requireOperator(autoResolveHandler.HandleRuleByID)))
	mux.Handle("/api/v1/auto-resolve/rules", withAuth(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			autoResolveHandler.HandleRules(w, r)
		} else {
			requireOperator(autoResolveHandler.HandleRules)(w, r)
		}
	}))

	// Runbooks — viewer reads, operator writes
	mux.Handle("/api/v1/runbooks/", withAuth(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			runbookHandler.HandleRunbookByID(w, r)
		} else {
			requireOperator(runbookHandler.HandleRunbookByID)(w, r)
		}
	}))
	mux.Handle("/api/v1/runbooks", withAuth(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			runbookHandler.HandleRunbooks(w, r)
		} else {
			requireOperator(runbookHandler.HandleRunbooks)(w, r)
		}
	}))

	// Dependency catalog — viewer reads, operator writes
	mux.Handle("/api/v1/dependencies/", withAuth(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			dependencyHandler.HandleDependencyByID(w, r)
		} else {
			requireOperator(dependencyHandler.HandleDependencyByID)(w, r)
		}
	}))
	mux.Handle("/api/v1/dependencies", withAuth(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			dependencyHandler.HandleDependencies(w, r)
		} else {
			requireOperator(dependencyHandler.HandleDependencies)(w, r)
		}
	}))

	// WhatsApp config — integration config is operator+
	mux.Handle("/api/v1/whatsapp/config", withAuth(requireOperator(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			whatsappHandler.HandleGetConfig(w, r)
		case http.MethodPut:
			whatsappHandler.HandleSaveConfig(w, r)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})))
	mux.Handle("/api/v1/whatsapp/test", withAuth(requireOperator(whatsappHandler.HandleTest)))

	// Compliance — viewer can read reports (useful for auditors / stakeholders)
	mux.Handle("/api/v1/compliance/report", withAuth(complianceHandler.HandleReport))
	mux.Handle("/api/v1/compliance/export", withAuth(requireOperator(complianceHandler.HandleExport)))
}
