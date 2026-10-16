package app

import (
	"context"
	"time"

	"assistente/internal/commanddecision"
	"assistente/internal/commandexecution"
	"assistente/internal/commandledger"
	"assistente/internal/commandsecurity"
	"assistente/internal/database"
)

// drainCommandExecutors fecha o mesmo core das fábricas do App. A construção
// por sync.Once serializa inclusive uma fábrica concorrente que ainda não
// tinha obtido o core. Depois desta chamada não há registro/admissão nova.
// Não é um binding Wails, não inicializa banco/cofre e não recupera outra
// instância por inferência. Erro impede destruir dependências do executor.
func (a *App) drainCommandExecutors(ctx context.Context) error {
	if a == nil || ctx == nil {
		return commandexecution.ErrInvalidRequest
	}
	core, err := a.commandSecurityService()
	if err != nil {
		return err
	}
	drained, err := core.CloseAndDrain(ctx)
	if err != nil {
		return err
	}
	if !a.commandDrainRecoveryReady() {
		return core.ReleaseInstance(ctx)
	}
	if err := recoverDrainedCommandDecisions(ctx, drained); err != nil {
		return err
	}
	if err := recoverDrainedCommandInvocations(ctx, drained); err != nil {
		return err
	}
	return core.ReleaseInstance(ctx)
}

func (a *App) commandDrainRecoveryReady() bool {
	if a == nil {
		return false
	}
	a.authMu.RLock()
	defer a.authMu.RUnlock()
	return a.commandStorageErr == nil && a.commandStorageVersion != ""
}

func recoverDrainedCommandDecisions(ctx context.Context, drained commandsecurity.DrainedGenerations) error {
	if ctx == nil {
		return commandexecution.ErrInvalidRequest
	}
	if !drained.Valid() {
		return nil
	}
	store, err := commanddecision.New(database.DB(), &commandDecisionPresenter{}, time.Now)
	if err != nil {
		return err
	}
	recovery, err := commanddecision.NewCoordinatorRecovery(store, drained)
	if err != nil {
		return err
	}
	for {
		result, err := recovery.Recover(ctx, commanddecision.MaxRecoveryBatch)
		if err != nil {
			return err
		}
		if !result.More {
			return nil
		}
	}
}

func recoverDrainedCommandInvocations(ctx context.Context, drained commandsecurity.DrainedGenerations) error {
	if ctx == nil {
		return commandexecution.ErrInvalidRequest
	}
	if !drained.Valid() {
		return nil
	}
	store, err := commandledger.New(database.DB(), time.Now)
	if err != nil {
		return err
	}
	recovery, err := commandledger.NewCoordinatorRecovery(store, drained)
	if err != nil {
		return err
	}
	for {
		result, err := recovery.Recover(ctx, 32)
		if err != nil {
			return err
		}
		if !result.More {
			return nil
		}
	}
}
