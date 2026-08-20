package routes

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// ── Helper unit tests ─────────────────────────────────────────────────────────

func TestIncidentSubPath(t *testing.T) {
	cases := []struct {
		path  string
		wantID  string
		wantSub string
		wantOK  bool
	}{
		// valid: id only
		{"/api/v1/incidents/abc-123", "abc-123", "", true},
		// valid: id + sub-resource
		{"/api/v1/incidents/abc-123/postmortem", "abc-123", "postmortem", true},
		// valid: id + nested sub-resource
		{"/api/v1/incidents/abc-123/postmortem/generate", "abc-123", "postmortem/generate", true},
		// valid: id + business-impact/generate
		{"/api/v1/incidents/abc-123/business-impact/generate", "abc-123", "business-impact/generate", true},
		// valid: id + executions
		{"/api/v1/incidents/abc-123/executions", "abc-123", "executions", true},
		// invalid: no id (bare prefix with trailing slash)
		{"/api/v1/incidents/", "", "", false},
		// invalid: wrong prefix
		{"/api/v2/incidents/abc-123", "", "", false},
		// invalid: completely different path
		{"/api/v1/events/abc-123", "", "", false},
	}

	for _, tc := range cases {
		t.Run(tc.path, func(t *testing.T) {
			id, sub, ok := incidentSubPath(tc.path)
			if ok != tc.wantOK {
				t.Errorf("ok: got %v, want %v", ok, tc.wantOK)
			}
			if id != tc.wantID {
				t.Errorf("id: got %q, want %q", id, tc.wantID)
			}
			if sub != tc.wantSub {
				t.Errorf("sub: got %q, want %q", sub, tc.wantSub)
			}
		})
	}
}

func TestMethodHandler(t *testing.T) {
	called := ""

	h := methodHandler(map[string]http.HandlerFunc{
		http.MethodGet: func(w http.ResponseWriter, r *http.Request) {
			called = "GET"
			w.WriteHeader(http.StatusOK)
		},
		http.MethodPost: func(w http.ResponseWriter, r *http.Request) {
			called = "POST"
			w.WriteHeader(http.StatusCreated)
		},
	})

	t.Run("GET dispatches correctly", func(t *testing.T) {
		called = ""
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		h(rec, req)
		if called != "GET" {
			t.Errorf("expected GET handler to be called, got %q", called)
		}
		if rec.Code != http.StatusOK {
			t.Errorf("expected 200, got %d", rec.Code)
		}
	})

	t.Run("POST dispatches correctly", func(t *testing.T) {
		called = ""
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/", nil)
		h(rec, req)
		if called != "POST" {
			t.Errorf("expected POST handler to be called, got %q", called)
		}
		if rec.Code != http.StatusCreated {
			t.Errorf("expected 201, got %d", rec.Code)
		}
	})

	t.Run("unregistered method returns 405", func(t *testing.T) {
		called = ""
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodDelete, "/", nil)
		h(rec, req)
		if called != "" {
			t.Errorf("no handler should be called for DELETE, got %q", called)
		}
		if rec.Code != http.StatusMethodNotAllowed {
			t.Errorf("expected 405, got %d", rec.Code)
		}
	})

	t.Run("PUT also returns 405 when not registered", func(t *testing.T) {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPut, "/", nil)
		h(rec, req)
		if rec.Code != http.StatusMethodNotAllowed {
			t.Errorf("expected 405, got %d", rec.Code)
		}
	})
}

// ── Route registration smoke tests ────────────────────────────────────────────
//
// These tests verify that the catch-all dispatch logic in the incident routes
// returns the correct sub-handler without spinning up the full application.
// We wire a minimal mux with stub handlers and fire real HTTP requests.

func stubHandler(id string, status int) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Handler", id)
		w.WriteHeader(status)
	}
}

