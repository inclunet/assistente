package plugins

import (
	"context"
	"golang.org/x/sys/windows"
	"os/exec"
	"path/filepath"
)

func preToolCommand(ctx context.Context, script string) (*exec.Cmd, error) {
	system, err := windows.GetSystemDirectory()
	if err != nil {
		return nil, err
	}
	cmd := exec.CommandContext(ctx, filepath.Join(system, "WindowsPowerShell", "v1.0", "powershell.exe"), "-NoLogo", "-NoProfile", "-NonInteractive", "-Command", "[Console]::InputEncoding = [Console]::OutputEncoding = New-Object System.Text.UTF8Encoding($false); [Console]::In.ReadToEnd() | & $env:ASSISTENTE_HOOK_SCRIPT; if ($?) { exit 0 }; if ($LASTEXITCODE) { exit $LASTEXITCODE }; exit 1")
	cmd.Env = append(cmd.Environ(), "ASSISTENTE_HOOK_SCRIPT="+script)
	return cmd, nil
}
