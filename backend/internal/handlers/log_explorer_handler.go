package handlers

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"ai-incident-platform/backend/internal/api"
	"ai-incident-platform/backend/internal/middleware"
	"ai-incident-platform/backend/internal/models"
	"ai-incident-platform/backend/internal/store"
)

// LogExplorerHandler handles HTTP requests for the Log Explorer feature.
type LogExplorerHandler struct {
	logStore   *store.LogStore
	agentStore *store.AgentStore
}

// NewLogExplorerHandler creates a new LogExplorerHandler.
func NewLogExplorerHandler(ls *store.LogStore, as *store.AgentStore) *LogExplorerHandler {
	return &LogExplorerHandler{logStore: ls, agentStore: as}
}

// Query handles GET /api/v1/logs — query logs with filters.
func (h *LogExplorerHandler) Query(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		api.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	tenantID := middleware.TenantFromRequest(r)
	agentID := r.URL.Query().Get("agent_id")

	if agentID != "" {
		agent, err := h.agentStore.GetAgentForTenant(agentID, tenantID)
		if err != nil {
			slog.ErrorContext(r.Context(), "log_explorer: agent lookup error", "error", err)
			api.WriteError(w, http.StatusInternalServerError, "failed to verify agent")
			return
		}
		if agent == nil {
			api.WriteError(w, http.StatusForbidden, "agent not found in this tenant")
			return
		}
	}

	q := models.LogQuery{
		TenantID: tenantID,
		AgentID:  agentID,
		HostIP:   r.URL.Query().Get("host"),
		Category: r.URL.Query().Get("category"),
		Tag:      r.URL.Query().Get("tag"),
		Search:   r.URL.Query().Get("search"),
		Limit:    100,
		Offset:   0,
	}

	if v := r.URL.Query().Get("limit"); v != "" {
		if l, err := strconv.Atoi(v); err == nil && l > 0 {
			if l > 1000 {
				l = 1000
			}
			q.Limit = l
		}
	}
	if v := r.URL.Query().Get("offset"); v != "" {
		if o, err := strconv.Atoi(v); err == nil && o >= 0 {
			q.Offset = o
		}
	}

	if v := r.URL.Query().Get("from"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			q.From = &t
		}
	}
	if v := r.URL.Query().Get("to"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			q.To = &t
		}
	}

	resp, err := h.logStore.Query(q)
	if err != nil {
		slog.ErrorContext(r.Context(), "log_explorer: query error", "error", err)
		api.WriteError(w, http.StatusInternalServerError, "failed to query logs")
		return
	}

	api.WriteJSON(w, http.StatusOK, resp)
}

// Stats handles GET /api/v1/logs/stats — get log statistics.
func (h *LogExplorerHandler) Stats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		api.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	sinceHours := 24
	if v := r.URL.Query().Get("since"); v != "" {
		if h, err := strconv.Atoi(v); err == nil && h > 0 {
			sinceHours = h
		}
	}

	since := time.Now().Add(-time.Duration(sinceHours) * time.Hour)
	stats, err := h.logStore.GetStats(middleware.TenantFromRequest(r), since)
	if err != nil {
		slog.ErrorContext(r.Context(), "log_explorer: stats error", "error", err)
		api.WriteError(w, http.StatusInternalServerError, "failed to get log stats")
		return
	}

	api.WriteJSON(w, http.StatusOK, stats)
}

// Stream handles GET /api/v1/logs/stream — SSE stream of new logs.
func (h *LogExplorerHandler) Stream(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		api.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		api.WriteError(w, http.StatusInternalServerError, "streaming not supported")
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	// Parse last_id from query params (client can resume from a known position)
	var lastID int64
	if v := r.URL.Query().Get("last_id"); v != "" {
		if id, err := strconv.ParseInt(v, 10, 64); err == nil {
			lastID = id
		}
	}

	tenantID := middleware.TenantFromRequest(r)

	if agentID := r.URL.Query().Get("agent_id"); agentID != "" {
		agent, err := h.agentStore.GetAgentForTenant(agentID, tenantID)
		if err != nil {
			slog.ErrorContext(r.Context(), "log_explorer: stream agent lookup error", "error", err)
			api.WriteError(w, http.StatusInternalServerError, "failed to verify agent")
			return
		}
		if agent == nil {
			api.WriteError(w, http.StatusForbidden, "agent not found in this tenant")
			return
		}
	}

	// Timeout after 30 seconds — client will reconnect
	timeout := time.After(30 * time.Second)
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-timeout:
			// Send a comment to signal end, then return
			fmt.Fprintf(w, ": timeout\n\n")
			flusher.Flush()
			return
		case <-ticker.C:
			entries, err := h.logStore.GetNewLogsSince(tenantID, lastID, 50)
			if err != nil {
				slog.Error("log_explorer: stream query error", "error", err)
				continue
			}
			for _, entry := range entries {
				data, err := json.Marshal(entry)
				if err != nil {
					continue
				}
				fmt.Fprintf(w, "data: %s\n\n", data)
				if entry.ID > lastID {
					lastID = entry.ID
				}
			}
			if len(entries) > 0 {
				flusher.Flush()
			}
		}
	}
}
