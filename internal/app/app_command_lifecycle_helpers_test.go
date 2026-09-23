package app

// Helpers de caracterização de primitivas; não são caminhos alternativos de
// bootstrap de produto. O produto publica exclusivamente ProjectComplete.
import (
	"assistente/internal/auth"
	"assistente/internal/commandbindings"
	"assistente/internal/commandconfig"
	"assistente/internal/commandexecution"
	"assistente/internal/database"
	"context"
	"errors"
	"time"
)

func (a *App) rebuildCommandLifecycleEmptyConfiguration(ctx context.Context) error {
	if a == nil {
		return commandexecution.ErrInvalidConfiguration
	}
	a.authMu.RLock()
	state := a.commandHost
	a.authMu.RUnlock()
	if state == nil {
		return commandexecution.ErrInvalidConfiguration
	}
	configuration, err := commandbindings.NewConfiguration(nil, nil, nil)
	if err != nil {
		return err
	}
	return state.RebuildUserConfiguration(ctx, func(context.Context) (auth.LocalSessionPrincipal, error) {
		return a.currentCommandPrincipal()
	}, func(context.Context, auth.LocalSessionPrincipal) (*commandbindings.Configuration, []string, error) {
		return configuration, nil, nil
	})
}

func (a *App) loadCommandLifecyclePersistedConfiguration(ctx context.Context, store *commandconfig.Store, options commandconfig.LocalReadProjection) (commandLifecycleLoadedConfiguration, bool, error) {
	if store == nil {
		return commandLifecycleLoadedConfiguration{}, false, commandexecution.ErrInvalidConfiguration
	}
	principal, err := a.currentCommandPrincipal()
	if err != nil {
		return commandLifecycleLoadedConfiguration{}, false, err
	}
	publicationScope, err := a.commandMutationCurrentScope(principal)
	if err != nil {
		return commandLifecycleLoadedConfiguration{}, false, err
	}
	if err := store.EnsureScope(ctx, publicationScope); err != nil {
		return commandLifecycleLoadedConfiguration{}, false, err
	}
	snapshot, err := store.Load(ctx, publicationScope)
	if err != nil {
		if errors.Is(err, commandconfig.ErrInvalid) {
			hasGeneration, generationErr := commandLifecycleHasBaseGeneration(ctx, publicationScope)
			if generationErr != nil {
				return commandLifecycleLoadedConfiguration{}, false, generationErr
			}
			if !hasGeneration {
				return commandLifecycleLoadedConfiguration{}, false, nil
			}
		}
		return commandLifecycleLoadedConfiguration{}, false, err
	}
	activeLayers := commandLifecycleActiveUserLayerIDs(snapshot, principal, time.Now())
	options.ActiveUserLayerIDs = activeLayers
	configuration, err := commandconfig.ProjectLocalRead(ctx, snapshot, options)
	if err != nil {
		return commandLifecycleLoadedConfiguration{}, false, err
	}
	return commandLifecycleLoadedConfiguration{app: a, store: store, principal: principal, workspaceID: cloneCommandWorkspace(publicationScope.WorkspaceID), snapshot: snapshot, configuration: configuration, activeLayers: activeLayers}, true, nil
}

func commandLifecycleHasBaseGeneration(ctx context.Context, scope commandconfig.Scope) (bool, error) {
	var count int64
	query := database.DB().WithContext(ctx).Model(&commandconfig.Generation{}).Where("user_id = ?", scope.UserID)
	if scope.WorkspaceID == nil {
		query = query.Where("workspace_id IS NULL")
	} else {
		query = query.Where("workspace_id IS NULL OR workspace_id = ?", *scope.WorkspaceID)
	}
	err := query.
		Count(&count).Error
	if err != nil {
		return false, err
	}
	want := int64(1)
	if scope.WorkspaceID != nil {
		want = 2
	}
	return count == want, nil
}
