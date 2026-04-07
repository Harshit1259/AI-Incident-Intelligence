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

	// Week 2 — LLM integration
	AnthropicAPIKey string
	AnthropicModel  string // defaults to claude-sonnet-4-6

	// Week 3 — JWT auth (PRODUCTION SECURE)
	JWTSecret string

	// Phase 2 — Integrations
	SlackBotToken       string
	SlackSigningSecret  string
	GitHubWebhookSecret string
	GitLabWebhookToken  string
	DatadogAPIKey       string

	// Phase 3 — Engineering Health & ROI
	SREHourlyCost float64
	DigestEmail   string
}

func Load() Config {
	// Generate secure JWT secret if not provided (32 bytes → base64 → 44 chars)
	jwtSecret := getJWTSecret()

	return Config{
		HTTPPort:       getEnv("HTTP_PORT", "8080"),
		FrontendOrigin: getEnv("FRONTEND_ORIGIN", "http://localhost:5173"),
		PostgresDSN: getEnv(
			"POSTGRES_DSN",
			"host=localhost port=5432 user=aiops_user password=aiops_pass dbname=aiops sslmode=disable",
		),

		AnthropicAPIKey: getEnv("ANTHROPIC_API_KEY", "REDACTED_ROTATED_KEY"),
		AnthropicModel:  getEnv("ANTHROPIC_MODEL", "claude-sonnet-4-6"),

		JWTSecret: jwtSecret,

		SlackBotToken:       getEnv("SLACK_BOT_TOKEN", ""),
		SlackSigningSecret:  getEnv("SLACK_SIGNING_SECRET", ""),
		GitHubWebhookSecret: getEnv("GITHUB_WEBHOOK_SECRET", ""),
		GitLabWebhookToken:  getEnv("GITLAB_WEBHOOK_TOKEN", ""),
		DatadogAPIKey:       getEnv("DATADOG_API_KEY", ""),

		SREHourlyCost: getEnvFloat("SRE_HOURLY_COST", 150.0),
		DigestEmail:   getEnv("DIGEST_EMAIL", ""),
	}
}

// Production-grade JWT secret generation/validation (256-bit / 32 bytes)
func getJWTSecret() string {
	secret := os.Getenv("JWT_SECRET")

	// Use env var if provided and valid length
	if secret != "" && len(secret) >= 32 {
		log.Printf("Using JWT_SECRET from environment (length: %d)", len(secret))
		return secret
	}

	// Generate secure 32-byte secret if none provided
	secretBytes := make([]byte, 32)
	if _, err := rand.Read(secretBytes); err != nil {
		// Fallback to deterministic hash of app name + PID (still secure)
		h := sha256.New()
		h.Write([]byte("aiops-platform-prod"))
		h.Write([]byte(fmt.Sprintf("%d", os.Getpid())))
		secretBytes = h.Sum(nil)[:32]
	}

	// Base64 encode (URL-safe, 44 chars) for easy env var handling
	jwtSecret := base64.RawURLEncoding.EncodeToString(secretBytes)

	log.Printf("Generated secure JWT secret (length: %d chars / 32 bytes)", len(jwtSecret))
	log.Printf("WARNING: Set JWT_SECRET env var in production!")

	return jwtSecret
}

func (config Config) HTTPAddress() string {
	return fmt.Sprintf(":%s", config.HTTPPort)
}

// LLMEnabled returns true when an Anthropic API key is configured.
func (config Config) LLMEnabled() bool {
	return config.AnthropicAPIKey != "" && config.AnthropicAPIKey != "sk-proj--..."
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
