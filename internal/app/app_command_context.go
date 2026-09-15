package app

import (
	"context"
	"strings"
	"time"

	"assistente/internal/auth"
	"assistente/internal/commandcontext"
	"assistente/internal/commandforeground"
	"assistente/internal/workspace"
)

// commandWorkspaceProvider liga o Manager real a UMA sessão desktop. Não usa
// IDs recebidos de UI como ownership, nem infere uma surface da aba ativa.
// A fábrica é interna; a publicação do bus/executor pertence ao bootstrap.
type commandWorkspaceProvider struct {
	// manager e principal são capturados juntos sob authMu. Snapshot mantém
	// authMu até terminar a leitura autoritativa, impedindo logout/troca de
	// sessão entre a validação do owner e o CommandSnapshot.
	app       *App
	manager   *workspace.Manager
	principal auth.LocalSessionPrincipal
}

func (a *App) newCommandWorkspaceProvider(principal auth.LocalSessionPrincipal) (commandcontext.ScopedProvider, error) {
	if a == nil {
		return nil, commandcontext.ErrProviderUnavailable
	}
	if err := validateCommandPrincipal(principal); err != nil {
		return nil, err
	}

	a.authMu.RLock()
	defer a.authMu.RUnlock()
	if a.workspaceMgr == nil || a.currentAuthUser == nil ||
		a.currentUserID != principal.UserID || a.currentAuthUser.UserID != principal.UserID || a.currentAuthUser.SessionID != principal.SessionID {
		return nil, commandcontext.ErrOwnerMismatch
	}
	return &commandWorkspaceProvider{app: a, manager: a.workspaceMgr, principal: principal}, nil
}

func (p *commandWorkspaceProvider) Snapshot(ctx context.Context, scope commandcontext.Scope, fact string) (commandcontext.OwnedSnapshot, error) {
	if p == nil || p.app == nil || p.manager == nil || ctx == nil {
		return commandcontext.OwnedSnapshot{}, commandcontext.ErrProviderUnavailable
	}
	if err := ctx.Err(); err != nil {
		return commandcontext.OwnedSnapshot{}, err
	}
	if err := scope.Validate(); err != nil {
		return commandcontext.OwnedSnapshot{}, err
	}
	if scope.UserID != p.principal.UserID || scope.AuthContextID != p.principal.SessionID || scope.WorkspaceID == nil {
		return commandcontext.OwnedSnapshot{}, commandcontext.ErrOwnerMismatch
	}

	p.app.authMu.RLock()
	defer p.app.authMu.RUnlock()
	current := p.app.currentAuthUser
	if p.app.workspaceMgr != p.manager || current == nil || current.UserID != p.principal.UserID || current.SessionID != p.principal.SessionID || p.app.currentUserID != p.principal.UserID {
		return commandcontext.OwnedSnapshot{}, commandcontext.ErrOwnerMismatch
	}

	// Este instante pertence à consulta da fonte, nunca ao payload de ingresso.
	capturedAt := time.Now().UTC()
	snapshot, err := p.manager.CommandSnapshot()
	if err != nil {
		return commandcontext.OwnedSnapshot{}, commandcontext.ErrProviderUnavailable
	}
	if snapshot.WorkspaceID != *scope.WorkspaceID {
		return commandcontext.OwnedSnapshot{}, commandcontext.ErrOwnerMismatch
	}
	version := ""
	switch fact {
	case "workspace", "active_tab":
		version = snapshot.Version
	case "profile":
		if snapshot.WorkspaceProfile == "" && snapshot.Tab.ProfileOverrideSlug == "" {
			return commandcontext.OwnedSnapshot{}, commandcontext.ErrProviderUnavailable
		}
		version = snapshot.Version
	default:
		return commandcontext.OwnedSnapshot{}, commandcontext.ErrProviderUnavailable
	}
	if err := ctx.Err(); err != nil {
		return commandcontext.OwnedSnapshot{}, err
	}
	return commandcontext.NewOwnedSnapshot(scope, commandcontext.Snapshot{Version: version, CapturedAt: capturedAt})
}

// commandForegroundProvider liga a captura nativa à mesma sessão que criou o
// bus. O scopeBind é privado e fecha sobre o principal; não há como o
// chamador trocar o principal do provider depois da construção.
type commandForegroundProvider struct {
	app       *App
	reader    commandforeground.Reader
	scopeBind func(commandcontext.Scope) bool
}

