package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"

	"ai-incident-platform/backend/internal/api"
	"ai-incident-platform/backend/internal/middleware"
	"ai-incident-platform/backend/internal/models"
	"ai-incident-platform/backend/internal/services"
)

// AlertQualityHandler serves the alert quality governance API.
type AlertQualityHandler struct {
	svc *services.AlertQualityService
}

func NewAlertQualityHandler(svc *services.AlertQualityService) *AlertQualityHandler {
	return &AlertQualityHandler{svc: svc}
}

// HandleReport — GET /api/v1/alert-quality/report?window=30
func (h *AlertQualityHandler) HandleReport(w http.ResponseWriter, r *http.Request) {
	tenantID := middleware.TenantFromRequest(r)
	windowDays := parseIntQuery(r, "window", 30)

	report, err := h.svc.GetReport(tenantID, windowDays)
	if err != nil {
		api.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	api.WriteJSON(w, http.StatusOK, report)
}

// HandleNoisy — GET /api/v1/alert-quality/noisy?window=7
func (h *AlertQualityHandler) HandleNoisy(w http.ResponseWriter, r *http.Request) {
	tenantID := middleware.TenantFromRequest(r)
	windowDays := parseIntQuery(r, "window", 7)

	alerts, err := h.svc.GetNoisyAlerts(tenantID, windowDays)
	if err != nil {
		api.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if alerts == nil {
		alerts = []models.NoisyAlertRule{}
	}
	api.WriteJSON(w, http.StatusOK, map[string]any{
		"tenant_id":    tenantID,
		"window_days":  windowDays,
		"noisy_alerts": alerts,
		"count":        len(alerts),
	})
}

// HandleDuplicates — GET /api/v1/alert-quality/duplicates?window=7
func (h *AlertQualityHandler) HandleDuplicates(w http.ResponseWriter, r *http.Request) {
	tenantID := middleware.TenantFromRequest(r)
	windowDays := parseIntQuery(r, "window", 7)

	dups, err := h.svc.GetDuplicates(tenantID, windowDays)
	if err != nil {
		api.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if dups == nil {
		dups = []models.DuplicatePair{}
	}
	api.WriteJSON(w, http.StatusOK, map[string]any{
		"tenant_id":   tenantID,
		"window_days": windowDays,
		"duplicates":  dups,
		"count":       len(dups),
	})
}

// HandleStale — GET /api/v1/alert-quality/stale
func (h *AlertQualityHandler) HandleStale(w http.ResponseWriter, r *http.Request) {
	tenantID := middleware.TenantFromRequest(r)

	stale, err := h.svc.GetStaleRules(tenantID)
	if err != nil {
		api.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if stale == nil {
		stale = []models.StaleRule{}
	}
	api.WriteJSON(w, http.StatusOK, map[string]any{
		"tenant_id":  tenantID,
		"stale_days": 30,
		"stale":      stale,
		"count":      len(stale),
	})
}

// HandleDebt — GET /api/v1/alert-quality/debt?window=30
func (h *AlertQualityHandler) HandleDebt(w http.ResponseWriter, r *http.Request) {
	tenantID := middleware.TenantFromRequest(r)
	windowDays := parseIntQuery(r, "window", 30)

	// Full report needed to cross-reference noisy/stale/dup into debt.
	report, err := h.svc.GetReport(tenantID, windowDays)
	if err != nil {
		api.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	debt := report.AlertDebt
	if debt == nil {
		debt = []models.TeamAlertDebt{}
	}
	api.WriteJSON(w, http.StatusOK, map[string]any{
		"tenant_id":   tenantID,
		"window_days": windowDays,
		"alert_debt":  debt,
		"count":       len(debt),
	})
}

// HandleRegisterRule — POST /api/v1/alert-quality/rules
func (h *AlertQualityHandler) HandleRegisterRule(w http.ResponseWriter, r *http.Request) {
	tenantID := middleware.TenantFromRequest(r)

	var reg models.AlertRuleRegistration
	if err := json.NewDecoder(r.Body).Decode(&reg); err != nil {
		api.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if err := h.svc.RegisterRule(tenantID, reg); err != nil {
		api.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	api.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok", "id": reg.ID})
}

func parseIntQuery(r *http.Request, key string, defaultVal int) int {
	v := r.URL.Query().Get(key)
	if v == "" {
		return defaultVal
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		return defaultVal
	}
	return n
}
