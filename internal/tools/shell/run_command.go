package shell

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strconv"
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

	if strings.TrimSpace(a.Command) == "" {
		return tools.ToolResult{
			Content: "O parâmetro 'command' é obrigatório e não pode ser vazio",
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

	// Avalia o comando contra a politica (allowlist + parser conservador)
	policyResult := rc.evaluateCommand(a.Command)
	decision := policyResult.Decision
	// Nunca logamos a string crua de a.Command: ela pode conter tokens em
	// flags (-W, --token=) ou env inline. Em vez disso, derivamos um resumo
	// seguro do parse (programas + contagem de args). Reasons so vao para o
	// log quando a decisao for diferente de approve, e mesmo assim usam
	// summarizePolicyReasons (sem repetir args do comando).
	commandSummary := redactCommandForLog(a.Command, policyResult)
	if decision == allowlist.DecisionApprove {
		log.Printf("[RunCommand] Comando: %s, decisão: %s", commandSummary, decision)
	} else {
		log.Printf("[RunCommand] Comando: %s, decisão: %s, motivos: %s", commandSummary, decision, summarizePolicyReasons(policyResult))
	}

	switch decision {
	case allowlist.DecisionDeny:
		// Nao retornamos a.Command cru: este Content e enviado ao LLM e poderia
		// vazar tokens/senhas em flags ou env inline. Usamos o mesmo resumo
		// redigido aplicado nos logs (programas + contagem de args). Reasons
		// (vs DetailedReasons) e o slice "safe" do EvaluationResult — citamos
		// apenas programa, tipo de regra e indice (rule[N]/always_deny[N]) e
		// nunca interpolamos pattern bruto, subcommands/args/description que o
		// usuario possa ter colocado na allowlist com dados sensiveis.
		return tools.ToolResult{
			Content: fmt.Sprintf("Comando bloqueado pela política de comandos: %s\nMotivos: %s", commandSummary, strings.Join(policyResult.Reasons, "; ")),
			IsError: true,
		}, nil

	case allowlist.DecisionConfirm:
		if rc.confirmFn != nil {
			// confirmFn recebe o comando bruto: o usuario precisa ver tudo na
			// UI local para decidir, e esse caminho nao vai para o LLM.
			approved, err := rc.confirmFn(ctx, a.Command, workDir)
			if err != nil {
				return tools.ToolResult{
					Content: fmt.Sprintf("Erro ao solicitar confirmação: %v", err),
					IsError: true,
				}, nil
			}
			if !approved {
				// Mesmo motivo do deny acima: a string vai para o LLM, nao
				// repetimos o comando bruto aqui.
				return tools.ToolResult{
					Content: fmt.Sprintf("Comando negado pelo usuário: %s", commandSummary),
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

// evaluateCommand avalia o comando passando pelo pipeline do commandpolicy:
// parsing conservador da linha (separa atomos, detecta features de shell
// como redirecionamentos, pipes e substituicao de comando) e agregacao de
// decisoes (deny/confirm/approve) consultando a allowlist ativa do perfil.
// Quando nao ha allowlist configurada, o resultado e sempre confirm.
func (rc *RunCommand) evaluateCommand(command string) commandpolicy.EvaluationResult {
	if rc.getAllowlistFn == nil {
		return commandpolicy.Evaluate(command, nil)
	}

	al := rc.getAllowlistFn()
	return commandpolicy.Evaluate(command, al)
}

// redactCommandForLog devolve uma representacao segura do command line
// para log. Em vez de imprimir a.Command (que pode conter tokens, senhas
// em flags ou env inline), exibimos:
//   - lista de programas detectados pelo parser, separados por " | ";
//   - quando algum atomo nao tem programa identificado (parse vazio),
//     fallback para "<unparsed:N bytes>" com o tamanho original.
//
// Evita expor args/values mas preserva diagnostico minimo (qual ferramenta
// foi pedida).
func redactCommandForLog(command string, result commandpolicy.EvaluationResult) string {
	if len(result.Parse.Commands) == 0 {
		return fmt.Sprintf("<unparsed:%d bytes>", len(command))
	}
	programs := make([]string, 0, len(result.Parse.Commands))
	for _, cmd := range result.Parse.Commands {
		program := cmd.Program
		if program == "" {
			program = "<empty>"
		}
		programs = append(programs, fmt.Sprintf("%s(%d args)", program, len(cmd.Args)))
	}
	return strings.Join(programs, " | ")
}

// summarizePolicyReasons gera um resumo curto para log LOCAL. Diferente do
// Content enviado ao LLM, o log fica na maquina do usuario, entao incluimos
// as DetailedReasons (que citam patterns/subcommands/description). Mesmo
// assim evitamos repetir args do comando bruto: o resumo agrega contagens,
// features detectadas e os motivos verbosos resumidos por palavras-chave.
func summarizePolicyReasons(result commandpolicy.EvaluationResult) string {
	parts := make([]string, 0, 5)
	parts = append(parts, fmt.Sprintf("atomos=%d", len(result.Parse.Commands)))
	parts = append(parts, fmt.Sprintf("motivos=%d", len(result.Reasons)))
	if len(result.Parse.Features) > 0 {
		featureNames := make([]string, 0, len(result.Parse.Features))
		for _, f := range result.Parse.Features {
			featureNames = append(featureNames, string(f))
		}
		parts = append(parts, "features=["+strings.Join(featureNames, ",")+"]")
	}
	if len(result.Parse.Errors) > 0 {
		parts = append(parts, "parse_errors="+strconv.Itoa(len(result.Parse.Errors)))
	}
	if len(result.DetailedReasons) > 0 {
		parts = append(parts, "detail=["+strings.Join(result.DetailedReasons, " | ")+"]")
	}
	return strings.Join(parts, " ")
}
