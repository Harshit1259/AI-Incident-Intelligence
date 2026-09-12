package routes

import (
	"net/http"

	"ai-incident-platform/backend/internal/handlers"
)

// registerAlertMuteRoutes wires /api/v1/mutes. Seeing what is muted is open
// to every signed-in user; creating, previewing and ending mutes is admin-only.
func registerAlertMuteRoutes(
	mux *http.ServeMux,
	withAuth func(http.HandlerFunc) http.Handler,
	requireAdmin func(http.HandlerFunc) http.HandlerFunc,
	h *handlers.AlertMuteHandler,
) {
	mux.Handle("/api/v1/mutes", withAuth(methodHandler(map[string]http.HandlerFunc{
		http.MethodGet:  h.HandleList,
		http.MethodPost: requireAdmin(h.HandleCreate),
	})))
	mux.Handle("/api/v1/mutes/log", withAuth(methodHandler(map[string]http.HandlerFunc{
		http.MethodGet: h.HandleLog,
	})))
	mux.Handle("/api/v1/mutes/preview", withAuth(methodHandler(map[string]http.HandlerFunc{
		http.MethodPost: requireAdmin(h.HandlePreview),
	})))
	mux.Handle("/api/v1/mutes/suggest", withAuth(methodHandler(map[string]http.HandlerFunc{
		http.MethodGet: requireAdmin(h.HandleSuggest),
	})))
	// /api/v1/mutes/{id}/unmute
	mux.Handle("/api/v1/mutes/", withAuth(requireAdmin(h.HandleUnmute)))
}
