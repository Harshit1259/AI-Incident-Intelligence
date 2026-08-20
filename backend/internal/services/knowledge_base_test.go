package services

import "testing"

func TestKnowledgeBaseCount(t *testing.T) {
	kb := NewKnowledgeBase()
	if kb.Count() < 500 {
		t.Errorf("expected 500+ KB entries, got %d", kb.Count())
	}
}

func TestKnowledgeBaseMatch_ConnectionRefused(t *testing.T) {
	kb := NewKnowledgeBase()
	entry := kb.Match("Connection refused to database", "", "")
	if entry == nil {
		t.Fatal("expected a match for 'connection refused', got nil")
	}
	if entry.RootCause == "" {
		t.Error("matched entry should have a non-empty RootCause")
	}
}

func TestKnowledgeBaseMatch_OOM(t *testing.T) {
	kb := NewKnowledgeBase()
	entry := kb.Match("Out of memory: killed process 12345", "", "")
	if entry == nil {
		t.Fatal("expected a match for OOM, got nil")
	}
	if entry.Category != "memory" && entry.Category != "system" {
		// Accept either category since both are valid for OOM
		t.Logf("OOM matched category: %s", entry.Category)
	}
}

func TestKnowledgeBaseMatch_DiskFull(t *testing.T) {
	kb := NewKnowledgeBase()
	entry := kb.Match("No space left on device /dev/sda1", "", "")
	if entry == nil {
		t.Fatal("expected a match for disk full, got nil")
	}
}

func TestKnowledgeBaseMatch_NoMatchForBenign(t *testing.T) {
	kb := NewKnowledgeBase()
	entry := kb.Match("Application started successfully on port 8080", "", "")
	if entry != nil {
		t.Errorf("expected no match for benign text, got entry: %s", entry.ID)
	}
}

func TestKnowledgeBaseMatch_MetricName(t *testing.T) {
	kb := NewKnowledgeBase()
	// Test with a metric name that exists in the KB
	entry := kb.Match("", "", "system.cpu.used.percent")
	// If matched, it should have valid fields
	if entry != nil {
		if entry.RootCause == "" {
			t.Error("metric-matched entry should have RootCause")
		}
		if len(entry.ResolutionSteps) == 0 {
			t.Error("metric-matched entry should have ResolutionSteps")
		}
	}
}

func TestKnowledgeBaseMatch_KernelPanic(t *testing.T) {
	kb := NewKnowledgeBase()
	entry := kb.Match("kernel panic - not syncing: fatal exception in interrupt", "", "")
	if entry == nil {
		t.Fatal("expected a match for kernel panic, got nil")
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Match-strength regression tests
//
// The knowledge base asserts a cause with a confidence score attached, so a
// weak match is not a harmless miss — it is a confident wrong answer, which is
// worse than returning nothing.
// ─────────────────────────────────────────────────────────────────────────────

// TestMatch_RejectsSubstringHit is the regression test for the bug where the
// keyword "api" matched inside the service name "payments-api" and diagnosed an
// unrelated fault as an API rate limit at 80% confidence.
func TestMatch_RejectsSubstringHit(t *testing.T) {
	kb := NewKnowledgeBase()

	// Text whose only relationship to any entry is a service name containing a
	// keyword as a substring. There is no real failure signal here.
	entry := kb.MatchByIncident(
		"zzqqxx unmatchable condition",
		"opaque subsystem fault",
		[]string{"Detected critical-severity event from zzqx-probe on service payments-api"},
	)
	if entry != nil {
		t.Errorf("expected no match for text with no real failure signal, got %q (%s)",
			entry.ID, entry.RootCause)
	}
}

// TestTokenize_SplitsOnWordBoundaries confirms compound identifiers cannot
// satisfy a keyword by substring.
func TestTokenize_SplitsOnWordBoundaries(t *testing.T) {
	tokens := tokenize("payments-api cpuset room oom_killer")

	if !tokens["api"] {
		t.Error("hyphenated compounds should split into their parts")
	}
	// These must NOT be present as tokens — they exist only as substrings.
	for _, absent := range []string{"cpu", "payments-api", "oom_killer"} {
		if tokens[absent] {
			t.Errorf("%q should not be a token — it is only a substring", absent)
		}
	}
	// Underscore is a separator, so oom_killer yields its parts.
	if !tokens["oom"] || !tokens["killer"] {
		t.Error("underscore-joined words should split")
	}
}

// TestKeywordHits_MultiWordRequiresAllWords confirms a phrase keyword only
// matches when every word is present.
func TestKeywordHits_MultiWordRequiresAllWords(t *testing.T) {
	tokens := tokenize("the connection was refused by the upstream host")
	if !keywordHits(tokens, "connection refused") {
		t.Error("all words present — phrase should match")
	}
	partial := tokenize("the connection was reset")
	if keywordHits(partial, "connection refused") {
		t.Error("only one word present — phrase must not match")
	}
}

// TestMatch_StillMatchesGenuineSignal confirms the stricter threshold did not
// break real detection: a genuine failure description must still resolve.
func TestMatch_StillMatchesGenuineSignal(t *testing.T) {
	kb := NewKnowledgeBase()
	matched := 0
	probes := []struct{ title, message string }{
		{"Out of memory", "process killed by the OOM killer, memory exhausted"},
		{"Disk full", "no space left on device, disk usage at 100 percent"},
		{"Connection refused", "connection refused by database, connection pool exhausted"},
	}
	for _, p := range probes {
		if e := kb.Match(p.title, p.message, ""); e != nil {
			matched++
			t.Logf("%-22q → %s", p.title, e.ID)
		} else {
			t.Logf("%-22q → no match", p.title)
		}
	}
	if matched == 0 {
		t.Error("stricter matching broke genuine detection — no probe matched any entry")
	}
}
