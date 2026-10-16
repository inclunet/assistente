//go:build windows

package credentials

import (
	"os/exec"
	"syscall"
)

func configureCredentialCommand(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
}
