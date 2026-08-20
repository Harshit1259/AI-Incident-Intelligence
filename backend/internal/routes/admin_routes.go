package routes

import (
	"net/http"

	"ai-incident-platform/backend/internal/handlers"
)

func registerAdminRoutes(
	mux *http.ServeMux,
	withAuth func(http.HandlerFunc) http.Handler,
	requireAdmin func(http.HandlerFunc) http.HandlerFunc,
	requireOperator func(http.HandlerFunc) http.HandlerFunc,
	tenantConfigHandler *handlers.TenantConfigHandler,
	importHandler *handlers.ImportHandler,
) {
	// Service catalog — operator+ reads; admin-only writes.
	mux.Handle("/api/v1/admin/services", withAuth(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			requireOperator(tenantConfigHandler.HandleServices)(w, r)
		} else {
			requireAdmin(tenantConfigHandler.HandleServices)(w, r)
		}
	}))
	mux.Handle("/api/v1/admin/services/", withAuth(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			requireOperator(tenantConfigHandler.HandleServiceByID)(w, r)
		} else {
			requireAdmin(tenantConfigHandler.HandleServiceByID)(w, r)
		}
	}))

	// Tenant settings — operator+ reads; admin-only writes.
	mux.Handle("/api/v1/admin/settings", withAuth(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			requireOperator(tenantConfigHandler.HandleSettings)(w, r)
		} else {
			requireAdmin(tenantConfigHandler.HandleSettings)(w, r)
		}
	}))

	// Industry templates — read-only; operator+
	mux.Handle("/api/v1/admin/industry-templates", withAuth(requireOperator(tenantConfigHandler.HandleIndustryTemplates)))

	// Onboarding config — admin-only
	mux.Handle("/api/v1/admin/onboarding/config", withAuth(requireAdmin(tenantConfigHandler.HandleOnboardingConfig)))

	// CSV bulk import — admin-only (high-impact data operations)
	mux.Handle("/api/v1/admin/import/services", withAuth(requireAdmin(importHandler.HandleImportServices)))
	mux.Handle("/api/v1/admin/import/profiles", withAuth(requireAdmin(importHandler.HandleImportProfiles)))
	mux.Handle("/api/v1/admin/import/baselines", withAuth(requireAdmin(importHandler.HandleImportBaselines)))
}
