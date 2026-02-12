package mcp

import (
	"fmt"
	"os"
)

// buildEnv cria um slice de env vars herdando o ambiente do processo pai
// e adicionando as variaveis extras da config do servidor MCP.
// Isso garante que processos filhos tenham PATH e demais vars do sistema.
func buildEnv(extra map[string]string) []string {
	base := os.Environ()
	for k, v := range extra {
		base = append(base, fmt.Sprintf("%s=%s", k, v))
	}
	return base
}
