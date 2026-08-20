package routes

import (
	"net/http"

	"ai-incident-platform/backend/internal/handlers"
)

func registerPushRoutes(
	mux *http.ServeMux,
	withAuth func(http.HandlerFunc) http.Handler,
	h *handlers.PushHandler,
) {
	mux.Handle("/api/v1/push/register", withAuth(h.Handle))
	mux.Handle("/api/v1/push/register/", withAuth(h.Handle))
}
