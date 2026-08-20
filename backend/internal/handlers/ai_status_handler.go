package handlers

import (
	"encoding/json"
	"net/http"

	"ai-incident-platform/backend/internal/llm"
)

// AIStatusHandler reports the current AI provider, mode, and call statistics.
type AIStatusHandler struct {
	client *llm.Client
}

func NewAIStatusHandler(c *llm.Client) *AIStatusHandler {
	return &AIStatusHandler{client: c}
}

// HandleGetAIStatus handles GET /api/v1/ai/status
// Public endpoint — no auth required (stats contain no sensitive data).
func (h *AIStatusHandler) HandleGetAIStatus(w http.ResponseWriter, r *http.Request) {
	stats := h.client.Stats()

	payload := map[string]interface{}{
		"configured":   h.client.IsConfigured(),
		"provider":     h.client.ProviderName(),
		"data_mode":    h.client.DataModeName(),
		"total_calls":  stats.TotalCalls,
		"failed_calls": stats.FailedCalls,
		"last_call_at": stats.LastCalledAt,
		// last_input_hash deliberately omitted from public status
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(payload)
}
