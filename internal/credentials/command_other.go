//go:build !windows

package credentials

import "os/exec"

func configureCredentialCommand(cmd *exec.Cmd) {}
