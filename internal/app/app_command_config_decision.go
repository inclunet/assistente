package app

import (
	"context"
	"time"

	"assistente/internal/auth"
	"assistente/internal/commandconfig"
	"assistente/internal/commanddecision"
	"assistente/internal/commandexecution"
	"assistente/internal/commandledger"
	"assistente/internal/commandsecurity"
)

// Borda interna de composição, sem método Wails/tool e sem bootstrap automático.
// Presenter, política, chave e renderizador são injetados pelo host confiável.
// A espera pelo diálogo e a leitura da chave ficam fora do DispatchGate.
func (a *App) changeCommandBindingEnabledWithDecision(ctx context.Context, token string,
	store *commandconfig.Store, bindingID string, enabled bool, options commandconfig.LocalReadProjection,
	authorize func(context.Context, auth.LocalSessionPrincipal) error, receipts *commanddecision.Store,
	version string, keys commandledger.FingerprintKeyProvider,
	render func(commandconfig.Binding, commandconfig.Binding) (string, error),
) error {
	if ctx == nil || store == nil || receipts == nil || keys == nil || render == nil || len(options.ActiveUserLayerIDs) != 0 {
		return commandexecution.ErrInvalidConfiguration
	}
	state, authenticate, err := a.commandBindingWriteHost(token, authorize)
	if err != nil {
		return err
	}
	return state.ChangeUserConfigurationWithEpoch(ctx, authenticate,
		func(ctx context.Context, principal auth.LocalSessionPrincipal, epoch commandsecurity.EpochSnapshot) (func(context.Context) error, error) {
			change, err := store.PrepareBindingEnabled(ctx, commandconfig.Scope{UserID: principal.UserID}, bindingID, enabled, options)
			if err != nil {
				return nil, err
			}
			confirmed, err := store.ConfirmBindingEnabled(ctx, change, epoch, receipts, version, keys, time.Now().Add(2*time.Minute), render)
			if err != nil {
				return nil, err
			}
			return func(ctx context.Context) error {
				// HostState acaba de revalidar o mesmo epoch e a política sob gate
				// exclusivo, antes de invalidar o mapa e entregar este callback.
				return store.CommitConfirmedBindingEnabled(ctx, confirmed, epoch)
			}, nil
		})
}
