package services

// tenant_config_service.go
//
// Runtime config lookup chain:
//   customer override (DB) → industry template (code defaults per vertical) → global system default
//
// This service is the single point of truth for "what does this service cost"
// and "what multipliers apply to this tenant". BusinessImpactService delegates
// to it instead of embedding hardcoded maps.

import (
	"fmt"
	"strings"
	"time"

	"ai-incident-platform/backend/internal/models"
	"ai-incident-platform/backend/internal/store"
)

// TenantConfigService resolves per-tenant configuration at runtime.
type TenantConfigService struct {
	tenantStore         *store.TenantStore
	serviceCatalogStore *store.ServiceCatalogStore
	tenantSettingsStore *store.TenantSettingsStore
	bizImpactStore      *store.BusinessImpactStore
}

func NewTenantConfigService(
	tenantStore *store.TenantStore,
	serviceCatalogStore *store.ServiceCatalogStore,
	tenantSettingsStore *store.TenantSettingsStore,
	bizImpactStore *store.BusinessImpactStore,
) *TenantConfigService {
	return &TenantConfigService{
		tenantStore:         tenantStore,
		serviceCatalogStore: serviceCatalogStore,
		tenantSettingsStore: tenantSettingsStore,
		bizImpactStore:      bizImpactStore,
	}
}

// ── Public lookup API ────────────────────────────────────────────────────────

// GetServiceProfile resolves a full ServiceProfile for the given tenant+service.
// Lookup order: customer DB profile → catalog+industry template → global default.
func (s *TenantConfigService) GetServiceProfile(tenantID, serviceName string) (*models.ServiceProfile, error) {
	// Layer 1: customer-configured DB profile
	profile, err := s.bizImpactStore.GetProfileByService(tenantID, serviceName)
	if err != nil {
		return nil, fmt.Errorf("get service profile: %w", err)
	}

	// Load catalog entry regardless — it may carry authoritative tier even if
	// the customer has a financial profile already.
	catalog, _ := s.serviceCatalogStore.GetByServiceName(tenantID, serviceName)

	if profile != nil {
		// Override tier from catalog if catalog has a more specific value.
		if catalog != nil && catalog.Tier != "" {
			profile.Tier = catalog.Tier
		}
		return profile, nil
	}

	// Layer 2: build a profile from the catalog entry + industry template defaults.
	if catalog != nil {
		return s.buildFromCatalog(tenantID, catalog), nil
	}

	// Layer 3: global default derived from service name heuristics.
	tenant, _ := s.tenantStore.GetByID(tenantID)
	industryType := "general"
	if tenant != nil && tenant.IndustryType != "" {
		industryType = tenant.IndustryType
	}
	currency := "USD"
	if tenant != nil && tenant.DefaultCurrency != "" {
		currency = tenant.DefaultCurrency
	}
	p := industryDefaultProfile(industryType, serviceName)
	p.TenantID = tenantID
	p.Currency = currency
	return p, nil
}

// GetBaseline resolves a baseline MTTR for tenant+service+incidentType.
// Falls back to the "auto" type baseline if a type-specific one does not exist.
func (s *TenantConfigService) GetBaseline(tenantID, serviceName, incidentType string) (*models.IncidentBaseline, error) {
	b, err := s.bizImpactStore.GetBaseline(tenantID, serviceName, incidentType)
	if err != nil {
		return nil, fmt.Errorf("get baseline: %w", err)
	}
	if b != nil {
		return b, nil
	}
	if incidentType != "auto" {
		b, err = s.bizImpactStore.GetBaseline(tenantID, serviceName, "auto")
		if err != nil {
			return nil, fmt.Errorf("get auto baseline: %w", err)
		}
	}
	return b, nil
}

