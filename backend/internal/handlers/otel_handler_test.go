package handlers

import "testing"

// Source-type binding is a security control: without it a token issued for one
// integration can post as any other, and on the custom endpoints that also
// selects which schema mapping interprets the payload. The matrix below is the
// contract — every row states an intent, not just an expectation.
func TestAuthorizeSourceType(t *testing.T) {
	cases := []struct {
		name     string
		srcType  string
		endpoint string
		allow    bool
	}{
		// Unspecialised sources make no claim about what they send, so they
		// stay usable everywhere — same rule /ingest/prometheus already applies.
		{"empty type on logs", "", "otel-logs", true},
		{"generic on metrics", "generic", "otel-metrics", true},
		{"webhook on traces", "webhook", "otel-traces", true},
		{"generic on custom", "generic", "custom:acme", true},

		// A source registered plainly as "otel" may use any OTLP signal.
		{"otel on logs", "otel", "otel-logs", true},
		{"otel on metrics", "otel", "otel-metrics", true},
		{"otel on traces", "otel", "otel-traces", true},

		// Signal-specific OTLP sources are pinned to their own signal.
		{"otel-logs on logs", "otel-logs", "otel-logs", true},
		{"otel-logs on metrics", "otel-logs", "otel-metrics", false},
		{"otel-traces on logs", "otel-traces", "otel-logs", false},

		// Custom sources are pinned to their own source type. This is the one
		// that matters most: the type picks the schema mapping.
		{"custom matches", "custom:acme", "custom:acme", true},
		{"custom mismatched", "custom:acme", "custom:other", false},
		{"custom on otlp", "custom:acme", "otel-logs", false},

		// Cross-integration reuse is exactly what this prevents.
		{"prometheus on logs", "prometheus", "otel-logs", false},
		{"datadog on traces", "datadog", "otel-traces", false},
		{"pagerduty on custom", "pagerduty", "custom:acme", false},
		{"otel on custom", "otel", "custom:acme", false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			reason := authorizeSourceType(c.srcType, c.endpoint)
			allowed := reason == ""
			if allowed != c.allow {
				t.Fatalf("authorizeSourceType(%q, %q): allowed=%v, want %v (reason=%q)",
					c.srcType, c.endpoint, allowed, c.allow, reason)
			}
			// A refusal must say what was wrong; a bare 403 is unusable to the
			// operator wiring the integration up.
			if !allowed {
				if reason == "" {
					t.Fatal("refusal returned an empty reason")
				}
				for _, want := range []string{c.srcType, c.endpoint} {
					if want != "" && !contains(reason, want) {
						t.Errorf("reason %q does not mention %q", reason, want)
					}
				}
			}
		})
	}
}

func contains(haystack, needle string) bool {
	return len(needle) > 0 && len(haystack) >= len(needle) &&
		(haystack == needle || indexOf(haystack, needle) >= 0)
}

func indexOf(haystack, needle string) int {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return i
		}
	}
	return -1
}
