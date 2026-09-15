package app

import (
	"context"
	"slices"
	"strings"

	"assistente/internal/auth"
	"assistente/internal/commandbindings"
	"assistente/internal/commandconfig"
	"assistente/internal/commandexecution"
	"assistente/internal/database"
)

// commandMutationResult distingue falha de reconstrução de falha de commit.
// Committed=true proíbe repetir automaticamente a mutação confirmada.
type commandMutationResult struct {
	Diff      commandconfig.MutationDiff
	Committed bool
	Rebuilt   bool
}

type commandMutationApplier struct {
	apply func(context.Context, string, commandconfig.MutationIntent) (commandMutationResult, error)
}

func (s *commandMutationApplier) Apply(ctx context.Context, token string, intent commandconfig.MutationIntent) (commandMutationResult, error) {
	if s == nil || s.apply == nil || ctx == nil {
		return commandMutationResult{}, commandconfig.ErrInvalid
	}
	return s.apply(ctx, token, intent)
}

// newCommandMutationApplier compõe apenas o mapa global. Projection deve reler
// a união efetiva autoritativa após o commit, inclusive condições e expiração;
// não pode consultar o mapa suspenso nem reutilizar a prova anterior. Version
// cobre catálogo/defaults/política. Ambas as portas já são usadas sob gate pela
// fábrica base e precisam ser curtas, sem UI ou aquisição recursiva do gate.
// Nenhuma entrada de produto é instalada por esta composição.
func (a *App) newCommandMutationApplier(inputs commandCompleteMutationInputs) (*commandMutationApplier, error) {
	service, err := a.newCommandCompleteMutationService(inputs)
	if err != nil {
		return nil, err
	}
	a.authMu.RLock()
	state, sessions, manager, epochs := a.commandHost, a.sessionSvc, a.credMgr, a.commandEpochs
	storageVersion := a.commandStorageVersion
	a.authMu.RUnlock()
	db := database.DB()
	store, err := commandconfig.New(db)
	if err != nil {
		return nil, err
	}
	stable := func() bool {
		return a.commandMutationDependenciesStable(state, sessions, manager, epochs, storageVersion, db)
	}
	if !stable() {
		return nil, commandconfig.ErrStale
	}
	return &commandMutationApplier{apply: func(ctx context.Context, token string, intent commandconfig.MutationIntent) (commandMutationResult, error) {
		var result commandMutationResult
		principal, err := sessions.AuthenticateLocalAccess(ctx, token)
		if err != nil {
			return result, err
		}
		if !stable() || !a.commandPrincipalMatches(sessions, manager, principal) {
			return result, commandexecution.ErrDenied
		}
		result.Diff, err = service.Apply(ctx, token, nil, intent)
		if err != nil {
			return result, err
		}
		result.Committed = true
		scope := commandconfig.Scope{UserID: principal.UserID}
		var built *commandconfig.Snapshot
		var version string
		var active []string
		authenticate := func(ctx context.Context) (auth.LocalSessionPrincipal, error) {
			current, err := sessions.AuthenticateLocalAccess(ctx, token)
			if err != nil {
				return current, err
			}
			if !stable() || current.UserID != principal.UserID || current.SessionID != principal.SessionID || !a.commandPrincipalMatches(sessions, manager, current) {
				return current, commandexecution.ErrDenied
			}
			if err := inputs.Authorize(ctx, current, scope, intent.Operation); err != nil {
				return current, err
			}
			if built != nil {
				if err := store.CheckCurrent(ctx, *built); err != nil {
					return current, err
				}
				v, err := inputs.Version(ctx)
				if err != nil {
					return current, err
				}
				options, err := inputs.Projection(ctx, scope)
				if err != nil {
					return current, err
				}
				currentActive, err := commandMutationActiveLayerSet(options.ActiveUserLayerIDs)
				if err != nil {
					return current, err
				}
				if v != version || !slices.Equal(currentActive, active) {
					return current, commandconfig.ErrStale
				}
			}
			if !stable() || !a.commandPrincipalMatches(sessions, manager, current) {
				return current, commandconfig.ErrStale
			}
			// Portas confiáveis podem executar código que revogue a sessão.
			// Reconsultar o JWT também depois delas, antes da publicação.
			verified, err := sessions.AuthenticateLocalAccess(ctx, token)
			if err != nil {
				return current, err
			}
			if verified.UserID != principal.UserID || verified.SessionID != principal.SessionID {
				return current, commandconfig.ErrStale
			}
			return current, nil
		}
		err = state.RebuildUserConfiguration(ctx, authenticate, func(ctx context.Context, _ auth.LocalSessionPrincipal) (*commandbindings.Configuration, []string, error) {
			version, err = inputs.Version(ctx)
			if err != nil {
				return nil, nil, err
			}
			if version == "" {
				return nil, nil, commandconfig.ErrInvalid
			}
			snapshot, err := store.Load(ctx, scope)
			if err != nil {
				return nil, nil, err
			}
			// O diff público não carrega gerações. A auditoria do mesmo commit
			// fornece o vínculo autenticado, sem inventar uma geração esperada.
			audit, err := store.GetBindingMutation(ctx, principal.UserID, principal.SessionID, result.Diff.MutationID)
			if err != nil {
				return nil, nil, err
			}
			if len(snapshot.Generations) != 1 || snapshot.Generations[0].ID != audit.GenerationID || snapshot.Generations[0].Generation != audit.AfterGeneration {
				return nil, nil, commandconfig.ErrStale
			}
			options, err := inputs.Projection(ctx, scope)
			if err != nil {
				return nil, nil, err
			}
			active, err = commandMutationActiveLayerSet(options.ActiveUserLayerIDs)
			if err != nil {
				return nil, nil, err
			}
			configuration, err := commandconfig.ProjectComplete(ctx, snapshot, options)
			if err != nil {
				return nil, nil, err
			}
			if !stable() {
				return nil, nil, commandconfig.ErrStale
			}
			built = &snapshot
			return configuration, active, nil
		})
		result.Rebuilt = err == nil
		return result, err
	}}, nil
}

// A ordem do provider não é semântica. Copiar antes de ordenar preserva sua
// fotografia; duplicatas continuam inválidas, sem deduplicação permissiva.
func commandMutationActiveLayerSet(ids []string) ([]string, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	canonical := slices.Clone(ids)
	slices.Sort(canonical)
	for i, id := range canonical {
		if id == "" || strings.TrimSpace(id) != id || strings.ContainsRune(id, '\x00') || (i > 0 && canonical[i-1] == id) {
			return nil, commandconfig.ErrInvalid
		}
	}
	return canonical, nil
}
