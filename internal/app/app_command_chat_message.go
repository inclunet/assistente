package app

import (
	"context"
	"strings"
	"time"

	"assistente/controllers"
	"assistente/internal/chat"
	"assistente/internal/commandcatalog"
	"assistente/internal/commandexecution"
	"assistente/internal/database"
	"assistente/internal/workspace"
)

const (
	commandMessageCopyID         = "chat.message.copy"
	commandMessageCopyMarkdownID = "chat.message.copy_markdown"
	commandMessageSpeakID        = "chat.message.speak"
	commandMessageEditID         = "chat.message.edit.open"
	commandMessagePinID          = "chat.message.pin.toggle"
	commandMessageDeleteID       = "chat.message.delete"
	commandMessageEditSaveID     = "chat.message.edit.save"
	commandMessageSendEditorID   = "chat.message.send_to_editor"
)

// Only audited actions participate in the preparation/handoff protocol.
var commandChatMessageIDs = []string{commandMessageCopyID, commandMessageCopyMarkdownID, commandMessageSpeakID, commandMessagePinID, commandMessageDeleteID, commandMessageEditSaveID, commandMessageSendEditorID}

func isChatMessageCommand(id string) bool {
	for _, item := range commandChatMessageIDs {
		if item == id {
			return true
		}
	}
	return false
}

func isChatMessageBackendCommand(id string) bool {
	return id == commandMessagePinID || id == commandMessageDeleteID || id == commandMessageEditSaveID
}
func isChatMessageUICommand(id string) bool {
	return isChatMessageCommand(id) && !isChatMessageBackendCommand(id)
}

type chatMessagePreparation struct {
	ready         chan struct{}
	ctx           context.Context
	claimed       bool // guarded by product.mu; even a failed Prepare consumes the claim
	messageID     string
	fingerprint   string
	versions      commandexecution.Versions
	controller    *controllers.ConversationsController
	editedContent *string // runtime only; nil is distinct from an empty edit
	editor        *chatEditorPreparation
}

func commandChatMessageRegistrations() []commandcatalog.Registration {
	labels := [][3]string{
		{"Copiar mensagem", "Copy message", "Copiar mensaje"},
		{"Copiar mensagem como Markdown", "Copy message as Markdown", "Copiar mensaje como Markdown"},
		{"Ler mensagem em voz alta", "Read message aloud", "Leer mensaje en voz alta"},
		{"Alternar fixação da mensagem", "Toggle message pin", "Alternar fijación del mensaje"},
		{"Excluir mensagem e respostas", "Delete message and replies", "Eliminar mensaje y respuestas"},
		{"Salvar edição da mensagem", "Save message edit", "Guardar edición del mensaje"},
		{"Enviar mensagem ao editor", "Send message to editor", "Enviar mensaje al editor"},
	}
	var result []commandcatalog.Registration
	for i, id := range commandChatMessageIDs {
		d, h := commandWorkspaceChatOpenRegistration()
		d.ID, d.HandlerRoute = id, "contextual/"+id
		h.Route = d.HandlerRoute
		if isChatMessageUICommand(id) {
			d.HandlerClassification, h.Classification = commandcatalog.HandlerUI, commandcatalog.HandlerUI
		}
		if id == commandMessageDeleteID {
			d.Effect, h.Effect, d.Decision, d.Risk = commandcatalog.Destructive, commandcatalog.Destructive, commandcatalog.Interactive, commandcatalog.RiskHigh
		}
		d.Persistence.Result = commandcatalog.PersistenceNever
		d.Presentation = &commandcatalog.Presentation{Version: "chat-message-v1", Locales: map[string]commandcatalog.LocalizedMetadata{}}
		for n, locale := range []string{"pt-BR", "en", "es"} {
			d.Presentation.Locales[locale] = commandcatalog.LocalizedMetadata{Name: labels[i][n], Description: labels[i][n], Category: "Chat", Aliases: []string{labels[i][n]}}
		}
		result = append(result, commandcatalog.Registration{Definition: d, Handler: h})
	}
	return result
}

func (a *App) captureChatMessageTarget(ctx context.Context, p *commandProductRuntime) (workspace.CommandSnapshot, error) {
	var expected workspace.CommandSnapshot
	err := a.withConversationClearTarget(ctx, p, func(snapshot workspace.CommandSnapshot) error {
		_, err := database.GetConversationInfoWithContext(database.WithUserID(ctx, p.principal.UserID), snapshot.Tab.ConversationID)
		expected = snapshot
		return err
	})
	return expected, err
}

