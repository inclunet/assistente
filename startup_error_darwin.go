//go:build darwin

package main

import (
	"os"
	"os/exec"
)

var showNativeFatalError = showFatalErrorAppleScript

func showFatalErrorAppleScript(message string) {
	command := exec.Command(
		"osascript",
		"-e",
		`display alert (system attribute "ASSISTENTE_STARTUP_ERROR") as critical buttons {"OK"} default button "OK"`,
	)
	command.Env = append(os.Environ(), "ASSISTENTE_STARTUP_ERROR="+message)
	_ = command.Run()
}
