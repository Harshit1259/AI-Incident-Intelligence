// Package logger configures the process-wide slog logger and provides helpers
// for attaching request-scoped attributes (request_id, tenant_id, trace_id)
// to the request context so that slog.InfoContext / slog.ErrorContext calls
// automatically include them in every log line without the call site knowing.
package logger

import (
	"context"
	"log/slog"
	"os"
)

// context key types — unexported to avoid collisions with other packages.
type requestIDKey struct{}
type tenantIDKey struct{}
type traceIDKey struct{}

// Init sets the process-wide slog default handler.
// JSON format is used in production (machine-parseable, Datadog/Loki-ready).
// Text format is used elsewhere for human readability.
func Init(isProd bool) {
	opts := &slog.HandlerOptions{Level: slog.LevelInfo}
	var base slog.Handler
	if isProd {
		base = slog.NewJSONHandler(os.Stderr, opts)
	} else {
		base = slog.NewTextHandler(os.Stderr, opts)
	}
	slog.SetDefault(slog.New(&contextHandler{Handler: base}))
}

// WithRequestID returns a child context carrying the given request ID.
func WithRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, requestIDKey{}, id)
}

// WithTenantID returns a child context carrying the given tenant ID.
func WithTenantID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, tenantIDKey{}, id)
}

// WithTraceID returns a child context carrying the given trace ID.
func WithTraceID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, traceIDKey{}, id)
}

// RequestIDFromContext extracts the request ID stored by WithRequestID.
func RequestIDFromContext(ctx context.Context) string {
	v, _ := ctx.Value(requestIDKey{}).(string)
	return v
}

// TenantIDFromContext extracts the tenant ID stored by WithTenantID.
func TenantIDFromContext(ctx context.Context) string {
	v, _ := ctx.Value(tenantIDKey{}).(string)
	return v
}

// TraceIDFromContext extracts the trace ID stored by WithTraceID.
func TraceIDFromContext(ctx context.Context) string {
	v, _ := ctx.Value(traceIDKey{}).(string)
	return v
}

// contextHandler wraps a slog.Handler and injects request-scoped attributes
// from the context into every log record. Call sites use slog.InfoContext(ctx, ...)
// and these fields appear automatically — no boilerplate at the call site.
type contextHandler struct {
	slog.Handler
}

func (h *contextHandler) Handle(ctx context.Context, r slog.Record) error {
	if id, _ := ctx.Value(requestIDKey{}).(string); id != "" {
		r.AddAttrs(slog.String("request_id", id))
	}
	if tid, _ := ctx.Value(tenantIDKey{}).(string); tid != "" {
		r.AddAttrs(slog.String("tenant_id", tid))
	}
	if trid, _ := ctx.Value(traceIDKey{}).(string); trid != "" {
		r.AddAttrs(slog.String("trace_id", trid))
	}
	return h.Handler.Handle(ctx, r)
}

func (h *contextHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &contextHandler{Handler: h.Handler.WithAttrs(attrs)}
}

func (h *contextHandler) WithGroup(name string) slog.Handler {
	return &contextHandler{Handler: h.Handler.WithGroup(name)}
}
