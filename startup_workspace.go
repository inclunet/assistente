package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// parseWorkspaceArgs separa a pasta explícita dos argumentos do runtime Wails.
// Não muda o cwd nem a resolução de configuração/banco do processo.
func parseWorkspaceArgs(args []string) (string, []string, error) {
	var directory string
	var runtimeArgs []string
	positional := false
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" && !positional {
			positional = true
			continue
		}
		if strings.HasPrefix(arg, "-") && !positional {
			name, _, hasValue := strings.Cut(strings.TrimLeft(arg, "-"), "=")
			switch name {
			// Flags que o próprio Wails consome em dev e na geração de bindings.
			case "assetdir", "devserver", "frontenddevserverurl", "loglevel", "tsprefix", "tssuffix", "tsoutputtype":
				runtimeArgs = append(runtimeArgs, arg)
				if !hasValue {
					if i+1 == len(args) {
						return "", nil, errors.New(currentStartupMessages().workspaceArguments)
					}
					i++
					runtimeArgs = append(runtimeArgs, args[i])
				}
			case "debug":
				runtimeArgs = append(runtimeArgs, arg)
			default:
				return "", nil, errors.New(currentStartupMessages().workspaceArguments)
			}
			continue
		}
		if directory != "" || arg == "" {
			return "", nil, errors.New(currentStartupMessages().workspaceArguments)
		}
		path, err := filepath.Abs(arg)
		if err != nil {
			return "", nil, fmt.Errorf(currentStartupMessages().workspaceDirectory, arg, err)
		}
		info, err := os.Stat(path)
		if err != nil {
			return "", nil, fmt.Errorf(currentStartupMessages().workspaceDirectory, path, err)
		}
		if !info.IsDir() {
			return "", nil, fmt.Errorf(currentStartupMessages().workspaceNotDirectory, path)
		}
		directory = path
	}
	return directory, runtimeArgs, nil
}
