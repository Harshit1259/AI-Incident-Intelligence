package config

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"log"
	"os"
	"strconv"
)

type Config struct {
	HTTPPort       string
	FrontendOrigin string
	PostgresDSN    string

	// LLM — provider-agnostic (openai | anthropic | local)
	LLMAPIKey   string
	LLMModel    string
	LLMProvider string   // "openai" | "anthropic" | "local" — auto-detected from key if empty
	LLMBaseURL  string   // custom endpoint for BYOC/on-prem (e.g. http://ollama:11434)
	LLMDataMode string   // "cloud" | "private" | "offline" — default: cloud

	// Week 3 — JWT auth (PRODUCTION SECURE)
	JWTSecret string

	// Phase 2 — Integrations
	SlackBotToken      string
	SlackSigningSecret string

	// Phase 3 — Engineering Health & ROI
	SREHourlyCost float64
	DigestEmail   string

	// Gap features — WhatsApp
	WhatsAppPhoneNumberID string
	WhatsAppAccessToken   string
	WhatsAppVerifyToken   string

	// NeuroOps agent integration
	AgentIngestEnabled bool
	AgentIngestToken   string // legacy shared secret; used as dev-mode fallback only
	AgentSecretKey     string // AES-256 encryption key for per-agent secrets (32+ chars, required in production)
	IsProd             bool   // true when ENV=production (canonical). PRODUCTION=true is accepted for backward compat.

	// Stripe billing — required in production
	StripeSecretKey     string // STRIPE_SECRET_KEY — sk_live_... (fatal if empty in prod)
	StripeWebhookSecret string // STRIPE_WEBHOOK_SECRET — used to verify Stripe webhook signatures
	StripePriceStarter  string // STRIPE_PRICE_STARTER — Stripe Price ID for the starter tier
	StripePriceGrowth   string // STRIPE_PRICE_GROWTH  — Stripe Price ID for the growth tier
	StripePriceScale    string // STRIPE_PRICE_SCALE   — Stripe Price ID for the scale tier

	// Edition gating — comma-separated: "core,enterprise,agent,saasops"
	// Empty string (default) enables all editions for backward compatibility.
	PlatformEditions string

	// Team Workflow Primitives — Feature 8
	JiraBaseURL    string // e.g. "https://your-org.atlassian.net"
	JiraEmail      string
	JiraAPIToken   string
	JiraProjectKey string // e.g. "OPS"
	// ServiceNow
	ServiceNowURL      string // e.g. "https://your-instance.service-now.com"
	ServiceNowUser     string
	ServiceNowPassword string
	// Microsoft Teams
	TeamsWebhookURL string // Incoming Webhook URL for posting to Teams channel

	// SaaS multi-tenancy — Feature B1
	// 32-byte hex key used to derive per-tenant AES-256-GCM keys.
	// When empty, field-level encryption is disabled (passthrough).
	MasterEncryptionKey string

	// Multi-Region Data Residency — SaaS Feature 2
	// DATA_REGION identifies which geographic region this deployment serves.
	// Valid values: "us" (default) | "eu" | "apac"
	// EU deployments enforce GDPR data residency: cross-region requests are rejected 403.
	DataRegion string
}

