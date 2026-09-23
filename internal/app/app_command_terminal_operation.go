package app

import (
	"context"
	"errors"

	"assistente/internal/commandcatalog"
	"assistente/internal/commandexecution"
	"assistente/internal/commandui"
	"assistente/internal/terminal"
	"assistente/internal/workspace"
)

const commandTerminalInterruptID = "terminal.command.interrupt"

// PrepareTerminalInterruptCommand compares the visible target with the runtime
// target already captured by Begin. These IDs are assertions, never selectors
// for a new target; they are not persisted as command arguments.
func (a *App) PrepareTerminalInterruptCommand(ticket, workspaceID, tabID, sessionID, commandID string) error {
	p, run, err := a.commandUIRun(ticket)
	if err != nil {
		return err
	}
	if run.reservation.CommandID != commandTerminalInterruptID {
		return commandexecution.ErrDenied
	}
	p.mu.Lock()
	if run.terminalPreparationClaimed {
		p.mu.Unlock()
		return commandexecution.ErrDenied
	}
	run.terminalPreparationClaimed = true
	valid := workspaceID != "" && tabID != "" && sessionID != "" &&
		run.snapshot.WorkspaceID == workspaceID && run.snapshot.ActiveTabID == tabID &&
		run.terminalInterrupt.MatchesTarget(sessionID, commandID)
	run.terminalPrepared = valid
	p.mu.Unlock()
	if !valid {
		run.cancel()
		_ = p.ui.Cancel(p.uiOwner(), ticket)
		return commandexecution.ErrStale
	}
	return nil
}

func commandTerminalInterruptRegistration() (commandcatalog.Definition, commandcatalog.HandlerContract) {
	d, h := commandWorkspaceChatOpenRegistration()
	d.ID = commandTerminalInterruptID
	d.HandlerRoute = "contextual/terminal/command/interrupt"
	h.Route = d.HandlerRoute
	d.Persistence.Result = commandcatalog.PersistenceNever
	d.Presentation = &commandcatalog.Presentation{Version: "terminal-interrupt-v1", Locales: map[string]commandcatalog.LocalizedMetadata{
		"pt-BR": {Name: "Interromper comando do terminal", Description: "Envia interrupção somente à execução capturada no terminal ativo", Category: "Terminal"},
		"en":    {Name: "Interrupt terminal command", Description: "Sends an interrupt only to the captured execution in the active terminal", Category: "Terminal"},
		"es":    {Name: "Interrumpir comando del terminal", Description: "Envía una interrupción solo a la ejecución capturada en el terminal activo", Category: "Terminal"},
	}}
	return d, h
}

func (a *App) captureTerminalInterrupt(ctx context.Context, p *commandProductRuntime) (workspace.CommandSnapshot, *terminal.Manager, terminal.InterruptSnapshot, error) {
	var expected workspace.CommandSnapshot
	var target terminal.InterruptSnapshot
	a.authMu.RLock()
	defer a.authMu.RUnlock()
	mgr := a.terminalMgr
	if p == nil || mgr == nil || a.workspaceMgr != p.workspaceMgr || a.commandProduct.Load() != p {
		return expected, nil, target, commandexecution.ErrDenied
	}
	err := p.workspaceMgr.WithTerminalCommandSnapshot(ctx, func(snapshot workspace.CommandSnapshot, sessionID string) error {
		if snapshot.WorkspaceID != p.workspaceID {
			return commandexecution.ErrStale
		}
		var err error
		target, err = mgr.CaptureInterrupt(sessionID)
		expected = snapshot
		return err
	})
	if err != nil {
		// Runtime identifiers and raw terminal errors are not public diagnostics.
		return workspace.CommandSnapshot{}, nil, terminal.InterruptSnapshot{}, commandexecution.ErrStale
	}
	return expected, mgr, target, err
}

func (a *App) commitTerminalInterrupt(ctx context.Context, p *commandProductRuntime, run *commandUIRun, expected workspace.CommandSnapshot) error {
	var operation *terminal.InterruptOperation
	err := func() error {
		a.authMu.RLock()
		defer a.authMu.RUnlock()
		if run.terminalMgr == nil || a.terminalMgr != run.terminalMgr || a.workspaceMgr != p.workspaceMgr || a.commandProduct.Load() != p ||
			a.currentAuthUser == nil || a.currentAuthUser.UserID != p.principal.UserID || a.currentAuthUser.SessionID != p.principal.SessionID {
			return commandexecution.ErrStale
		}
		return p.workspaceMgr.WithTerminalCommandSnapshot(ctx, func(current workspace.CommandSnapshot, _ string) error {
			if current != expected || ctx.Err() != nil {
				return commandexecution.ErrStale
			}
			var err error
			operation, err = run.terminalMgr.PrepareInterrupt(run.terminalInterrupt)
			return err
		})
	}()
	if err != nil {
		return commandexecution.ErrStale
	}
	// The session is now pinned by the terminal's short admission. Never wait
	// for a PTY write while holding authentication or workspace locks.
	defer operation.Cancel()
	if err := operation.Execute(ctx); err != nil {
		if errors.Is(err, terminal.ErrInterruptStale) {
			return commandexecution.ErrStale
		}
		// The write may have been partial; no raw payload or automatic retry.
		return commandui.ErrOutcomeUnknown
	}
	return nil
}
