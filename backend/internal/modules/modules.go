// Package modules is the single source of truth for what belongs in each
// product edition. Every handler, service, and store traces back to exactly
// one module declared here.
//
// This package has no business logic. It only holds manifest data and two
// helpers (ActiveModules, ValidateDependencies) that main.go uses at startup.
package modules

import (
	"fmt"
	"strings"

	"ai-incident-platform/backend/internal/platform/edition"
)

// Manifest describes a product module: its identity, edition membership,
// the routes it owns, and which other editions it depends on.
type Manifest struct {
	ID          string
	Name        string
	Edition     edition.Edition
	Description string
	Routes      []string
	DependsOn   []edition.Edition
}

// ── Edition manifests ─────────────────────────────────────────────────────────

// Core is the irreducible product wedge. Every paying customer gets this.
// It is the only module that has no dependencies.
var Core = Manifest{
	ID:      "core",
	Name:    "Core",
	Edition: edition.Core,
	Description: "Alert ingest pipeline, incident correlation and lifecycle management, " +
		"JWT auth, source registry, status page, billing, and onboarding. " +
		"Required by every other module.",
	Routes: []string{
		"GET  /api/v1/health",
		"GET  /api/v1/config/schema",
		"POST /api/v1/auth/login",
		"POST /api/v1/auth/register",
		"*    /api/v1/events",
		"*    /api/v1/incidents/*",
		"*    /api/v1/sources/*",
		"POST /api/v1/ingest/*",
		"*    /api/v1/billing/*",
		"*    /api/v1/onboarding/*",
		"GET  /api/v1/platform/metrics",
	},
	DependsOn: nil,
}

// Enterprise adds governance and compliance capabilities for regulated teams.
var Enterprise = Manifest{
	ID:      "enterprise",
	Name:    "Enterprise",
	Edition: edition.Enterprise,
	Description: "Policy engine for operator governance, immutable audit log, " +
		"compliance reporting, tenant config, service catalog, and admin import tools.",
	Routes: []string{
		"*    /api/v1/policies/*",
		"*    /api/v1/config",
		"GET  /api/v1/audit",
		"*    /api/v1/admin/*",
	},
	DependsOn: []edition.Edition{edition.Core},
}

// AgentAutomation adds the NeuroOps observability agent ecosystem.
var AgentAutomation = Manifest{
	ID:      "agent",
	Name:    "Agent Automation",
	Edition: edition.AgentAutomation,
	Description: "NeuroOps agent deployment, enrollment, plugin execution engine, " +
		"structured log ingestion and exploration, and automated remediation actions.",
	Routes: []string{
		"POST /api/v1/agents/register",
		"POST /api/v1/ingest/agent",
		"*    /api/v1/agents/*",
		"*    /api/v1/logs/*",
	},
	DependsOn: []edition.Edition{edition.Core},
}

// SaaSOpsAddOn stacks analytics and AI enrichment on top of any tier.
var SaaSOpsAddOn = Manifest{
	ID:      "saasops",
	Name:    "SaaS Ops Add-on",
	Edition: edition.SaaSOpsAddOn,
	Description: "SLO tracking, on-call scheduling, anomaly detection, engineering health, " +
		"ROI dashboard, weekly digest, business impact profiling, alert quality feedback, " +
		"auto-resolve rules, runbook management, dependency mapping, WhatsApp notifications, " +
		"topology graph, incident memory and playbook learning, risk exposure dashboard, " +
		"AI copilot, AI explain, and postmortem generation.",
	Routes: []string{
		"*    /api/v1/slos/*",
		"*    /api/v1/oncall/*",
		"*    /api/v1/anomalies/*",
		"GET  /api/v1/engineering/health",
		"GET  /api/v1/roi",
		"*    /api/v1/digest/*",
		"*    /api/v1/business/*",
		"*    /api/v1/alerts/feedback/*",
		"*    /api/v1/auto-resolve/*",
		"*    /api/v1/runbooks/*",
		"*    /api/v1/dependencies/*",
		"*    /api/v1/whatsapp/*",
		"GET  /api/v1/compliance/*",
		"GET  /api/v1/incidents/topology/*",
		"*    /api/v1/incidents/memory/*",
		"GET  /api/v1/risk/*",
		"GET  /api/v1/ai/status",
	},
	DependsOn: []edition.Edition{edition.Core},
}

// All lists every module in ascending edition weight order.
var All = []Manifest{Core, Enterprise, AgentAutomation, SaaSOpsAddOn}

// ── Helpers ───────────────────────────────────────────────────────────────────

// ActiveModules returns manifests for editions present in gate, preserving order.
func ActiveModules(gate edition.Gate) []Manifest {
	var out []Manifest
	for _, m := range All {
		if gate.Has(m.Edition) {
			out = append(out, m)
		}
	}
	return out
}

// ValidateDependencies returns an error if any active module has an unmet dependency.
// Call this at startup before registering routes.
func ValidateDependencies(gate edition.Gate) error {
	for _, m := range All {
		if !gate.Has(m.Edition) {
			continue
		}
		for _, dep := range m.DependsOn {
			if !gate.Has(dep) {
				return fmt.Errorf(
					"module %q requires edition %q — add it to PLATFORM_EDITIONS",
					m.Name, editionName(dep),
				)
			}
		}
	}
	return nil
}

// Summary returns a comma-separated list of active module names for log output.
func Summary(gate edition.Gate) string {
	active := ActiveModules(gate)
	names := make([]string, len(active))
	for i, m := range active {
		names[i] = m.Name
	}
	return strings.Join(names, ", ")
}

func editionName(e edition.Edition) string {
	switch e {
	case edition.Core:
		return "core"
	case edition.Enterprise:
		return "enterprise"
	case edition.AgentAutomation:
		return "agent"
	case edition.SaaSOpsAddOn:
		return "saasops"
	default:
		return "unknown"
	}
}
