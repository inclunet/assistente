//go:build darwin

package main

import (
	"os/exec"
)

var showNativeFatalError = showFatalErrorAppleScript

func showFatalErrorAppleScript(title, message string) {
	command := exec.Command(
		"osascript",
		"-e",
		"on run argv",
		"-e",
		`display alert (item 1 of argv) message (item 2 of argv) as critical`,
		"-e",
		"end run",
		title,
		message,
	)
	_ = command.Run()
}
