package shell

import (
	"assistente/internal/logging"
	"context"
	"encoding/json"
	"fmt"
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
	RunEphemeral(ctx context.Context, workDir, command string, timeout time.Duration, source string) (*terminal.HistoryEntry, error)
	Release(sessionID string)
	Close(sessionID string) error
}

// sessionLookup é implementada pelo terminal.Manager e permite que o chat
// escolha explicitamente uma sessão já conhecida sem ampliar o contrato dos
// mocks legados de SessionManager.
type sessionLookup interface {
	Info(sessionID string) (terminal.SessionInfo, bool)
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

// CatalogMetadata declara os metadados de catálogo da tool (AEP-0077, Fase 1).
func (rc *RunCommand) CatalogMetadata() tools.CatalogMetadata {
	return tools.CatalogMetadata{Category: "shell", Class: "run_commands", Package: "coding_edit", Risk: "shell"}
}

func (rc *RunCommand) Description() string {
	return `Runs a shell command. By default it runs as a single ephemeral execution without leaving a persistent terminal tab; pass persistent=true to keep a terminal section alive for interactive use. Pass terminal_id to use exactly one live terminal returned by terminal_session; omit it to create a new execution. working_directory applies only to a new execution and cannot be combined with terminal_id. Results include a deep link for inspection when persistent or terminal_id is used (ephemeral executions have no terminal link). Respects allowlist and may require user confirmation. timeout_seconds max is 300.`
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
				"description": "Diretório inicial da nova sessão, relativo ao projeto. Só pode ser usado quando terminal_id é omitido; com terminal_id, o comando usa o diretório atual daquela sessão."
			},
			"terminal_id": {
				"type": "string",
				"description": "ID de um terminal vivo escolhido explicitamente. Se omitido, uma nova execução é criada; nenhuma sessão existente é reutilizada silenciosamente."
			},
			"persistent": {
				"type": "boolean",
				"description": "Se true, mantém a seção de terminal persistente após o comando (útil para sessões interativas). Padrão: false (execução única, sem lotar terminais)."
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
	TerminalID       string `json:"terminal_id"`
	Persistent       bool   `json:"persistent"`
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
	if result, blocked := validateSkillBashCommand(ctx, a.Command); blocked {
		return result, nil
	}

	// Resolve a sessão e o diretório exibido na confirmação antes de qualquer
	// efeito colateral. Um terminal existente é autoritativo sobre seu CWD.
	workDir := rc.workDir
	if a.TerminalID != "" {
		if strings.TrimSpace(a.WorkingDirectory) != "" {
			return tools.ToolResult{
				Content: "working_directory não pode ser combinado com terminal_id; o terminal selecionado mantém seu próprio diretório atual",
				IsError: true,
			}, nil
		}
		lookup, ok := rc.sessionMgr.(sessionLookup)
		if !ok {
			return tools.ToolResult{Content: "O gerenciador não suporta seleção explícita de terminal", IsError: true}, nil
		}
		info, live := lookup.Info(a.TerminalID)
		if !live {
			return tools.ToolResult{
				Content: fmt.Sprintf("Terminal %q não existe ou já foi encerrado", a.TerminalID),
				IsError: true,
			}, nil
		}
		workDir = info.CWD
	} else if a.WorkingDirectory != "" {
		resolvedWorkDir, resolveErr := resolveProjectWorkDir(rc.workDir, a.WorkingDirectory)
		if resolveErr != nil {
			return tools.ToolResult{
				Content: "working_directory inválido: " + resolveErr.Error(),
				IsError: true,
			}, nil
		}
		workDir = resolvedWorkDir
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
		logging.Infof(ctx, "tools.shell.run-command", "[RunCommand] Comando: %s, decisão: %s", commandSummary, decision)
	} else {
		logging.Infof(ctx, "tools.shell.run-command", "[RunCommand] Comando: %s, decisão: %s, motivos: %s", commandSummary, decision, summarizePolicyReasons(policyResult))
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

	var sessionID string
	var entry *terminal.HistoryEntry
	var err error
	if a.TerminalID != "" {
		sessionID = a.TerminalID
		entry, err = rc.sessionMgr.RunCommand(ctx, sessionID, a.Command, timeout, "llm")
	} else if a.Persistent {
		// AEP-0089: Acquire cria uma sessão nova e nunca captura uma idle.
		session, acquireErr := rc.sessionMgr.Acquire(ctx, workDir)
		err = acquireErr
		if err != nil {
			return tools.ToolResult{
				Content: fmt.Sprintf("Erro ao criar sessão de terminal: %v", err),
				IsError: true,
			}, nil
		}
		sessionID = session.ID()
		entry, err = rc.sessionMgr.RunCommand(ctx, sessionID, a.Command, timeout, "llm")
		// Sessão persistente permanece viva (idle) para uso interativo.
		rc.sessionMgr.Release(sessionID)
	} else {
		// Execução efêmera por padrão: não cria aba persistente nem ocupa o limite.
		entry, err = rc.sessionMgr.RunEphemeral(ctx, workDir, a.Command, timeout, "llm")
		// sessionID permanece vazio — sem deep link para terminal inexistente.
	}

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
					"command":    a.Command,
					"workDir":    workDir,
					"exitCode":   -1,
					"timeout":    true,
					"duration":   timeout.String(),
					"sessionId":  sessionID,
					"terminalId": sessionID,
					"commandId":  entry.ID,
					"deepLink":   deepLinkForSession(sessionID),
				},
			}, nil
		}

		// Erro real (sem output ou sem timeout)
		metadata := map[string]any{
			"command":    a.Command,
			"workDir":    workDir,
			"exitCode":   -1,
			"sessionId":  sessionID,
			"terminalId": sessionID,
			"deepLink":   deepLinkForSession(sessionID),
		}
		if entry != nil {
			metadata["commandId"] = entry.ID
		}
		return tools.ToolResult{
			Content:  fmt.Sprintf("Erro ao executar comando: %v\n\nOutput parcial:\n%s", err, output),
			IsError:  true,
			Metadata: metadata,
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
			"command":    a.Command,
			"workDir":    workDir,
			"exitCode":   entry.ExitCode,
			"duration":   entry.EndedAt.Sub(entry.StartedAt).String(),
			"sessionId":  sessionID,
			"terminalId": sessionID,
			"commandId":  entry.ID,
			"deepLink":   deepLinkForSession(sessionID),
		},
	}, nil
}

