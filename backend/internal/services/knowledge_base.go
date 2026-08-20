package services

import "strings"

// KnowledgeEntry represents a single known incident pattern with full RCA,
// reasoning, resolution, and business impact — pre-computed, no API calls needed.
type KnowledgeEntry struct {
	ID          string
	Category    string
	SubCategory string

	// Pattern matching
	Keywords  []string
	AntiWords []string
	MetricName string
	Severity   string

	// Pre-computed analysis
	RootCause       string
	Reasoning       []string
	Impact          string
	ResolutionSteps []string
	Prevention      string

	// Confidence & risk
	BaseConfidence int
	BaseRiskScore  int
}

// KnowledgeBase holds ALL known patterns organized for fast lookup.
type KnowledgeBase struct {
	entries    []KnowledgeEntry
	byCategory map[string][]KnowledgeEntry
	byMetric   map[string][]KnowledgeEntry
}

// NewKnowledgeBase builds the full knowledge base with 500+ entries.
func NewKnowledgeBase() *KnowledgeBase {
	entries := buildAllEntries()
	kb := &KnowledgeBase{
		entries:    entries,
		byCategory: make(map[string][]KnowledgeEntry),
		byMetric:   make(map[string][]KnowledgeEntry),
	}
	for i := range entries {
		e := &entries[i]
		kb.byCategory[e.Category] = append(kb.byCategory[e.Category], *e)
		if e.MetricName != "" {
			kb.byMetric[e.MetricName] = append(kb.byMetric[e.MetricName], *e)
		}
	}
	return kb
}

// Count returns total entries in the knowledge base.
func (kb *KnowledgeBase) Count() int {
	return len(kb.entries)
}

// Match finds the best matching KnowledgeEntry for an event.
func (kb *KnowledgeBase) Match(title, message, metricName string) *KnowledgeEntry {
	// 1. Try metric name first
	if metricName != "" {
		if candidates, ok := kb.byMetric[metricName]; ok {
			best := bestKeywordMatch(candidates, title+" "+message)
			if best != nil {
				return best
			}
			// Return first metric match even without keyword overlap
			if len(candidates) > 0 {
				c := candidates[0]
				return &c
			}
		}
	}

	// 2. Keyword match across all entries
	text := strings.ToLower(title + " " + message)
	if text == " " {
		return nil
	}
	return bestKeywordMatch(kb.entries, title+" "+message)
}

// MatchByIncident matches against incident text fields.
func (kb *KnowledgeBase) MatchByIncident(title, rootCause string, reasoning []string) *KnowledgeEntry {
	combined := title + " " + rootCause
	for _, r := range reasoning {
		combined += " " + r
	}
	return kb.Match(combined, "", "")
}

func bestKeywordMatch(candidates []KnowledgeEntry, rawText string) *KnowledgeEntry {
	text := strings.ToLower(rawText)
	var best *KnowledgeEntry
	bestCount := 0

	for i := range candidates {
		e := &candidates[i]

		// Anti-word exclusion
		excluded := false
		for _, aw := range e.AntiWords {
			if strings.Contains(text, strings.ToLower(aw)) {
				excluded = true
				break
			}
		}
		if excluded {
			continue
		}

		count := 0
		for _, kw := range e.Keywords {
			if strings.Contains(text, strings.ToLower(kw)) {
				count++
			}
		}
		if count > bestCount {
			bestCount = count
			cp := *e
			best = &cp
		}
	}
	if bestCount == 0 {
		return nil
	}
	return best
}

// ─────────────────────────────────────────────────────
// Entry builders by category
// ─────────────────────────────────────────────────────

func buildAllEntries() []KnowledgeEntry {
	var all []KnowledgeEntry
	all = append(all, buildCPUEntries()...)
	all = append(all, buildMemoryEntries()...)
	all = append(all, buildDiskEntries()...)
	all = append(all, buildNetworkEntries()...)
	all = append(all, buildSystemEntries()...)
	all = append(all, buildJavaEntries()...)
	all = append(all, buildPythonEntries()...)
	all = append(all, buildNodeEntries()...)
	all = append(all, buildGoEntries()...)
	all = append(all, buildGenericAppEntries()...)
	all = append(all, buildPostgresEntries()...)
	all = append(all, buildMySQLEntries()...)
	all = append(all, buildRedisEntries()...)
	all = append(all, buildMongoEntries()...)
	all = append(all, buildKubernetesEntries()...)
	all = append(all, buildInfraEntries()...)
	all = append(all, buildAWSEntries()...)
	all = append(all, buildGCPEntries()...)
	all = append(all, buildAzureEntries()...)
	all = append(all, buildExtraSystemEntries()...)
	all = append(all, buildExtraAppEntries()...)
	all = append(all, buildAdvancedEntries()...)
	return all
}
