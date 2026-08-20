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

// minKeywordMatches is how many distinct keywords must hit before the knowledge
// base is willing to assert a cause.
//
// This used to be 1, combined with naive substring matching — so the single
// keyword "api" matching inside the service name "payments-api" was enough to
// diagnose an unrelated failure as "API rate limit hit (HTTP 429)" at 80%
// confidence. A wrong confident answer is worse than no answer, so the bar is
// two independent signals, unless one signal is specific enough to stand alone
// (see standaloneKeywordMinLen).
const minKeywordMatches = 2

// standaloneKeywordMinLen is the length at which a single keyword is considered
// specific enough to match on its own. Multi-word keywords ("connection
// refused", "oom killer") always qualify regardless of length.
const standaloneKeywordMinLen = 10

// tokenize splits text into a lowercase word set, so keyword matching happens
// on word boundaries rather than substrings. Without this, "api" matches inside
// "payments-api", "cpu" inside "cpuset", and "oom" inside "room".
func tokenize(text string) map[string]bool {
	tokens := make(map[string]bool)
	var cur strings.Builder
	flush := func() {
		if cur.Len() > 0 {
			tokens[cur.String()] = true
			cur.Reset()
		}
	}
	for _, r := range strings.ToLower(text) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			cur.WriteRune(r)
		} else {
			// Split on every separator, including '-' and '.', so "payments-api"
			// yields {"payments","api"} and cannot be matched by the whole string.
			flush()
		}
	}
	flush()
	return tokens
}

// keywordHits reports whether a keyword is present as whole words.
// A multi-word keyword requires every one of its words to be present.
func keywordHits(tokens map[string]bool, keyword string) bool {
	parts := strings.Fields(strings.ToLower(keyword))
	if len(parts) == 0 {
		return false
	}
	for _, p := range parts {
		p = strings.Trim(p, ".,:;()[]{}\"'")
		if p == "" {
			continue
		}
		if !tokens[p] {
			return false
		}
	}
	return true
}

// isStandaloneKeyword reports whether a single hit on this keyword is specific
// enough to justify a causal claim by itself.
func isStandaloneKeyword(keyword string) bool {
	if len(strings.Fields(keyword)) > 1 {
		return true // multi-word phrases are inherently specific
	}
	return len(keyword) >= standaloneKeywordMinLen
}

func bestKeywordMatch(candidates []KnowledgeEntry, rawText string) *KnowledgeEntry {
	tokens := tokenize(rawText)
	if len(tokens) == 0 {
		return nil
	}

	var best *KnowledgeEntry
	bestCount := 0

	for i := range candidates {
		e := &candidates[i]

		// Anti-word exclusion — one anti-word disqualifies the entry outright.
		excluded := false
		for _, aw := range e.AntiWords {
			if keywordHits(tokens, aw) {
				excluded = true
				break
			}
		}
		if excluded {
			continue
		}

		count := 0
		standalone := false
		for _, kw := range e.Keywords {
			if keywordHits(tokens, kw) {
				count++
				if isStandaloneKeyword(kw) {
					standalone = true
				}
			}
		}

		// Require corroboration: two keywords, or one specific enough to carry
		// the claim on its own.
		if count == 0 || (count < minKeywordMatches && !standalone) {
			continue
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
