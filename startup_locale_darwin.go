//go:build darwin

package main

import (
	"os/exec"
	"strings"
)

func platformStartupLocale() string {
	output, err := exec.Command("defaults", "read", "-g", "AppleLocale").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(output))
}
