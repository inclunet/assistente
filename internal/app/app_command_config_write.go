package app

import (
	"context"

	"assistente/internal/auth"
	"assistente/internal/commandconfig"
	"assistente/internal/commandexecution"
)

// changeCommandBindingEnabled é uma borda interna, não um método Wails/tool.
// confirm pertence ao adapter confiável de decisão: deve mostrar o diff exato e
// retornar nil somente após aprovação explícita. Não há booleano de cliente que
// substitua essa decisão. A ligação ao DecisionDialog/ledger de write é futura.
// authorize também é obrigatório, autoritativo, curto e sem reentrada no gate;
// não há política permissiva padrão nem inferência de permissão pelo owner.
// Após tentativa de commit o mapa fica ausente, exigindo reconstrução autenticada.
func (a *App) changeCommandBindingEnabled(ctx context.Context, token string, store *commandconfig.Store,
	bindingID string, enabled bool, options commandconfig.LocalReadProjection,
	authorize func(context.Context, auth.LocalSessionPrincipal) error,
	confirm func(context.Context, commandconfig.Binding, commandconfig.Binding) error,
) error {
	if a == nil || ctx == nil || store == nil || authorize == nil || confirm == nil || len(options.ActiveUserLayerIDs) != 0 {
		return commandexecution.ErrInvalidConfiguration
	}
	state, authenticate, err := a.commandBindingWriteHost(token, authorize)
	if err != nil {
		return err
	}
	return state.ChangeUserConfiguration(ctx, authenticate, func(ctx context.Context, principal auth.LocalSessionPrincipal) (func(context.Context) error, error) {
		change, err := store.PrepareBindingEnabled(ctx, commandconfig.Scope{UserID: principal.UserID}, bindingID, enabled, options)
		if err != nil {
			return nil, err
		}
		before, after := change.Diff()
		if err := confirm(ctx, before, after); err != nil {
			return nil, err
		}
		return func(ctx context.Context) error { return store.CommitBindingEnabled(ctx, change) }, nil
	})
}

// Compartilhado pelas bordas internas: não duplicar ou enfraquecer a política
// de sessão ao ligar a confirmação persistida. authenticate só roda sob gate.
func (a *App) commandBindingWriteHost(token string, authorize func(context.Context, auth.LocalSessionPrincipal) error) (
	*commandexecution.HostState, func(context.Context) (auth.LocalSessionPrincipal, error), error,
) {
	if a == nil || authorize == nil {
		return nil, nil, commandexecution.ErrInvalidConfiguration
	}
	a.authMu.RLock()
	state, sessions, manager := a.commandHost, a.sessionSvc, a.credMgr
	a.authMu.RUnlock()
	if state == nil || sessions == nil || manager == nil {
		return nil, nil, commandexecution.ErrInvalidConfiguration
	}
	authenticate := func(ctx context.Context) (auth.LocalSessionPrincipal, error) {
		principal, err := sessions.AuthenticateLocalAccess(ctx, token)
		if err != nil {
			return auth.LocalSessionPrincipal{}, err
		}
		a.authMu.RLock()
		matches := a.commandHost == state && a.sessionSvc == sessions && a.credMgr == manager &&
			a.currentAuthUser != nil && a.currentUserID == principal.UserID &&
			a.currentAuthUser.UserID == principal.UserID && a.currentAuthUser.SessionID == principal.SessionID
		a.authMu.RUnlock()
		if !matches {
			return auth.LocalSessionPrincipal{}, commandexecution.ErrDenied
		}
		// Política autoritativa curta, reavaliada sob o gate nas duas etapas.
		// Ser proprietário e ter confirmado não substitui permissão de escrita.
		if err := authorize(ctx, principal); err != nil {
			return auth.LocalSessionPrincipal{}, err
		}
		return principal, nil
	}
	return state, authenticate, nil
}
