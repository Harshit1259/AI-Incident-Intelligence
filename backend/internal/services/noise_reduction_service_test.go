package services

import (
	"testing"
)

func TestNoiseGrade(t *testing.T) {
	cases := []struct {
		pct   float64
		grade string
	}{
		{95.0, "A"},
		{90.0, "A"},
		{80.0, "B"},
		{75.0, "B"},
		{60.0, "C"},
		{50.0, "C"},
		{30.0, "D"},
		{0.0, "D"},
	}
	for _, c := range cases {
		got := noiseGrade(c.pct)
		if got != c.grade {
			t.Errorf("noiseGrade(%.1f) = %s; want %s", c.pct, got, c.grade)
		}
	}
}

func TestPctReduction(t *testing.T) {
	if got := pctReduction(12847, 2341); got < 81 || got > 82 {
		t.Errorf("pctReduction(12847,2341) = %.2f; want ~81.8", got)
	}
	if got := pctReduction(0, 0); got != 0 {
		t.Errorf("pctReduction(0,0) should be 0, got %.2f", got)
	}
	if got := pctReduction(100, 0); got != 100 {
		t.Errorf("pctReduction(100,0) should be 100, got %.2f", got)
	}
}

func TestFunnelLineCount(t *testing.T) {
	lines := buildFunnelLines(12847, 2341, 187, 143, 81.8, 92.0)
	if len(lines) != 4 {
		t.Fatalf("expected 4 funnel lines, got %d", len(lines))
	}
	if lines[0].Count != 12847 {
		t.Errorf("line[0] count = %d; want 12847", lines[0].Count)
	}
	if !lines[3].Highlight {
		t.Error("last line (true incidents) should be highlighted")
	}
}

func TestOnCallHoursSaved(t *testing.T) {
	// 12847 raw - 143 confirmed = 12704 noise avoided
	// 12704 × 5 min / 60 = 1058.7 hours saved
	noiseAvoided := 12847 - 143
	hours := float64(noiseAvoided) * avgAlertHandleMinutes / 60.0
	if hours < 1000 || hours > 1100 {
		t.Errorf("on-call hours saved = %.1f; expected ~1058", hours)
	}
}