// PrepareChatMessageCommand pins one owned persisted message before admission
// and the destructive decision. The candidate/source created at Begin is retained.
func (a *App) PrepareChatMessageCommand(ticket, messageID string) error {
	return a.prepareChatMessageCommand(ticket, messageID, nil, "", nil)
}

// PrepareChatMessageEditCommand checks the editor's original text before
// admission. Text is a private runtime payload, never binding/ledger arguments.
func (a *App) PrepareChatMessageEditCommand(ticket, messageID, originalContent, content string) error {
	return a.prepareChatMessageCommand(ticket, messageID, &content, originalContent, nil)
}

func (a *App) prepareChatMessageCommand(ticket, messageID string, content *string, originalContent string, editorTarget *string) error {
	p, run, err := a.commandUIRun(ticket)
	if err != nil {
		return err
	}
	if !isChatMessageCommand(run.reservation.CommandID) || (run.reservation.CommandID == commandMessageEditSaveID) != (content != nil) || (run.reservation.CommandID == commandMessageSendEditorID) != (editorTarget != nil) {
		return commandexecution.ErrDenied
	}
	p.mu.Lock()
	prep := run.messagePreparation
	if prep == nil || prep.claimed {
		p.mu.Unlock()
		return commandexecution.ErrDenied
	}
	prep.claimed = true
	p.mu.Unlock()
	failed := true
	defer func() {
		if failed {
			run.cancel()
			_ = p.ui.Cancel(p.uiOwner(), ticket)
		}
	}()
	if messageID == "" {
		return commandexecution.ErrDenied
	}
	if content != nil && (strings.TrimSpace(*content) == "" || len(*content) > chat.MaxMessageContentSize) {
		return commandexecution.ErrDenied
	}
	if err := a.validateChatMessageOrigin(prep.ctx, p, run); err != nil {
		return err
	}
	var fingerprint string
	var editor *chatEditorPreparation
	if editorTarget != nil {
		fingerprint, err = database.MessageSourceCommandSnapshotWithContext(database.WithUserID(prep.ctx, p.principal.UserID), run.snapshot.Tab.ConversationID, messageID, originalContent)
		if err == nil {
			editor, err = a.prepareChatEditorTarget(prep.ctx, p, run, *editorTarget)
		}
	} else if content != nil {
		fingerprint, err = database.MessageEditCommandSnapshotWithContext(database.WithUserID(prep.ctx, p.principal.UserID), run.snapshot.Tab.ConversationID, messageID, originalContent)
	} else {
		fingerprint, err = database.MessageCommandSnapshotWithContext(database.WithUserID(prep.ctx, p.principal.UserID), run.snapshot.Tab.ConversationID, messageID, run.reservation.CommandID == commandMessageDeleteID)
	}
	if err != nil {
		return commandexecution.ErrDenied
	}
	if err := a.validateChatMessageOrigin(prep.ctx, p, run); err != nil {
		return err
	}
	p.mu.Lock()
	if prep.ctx.Err() != nil || p.closed || p.uiRuns[ticket] != run {
		p.mu.Unlock()
		return commandexecution.ErrStale
	}
	prep.messageID, prep.fingerprint = messageID, fingerprint
	prep.editedContent = content
	prep.editor = editor
	close(prep.ready)
	p.mu.Unlock()
	failed = false
	return nil
}

func (a *App) validateChatMessageOrigin(ctx context.Context, p *commandProductRuntime, run *commandUIRun) error {
	if ctx.Err() != nil || a.commandProduct.Load() != p || !p.dependenciesMatch(a) {
		return commandexecution.ErrStale
	}
	if run.sourceValid != nil && !run.sourceValid() {
		return commandexecution.ErrStale
	}
	p.mu.Lock()
	prep := run.messagePreparation
	expected := run.snapshot
	p.mu.Unlock()
	if prep == nil {
		return commandexecution.ErrDenied
	}
	versions, err := p.host.Snapshot(ctx, p.principal)
	if err != nil || versions != prep.versions {
		return commandexecution.ErrStale
	}
	a.authMu.RLock()
	defer a.authMu.RUnlock()
	if prep.controller != a.conversationsCtrl || a.workspaceMgr != p.workspaceMgr || a.currentAuthUser == nil || a.currentAuthUser.UserID != p.principal.UserID || a.currentAuthUser.SessionID != p.principal.SessionID {
		return commandexecution.ErrDenied
	}
	return p.workspaceMgr.WithCommandSnapshot(ctx, func(current workspace.CommandSnapshot) error {
		if current != expected {
			return commandexecution.ErrStale
		}
		return nil
	})
}

