package middleware

import (
	"log/slog"
	"net/http"
	"runtime/debug"

	"ai-incident-platform/backend/internal/api"
)

// Recovery catches panics, logs the stack trace as a structured error, and
// returns a 500 JSON response. It prevents the server from crashing on bugs.
func Recovery(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				slog.ErrorContext(r.Context(), "panic recovered",
					"panic", rec,
					"stack", string(debug.Stack()),
				)
				api.WriteErrorCode(w, http.StatusInternalServerError,
					"internal server error", api.ErrCodeInternal)
			}
		}()
		next.ServeHTTP(w, r)
	})
}
