package app

import (
	"context"
	"errors"
	"strings"
	"time"

	"assistente/internal/apidto"
	"assistente/internal/commandcatalog"
	"assistente/internal/commandexecution"
	"assistente/internal/commandui"
	"assistente/internal/database"
	"assistente/internal/tools/filesystem"
	"assistente/internal/workspace"
	"github.com/google/uuid"
)

const (
	commandEditorFileOpenID     = "editor.file.open"
	commandEditorFileSaveID     = "editor.file.save"
	commandEditorFileSaveCopyID = "editor.file.save_copy"
)

type editorFilePreparation struct {
	token             string
	sourcePath        string
	requiresOverwrite bool
	keyboardHandoff   bool
	paletteHandoff    bool
	paletteValidUntil int64
	ticket            string
	operation         string
	path              string
	content           string
	expected          filesystem.FileVersion
	opened            *apidto.EditorOpenResult
	snapshot          workspace.CommandSnapshot
	product           *commandProductRuntime
}

type workspaceEditorFileChangedEvent struct {
	Workspace *workspace.Workspace `json:"workspace"`
	TabID     string               `json:"tabId"`
}

func editorCommandContextMatches(ctx context.Context, p *commandProductRuntime) bool {
	if ctx == nil || p == nil {
		return false
	}
	userID, ok := database.UserIDFromContext(ctx)
	return ok && userID == p.principal.UserID
}

func isEditorFileCommand(commandID string) bool {
	switch commandID {
	case commandEditorFileOpenID, commandEditorFileSaveID, commandEditorFileSaveCopyID:
		return true
	default:
		return false
	}
}

func editorFileOperation(commandID string) string {
	switch commandID {
	case commandEditorFileOpenID:
		return "open"
	case commandEditorFileSaveID:
		return "save"
	case commandEditorFileSaveCopyID:
		return "save_copy"
	default:
		return ""
	}
}

func editorFileCommandRegistration(commandID string) (commandcatalog.Definition, commandcatalog.HandlerContract) {
	operation := editorFileOperation(commandID)
	if operation == "" {
		return commandcatalog.Definition{}, commandcatalog.HandlerContract{}
	}
	name := map[string][3]string{
		"open":      {"Abrir arquivo", "Open file", "Abrir archivo"},
		"save":      {"Salvar arquivo", "Save file", "Guardar archivo"},
		"save_copy": {"Salvar cópia", "Save a copy", "Guardar una copia"},
	}[operation]
	contract := commandcatalog.HandlerContract{Effect: commandcatalog.Write, HasMutableTarget: true, Route: "contextual/editor/file/" + operation, Classification: commandcatalog.HandlerBackend}
	return commandcatalog.Definition{
		ID: commandID, Effect: commandcatalog.Write, Decision: commandcatalog.NoDecision, HasMutableTarget: true,
		AllowedSources: []commandcatalog.Source{commandcatalog.Palette, commandcatalog.KeyboardLocal, commandcatalog.StreamDeck},
		Context:        commandcatalog.ContextPolicy{Facts: []commandcatalog.ContextFact{{Provider: "workspace", Fact: "active_tab", Mode: commandcatalog.ExactVersion}}},
		Presentation: &commandcatalog.Presentation{Version: "editor-file-" + operation + "-v1", Locales: map[string]commandcatalog.LocalizedMetadata{
			"pt-BR": {Name: name[0], Description: "Operação contextual segura de arquivo do editor", Category: "Editor"},
			"en":    {Name: name[1], Description: "Safe contextual editor file operation", Category: "Editor"},
			"es":    {Name: name[2], Description: "Operación contextual segura de archivo del editor", Category: "Editor"},
		}},
		ArgumentsSchema: &commandcatalog.Schema{Type: commandcatalog.SchemaObject}, ResultSchema: &commandcatalog.Schema{Type: commandcatalog.SchemaObject},
		Risk: commandcatalog.RiskMedium, Persistence: commandcatalog.PersistencePolicy{Arguments: commandcatalog.PersistenceNever, Result: commandcatalog.PersistenceSummary, Audit: commandcatalog.PersistenceRedacted},
		Scopes: []commandcatalog.Scope{commandcatalog.ScopeWorkspace}, Availability: commandcatalog.Availability{Status: commandcatalog.Available}, HandlerRoute: contract.Route, HandlerClassification: contract.Classification,
	}, contract
}