// GetSettings resolves TenantSettings, merging missing keys from system defaults.
func (s *TenantConfigService) GetSettings(tenantID string) (*models.TenantSettings, error) {
	settings, err := s.tenantSettingsStore.GetByTenant(tenantID)
	if err != nil {
		return nil, err
	}

	defaults := systemDefaultSettings()
	if settings == nil {
		defaults.TenantID = tenantID
		return defaults, nil
	}

	// Fill missing severity multipliers from system defaults.
	for k, v := range defaults.SeverityMultipliers {
		if _, ok := settings.SeverityMultipliers[k]; !ok {
			settings.SeverityMultipliers[k] = v
		}
	}
	// Fill missing tier multipliers from system defaults.
	for k, v := range defaults.TierMultipliers {
		if _, ok := settings.TierMultipliers[k]; !ok {
			settings.TierMultipliers[k] = v
		}
	}
	if settings.ConfidenceMode == "" {
		settings.ConfidenceMode = defaults.ConfidenceMode
	}
	return settings, nil
}

// GetTierForService returns the service tier from the catalog or "TIER_2" as default.
func (s *TenantConfigService) GetTierForService(tenantID, serviceName string) string {
	catalog, err := s.serviceCatalogStore.GetByServiceName(tenantID, serviceName)
	if err != nil || catalog == nil || catalog.Tier == "" {
		return "TIER_2"
	}
	return catalog.Tier
}

// ── Internal helpers ─────────────────────────────────────────────────────────

func (s *TenantConfigService) buildFromCatalog(tenantID string, catalog *models.ServiceCatalogEntry) *models.ServiceProfile {
	tenant, _ := s.tenantStore.GetByID(tenantID)
	industryType := "general"
	if tenant != nil && tenant.IndustryType != "" {
		industryType = tenant.IndustryType
	}
	currency := "USD"
	if tenant != nil && tenant.DefaultCurrency != "" {
		currency = tenant.DefaultCurrency
	}

	p := industryDefaultProfile(industryType, catalog.ServiceName)
	p.ID = fmt.Sprintf("sc-%s-%s", tenantID, catalog.ServiceName)
	p.TenantID = tenantID
	p.Service = catalog.ServiceName
	p.Tier = catalog.Tier
	p.Currency = currency
	return p
}

// ── Industry templates ────────────────────────────────────────────────────────

// industryDefaultProfile returns a ServiceProfile based on the industry vertical
// and service-name heuristics. This is the "industry template" layer —
// not customer-specific, but much more accurate than a single global default.
func industryDefaultProfile(industryType, serviceName string) *models.ServiceProfile {
	svc := strings.ToLower(serviceName)

	p := &models.ServiceProfile{
		CostModelType:          "mixed",
		BusinessHourMultiplier: 1.0,
		PeakMultiplier:         1.0,
		ConfidenceMode:         "balanced",
		Currency:               "USD",
		CreatedAt:              time.Now(),
		UpdatedAt:              time.Now(),
	}

	switch industryType {
	case "fintech", "banking", "insurance":
		applyFintechDefaults(p, svc)
	case "ecommerce", "retail", "marketplace":
		applyEcommerceDefaults(p, svc)
	case "saas", "b2b", "software":
		applySaaSDefaults(p, svc)
	case "healthcare", "pharma", "medtech":
		applyHealthcareDefaults(p, svc)
	case "manufacturing", "logistics", "supply_chain":
		applyManufacturingDefaults(p, svc)
	default:
		applyGeneralDefaults(p, svc)
	}
	return p
}

