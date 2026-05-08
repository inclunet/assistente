package shell

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"assistente/internal/allowlist"
	"assistente/internal/commandpolicy"
	"assistente/internal/terminal"
	"assistente/internal/tools"
)

const (
	// defaultTimeout é o timeout padrão para execução de comandos (30s)
	defaultTimeout = 30 * time.Second

	// maxTimeout é o timeout máximo permitido (5 minutos)
	maxTimeout = 5 * time.Minute

	// maxOutputForLLM é o tamanho máximo de output retornado ao LLM
	maxOutputForLLM = 50 * 1024
)

// ConfirmFunc é a função chamada para solicitar confirmação ao usuário.
// Retorna true se aprovado, false se negado.
type ConfirmFunc func(ctx context.Context, command, workDir string) (bool, error)

// GetAllowlistFunc é a função que retorna a allowlist ativa do perfil.
// Retorna nil se nenhuma allowlist está configurada.
type GetAllowlistFunc func() *allowlist.Allowlist

// SessionManager é a interface para gerenciar sessões PTY.
// Permite mockar o Manager para testes.
type SessionManager interface {
	Acquire(ctx context.Context, workDir string) (*terminal.Session, error)
	RunCommand(ctx context.Context, sessionID string, command string, timeout time.Duration, requesterID string) (*terminal.HistoryEntry, error)
	Release(sessionID string)
}

// RunCommand é a ferramenta que executa comandos shell via PTY.
// Suporta allowlist para controle de acesso e confirmação do usuário.
type RunCommand struct {
	sessionMgr     SessionManager
	confirmFn      ConfirmFunc
	getAllowlistFn GetAllowlistFunc
	workDir        string
}

// NewRunCommand cria uma nova instância da ferramenta run_command.
func NewRunCommand(
	sessionMgr SessionManager,
	confirmFn ConfirmFunc,
	getAllowlistFn GetAllowlistFunc,
	workDir string,
) *RunCommand {
	return &RunCommand{
		sessionMgr:     sessionMgr,
		confirmFn:      confirmFn,
		getAllowlistFn: getAllowlistFn,
		workDir:        workDir,
	}
}

func (rc *RunCommand) Name() string {
	return "run_command"
}

func (rc *RunCommand) Description() string {
	return `Runs a shell command in a persistent PTY session. Use for builds, tests, file inspection, git, etc. Respects allowlist and may require user confirmation. working_directory is project-relative; timeout_seconds max is 300.`
}

