package app

import (
	"context"

	"assistente/internal/commandsecurity"
)

// Uma instância por App, sem banco/keychain/bootstrap de handlers. O erro de
// inicialização fica retido: comandos falham fechados sem impedir logout legado.
func (a *App) commandSecurityService() (*commandsecurity.EpochService, error) {
	a.commandEpochsOnce.Do(func() {
		a.commandGate = &commandsecurity.DispatchGate{}
		a.commandEpochs, a.commandEpochsErr = commandsecurity.NewEpochService(a.commandGate)
	})
	return a.commandEpochs, a.commandEpochsErr
}

// Ordem: authSessionMu (quando aplicável) -> gate -> authMu. Admit futuro nunca
// deve adquirir authSessionMu. A barreira não mantém locks durante os runtimes,
// callbacks do cofre ou I/O dos fluxos existentes. Não altera seus retornos.
func (a *App) beginCommandAuthTransition() func() {
	service, err := a.commandSecurityService()
	if err != nil {
		return func() {}
	} // Sem serviço, nenhuma admissão é possível.
	finish, err := service.BeginTransition(context.Background())
	if err != nil {
		return func() {}
	} // Exaustão desabilita Capture/Admit permanentemente.
	return finish
}