func applyFintechDefaults(p *models.ServiceProfile, svc string) {
	switch {
	case containsAnyStr(svc, "payment", "transaction", "transfer", "wallet", "ledger"):
		p.Tier, p.HourlyRevenue, p.UsersPerHour = "TIER_0", 50000, 100000
		p.SLAPenaltyPerMinute, p.SLAThresholdMinutes = 500, 2
		p.EmployeeCostPerHour, p.InfraCostPerHour = 30, 200
		p.BusinessHourMultiplier, p.PeakMultiplier = 1.3, 1.8
	case containsAnyStr(svc, "auth", "login", "kyc", "identity", "fraud"):
		p.Tier, p.HourlyRevenue, p.UsersPerHour = "TIER_0", 20000, 50000
		p.SLAPenaltyPerMinute, p.SLAThresholdMinutes = 300, 3
		p.EmployeeCostPerHour, p.InfraCostPerHour = 30, 150
		p.BusinessHourMultiplier, p.PeakMultiplier = 1.2, 1.5
	case containsAnyStr(svc, "api", "gateway", "notification"):
		p.Tier, p.HourlyRevenue, p.UsersPerHour = "TIER_1", 10000, 20000
		p.SLAPenaltyPerMinute, p.SLAThresholdMinutes = 100, 5
		p.EmployeeCostPerHour, p.InfraCostPerHour = 30, 100
		p.BusinessHourMultiplier, p.PeakMultiplier = 1.2, 1.4
	case containsAnyStr(svc, "worker", "batch", "cron", "job"):
		p.Tier, p.HourlyRevenue, p.UsersPerHour = "TIER_3", 0, 0
		p.SLAPenaltyPerMinute, p.SLAThresholdMinutes = 0, 60
		p.EmployeeCostPerHour, p.InfraCostPerHour = 30, 20
	default:
		p.Tier, p.HourlyRevenue, p.UsersPerHour = "TIER_2", 2000, 5000
		p.SLAPenaltyPerMinute, p.SLAThresholdMinutes = 20, 15
		p.EmployeeCostPerHour, p.InfraCostPerHour = 30, 30
		p.BusinessHourMultiplier, p.PeakMultiplier = 1.1, 1.2
	}
}

func applyEcommerceDefaults(p *models.ServiceProfile, svc string) {
	switch {
	case containsAnyStr(svc, "checkout", "cart", "order", "payment"):
		p.Tier, p.HourlyRevenue, p.UsersPerHour = "TIER_0", 30000, 80000
		p.SLAPenaltyPerMinute, p.SLAThresholdMinutes = 300, 5
		p.EmployeeCostPerHour, p.InfraCostPerHour = 25, 150
		p.BusinessHourMultiplier, p.PeakMultiplier = 1.2, 2.5
	case containsAnyStr(svc, "catalog", "search", "product", "inventory"):
		p.Tier, p.HourlyRevenue, p.UsersPerHour = "TIER_1", 10000, 30000
		p.SLAPenaltyPerMinute, p.SLAThresholdMinutes = 100, 10
		p.EmployeeCostPerHour, p.InfraCostPerHour = 25, 80
		p.BusinessHourMultiplier, p.PeakMultiplier = 1.2, 2.0
	case containsAnyStr(svc, "recommendation", "personalization", "ml"):
		p.Tier, p.HourlyRevenue, p.UsersPerHour = "TIER_1", 8000, 25000
		p.SLAPenaltyPerMinute, p.SLAThresholdMinutes = 50, 15
		p.EmployeeCostPerHour, p.InfraCostPerHour = 25, 60
		p.BusinessHourMultiplier, p.PeakMultiplier = 1.1, 1.8
	case containsAnyStr(svc, "worker", "batch", "cron"):
		p.Tier, p.HourlyRevenue, p.UsersPerHour = "TIER_3", 0, 0
		p.SLAPenaltyPerMinute, p.SLAThresholdMinutes = 0, 60
		p.EmployeeCostPerHour, p.InfraCostPerHour = 25, 10
	default:
		p.Tier, p.HourlyRevenue, p.UsersPerHour = "TIER_2", 2000, 5000
		p.SLAPenaltyPerMinute, p.SLAThresholdMinutes = 20, 15
		p.EmployeeCostPerHour, p.InfraCostPerHour = 25, 25
		p.BusinessHourMultiplier, p.PeakMultiplier = 1.0, 1.5
	}
}

