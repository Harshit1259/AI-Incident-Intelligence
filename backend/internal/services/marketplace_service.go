package services

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"ai-incident-platform/backend/internal/models"
	"ai-incident-platform/backend/internal/store"
)

// marketplaceEventProcessor is the minimal interface needed to inject synthetic
// test alerts from the marketplace "Send Test Alert" button.
type marketplaceEventProcessor interface {
	ProcessEvent(event models.Event) bool
}

// MarketplaceService orchestrates the Integration Hub: catalog browsing,
// enable/disable, connection testing, and test-alert injection.
type MarketplaceService struct {
	store          *store.MarketplaceStore
	sourceRegistry *SourceRegistryService
	httpClient     *http.Client
	eventProc      marketplaceEventProcessor
	baseURL        string // server base URL, used to build inbound webhook URLs
}

func NewMarketplaceService(
	ms *store.MarketplaceStore,
	srs *SourceRegistryService,
	baseURL string,
) *MarketplaceService {
	return &MarketplaceService{
		store:          ms,
		sourceRegistry: srs,
		httpClient:     &http.Client{Timeout: 10 * time.Second},
		baseURL:        baseURL,
	}
}

// SetEventProcessor wires in the correlation service for test-alert injection.
func (svc *MarketplaceService) SetEventProcessor(ep marketplaceEventProcessor) {
	svc.eventProc = ep
}

// ListCatalog returns the full 53-integration catalog, overlaid with per-tenant state.
// Integrations the tenant has configured appear with enabled=true and test status.
func (svc *MarketplaceService) ListCatalog(tenantID string) ([]models.MarketplaceEntry, error) {
	configs, err := svc.store.ListEnabled(tenantID)
	if err != nil {
		return nil, fmt.Errorf("marketplace: list configs: %w", err)
	}

	// Build a fast lookup: integrationID → config.
	byID := make(map[string]models.MarketplaceConfig, len(configs))
	for _, c := range configs {
		byID[c.IntegrationID] = c
	}

	entries := allCatalogEntries()
	result := make([]models.MarketplaceEntry, 0, len(entries))
	for _, e := range entries {
		me := models.MarketplaceEntry{IntegrationEntry: e}
		if cfg, ok := byID[e.ID]; ok {
			me.Enabled = cfg.Enabled
			me.TestStatus = cfg.TestStatus
			me.TestMessage = cfg.TestMessage
			me.LastTestedAt = cfg.LastTestedAt
			me.RoutingConfig = cfg.RoutingConfig
			me.SourceID = cfg.SourceID
			if cfg.Enabled && e.Inbound {
				me.WebhookURL = svc.webhookURLFor(e, cfg.SourceID)
			}
		}
		// Strip sample payload from list view — available on /api/v1/marketplace/{id}
		me.SamplePayload = ""
		result = append(result, me)
	}
	return result, nil
}

// GetIntegration returns a single integration entry with per-tenant state and sample payload.
func (svc *MarketplaceService) GetIntegration(tenantID, integrationID string) (*models.MarketplaceEntry, error) {
	entry, ok := getCatalogEntry(integrationID)
	if !ok {
		return nil, fmt.Errorf("integration %q not found", integrationID)
	}

	me := &models.MarketplaceEntry{IntegrationEntry: *entry}

	cfg, err := svc.store.Get(tenantID, integrationID)
	if err != nil {
		return nil, fmt.Errorf("marketplace: get config: %w", err)
	}
	if cfg != nil {
		me.Enabled = cfg.Enabled
		me.TestStatus = cfg.TestStatus
		me.TestMessage = cfg.TestMessage
		me.LastTestedAt = cfg.LastTestedAt
		me.RoutingConfig = cfg.RoutingConfig
		me.SourceID = cfg.SourceID
		if cfg.Enabled && entry.Inbound {
			me.WebhookURL = svc.webhookURLFor(*entry, cfg.SourceID)
		}
	}
	return me, nil
}

// Enable activates an integration for a tenant with the provided configuration.
// For inbound integrations a source_registry entry is created (or reused) to
// generate a secure ingest token and webhook URL.
func (svc *MarketplaceService) Enable(
	tenantID, integrationID string,
	req models.EnableRequest,
) (*models.MarketplaceEntry, error) {
	entry, ok := getCatalogEntry(integrationID)
	if !ok {
		return nil, fmt.Errorf("integration %q not found", integrationID)
	}

	// Validate required config fields.
	if err := validateConfigFields(entry.ConfigFields, req.AuthConfig); err != nil {
		return nil, err
	}

	cfg := models.MarketplaceConfig{
		TenantID:      tenantID,
		IntegrationID: integrationID,
		Enabled:       true,
		AuthConfig:    req.AuthConfig,
		RoutingConfig: req.RoutingConfig,
		TestStatus:    "",
	}

	// For inbound integrations: create a source_registry entry for the ingest token.
	if entry.Inbound {
		existing, _ := svc.store.Get(tenantID, integrationID)
		if existing != nil && existing.SourceID != "" {
			cfg.SourceID = existing.SourceID
		} else {
			src := svc.sourceRegistry.CreateSource(tenantID, entry.Name, integrationID)
			cfg.SourceID = src.ID
		}
	}

	now := time.Now()
	cfg.EnabledAt = &now

	if err := svc.store.Upsert(cfg); err != nil {
		return nil, fmt.Errorf("marketplace: save config: %w", err)
	}

	return svc.GetIntegration(tenantID, integrationID)
}

