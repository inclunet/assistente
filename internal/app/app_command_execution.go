package app

import (
	"context"
	"time"

	"assistente/internal/auth"
	"assistente/internal/commandexecution"
	"assistente/internal/commandledger"
	"assistente/internal/credentials"
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
	prepared, err := a.prepareCommandExecutor(config, state)
	if err != nil {
		return nil, err
	}
	config = prepared.config
	config.Snapshot = a.commandSnapshotCallback(prepared.sessions, prepared.manager, state)
	service, err := commandexecution.New(config)
	if err != nil {
		return nil, err
	}
	if err := a.installCommandHost(state); err != nil {
		closeUninstalledCommandService(service, config.FinalizationTimeout)
		return nil, err
	}
	return service, nil
}

type preparedCommandExecutor struct {
	config   commandexecution.Config
	sessions *auth.SessionService
	manager  *credentials.Manager
}

// prepareCommandExecutor é a parte comum das fábricas local e completa. Ela
// somente captura dependências já montadas pelo bootstrap; não cria epochs,
// sessão ou uma política paralela.
func (a *App) prepareCommandExecutor(config commandexecution.Config, state *commandexecution.HostState) (preparedCommandExecutor, error) {
	if a == nil || state == nil {
		return preparedCommandExecutor{}, commandexecution.ErrInvalidConfiguration
	}
	epochs, err := a.commandSecurityService()
	if err != nil {
		return preparedCommandExecutor{}, err
	}
	if state.Epochs() != epochs {
		return preparedCommandExecutor{}, commandexecution.ErrInvalidConfiguration
	}
	a.authMu.RLock()
	sessions, manager := a.sessionSvc, a.credMgr
	a.authMu.RUnlock()
	if sessions == nil || manager == nil {
		return preparedCommandExecutor{}, commandexecution.ErrInvalidConfiguration
	}
	keys, err := commandledger.NewCredentialKeyProvider(manager)
	if err != nil {
		return preparedCommandExecutor{}, err
	}
	config.Sessions, config.Epochs, config.Keys = sessions, epochs, keys
	return preparedCommandExecutor{config: config, sessions: sessions, manager: manager}, nil
}

func (a *App) commandSnapshotCallback(sessions *auth.SessionService, manager *credentials.Manager, state *commandexecution.HostState) func(context.Context, auth.LocalSessionPrincipal) (commandexecution.Versions, error) {
	return func(ctx context.Context, principal auth.LocalSessionPrincipal) (commandexecution.Versions, error) {
		if !a.commandPrincipalMatches(sessions, manager, principal) {
			return commandexecution.Versions{}, commandexecution.ErrDenied
		}
		return state.Snapshot(ctx, principal)
	}
}

func (a *App) commandPrincipalMatches(sessions *auth.SessionService, manager *credentials.Manager, principal auth.LocalSessionPrincipal) bool {
	if a == nil || sessions == nil || manager == nil || principal.UserID == "" || principal.SessionID == "" {
		return false
	}
	a.authMu.RLock()
	defer a.authMu.RUnlock()
	return a.sessionSvc == sessions && a.credMgr == manager && a.currentAuthUser != nil &&
		a.currentUserID == principal.UserID && a.currentAuthUser.UserID == principal.UserID &&
		a.currentAuthUser.SessionID == principal.SessionID
}

func (a *App) installCommandHost(state *commandexecution.HostState) error {
	if a == nil || state == nil {
		return commandexecution.ErrInvalidConfiguration
	}
	// A fábrica registra o executor no core antes desta instalação. Adquirir
	// primeiro o mesmo lock da montagem/shutdown evita publicar um monitor
	// depois que App.Shutdown marcou o ciclo como terminal.
	a.commandLifecycleMount.Lock()
	defer a.commandLifecycleMount.Unlock()
	if a.commandLifecycleClosing {
		return commandexecution.ErrInvalidConfiguration
	}
	a.authMu.Lock()
	defer a.authMu.Unlock()
	if a.commandHost != nil && a.commandHost != state {
		return commandexecution.ErrInvalidConfiguration
	}
	a.commandHost = state
	a.startCommandOSSessionMonitorLocked()
	return nil
}

func closeUninstalledCommandService(service *commandexecution.Service, timeout time.Duration) {
	if service == nil {
		return
	}
	if timeout <= 0 {
		timeout = time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	_ = service.Shutdown(ctx)
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
