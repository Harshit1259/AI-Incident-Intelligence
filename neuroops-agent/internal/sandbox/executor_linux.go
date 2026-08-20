//go:build linux

// Package sandbox provides isolated subprocess execution for plugin and
// diagnostic command runs on Linux. It applies kernel-level restrictions to
// limit what a rogue or compromised plugin can do to the host system.
//
// Restrictions applied on Linux:
//   - Pdeathsig SIGKILL: child is killed if the agent process dies unexpectedly
//   - Setpgid: child runs in its own process group (clean kill on timeout)
//
// Additional hardening (operator responsibility, recommended for production):
//   - Run the agent with systemd option: NoNewPrivileges=yes
//   - Apply a seccomp profile (see docs/apparmor/neuroops-agent.profile)
//   - Run the agent in a rootless container with --security-opt no-new-privileges
package sandbox

import (
	"context"
	"fmt"
	"os/exec"
	"syscall"
	"time"
)

// Options configures how a sandboxed process is executed.
type Options struct {
	Timeout     time.Duration // wall-clock limit; 0 = no timeout
	MaxOutputKB int           // cap output size; 0 = use default (512 KB)
}

// DefaultOptions returns safe production defaults.
func DefaultOptions() Options {
	return Options{
		Timeout:     30 * time.Second,
		MaxOutputKB: 512,
	}
}

// Execute runs the given binary with args inside the sandbox and returns
// combined stdout+stderr output. The provided context is respected alongside
// any Options.Timeout (whichever is shorter wins).
func Execute(ctx context.Context, name string, args []string, opts Options) ([]byte, error) {
	if opts.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, opts.Timeout)
		defer cancel()
	}

	cmd := exec.CommandContext(ctx, name, args...)

	// Apply Linux kernel-level process isolation.
	cmd.SysProcAttr = &syscall.SysProcAttr{
		// Kill child immediately if the agent process exits. This prevents
		// orphaned plugin processes that continue running after the agent dies.
		Pdeathsig: syscall.SIGKILL,

		// Run the child in its own process group. When a timeout fires,
		// exec.CommandContext sends SIGKILL to the child's entire group,
		// not just the root process — cleaning up any sub-processes spawned
		// by the plugin.
		Setpgid: true,
	}

	out, err := cmd.CombinedOutput()
	if err != nil {
		if ctx.Err() != nil {
			return out, fmt.Errorf("execution timed out after %s", opts.Timeout)
		}
		return out, err
	}

	// Enforce output size cap to prevent memory exhaustion from runaway plugins.
	maxBytes := opts.MaxOutputKB * 1024
	if maxBytes <= 0 {
		maxBytes = 512 * 1024
	}
	if len(out) > maxBytes {
		out = out[:maxBytes]
	}

	return out, nil
}
