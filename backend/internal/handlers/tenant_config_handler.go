package handlers

// tenant_config_handler.go
//
// Admin API for the tenant configuration layer.
//
// Routes (all require JWT; write routes require admin role):
//   GET    /api/v1/admin/tenants            – list all tenants
//   POST   /api/v1/admin/tenants            – create / update tenant master
//   GET    /api/v1/admin/tenants/{id}       – get single tenant
//   PUT    /api/v1/admin/tenants/{id}       – update tenant fields
//
//   GET    /api/v1/admin/services           – list service catalog (current tenant)
//   POST   /api/v1/admin/services           – create catalog entry
//   PUT    /api/v1/admin/services/{id}      – update catalog entry
//   DELETE /api/v1/admin/services/{id}      – remove catalog entry
//
//   GET    /api/v1/admin/settings           – get tenant settings / multipliers
//   PUT    /api/v1/admin/settings           – update tenant settings
//
//   GET    /api/v1/admin/industry-templates – list available templates (read-only)

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"ai-incident-platform/backend/internal/api"
	"ai-incident-platform/backend/internal/middleware"
	"ai-incident-platform/backend/internal/models"
	"ai-incident-platform/backend/internal/services"
	"ai-incident-platform/backend/internal/store"
)

// TenantConfigHandler handles the admin configuration endpoints.
type TenantConfigHandler struct {
	tenantStore         *store.TenantStore
	serviceCatalogStore *store.ServiceCatalogStore
	tenantSettingsStore *store.TenantSettingsStore
	tenantCfgSvc        *services.TenantConfigService
}

func NewTenantConfigHandler(
	tenantStore *store.TenantStore,
	serviceCatalogStore *store.ServiceCatalogStore,
	tenantSettingsStore *store.TenantSettingsStore,
	tenantCfgSvc *services.TenantConfigService,
) *TenantConfigHandler {
	return &TenantConfigHandler{
		tenantStore:         tenantStore,
		serviceCatalogStore: serviceCatalogStore,
		tenantSettingsStore: tenantSettingsStore,
		tenantCfgSvc:        tenantCfgSvc,
	}
}

// ── Tenant master endpoints ───────────────────────────────────────────────────

