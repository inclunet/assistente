package app

import (
	"context"
	"encoding/json"
	"strings"
	"sync"

	"assistente/internal/chat"
	"assistente/internal/commandexecution"
	"assistente/internal/commandui"
	"assistente/internal/database"
	"assistente/internal/workspace"
	"github.com/google/uuid"
)

type ChatEditorCommandPlan struct {
	TabID    string `json:"tabId"`
	DraftID  string `json:"draftId,omitempty"`
	FilePath string `json:"filePath,omitempty"`
}

type ChatEditorCommandTarget struct {
	Workspace *workspace.Workspace `json:"workspace"`
	TabID     string               `json:"tabId"`
	DraftID   string               `json:"draftId,omitempty"`
	FilePath  string               `json:"filePath,omitempty"`
}

type chatEditorPreparation struct {
	mu          sync.Mutex
	plan        workspace.EditorTransferPlan
	keyboard    bool
	stream      *chat.StreamingManager
	attempted   bool
	result      *ChatEditorCommandTarget
	destination workspace.CommandSnapshot
}

func (a *App) prepareChatEditorTarget(ctx context.Context, p *commandProductRuntime, run *commandUIRun, target string) (*chatEditorPreparation, error) {
	a.authMu.RLock()
	stream := a.streamMgr
	a.authMu.RUnlock()
	if stream != nil {
		if _, active := stream.CurrentGeneration(run.snapshot.Tab.ConversationID); active {
			return nil, commandexecution.ErrDenied
		}
	}
	id, err := uuid.NewV7()
	if err != nil {
		return nil, err
	}
	plan, err := p.workspaceMgr.PrepareEditorTransferForCommand(ctx, run.snapshot, target, id.String())
	if err != nil {
		return nil, err
	}
	p.keyboardMu.Lock()
	_, keyboard := p.keyboardEvents[run.reservation.InvocationID]
	p.keyboardMu.Unlock()
	return &chatEditorPreparation{plan: plan, keyboard: keyboard, stream: stream}, nil
}

func (a *App) PrepareChatEditorCommand(ticket, messageID, originalContent, targetDocumentID string) (*ChatEditorCommandPlan, error) {
	if err := a.prepareChatMessageCommand(ticket, messageID, nil, originalContent, &targetDocumentID); err != nil {
		return nil, err
	}
	p, run, err := a.commandUIRun(ticket)
	if err != nil {
		return nil, err
	}
	p.mu.Lock()
	editor := run.messagePreparation.editor
	p.mu.Unlock()
	if editor == nil {
		return nil, commandexecution.ErrDenied
	}
	return &ChatEditorCommandPlan{TabID: editor.plan.TabID, DraftID: editor.plan.DraftID, FilePath: editor.plan.FilePath}, nil
}

func chatEditorForRun(p *commandProductRuntime, run *commandUIRun) *chatEditorPreparation {
	p.mu.Lock()
	defer p.mu.Unlock()
	if run.messagePreparation == nil {
		return nil
	}
	return run.messagePreparation.editor
}

// Open claims a single workspace transition, not the command outcome. Failed
// or uncertain attempts cannot create a second tab on transport retries.
func (a *App) OpenChatEditorCommand(ticket, handoffID, title string) (*ChatEditorCommandTarget, error) {
	p, run, err := a.commandUIRun(ticket)
	if err != nil {
		return nil, err
	}
	if run.reservation.CommandID != commandMessageSendEditorID {
		return nil, commandexecution.ErrDenied
	}
	editor := chatEditorForRun(p, run)
	if editor == nil {
		return nil, commandexecution.ErrDenied
	}
	editor.mu.Lock()
	result, emit, err := a.openChatEditorLocked(p, run, editor, ticket, handoffID, title)
	editor.mu.Unlock()
	if err == nil && emit && a.emitter != nil {
		a.emitter.Emit("workspace:tab_activated", result.Workspace)
	}
	return result, err
}

func (a *App) openChatEditorLocked(p *commandProductRuntime, run *commandUIRun, editor *chatEditorPreparation, ticket, handoffID, title string) (*ChatEditorCommandTarget, bool, error) {
	if err := p.ui.ValidateHandoff(p.uiOwner(), ticket, handoffID); err != nil {
		return nil, false, err
	}
	if editor.attempted {
		if editor.result == nil {
			return nil, false, commandui.ErrOutcomeUnknown
		}
		if err := a.validateChatEditorDestination(p, run, editor, ticket, handoffID); err != nil {
			return nil, false, err
		}
		return cloneChatEditorTarget(editor.result), false, nil
	}
	if len(title) > 512 || strings.ContainsRune(title, '\x00') {
		return nil, false, commandexecution.ErrDenied
	}
	if err := a.validatePreparedChatMessage(run.messagePreparation.ctx, p, run); err != nil {
		return nil, false, err
	}
	p.mu.Lock()
	admission := run.admission
	p.mu.Unlock()
	if admission == nil {
		return nil, false, commandexecution.ErrDenied
	}
	var effectCtx context.Context
	release, err := p.epochs.AdmitExecution(run.messagePreparation.ctx, admission.epoch, func(ctx context.Context) error {
		versions, err := p.host.Snapshot(ctx, p.principal)
		if err != nil || versions != admission.versions || (run.sourceValid != nil && !run.sourceValid()) {
			return commandexecution.ErrStale
		}
		return nil
	}, func(ctx context.Context) error { effectCtx = ctx; return nil })
	if err != nil {
		return nil, false, err
	}
	defer release()
	a.authMu.RLock()
	defer a.authMu.RUnlock()
	if a.commandProduct.Load() != p || a.workspaceMgr != p.workspaceMgr || a.currentAuthUser == nil || a.currentAuthUser.UserID != p.principal.UserID || a.currentAuthUser.SessionID != p.principal.SessionID {
		return nil, false, commandexecution.ErrDenied
	}
	editor.attempted = true
	ws, destination, err := p.workspaceMgr.OpenEditorTransferForCommand(effectCtx, run.snapshot, editor.plan, title)
	if err != nil {
		return nil, false, err
	}
	editor.destination = destination
	editor.result = &ChatEditorCommandTarget{Workspace: ws, TabID: editor.plan.TabID, DraftID: editor.plan.DraftID, FilePath: editor.plan.FilePath}
	return cloneChatEditorTarget(editor.result), true, nil
}

