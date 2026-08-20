package trace_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"ai-incident-platform/backend/internal/platform/trace"
)

func TestMiddleware_SetsTraceparentOnResponse(t *testing.T) {
	handler := trace.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/", nil))

	tp := rr.Header().Get("traceparent")
	if tp == "" {
		t.Fatal("traceparent response header not set")
	}
	// Format: 00-{32hex}-{16hex}-01
	parts := strings.Split(tp, "-")
	if len(parts) != 4 {
		t.Fatalf("traceparent has %d parts, want 4: %q", len(parts), tp)
	}
	if parts[0] != "00" {
		t.Errorf("version: got %q, want 00", parts[0])
	}
	if len(parts[1]) != 32 {
		t.Errorf("trace_id length: got %d, want 32", len(parts[1]))
	}
	if len(parts[2]) != 16 {
		t.Errorf("span_id length: got %d, want 16", len(parts[2]))
	}
	if parts[3] != "01" {
		t.Errorf("flags: got %q, want 01", parts[3])
	}
}

func TestMiddleware_PropagatesIncomingTraceID(t *testing.T) {
	const traceID = "4bf92f3577b34da6a3ce929d0e0e4736"
	incomingTraceparent := "00-" + traceID + "-00f067aa0ba902b7-01"

	var capturedSpan trace.Span

	handler := trace.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedSpan = trace.FromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("traceparent", incomingTraceparent)

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, r)

	if capturedSpan.TraceID != traceID {
		t.Errorf("TraceID: got %q, want %q", capturedSpan.TraceID, traceID)
	}
	// Span ID must be fresh (not the parent's span ID).
	if capturedSpan.SpanID == "00f067aa0ba902b7" {
		t.Error("SpanID must be a fresh child span, not the parent's")
	}
	if len(capturedSpan.SpanID) != 16 {
		t.Errorf("SpanID length: got %d, want 16", len(capturedSpan.SpanID))
	}
}

func TestMiddleware_GeneratesNewTraceWhenNoneProvided(t *testing.T) {
	var span1, span2 trace.Span

	handler := trace.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if span1.TraceID == "" {
			span1 = trace.FromContext(r.Context())
		} else {
			span2 = trace.FromContext(r.Context())
		}
		w.WriteHeader(http.StatusOK)
	}))

	rr1 := httptest.NewRecorder()
	handler.ServeHTTP(rr1, httptest.NewRequest(http.MethodGet, "/", nil))

	rr2 := httptest.NewRecorder()
	handler.ServeHTTP(rr2, httptest.NewRequest(http.MethodGet, "/", nil))

	if span1.TraceID == "" || span2.TraceID == "" {
		t.Fatal("both spans must have non-empty TraceID")
	}
	if span1.TraceID == span2.TraceID {
		t.Error("two independent requests must have different TraceIDs")
	}
	if span1.SpanID == span2.SpanID {
		t.Error("two independent requests must have different SpanIDs")
	}
}

func TestMiddleware_StoresSpanInContext(t *testing.T) {
	handler := trace.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		span := trace.FromContext(r.Context())
		if span.TraceID == "" {
			http.Error(w, "no trace in context", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/", nil))

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestMiddleware_InvalidTraceparentGeneratesNew(t *testing.T) {
	badInputs := []string{
		"",
		"not-valid",
		"00-tooshort-abc-01",
		"01-4bf92f3577b34da6a3ce929d0e0e4736-abc-01", // wrong version
	}

	for _, bad := range badInputs {
		t.Run(bad, func(t *testing.T) {
			handler := trace.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				span := trace.FromContext(r.Context())
				if span.TraceID == "" {
					http.Error(w, "empty trace id", 500)
					return
				}
				w.WriteHeader(http.StatusOK)
			}))

			r := httptest.NewRequest(http.MethodGet, "/", nil)
			if bad != "" {
				r.Header.Set("traceparent", bad)
			}
			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, r)

			if rr.Code != http.StatusOK {
				t.Errorf("bad traceparent %q: got %d, want 200 (should generate fresh trace)", bad, rr.Code)
			}
		})
	}
}
