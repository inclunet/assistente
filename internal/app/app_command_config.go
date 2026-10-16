package app

import (
	"context"

	"assistente/internal/auth"
	"assistente/internal/commandbindings"
	"assistente/internal/commandconfig"
	"assistente/internal/commandexecution"
)

// rebuildPersistedLocalReadConfiguration instala o projetor estrito do
// subconjunto local de leitura. Camadas de usuário permanecem inativas: o
// restore autenticado de claims ainda não foi implementado. Dependências e
// defaults pertencem ao bootstrap confiável, não ao chamador Wails.
func (a *App) rebuildPersistedLocalReadConfiguration(ctx context.Context, token string, store *commandconfig.Store, options commandconfig.LocalReadProjection) error {
	if len(options.ActiveUserLayerIDs) != 0 {
		return commandexecution.ErrInvalidConfiguration
	}
	return a.rebuildPersistedCommandConfiguration(ctx, token, store,
		func(ctx context.Context, snapshot commandconfig.Snapshot) (*commandbindings.Configuration, error) {
			return commandconfig.ProjectLocalRead(ctx, snapshot, options)
		})
}

// rebuildPersistedCommandConfiguration conecta a leitura SQLite à publicação
// autenticada. Somente escopo global: HostState ainda não separa workspaces.
// project é obrigatório e pertence ao bootstrap confiável: valida versões e
// schemas dos documentos, catálogo, defaults e ausência de segredos brutos.
// Não há projetor permissivo padrão nem API Wails, gravação, migração automática
// ou restore de claims. Uma configuração carregada não é autorização.
func (a *App) rebuildPersistedCommandConfiguration(ctx context.Context, token string, store *commandconfig.Store,
	project func(context.Context, commandconfig.Snapshot) (*commandbindings.Configuration, error),
) error {
	if store == nil || project == nil {
		return commandexecution.ErrInvalidConfiguration
	}
	var loaded commandconfig.Snapshot
	var hasSnapshot bool
	return a.rebuildCommandUserConfigurationChecked(ctx, token,
		func(ctx context.Context, principal auth.LocalSessionPrincipal) (*commandbindings.Configuration, []string, error) {
			snapshot, err := store.Load(ctx, commandconfig.Scope{UserID: principal.UserID})
			if err != nil {
				return nil, nil, err
			}
			loaded, hasSnapshot = snapshot, true
			configuration, err := project(ctx, snapshot)
			return configuration, nil, err
		}, func(ctx context.Context) error {
			if !hasSnapshot {
				return nil
			}
			return store.CheckCurrent(ctx, loaded)
		})
}
