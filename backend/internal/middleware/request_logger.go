package middleware

import (
	"log/slog"
	"net/http"
	"time"

	"ai-incident-platform/backend/internal/platform/trace"
)

type statusRecorder struct {
	http.ResponseWriter
	statusCode int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.statusCode = code
	r.ResponseWriter.WriteHeader(code)
}

// RequestLogger logs every completed request as a structured slog record.
// It records method, path, status, duration, request_id, tenant_id, and
// trace_id — the latter three are auto-enriched by the contextHandler
// when they are present in the request context.
func RequestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		rec := &statusRecorder{
			ResponseWriter: w,
			statusCode:     http.StatusOK,
		}

		next.ServeHTTP(rec, r)

		span := trace.FromContext(r.Context())

		slog.InfoContext(r.Context(), "request",
			"method", r.Method,
			"path", r.URL.RequestURI(),
			"status", rec.statusCode,
			"duration_ms", time.Since(start).Milliseconds(),
			"span_id", span.SpanID,
		)
	})
}