func (a *App) editorCommandTarget(ctx context.Context, ticket, handoffID string) (string, string, error) {
	p, run, err := a.commandUIRun(ticket)
	if err != nil {
		return "", "", err
	}
	if !isEditorFileCommand(run.reservation.CommandID) || p.ui == nil || !editorCommandContextMatches(ctx, p) {
		return "", "", commandexecution.ErrDenied
	}
	if err := p.ui.ValidateHandoff(p.uiOwner(), ticket, handoffID); err != nil {
		return "", "", err
	}
	if run.sourceValid != nil && !run.sourceValid() {
		return "", "", commandexecution.ErrStale
	}
	p.keyboardMu.Lock()
	_, keyboardHandoff := p.keyboardEvents[run.reservation.InvocationID]
	paletteHandoff := run.paletteContext != nil
	var paletteValidUntil int64
	if paletteHandoff && p.keyboardMap != nil {
		paletteValidUntil = p.keyboardMap.view.ValidUntil
	}
	p.keyboardMu.Unlock()
	// A continuação só nasce após a fonte original ainda ser válida. Depois
	// do diálogo, o mapa DOM pode ter sido descartado por blur legítimo.
	if paletteHandoff && (run.sourceValid == nil || !run.sourceValid()) {
		return "", "", commandexecution.ErrStale
	}
	operation := editorFileOperation(run.reservation.CommandID)
	if run.snapshot.Tab.Type != workspace.TabTypeEditor {
		return "", "", commandexecution.ErrStale
	}
	active := p.workspaceMgr.Active()
	if active == nil || active.ID != run.snapshot.WorkspaceID || active.Tabs.Active != run.snapshot.ActiveTabID {
		return "", "", commandexecution.ErrStale
	}
	tab := active.FindTab(run.snapshot.ActiveTabID)
	if tab == nil || tab.Type != workspace.TabTypeEditor {
		return "", "", commandexecution.ErrStale
	}
	path, _ := tab.State["filePath"].(string)
	current, err := p.workspaceMgr.CommandSnapshot()
	if err != nil || current != run.snapshot {
		return "", "", commandexecution.ErrStale
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if run.editorFile != nil || run.editorFilePreparing {
		return "", "", commandui.ErrAlreadyTaken
	}
	run.editorFilePreparing = true
	run.editorFile = &editorFilePreparation{ticket: ticket, operation: operation, sourcePath: strings.TrimSpace(path), snapshot: run.snapshot, product: p, keyboardHandoff: keyboardHandoff, paletteHandoff: paletteHandoff, paletteValidUntil: paletteValidUntil}
	if operation != "save" {
		return operation, "", nil
	}
	return operation, strings.TrimSpace(path), nil
}

func (a *App) editorCommandPrepare(ctx context.Context, request apidto.EditorCommandPrepareRequest, operation, path string, version filesystem.FileVersion, opened *apidto.EditorOpenResult) (string, error) {
	p, run, err := a.commandUIRun(request.Ticket)
	if err != nil {
		return "", err
	}
	if !isEditorFileCommand(run.reservation.CommandID) || editorFileOperation(run.reservation.CommandID) != operation || p.ui == nil || !editorCommandContextMatches(ctx, p) {
		return "", commandexecution.ErrDenied
	}
	if err := p.ui.ValidateHandoff(p.uiOwner(), request.Ticket, request.HandoffID); err != nil {
		return "", err
	}
	p.mu.Lock()
	prepared := run.editorFile
	preparing := run.editorFilePreparing && prepared != nil && prepared.token == "" && prepared.operation == operation
	p.mu.Unlock()
	if !preparing {
		return "", commandui.ErrAlreadyTaken
	}
	current, err := p.workspaceMgr.CommandSnapshot()
	if err != nil || current != run.snapshot {
		return "", commandexecution.ErrStale
	}
	if operation == "open" && opened == nil {
		return "", commandexecution.ErrInvalidRequest
	}
	if operation == "open" && strings.TrimSpace(request.Content) != "" {
		return "", commandexecution.ErrInvalidRequest
	}
	if operation != "open" && strings.TrimSpace(path) == "" {
		return "", commandexecution.ErrInvalidRequest
	}
	tokenID, err := uuid.NewV7()
	if err != nil {
		return "", err
	}
	token := tokenID.String()
	p.mu.Lock()
	defer p.mu.Unlock()
	if run.editorFile != prepared || !run.editorFilePreparing || prepared.token != "" {
		return "", commandui.ErrAlreadyTaken
	}
	prepared.token, prepared.path, prepared.content = token, path, request.Content
	prepared.expected, prepared.opened = version, cloneEditorOpenResult(opened)
	prepared.requiresOverwrite = version.Exists() && (operation == "save_copy" || operation == "save" && prepared.sourcePath == "")
	run.editorFilePreparing = false
	return token, nil
}

func cloneEditorOpenResult(in *apidto.EditorOpenResult) *apidto.EditorOpenResult {
	if in == nil {
		return nil
	}
	out := *in
	out.Warnings = append([]string(nil), in.Warnings...)
	return &out
}

func (a *App) editorCommandCommit(ctx context.Context, request apidto.EditorCommandCommitRequest, write func(context.Context, string, string, filesystem.FileVersion) error) (*apidto.EditorCommandResult, error) {
	p, run, err := a.commandUIRun(request.Ticket)
	if err != nil {
		return nil, err
	}
	if !isEditorFileCommand(run.reservation.CommandID) || p.ui == nil || write == nil || !editorCommandContextMatches(ctx, p) {
		return nil, commandexecution.ErrDenied
	}
	p.mu.Lock()
	prepared := run.editorFile
	preparing := run.editorFilePreparing
	admission := run.admission
	p.mu.Unlock()
	if preparing || prepared == nil || prepared.product != p || prepared.ticket != request.Ticket || prepared.token == "" || prepared.token != request.Token || admission == nil {
		return nil, commandexecution.ErrDenied
	}
	ctx, cancel := context.WithTimeout(ctx, 35*time.Second)
	defer cancel()
	result := &apidto.EditorCommandResult{Status: "failed", Path: prepared.path}
	claimed := false
	commitErr := p.ui.Commit(ctx, p.uiOwner(), request.Ticket, request.HandoffID, func(commitCtx context.Context) error {
		claimed = true
		current, err := a.authenticatedCommandProduct()
		if err != nil || current != p {
			result.Status = "stale"
			return commandexecution.ErrStale
		}
		var executionCtx context.Context
		release, err := p.epochs.AdmitExecution(commitCtx, admission.epoch, func(gateCtx context.Context) error {
			// O Take já consumiu a ocorrência local. O diálogo nativo derruba o
			// mapa DOM por blur: isso não revoga a sessão/configuração admitida.
			// As gerações hostside continuam obrigatórias logo abaixo; a fonte
			// física da Deck não ganha essa exceção de apresentação. A paleta
			// contextual consome a mesma continuação local em editorCommandTarget;
			// mantém deadline, snapshot canônico e gerações mesmo sem mapa DOM.
			if !prepared.keyboardHandoff && !prepared.paletteHandoff && run.sourceValid != nil && !run.sourceValid() {
				return commandexecution.ErrStale
			}
			if prepared.paletteHandoff && prepared.paletteValidUntil != 0 && time.Now().UnixMilli() >= prepared.paletteValidUntil {
				return commandexecution.ErrStale
			}
			versions, err := p.host.Snapshot(gateCtx, p.principal)
			if err != nil || versions != admission.versions || !versions.Unlocked || !p.dependenciesMatch(a) || a.commandProduct.Load() != p {
				return commandexecution.ErrStale
			}
			return nil
		}, func(runCtx context.Context) error { executionCtx = runCtx; return nil })
		if err != nil {
			return err
		}
		defer release()
		a.authMu.RLock()
		defer a.authMu.RUnlock()
		if a.workspaceMgr != p.workspaceMgr || a.currentAuthUser == nil || a.currentAuthUser.UserID != p.principal.UserID || a.currentAuthUser.SessionID != p.principal.SessionID {
			return commandexecution.ErrDenied
		}
		if err := prepared.expected.Validate(prepared.path); err != nil {
			result.Status = "conflict"
			return err
		}
		if prepared.requiresOverwrite && !request.ConfirmOverwrite {
			result.Status = "cancelled"
			return commandui.ErrCancelled
		}
		var updated *workspace.Workspace
		var tabID string
		var written bool
		updated, tabID, written, err = p.workspaceMgr.CommitEditorFileForCommand(executionCtx, prepared.snapshot, prepared.operation, prepared.path, uuid.New().String(), func(writeCtx context.Context) error {
			if prepared.operation == "open" {
				return nil
			}
			return write(writeCtx, prepared.path, prepared.content, prepared.expected)
		})
		result.Written = written
		if err != nil {
			result.Status = "failed"
			if written {
				return errors.Join(commandui.ErrOutcomeUnknown, err)
			}
			return err
		}
		result.Status, result.TabID, result.Written = "succeeded", tabID, written
		if prepared.opened != nil {
			result.Opened = cloneEditorOpenResult(prepared.opened)
		}
		if updated != nil && a.emitter != nil {
			a.emitter.Emit("workspace:editor_file_changed", workspaceEditorFileChangedEvent{Workspace: updated, TabID: tabID})
		}
		return nil
	})
	p.mu.Lock()
	if claimed && run.editorFile == prepared {
		run.editorFile = nil
	}
	p.mu.Unlock()
	if commitErr != nil {
		return result, commitErr
	}
	return result, nil
}
