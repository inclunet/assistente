//go:build linux

package main

import "os/exec"

var showNativeFatalError = showFatalErrorDesktopDialog

func showFatalErrorDesktopDialog(message string) {
	commands := [][]string{
		{"zenity", "--error", "--title=Assistente", "--text=" + message},
		{"kdialog", "--error", message, "--title", "Assistente"},
		{"xmessage", "-center", "-title", "Assistente", message},
	}
	for _, args := range commands {
		if err := exec.Command(args[0], args[1:]...).Run(); err == nil {
			return
		}
	}
}
