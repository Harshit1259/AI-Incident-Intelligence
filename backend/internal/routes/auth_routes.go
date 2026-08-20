package routes

import (
	"net/http"

	"ai-incident-platform/backend/internal/handlers"
)

func registerAuthRoutes(
	mux *http.ServeMux,
	withOps func(http.HandlerFunc) http.Handler,
	withAuth func(http.HandlerFunc) http.Handler,
	authHandler *handlers.AuthHandler,
) {
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

	mux.Handle("/api/v1/auth/me", withAuth(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			authHandler.Me(w, r)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	}))
}
