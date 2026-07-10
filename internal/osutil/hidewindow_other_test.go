//go:build !windows

package osutil

import (
	"os/exec"
	"testing"
)

func TestHideConsoleWindowNoOpForaDoWindows(t *testing.T) {
	cmd := exec.Command("sh", "-c", "echo")

	HideConsoleWindow(cmd)

	if cmd.SysProcAttr != nil {
		t.Error("fora do Windows o comando não deveria ser modificado")
	}
}
