package app

import (
	"context"

	"assistente/internal/auth"
	"assistente/internal/commandexecution"
	"assistente/internal/commandledger"
)

// newCommandReadExecutor é uma fábrica interna, não uma API Wails nem startup.
// O bootstrap futuro fornece catálogo/handlers, política explícita, versões e
// estado de lock já mantido em memória. Não consultar Vault.Status sob o gate:
// isso faria I/O de keychain. Nenhuma dependência ausente recebe fallback.
// Construir somente no bootstrap serializado, antes de aceitar transições de
// autenticação. Substituição de SessionService exige construir novo executor.
func (a *App) newCommandReadExecutor(config commandexecution.Config) (*commandexecution.Service, error) {
	if a == nil || config.Snapshot == nil {
		return nil, commandexecution.ErrInvalidConfiguration
	}
	epochs, err := a.commandSecurityService()
	if err != nil {
		return nil, err
	}
	a.authMu.RLock()
	sessions, manager := a.sessionSvc, a.credMgr
	a.authMu.RUnlock()
	if sessions == nil || manager == nil {
		return nil, commandexecution.ErrInvalidConfiguration
	}
	keys, err := commandledger.NewCredentialKeyProvider(manager)
	if err != nil {
		return nil, err
	}
	snapshot := config.Snapshot
	config.Sessions, config.Epochs, config.Keys = sessions, epochs, keys
	config.Snapshot = func(ctx context.Context, principal auth.LocalSessionPrincipal) (commandexecution.Versions, error) {
		a.authMu.RLock()
		matches := a.sessionSvc == sessions && a.credMgr == manager && a.currentAuthUser != nil &&
			a.currentUserID == principal.UserID && a.currentAuthUser.UserID == principal.UserID && a.currentAuthUser.SessionID == principal.SessionID
		a.authMu.RUnlock()
		if !matches {
			return commandexecution.Versions{}, commandexecution.ErrDenied
		}
		return snapshot(ctx, principal)
	}
	return commandexecution.New(config)
}
