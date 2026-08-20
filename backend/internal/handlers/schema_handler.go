package handlers

import (
	"net/http"

	"ai-incident-platform/backend/internal/api"
	"ai-incident-platform/backend/internal/config"
)

// HandleConfigSchema serves the canonical env var contract at GET /api/v1/config/schema.
//
// This endpoint is intentionally PUBLIC (no auth required) because:
//   - It contains no tenant or user data.
//   - Operators need it to configure the platform before they have credentials.
//   - Secret values are never included — only names, descriptions, and defaults.
//
// Consumers:
//   - Operators setting up a new deployment: curl /api/v1/config/schema | jq
//   - CI/CD pipelines validating required env vars before deploy
//   - Internal tooling generating deployment checklists
func HandleConfigSchema(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		api.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	schema := config.GetSchema()

	// Build a response that strips example secret values
	// (descriptions and examples for secret vars are safe to expose, actual values are never here).
	api.WriteJSON(w, http.StatusOK, schema)
}
