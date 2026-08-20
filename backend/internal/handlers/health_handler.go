package handlers

import (
	"database/sql"
	"fmt"
	"log/slog"
	"net/http"
	"runtime"
	"sync/atomic"
	"time"

	"ai-incident-platform/backend/internal/api"
	"ai-incident-platform/backend/internal/platform/edition"
)

// Platform metrics tracked globally
var (
	startTime          = time.Now()
	eventsIngested     atomic.Int64
	eventsDropped      atomic.Int64
	incidentsCreated   atomic.Int64
	requestsServed     atomic.Int64
	requestErrors      atomic.Int64

	activeGate edition.Gate
)

// SetEditionGate stores the active edition gate so the health endpoint can
// report which modules are running. Called once from main before serving.
func SetEditionGate(g edition.Gate) { activeGate = g }

// IncrEventsIngested increments the ingestion counter.
func IncrEventsIngested(n int64)   { eventsIngested.Add(n) }
func IncrEventsDropped(n int64)    { eventsDropped.Add(n) }
func IncrIncidentsCreated()        { incidentsCreated.Add(1) }
func IncrRequestsServed()          { requestsServed.Add(1) }
func IncrRequestErrors()           { requestErrors.Add(1) }

// HealthHandler returns a readiness probe for Kubernetes / load balancers.
// Returns 503 if the database cannot be reached so the orchestrator can
// remove the pod from rotation and restart it.
func HealthHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := db.PingContext(r.Context()); err != nil {
			slog.ErrorContext(r.Context(), "health: database unreachable", "error", err)
			api.WriteJSON(w, http.StatusServiceUnavailable, map[string]interface{}{
				"status": "unhealthy",
				"error":  "database unreachable",
			})
			return
		}
		api.WriteJSON(w, http.StatusOK, map[string]interface{}{
			"status":   "ok",
			"service":  "backend",
			"editions": activeGate.Names(),
		})
	}
}

// PlatformMetricsHandler returns operational metrics for the platform itself.
// GET /api/v1/platform/metrics — no auth required (for monitoring tools)
func PlatformMetricsHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var memStats runtime.MemStats
		runtime.ReadMemStats(&memStats)

		// DB counts — query failures are surfaced in the response rather than
		// silently zeroed, so monitoring dashboards can detect a sick database.
		var incidentCount, eventCount, logCount, agentCount int
		dbAvailable := true
		var dbErrMsg string

		queries := []struct {
			sql  string
			dest *int
		}{
			{"SELECT COUNT(*) FROM incidents", &incidentCount},
			{"SELECT COUNT(*) FROM events", &eventCount},
			{"SELECT COUNT(*) FROM log_entries", &logCount},
			{"SELECT COUNT(*) FROM agents WHERE status = 'active'", &agentCount},
		}
		for _, q := range queries {
			if err := db.QueryRowContext(r.Context(), q.sql).Scan(q.dest); err != nil {
				dbAvailable = false
				dbErrMsg = err.Error()
				slog.ErrorContext(r.Context(), "platform_metrics: db query failed", "sql", q.sql, "error", err)
				break
			}
		}

		uptime := time.Since(startTime)

		payload := map[string]interface{}{
			"status":         "ok",
			"db_available":   dbAvailable,
			"uptime":         uptime.Round(time.Second).String(),
			"uptime_seconds": int(uptime.Seconds()),

			// Throughput counters (in-memory — always available)
			"events_ingested":   eventsIngested.Load(),
			"events_dropped":    eventsDropped.Load(),
			"incidents_created": incidentsCreated.Load(),
			"requests_served":   requestsServed.Load(),
			"request_errors":    requestErrors.Load(),

			// Database row counts
			"total_incidents": incidentCount,
			"total_events":    eventCount,
			"total_logs":      logCount,
			"active_agents":   agentCount,

			// Runtime
			"goroutines":      runtime.NumGoroutine(),
			"memory_alloc_mb": fmt.Sprintf("%.1f", float64(memStats.Alloc)/1e6),
			"memory_sys_mb":   fmt.Sprintf("%.1f", float64(memStats.Sys)/1e6),
			"gc_count":        memStats.NumGC,

			// DB pool
			"db_open_connections": db.Stats().OpenConnections,
			"db_in_use":           db.Stats().InUse,
			"db_idle":             db.Stats().Idle,
		}
		if !dbAvailable {
			payload["db_error"] = dbErrMsg
		}

		api.WriteJSON(w, http.StatusOK, payload)
	}
}
