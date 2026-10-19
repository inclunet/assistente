package app

import (
	"context"
	"time"

	"assistente/internal/auth"
	"assistente/internal/commanddecision"
	"assistente/internal/commandexecution"
	"assistente/internal/commandsecurity"
	"assistente/internal/database"
	"assistente/internal/logging"
)

// O bootstrap desktop não possui JWT. Usa a identidade local revalidada e a
// mesma fronteira de publicação autenticada do recovery explícito. Só encerra
// recibos vencidos/obsoletos da própria sessão, de configuração ou invocação:
// não consome aceitações, executa efeitos ou altera invocações/ledger. Chamar
// sob commandBootstrap, após retirar a publicação e antes de reconstruí-la.
func (a *App) recoverCurrentCommandDecisionSession(ctx context.Context) error {
	if a == nil || ctx == nil {
		return commandexecution.ErrInvalidConfiguration
	}
	ctx, cancel := context.WithTimeout(ctx, commandReadTimeout)
	defer cancel()
	p := a.commandProduct.Load()
	if p == nil || !p.dependenciesMatch(a) {
		return commandexecution.ErrStale
	}
	receipts, err := commanddecision.New(database.DB(), &commandDecisionPresenter{manager: a.questionnaireMgr}, time.Now)
	if err != nil {
		return err
	}
	authenticate := func(authCtx context.Context) (auth.LocalSessionPrincipal, error) {
		if a.commandProduct.Load() != p || !p.dependenciesMatch(a) {
			return auth.LocalSessionPrincipal{}, commandexecution.ErrStale
		}
		current, err := p.sessionSvc.RevalidateLocalSession(authCtx, p.principal)
		if err != nil {
			return auth.LocalSessionPrincipal{}, err
		}
		if current != p.principal || a.commandProduct.Load() != p || !p.dependenciesMatch(a) {
			return auth.LocalSessionPrincipal{}, commandexecution.ErrDenied
		}
		return current, nil
	}
	for {
		var result commanddecision.RecoveryResult
		// A transação de recibos abortada por BUSY pode ser repetida. A fronteira
		// também invalida watches: por isso os chamadores já retiraram a
		// publicação. A espera fica fora do gate; cada tentativa revalida
		// identidade, cofre, SO e epochs antes de reconstruir a configuração.
		err := database.WithSQLiteBusyRetry(ctx, "command decision bootstrap recovery", func() error {
			return p.host.ChangeUserConfigurationWithEpoch(ctx, authenticate, func(_ context.Context, _ auth.LocalSessionPrincipal, epoch commandsecurity.EpochSnapshot) (func(context.Context) error, error) {
				return func(commitCtx context.Context) error {
					var err error
					result, err = receipts.ReconcileSession(commitCtx, epoch, commanddecision.MaxRecoveryBatch)
					return err
				}, nil
			})
		})
		if err != nil {
			return err
		}
		if result.Closed > 0 {
			logging.Infof(context.Background(), "app.commands", "Confirmações obsoletas reconciliadas antes do bootstrap: closed=%d more=%t", result.Closed, result.More)
		}
		if !result.More {
			return nil
		}
		if err := ctx.Err(); err != nil {
			return err
		}
	}
}

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