func (rc *RunCommand) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"command": {
				"type": "string",
				"description": "O comando a ser executado no shell (ex: 'git status', 'go test ./...', 'ls -la')"
			},
			"working_directory": {
				"type": "string",
				"description": "Diretório de trabalho para execução do comando. Caminho relativo ao diretório do projeto. Se omitido, usa o diretório raiz do projeto."
			},
			"timeout_seconds": {
				"type": "integer",
				"description": "Timeout em segundos para a execução do comando. Padrão: 30, máximo: 300."
			}
		},
		"required": ["command"]
	}`)
}

// runCommandArgs são os argumentos parseados da chamada.
type runCommandArgs struct {
	Command          string `json:"command"`
	WorkingDirectory string `json:"working_directory"`
	TimeoutSeconds   int    `json:"timeout_seconds"`
}

func (rc *RunCommand) Execute(ctx context.Context, args json.RawMessage) (tools.ToolResult, error) {
	// Parse argumentos
	var a runCommandArgs
	if err := json.Unmarshal(args, &a); err != nil {
		return tools.ToolResult{
			Content: fmt.Sprintf("Erro ao parsear argumentos: %v", err),
			IsError: true,
		}, nil
	}

	if a.Command == "" {
		return tools.ToolResult{
			Content: "O parâmetro 'command' é obrigatório",
			IsError: true,
		}, nil
	}

	// Resolve diretório de trabalho
	workDir := rc.workDir
	if a.WorkingDirectory != "" {
		workDir = a.WorkingDirectory
	}

	// Calcula timeout
	timeout := defaultTimeout
	if a.TimeoutSeconds > 0 {
		timeout = time.Duration(a.TimeoutSeconds) * time.Second
		if timeout > maxTimeout {
			timeout = maxTimeout
		}
	}

	// Avalia allowlist
	policyResult := rc.evaluateCommand(a.Command)
	decision := policyResult.Decision
	log.Printf("[RunCommand] Comando: %q, decisão: %s, motivos: %v", a.Command, decision, policyResult.Reasons)

	switch decision {
	case allowlist.DecisionDeny:
		return tools.ToolResult{
			Content: fmt.Sprintf("Comando bloqueado pela allowlist: %q\nMotivos: %s", a.Command, strings.Join(policyResult.Reasons, "; ")),
			IsError: true,
		}, nil

	case allowlist.DecisionConfirm:
		if rc.confirmFn != nil {
			approved, err := rc.confirmFn(ctx, a.Command, workDir)
			if err != nil {
				return tools.ToolResult{
					Content: fmt.Sprintf("Erro ao solicitar confirmação: %v", err),
					IsError: true,
				}, nil
			}
			if !approved {
				return tools.ToolResult{
					Content: fmt.Sprintf("Comando negado pelo usuário: %q", a.Command),
					IsError: true,
				}, nil
			}
		}
	}

	// Adquire sessão PTY
	session, err := rc.sessionMgr.Acquire(ctx, workDir)
	if err != nil {
		return tools.ToolResult{
			Content: fmt.Sprintf("Erro ao obter sessão de terminal: %v", err),
			IsError: true,
		}, nil
	}

	// Executa o comando
	entry, err := rc.sessionMgr.RunCommand(ctx, session.ID(), a.Command, timeout, "llm")

	// Libera a sessão para uso futuro (mesmo com erro)
	rc.sessionMgr.Release(session.ID())

	if err != nil {
		// Timeout ou erro — mas se temos output parcial, retorna como sucesso
		// para que o LLM possa analisar o resultado parcial
		output := ""
		if entry != nil {
			output = entry.Output
		}

		isTimeout := entry != nil && entry.ExitCode == -1

		if isTimeout && output != "" {
			// Timeout COM output: retorna como sucesso com nota sobre timeout
			content := output
			if len(content) > maxOutputForLLM {
				content = content[:maxOutputForLLM] + fmt.Sprintf(
					"\n\n[TRUNCADO: output original tinha %d bytes]", len(output),
				)
			}
			content = fmt.Sprintf("[TIMEOUT após %ds — o comando foi interrompido com Ctrl+C. Output parcial capturado:]\n\n%s", int(timeout.Seconds()), content)

			return tools.ToolResult{
				Content: content,
				Metadata: map[string]any{
					"command":   a.Command,
					"workDir":   workDir,
					"exitCode":  -1,
					"timeout":   true,
					"duration":  timeout.String(),
					"sessionId": session.ID(),
				},
			}, nil
		}

		// Erro real (sem output ou sem timeout)
		return tools.ToolResult{
			Content: fmt.Sprintf("Erro ao executar comando: %v\n\nOutput parcial:\n%s", err, output),
			IsError: true,
			Metadata: map[string]any{
				"command":  a.Command,
				"workDir":  workDir,
				"exitCode": -1,
			},
		}, nil
	}

	// Formata resultado
	content := entry.Output
	if len(content) > maxOutputForLLM {
		content = content[:maxOutputForLLM] + fmt.Sprintf(
			"\n\n[TRUNCADO: output original tinha %d bytes]", len(entry.Output),
		)
	}

	// Adiciona informação do exit code
	if entry.ExitCode != 0 {
		content = fmt.Sprintf("[exit code: %d]\n\n%s", entry.ExitCode, content)
	}

	return tools.ToolResult{
		Content: content,
		Metadata: map[string]any{
			"command":   a.Command,
			"workDir":   workDir,
			"exitCode":  entry.ExitCode,
			"duration":  entry.EndedAt.Sub(entry.StartedAt).String(),
			"sessionId": session.ID(),
		},
	}, nil
}

// evaluateCommand avalia o comando contra a allowlist ativa.
func (rc *RunCommand) evaluateCommand(command string) commandpolicy.EvaluationResult {
	if rc.getAllowlistFn == nil {
		return commandpolicy.Evaluate(command, nil)
	}

	al := rc.getAllowlistFn()
	return commandpolicy.Evaluate(command, al)
}
