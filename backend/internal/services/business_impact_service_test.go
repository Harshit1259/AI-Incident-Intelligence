package services

import "testing"

func TestSeverityToImpactPct(t *testing.T) {
	tests := []struct {
		sev  string
		want float64
	}{
		{"critical", 0.8},
		{"high", 0.5},
		{"medium", 0.2},
		{"low", 0.05},
		{"unknown", 0.2},
		{"", 0.2},
	}
	for _, tt := range tests {
		got := severityToImpactPct(tt.sev)
		if got != tt.want {
			t.Errorf("severityToImpactPct(%q) = %f, want %f", tt.sev, got, tt.want)
		}
	}
}

func TestSeverityMultiplier(t *testing.T) {
	tests := []struct {
		sev  string
		want float64
	}{
		{"critical", 1.0},
		{"high", 0.7},
		{"medium", 0.4},
		{"low", 0.2},
		{"unknown", 0.4},
		{"", 0.4},
	}
	for _, tt := range tests {
		got := severityMultiplier(tt.sev)
		if got != tt.want {
			t.Errorf("severityMultiplier(%q) = %f, want %f", tt.sev, got, tt.want)
		}
	}
}

func TestTierMultiplier(t *testing.T) {
	tests := []struct {
		tier string
		want float64
	}{
		{"TIER_0", 1.5},
		{"TIER_1", 1.2},
		{"TIER_2", 1.0},
		{"TIER_3", 0.7},
		{"tier_0", 1.5}, // case insensitive via ToUpper
		{"UNKNOWN", 1.0},
		{"", 1.0},
	}
	for _, tt := range tests {
		got := tierMultiplier(tt.tier)
		if got != tt.want {
			t.Errorf("tierMultiplier(%q) = %f, want %f", tt.tier, got, tt.want)
		}
	}
}

func TestRound2(t *testing.T) {
	tests := []struct {
		input float64
		want  float64
	}{
		{1.234, 1.23},
		{1.235, 1.24},
		{0.0, 0.0},
		{100.999, 101.0},
	}
	for _, tt := range tests {
		got := round2(tt.input)
		if got != tt.want {
			t.Errorf("round2(%f) = %f, want %f", tt.input, got, tt.want)
		}
	}
}
