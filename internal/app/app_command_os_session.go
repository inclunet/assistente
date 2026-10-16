package app

import (
	"context"

	"assistente/internal/auth"
	"assistente/internal/commandbindings"
	"assistente/internal/commandexecution"
	"assistente/internal/logging"
	"assistente/internal/ossession"
)

// Chamado pelo bootstrap serializado, sob authMu. Um App sem ciclo de vida
// iniciado não lança observadores. Nunca tenta reiniciar silenciosamente um
// monitor que falhou; nesse caso o HostState permanece desconhecido/fechado.
func (a *App) startCommandOSSessionMonitorLocked() {
	a.startCommandOSSessionMonitorWithLocked(ossession.Watch)
}

func (a *App) startCommandOSSessionMonitorWithLocked(watch func(context.Context, func(ossession.State) error) error) {
	if watch == nil || a.commandHost == nil || a.commandOSStarted || a.ctx == nil || a.cancel == nil || a.ctx.Err() != nil {
		return
	}
	a.commandOSStarted = true
	ctx := a.ctx
	a.bgWG.Add(1)
	go func() {
		defer a.bgWG.Done()
		if err := a.observeCommandOSSession(ctx, watch); err != nil && ctx.Err() == nil {
			logging.Warnf(ctx, "app.commands", "Observador de sessão do SO indisponível: %v", err)
		}
	}()
}

// observeCommandOSSession bloqueia até cancelamento/erro e é rastreado no
// background do App. O seam permite testes sem criar janelas nativas.
func (a *App) observeCommandOSSession(ctx context.Context, watch func(context.Context, func(ossession.State) error) error) error {
	if a == nil || ctx == nil || watch == nil {
		return commandexecution.ErrInvalidConfiguration
	}
	a.authMu.RLock()
	state := a.commandHost
	a.authMu.RUnlock()
	if state == nil {
		return commandexecution.ErrInvalidConfiguration
	}
	// Não reutilizar observação de uma execução anterior do monitor.
	if err := state.SetOSSessionState(context.Background(), false, true); err != nil {
		a.clearExternalUIConnections()
		return err
	}
	a.clearExternalUIConnections()
	// A primeira observação chega depois da montagem e pode chegar depois
	// do bootstrap de autenticação. Unlock invalida os mapas no HostState;
	// portanto precisa reconstruí-los, não apenas mudar o bit de segurança.
	// I/O de reconstrução nunca bloqueia a recepção do próximo lock do SO.
	workerCtx, stopWorker := context.WithCancel(ctx)
	pending := make(chan context.Context, 1)
	workerDone := make(chan struct{})
	go func() {
		defer close(workerDone)
		for {
			select {
			case <-workerCtx.Done():
				return
			case rebuildCtx := <-pending:
				if rebuildCtx.Err() != nil {
					continue
				}
				a.bootstrapCommandLifecycleAfterOSUnlock(rebuildCtx, state)
			}
		}
	}()
	var cancelObservation context.CancelFunc
	defer func() {
		defer a.clearExternalUIConnections()
		stopWorker()
		if cancelObservation != nil {
			cancelObservation()
		}
		if err := state.SetOSSessionState(context.Background(), false, true); err != nil {
			// O reset fail-closed é tentado mesmo no encerramento; se o HostState
			// recusar a escrita, a falha precisa permanecer observável.
			logging.Warnf(context.Background(), "app.commands", "Falha ao fechar observação da sessão do SO: %v", err)
		}
		<-workerDone
	}()
	return watch(ctx, func(observed ossession.State) error {
		defer a.clearExternalUIConnections()
		if cancelObservation != nil {
			cancelObservation()
		}
		if err := state.SetOSSessionState(ctx, observed.Known, observed.Locked); err != nil {
			return err
		}
		if observed.Known && !observed.Locked {
			var rebuildCtx context.Context
			rebuildCtx, cancelObservation = context.WithCancel(workerCtx)
			// Só a observação mais recente importa; nunca acumular unlocks.
			select {
			case <-pending:
			default:
			}
			pending <- rebuildCtx
		}
		return nil
	})
}

// rebuildCommandUserConfiguration usa JWT apenas para autenticar a sessão
// local vigente, antes e depois do carregamento. Não armazena o token nem
// confia em IDs fornecidos pelo builder. O carregador de persistência pertence
// ao bootstrap; não há fallback para um mapa em cache.
func (a *App) rebuildCommandUserConfiguration(ctx context.Context, token string,
	build func(context.Context, auth.LocalSessionPrincipal) (*commandbindings.Configuration, []string, error),
) error {
	return a.rebuildCommandUserConfigurationChecked(ctx, token, build, nil)
}

// check roda dentro da reautenticação sob gate, antes da publicação. Deve
// apenas revalidar estado local, sem iniciar mutações ou readquirir o gate.
func (a *App) rebuildCommandUserConfigurationChecked(ctx context.Context, token string,
	build func(context.Context, auth.LocalSessionPrincipal) (*commandbindings.Configuration, []string, error),
	check func(context.Context) error,
) error {
	if a == nil {
		return commandexecution.ErrInvalidConfiguration
	}
	a.authMu.RLock()
	state, sessions, manager := a.commandHost, a.sessionSvc, a.credMgr
	a.authMu.RUnlock()
	if state == nil || sessions == nil || manager == nil {
		return commandexecution.ErrInvalidConfiguration
	}
	return state.RebuildUserConfiguration(ctx, func(ctx context.Context) (auth.LocalSessionPrincipal, error) {
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
		if check != nil {
			if err := check(ctx); err != nil {
				return auth.LocalSessionPrincipal{}, err
			}
		}
		return principal, nil
	}, build)
}