func (a *App) validatePreparedChatMessage(ctx context.Context, p *commandProductRuntime, run *commandUIRun) error {
	if err := a.validateChatMessageOrigin(ctx, p, run); err != nil {
		return err
	}
	p.mu.Lock()
	prep := run.messagePreparation
	messageID, fingerprint := prep.messageID, prep.fingerprint
	editor := prep.editor
	p.mu.Unlock()
	if editor != nil {
		if err := a.validateChatEditorStream(run, editor); err != nil {
			return err
		}
	}
	if messageID == "" || fingerprint == "" {
		return commandexecution.ErrDenied
	}
	current, err := database.MessageCommandSnapshotWithContext(database.WithUserID(ctx, p.principal.UserID), run.snapshot.Tab.ConversationID, messageID, run.reservation.CommandID == commandMessageDeleteID)
	if err != nil || current != fingerprint {
		return commandexecution.ErrStale
	}
	return nil
}

// CommitChatMessageCommand is only for durable pin/delete/edit. UI effects use the
// existing CompleteUICommand; no message text is accepted or returned here.
func (a *App) CommitChatMessageCommand(ticket, handoffID string) error {
	p, run, err := a.commandUIRun(ticket)
	if err != nil {
		return err
	}
	if !isChatMessageBackendCommand(run.reservation.CommandID) {
		return commandexecution.ErrDenied
	}
	ctx, cancel := context.WithTimeout(a.commandBridgeContext(), 35*time.Second)
	defer cancel()
	return p.ui.Commit(ctx, p.uiOwner(), ticket, handoffID, func(commitCtx context.Context) error {
		if err := a.validatePreparedChatMessage(commitCtx, p, run); err != nil {
			return err
		}
		p.mu.Lock()
		admission, expected, prep := run.admission, run.snapshot, run.messagePreparation
		editedContent := prep.editedContent
		p.mu.Unlock()
		if admission == nil {
			return commandexecution.ErrDenied
		}
		var effectCtx context.Context
		release, err := p.epochs.AdmitExecution(commitCtx, admission.epoch, func(ctx context.Context) error {
			versions, err := p.host.Snapshot(ctx, p.principal)
			if err != nil || versions != admission.versions || (run.sourceValid != nil && !run.sourceValid()) {
				return commandexecution.ErrStale
			}
			return nil
		}, func(ctx context.Context) error { effectCtx = ctx; return nil })
		if err != nil {
			return err
		}
		defer release()
		guard := func(commit func() error) error {
			a.authMu.RLock()
			defer a.authMu.RUnlock()
			if a.conversationsCtrl != prep.controller || a.commandProduct.Load() != p || a.workspaceMgr != p.workspaceMgr || a.currentAuthUser == nil || a.currentAuthUser.UserID != p.principal.UserID || a.currentAuthUser.SessionID != p.principal.SessionID {
				return commandexecution.ErrDenied
			}
			return p.workspaceMgr.WithCommandSnapshot(effectCtx, func(current workspace.CommandSnapshot) error {
				if current != expected {
					return commandexecution.ErrStale
				}
				return commit()
			})
		}
		if run.reservation.CommandID == commandMessageEditSaveID {
			if editedContent == nil {
				return commandexecution.ErrDenied
			}
			return prep.controller.UpdateMessageCommandGuarded(database.WithUserID(effectCtx, p.principal.UserID), expected.Tab.ConversationID, prep.messageID, prep.fingerprint, *editedContent, guard)
		}
		return prep.controller.CommitMessageCommandGuarded(database.WithUserID(effectCtx, p.principal.UserID), expected.Tab.ConversationID, prep.messageID, prep.fingerprint, run.reservation.CommandID == commandMessageDeleteID, guard)
	})
}
