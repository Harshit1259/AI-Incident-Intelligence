package handlers

import (
	"encoding/json"
	"net/http"
	"strings"

	"ai-incident-platform/backend/internal/api"
	"ai-incident-platform/backend/internal/middleware"
	"ai-incident-platform/backend/internal/models"
	"ai-incident-platform/backend/internal/services"
)

// HealthScoreHandler exposes the Customer Health Score & Churn Prevention admin API.
// All routes require admin role — this is an internal operator dashboard, never
// exposed to the customer-facing UI.
//
// Routes:
//   GET  /api/v1/admin/health-scores              — all-tenant health dashboard
//   GET  /api/v1/admin/health-scores/{tenantID}   — single tenant score
//   GET  /api/v1/admin/health-scores/{tenantID}/actions  — CSM action log
//   POST /api/v1/admin/health-scores/{tenantID}/actions  — log a CSM action
type HealthScoreHandler struct {
	svc *services.HealthScoreService
}

func NewHealthScoreHandler(svc *services.HealthScoreService) *HealthScoreHandler {
	return &HealthScoreHandler{svc: svc}
}

// Handle dispatches all /api/v1/admin/health-scores* requests.
func (h *HealthScoreHandler) Handle(w http.ResponseWriter, r *http.Request) {
	sub := strings.TrimPrefix(r.URL.Path, "/api/v1/admin/health-scores")

	// /api/v1/admin/health-scores  — list all tenants
	if sub == "" || sub == "/" {
		if r.Method != http.MethodGet {
			api.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		h.listAll(w, r)
		return
	}

	tenantID, rest := healthPathID(sub)
	if tenantID == "" {
		api.WriteError(w, http.StatusNotFound, "not found")
		return
	}

	switch rest {
	case "", "/":
		// /api/v1/admin/health-scores/{tenantID}
		if r.Method != http.MethodGet {
			api.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		h.getOne(w, r, tenantID)

	case "/actions", "/actions/":
		switch r.Method {
		case http.MethodGet:
			h.listActions(w, r, tenantID)
		case http.MethodPost:
			h.logAction(w, r, tenantID)
		default:
			api.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		}

	default:
		api.WriteError(w, http.StatusNotFound, "not found")
	}
}

// listAll returns health scores for all active tenants, sorted lowest score first
// so at-risk customers appear at the top of the CSM dashboard.
func (h *HealthScoreHandler) listAll(w http.ResponseWriter, _ *http.Request) {
	scores, err := h.svc.ComputeAllScores()
	if err != nil {
		api.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Sort: critical first, then high, medium, low (ascending score = ascending risk).
	sortHealthScores(scores)

	api.WriteJSON(w, http.StatusOK, map[string]any{
		"tenants": scores,
		"total":   len(scores),
		"summary": buildSummary(scores),
	})
}

// getOne returns the health score for a single tenant.
func (h *HealthScoreHandler) getOne(w http.ResponseWriter, _ *http.Request, tenantID string) {
	score, err := h.svc.ComputeScore(tenantID)
	if err != nil {
		api.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	api.WriteJSON(w, http.StatusOK, score)
}

// listActions returns CSM actions previously logged against a tenant.
func (h *HealthScoreHandler) listActions(w http.ResponseWriter, _ *http.Request, tenantID string) {
	actions, err := h.svc.ListActions(tenantID)
	if err != nil {
		api.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if actions == nil {
		actions = []models.HealthAction{}
	}
	api.WriteJSON(w, http.StatusOK, map[string]any{
		"actions": actions,
		"total":   len(actions),
	})
}

// logAction records a new CSM action (call, email, note) against a tenant.
func (h *HealthScoreHandler) logAction(w http.ResponseWriter, r *http.Request, tenantID string) {
	claims, ok := middleware.ClaimsFromContext(r)
	if !ok {
		api.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var body struct {
		ActionType string `json:"action_type"`
		Notes      string `json:"notes"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		api.WriteError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	validTypes := map[string]bool{"email": true, "call": true, "note": true, "task": true}
	if !validTypes[body.ActionType] {
		api.WriteError(w, http.StatusBadRequest, "action_type must be one of: email, call, note, task")
		return
	}

	if err := h.svc.LogAction(models.HealthAction{
		TenantID:   tenantID,
		Actor:      claims.UserID,
		ActionType: body.ActionType,
		Notes:      body.Notes,
	}); err != nil {
		api.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	api.WriteJSON(w, http.StatusCreated, map[string]string{
		"status":      "logged",
		"tenant_id":   tenantID,
		"action_type": body.ActionType,
	})
}

// ── Helpers ───────────────────────────────────────────────────────────────────

type dashboardSummary struct {
	CriticalCount int `json:"critical_count"`
	HighCount     int `json:"high_count"`
	MediumCount   int `json:"medium_count"`
	LowCount      int `json:"low_count"`
	AvgScore      int `json:"avg_score"`
}

func buildSummary(scores []models.TenantHealthScore) dashboardSummary {
	s := dashboardSummary{}
	if len(scores) == 0 {
		return s
	}
	total := 0
	for _, sc := range scores {
		total += sc.Score
		switch sc.ChurnRisk {
		case models.ChurnRiskCritical:
			s.CriticalCount++
		case models.ChurnRiskHigh:
			s.HighCount++
		case models.ChurnRiskMedium:
			s.MediumCount++
		case models.ChurnRiskLow:
			s.LowCount++
		}
	}
	s.AvgScore = total / len(scores)
	return s
}

func sortHealthScores(scores []models.TenantHealthScore) {
	// Insertion sort — N tenants is small; avoids importing sort for readability.
	for i := 1; i < len(scores); i++ {
		for j := i; j > 0 && scores[j].Score < scores[j-1].Score; j-- {
			scores[j], scores[j-1] = scores[j-1], scores[j]
		}
	}
}

func healthPathID(sub string) (id, rest string) {
	sub = strings.TrimPrefix(sub, "/")
	if sub == "" {
		return "", ""
	}
	parts := strings.SplitN(sub, "/", 2)
	id = parts[0]
	if len(parts) > 1 {
		rest = "/" + parts[1]
	}
	return id, rest
}
