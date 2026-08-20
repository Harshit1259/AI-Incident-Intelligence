package handlers

// White-box tests for normalizeSeverityInput (unexported).
// Must be in package handlers (not handlers_test) to access unexported symbols.

import "testing"

func TestNormalizeSeverityInput(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		// Canonical pass-through
		{"critical", "critical"},
		{"high", "high"},
		{"medium", "medium"},
		{"low", "low"},
		{"info", "info"},

		// critical synonyms
		{"crit", "critical"},
		{"fatal", "critical"},
		{"p0", "critical"},
		{"emergency", "critical"},
		{"CRITICAL", "critical"}, // case-insensitive

		// high synonyms
		{"error", "high"},
		{"err", "high"},
		{"p1", "high"},
		{"major", "high"},
		{"HIGH", "high"},

		// medium synonyms
		{"warning", "medium"},
		{"warn", "medium"},
		{"med", "medium"},
		{"p2", "medium"},
		{"moderate", "medium"},
		{"WARN", "medium"},

		// low synonyms
		{"minor", "low"},
		{"p3", "low"},

		// info synonyms
		{"information", "info"},
		{"informational", "info"},
		{"notice", "info"},
		{"p4", "info"},
		{"debug", "info"},

		// Unknown / invalid → default medium (BUG #15 regression)
		{"SUPER_CRITICAL_VERY_BAD", "medium"},
		{"", "medium"},
		{"   ", "medium"},
		{"unknown_severity_level", "medium"},
	}

	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			got := normalizeSeverityInput(tc.input)
			if got != tc.want {
				t.Errorf("normalizeSeverityInput(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}
