//go:build darwin

package main

import (
	"os/exec"
)

var showNativeFatalError = showFatalErrorAppleScript

func showFatalErrorAppleScript(message string) {
	command := exec.Command(
		"osascript",
		"-e",
		"on run argv",
		"-e",
		`display alert (item 1 of argv) as critical buttons {"OK"} default button "OK"`,
		"-e",
		"end run",
		message,
	)
	_ = command.Run()
}