func (a *App) newCommandForegroundProvider(principal auth.LocalSessionPrincipal) (commandcontext.ScopedProvider, error) {
	if a == nil {
		return nil, commandcontext.ErrProviderUnavailable
	}
	if err := validateCommandPrincipal(principal); err != nil {
		return nil, err
	}

	a.authMu.RLock()
	defer a.authMu.RUnlock()
	if a.currentAuthUser == nil || a.currentUserID != principal.UserID ||
		a.currentAuthUser.UserID != principal.UserID || a.currentAuthUser.SessionID != principal.SessionID {
		return nil, commandcontext.ErrOwnerMismatch
	}
	return &commandForegroundProvider{
		app:       a,
		reader:    commandforeground.NewNative(),
		scopeBind: commandForegroundScopeBind(principal),
	}, nil
}

func commandForegroundScopeBind(principal auth.LocalSessionPrincipal) func(commandcontext.Scope) bool {
	return func(scope commandcontext.Scope) bool {
		return scope.UserID == principal.UserID &&
			scope.AuthContextID == principal.SessionID
	}
}

func (p *commandForegroundProvider) Snapshot(ctx context.Context, scope commandcontext.Scope, fact string) (commandcontext.OwnedSnapshot, error) {
	if p == nil || p.app == nil || p.reader == nil || p.scopeBind == nil || ctx == nil {
		return commandcontext.OwnedSnapshot{}, commandcontext.ErrProviderUnavailable
	}
	if fact != "foreground" {
		return commandcontext.OwnedSnapshot{}, commandcontext.ErrProviderUnavailable
	}
	if err := ctx.Err(); err != nil {
		return commandcontext.OwnedSnapshot{}, err
	}
	if err := scope.Validate(); err != nil {
		return commandcontext.OwnedSnapshot{}, err
	}
	if !p.scopeBind(scope) {
		return commandcontext.OwnedSnapshot{}, commandcontext.ErrOwnerMismatch
	}

	// O guard permanece durante a captura: logout/troca de sessão não pode
	// ocorrer entre a validação do principal e a observação nativa.
	p.app.authMu.RLock()
	defer p.app.authMu.RUnlock()
	current := p.app.currentAuthUser
	if current == nil || p.app.currentUserID != scope.UserID ||
		current.UserID != scope.UserID || current.SessionID != scope.AuthContextID {
		return commandcontext.OwnedSnapshot{}, commandcontext.ErrOwnerMismatch
	}

	nativeSnapshot, err := p.reader.Capture(ctx)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return commandcontext.OwnedSnapshot{}, ctxErr
		}
		return commandcontext.OwnedSnapshot{}, commandcontext.ErrProviderUnavailable
	}
	if nativeSnapshot.CapturedAt.IsZero() || strings.TrimSpace(nativeSnapshot.Version) == "" {
		return commandcontext.OwnedSnapshot{}, commandcontext.ErrProviderUnavailable
	}
	if err := ctx.Err(); err != nil {
		return commandcontext.OwnedSnapshot{}, err
	}
	return commandcontext.NewOwnedSnapshot(scope, commandcontext.Snapshot{
		Version:    nativeSnapshot.Version,
		CapturedAt: nativeSnapshot.CapturedAt,
	})
}

// newCommandFactBus publica somente adapters backend reais. Surface, dialog,
// focus e window UI dependem de um trusted frontend adapter da I14 e ficam
// deliberadamente ausentes até essa porta existir.
func (a *App) newCommandFactBus(principal auth.LocalSessionPrincipal) (*commandcontext.FactBus, error) {
	workspaceProvider, err := a.newCommandWorkspaceProvider(principal)
	if err != nil {
		return nil, err
	}
	foregroundProvider, err := a.newCommandForegroundProvider(principal)
	if err != nil {
		return nil, err
	}
	return commandcontext.NewFactBus(map[string]commandcontext.ScopedProvider{
		"workspace":  workspaceProvider,
		"foreground": foregroundProvider,
	})
}

func validateCommandPrincipal(principal auth.LocalSessionPrincipal) error {
	scope := commandcontext.Scope{UserID: principal.UserID, AuthContextID: principal.SessionID}
	if err := scope.Validate(); err != nil {
		return commandcontext.ErrOwnerMismatch
	}
	return nil
}
