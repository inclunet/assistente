package app

import (
	"context"
	"errors"
	"fmt"
	"os"

	"assistente/internal/commandexecution"
	"assistente/internal/terminal"
	"assistente/internal/workspace"
)

// terminalSessionCreator é a fronteira do lifecycle real. O teste injeta uma
// implementação fake; o caminho produtivo recebe o mesmo terminal.Manager
// usado pelo controller, sem criar um pool paralelo.
type terminalSessionCreator interface {
	CreateInfo(name, workDir string) (terminal.SessionInfo, error)
	Close(sessionID string) error
	Has(sessionID string) bool
}

func (a *App) currentTerminalManager() *terminal.Manager {
	if a == nil {
		return nil
	}
	a.authMu.RLock()
	defer a.authMu.RUnlock()
	return a.terminalMgr
}

// createTerminalSessionAndCommit mantém a sessão recém-criada como recurso
// pendente até o callback concluir a escrita da aba. O erro de cleanup não é
// descartado: ele é unido ao erro primário para não esconder um PTY órfão.
func createTerminalSessionAndCommit(
	ctx context.Context,
	tabID string,
	sessions terminalSessionCreator,
	commit func(context.Context, terminal.SessionInfo, string) (*workspace.Workspace, error),
) (result *workspace.Workspace, err error) {
	if ctx == nil || tabID == "" || sessions == nil || commit == nil {
		return nil, fmt.Errorf("terminal workspace command: invalid dependencies")
	}
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	workDir, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("terminal workspace command: get cwd: %w", err)
	}
	info, err := sessions.CreateInfo("", workDir)
	if err != nil {
		return nil, err
	}
	committed := false
	defer func() {
		if committed || info.ID == "" {
			return
		}
		if cleanupErr := sessions.Close(info.ID); cleanupErr != nil {
			err = errors.Join(err, fmt.Errorf("terminal workspace command: cleanup session %s: %w", info.ID, cleanupErr))
			result = nil
		}
	}()
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	if info.ID == "" || !sessions.Has(info.ID) {
		return nil, fmt.Errorf("terminal workspace command: created session is not live")
	}
	result, err = commit(ctx, info, workDir)
	if err != nil {
		return nil, err
	}
	committed = true
	return result, nil
}

func (a *App) commitTerminalWorkspaceTab(
	ctx context.Context,
	p *commandProductRuntime,
	run *commandUIRun,
	expected workspace.CommandSnapshot,
	tabID string,
) (*workspace.Workspace, error) {
	if p == nil || run == nil || run.terminalMgr == nil || a.currentTerminalManager() != run.terminalMgr {
		return nil, commandexecution.ErrStale
	}
	// Recusar um alvo já obsoleto antes de iniciar qualquer processo. O CAS
	// abaixo continua necessário: a criação da sessão pode levar tempo.
	current, err := p.workspaceMgr.CommandSnapshot()
	if err != nil || current.WorkspaceID != expected.WorkspaceID || current.Version != expected.Version || current.ActiveTabID != expected.ActiveTabID {
		return nil, commandexecution.ErrStale
	}
	return createTerminalSessionAndCommit(ctx, tabID, run.terminalMgr, func(commitCtx context.Context, info terminal.SessionInfo, _ string) (*workspace.Workspace, error) {
		a.authMu.RLock()
		defer a.authMu.RUnlock()
		if a.currentAuthUser == nil || a.currentAuthUser.UserID != p.principal.UserID || a.currentAuthUser.SessionID != p.principal.SessionID || a.workspaceMgr != p.workspaceMgr || a.terminalMgr != run.terminalMgr || a.commandProduct.Load() != p {
			return nil, commandexecution.ErrStale
		}
		current, err := p.workspaceMgr.CommandSnapshot()
		if err != nil || current.WorkspaceID != expected.WorkspaceID || current.Version != expected.Version || current.ActiveTabID != expected.ActiveTabID {
			return nil, commandexecution.ErrStale
		}
		return p.workspaceMgr.AddTerminalTabForCommand(commitCtx, expected, workspace.Tab{
			ID:    tabID,
			Type:  workspace.TabTypeTerminal,
			Title: "Terminal",
			State: map[string]any{"sessionId": info.ID},
		}, run.terminalMgr)
	})
}
