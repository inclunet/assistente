package app

import (
	"context"

	"assistente/internal/auth"
	"assistente/internal/commandexecution"
	"assistente/internal/commandledger"
)

// newCommandReadExecutor é uma fábrica interna, não uma API Wails. Com o ciclo
// de vida do App iniciado, instala também seu único observador de sessão do SO.
// A fábrica ainda não é chamada pelo startup de produto nem habilita rotas.
// O bootstrap fornece catálogo/handlers, política explícita e HostState concreto
// (desconhecido começa bloqueado). Não consultar Vault.Status sob o gate:
// isso faria I/O de keychain. Nenhuma dependência ausente recebe fallback.
// Construir somente no bootstrap serializado, antes de aceitar transições de
// autenticação. Substituição de SessionService exige construir novo executor.
func (a *App) newCommandReadExecutor(config commandexecution.Config, state *commandexecution.HostState) (*commandexecution.Service, error) {
	if a == nil || state == nil {
		return nil, commandexecution.ErrInvalidConfiguration
	}
	epochs, err := a.commandSecurityService()
	if err != nil {
		return nil, err
	}
	if state.Epochs() != epochs {
		return nil, commandexecution.ErrInvalidConfiguration
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
	config.Sessions, config.Epochs, config.Keys = sessions, epochs, keys
	config.Snapshot = func(ctx context.Context, principal auth.LocalSessionPrincipal) (commandexecution.Versions, error) {
		a.authMu.RLock()
		matches := a.sessionSvc == sessions && a.credMgr == manager && a.currentAuthUser != nil &&
			a.currentUserID == principal.UserID && a.currentAuthUser.UserID == principal.UserID && a.currentAuthUser.SessionID == principal.SessionID
		a.authMu.RUnlock()
		if !matches {
			return commandexecution.Versions{}, commandexecution.ErrDenied
		}
		return state.Snapshot(ctx, principal)
	}
	service, err := commandexecution.New(config)
	if err != nil {
		return nil, err
	}
	a.authMu.Lock()
	defer a.authMu.Unlock()
	if a.commandHost != nil && a.commandHost != state {
		return nil, commandexecution.ErrInvalidConfiguration
	}
	a.commandHost = state
	a.startCommandOSSessionMonitorLocked()
	return service, nil
}

// Hooks curtos, chamados dentro da barreira de autenticação, mas fora do seu
// lock. Não leem/gravam cofre ou configuração em disco. Falhas deixam o serviço
// indisponível (HostState desabilitado/epoch esgotado), sem mudar retornos legados.
func (a *App) resetCommandHostSession(lockVault bool) {
	a.authMu.RLock()
	state, user := a.commandHost, a.currentUserID
	a.authMu.RUnlock()
	if state == nil {
		return
	}
	if user != "" {
		_ = state.ForgetUserConfiguration(context.Background(), user)
	}
	if lockVault {
		_ = state.SetVaultUnlocked(context.Background(), false)
	}
}

func (a *App) markCommandVaultUnlocked() {
	a.authMu.RLock()
	state := a.commandHost
	a.authMu.RUnlock()
	if state != nil {
		_ = state.SetVaultUnlocked(context.Background(), true)
	}
}