// HandleTenants handles GET and POST /api/v1/admin/tenants
func (h *TenantConfigHandler) HandleTenants(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.listTenants(w, r)
	case http.MethodPost:
		h.createTenant(w, r)
	default:
		api.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// HandleTenantByID handles GET and PUT /api/v1/admin/tenants/{id}
func (h *TenantConfigHandler) HandleTenantByID(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.getTenant(w, r)
	case http.MethodPut:
		h.updateTenant(w, r)
	default:
		api.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (h *TenantConfigHandler) listTenants(w http.ResponseWriter, r *http.Request) {
	tenants, err := h.tenantStore.List()
	if err != nil {
		api.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if tenants == nil {
		tenants = []models.TenantMaster{}
	}
	api.WriteJSON(w, http.StatusOK, tenants)
}

func (h *TenantConfigHandler) createTenant(w http.ResponseWriter, r *http.Request) {
	var req models.TenantMaster
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		api.WriteError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if req.ID == "" {
		api.WriteError(w, http.StatusBadRequest, "id is required")
		return
	}
	if req.Name == "" {
		api.WriteError(w, http.StatusBadRequest, "name is required")
		return
	}
	if req.Slug == "" {
		req.Slug = slugify(req.Name)
	}
	setTenantDefaults(&req)

	if err := h.tenantStore.Upsert(req); err != nil {
		api.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	api.WriteJSON(w, http.StatusOK, req)
}

func (h *TenantConfigHandler) getTenant(w http.ResponseWriter, r *http.Request) {
	id := extractLastSegment(r.URL.Path)
	if id == "" {
		api.WriteError(w, http.StatusBadRequest, "tenant id required")
		return
	}
	t, err := h.tenantStore.GetByID(id)
	if err != nil {
		api.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if t == nil {
		api.WriteError(w, http.StatusNotFound, "tenant not found")
		return
	}
	api.WriteJSON(w, http.StatusOK, t)
}

func (h *TenantConfigHandler) updateTenant(w http.ResponseWriter, r *http.Request) {
	id := extractLastSegment(r.URL.Path)
	if id == "" {
		api.WriteError(w, http.StatusBadRequest, "tenant id required")
		return
	}

	existing, err := h.tenantStore.GetByID(id)
	if err != nil {
		api.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if existing == nil {
		api.WriteError(w, http.StatusNotFound, "tenant not found")
		return
	}

	var req models.TenantMaster
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		api.WriteError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	req.ID = id // ensure ID cannot be overwritten from body
	if req.Name == "" {
		req.Name = existing.Name
	}
	if req.Slug == "" {
		req.Slug = existing.Slug
	}
	req.CreatedAt = existing.CreatedAt
	setTenantDefaults(&req)

	if err := h.tenantStore.Upsert(req); err != nil {
		api.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	api.WriteJSON(w, http.StatusOK, req)
}

// ── Service catalog endpoints ─────────────────────────────────────────────────

// HandleServices handles GET and POST /api/v1/admin/services
func (h *TenantConfigHandler) HandleServices(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.listServices(w, r)
	case http.MethodPost:
		h.createService(w, r)
	default:
		api.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// HandleServiceByID handles PUT and DELETE /api/v1/admin/services/{id}
func (h *TenantConfigHandler) HandleServiceByID(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPut:
		h.updateService(w, r)
	case http.MethodDelete:
		h.deleteService(w, r)
	default:
		api.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (h *TenantConfigHandler) listServices(w http.ResponseWriter, r *http.Request) {
	tenantID := middleware.TenantFromRequest(r)
	entries, err := h.serviceCatalogStore.ListByTenant(tenantID)
	if err != nil {
		api.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if entries == nil {
		entries = []models.ServiceCatalogEntry{}
	}
	api.WriteJSON(w, http.StatusOK, entries)
}

func (h *TenantConfigHandler) createService(w http.ResponseWriter, r *http.Request) {
	tenantID := middleware.TenantFromRequest(r)
	var req models.ServiceCatalogEntry
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		api.WriteError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if req.ServiceName == "" {
		api.WriteError(w, http.StatusBadRequest, "service_name is required")
		return
	}
	req.TenantID = tenantID
	if req.Environment == "" {
		req.Environment = "prod"
	}
	if req.Tier == "" {
		req.Tier = "TIER_2"
	}
	if req.ID == "" {
		req.ID = fmt.Sprintf("sc-%s-%s-%s",
			tenantID, sanitizeCatalogID(req.ServiceName), sanitizeCatalogID(req.Environment))
	}
	now := time.Now()
	if req.CreatedAt.IsZero() {
		req.CreatedAt = now
	}
	req.UpdatedAt = now

	if err := h.serviceCatalogStore.Upsert(req); err != nil {
		api.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	api.WriteJSON(w, http.StatusOK, req)
}

func (h *TenantConfigHandler) updateService(w http.ResponseWriter, r *http.Request) {
	id := extractLastSegment(r.URL.Path)
	if id == "" {
		api.WriteError(w, http.StatusBadRequest, "service id required")
		return
	}

	existing, err := h.serviceCatalogStore.GetByID(id)
	if err != nil {
		api.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if existing == nil {
		api.WriteError(w, http.StatusNotFound, "service not found")
		return
	}

	var req models.ServiceCatalogEntry
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		api.WriteError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	req.ID = id
	req.TenantID = existing.TenantID
	req.CreatedAt = existing.CreatedAt
	req.UpdatedAt = time.Now()
	if req.ServiceName == "" {
		req.ServiceName = existing.ServiceName
	}
	if req.Environment == "" {
		req.Environment = existing.Environment
	}
	if req.Tier == "" {
		req.Tier = existing.Tier
	}

	if err := h.serviceCatalogStore.Upsert(req); err != nil {
		api.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	api.WriteJSON(w, http.StatusOK, req)
}

func (h *TenantConfigHandler) deleteService(w http.ResponseWriter, r *http.Request) {
	id := extractLastSegment(r.URL.Path)
	if id == "" {
		api.WriteError(w, http.StatusBadRequest, "service id required")
		return
	}
	if err := h.serviceCatalogStore.Delete(id); err != nil {
		api.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	api.WriteJSON(w, http.StatusOK, map[string]string{"status": "deleted", "id": id})
}

// ── Tenant settings endpoints ─────────────────────────────────────────────────

// HandleSettings handles GET and PUT /api/v1/admin/settings
func (h *TenantConfigHandler) HandleSettings(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.getSettings(w, r)
	case http.MethodPut:
		h.updateSettings(w, r)
	default:
		api.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (h *TenantConfigHandler) getSettings(w http.ResponseWriter, r *http.Request) {
	tenantID := middleware.TenantFromRequest(r)
	settings, err := h.tenantCfgSvc.GetSettings(tenantID)
	if err != nil {
		api.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	settings.TenantID = tenantID
	api.WriteJSON(w, http.StatusOK, settings)
}

func (h *TenantConfigHandler) updateSettings(w http.ResponseWriter, r *http.Request) {
	tenantID := middleware.TenantFromRequest(r)
	var req models.TenantSettings
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		api.WriteError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	req.TenantID = tenantID
	if req.ID == "" {
		req.ID = fmt.Sprintf("ts-%s", tenantID)
	}
	if req.ConfidenceMode == "" {
		req.ConfidenceMode = "balanced"
	}
	if req.SeverityMultipliers == nil {
		req.SeverityMultipliers = map[string]float64{}
	}
	if req.TierMultipliers == nil {
		req.TierMultipliers = map[string]float64{}
	}
	if req.MonthlyReportPrefs == nil {
		req.MonthlyReportPrefs = map[string]interface{}{}
	}

	if err := h.tenantSettingsStore.Upsert(req); err != nil {
		api.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	api.WriteJSON(w, http.StatusOK, req)
}

// ── Industry templates (read-only) ────────────────────────────────────────────

// HandleIndustryTemplates handles GET /api/v1/admin/industry-templates
func (h *TenantConfigHandler) HandleIndustryTemplates(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		api.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	api.WriteJSON(w, http.StatusOK, services.AvailableIndustryTemplates())
}

// ── Onboarding config bundle ──────────────────────────────────────────────────

// HandleOnboardingConfig handles POST /api/v1/admin/onboarding/config
// Accepts a full OnboardingConfig JSON body and persists all its parts atomically.
func (h *TenantConfigHandler) HandleOnboardingConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		api.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var cfg models.OnboardingConfig
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		api.WriteError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if cfg.Tenant.ID == "" {
		api.WriteError(w, http.StatusBadRequest, "tenant.id is required")
		return
	}

	setTenantDefaults(&cfg.Tenant)
	if err := h.tenantStore.Upsert(cfg.Tenant); err != nil {
		api.WriteError(w, http.StatusInternalServerError, "save tenant: "+err.Error())
		return
	}

	now := time.Now()
	for i := range cfg.Services {
		e := &cfg.Services[i]
		e.TenantID = cfg.Tenant.ID
		if e.ID == "" {
			e.ID = fmt.Sprintf("sc-%s-%s-%s",
				cfg.Tenant.ID, sanitizeCatalogID(e.ServiceName), sanitizeCatalogID(e.Environment))
		}
		if e.CreatedAt.IsZero() {
			e.CreatedAt = now
		}
		e.UpdatedAt = now
	}
	if len(cfg.Services) > 0 {
		if err := h.serviceCatalogStore.BulkUpsert(cfg.Services); err != nil {
			api.WriteError(w, http.StatusInternalServerError, "save services: "+err.Error())
			return
		}
	}

	if cfg.Settings != nil {
		cfg.Settings.TenantID = cfg.Tenant.ID
		if cfg.Settings.ID == "" {
			cfg.Settings.ID = fmt.Sprintf("ts-%s", cfg.Tenant.ID)
		}
		if cfg.Settings.SeverityMultipliers == nil {
			cfg.Settings.SeverityMultipliers = map[string]float64{}
		}
		if cfg.Settings.TierMultipliers == nil {
			cfg.Settings.TierMultipliers = map[string]float64{}
		}
		if cfg.Settings.MonthlyReportPrefs == nil {
			cfg.Settings.MonthlyReportPrefs = map[string]interface{}{}
		}
		if err := h.tenantSettingsStore.Upsert(*cfg.Settings); err != nil {
			api.WriteError(w, http.StatusInternalServerError, "save settings: "+err.Error())
			return
		}
	}

	api.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"status":         "ok",
		"tenant_id":      cfg.Tenant.ID,
		"services_saved": len(cfg.Services),
	})
}

// ── Helpers ───────────────────────────────────────────────────────────────────


func setTenantDefaults(t *models.TenantMaster) {
	if t.DeploymentMode == "" {
		t.DeploymentMode = "cloud"
	}
	if t.IndustryType == "" {
		t.IndustryType = "general"
	}
	if t.DefaultCurrency == "" {
		t.DefaultCurrency = "USD"
	}
	if t.Timezone == "" {
		t.Timezone = "UTC"
	}
	if t.EstimationMode == "" {
		t.EstimationMode = "balanced"
	}
}

func slugify(name string) string {
	s := strings.ToLower(strings.TrimSpace(name))
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		} else if r == ' ' || r == '_' || r == '-' {
			b.WriteRune('-')
		}
	}
	return b.String()
}

func sanitizeCatalogID(s string) string {
	s = strings.ToLower(s)
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			b.WriteRune(r)
		} else {
			b.WriteRune('-')
		}
	}
	return b.String()
}

func extractLastSegment(path string) string {
	path = strings.TrimSuffix(path, "/")
	parts := strings.Split(path, "/")
	if len(parts) == 0 {
		return ""
	}
	return parts[len(parts)-1]
}