func validateSkillBashCommand(ctx context.Context, command string) (tools.ToolResult, bool) {
	ec, ok := tools.GetExecutionContext(ctx)
	if !ok {
		return tools.ToolResult{}, false
	}
	if containsString(ec.DeniedBash, command) {
		return tools.ToolResult{
			Content: fmt.Sprintf("Comando bloqueado pela denylist do skill '%s'", ec.InvokedSkillSlug),
			IsError: true,
		}, true
	}
	if len(ec.AllowedBash) > 0 && !containsString(ec.AllowedBash, command) {
		return tools.ToolResult{
			Content: fmt.Sprintf("Skill '%s' não permite executar este comando", ec.InvokedSkillSlug),
			IsError: true,
		}, true
	}
	return tools.ToolResult{}, false
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
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
//   - contagem de env assignments (sem valores) e de args para cada atomo;
//   - quando algum atomo nao tem programa identificado (parse vazio),
//     fallback para "<unparsed:N bytes>" com o tamanho original.
//
// Evita expor args/values mas preserva diagnostico minimo (qual ferramenta
// foi pedida e se houve env inline). Aplica defesa em profundidade via
// redactProgramSegment para o caso raro em que o parser deixe escapar um
// Program contendo "=" (perfis legados, configuracoes manuais).
func redactCommandForLog(command string, result commandpolicy.EvaluationResult) string {
	if len(result.Parse.Commands) == 0 {
		return fmt.Sprintf("<unparsed:%d bytes>", len(command))
	}
	programs := make([]string, 0, len(result.Parse.Commands))
	for _, cmd := range result.Parse.Commands {
		program := redactProgramSegment(cmd.Program)
		if program == "" {
			program = "<empty>"
		}
		segment := fmt.Sprintf("%s(%d args)", program, len(cmd.Args))
		if envCount := len(cmd.EnvAssignments); envCount > 0 {
			segment = fmt.Sprintf("[env=%d]%s", envCount, segment)
		}
		programs = append(programs, segment)
	}
	return strings.Join(programs, " | ")
}

// redactProgramSegment redige qualquer "=" no nome do programa (defesa em
// profundidade contra Programs que escaparam do parser ainda contendo
// atribuicoes inline). Mesma logica do redactProgramForReason no evaluator.
func redactProgramSegment(program string) string {
	eq := strings.IndexByte(program, '=')
	if eq < 0 {
		return program
	}
	return program[:eq] + "=<redacted>"
}

// summarizePolicyReasons gera um resumo curto para log LOCAL. Como esses
// logs podem ser anexados a bug reports ou copiados manualmente, usamos
// EXCLUSIVAMENTE Reasons (safe, sem patterns/description) — DetailedReasons
// fica reservado para uso ao vivo na UI do desktop, onde o usuario ja tem
// visibilidade do conteudo da allowlist e nao ha risco de envio externo.
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
	if len(result.Reasons) > 0 {
		parts = append(parts, "reasons=["+strings.Join(result.Reasons, " | ")+"]")
	}
	return strings.Join(parts, " ")
}

func deepLinkForSession(sessionID string) string {
	if sessionID == "" {
		return ""
	}
	return fmt.Sprintf("assistente://terminal/%s", sessionID)
}
