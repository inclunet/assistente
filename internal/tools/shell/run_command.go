package shell

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"

	"assistente/internal/allowlist"
	"assistente/internal/commandpolicy"
	"assistente/internal/logging"
	"assistente/internal/shelltrust"
	"assistente/internal/terminal"
	"assistente/internal/tools"
	"assistente/internal/tools/invocationctx"
)

const (
	// FailurePolicyVetoed identifica um veto da política vigente.
	FailurePolicyVetoed = "shell_policy_vetoed"
	// FailureCommandRefused identifica a recusa expressa da pessoa.
	FailureCommandRefused = "shell_command_refused"
	// FailureConfirmationUnavailable indica que não houve uma decisão válida.
	FailureConfirmationUnavailable = "shell_confirmation_unavailable"
	// FailurePrerequisiteUnavailable identifica falha ao verificar um pré-requisito.
	FailurePrerequisiteUnavailable = "shell_prerequisite_unavailable"
	// FailureSessionStart identifica uma falha de abertura da sessão do terminal.
	FailureSessionStart = "shell_session_start_failed"

	// defaultTimeout é o timeout padrão para execução de comandos (15s)
	defaultTimeout = 15 * time.Second

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
type GetAllowlistFunc func(context.Context) (*allowlist.Allowlist, error)

// SessionManager é a interface para gerenciar sessões PTY.
// Permite mockar o Manager para testes.
type SessionManager interface {
	Acquire(ctx context.Context, workDir terminal.WorkingDirectory) (*terminal.Session, error)
	RunCommandWithDisplay(ctx context.Context, sessionID string, command terminal.CommandText, timeout time.Duration, requesterID string) (*terminal.HistoryEntry, error)
	RunEphemeral(ctx context.Context, workDir terminal.WorkingDirectory, command terminal.CommandText, timeout time.Duration, source string) (*terminal.HistoryEntry, error)
	Release(sessionID string)
	Close(sessionID string) error
}

// sessionLookup é implementada pelo terminal.Manager e permite que o chat
// escolha explicitamente uma sessão já conhecida sem ampliar o contrato dos
// mocks legados de SessionManager.
type sessionLookup interface {
	ExecutionDirectory(sessionID string) (terminal.WorkingDirectory, bool)
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
	return `Executes a shell command in a PTY. Use when a task requires a process, such as building, testing, Git, or a CLI; for example {"command":"go test ./...","timeout_seconds":120}. Do not use when a dedicated file/search tool can perform the operation, or merely to list, create, interrupt, or close terminals—use terminal_session for that lifecycle. By default each call is ephemeral and leaves no terminal tab. For a persistent or interactive workflow, set persistent=true, take terminalId from the result metadata, and pass it as terminal_id to later run_command calls. working_directory applies only when starting a new execution and cannot be combined with terminal_id. Risk: runs local commands, respects the command allowlist, and may require user confirmation; timeout_seconds is capped at 300. If unavailable, discover and load it with tool_catalog when the profile permits on-demand tools.`
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
				"description": "Timeout em segundos para a execução do comando. Padrão: 15, máximo: 300."
			}
		},
		"required": ["command"],
		"additionalProperties": false
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
	outputLimit := maxOutputForLLM
	if effective, explicit := tools.ExplicitMaxResultSizeFromContext(ctx); explicit {
		outputLimit = effective
	}

	// Resolve a sessão e o diretório exibido na confirmação antes de qualquer
	// efeito colateral. Um terminal existente é autoritativo sobre seu CWD.
	var workDir string
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
		directory, live := lookup.ExecutionDirectory(a.TerminalID)
		if !live {
			return tools.ToolResult{
				Content: fmt.Sprintf("Terminal %q não existe ou já foi encerrado", a.TerminalID),
				IsError: true,
			}, nil
		}
		workDir = directory.Path
		if directory.PublicOverride != "" {
			ctx = invocationctx.WithShellDirectoryMetadata(ctx, directory.PublicOverride)
		}
	} else {
		resolvedWorkDir, resolveErr := resolveProjectWorkDir(rc.workDir, a.WorkingDirectory)
		if errors.Is(resolveErr, errProjectDirectoryUnresolved) {
			return shellPrerequisiteFailure(ctx, "directory_unresolved", "Não foi possível resolver o diretório para autorização."), nil
		}
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
	policyResult, policyErr := rc.evaluateCommand(ctx, a.Command)
	if policyErr != nil {
		return shellPrerequisiteFailure(ctx, "policy_unavailable", "Política de autorização shell indisponível."), nil
	}
	decision := policyResult.Decision
	if decision == allowlist.DecisionApprove {
		tools.PublishAuthorizationApproval(ctx)
	}
	// Nunca logamos a string crua de a.Command: ela pode conter tokens em
	// flags (-W, --token=) ou env inline. Em vez disso, derivamos um resumo
	// seguro do parse (programas + contagem de args). Reasons so vao para o
	// log quando a decisao for diferente de approve, e mesmo assim usam
	// summarizePolicyReasons (sem repetir args do comando).
	commandSummary, sensitive := invocationctx.LookupShellCommandMetadata(ctx)
	publicPolicy := publicPolicyProjection(policyResult, sensitive)
	if !sensitive {
		commandSummary = redactCommandForLog(a.Command, policyResult)
	}
	if decision == allowlist.DecisionDeny {
		denied := tools.ToolResult{
			Failure: &tools.ToolFailure{Code: FailurePolicyVetoed, Kind: tools.ErrorKindAuthorization, Retryable: false},
			Content: fmt.Sprintf("Comando bloqueado pela política de comandos: %s\nMotivos: %s", commandSummary, publicPolicy.reasonText("; ")),
			IsError: true,
		}
		tools.PublishAuthorizationRefusal(ctx, FailurePolicyVetoed, denied)
		logPolicyDecision(ctx, nil, commandSummary, publicPolicy)
		return denied, nil
	}
	logPolicyDecision(ctx, nil, commandSummary, publicPolicy)

	if decision == allowlist.DecisionConfirm {
		if rc.confirmFn == nil {
			return unavailableShellAuthorization("Comando requer autorização, mas não há interlocutor disponível."), nil
		}
		// O autorizador reavalia o comando real e prepara a representação
		// que o interlocutor pode receber na superfície da chamada.
		ctx = invocationctx.WithPersistentShell(ctx, a.Persistent || a.TerminalID != "")
		if failure := confirmShellCommand(ctx, rc.confirmFn, a.Command, workDir, commandSummary); failure != nil {
			return *failure, nil
		}
	}

	if decision == allowlist.DecisionConfirm && a.TerminalID == "" {
		current, err := resolveProjectWorkDir(rc.workDir, a.WorkingDirectory)
		if err != nil || current != workDir {
			return shellPrerequisiteFailure(ctx, "directory_changed", "O diretório mudou durante a autorização."), nil
		}
	}
	// O executor pode ter devolvido o timeout enquanto a política ou o log
	// concluíam. O marcador de retry já está publicado antes desta checagem.
	if err := ctx.Err(); err != nil {
		return tools.ToolResult{}, err
	}
	command := terminal.CommandText{Raw: a.Command, Display: invocationctx.ShellCommandMetadata(ctx, a.Command)}
	directory := terminal.WorkingDirectory{Path: workDir, PublicOverride: invocationctx.ShellDirectoryMetadata(ctx)}
	var sessionID string
	var entry *terminal.HistoryEntry
	var err error
	if a.TerminalID != "" {
		sessionID = a.TerminalID
		entry, err = rc.sessionMgr.RunCommandWithDisplay(ctx, sessionID, command, timeout, "llm")
	} else if a.Persistent {
		// AEP-0089: Acquire cria uma sessão nova e nunca captura uma idle.
		session, acquireErr := rc.sessionMgr.Acquire(ctx, directory)
		err = acquireErr
		if err != nil {
			return sessionStartFailure(decision, err), nil
		}
		sessionID = session.ID()
		entry, err = rc.sessionMgr.RunCommandWithDisplay(ctx, sessionID, command, timeout, "llm")
		// Sessão persistente permanece viva (idle) para uso interativo.
		rc.sessionMgr.Release(sessionID)
	} else {
		// Execução efêmera por padrão: não cria aba persistente nem ocupa o limite.
		entry, err = rc.sessionMgr.RunEphemeral(ctx, directory, command, timeout, "llm")
		if errors.Is(err, terminal.ErrDirectoryUnresolved) {
			return shellPrerequisiteFailure(ctx, "directory_unresolved", "Não foi possível resolver o diretório para autorização."), nil
		}
		if errors.Is(err, terminal.ErrSessionStart) {
			return sessionStartFailure(decision, err), nil
		}
		// sessionID permanece vazio — sem deep link para terminal inexistente.
	}

	if err != nil {
		// Um prazo expirado pode entregar saída parcial. A duração da entrada
		// reflete a execução mesmo quando o contexto pai venceu primeiro.
		output, duration := "", ""
		if entry != nil {
			output = entry.Output
			duration = entry.EndedAt.Sub(entry.StartedAt).String()
		}

		isTimeout := errors.Is(err, context.DeadlineExceeded) && !terminalFailure(err)

		if isTimeout && output != "" {
			// Timeout COM output: retorna como sucesso com nota sobre timeout
			content := fmt.Sprintf("[TIMEOUT — duração registrada: %s. Output parcial capturado:]\n\n%s", duration, output)

			result := tools.ToolResult{
				Content: content,
				Metadata: func() map[string]any {
					m := map[string]any{
						"command":   command.Display,
						"workDir":   directory.Display(),
						"exitCode":  -1,
						"timeout":   true,
						"duration":  duration,
						"commandId": entry.ID,
					}
					if sessionID != "" {
						m["sessionId"] = sessionID
						m["terminalId"] = sessionID
						m["deepLink"] = deepLinkForSession(sessionID)
					}
					return m
				}(),
			}
			protected, ok := tools.ProtectToolResult(ctx, result, outputLimit)
			if !ok {
				return tools.ToolResult{
					Content: "Output parcial excede a capacidade segura de preservação.",
					IsError: true,
					Failure: &tools.ToolFailure{Code: "result_storage_limit", Kind: tools.ErrorKindUnknown, Retryable: false},
				}, nil
			}
			return protected, nil
		}

		// Erro real (sem output ou sem timeout)
		metadata := map[string]any{
			"command":  command.Display,
			"workDir":  directory.Display(),
			"exitCode": -1,
		}
		if sessionID != "" {
			metadata["sessionId"] = sessionID
			metadata["terminalId"] = sessionID
			metadata["deepLink"] = deepLinkForSession(sessionID)
		}
		message := fmt.Sprintf("Erro ao executar comando: %v", err)
		if directory.PublicOverride != "" {
			message = protectedExecutionError(err)
		}
		// A sessão fornece os horários na entrada de histórico. Sem ela, a duração é desconhecida.
		if entry != nil {
			message += "\nDuração registrada: " + duration
			metadata["commandId"] = entry.ID
			metadata["duration"] = duration
		}
		result := tools.ToolResult{
			Content:  message + "\n\nOutput parcial:\n" + output,
			IsError:  true,
			Metadata: metadata,
		}
		protected, ok := tools.ProtectToolResult(ctx, result, outputLimit)
		if !ok {
			return tools.ToolResult{
				Content: "Output parcial do erro excede a capacidade segura de preservação.",
				IsError: true,
				Failure: &tools.ToolFailure{Code: "result_storage_limit", Kind: tools.ErrorKindUnknown, Retryable: false},
			}, nil
		}
		return protected, nil
	}

	// Formata resultado
	content := entry.Output

	// Adiciona informação do exit code
	if entry.ExitCode != 0 {
		content = fmt.Sprintf("[exit code: %d]\n\n%s", entry.ExitCode, content)
	}

	result := tools.ToolResult{
		Content: content,
		Metadata: func() map[string]any {
			m := map[string]any{
				"command":   command.Display,
				"workDir":   directory.Display(),
				"exitCode":  entry.ExitCode,
				"duration":  entry.EndedAt.Sub(entry.StartedAt).String(),
				"commandId": entry.ID,
			}
			if sessionID != "" {
				m["sessionId"] = sessionID
				m["terminalId"] = sessionID
				m["deepLink"] = deepLinkForSession(sessionID)
			}
			return m
		}(),
	}
	if entry.ExitCode == 0 && len(entry.Output) > outputLimit && tools.IsCanonicalJSON(entry.Output) {
		result.Content = entry.Output
		result.Structured = true
		return result, nil
	}
	protected, ok := tools.ProtectToolResult(ctx, result, outputLimit)
	if !ok {
		return tools.ToolResult{
			Content: "Output excede a capacidade segura de preservação; reduza a saída do comando.",
			IsError: true,
			Failure: &tools.ToolFailure{Code: "result_storage_limit", Kind: tools.ErrorKindUnknown, Retryable: false},
		}, nil
	}
	return protected, nil
}

