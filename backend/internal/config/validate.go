package config

import (
	"fmt"
	"log"
	"os"
	"strings"
)

// VarStatus is the resolved state of one env var at startup.
type VarStatus struct {
	EnvVar
	IsSet       bool   // whether the env var is non-empty in the current environment
	ActualValue string // redacted value shown in summary ("***" for secrets, actual value otherwise)
	Warning     string // non-empty when something requires operator attention
}

// ValidationResult is the output of Validate().
type ValidationResult struct {
	IsProd    bool
	HasFatal  bool
	Statuses  []VarStatus
}

// Validate reads the canonical schema and cross-checks against the loaded config.
// It returns a ValidationResult that can be printed or serialized.
// In production mode, missing required-in-prod vars are logged as fatal.
func Validate(cfg Config) ValidationResult {
	schema := GetSchema()
	result := ValidationResult{IsProd: cfg.IsProd}

	for _, v := range schema.EnvVars {
		raw := os.Getenv(v.Name)
		isSet := raw != ""

		displayValue := ""
		if isSet {
			if v.Secret {
				displayValue = redactSecret(raw)
			} else {
				displayValue = raw
			}
		}

		warning := ""
		switch v.Required {
		case RequiredAlways:
			if !isSet && v.Default == "" {
				warning = "NOT SET — required in all environments"
				result.HasFatal = true
			}
		case RequiredInProd:
			if cfg.IsProd && !isSet {
				warning = "NOT SET — required in production (ENV=production)"
				result.HasFatal = true
			} else if !cfg.IsProd && !isSet {
				warning = "not set (ephemeral/dev default used)"
			}
		case OptionalDeprecated:
			if isSet {
				warning = "SET — this variable is deprecated; migrate away from it"
			}
		}

		status := VarStatus{
			EnvVar:      v,
			IsSet:       isSet,
			ActualValue: displayValue,
			Warning:     warning,
		}
		result.Statuses = append(result.Statuses, status)
	}

	return result
}

// PrintStartupSummary logs a structured configuration summary on startup.
// Fatal entries are logged individually so operators see them immediately.
func PrintStartupSummary(cfg Config) {
	result := Validate(cfg)

	mode := "development"
	if cfg.IsProd {
		mode = "production"
	}

	border := strings.Repeat("─", 60)
	log.Printf("config: %s", border)
	log.Printf("config:   NeurOps Platform — Configuration Summary")
	log.Printf("config:   Mode: %s", strings.ToUpper(mode))
	log.Printf("config: %s", border)

	currentGroup := EnvGroup("")
	for _, s := range result.Statuses {
		// Print group header when the group changes.
		if s.Group != currentGroup {
			currentGroup = s.Group
			log.Printf("config:   [%s]", strings.ToUpper(string(currentGroup)))
		}

		icon := "✓"
		detail := ""

		if !s.IsSet {
			icon = "○" // not set, but has a default
			if s.Default != "" {
				detail = fmt.Sprintf("default: %s", s.Default)
			} else {
				detail = "not set"
			}
		} else {
			detail = s.ActualValue
		}

		if s.Warning != "" {
			icon = "!"
			detail = s.Warning
		}

		log.Printf("config:   %s %-36s %s", icon, s.Name, detail)
	}

	log.Printf("config: %s", border)

	if result.HasFatal && cfg.IsProd {
		log.Fatal("FATAL: one or more required env vars are missing in production mode. Fix the configuration above and restart.")
	}
	if result.HasFatal {
		log.Printf("config: WARNING — one or more required-in-prod vars are unset. This is acceptable in development but will be fatal if ENV=production is set.")
	}
}

// redactSecret masks a secret value while preserving its prefix for debugging.
// "sk-proj-abc123" → "sk-proj-[redacted]"
// "verylongsecret"  → "[set]"
func redactSecret(value string) string {
	if len(value) == 0 {
		return ""
	}
	// Preserve up to 8 chars of prefix for key-type identification (e.g. "sk-proj-", "sk-ant-").
	prefix := value
	if len(prefix) > 8 {
		prefix = prefix[:8]
	}
	// Only show prefix if it looks like a recognizable token prefix (no spaces, no '=').
	if strings.ContainsAny(prefix, " =\t\n") {
		return "[set]"
	}
	if len(value) <= 8 {
		return "[set]"
	}
	return prefix + "[redacted]"
}