func applySaaSDefaults(p *models.ServiceProfile, svc string) {
	switch {
	case containsAnyStr(svc, "api", "gateway", "auth", "sso"):
		p.Tier, p.HourlyRevenue, p.UsersPerHour = "TIER_0", 15000, 40000
		p.SLAPenaltyPerMinute, p.SLAThresholdMinutes = 200, 5
		p.EmployeeCostPerHour, p.InfraCostPerHour = 30, 120
		p.BusinessHourMultiplier, p.PeakMultiplier = 1.3, 1.5
	case containsAnyStr(svc, "database", "db", "postgres", "mysql", "redis"):
		p.Tier, p.HourlyRevenue, p.UsersPerHour = "TIER_0", 12000, 0
		p.SLAPenaltyPerMinute, p.SLAThresholdMinutes = 150, 5
		p.EmployeeCostPerHour, p.InfraCostPerHour = 30, 100
		p.BusinessHourMultiplier, p.PeakMultiplier = 1.2, 1.4
	case containsAnyStr(svc, "notification", "email", "webhook"):
		p.Tier, p.HourlyRevenue, p.UsersPerHour = "TIER_2", 3000, 10000
		p.SLAPenaltyPerMinute, p.SLAThresholdMinutes = 30, 15
		p.EmployeeCostPerHour, p.InfraCostPerHour = 30, 40
		p.BusinessHourMultiplier, p.PeakMultiplier = 1.1, 1.3
	case containsAnyStr(svc, "worker", "queue", "job", "cron", "batch"):
		p.Tier, p.HourlyRevenue, p.UsersPerHour = "TIER_3", 0, 0
		p.SLAPenaltyPerMinute, p.SLAThresholdMinutes = 0, 60
		p.EmployeeCostPerHour, p.InfraCostPerHour = 30, 15
	default:
		p.Tier, p.HourlyRevenue, p.UsersPerHour = "TIER_2", 3000, 10000
		p.SLAPenaltyPerMinute, p.SLAThresholdMinutes = 30, 15
		p.EmployeeCostPerHour, p.InfraCostPerHour = 30, 40
		p.BusinessHourMultiplier, p.PeakMultiplier = 1.1, 1.3
	}
}

func applyHealthcareDefaults(p *models.ServiceProfile, svc string) {
	switch {
	case containsAnyStr(svc, "ehr", "patient", "clinical", "prescription", "pharmacy"):
		p.Tier, p.HourlyRevenue, p.UsersPerHour = "TIER_0", 5000, 10000
		p.SLAPenaltyPerMinute, p.SLAThresholdMinutes = 1000, 5
		p.EmployeeCostPerHour, p.InfraCostPerHour = 40, 100
		p.BusinessHourMultiplier, p.PeakMultiplier = 1.5, 1.2
	case containsAnyStr(svc, "billing", "insurance", "claim"):
		p.Tier, p.HourlyRevenue, p.UsersPerHour = "TIER_1", 3000, 5000
		p.SLAPenaltyPerMinute, p.SLAThresholdMinutes = 200, 10
		p.EmployeeCostPerHour, p.InfraCostPerHour = 40, 60
		p.BusinessHourMultiplier, p.PeakMultiplier = 1.3, 1.1
	default:
		p.Tier, p.HourlyRevenue, p.UsersPerHour = "TIER_2", 1000, 2000
		p.SLAPenaltyPerMinute, p.SLAThresholdMinutes = 100, 15
		p.EmployeeCostPerHour, p.InfraCostPerHour = 40, 30
		p.BusinessHourMultiplier, p.PeakMultiplier = 1.2, 1.1
	}
}

