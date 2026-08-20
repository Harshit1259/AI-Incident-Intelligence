//go:build !linux

// Package sandbox provides isolated subprocess execution.
// On non-Linux platforms the sandbox runs without kernel security attributes
// (no NoNewPrivs, no Pdeathsig, no Setpgid) because those are Linux-specific.
// Production deployments of the NeurOps agent are expected to run on Linux.
package sandbox

import (
	"context"
	"fmt"
	"os/exec"
	"time"
)

// Options configures how a sandboxed process is executed.
type Options struct {
	Timeout     time.Duration
	MaxOutputKB int
}

// DefaultOptions returns safe production defaults.
func DefaultOptions() Options {
	return Options{
		Timeout:     30 * time.Second,
		MaxOutputKB: 512,
	}
}

// Execute runs the given binary with args and returns combined output.
// On non-Linux platforms no kernel sandbox restrictions are applied.
func Execute(ctx context.Context, name string, args []string, opts Options) ([]byte, error) {
	if opts.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, opts.Timeout)
		defer cancel()
	}

	cmd := exec.CommandContext(ctx, name, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		if ctx.Err() != nil {
			return out, fmt.Errorf("execution timed out after %s", opts.Timeout)
		}
		return out, err
	}

	maxBytes := opts.MaxOutputKB * 1024
	if maxBytes <= 0 {
		maxBytes = 512 * 1024
	}
	if len(out) > maxBytes {
		out = out[:maxBytes]
	}

	return out, nil
}
