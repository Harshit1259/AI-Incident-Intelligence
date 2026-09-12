package routes

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"ai-incident-platform/backend/internal/handlers"
	"ai-incident-platform/backend/internal/middleware"
	"ai-incident-platform/backend/internal/models"
)

// Only admins may create, preview, suggest or end mutes. The handler is never
// reached for other roles, so a nil service is enough to prove the wiring.
func TestAlertMuteWritesAreAdminOnly(t *testing.T) {
	mux := http.NewServeMux()
	passThrough := func(h http.HandlerFunc) http.Handler { return h }
	registerAlertMuteRoutes(mux, passThrough, middleware.RequireMinRole(models.RoleAdmin),
		handlers.NewAlertMuteHandler(nil))

	writes := []struct{ method, path string }{
		{http.MethodPost, "/api/v1/mutes"},
		{http.MethodPost, "/api/v1/mutes/preview"},
		{http.MethodGet, "/api/v1/mutes/suggest?event_id=evt-1"},
		{http.MethodPost, "/api/v1/mutes/mute-1/unmute"},
	}
	for _, role := range []string{models.RoleOperator, models.RoleViewer} {
		for _, w := range writes {
			t.Run(role+" "+w.method+" "+w.path, func(t *testing.T) {
				req := httptest.NewRequest(w.method, w.path, nil)
				req = req.WithContext(context.WithValue(req.Context(), models.ClaimsContextKey,
					models.TokenClaims{UserID: "u-1", TenantID: "acme", Role: role}))
				rec := httptest.NewRecorder()
				mux.ServeHTTP(rec, req)
				if rec.Code != http.StatusForbidden {
					t.Errorf("status = %d, want 403", rec.Code)
				}
			})
		}
	}
}