// Disable deactivates an integration. The config is retained for re-enable.
func (svc *MarketplaceService) Disable(tenantID, integrationID string) error {
	_, ok := getCatalogEntry(integrationID)
	if !ok {
		return fmt.Errorf("integration %q not found", integrationID)
	}

	cfg, err := svc.store.Get(tenantID, integrationID)
	if err != nil {
		return fmt.Errorf("marketplace: get config: %w", err)
	}
	if cfg == nil {
		return nil // nothing to disable
	}

	cfg.Enabled = false
	return svc.store.Upsert(*cfg)
}

// TestConnection validates or probes an integration's connectivity.
// Strategy depends on the integration's TestStrategy field.
func (svc *MarketplaceService) TestConnection(tenantID, integrationID string) (*models.ConnectionTestResult, error) {
	entry, ok := getCatalogEntry(integrationID)
	if !ok {
		return nil, fmt.Errorf("integration %q not found", integrationID)
	}

	cfg, err := svc.store.Get(tenantID, integrationID)
	if err != nil {
		return nil, fmt.Errorf("marketplace: get config: %w", err)
	}

	authConfig := map[string]string{}
	if cfg != nil {
		authConfig = cfg.AuthConfig
	}

	result := &models.ConnectionTestResult{TestedAt: time.Now()}

	switch entry.TestStrategy {
	case models.TestInboundWebhook:
		result.Status = "ok"
		result.Message = fmt.Sprintf("Send a %s webhook to the URL below to start receiving alerts.", entry.Name)
		if cfg != nil && cfg.SourceID != "" {
			result.WebhookURL = svc.webhookURLFor(*entry, cfg.SourceID)
		} else {
			result.WebhookURL = svc.baseURL + entry.WebhookPath
		}

	case models.TestOutboundJira:
		result = svc.testJira(authConfig)

	case models.TestOutboundSnow:
		result = svc.testServiceNow(authConfig)

	case models.TestOutboundSlack:
		result = svc.testSlack(authConfig)

	case models.TestOutboundTeams:
		result = svc.testTeams(authConfig)

	default: // TestValidateConfig
		if err := validateConfigFields(entry.ConfigFields, authConfig); err != nil {
			result.Status = "error"
			result.Message = err.Error()
		} else {
			result.Status = "config_ready"
			result.Message = "All required configuration fields are present. Enable to activate."
		}
	}

	// Persist the test result.
	_ = svc.store.UpdateTestResult(tenantID, integrationID, result.Status, result.Message)
	return result, nil
}

// SendTestAlert injects a synthetic alert through the full correlation pipeline.
// This lets the user confirm their integration is live without waiting for a real incident.
func (svc *MarketplaceService) SendTestAlert(tenantID, integrationID string) error {
	if svc.eventProc == nil {
		return fmt.Errorf("event processor not configured")
	}

	entry, ok := getCatalogEntry(integrationID)
	if !ok {
		return fmt.Errorf("integration %q not found", integrationID)
	}

	now := time.Now()
	event := models.Event{
		ID:          fmt.Sprintf("test-mkt-%d", now.UnixNano()),
		TenantID:    tenantID,
		Source:      integrationID,
		ExternalID:  fmt.Sprintf("test-%s-%d", integrationID, now.UnixNano()),
		Service:     "test-service",
		Resource:    "test-service",
		Environment: "production",
		Severity:    "high",
		Type:        "alert",
		Title:       fmt.Sprintf("[Test] %s integration test alert", entry.Name),
		Message:     fmt.Sprintf("Test alert sent from the %s integration in the NeuroOps Marketplace. This is a synthetic event.", entry.Name),
		Labels:      map[string]string{"source": integrationID, "synthetic": "true", "integration": integrationID},
		Timestamp:   now,
		IngestSchema: "webhook",
	}

	go svc.eventProc.ProcessEvent(event)
	return nil
}

// GetSamplePayload returns the raw sample webhook payload for an integration.
func (svc *MarketplaceService) GetSamplePayload(integrationID string) (string, error) {
	entry, ok := getCatalogEntry(integrationID)
	if !ok {
		return "", fmt.Errorf("integration %q not found", integrationID)
	}
	if entry.SamplePayload == "" {
		return "{}", nil
	}
	return entry.SamplePayload, nil
}

// ── Connection probes ─────────────────────────────────────────────────────────

