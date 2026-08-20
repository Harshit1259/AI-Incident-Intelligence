// Package edition defines the product's module-gating system.
//
// Every feature in this platform belongs to exactly one Edition. Operators
// control which editions are active via the PLATFORM_EDITIONS environment
// variable. The zero value (empty string) enables all editions, so existing
// deployments see zero behavior change.
//
// Edition boundary rules:
//   - Core is the irreducible wedge required by every other edition.
//   - Enterprise, AgentAutomation, and SaaSOpsAddOn each depend on Core.
//   - Modules may only import the edition package — never the modules package.
package edition

import (
	"sort"
	"strings"
)

// Edition is a bitmask flag representing one product module set.
type Edition uint8

const (
	Core            Edition = 1 << iota // 1 — alert ingest, incident lifecycle, auth
	Enterprise                          // 2 — policy, audit, compliance governance
	AgentAutomation                     // 4 — NeuroOps agent pipeline, log explorer
	SaaSOpsAddOn                        // 8 — analytics, AI enrichment, risk, runbooks
)

// All enables every edition. Used as the default so a server without
// PLATFORM_EDITIONS set registers every route (backward compatible).
const All Edition = Core | Enterprise | AgentAutomation | SaaSOpsAddOn

// Gate records which editions are active for a running server instance.
// It is immutable after construction.
type Gate struct{ active Edition }

// NewGate constructs a Gate from a bitmask.
func NewGate(active Edition) Gate { return Gate{active: active} }

// AllEditions returns a Gate with every edition enabled.
func AllEditions() Gate { return Gate{active: All} }

// Has reports whether edition e is active in this gate.
func (g Gate) Has(e Edition) bool { return g.active&e != 0 }

// Names returns lowercase identifiers of active editions, sorted alphabetically.
// The identifiers match the tokens accepted by ParseEditions.
func (g Gate) Names() []string {
	var out []string
	if g.Has(Core)            { out = append(out, "core") }
	if g.Has(Enterprise)      { out = append(out, "enterprise") }
	if g.Has(AgentAutomation) { out = append(out, "agent") }
	if g.Has(SaaSOpsAddOn)    { out = append(out, "saasops") }
	sort.Strings(out)
	return out
}

// ParseEditions converts a comma-separated string to a Gate.
//
// Recognised tokens (case-insensitive): "core", "enterprise", "agent", "saasops"
// Unknown tokens are silently skipped.
// Empty or blank input returns AllEditions() for backward compatibility.
func ParseEditions(s string) Gate {
	s = strings.TrimSpace(s)
	if s == "" {
		return AllEditions()
	}
	var active Edition
	for _, tok := range strings.Split(s, ",") {
		switch strings.TrimSpace(strings.ToLower(tok)) {
		case "core":
			active |= Core
		case "enterprise":
			active |= Enterprise
		case "agent":
			active |= AgentAutomation
		case "saasops":
			active |= SaaSOpsAddOn
		}
	}
	if active == 0 {
		return AllEditions()
	}
	return Gate{active: active}
}