const protectedPolicyReason = "detalhes protegidos"

type publicPolicyResult struct {
	commandpolicy.EvaluationResult
	protected bool
}

func publicPolicyProjection(result commandpolicy.EvaluationResult, sensitive bool) publicPolicyResult {
	public := publicPolicyResult{EvaluationResult: result, protected: sensitive}
	if sensitive {
		public.Reasons = make([]string, len(result.Reasons))
		for i := range public.Reasons {
			public.Reasons[i] = protectedPolicyReason
		}
	}
	return public
}

// protectedExecutionError publica somente categorias conhecidas. Causas
// encadeadas do sistema operacional podem conter o caminho privado.
func protectedExecutionError(err error) string {
	for _, cause := range []error{terminal.ErrSessionRead, terminal.ErrSessionWrite, terminal.ErrSessionClosed, terminal.ErrSessionBusy, terminal.ErrSessionUnavailable} {
		if errors.Is(err, cause) {
			return "Erro ao executar comando: " + cause.Error()
		}
	}
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return "Prazo encerrado durante a execução do comando."
	case errors.Is(err, context.Canceled):
		return "Execução do comando cancelada."
	default:
		return "Erro ao executar comando no diretório protegido."
	}
}

func sessionStartFailure(decision allowlist.Decision, err error) tools.ToolResult {
	message, kind, transient := classifySessionStartError(err)
	return tools.ToolResult{
		Content: message, IsError: true,
		Failure: &tools.ToolFailure{
			Code: FailureSessionStart, Kind: kind, NotDispatched: true,
			Retryable: decision == allowlist.DecisionApprove && transient,
		},
	}
}

