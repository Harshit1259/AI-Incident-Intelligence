package config

// schema.go — Canonical environment variable contract for NeurOps Platform.
//
// This is the single source of truth for every env var the platform reads.
// It drives two things simultaneously:
//   1. The startup validation summary printed to stdout on every boot.
//   2. The GET /api/v1/config/schema endpoint operators can query at runtime.
//
// When you add, rename, or remove an env var in config.go, update this file
// first. The compiler cannot enforce that — discipline must.

// EnvRequirement classifies when a variable is needed.
type EnvRequirement string

const (
	RequiredAlways      EnvRequirement = "required"            // must be set in all environments
	RequiredInProd      EnvRequirement = "required_in_prod"    // fatal if ENV=production and missing
	Optional            EnvRequirement = "optional"            // omit freely; a default is used
	OptionalDeprecated  EnvRequirement = "optional_deprecated" // still works but discouraged
)

// EnvGroup is the logical section an env var belongs to.
type EnvGroup string

const (
	GroupCore         EnvGroup = "core"
	GroupAuth         EnvGroup = "auth"
	GroupLLM          EnvGroup = "llm"
	GroupIntegrations EnvGroup = "integrations"
	GroupAgent        EnvGroup = "agent"
	GroupAnalytics    EnvGroup = "analytics"
)

// EnvVar describes one environment variable in the platform contract.
type EnvVar struct {
	Name        string         `json:"name"`
	Group       EnvGroup       `json:"group"`
	Type        string         `json:"type"`         // "string" | "bool" | "float"
	Required    EnvRequirement `json:"required"`
	Default     string         `json:"default"`      // empty string = no default
	Description string         `json:"description"`
	Example     string         `json:"example"`
	Secret      bool           `json:"secret"`       // true = redact value in logs and schema response
}

// Schema is the top-level contract document served at /api/v1/config/schema.
type Schema struct {
	Version     string   `json:"version"`
	Description string   `json:"description"`
	EnvVars     []EnvVar `json:"env_vars"`
}

