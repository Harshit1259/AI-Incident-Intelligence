/*
 * NeurOps Agent — Go SDK Plugin Interfaces
 * Copyright (c) NeurOps 2025. All rights reserved.
 *
 * Every Go plugin binary implements one of these interfaces.
 * The plugin engine discovers the binary, marshals context to JSON,
 * base64-encodes it, passes it as a CLI arg, and decodes stdout.
 *
 * Plugin contract (Go binary):
 *   Input:  os.Args[1] = base64(json(context))
 *   Output: stdout     = base64(json(result))   [exit 0]
 *           stderr     = error text             [exit non-zero]
 *
 * Context keys injected by the engine:
 *   "request.type"   "discover" | "collect" | "rediscover" | "run"
 *   "plugin.id"      plugin directory name
 *   "plugin.type"    "metric" | "runbook" | "topology"
 *   "agent.id"       agent UUID
 *   "object.ip"      target IP (for remote plugins)
 *   "object.context" full credential/config map for the target object
 *
 * Result keys expected by the engine:
 *   "status"     "succeed" | "fail"
 *   "result"     map of metric names → values  (on success)
 *   "error"      human-readable error message  (on failure)
 *   "error.code" NeurOps error code, e.g. "NO047"
 *
 * Minimal plugin skeleton:
 *   package main
 *
 *   import (
 *       "github.com/neuroops/agent/sdk/go/plugin"
 *   )
 *
 *   type MyPlugin struct{}
 *
 *   func (p *MyPlugin) Discover(ctx plugin.Context) plugin.Result { ... }
 *   func (p *MyPlugin) Collect(ctx plugin.Context)  plugin.Result { ... }
 *
 *   func main() { plugin.RunMetric(&MyPlugin{}) }
 */

package plugin

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"

	"github.com/neuroops/agent/sdk/go/types"
)

// ── Context ───────────────────────────────────────────────────────────────────

// Context is the decoded input map passed to every plugin method.
type Context = types.NeurOpsMap

// ── Result ────────────────────────────────────────────────────────────────────

// Result is the output map written by every plugin method.
type Result = types.NeurOpsMap

// ── Interfaces ────────────────────────────────────────────────────────────────

// MetricPlugin is implemented by Go metric collection plugins.
type MetricPlugin interface {
	// Discover returns object metadata (name, OS, version, etc.).
	// Called once at startup and on rediscovery triggers.
	Discover(ctx Context) Result

	// Collect returns metric key/value pairs for the current poll cycle.
	// Called on every configured poll interval.
	Collect(ctx Context) Result
}

// MetricPluginWithRediscover extends MetricPlugin with optional rediscovery.
type MetricPluginWithRediscover interface {
	MetricPlugin
	// Rediscover is called when the object context changes.
	// Default behaviour: delegate to Discover.
	Rediscover(ctx Context) Result
}

// RunbookPlugin is implemented by Go remediation plugins.
type RunbookPlugin interface {
	// Init allocates resources (connections, sessions, locks).
	Init(ctx Context) Result
	// Run executes the remediation action.
	Run(ctx Context) Result
	// Destroy releases all resources. Always called, even after Run failure.
	Destroy(ctx Context) Result
}

// TopologyPlugin is implemented by Go topology discovery plugins.
type TopologyPlugin interface {
	// Discover returns network topology information (neighbours, links, etc.).
	Discover(ctx Context) Result
}

// ── Runner helpers ────────────────────────────────────────────────────────────

// RunMetric is the main() helper for metric plugins.
// It decodes the context, dispatches to the correct method, and writes output.
func RunMetric(p MetricPlugin) {
	ctx, reqType := readContext()

	var result Result
	switch reqType {
	case "collect":
		result = p.Collect(ctx)
	case "rediscover":
		if rp, ok := p.(MetricPluginWithRediscover); ok {
			result = rp.Rediscover(ctx)
		} else {
			result = p.Discover(ctx)
		}
	default: // "discover" or empty
		result = p.Discover(ctx)
	}

	writeResult(result)
}

// RunRunbook is the main() helper for runbook plugins.
func RunRunbook(p RunbookPlugin) {
	ctx, reqType := readContext()

	switch reqType {
	case "init":
		writeResult(p.Init(ctx))
	case "run":
		initResult := p.Init(ctx)
		if initResult.GetString("status") != "succeed" {
			writeResult(initResult)
			return
		}
		runResult := p.Run(ctx)
		_ = p.Destroy(ctx)
		writeResult(runResult)
	case "destroy":
		writeResult(p.Destroy(ctx))
	default:
		// Full lifecycle in one invocation
		initResult := p.Init(ctx)
		if initResult.GetString("status") != "succeed" {
			writeResult(initResult)
			return
		}
		runResult := p.Run(ctx)
		_ = p.Destroy(ctx)
		writeResult(runResult)
	}
}

// RunTopology is the main() helper for topology plugins.
func RunTopology(p TopologyPlugin) {
	ctx, _ := readContext()
	writeResult(p.Discover(ctx))
}

// ── Convenience result constructors ──────────────────────────────────────────

// Succeed wraps metrics in a standard success envelope.
func Succeed(metrics Result) Result { return types.Succeed(metrics) }

// Fail wraps an error in a standard failure envelope.
func Fail(message, errorCode string) Result { return types.Fail(message, errorCode) }

// ── I/O helpers ───────────────────────────────────────────────────────────────

func readContext() (Context, string) {
	if len(os.Args) < 2 {
		writeError("no context argument", "NO031")
		os.Exit(1)
	}

	raw, err := base64.StdEncoding.DecodeString(os.Args[1])
	if err != nil {
		writeError(fmt.Sprintf("base64 decode failed: %v", err), "NO031")
		os.Exit(1)
	}

	ctx := make(Context)
	if err := json.Unmarshal(raw, &ctx); err != nil {
		writeError(fmt.Sprintf("JSON decode failed: %v", err), "NO031")
		os.Exit(1)
	}

	reqType := ctx.GetString("request.type")
	return ctx, reqType
}

func writeResult(result Result) {
	data, _ := json.Marshal(result)
	encoded := base64.StdEncoding.EncodeToString(data)
	fmt.Println(encoded)
}

func writeError(message, code string) {
	writeResult(Fail(message, code))
}
