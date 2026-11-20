package app

import (
	"context"
	"errors"
	"log/slog"
	"regexp"
	"time"

	"assistente/internal/commandcatalog"
	"assistente/internal/commandexecution"
	"assistente/internal/commandledger"
	"assistente/internal/commandui"
	"assistente/internal/logging"
	"assistente/internal/workspace"
	"github.com/google/uuid"
)

var diagnosticCommandID = regexp.MustCompile(`^[a-z][a-z0-9_]*(\.[a-z][a-z0-9_]*)+$`)

const localActionAccepted commandledger.Status = "accepted"
const localActionNotCompleted commandledger.Status = "not_completed"

func diagnosticRegisteredCommand(a *App, commandID string) string {
	if a != nil {
		return diagnosticCommandForProduct(a.commandProduct.Load(), commandID)
	}
	return diagnosticCommandForProduct(nil, commandID)
}

func diagnosticCommandForProduct(p *commandProductRuntime, commandID string) string {
	if p != nil && p.registry != nil {
		if _, ok := p.registry.Lookup(commandID); ok {
			return commandID
		}
		return "unknown"
	}
	// Política fechada e independente do lifecycle: nunca aceite prefixos ou
	// identificadores dinâmicos quando o catálogo estiver indisponível.
	if isWorkspaceMutationCommand(commandID) || commandExecutionClassForID(commandID) == commandExecutionLocalUI || commandID == commandProductWorkspaceListID || isCommandLayerAction(commandID) || isEditorFormatCommand(commandID) || isChatMessageCommand(commandID) {
		return commandID
	}
	return "unknown"
}

type commandDiagnosticRecord struct {
	level slog.Level
	attrs []slog.Attr
}

// Fila limitada: a lentidão do disco nunca aguarda nem replica um comando.
// Sob pressão, um diagnóstico pode ser descartado; a execução não é afetada.
var commandDiagnostics = make(chan commandDiagnosticRecord, 128)

func init() {
	go func() {
		for record := range commandDiagnostics {
			writeCommandDiagnostic(record)
		}
	}()
}

func writeCommandDiagnostic(record commandDiagnosticRecord) {
	logging.Logger(context.Background(), "app.command-execution").LogAttrs(context.Background(), record.level, "Command execution", record.attrs...)
}

// Só são registrados identificadores e classificações, nunca argumentos,
// tickets, conteúdo, caminhos ou mensagens arbitrárias de erro.
func logCommandExecution(commandID, invocationID, source, stage string, start time.Time, status commandledger.Status, code *string, err error) {
	if len(commandID) > 128 || !diagnosticCommandID.MatchString(commandID) {
		commandID = "unknown"
	}
	if _, parseErr := uuid.Parse(invocationID); parseErr != nil {
		invocationID = ""
	}
	switch commandcatalog.Source(source) {
	case commandcatalog.KeyboardLocal, commandcatalog.KeyboardGlobal, commandcatalog.StreamDeck, commandcatalog.Palette, commandcatalog.UI, commandcatalog.Chat, commandcatalog.CLI, commandcatalog.Event, commandcatalog.System:
	default:
		source = "unknown"
	}
	attrs := []slog.Attr{slog.String("command_id", commandID), slog.String("invocation_id", invocationID), slog.String("source", source), slog.String("stage", stage), slog.Int64("elapsed_ms", time.Since(start).Milliseconds())}
	if status != "" {
		attrs = append(attrs, slog.String("status", string(status)))
	}
	if code != nil {
		safeCode := "other"
		for _, value := range []string{"command_failed", "command_denied", "command_cancelled", "command_cancelled_stale", "command_timed_out", "command_outcome_unknown", "command_suppressed", "command_rejected_stale"} {
			if *code == value {
				safeCode = value
				break
			}
		}
		attrs = append(attrs, slog.String("error_code", safeCode))
	}
	level := slog.LevelInfo
	if err != nil {
		level = slog.LevelWarn
		attrs = append(attrs, slog.String("error_class", commandExecutionErrorClass(err)))
	} else if status != "" && status != commandledger.Succeeded && status != localActionAccepted {
		level = slog.LevelWarn
	}
	enqueueCommandDiagnostic(commandDiagnostics, commandDiagnosticRecord{level: level, attrs: attrs})
}