func classifySessionStartError(err error) (string, tools.ErrorKind, bool) {
	switch {
	case errors.Is(err, context.Canceled):
		return "Início da sessão de terminal cancelado.", tools.ErrorKindCancelled, false
	case errors.Is(err, context.DeadlineExceeded):
		return "Prazo encerrado ao iniciar a sessão de terminal.", tools.ErrorKindTimeout, false
	case errors.Is(err, terminal.ErrDirectoryUnresolved):
		return "Diretório indisponível ao iniciar a sessão de terminal.", tools.ErrorKindUnavailable, false
	case errors.Is(err, os.ErrNotExist), errors.Is(err, exec.ErrNotFound):
		return "Executável do terminal não encontrado.", tools.ErrorKindUnavailable, false
	case errors.Is(err, os.ErrPermission):
		return "Permissão negada ao iniciar a sessão de terminal.", tools.ErrorKindUnavailable, false
	case errors.Is(err, terminal.ErrSessionsDisabled):
		return "Sessões de terminal desabilitadas pela configuração.", tools.ErrorKindUnavailable, false
	case errors.Is(err, terminal.ErrSessionCapacity), errors.Is(err, syscall.EAGAIN), errors.Is(err, syscall.EBUSY):
		return "Recurso do terminal temporariamente indisponível.", tools.ErrorKindUnavailable, true
	default:
		return "Não foi possível iniciar a sessão de terminal.", tools.ErrorKindUnavailable, false
	}
}

