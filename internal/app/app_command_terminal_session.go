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

const (
	commandTerminalSessionCreateID = "terminal.session.create"
	commandTerminalSessionCloseID  = "terminal.session.close"
)

func terminalSessionCommandRegistration(commandID string) (commandcatalog.Definition, commandcatalog.HandlerContract) {
	contract := commandcatalog.HandlerContract{
		Effect:           commandcatalog.Write,
		HasMutableTarget: true,
		Route:            "contextual/terminal/session/" + commandID[len("terminal.session."):],
		Classification:   commandcatalog.HandlerBackend,
	}
	if commandID == commandTerminalSessionCloseID {
		contract.Effect = commandcatalog.Destructive
	}
	name, description := "Criar sessão de terminal", "Cria uma sessão e a vincula à aba terminal ativa"
	enName, enDescription := "Create terminal session", "Creates a session and binds it to the active terminal tab"
	esName, esDescription := "Crear sesión de terminal", "Crea una sesión y la vincula a la pestaña terminal activa"
	effect := commandcatalog.Write
	decision := commandcatalog.NoDecision
	risk := commandcatalog.RiskLow
	version := "terminal-session-create-v1"
	if commandID == commandTerminalSessionCloseID {
		name, description = "Fechar sessão de terminal", "Encerra a sessão capturada e desassocia a aba terminal ativa"
		enName, enDescription = "Close terminal session", "Closes the captured session and unbinds the active terminal tab"
		esName, esDescription = "Cerrar sesión de terminal", "Cierra la sesión capturada y desvincula la pestaña terminal activa"
		effect = commandcatalog.Destructive
		decision = commandcatalog.Interactive
		risk = commandcatalog.RiskHigh
		version = "terminal-session-close-v1"
	}
	return commandcatalog.Definition{
		ID: commandID, Effect: effect, Decision: decision, HasMutableTarget: true,
		AllowedSources: []commandcatalog.Source{commandcatalog.Palette, commandcatalog.KeyboardLocal, commandcatalog.StreamDeck},
		Context:        commandcatalog.ContextPolicy{Facts: []commandcatalog.ContextFact{{Provider: "workspace", Fact: "active_tab", Mode: commandcatalog.ExactVersion}}},
		Presentation: &commandcatalog.Presentation{Version: version, Locales: map[string]commandcatalog.LocalizedMetadata{
			"pt-BR": {Name: name, Description: description, Category: "Terminal"},
			"en":    {Name: enName, Description: enDescription, Category: "Terminal"},
			"es":    {Name: esName, Description: esDescription, Category: "Terminal"},
		}},
		ArgumentsSchema: &commandcatalog.Schema{Type: commandcatalog.SchemaObject},
		ResultSchema:    &commandcatalog.Schema{Type: commandcatalog.SchemaObject},
		Risk:            risk,
		Persistence: commandcatalog.PersistencePolicy{
			Arguments: commandcatalog.PersistenceNever,
			Result:    commandcatalog.PersistenceNever,
			Audit:     commandcatalog.PersistenceRedacted,
		},
		Scopes: []commandcatalog.Scope{commandcatalog.ScopeWorkspace}, Availability: commandcatalog.Availability{Status: commandcatalog.Available},
		HandlerRoute: contract.Route, HandlerClassification: contract.Classification,
	}, contract
}

func commandTerminalSessionRegistrations() []commandcatalog.Registration {
	registrations := make([]commandcatalog.Registration, 0, 2)
	for _, id := range []string{commandTerminalSessionCreateID, commandTerminalSessionCloseID} {
		definition, handler := terminalSessionCommandRegistration(id)
		registrations = append(registrations, commandcatalog.Registration{Definition: definition, Handler: handler})
	}
	return registrations
}

func (a *App) captureTerminalClose(ctx context.Context, p *commandProductRuntime) (workspace.CommandSnapshot, *terminal.Manager, terminal.CloseSnapshot, error) {
	var expected workspace.CommandSnapshot
	var target terminal.CloseSnapshot
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
		target, err = mgr.CaptureClose(sessionID)
		expected = snapshot
		return err
	})
	if err != nil {
		return workspace.CommandSnapshot{}, nil, terminal.CloseSnapshot{}, commandexecution.ErrStale
	}
	return expected, mgr, target, nil
}

func (a *App) captureTerminalSessionCreate(ctx context.Context, p *commandProductRuntime) (workspace.CommandSnapshot, *terminal.Manager, string, error) {
	a.authMu.RLock()
	defer a.authMu.RUnlock()
	mgr := a.terminalMgr
	if p == nil || mgr == nil || a.workspaceMgr != p.workspaceMgr || a.commandProduct.Load() != p {
		return workspace.CommandSnapshot{}, nil, "", commandexecution.ErrDenied
	}
	var expected workspace.CommandSnapshot
	var sessionID string
	err := p.workspaceMgr.WithTerminalCommandTabSnapshot(ctx, func(snapshot workspace.CommandSnapshot, currentSessionID string) error {
		if snapshot.WorkspaceID != p.workspaceID || snapshot.Tab.Type != workspace.TabTypeTerminal {
			return commandexecution.ErrStale
		}
		expected = snapshot
		sessionID = currentSessionID
		return nil
	})
	if err != nil {
		return workspace.CommandSnapshot{}, nil, "", commandexecution.ErrStale
	}
	return expected, mgr, sessionID, nil
}

