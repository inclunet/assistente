package app

import (
	"context"

	"assistente/internal/auth"
	"assistente/internal/commandexecution"
)

type commandDesktopMutationSession struct {
	app       *App
	sessions  *auth.SessionService
	principal auth.LocalSessionPrincipal
}

func (s commandDesktopMutationSession) AuthenticateLocalAccess(ctx context.Context, token string) (auth.LocalSessionPrincipal, error) {
	if token != "" || s.app == nil || s.sessions == nil || ctx == nil {
		return auth.LocalSessionPrincipal{}, commandexecution.ErrDenied
	}
	s.app.authMu.RLock()
	current := s.app.currentAuthUser
	valid := s.app.sessionSvc == s.sessions && current != nil && s.app.currentUserID == s.principal.UserID && current.UserID == s.principal.UserID && current.SessionID == s.principal.SessionID
	s.app.authMu.RUnlock()
	if !valid {
		return auth.LocalSessionPrincipal{}, commandexecution.ErrDenied
	}
	return s.sessions.RevalidateLocalSession(ctx, s.principal)
}

// A sessão fica vinculada na criação, sem JWT do cliente e sem recapturar
// outro usuário após uma decisão demorada.
func (a *App) newCommandDesktopMutationApplier(inputs commandCompleteMutationInputs) (*commandMutationApplier, error) {
	if a == nil {
		return nil, commandexecution.ErrDenied
	}
	principal, err := a.currentCommandPrincipal()
	if err != nil {
		return nil, err
	}
	a.authMu.RLock()
	sessions := a.sessionSvc
	a.authMu.RUnlock()
	authenticator := commandDesktopMutationSession{app: a, sessions: sessions, principal: principal}
	if _, err := authenticator.AuthenticateLocalAccess(a.commandBridgeContext(), ""); err != nil {
		return nil, err
	}
	return a.newCommandMutationApplierAuthenticated(inputs, authenticator)
}
