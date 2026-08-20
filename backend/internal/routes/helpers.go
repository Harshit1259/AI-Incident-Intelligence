package routes

import (
	"net/http"
	"strings"
)

// methodHandler builds a HandlerFunc that dispatches by HTTP method.
// Unregistered methods respond with 405 Method Not Allowed.
//
// Usage:
//
//	mux.Handle("/api/v1/foo", withAuth(methodHandler(map[string]http.HandlerFunc{
//	    http.MethodGet:  handler.Get,
//	    http.MethodPost: requireOperator(handler.Create),
//	})))
func methodHandler(methods map[string]http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if h, ok := methods[r.Method]; ok {
			h(w, r)
			return
		}
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// incidentSubPath extracts the incident ID and sub-resource path from an
// /api/v1/incidents/... URL.
//
// Expected URL shapes and their return values:
//
//	/api/v1/incidents/{id}                → id, "",                  true
//	/api/v1/incidents/{id}/postmortem     → id, "postmortem",        true
//	/api/v1/incidents/{id}/postmortem/... → id, "postmortem/...",    true
//	/api/v1/incidents/   (no id)          →  "",  "", false
func incidentSubPath(path string) (id, sub string, ok bool) {
	const base = "/api/v1/incidents/"
	rest := strings.TrimPrefix(path, base)
	if rest == path { // prefix was not present
		return "", "", false
	}
	idx := strings.IndexByte(rest, '/')
	if idx < 0 {
		// /api/v1/incidents/{id} — no trailing slash, no sub-resource
		return rest, "", rest != ""
	}
	id = rest[:idx]
	sub = rest[idx+1:]
	return id, sub, id != ""
}
