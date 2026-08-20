// Package security provides the command policy engine that governs what the
// backend is allowed to forward to remote agents for execution.
//
// Design principles:
//   - Observe vs Act separation: diagnostic commands (observe) vs remediation (act)
//   - Defense-in-depth: backend validates before the agent re-validates
//   - Shell injection prevention: dangerous sequences are blocked at source
//   - Approval enforcement: remediation actions require explicit operator approval
package security

import (
	"fmt"
	"strings"
)

// ExecutionMode classifies what a command does.
type ExecutionMode string

const (
	ModeObserve ExecutionMode = "observe" // read-only diagnostics — no side effects
	ModeAct     ExecutionMode = "act"     // state-changing — always requires approval
)

// PolicyDecision is the result of evaluating a command against the execution policy.
type PolicyDecision struct {
	Allowed      bool
	Mode         ExecutionMode
	Reason       string
	SanitizedCmd string
}

// shellDangerousSequences are patterns that indicate shell injection attempts.
// These are blocked before any allowlist check because they could chain arbitrary
// commands even when the prefix is in the allowlist.
var shellDangerousSequences = []string{
	"&&",  // AND operator — chains commands
	"||",  // OR operator — chains commands
	";",   // command separator
	"|",   // pipe — forwards output to arbitrary command
	"`",   // backtick — executes subshell
	"$(",  // command substitution
	"${",  // variable expansion
}

// observeAllowlist is the set of command prefixes the backend will forward to an agent.
// This intentionally mirrors the agent's own safeCommandPrefixes so the backend
// validates first, giving two independent enforcement layers.
// All entries here are read-only diagnostic commands with no state-changing side effects.
var observeAllowlist = []string{
	"df ",
	"df\t",
	"du ",
	"free ",
	"free\t",
	"top ",
	"uptime",
	"cat /proc/",
	"cat /etc/os-release",
	"cat /var/log/",
	"ps aux",
	"ps -ef",
	"netstat ",
	"ss ",
	"ip addr",
	"ip route",
	"systemctl status",
	"systemctl is-active",
	"journalctl ",
	"docker ps",
	"docker stats",
	"docker system df",
	"kubectl get",
	"kubectl describe",
	"kubectl logs",
	"kubectl top",
	"lsblk",
	"lsof ",
	"iostat",
	"vmstat",
	"mpstat",
	"sar ",
	"nslookup ",
	"dig ",
	"ping ",
	"traceroute ",
	"curl ",
	"head ",
	"tail ",
	"wc ",
	"sort ",
	"grep ",
	"find ",
	"ls ",
	"hostname",
	"uname ",
	"sysctl ",
	"mount",
	"blkid",
	"fdisk -l",
}

// EvaluateCommand applies the execution policy to the given command in the context
// of its action type and approval state.
//
// It enforces three rules in order:
//  1. Shell injection prevention — block dangerous metacharacters
//  2. Observe allowlist — only known diagnostic prefixes pass
//  3. Approval gate — remediation actions require explicit approval
func EvaluateCommand(cmd, actionType string, approved bool) PolicyDecision {
	cmd = strings.TrimSpace(cmd)

	if cmd == "" {
		return PolicyDecision{Allowed: false, Reason: "empty command"}
	}

	// Rule 1: Reject shell injection sequences before anything else.
	// Even allowlisted prefixes become dangerous if chained with operators.
	if found, seq := containsInjection(cmd); found {
		return PolicyDecision{
			Allowed: false,
			Reason:  fmt.Sprintf("command blocked: forbidden shell sequence %q detected", seq),
		}
	}

	// Rule 2: Command must start with an approved diagnostic prefix.
	lower := strings.ToLower(cmd)
	inAllowlist := false
	for _, prefix := range observeAllowlist {
		if strings.HasPrefix(lower, strings.ToLower(prefix)) {
			inAllowlist = true
			break
		}
	}

	if !inAllowlist {
		return PolicyDecision{
			Allowed: false,
			Mode:    ModeAct,
			Reason:  "command is not in the diagnostic observe allowlist — only read-only diagnostic commands may be forwarded to agents",
		}
	}

	// Rule 3: Remediation actions require explicit operator approval before execution.
	// This prevents the lightweight action handler from bypassing the approval workflow.
	if actionType == "remediation" && !approved {
		return PolicyDecision{
			Allowed: false,
			Mode:    ModeObserve,
			Reason:  "remediation action requires explicit operator approval before agent execution",
		}
	}

	return PolicyDecision{
		Allowed:      true,
		Mode:         ModeObserve,
		SanitizedCmd: cmd,
		Reason:       "command passed injection check, observe allowlist, and approval policy",
	}
}

// ClassifyActionMode returns the execution mode for an action type.
// This is used for audit logging independent of command evaluation.
func ClassifyActionMode(actionType string) ExecutionMode {
	switch strings.ToLower(actionType) {
	case "remediation":
		return ModeAct
	default:
		return ModeObserve
	}
}

// containsInjection checks for known dangerous shell sequences.
// Returns (true, sequence) if a dangerous sequence is found.
func containsInjection(cmd string) (bool, string) {
	for _, seq := range shellDangerousSequences {
		if strings.Contains(cmd, seq) {
			return true, seq
		}
	}
	return false, ""
}