func Load() Config {
	// Resolve production mode once so we never check multiple env vars repeatedly
	// and the deprecation warning fires exactly once per startup.
	isProd := isProdEnv()
	jwtSecret := getJWTSecret(isProd)
	agentSecretKey := getAgentSecretKey(isProd)
	stripeSecretKey := getStripeSecretKey(isProd)

	return Config{
		HTTPPort:       getEnv("HTTP_PORT", "8080"),
		FrontendOrigin: getEnv("FRONTEND_ORIGIN", "http://localhost:5173"),
		PostgresDSN: getEnv(
			"POSTGRES_DSN",
			"host=localhost port=5432 user=aiops_user password=aiops_pass dbname=aiops sslmode=disable",
		),

		LLMAPIKey:   getEnv("LLM_API_KEY", ""),
		LLMModel:    getEnv("LLM_MODEL", ""),
		LLMProvider: getEnv("LLM_PROVIDER", ""),
		LLMBaseURL:  getEnv("LLM_BASE_URL", ""),
		LLMDataMode: getEnv("LLM_DATA_MODE", "cloud"),

		JWTSecret: jwtSecret,

		SlackBotToken:      getEnv("SLACK_BOT_TOKEN", ""),
		SlackSigningSecret: getEnv("SLACK_SIGNING_SECRET", ""),

		SREHourlyCost: getEnvFloat("SRE_HOURLY_COST", 150.0),
		DigestEmail:   getEnv("DIGEST_EMAIL", ""),

		WhatsAppPhoneNumberID: getEnv("WHATSAPP_PHONE_NUMBER_ID", ""),
		WhatsAppAccessToken:   getEnv("WHATSAPP_ACCESS_TOKEN", ""),
		WhatsAppVerifyToken:   getEnv("WHATSAPP_VERIFY_TOKEN", ""),

		AgentIngestEnabled: getEnv("AGENT_INGEST_ENABLED", "true") == "true",
		AgentIngestToken:   getEnv("AGENT_INGEST_TOKEN", ""),
		AgentSecretKey:     agentSecretKey,
		IsProd:             isProd,

		StripeSecretKey:     stripeSecretKey,
		StripeWebhookSecret: getEnv("STRIPE_WEBHOOK_SECRET", ""),
		StripePriceStarter:  getEnv("STRIPE_PRICE_STARTER", ""),
		StripePriceGrowth:   getEnv("STRIPE_PRICE_GROWTH", ""),
		StripePriceScale:    getEnv("STRIPE_PRICE_SCALE", ""),

		PlatformEditions: getEnv("PLATFORM_EDITIONS", ""),

		JiraBaseURL:    getEnv("JIRA_BASE_URL", ""),
		JiraEmail:      getEnv("JIRA_EMAIL", ""),
		JiraAPIToken:   getEnv("JIRA_API_TOKEN", ""),
		JiraProjectKey: getEnv("JIRA_PROJECT_KEY", "OPS"),

		ServiceNowURL:      getEnv("SERVICENOW_URL", ""),
		ServiceNowUser:     getEnv("SERVICENOW_USER", ""),
		ServiceNowPassword: getEnv("SERVICENOW_PASSWORD", ""),

		TeamsWebhookURL: getEnv("TEAMS_WEBHOOK_URL", ""),

		MasterEncryptionKey: getEnv("MASTER_ENCRYPTION_KEY", ""),

		DataRegion: getEnv("DATA_REGION", "us"),
	}
}

// isProdEnv returns true when the runtime environment is production.
// Canonical check: ENV=production
// Backward-compat: PRODUCTION=true (deprecated — migrate to ENV=production)
func isProdEnv() bool {
	if os.Getenv("ENV") == "production" {
		return true
	}
	if os.Getenv("PRODUCTION") == "true" {
		log.Printf("config: DEPRECATION WARNING — PRODUCTION=true is deprecated. Use ENV=production instead.")
		return true
	}
	return false
}

// getJWTSecret returns a validated JWT secret.
// In production (ENV=production), a missing or weak secret is fatal.
func getJWTSecret(isProd bool) string {
	secret := os.Getenv("JWT_SECRET")

	if secret != "" && len(secret) >= 32 {
		return secret
	}

	if isProd {
		log.Fatal("FATAL: JWT_SECRET must be set to a 32+ character random string in production. " +
			"Generate one with: openssl rand -base64 32")
	}

	// Dev/test only: generate ephemeral secret (tokens invalidated on restart)
	secretBytes := make([]byte, 32)
	if _, err := rand.Read(secretBytes); err != nil {
		h := sha256.New()
		h.Write([]byte("aiops-platform-dev"))
		fmt.Fprintf(h, "%d", os.Getpid())
		secretBytes = h.Sum(nil)[:32]
	}
	jwtSecret := base64.RawURLEncoding.EncodeToString(secretBytes)
	log.Printf("WARNING: JWT_SECRET not set — using ephemeral secret. Tokens will be invalidated on restart. Set JWT_SECRET in production.")
	return jwtSecret
}

func (config Config) HTTPAddress() string {
	return fmt.Sprintf(":%s", config.HTTPPort)
}

// LLMEnabled returns true when LLM is usable — either an API key is set, or using a local endpoint.
func (config Config) LLMEnabled() bool {
	return config.LLMAPIKey != "" || config.LLMBaseURL != ""
}

// SlackEnabled returns true when a Slack bot token is configured.
func (c Config) SlackEnabled() bool { return c.SlackBotToken != "" }

func getEnv(key, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}

// getAgentSecretKey returns the agent secret key with production enforcement.
// In production, a missing or short key is fatal — a weak key breaks agent auth.
func getAgentSecretKey(isProd bool) string {
	key := os.Getenv("AGENT_SECRET_KEY")
	if isProd && len(key) < 32 {
		log.Fatal("FATAL: AGENT_SECRET_KEY must be set to a 32+ character random string in production. " +
			"Generate one with: openssl rand -base64 32")
	}
	return key
}

// getStripeSecretKey returns the Stripe secret key with production enforcement.
// In production, billing without Stripe silently creates fake subscriptions — fatal.
func getStripeSecretKey(isProd bool) string {
	key := os.Getenv("STRIPE_SECRET_KEY")
	if isProd && key == "" {
		log.Fatal("FATAL: STRIPE_SECRET_KEY must be set in production. " +
			"Obtain it from https://dashboard.stripe.com/apikeys")
	}
	return key
}

func getEnvFloat(key string, fallback float64) float64 {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	f, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return fallback
	}
	return f
}
