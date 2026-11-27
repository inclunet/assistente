//go:build !windows

package plugins

import (
	"context"
	"os/exec"
)

func preToolCommand(ctx context.Context, script string) (*exec.Cmd, error) {
	return exec.CommandContext(ctx, "/bin/sh", script), nil
}