func enqueueCommandDiagnostic(queue chan<- commandDiagnosticRecord, record commandDiagnosticRecord) bool {
	select {
	case queue <- record:
		return true
	default:
		return false
	}
}

func commandExecutionErrorClass(err error) string {
	for _, item := range []struct {
		err  error
		name string
	}{
		{workspace.ErrCommandCreateTabStale, "workspace_tab_stale"},
		{workspace.ErrCommandCreateTabUnsupportedType, "workspace_tab_type"},
		{workspace.ErrCommandCreateTabDuplicateID, "workspace_tab_duplicate"},
		{workspace.ErrCommandSnapshotUninitialized, "workspace_uninitialized"},
		{workspace.ErrCommandSnapshotActiveTabUnavailable, "workspace_active_tab_unavailable"},
		{commandui.ErrNotFound, "handoff_not_found"}, {commandui.ErrNotStarted, "handoff_not_started"},
		{commandui.ErrExpired, "handoff_expired"}, {commandui.ErrOutcomeUnknown, "outcome_unknown"},
	} {
		if errors.Is(err, item.err) {
			return item.name
		}
	}
	return commandLoadErrorClass(err)
}

// ReportLocalPaletteCommand registra apenas o resultado de apresentação local.
// Não autoriza nem executa comandos; não recebe argumentos ou alvos da UI.
func (a *App) ReportLocalPaletteCommand(commandID, invocationID, outcome string, elapsedMS int, stage, source string) error {
	if a == nil {
		return commandexecution.ErrDenied
	}
	p := a.commandProduct.Load()
	principal, err := a.currentCommandPrincipal()
	if err != nil {
		return commandexecution.ErrDenied
	}
	a.authMu.RLock()
	sessions, credentials := a.sessionSvc, a.credMgr
	a.authMu.RUnlock()
	if sessions == nil || credentials == nil || (p != nil && (p.registry == nil || p.principal != principal || p.sessionSvc != sessions || p.credMgr != credentials)) {
		return commandexecution.ErrDenied
	}
	current, err := sessions.RevalidateLocalSession(a.commandBridgeContext(), principal)
	if err != nil || current != principal || a.commandProduct.Load() != p || !a.commandPrincipalMatches(sessions, credentials, principal) {
		return commandexecution.ErrDenied
	}
	if stage != "local_ui" && stage != "creation_selection" && stage != "creation_catalog" && stage != "context_target" && stage != "context_navigation" {
		return commandexecution.ErrDenied
	}
	if (stage == "local_ui" && source != "palette") || ((stage == "creation_selection" || stage == "creation_catalog") && source != "keyboard.local" && source != "ui.action") || ((stage == "context_target" || stage == "context_navigation") && source != "ui.action" && source != "keyboard.local" && source != "palette" && source != "streamdeck.key" && source != "unknown") {
		return commandexecution.ErrDenied
	}
	if stage == "creation_catalog" && commandID == "" {
		commandID = "unknown"
	} else if diagnosticCommandForProduct(p, commandID) == "unknown" || len(commandID) > 128 || !diagnosticCommandID.MatchString(commandID) {
		return commandexecution.ErrDenied
	}
	if _, err := uuid.Parse(invocationID); err != nil || elapsedMS < 0 || elapsedMS > 60000 {
		return commandexecution.ErrDenied
	}
	status := commandledger.Status(outcome)
	if status != commandledger.Succeeded && status != commandledger.Cancelled && status != commandledger.Failed && (stage != "local_ui" || status != localActionAccepted) && (stage != "context_navigation" || status != localActionNotCompleted) {
		return commandexecution.ErrDenied
	}
	logCommandExecution(commandID, invocationID, source, stage, time.Now().Add(-time.Duration(elapsedMS)*time.Millisecond), status, nil, nil)
	return nil
}