func cloneChatEditorTarget(in *ChatEditorCommandTarget) *ChatEditorCommandTarget {
	if in == nil {
		return nil
	}
	raw, err := json.Marshal(in)
	if err != nil {
		return nil
	}
	var out ChatEditorCommandTarget
	if json.Unmarshal(raw, &out) != nil {
		return nil
	}
	return &out
}

// Validate is a readonly barrier after async editor mounting and before the UI
// revalidates its own instance/version synchronously and applies the content.
func (a *App) ValidateChatEditorCommand(ticket, handoffID string) error {
	p, run, err := a.commandUIRun(ticket)
	if err != nil {
		return err
	}
	if run.reservation.CommandID != commandMessageSendEditorID {
		return commandexecution.ErrDenied
	}
	editor := chatEditorForRun(p, run)
	if editor == nil {
		return commandexecution.ErrDenied
	}
	editor.mu.Lock()
	defer editor.mu.Unlock()
	return a.validateChatEditorDestination(p, run, editor, ticket, handoffID)
}

func (a *App) validateChatEditorDestination(p *commandProductRuntime, run *commandUIRun, editor *chatEditorPreparation, ticket, handoffID string) error {
	return a.withChatEditorDestination(p, run, editor, ticket, handoffID, nil)
}

func (a *App) withChatEditorDestination(p *commandProductRuntime, run *commandUIRun, editor *chatEditorPreparation, ticket, handoffID string, complete func() error) error {
	if editor.result == nil {
		return commandexecution.ErrDenied
	}
	if err := a.validateChatEditorStream(run, editor); err != nil {
		return err
	}
	if err := p.ui.ValidateHandoff(p.uiOwner(), ticket, handoffID); err != nil {
		return err
	}
	p.mu.Lock()
	admission, prep := run.admission, run.messagePreparation
	p.mu.Unlock()
	if admission == nil || prep.ctx.Err() != nil {
		return commandexecution.ErrStale
	}
	if !editor.keyboard && run.sourceValid != nil && !run.sourceValid() {
		return commandexecution.ErrStale
	}
	fingerprint, err := database.MessageCommandSnapshotWithContext(database.WithUserID(prep.ctx, p.principal.UserID), run.snapshot.Tab.ConversationID, prep.messageID, false)
	if err != nil || fingerprint != prep.fingerprint {
		return commandexecution.ErrStale
	}
	return p.epochs.Admit(prep.ctx, admission.epoch, func(ctx context.Context) error {
		if !editor.keyboard && run.sourceValid != nil && !run.sourceValid() {
			return commandexecution.ErrStale
		}
		versions, err := p.host.Snapshot(ctx, p.principal)
		if err != nil || versions != admission.versions || a.commandProduct.Load() != p || !p.dependenciesMatch(a) {
			return commandexecution.ErrStale
		}
		return nil
	}, func() error {
		a.authMu.RLock()
		defer a.authMu.RUnlock()
		if a.workspaceMgr != p.workspaceMgr || a.conversationsCtrl != prep.controller || a.currentAuthUser == nil || a.currentAuthUser.UserID != p.principal.UserID || a.currentAuthUser.SessionID != p.principal.SessionID {
			return commandexecution.ErrDenied
		}
		return p.workspaceMgr.WithCommandSnapshot(prep.ctx, func(current workspace.CommandSnapshot) error {
			if current != editor.destination {
				return commandexecution.ErrStale
			}
			if complete != nil {
				return complete()
			}
			return nil
		})
	})
}

func (a *App) validateChatEditorStream(run *commandUIRun, editor *chatEditorPreparation) error {
	a.authMu.RLock()
	stream := a.streamMgr
	a.authMu.RUnlock()
	if stream != editor.stream {
		return commandexecution.ErrStale
	}
	if stream != nil {
		if _, active := stream.CurrentGeneration(run.snapshot.Tab.ConversationID); active {
			return commandexecution.ErrStale
		}
	}
	return nil
}

func (a *App) completeChatEditorCommand(p *commandProductRuntime, run *commandUIRun, ticket, handoffID, status string) error {
	editor := chatEditorForRun(p, run)
	if editor == nil {
		return commandexecution.ErrDenied
	}
	editor.mu.Lock()
	defer editor.mu.Unlock()
	if err := p.ui.ValidateHandoff(p.uiOwner(), ticket, handoffID); err != nil {
		return err
	}
	if status != "succeeded" {
		if status != "failed" && status != "cancelled" {
			return commandui.ErrInvalidRequest
		}
		return p.ui.Cancel(p.uiOwner(), ticket)
	}
	if err := a.withChatEditorDestination(p, run, editor, ticket, handoffID, func() error { return p.ui.Complete(p.uiOwner(), ticket, handoffID, status) }); err != nil {
		if editor.result != nil {
			_ = p.ui.Cancel(p.uiOwner(), ticket)
		}
		return err
	}
	return nil
}
