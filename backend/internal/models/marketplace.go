package models

import "time"

// IntegrationCategory groups integrations for the marketplace UI.
type IntegrationCategory string

const (
	CategoryMonitoring    IntegrationCategory = "monitoring"
	CategoryTicketing     IntegrationCategory = "ticketing"
	CategoryCommunication IntegrationCategory = "communication"
	CategoryCICD          IntegrationCategory = "cicd"
	CategoryCloud         IntegrationCategory = "cloud"
	CategoryAPM           IntegrationCategory = "apm"
	CategoryLogging       IntegrationCategory = "logging"
	CategorySecurity      IntegrationCategory = "security"
)

// IntegrationAuthMethod describes how the integration authenticates.
type IntegrationAuthMethod string

const (
	AuthWebhook         IntegrationAuthMethod = "webhook"          // we receive; they send
	AuthAPIKey          IntegrationAuthMethod = "api_key"          // we store their API key
	AuthBasicAuth       IntegrationAuthMethod = "basic_auth"       // username + password
	AuthBotToken        IntegrationAuthMethod = "bot_token"        // Slack-style bot token
	AuthIncomingWebhook IntegrationAuthMethod = "incoming_webhook" // Teams/Discord webhook URL
	AuthBearerToken     IntegrationAuthMethod = "bearer_token"     // OAuth bearer
	AuthNone            IntegrationAuthMethod = "none"             // public endpoint
)

// TestStrategy defines how a connection test is performed.
type TestStrategy string

const (
	TestValidateConfig  TestStrategy = "validate_config"  // check required fields are present
	TestInboundWebhook  TestStrategy = "inbound_webhook"  // return webhook URL + instructions
	TestOutboundJira    TestStrategy = "outbound_jira"    // live Jira API probe
	TestOutboundSnow    TestStrategy = "outbound_snow"    // live ServiceNow API probe
	TestOutboundSlack   TestStrategy = "outbound_slack"   // live Slack auth.test probe
	TestOutboundTeams   TestStrategy = "outbound_teams"   // validate webhook URL format
)

// IntegrationConfigField describes one configuration field shown in the enable modal.
type IntegrationConfigField struct {
	Name        string   `json:"name"`
	Label       string   `json:"label"`
	Type        string   `json:"type"`        // "text" | "password" | "url" | "select"
	Required    bool     `json:"required"`
	Placeholder string   `json:"placeholder"`
	Options     []string `json:"options,omitempty"` // for "select" type
	HelpText    string   `json:"help_text,omitempty"`
}

// IntegrationEntry is the static catalog definition for one integration.
// These are product-defined and never change per-tenant.
type IntegrationEntry struct {
	ID            string                   `json:"id"`
	Name          string                   `json:"name"`
	Category      IntegrationCategory      `json:"category"`
	Description   string                   `json:"description"`
	AuthMethod    IntegrationAuthMethod    `json:"auth_method"`
	Inbound       bool                     `json:"inbound"`
	Outbound      bool                     `json:"outbound"`
	WebhookPath   string                   `json:"webhook_path,omitempty"` // path for inbound integrations
	ConfigFields  []IntegrationConfigField `json:"config_fields"`
	SamplePayload string                   `json:"sample_payload,omitempty"`
	TestStrategy  TestStrategy             `json:"test_strategy"`
	Popular       bool                     `json:"popular"`
	Tags          []string                 `json:"tags"`
	DocsHint      string                   `json:"docs_hint,omitempty"`
}

// RoutingConfig defines per-tenant alert filtering for an integration.
type RoutingConfig struct {
	Severities []string `json:"severities,omitempty"` // e.g. ["critical","high"]
	EventTypes []string `json:"event_types,omitempty"`
	Services   []string `json:"services,omitempty"`
}

// MarketplaceConfig is the DB-backed per-tenant state for one integration.
type MarketplaceConfig struct {
	TenantID      string            `json:"tenant_id"`
	IntegrationID string            `json:"integration_id"`
	Enabled       bool              `json:"enabled"`
	AuthConfig    map[string]string `json:"auth_config,omitempty"` // sensitive — omit in list views
	RoutingConfig RoutingConfig     `json:"routing_config"`
	SourceID      string            `json:"source_id,omitempty"` // linked source_registry entry
	LastTestedAt  *time.Time        `json:"last_tested_at,omitempty"`
	TestStatus    string            `json:"test_status"`    // "ok" | "error" | "config_ready" | ""
	TestMessage   string            `json:"test_message"`
	EnabledAt     *time.Time        `json:"enabled_at,omitempty"`
	CreatedAt     time.Time         `json:"created_at"`
	UpdatedAt     time.Time         `json:"updated_at"`
}

// MarketplaceEntry is what the API returns: catalog entry merged with tenant state.
type MarketplaceEntry struct {
	IntegrationEntry
	Enabled       bool          `json:"enabled"`
	TestStatus    string        `json:"test_status"`
	TestMessage   string        `json:"test_message"`
	LastTestedAt  *time.Time    `json:"last_tested_at,omitempty"`
	RoutingConfig RoutingConfig `json:"routing_config"`
	WebhookURL    string        `json:"webhook_url,omitempty"` // set only when inbound + enabled
	SourceID      string        `json:"source_id,omitempty"`
}

// ConnectionTestResult is returned by the test-connection endpoint.
type ConnectionTestResult struct {
	Status     string     `json:"status"`  // "ok" | "error" | "config_ready"
	Message    string     `json:"message"`
	WebhookURL string     `json:"webhook_url,omitempty"`
	TestedAt   time.Time  `json:"tested_at"`
}

// EnableRequest is the request body for enabling an integration.
type EnableRequest struct {
	AuthConfig    map[string]string `json:"auth_config"`
	RoutingConfig RoutingConfig     `json:"routing_config"`
}