// PrepareTerminalSessionCommand compares the visible terminal tab/binding
// with the Begin snapshot. It never selects a replacement tab or session.
func (a *App) PrepareTerminalSessionCommand(ticket, workspaceID, tabID, sessionID string) error {
	p, run, err := a.commandUIRun(ticket)
	if err != nil {
		return err
	}
	if run.reservation.CommandID != commandTerminalSessionCreateID && run.reservation.CommandID != commandTerminalSessionCloseID {
		return commandexecution.ErrDenied
	}
	if workspaceID == "" || tabID == "" || workspaceID != run.snapshot.WorkspaceID || tabID != run.snapshot.ActiveTabID {
		run.cancel()
		_ = p.ui.Cancel(p.uiOwner(), ticket)
		return commandexecution.ErrStale
	}
	if sessionID != run.terminalSessionID {
		run.cancel()
		_ = p.ui.Cancel(p.uiOwner(), ticket)
		return commandexecution.ErrStale
	}
	valid := false
	err = p.workspaceMgr.WithTerminalCommandTabSnapshot(a.commandBridgeContext(), func(snapshot workspace.CommandSnapshot, currentSessionID string) error {
		valid = snapshot == run.snapshot && currentSessionID == sessionID
		return nil
	})
	if err != nil || !valid {
		run.cancel()
		_ = p.ui.Cancel(p.uiOwner(), ticket)
		return commandexecution.ErrStale
	}
	p.mu.Lock()
	if run.terminalPreparationClaimed {
		p.mu.Unlock()
		return commandexecution.ErrDenied
	}
	run.terminalPreparationClaimed = true
	run.terminalPrepared = true
	if run.terminalPreparationReady != nil {
		close(run.terminalPreparationReady)
	}
	p.mu.Unlock()
	return nil
}

func (a *App) commitTerminalSessionCreate(ctx context.Context, p *commandProductRuntime, run *commandUIRun, expected workspace.CommandSnapshot) (*workspace.Workspace, error) {
	if p == nil || run == nil || run.terminalMgr == nil || a.currentTerminalManager() != run.terminalMgr {
		return nil, commandexecution.ErrStale
	}
	current, err := p.workspaceMgr.CommandSnapshot()
	if err != nil || current != expected || expected.Tab.Type != workspace.TabTypeTerminal {
		return nil, commandexecution.ErrStale
	}
	return createTerminalSessionAndCommit(ctx, expected.ActiveTabID, run.terminalMgr, func(commitCtx context.Context, info terminal.SessionInfo, _ string) (*workspace.Workspace, error) {
		a.authMu.RLock()
		defer a.authMu.RUnlock()
		if a.currentAuthUser == nil || a.currentAuthUser.UserID != p.principal.UserID || a.currentAuthUser.SessionID != p.principal.SessionID || a.workspaceMgr != p.workspaceMgr || a.terminalMgr != run.terminalMgr || a.commandProduct.Load() != p {
			return nil, commandexecution.ErrStale
		}
		current, err := p.workspaceMgr.CommandSnapshot()
		if err != nil || current != expected {
			return nil, commandexecution.ErrStale
		}
		return p.workspaceMgr.BindTerminalSessionForCommand(commitCtx, expected, info.ID, run.terminalMgr)
	})
}

func (a *App) commitTerminalSessionClose(ctx context.Context, p *commandProductRuntime, run *commandUIRun, expected workspace.CommandSnapshot) error {
	if p == nil || run == nil || run.terminalMgr == nil || a.currentTerminalManager() != run.terminalMgr {
		return commandexecution.ErrStale
	}
	var operation *terminal.CloseOperation
	var sessionID string
	err := func() error {
		a.authMu.RLock()
		defer a.authMu.RUnlock()
		if a.currentAuthUser == nil || a.currentAuthUser.UserID != p.principal.UserID || a.currentAuthUser.SessionID != p.principal.SessionID || a.workspaceMgr != p.workspaceMgr || a.terminalMgr != run.terminalMgr || a.commandProduct.Load() != p {
			return commandexecution.ErrStale
		}
		var current workspace.CommandSnapshot
		err := p.workspaceMgr.WithTerminalCommandSnapshot(ctx, func(snapshot workspace.CommandSnapshot, currentSessionID string) error {
			if snapshot != expected || currentSessionID == "" {
				return commandexecution.ErrStale
			}
			current = snapshot
			sessionID = currentSessionID
			return nil
		})
		if err != nil {
			return err
		}
		if current != expected {
			return commandexecution.ErrStale
		}
		operation, err = run.terminalMgr.PrepareClose(run.terminalClose)
		if err != nil {
			return err
		}
		if _, err := p.workspaceMgr.UnbindTerminalSessionForCommand(ctx, expected, sessionID); err != nil {
			operation.Cancel()
			operation = nil
			return err
		}
		return nil
	}()
	if err != nil {
		if operation != nil {
			operation.Cancel()
		}
		if errors.Is(err, terminal.ErrCloseStale) || errors.Is(err, workspace.ErrCommandTerminalBindingStale) {
			return commandexecution.ErrStale
		}
		return err
	}
	if operation == nil {
		return commandexecution.ErrStale
	}
	// UnbindTerminalSessionForCommand já confirmou a mutação persistida. A
	// partir daqui o ownership do teardown é deste comando, mesmo que a UI ou
	// o contexto de execução seja cancelado exatamente após o CAS do vínculo.
	// Não permitir que Execute veja o cancelamento e abandone uma sessão viva
	// sem vínculo; a operação preparada continua sem depender do caller.
	teardownCtx := context.Background()
	if ctx != nil {
		teardownCtx = context.WithoutCancel(ctx)
	}
	if err := operation.Execute(teardownCtx); err != nil {
		// O vínculo já foi removido e o efeito destrutivo pode ter começado (ou
		// ficado indeterminado). Não declarar stale nem permitir retry automático.
		return commandui.ErrOutcomeUnknown
	}
	return nil
}
