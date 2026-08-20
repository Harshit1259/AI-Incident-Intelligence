package handlers

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"ai-incident-platform/backend/internal/api"
	"ai-incident-platform/backend/internal/models"
	"ai-incident-platform/backend/internal/services"
)

// ChangeIntelligenceHandler serves change intelligence endpoints.
type ChangeIntelligenceHandler struct {
	svc       *services.ChangeIntelligenceService
	detailSvc *services.IncidentDetailService
}

// NewChangeIntelligenceHandler creates a new ChangeIntelligenceHandler.
func NewChangeIntelligenceHandler(
	svc *services.ChangeIntelligenceService,
	detailSvc *services.IncidentDetailService,
) *ChangeIntelligenceHandler {
	return &ChangeIntelligenceHandler{svc: svc, detailSvc: detailSvc}
}

// HandleIncidentReport builds and returns the full change intelligence report
// for an incident.
// GET /api/v1/incidents/{id}/change-intelligence
func (h *ChangeIntelligenceHandler) HandleIncidentReport(w http.ResponseWriter, r *http.Request) {
	incidentID := extractSegment(r.URL.Path, "/change-intelligence")
	if incidentID == "" {
		api.WriteError(w, http.StatusBadRequest, "incident id is required")
		return
	}

	detail, found := h.detailSvc.GetIncidentDetail(incidentID)
	if !found {
		api.WriteError(w, http.StatusNotFound, "incident not found")
		return
	}

	report, err := h.svc.BuildReport(detail.Incident)
	if err != nil {
		api.WriteError(w, http.StatusInternalServerError, "failed to build change intelligence report")
		return
	}

	api.WriteJSON(w, http.StatusOK, report)
}

// HandleIngestFeatureFlag accepts a feature flag change event for correlation.
// POST /api/v1/ingest/feature-flags
// Body: FeatureFlagChange JSON
func (h *ChangeIntelligenceHandler) HandleIngestFeatureFlag(w http.ResponseWriter, r *http.Request) {
	tenantID := tenantFromRequest(r)

	var f models.FeatureFlagChange
	if err := json.NewDecoder(r.Body).Decode(&f); err != nil {
		api.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if f.FlagName == "" {
		api.WriteError(w, http.StatusBadRequest, "flag_name is required")
		return
	}

	result, err := h.svc.IngestFeatureFlag(tenantID, f)
	if err != nil {
		api.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	api.WriteJSON(w, http.StatusCreated, result)
}

// HandleConfigDrift accepts a config snapshot and detects drift vs. baseline.
// POST /api/v1/ingest/config-drift
// Body: {"service": "auth", "config": {"key": "value", ...}}
func (h *ChangeIntelligenceHandler) HandleConfigDrift(w http.ResponseWriter, r *http.Request) {
	tenantID := tenantFromRequest(r)

	var body struct {
		Service string            `json:"service"`
		Config  map[string]string `json:"config"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		api.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if body.Service == "" {
		api.WriteError(w, http.StatusBadRequest, "service is required")
		return
	}
	if len(body.Config) == 0 {
		api.WriteError(w, http.StatusBadRequest, "config map must not be empty")
		return
	}

	drifted, err := h.svc.DetectConfigDrift(tenantID, body.Service, body.Config)
	if err != nil {
		api.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if drifted == nil {
		drifted = []models.ConfigDriftEvent{}
	}
	api.WriteJSON(w, http.StatusOK, map[string]any{
		"service":      body.Service,
		"drift_events": drifted,
		"drift_count":  len(drifted),
	})
}

// HandleGetRecent returns all enriched change events in a look-back window.
// GET /api/v1/change-intelligence/recent?since=2h
func (h *ChangeIntelligenceHandler) HandleGetRecent(w http.ResponseWriter, r *http.Request) {
	tenantID := tenantFromRequest(r)

	since := time.Now().Add(-2 * time.Hour)
	if raw := r.URL.Query().Get("since"); raw != "" {
		d, err := time.ParseDuration(raw)
		if err == nil && d > 0 {
			since = time.Now().Add(-d)
		}
	}

	changes, err := h.svc.GetRecentChanges(tenantID, since)
	if err != nil {
		api.WriteError(w, http.StatusInternalServerError, "failed to load changes")
		return
	}
	if changes == nil {
		changes = []models.ChangeEvent{}
	}

	api.WriteJSON(w, http.StatusOK, map[string]any{
		"since":   since.Format(time.RFC3339),
		"count":   len(changes),
		"changes": changes,
	})
}

// ─────────────────────────────────────────────────────────────────────────────
// Path helpers (local to this file)
// ─────────────────────────────────────────────────────────────────────────────

// extractSegment extracts the path segment immediately before suffix.
// e.g. /api/v1/incidents/inc-123/change-intelligence  → "inc-123"
func extractSegment(path, suffix string) string {
	path = strings.TrimSuffix(path, suffix)
	path = strings.TrimSuffix(path, "/")
	parts := strings.Split(path, "/")
	if len(parts) == 0 {
		return ""
	}
	return strings.TrimSpace(parts[len(parts)-1])
}

// tenantFromRequest resolves the tenant from a Bearer JWT claim (best-effort)
// or falls back to "default".
func tenantFromRequest(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	if auth == "" {
		return "default"
	}
	// Simple heuristic: read X-Tenant-ID header set by auth middleware if present.
	if t := r.Header.Get("X-Tenant-ID"); t != "" {
		return t
	}
	return "default"
}
