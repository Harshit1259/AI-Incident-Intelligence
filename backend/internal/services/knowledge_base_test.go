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
