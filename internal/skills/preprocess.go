package skills

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

const (
	// DefaultCommandTimeout é o timeout padrão para execução de !commands (5 segundos).
	DefaultCommandTimeout = 5 * time.Second
	// MaxCommandOutputSize é o tamanho máximo da saída de um comando (100KB).
	MaxCommandOutputSize = 100 * 1024
)

// PreprocessCommands processa linhas com prefixo `!` no conteúdo de um skill.
// Cada linha `!command` ou `` !`command` `` é executada como shell command e substituída pelo output.
// Compatível com a spec oficial do Claude Code.
//
// Formatos suportados:
//   - !command → formato simples
//   - !`command` → formato oficial Claude Code (com backticks)
//
// Regras:
//   - Apenas linhas que começam com `!` (opcionalmente com espaço antes) são processadas
//   - O `!` (e backticks opcionais) são removidos; o restante é executado como comando shell
//   - O output substitui a linha inteira
//   - Se o comando falha, a linha é substituída por um comentário de erro
//   - Timeout de 5s por comando por padrão
//
// allowedCommands: se não-nil, apenas comandos cujo executável está na lista são executados.
// Se nil, todos os comandos são permitidos.
func PreprocessCommands(content string, allowedCommands []string) string {
	if content == "" {
		return ""
	}

	lines := strings.Split(content, "\n")
	result := make([]string, 0, len(lines))

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		if !strings.HasPrefix(trimmed, "!") || len(trimmed) < 2 {
			result = append(result, line)
			continue
		}

		// Extrai o comando (remove o !)
		cmd := strings.TrimSpace(trimmed[1:])
		if cmd == "" {
			result = append(result, line)
			continue
		}

		// Suporta sintaxe com backticks: !`command` → remove backticks
		cmd = stripBackticks(cmd)

		// Verifica se o comando é permitido
		if !isCommandAllowed(cmd, allowedCommands) {
			result = append(result, fmt.Sprintf("<!-- command blocked: %s (not in allowed list) -->", cmd))
			continue
		}

		// Executa o comando
		output, err := executeCommand(cmd)
		if err != nil {
			result = append(result, fmt.Sprintf("<!-- command failed: %s — %v -->", cmd, err))
			continue
		}

		// Adiciona output (sem trailing newline extra)
		output = strings.TrimRight(output, "\n\r")
		if output != "" {
			result = append(result, output)
		}
	}

	return strings.Join(result, "\n")
}

// stripBackticks remove backticks envolvendo o comando.
// Suporta: `command` → command, ``command`` → command
func stripBackticks(cmd string) string {
	// Remove backticks do início e fim
	if len(cmd) >= 2 && cmd[0] == '`' && cmd[len(cmd)-1] == '`' {
		cmd = cmd[1 : len(cmd)-1]
	}
	return strings.TrimSpace(cmd)
}

// isCommandAllowed verifica se um comando está na lista de permitidos.
// Se allowedCommands for nil, tudo é permitido (sem restrição).
// Compara pelo nome do executável (primeiro token do comando).
func isCommandAllowed(cmd string, allowedCommands []string) bool {
	if allowedCommands == nil {
		return true
	}

	// Extrai o primeiro token (executável)
	executable := strings.Fields(cmd)[0]

	for _, allowed := range allowedCommands {
		if strings.EqualFold(executable, allowed) {
			return true
		}
	}

	return false
}

// executeCommand executa um comando shell com timeout.
func executeCommand(cmd string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), DefaultCommandTimeout)
	defer cancel()

	var c *exec.Cmd
	if runtime.GOOS == "windows" {
		c = exec.CommandContext(ctx, "cmd", "/C", cmd)
	} else {
		c = exec.CommandContext(ctx, "sh", "-c", cmd)
	}

	var stdout, stderr bytes.Buffer
	c.Stdout = &stdout
	c.Stderr = &stderr

	err := c.Run()
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return "", fmt.Errorf("timeout after %v", DefaultCommandTimeout)
		}
		errMsg := strings.TrimSpace(stderr.String())
		if errMsg != "" {
			return "", fmt.Errorf("exit %v: %s", err, errMsg)
		}
		return "", err
	}

	output := stdout.String()

	// Limita o tamanho do output
	if len(output) > MaxCommandOutputSize {
		output = output[:MaxCommandOutputSize] + "\n<!-- output truncated -->"
	}

	return output, nil
}
