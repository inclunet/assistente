package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// parseWorkspaceArgs distingue a escolha explícita da pasta herdada do launcher.
func parseWorkspaceArgs(args []string) (string, []string, error) {
	remaining := make([]string, 0, len(args))
	var path string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		var value string
		switch {
		case i == 0 && !strings.HasPrefix(arg, "-"):
			value = arg
		default:
			remaining = append(remaining, arg)
			continue
		}
		if strings.TrimSpace(value) == "" || strings.HasPrefix(value, "--") {
			return "", nil, fmt.Errorf("é necessário informar uma pasta")
		}
		if path != "" {
			return "", nil, fmt.Errorf("pasta de workspace informada mais de uma vez")
		}
		absolute, err := filepath.Abs(value)
		if err != nil {
			return "", nil, fmt.Errorf("resolver pasta do workspace: %w", err)
		}
		info, err := os.Stat(absolute)
		if err != nil {
			return "", nil, fmt.Errorf("acessar pasta do workspace: %w", err)
		}
		if !info.IsDir() {
			return "", nil, fmt.Errorf("é necessário informar uma pasta")
		}
		path = absolute
	}
	return path, remaining, nil
}