// GetSchema returns the complete, authoritative env var contract.
// This is generated from the same logic as config.Load() — if they drift,
// the startup validator will catch it.
func GetSchema() Schema {
	return Schema{
		Version:     "1",
		Description: "NeurOps Platform environment variable contract. All env vars read at startup are listed here with type, requirement level, default, and description.",
		EnvVars: []EnvVar{
			// ── Core ──────────────────────────────────────────────────────────
			{
				Name:        "ENV",
				Group:       GroupCore,
				Type:        "string",
				Required:    Optional,
				Default:     "development",
				Description: "Runtime environment. Set to 'production' to enable production mode: enforces JWT_SECRET, AGENT_SECRET_KEY, and enables stricter security checks.",
				Example:     "production",
				Secret:      false,
			},
			{
				Name:        "HTTP_PORT",
				Group:       GroupCore,
				Type:        "string",
				Required:    Optional,
				Default:     "8080",
				Description: "TCP port the HTTP server listens on.",
				Example:     "8080",
				Secret:      false,
			},
			{
				Name:        "FRONTEND_ORIGIN",
				Group:       GroupCore,
				Type:        "string",
				Required:    Optional,
				Default:     "http://localhost:5173",
				Description: "Allowed CORS origin for browser clients. Set to your frontend URL in production.",
				Example:     "https://app.example.com",
				Secret:      false,
			},
			{
				Name:        "POSTGRES_DSN",
				Group:       GroupCore,
				Type:        "string",
				Required:    RequiredAlways,
				Default:     "host=localhost port=5432 user=aiops_user password=aiops_pass dbname=aiops sslmode=disable",
				Description: "PostgreSQL connection string. Use sslmode=require in production.",
				Example:     "host=db port=5432 user=aiops password=secret dbname=aiops sslmode=require",
				Secret:      true,
			},
			// ── Auth ──────────────────────────────────────────────────────────
			{
				Name:        "JWT_SECRET",
				Group:       GroupAuth,
				Type:        "string",
				Required:    RequiredInProd,
				Default:     "",
				Description: "HMAC-SHA256 secret for signing JWT tokens. Must be 32+ random characters. In development an ephemeral secret is auto-generated (tokens reset on restart). Generate: openssl rand -base64 32",
				Example:     "$(openssl rand -base64 32)",
				Secret:      true,
			},
			// ── LLM ───────────────────────────────────────────────────────────
			{
				Name:        "LLM_API_KEY",
				Group:       GroupLLM,
				Type:        "string",
				Required:    Optional,
				Default:     "",
				Description: "API key for the LLM provider. For OpenAI: sk-proj-... For Anthropic: sk-ant-... Auto-detected from key prefix when LLM_PROVIDER is not set. Leave empty to use offline/rule-based fallback.",
				Example:     "sk-proj-REPLACE_WITH_YOUR_KEY",
				Secret:      true,
			},
			{
				Name:        "LLM_PROVIDER",
				Group:       GroupLLM,
				Type:        "string",
				Required:    Optional,
				Default:     "",
				Description: "LLM provider override. Values: 'openai' | 'anthropic' | 'local'. Auto-detected from LLM_API_KEY prefix when empty.",
				Example:     "anthropic",
				Secret:      false,
			},
			{
				Name:        "LLM_MODEL",
				Group:       GroupLLM,
				Type:        "string",
				Required:    Optional,
				Default:     "gpt-4o (openai) | claude-sonnet-4-6 (anthropic) | llama3 (local)",
				Description: "Model name to use. Defaults to a sensible model per provider.",
				Example:     "gpt-4o",
				Secret:      false,
			},
			{
				Name:        "LLM_DATA_MODE",
				Group:       GroupLLM,
				Type:        "string",
				Required:    Optional,
				Default:     "cloud",
				Description: "Controls data routing. 'cloud' sends to public APIs (PII is scrubbed). 'private' routes to LLM_BASE_URL only. 'offline' disables all LLM calls and uses rule-based templates.",
				Example:     "private",
				Secret:      false,
			},
			{
				Name:        "LLM_BASE_URL",
				Group:       GroupLLM,
				Type:        "string",
				Required:    Optional,
				Default:     "",
				Description: "Custom LLM endpoint for BYOC or on-prem deployments (Ollama, vLLM, LM Studio). Required when LLM_DATA_MODE=private or LLM_PROVIDER=local.",
				Example:     "http://ollama:11434",
				Secret:      false,
			},
			// ── Integrations ──────────────────────────────────────────────────
			{
				Name:        "SLACK_BOT_TOKEN",
				Group:       GroupIntegrations,
				Type:        "string",
				Required:    Optional,
				Default:     "",
				Description: "Slack Bot OAuth token (xoxb-...). Enables incident channel creation and alert notifications.",
				Example:     "xoxb-REPLACE_IF_USING_SLACK",
				Secret:      true,
			},
			{
				Name:        "SLACK_SIGNING_SECRET",
				Group:       GroupIntegrations,
				Type:        "string",
				Required:    Optional,
				Default:     "",
				Description: "Slack signing secret for verifying incoming webhook payloads.",
				Example:     "REPLACE_IF_USING_SLACK",
				Secret:      true,
			},
			{
				Name:        "GITHUB_WEBHOOK_SECRET",
				Group:       GroupIntegrations,
				Type:        "string",
				Required:    Optional,
				Default:     "",
				Description: "HMAC secret for verifying GitHub webhook signatures (X-Hub-Signature-256).",
				Example:     "REPLACE_IF_USING_GITHUB",
				Secret:      true,
			},
			{
				Name:        "GITLAB_WEBHOOK_TOKEN",
				Group:       GroupIntegrations,
				Type:        "string",
				Required:    Optional,
				Default:     "",
				Description: "Token for verifying GitLab webhook requests (X-Gitlab-Token header).",
				Example:     "REPLACE_IF_USING_GITLAB",
				Secret:      true,
			},
			{
				Name:        "DATADOG_API_KEY",
				Group:       GroupIntegrations,
				Type:        "string",
				Required:    Optional,
				Default:     "",
				Description: "Datadog API key for sending metrics and events to Datadog.",
				Example:     "REPLACE_IF_USING_DATADOG",
				Secret:      true,
			},
			{
				Name:        "DATADOG_WEBHOOK_SECRET",
				Group:       GroupIntegrations,
				Type:        "string",
				Required:    Optional,
				Default:     "",
				Description: "Shared secret sent in X-Datadog-Webhook-Token for inbound Datadog custom webhooks. Leave empty to skip token validation.",
				Example:     "REPLACE_IF_USING_DATADOG",
				Secret:      true,
			},
			{
				Name:        "PAGERDUTY_WEBHOOK_SECRET",
				Group:       GroupIntegrations,
				Type:        "string",
				Required:    Optional,
				Default:     "",
				Description: "HMAC secret for verifying PagerDuty webhook signatures (X-PagerDuty-Signature v3). Configure in PagerDuty → Webhooks → Secret.",
				Example:     "REPLACE_IF_USING_PAGERDUTY",
				Secret:      true,
			},
			{
				Name:        "WHATSAPP_PHONE_NUMBER_ID",
				Group:       GroupIntegrations,
				Type:        "string",
				Required:    Optional,
				Default:     "",
				Description: "WhatsApp Business API phone number ID for sending alert notifications.",
				Example:     "123456789",
				Secret:      false,
			},
			{
				Name:        "WHATSAPP_ACCESS_TOKEN",
				Group:       GroupIntegrations,
				Type:        "string",
				Required:    Optional,
				Default:     "",
				Description: "WhatsApp Business API access token.",
				Example:     "EAABxx...",
				Secret:      true,
			},
			{
				Name:        "WHATSAPP_VERIFY_TOKEN",
				Group:       GroupIntegrations,
				Type:        "string",
				Required:    Optional,
				Default:     "",
				Description: "WhatsApp webhook verification token set in Meta Business Manager.",
				Example:     "my-verify-token",
				Secret:      true,
			},
			// ── Analytics ─────────────────────────────────────────────────────
			{
				Name:        "SRE_HOURLY_COST",
				Group:       GroupAnalytics,
				Type:        "float",
				Required:    Optional,
				Default:     "150",
				Description: "Average SRE hourly cost in USD, used for ROI calculations in the engineering health dashboard.",
				Example:     "150",
				Secret:      false,
			},
			{
				Name:        "DIGEST_EMAIL",
				Group:       GroupAnalytics,
				Type:        "string",
				Required:    Optional,
				Default:     "",
				Description: "Email address to receive weekly incident digest reports.",
				Example:     "oncall@example.com",
				Secret:      false,
			},
			// ── Agent ─────────────────────────────────────────────────────────
			{
				Name:        "AGENT_INGEST_ENABLED",
				Group:       GroupAgent,
				Type:        "bool",
				Required:    Optional,
				Default:     "true",
				Description: "Enable the NeuroOps agent ingest pipeline. Set to false to disable all agent data ingestion.",
				Example:     "true",
				Secret:      false,
			},
			{
				Name:        "AGENT_SECRET_KEY",
				Group:       GroupAgent,
				Type:        "string",
				Required:    RequiredInProd,
				Default:     "",
				Description: "AES-256 encryption key protecting per-agent HMAC signing secrets at rest. Must be 32+ random characters in production. Generate: openssl rand -base64 32",
				Example:     "$(openssl rand -base64 32)",
				Secret:      true,
			},
			{
				Name:        "AGENT_INGEST_TOKEN",
				Group:       GroupAgent,
				Type:        "string",
				Required:    OptionalDeprecated,
				Default:     "",
				Description: "Legacy shared token for agent ingest (dev fallback only). Leave empty to enforce per-agent HMAC identity everywhere. Do not use in production.",
				Example:     "",
				Secret:      true,
			},
			// ── Edition gating ────────────────────────────────────────────────
			{
				Name:        "PLATFORM_EDITIONS",
				Group:       GroupCore,
				Type:        "string",
				Required:    Optional,
				Default:     "",
				Description: "Comma-separated list of active product editions. Unlisted editions have their HTTP routes completely disabled at startup. Empty value enables all editions (backward compatible). Valid tokens: core, enterprise, agent, saasops.",
				Example:     "core,enterprise",
				Secret:      false,
			},
		},
	}
}