func applyManufacturingDefaults(p *models.ServiceProfile, svc string) {
	switch {
	case containsAnyStr(svc, "plc", "scada", "production", "line", "assembly"):
		p.Tier, p.HourlyRevenue, p.UsersPerHour = "TIER_0", 0, 0
		p.SLAPenaltyPerMinute, p.SLAThresholdMinutes = 0, 5
		p.EmployeeCostPerHour, p.InfraCostPerHour = 35, 500
	case containsAnyStr(svc, "erp", "inventory", "supply"):
		p.Tier, p.HourlyRevenue, p.UsersPerHour = "TIER_1", 2000, 500
		p.SLAPenaltyPerMinute, p.SLAThresholdMinutes = 50, 15
		p.EmployeeCostPerHour, p.InfraCostPerHour = 35, 80
	default:
		p.Tier, p.HourlyRevenue, p.UsersPerHour = "TIER_2", 0, 0
		p.SLAPenaltyPerMinute, p.SLAThresholdMinutes = 0, 30
		p.EmployeeCostPerHour, p.InfraCostPerHour = 35, 50
	}
}

func applyGeneralDefaults(p *models.ServiceProfile, svc string) {
	switch {
	case containsAnyStr(svc, "payment", "checkout", "order", "billing"):
		p.Tier, p.HourlyRevenue, p.UsersPerHour = "TIER_0", 10000, 20000
		p.SLAPenaltyPerMinute, p.SLAThresholdMinutes = 200, 5
		p.EmployeeCostPerHour, p.InfraCostPerHour = 25, 100
	case containsAnyStr(svc, "api", "service", "gateway"):
		p.Tier, p.HourlyRevenue, p.UsersPerHour = "TIER_1", 2000, 5000
		p.SLAPenaltyPerMinute, p.SLAThresholdMinutes = 50, 10
		p.EmployeeCostPerHour, p.InfraCostPerHour = 25, 50
	case containsAnyStr(svc, "worker", "cron", "batch", "queue"):
		p.Tier, p.HourlyRevenue, p.UsersPerHour = "TIER_3", 0, 100
		p.SLAPenaltyPerMinute, p.SLAThresholdMinutes = 0, 60
		p.EmployeeCostPerHour, p.InfraCostPerHour = 25, 10
	default:
		p.Tier, p.HourlyRevenue, p.UsersPerHour = "TIER_2", 500, 1000
		p.SLAPenaltyPerMinute, p.SLAThresholdMinutes = 10, 15
		p.EmployeeCostPerHour, p.InfraCostPerHour = 25, 20
	}
}

// systemDefaultSettings returns the platform-wide multiplier defaults.
func systemDefaultSettings() *models.TenantSettings {
	return &models.TenantSettings{
		SeverityMultipliers: map[string]float64{
			"critical": 1.0,
			"high":     0.7,
			"medium":   0.4,
			"low":      0.2,
		},
		TierMultipliers: map[string]float64{
			"TIER_0": 1.5,
			"TIER_1": 1.2,
			"TIER_2": 1.0,
			"TIER_3": 0.7,
		},
		ConfidenceMode: "balanced",
	}
}

// AvailableIndustryTemplates returns the list of supported industry verticals.
func AvailableIndustryTemplates() []models.IndustryTemplateInfo {
	return []models.IndustryTemplateInfo{
		{IndustryType: "fintech", DisplayName: "Fintech / Banking", Description: "Payments, transfers, wallets, KYC — high SLA penalty, low threshold"},
		{IndustryType: "ecommerce", DisplayName: "E-Commerce / Retail", Description: "Checkout, catalog, orders — peak multipliers for sale events"},
		{IndustryType: "saas", DisplayName: "SaaS / B2B Software", Description: "APIs, auth, webhooks — uptime-critical customer-facing services"},
		{IndustryType: "healthcare", DisplayName: "Healthcare / Pharma", Description: "EHR, clinical systems — regulatory SLA penalties"},
		{IndustryType: "manufacturing", DisplayName: "Manufacturing / Logistics", Description: "SCADA, ERP, production lines — infrastructure cost focused"},
		{IndustryType: "general", DisplayName: "General / Other", Description: "Balanced defaults for any industry"},
	}
}

// containsAnyStr is a helper for substring matching used in profile heuristics.
func containsAnyStr(s string, substrs ...string) bool {
	for _, sub := range substrs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}