func (svc *MarketplaceService) testJira(auth map[string]string) *models.ConnectionTestResult {
	baseURL := auth["base_url"]
	email := auth["email"]
	token := auth["api_token"]
	if baseURL == "" || email == "" || token == "" {
		return &models.ConnectionTestResult{
			Status:   "error",
			Message:  "base_url, email, and api_token are required",
			TestedAt: time.Now(),
		}
	}

	req, err := http.NewRequest(http.MethodGet, strings.TrimRight(baseURL, "/")+"/rest/api/2/serverInfo", nil)
	if err != nil {
		return &models.ConnectionTestResult{Status: "error", Message: err.Error(), TestedAt: time.Now()}
	}
	req.SetBasicAuth(email, token)

	resp, err := svc.httpClient.Do(req)
	if err != nil {
		return &models.ConnectionTestResult{Status: "error", Message: "cannot reach Jira: " + err.Error(), TestedAt: time.Now()}
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		return &models.ConnectionTestResult{Status: "ok", Message: "Jira connection successful", TestedAt: time.Now()}
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
	return &models.ConnectionTestResult{
		Status:   "error",
		Message:  fmt.Sprintf("Jira returned HTTP %d: %s", resp.StatusCode, string(body)),
		TestedAt: time.Now(),
	}
}

func (svc *MarketplaceService) testServiceNow(auth map[string]string) *models.ConnectionTestResult {
	instanceURL := auth["instance_url"]
	username := auth["username"]
	password := auth["password"]
	if instanceURL == "" || username == "" || password == "" {
		return &models.ConnectionTestResult{
			Status:   "error",
			Message:  "instance_url, username, and password are required",
			TestedAt: time.Now(),
		}
	}

	url := strings.TrimRight(instanceURL, "/") + "/api/now/table/incident?sysparm_limit=1&sysparm_fields=number"
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return &models.ConnectionTestResult{Status: "error", Message: err.Error(), TestedAt: time.Now()}
	}
	req.SetBasicAuth(username, password)
	req.Header.Set("Accept", "application/json")

	resp, err := svc.httpClient.Do(req)
	if err != nil {
		return &models.ConnectionTestResult{Status: "error", Message: "cannot reach ServiceNow: " + err.Error(), TestedAt: time.Now()}
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		return &models.ConnectionTestResult{Status: "ok", Message: "ServiceNow connection successful", TestedAt: time.Now()}
	}
	return &models.ConnectionTestResult{
		Status:   "error",
		Message:  fmt.Sprintf("ServiceNow returned HTTP %d — check credentials", resp.StatusCode),
		TestedAt: time.Now(),
	}
}

func (svc *MarketplaceService) testSlack(auth map[string]string) *models.ConnectionTestResult {
	token := auth["bot_token"]
	if token == "" {
		return &models.ConnectionTestResult{Status: "error", Message: "bot_token is required", TestedAt: time.Now()}
	}

	body, _ := json.Marshal(map[string]string{})
	req, err := http.NewRequest(http.MethodPost, "https://slack.com/api/auth.test", bytes.NewReader(body))
	if err != nil {
		return &models.ConnectionTestResult{Status: "error", Message: err.Error(), TestedAt: time.Now()}
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := svc.httpClient.Do(req)
	if err != nil {
		return &models.ConnectionTestResult{Status: "error", Message: "cannot reach Slack: " + err.Error(), TestedAt: time.Now()}
	}
	defer resp.Body.Close()

	var result struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
		Team  string `json:"team"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil || !result.OK {
		msg := "Slack auth failed"
		if result.Error != "" {
			msg = "Slack: " + result.Error
		}
		return &models.ConnectionTestResult{Status: "error", Message: msg, TestedAt: time.Now()}
	}
	return &models.ConnectionTestResult{
		Status:   "ok",
		Message:  fmt.Sprintf("Slack connection successful — workspace: %s", result.Team),
		TestedAt: time.Now(),
	}
}

func (svc *MarketplaceService) testTeams(auth map[string]string) *models.ConnectionTestResult {
	webhookURL := auth["webhook_url"]
	if webhookURL == "" {
		return &models.ConnectionTestResult{Status: "error", Message: "webhook_url is required", TestedAt: time.Now()}
	}
	if !strings.Contains(webhookURL, "webhook.office.com") && !strings.Contains(webhookURL, "teams.microsoft.com") {
		return &models.ConnectionTestResult{
			Status:   "error",
			Message:  "webhook_url does not look like a Teams Incoming Webhook URL",
			TestedAt: time.Now(),
		}
	}
	return &models.ConnectionTestResult{
		Status:   "config_ready",
		Message:  "Teams webhook URL format is valid. Enable the integration to start receiving incident notifications.",
		TestedAt: time.Now(),
	}
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func (svc *MarketplaceService) webhookURLFor(entry models.IntegrationEntry, sourceID string) string {
	base := strings.TrimRight(svc.baseURL, "/")
	// Integrations with dedicated handlers keep their path; generic ones use
	// the standard webhook ingest path.
	path := entry.WebhookPath
	if path == "" {
		path = "/api/v1/ingest/webhook"
	}
	if sourceID != "" {
		return base + path + "?source_id=" + sourceID
	}
	return base + path
}

func validateConfigFields(fields []models.IntegrationConfigField, auth map[string]string) error {
	for _, f := range fields {
		if f.Required && strings.TrimSpace(auth[f.Name]) == "" {
			return fmt.Errorf("field %q is required", f.Label)
		}
	}
	return nil
}
