package handlers

import (
	"encoding/json"
	"math"
	"net/http"
	"strings"
	"time"

	"ai-incident-platform/backend/internal/api"
	"ai-incident-platform/backend/internal/models"
	"ai-incident-platform/backend/internal/store"
)

// StatusHandler serves the public status page (no auth required).
//
// Routes:
//
//	GET  /api/v1/status                       — full status (default tenant)
//	GET  /api/v1/status/{tenant}              — full status for a specific tenant
//	POST /api/v1/status/subscribe             — subscribe to updates
//	GET  /api/v1/status/unsubscribe/{token}   — unsubscribe via link
type StatusHandler struct {
	statusStore *store.StatusStore
}

func NewStatusHandler(ss *store.StatusStore) *StatusHandler {
	return &StatusHandler{statusStore: ss}
}

// Handle dispatches all /api/v1/status* requests.
func (h *StatusHandler) Handle(w http.ResponseWriter, r *http.Request) {
	sub := strings.TrimPrefix(r.URL.Path, "/api/v1/status")

	switch {
	case sub == "" || sub == "/":
		if r.Method != http.MethodGet {
			api.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		h.getFullStatus(w, "default")

	case sub == "/subscribe":
		if r.Method != http.MethodPost {
			api.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		h.subscribe(w, r)

	case strings.HasPrefix(sub, "/unsubscribe/"):
		if r.Method != http.MethodGet {
			api.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		token := strings.TrimPrefix(sub, "/unsubscribe/")
		token = strings.TrimSuffix(token, "/")
		h.unsubscribe(w, token)

	default:
		// Could be /{tenant} or /{tenant}/subscribe
		trimmed := strings.TrimPrefix(sub, "/")
		parts := strings.SplitN(trimmed, "/", 2)
		tenant := parts[0]
		if tenant == "" {
			api.WriteError(w, http.StatusNotFound, "not found")
			return
		}
		if len(parts) == 2 && parts[1] == "subscribe" {
			if r.Method != http.MethodPost {
				api.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
				return
			}
			h.subscribeForTenant(w, r, tenant)
			return
		}
		if r.Method != http.MethodGet {
			api.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		h.getFullStatus(w, tenant)
	}
}

// GetStatus is kept as a named method for backward-compat with route registration.
func (h *StatusHandler) GetStatus(w http.ResponseWriter, r *http.Request) {
	h.Handle(w, r)
}

// ── Route handlers ────────────────────────────────────────────────────────────

func (h *StatusHandler) getFullStatus(w http.ResponseWriter, tenantID string) {
	if tenantID == "" {
		tenantID = "default"
	}

	// Fetch all data in parallel (each query is independent).
	type activeResult struct {
		items []models.StatusActiveIncident
		err   error
	}
	type historyResult struct {
		items []models.StatusHistoryEntry
		err   error
	}
	type uptimeResult struct {
		stats map[string]float64
		err   error
	}
	type mttrResult struct {
		minutes int
		err     error
	}
	type resolvedResult struct {
		count int
		err   error
	}
	type servicesResult struct {
		names []string
		err   error
	}

	activeCh := make(chan activeResult, 1)
	historyCh := make(chan historyResult, 1)
	uptimeCh := make(chan uptimeResult, 1)
	mttrCh := make(chan mttrResult, 1)
	resolvedCh := make(chan resolvedResult, 1)
	servicesCh := make(chan servicesResult, 1)

	go func() {
		items, err := h.statusStore.GetActiveIncidents(tenantID)
		activeCh <- activeResult{items, err}
	}()
	go func() {
		items, err := h.statusStore.GetResolvedHistory(tenantID, 30)
		historyCh <- historyResult{items, err}
	}()
	go func() {
		stats, err := h.statusStore.GetUptimePerService(tenantID)
		uptimeCh <- uptimeResult{stats, err}
	}()
	go func() {
		m, err := h.statusStore.GetMTTRMinutes(tenantID)
		mttrCh <- mttrResult{m, err}
	}()
	go func() {
		c, err := h.statusStore.CountResolvedLast30(tenantID)
		resolvedCh <- resolvedResult{c, err}
	}()
	go func() {
		names, err := h.statusStore.GetDistinctServices(tenantID)
		servicesCh <- servicesResult{names, err}
	}()

	activeRes := <-activeCh
	historyRes := <-historyCh
	uptimeRes := <-uptimeCh
	mttrRes := <-mttrCh
	resolvedRes := <-resolvedCh
	servicesRes := <-servicesCh

	// Use zero-value fallbacks on errors — status page must always respond.
	activeIncidents := activeRes.items
	if activeIncidents == nil {
		activeIncidents = []models.StatusActiveIncident{}
	}
	history := historyRes.items
	if history == nil {
		history = []models.StatusHistoryEntry{}
	}
	uptimeStats := uptimeRes.stats
	if uptimeStats == nil {
		uptimeStats = map[string]float64{}
	}

	// Build per-service status rows.
	// Count active incidents per service.
	activeByService := map[string]int{}
	for _, inc := range activeIncidents {
		activeByService[inc.Service]++
	}

	serviceNames := servicesRes.names
	if len(serviceNames) == 0 {
		// Fallback: derive from active incidents if GetDistinctServices failed.
		seen := map[string]bool{}
		for _, inc := range activeIncidents {
			if !seen[inc.Service] {
				serviceNames = append(serviceNames, inc.Service)
				seen[inc.Service] = true
			}
		}
	}

	services := make([]models.ServiceStatus, 0, len(serviceNames))
	for _, svc := range serviceNames {
		uptime, ok := uptimeStats[svc]
		if !ok {
			uptime = 100.0 // no incidents recorded → 100% uptime
		}
		activeCount := activeByService[svc]
		svcStatus := serviceStatusFromActiveCount(activeCount, activeIncidents, svc)
		services = append(services, models.ServiceStatus{
			Name:        svc,
			Status:      svcStatus,
			UptimePct:   uptime,
			ActiveCount: activeCount,
		})
	}

	// Compute overall platform uptime (average across services).
	overallUptime := computeOverallUptime(uptimeStats)

	// Compute overall page status.
	overallStatus := computeStatusFromIncidents(activeIncidents)

	resp := models.StatusPageFull{
		Tenant:          tenantID,
		OverallStatus:   overallStatus,
		Services:        services,
		ActiveIncidents: activeIncidents,
		History:         history,
		UptimeSummary: models.UptimeSummary{
			Uptime90d:      overallUptime,
			MTTRMinutes:    mttrRes.minutes,
			ResolvedLast30: resolvedRes.count,
		},
		UpdatedAt: time.Now().UTC(),
	}

	api.WriteJSON(w, http.StatusOK, resp)
}

func (h *StatusHandler) subscribe(w http.ResponseWriter, r *http.Request) {
	var req models.SubscribeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		api.WriteError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	h.doSubscribe(w, req)
}

func (h *StatusHandler) subscribeForTenant(w http.ResponseWriter, r *http.Request, tenant string) {
	var req models.SubscribeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		api.WriteError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	req.TenantID = tenant
	h.doSubscribe(w, req)
}

func (h *StatusHandler) doSubscribe(w http.ResponseWriter, req models.SubscribeRequest) {
	tenantID := req.TenantID
	if tenantID == "" {
		tenantID = "default"
	}
	if req.Channel != "email" && req.Channel != "slack" {
		api.WriteError(w, http.StatusBadRequest, "channel must be 'email' or 'slack'")
		return
	}
	if req.Target == "" {
		api.WriteError(w, http.StatusBadRequest, "target is required")
		return
	}

	token, err := h.statusStore.Subscribe(tenantID, req.Channel, req.Target)
	if err != nil {
		api.WriteError(w, http.StatusInternalServerError, "subscription failed")
		return
	}

	api.WriteJSON(w, http.StatusOK, map[string]any{
		"status":           "subscribed",
		"channel":          req.Channel,
		"unsubscribe_path": "/api/v1/status/unsubscribe/" + token,
		"message":          "You will be notified when this status page is updated.",
	})
}

func (h *StatusHandler) unsubscribe(w http.ResponseWriter, token string) {
	if token == "" {
		api.WriteError(w, http.StatusBadRequest, "token required")
		return
	}
	if err := h.statusStore.Unsubscribe(token); err != nil {
		api.WriteError(w, http.StatusInternalServerError, "unsubscribe failed")
		return
	}
	api.WriteJSON(w, http.StatusOK, map[string]string{
		"status":  "unsubscribed",
		"message": "You have been removed from status page notifications.",
	})
}

// ── Helpers ───────────────────────────────────────────────────────────────────

// computeStatusFromIncidents derives overall status from the severity of active incidents.
func computeStatusFromIncidents(incidents []models.StatusActiveIncident) string {
	if len(incidents) == 0 {
		return "operational"
	}
	for _, inc := range incidents {
		if strings.EqualFold(inc.Severity, "critical") {
			return "outage"
		}
	}
	return "degraded"
}

// serviceStatusFromActiveCount returns "outage", "degraded", or "operational"
// for a single service based on its active incidents' severities.
func serviceStatusFromActiveCount(count int, incidents []models.StatusActiveIncident, svc string) string {
	if count == 0 {
		return "operational"
	}
	for _, inc := range incidents {
		if inc.Service == svc && strings.EqualFold(inc.Severity, "critical") {
			return "outage"
		}
	}
	return "degraded"
}

// computeOverallUptime returns the average uptime across all services (0–100).
func computeOverallUptime(stats map[string]float64) float64 {
	if len(stats) == 0 {
		return 100.0
	}
	sum := 0.0
	for _, v := range stats {
		sum += v
	}
	avg := sum / float64(len(stats))
	return math.Round(avg*100) / 100
}