func terminalFailure(err error) bool {
	return errors.Is(err, terminal.ErrSessionWrite) || errors.Is(err, terminal.ErrSessionRead) || errors.Is(err, terminal.ErrSessionClosed)
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

func logPolicyDecision(
	ctx context.Context,
	logger *slog.Logger,
	commandSummary string,
	result publicPolicyResult,
) {
	if ctx == nil {
		ctx = context.Background()
	}
	if logger == nil {
		logger = logging.Logger(ctx, "tools.shell.run-command")
	}
	if !logger.Enabled(ctx, slog.LevelInfo) {
		return
	}
	decision := result.Decision
	decisionText := decision.String()
	attrs := []any{
		slog.String("command_summary", commandSummary),
		slog.String("decision", decisionText),
	}
	message := fmt.Sprintf("Comando: %s, decisão: %s", commandSummary, decisionText)
	if decision != allowlist.DecisionApprove {
		reasons := summarizePolicyReasons(result)
		message += fmt.Sprintf(", motivos: %s", reasons)
		attrs = append(attrs, slog.String("reasons", reasons))
	}
	logger.Log(ctx, slog.LevelInfo, message, attrs...)
}

func confirmShellCommand(ctx context.Context, confirm ConfirmFunc, command, workDir, commandSummary string) *tools.ToolResult {
	denied := tools.ToolResult{
		Failure: &tools.ToolFailure{Code: FailureCommandRefused, Kind: tools.ErrorKindAuthorization},
		Content: fmt.Sprintf("Comando negado pelo usuário: %s", commandSummary),
		IsError: true,
	}
	resolve, receipt := tools.PendingConfirmation(ctx, FailureConfirmationUnavailable, FailureCommandRefused, denied.Content)
	if ctx.Err() != nil {
		result := unavailableShellAuthorization("Confirmação encerrada sem autorização válida.")
		return &result
	}
	var failure *tools.ToolResult
	approvedInTime := false
	// O autorizador real publica antes do seu log. Os demais ConfirmFunc
	// entregam o mesmo resultado ao retornar, sem emitir uma segunda decisão.
	observe := func(outcome shelltrust.DecisionOutcome, err error) {
		if failure != nil {
			return
		}
		switch {
		case err == nil && outcome == shelltrust.Approved:
			if ctx.Err() == nil {
				approvedInTime = true
				resolve(nil)
			}
		case err == nil:
			if outcome == shelltrust.DeniedByPolicy {
				denied.Failure.Code = FailurePolicyVetoed
				denied.Content = fmt.Sprintf("Comando bloqueado pela política de comandos: %s\nMotivos: a política vigente vetou o comando na reavaliação.", commandSummary)
				// resolve aceita apenas o código de recusa da pessoa. Publique o
				// veto com seu próprio código antes que o prazo encerre a espera.
				tools.PublishAuthorizationRefusal(ctx, FailurePolicyVetoed, denied)
			} else {
				resolve(&denied)
			}
			failure = &denied
		case errors.Is(err, shelltrust.ErrPrerequisiteUnavailable) && ctx.Err() == nil:
			unavailable := shellPrerequisiteFailure(ctx, shelltrust.PrerequisiteReason(err), "Não foi possível validar os pré-requisitos da autorização shell.")
			failure = &unavailable
		}
	}
	approved, err := confirm(shelltrust.WithDecisionObserver(ctx, observe, receipt), command, workDir)
	outcome := shelltrust.DeniedByUser
	if approved {
		outcome = shelltrust.Approved
	}
	observe(outcome, err)
	if failure != nil {
		return failure
	}
	if ctx.Err() != nil {
		if approvedInTime {
			// A confirmação terminou; o prazo agora é da operação. Sem Failure,
			// o executor classifica o contexto mesmo se o worker chegar primeiro.
			return &tools.ToolResult{IsError: true, Content: "Execução interrompida após autorização."}
		}
		// O log preserva a causa operacional; o resultado informa que o prazo
		// encerrou a confirmação antes de uma autorização válida.
		if errors.Is(err, shelltrust.ErrPrerequisiteUnavailable) {
			warnShellPrerequisite(ctx, shelltrust.PrerequisiteReason(err))
		}
		result := unavailableShellAuthorization("Confirmação encerrada sem autorização válida.")
		return &result
	}
	if err != nil {
		result := unavailableShellAuthorization(fmt.Sprintf("Erro ao solicitar confirmação: %v", err))
		return &result
	}
	return nil
}

func shellPrerequisiteFailure(ctx context.Context, reason, message string) tools.ToolResult {
	failure := tools.ToolResult{IsError: true, Content: message,
		Failure: &tools.ToolFailure{Code: FailurePrerequisiteUnavailable, Kind: tools.ErrorKindUnavailable, Retryable: false},
	}
	tools.PublishAuthorizationPrerequisiteFailure(ctx, failure)
	warnShellPrerequisite(ctx, reason)
	return failure
}

func warnShellPrerequisite(ctx context.Context, reason string) {
	logging.Logger(ctx, "tools.shell.run-command").WarnContext(ctx, "Pré-requisito da autorização shell indisponível", "reason", reason)
}

func unavailableShellAuthorization(message string) tools.ToolResult {
	return tools.ToolResult{IsError: true, Content: message,
		Failure: &tools.ToolFailure{Code: FailureConfirmationUnavailable, Kind: tools.ErrorKindAuthorization, Retryable: false},
	}
}

// evaluateCommand avalia o comando passando pelo pipeline do commandpolicy:
// parsing conservador da linha (separa atomos, detecta features de shell
// como redirecionamentos, pipes e substituicao de comando) e agregacao de
// decisoes (deny/confirm/approve) consultando a allowlist ativa do perfil.
// Quando nao ha allowlist configurada, o resultado e sempre confirm.
func (rc *RunCommand) evaluateCommand(ctx context.Context, command string) (commandpolicy.EvaluationResult, error) {
	if rc.getAllowlistFn == nil {
		return commandpolicy.Evaluate(command, nil), nil
	}

	al, err := rc.getAllowlistFn(ctx)
	if err != nil {

		return commandpolicy.EvaluationResult{Decision: allowlist.DecisionDeny, Parse: commandpolicy.Parse(command), Reasons: []string{"política de autorização indisponível"}}, err
	}
	return commandpolicy.Evaluate(command, al), nil
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

func (p publicPolicyResult) reasonText(separator string) string {
	if p.protected {
		return protectedPolicyReason
	}
	return strings.Join(p.Reasons, separator)
}

// summarizePolicyReasons gera um resumo curto para log LOCAL. Como esses
// logs podem ser anexados a bug reports ou copiados manualmente, usamos
// EXCLUSIVAMENTE Reasons (safe, sem patterns/description) — DetailedReasons
// fica reservado para uso ao vivo na UI do desktop, onde o usuario ja tem
// visibilidade do conteudo da allowlist e nao ha risco de envio externo.
func summarizePolicyReasons(result publicPolicyResult) string {
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
		parts = append(parts, "reasons=["+result.reasonText(" | ")+"]")
	}
	return strings.Join(parts, " ")
}

func deepLinkForSession(sessionID string) string {
	if sessionID == "" {
		return ""
	}
	return fmt.Sprintf("assistente://terminal/%s", sessionID)
}