// TestIncidentCatchAllDispatch registers a minimal incidents mux using the
// production dispatch logic and verifies that each sub-path routes correctly.
func TestIncidentCatchAllDispatch(t *testing.T) {
	// Thin wrappers that mimic what registerIncidentRoutes wires in production
	mux := http.NewServeMux()

	// identity — no-op role guard so tests don't need a JWT
	pass := func(h http.HandlerFunc) http.HandlerFunc { return h }
	noopAuth := func(h http.HandlerFunc) http.Handler { return h }

	// Stub handlers keyed by what sub-path they should receive
	handlers := map[string]http.HandlerFunc{
		"detail":                   stubHandler("detail", 200),
		"ack":                      stubHandler("ack", 200),
		"postmortem-get":           stubHandler("postmortem-get", 200),
		"postmortem-put":           stubHandler("postmortem-put", 200),
		"postmortem-generate":      stubHandler("postmortem-generate", 200),
		"business-impact-get":      stubHandler("business-impact-get", 200),
		"business-impact-generate": stubHandler("business-impact-generate", 200),
		"runbooks":                 stubHandler("runbooks", 200),
		"attribution":              stubHandler("attribution", 200),
		"verify":                   stubHandler("verify", 200),
		"verification":             stubHandler("verification-get", 200),
		"executions-get":           stubHandler("executions-get", 200),
		"executions-post":          stubHandler("executions-post", 201),
	}

	// Wire the catch-all using the production incidentSubPath + methodHandler
	mux.Handle("/api/v1/incidents/", noopAuth(func(w http.ResponseWriter, r *http.Request) {
		_, sub, ok := incidentSubPath(r.URL.Path)
		if !ok {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		switch sub {
		case "":
			methodHandler(map[string]http.HandlerFunc{http.MethodGet: handlers["detail"]})(w, r)
		case "ack", "resolve", "reopen":
			methodHandler(map[string]http.HandlerFunc{http.MethodPost: pass(handlers["ack"])})(w, r)
		case "postmortem":
			methodHandler(map[string]http.HandlerFunc{
				http.MethodGet: handlers["postmortem-get"],
				http.MethodPut: pass(handlers["postmortem-put"]),
			})(w, r)
		case "postmortem/generate":
			methodHandler(map[string]http.HandlerFunc{http.MethodPost: pass(handlers["postmortem-generate"])})(w, r)
		case "business-impact":
			methodHandler(map[string]http.HandlerFunc{http.MethodGet: handlers["business-impact-get"]})(w, r)
		case "business-impact/generate":
			methodHandler(map[string]http.HandlerFunc{http.MethodPost: pass(handlers["business-impact-generate"])})(w, r)
		case "runbooks":
			methodHandler(map[string]http.HandlerFunc{http.MethodGet: handlers["runbooks"]})(w, r)
		case "attribution":
			methodHandler(map[string]http.HandlerFunc{http.MethodGet: handlers["attribution"]})(w, r)
		case "verify":
			methodHandler(map[string]http.HandlerFunc{http.MethodPost: pass(handlers["verify"])})(w, r)
		case "verification":
			methodHandler(map[string]http.HandlerFunc{http.MethodGet: handlers["verification"]})(w, r)
		case "executions":
			methodHandler(map[string]http.HandlerFunc{
				http.MethodGet:  handlers["executions-get"],
				http.MethodPost: pass(handlers["executions-post"]),
			})(w, r)
		default:
			http.Error(w, "not found", http.StatusNotFound)
		}
	}))

	tests := []struct {
		method      string
		path        string
		wantStatus  int
		wantHandler string
	}{
		// Detail
		{"GET", "/api/v1/incidents/abc-123", 200, "detail"},
		{"POST", "/api/v1/incidents/abc-123", 405, ""},
		// State transitions
		{"POST", "/api/v1/incidents/abc-123/ack", 200, "ack"},
		{"POST", "/api/v1/incidents/abc-123/resolve", 200, "ack"},
		{"POST", "/api/v1/incidents/abc-123/reopen", 200, "ack"},
		{"GET", "/api/v1/incidents/abc-123/ack", 405, ""},
		// Postmortem
		{"GET", "/api/v1/incidents/abc-123/postmortem", 200, "postmortem-get"},
		{"PUT", "/api/v1/incidents/abc-123/postmortem", 200, "postmortem-put"},
		{"POST", "/api/v1/incidents/abc-123/postmortem/generate", 200, "postmortem-generate"},
		{"GET", "/api/v1/incidents/abc-123/postmortem", 200, "postmortem-get"},
		// Business impact
		{"GET", "/api/v1/incidents/abc-123/business-impact", 200, "business-impact-get"},
		{"POST", "/api/v1/incidents/abc-123/business-impact/generate", 200, "business-impact-generate"},
		// Runbooks & attribution
		{"GET", "/api/v1/incidents/abc-123/runbooks", 200, "runbooks"},
		{"GET", "/api/v1/incidents/abc-123/attribution", 200, "attribution"},
		// Verify / verification
		{"POST", "/api/v1/incidents/abc-123/verify", 200, "verify"},
		{"GET", "/api/v1/incidents/abc-123/verification", 200, "verification-get"},
		// Executions
		{"GET", "/api/v1/incidents/abc-123/executions", 200, "executions-get"},
		{"POST", "/api/v1/incidents/abc-123/executions", 201, "executions-post"},
		// Unknown sub-path
		{"GET", "/api/v1/incidents/abc-123/unknown-sub", 404, ""},
		// Empty ID (bare catch-all)
		{"GET", "/api/v1/incidents/", 404, ""},
	}

	for _, tt := range tests {
		t.Run(tt.method+" "+tt.path, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, nil)
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("status: got %d, want %d", rec.Code, tt.wantStatus)
			}
			if tt.wantHandler != "" {
				got := rec.Header().Get("X-Handler")
				if got != tt.wantHandler {
					t.Errorf("handler: got %q, want %q", got, tt.wantHandler)
				}
			}
		})
	}
}
