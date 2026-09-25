package app

import (
	"context"

	"assistente/internal/auth"
	"assistente/internal/commanddecision"
	"assistente/internal/commandexecution"
	"assistente/internal/commandsecurity"
)

// Recuperação interna anterior à reconstrução autenticada do mapa. Um lote por
// chamada, sem loop sob gate. Revalida JWT, sessão do App, cofre/SO e política
// antes de escrever. O mapa fica ausente após a tentativa: o bootstrap futuro
// deve terminar os lotes e reconstruí-lo explicitamente; nunca reutiliza cache.
// Não migra o banco, registra presenter ou acessa o Credential Manager.
func (a *App) recoverCommandDecisionSession(ctx context.Context, token string, receipts *commanddecision.Store,
	authorize func(context.Context, auth.LocalSessionPrincipal) error,
) (commanddecision.RecoveryResult, error) {
	if ctx == nil || receipts == nil {
		return commanddecision.RecoveryResult{}, commandexecution.ErrInvalidConfiguration
	}
	state, authenticate, err := a.commandBindingWriteHost(token, authorize)
	if err != nil {
		return commanddecision.RecoveryResult{}, err
	}
	var result commanddecision.RecoveryResult
	err = state.ChangeUserConfigurationWithEpoch(ctx, authenticate, func(_ context.Context, _ auth.LocalSessionPrincipal, epoch commandsecurity.EpochSnapshot) (func(context.Context) error, error) {
		return func(commitCtx context.Context) error {
			var err error
			result, err = receipts.ReconcileSession(commitCtx, epoch, commanddecision.MaxRecoveryBatch)
			return err
		}, nil
	})
	if err != nil {
		return commanddecision.RecoveryResult{}, err
	}
	return result, nil
}
