// Package trace implements lightweight W3C traceparent propagation without
// requiring the OpenTelemetry SDK. It follows the same header conventions so a
// future OTel SDK swap is purely additive — no call-site changes needed.
//
// W3C Trace Context spec: https://www.w3.org/TR/trace-context/
// Header format: traceparent: 00-{traceID:32hex}-{spanID:16hex}-{flags:2hex}
package trace

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"strings"

	"ai-incident-platform/backend/internal/platform/logger"
)

// Span carries the W3C trace context for a single request hop.
type Span struct {
	TraceID string // 128-bit, 32 lowercase hex chars
	SpanID  string // 64-bit,  16 lowercase hex chars
}

type ctxKey struct{}

// FromContext returns the Span stored in ctx, or an empty Span if none.
func FromContext(ctx context.Context) Span {
	s, _ := ctx.Value(ctxKey{}).(Span)
	return s
}

// Middleware parses the incoming W3C traceparent header (if present) or
// generates a new trace. It always generates a fresh span ID (this hop).
// The span is stored in the request context and emitted in the response
// traceparent header so callers can correlate distributed traces.
//
// This middleware should sit at the outermost layer — before RequestID —
// so trace IDs are available in all subsequent log calls.
func Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		span := parse(r.Header.Get("traceparent"))
		span.SpanID = newHex(8) // fresh child span ID for this hop

		// Store in context for slog enrichment and downstream handlers.
		ctx := context.WithValue(r.Context(), ctxKey{}, span)
		ctx = logger.WithTraceID(ctx, span.TraceID)

		// Propagate to response so the caller can correlate.
		w.Header().Set("traceparent", format(span))

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// parse extracts the trace ID from a W3C traceparent header value.
// Returns a freshly generated Span on any parse failure.
func parse(header string) Span {
	// "00-{32hex}-{16hex}-{2hex}"
	if len(header) < 55 {
		return newSpan()
	}
	parts := strings.Split(header, "-")
	if len(parts) != 4 || parts[0] != "00" || len(parts[1]) != 32 {
		return newSpan()
	}
	// Validate: must be lowercase hex
	if !isHex(parts[1]) {
		return newSpan()
	}
	return Span{TraceID: parts[1]}
}

// format encodes a Span as a W3C traceparent header value (flags=01 = sampled).
func format(s Span) string {
	return "00-" + s.TraceID + "-" + s.SpanID + "-01"
}

func newSpan() Span {
	return Span{TraceID: newHex(16), SpanID: newHex(8)}
}

func newHex(bytes int) string {
	b := make([]byte, bytes)
	if _, err := rand.Read(b); err != nil {
		// Fallback: fill with counter bytes — not cryptographically random
		// but good enough for trace IDs. rand.Read failing is extremely rare.
		for i := range b {
			b[i] = byte(i)
		}
	}
	return hex.EncodeToString(b)
}

func isHex(s string) bool {
	for _, c := range s {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			return false
		}
	}
	return true
}
