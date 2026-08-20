package middleware

import (
	"fmt"
	"net/http"
	"sync/atomic"
	"time"

	"ai-incident-platform/backend/internal/platform/logger"
)

var requestCounter uint64

// RequestID injects a unique X-Request-ID into both the request and response.
// The ID is also stored in the request context so slog.InfoContext / ErrorContext
// calls downstream automatically include it via the contextHandler enrichment.
func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := buildRequestID()

		// Set on response so the caller can correlate.
		w.Header().Set("X-Request-ID", requestID)

		// Clone request to safely mutate header + context.
		cloned := r.Clone(r.Context())
		cloned.Header.Set("X-Request-ID", requestID)

		// Store in context for automatic slog enrichment.
		ctx := logger.WithRequestID(cloned.Context(), requestID)

		next.ServeHTTP(w, cloned.WithContext(ctx))
	})
}

func buildRequestID() string {
	counter := atomic.AddUint64(&requestCounter, 1)
	return fmt.Sprintf("req-%d-%d", time.Now().UnixNano(), counter)
}
