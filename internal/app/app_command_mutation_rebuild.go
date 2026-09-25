package app

import (
	"context"
	"slices"
	"strings"

	"assistente/internal/auth"
	"assistente/internal/commandbindings"
	"assistente/internal/commandconfig"
	"assistente/internal/commandexecution"
	"assistente/internal/commandportability"
	"assistente/internal/database"
	"assistente/internal/portability"
)

// commandMutationResult distingue falha de reconstrução de falha de commit.
// Committed=true proíbe repetir automaticamente a mutação confirmada.
type commandMutationResult struct {
	Diff      commandconfig.MutationDiff
	Committed bool
	Rebuilt   bool
}

// O commit do lote é indivisível. Rebuilt descreve a única publicação do
// mapa ativo, não uma publicação intermediária por escopo.
type commandMutationBatchResult struct {
	Diffs     []commandconfig.MutationDiff
	Report    *commandportability.ImportReport
	Committed bool
	Rebuilt   bool
}

type commandMutationApplier struct {
	apply               func(context.Context, string, *string, commandconfig.MutationIntent) (commandMutationResult, error)
	upgradeDefaults     func(context.Context, string, *string) (commandMutationResult, error)
	rebaseDefault       func(context.Context, string, *string, commandconfig.DefaultRebaseRequest) (commandMutationResult, error)
	regrantEventRule    func(context.Context, string, *string, string) (commandMutationResult, error)
	importEnvelope      func(context.Context, string, *string, []byte, commandportability.PlanOptions, commandportability.ReferencePort) (commandMutationResult, error)
	importEnvelopeBatch func(context.Context, string, []byte, commandportability.PlanOptions, commandportability.ReferencePort) (commandMutationBatchResult, error)
}

// Defaults use the same confirmation, invalidation and publication boundary as
// ordinary edits. A committed mutation must never be retried after publish fails.
func (s *commandMutationApplier) UpgradeDefaults(ctx context.Context, token string, workspace *string) (commandMutationResult, error) {
	if s == nil || s.upgradeDefaults == nil || ctx == nil {
		return commandMutationResult{}, commandconfig.ErrInvalid
	}
	return s.upgradeDefaults(ctx, token, workspace)
}

func (s *commandMutationApplier) RegrantEventRule(ctx context.Context, token string, workspace *string, ruleID string) (commandMutationResult, error) {
	if s == nil || s.regrantEventRule == nil || ctx == nil {
		return commandMutationResult{}, commandconfig.ErrInvalid
	}
	return s.regrantEventRule(ctx, token, workspace, ruleID)
}

func (s *commandMutationApplier) RebaseDefault(ctx context.Context, token string, workspace *string, request commandconfig.DefaultRebaseRequest) (commandMutationResult, error) {
	if s == nil || s.rebaseDefault == nil || ctx == nil {
		return commandMutationResult{}, commandconfig.ErrInvalid
	}
	return s.rebaseDefault(ctx, token, workspace, request)
}

func (s *commandMutationApplier) ImportEnvelopeBatch(ctx context.Context, token string, raw []byte, options commandportability.PlanOptions, refs commandportability.ReferencePort) (commandMutationBatchResult, error) {
	if s == nil || s.importEnvelopeBatch == nil || ctx == nil {
		return commandMutationBatchResult{}, commandconfig.ErrInvalid
	}
	return s.importEnvelopeBatch(ctx, token, raw, options, refs)
}

// ImportEnvelope usa a mesma suspensão, auditoria e reconstrução de Apply.
// As portas pertencem ao host; o arquivo nunca fornece autoridade de owner,
// referências ou acesso a workspace.
func (s *commandMutationApplier) ImportEnvelope(ctx context.Context, token string, workspace *string, raw []byte, options commandportability.PlanOptions, refs commandportability.ReferencePort) (commandMutationResult, error) {
	if s == nil || s.importEnvelope == nil || ctx == nil {
		return commandMutationResult{}, commandconfig.ErrInvalid
	}
	return s.importEnvelope(ctx, token, workspace, raw, options, refs)
}

func (s *commandMutationApplier) Apply(ctx context.Context, token string, intent commandconfig.MutationIntent) (commandMutationResult, error) {
	if s == nil || s.apply == nil || ctx == nil {
		return commandMutationResult{}, commandconfig.ErrInvalid
	}
	return s.apply(ctx, token, nil, intent)
}

func (s *commandMutationApplier) ApplyScoped(ctx context.Context, token string, workspace *string, intent commandconfig.MutationIntent) (commandMutationResult, error) {
	if s == nil || s.apply == nil || ctx == nil {
		return commandMutationResult{}, commandconfig.ErrInvalid
	}
	return s.apply(ctx, token, workspace, intent)
}

// newCommandMutationApplier compõe a união global+workspace autorizada pelo host. Projection deve reler
// a união efetiva autoritativa após o commit, inclusive condições e expiração;
// não pode consultar o mapa suspenso nem reutilizar a prova anterior. Version
// cobre catálogo/defaults/política. Ambas as portas já são usadas sob gate pela
// fábrica base e precisam ser curtas, sem UI ou aquisição recursiva do gate.
// Nenhuma entrada de produto é instalada por esta composição.
func (a *App) newCommandMutationApplier(inputs commandCompleteMutationInputs) (*commandMutationApplier, error) {
	return a.newCommandMutationApplierAuthenticated(inputs, nil)
}

func (a *App) newCommandMutationApplierAuthenticated(inputs commandCompleteMutationInputs, authenticator commandMutationSessions) (*commandMutationApplier, error) {
	service, err := a.newCommandCompleteMutationServiceAuthenticated(inputs, authenticator)
	if err != nil {
		return nil, err
	}
	a.authMu.RLock()
	state, sessions, manager, epochs := a.commandHost, a.sessionSvc, a.credMgr, a.commandEpochs
	storageVersion := a.commandStorageVersion
	a.authMu.RUnlock()
	if authenticator == nil {
		authenticator = sessions
	}
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
	run := func(ctx context.Context, token string, operation commandconfig.Operation, mutate func(context.Context) ([]commandconfig.MutationDiff, error)) (commandMutationBatchResult, error) {
		var result commandMutationBatchResult
		principal, err := authenticator.AuthenticateLocalAccess(ctx, token)
		if err != nil {
			return result, err
		}
		if !stable() || !a.commandPrincipalMatches(sessions, manager, principal) {
			return result, commandexecution.ErrDenied
		}
		ctx = database.WithUserID(ctx, principal.UserID)
		result.Diffs, err = mutate(ctx)
		if err != nil {
			return result, err
		}
		result.Committed = true
		if len(result.Diffs) == 0 || len(result.Diffs) > 64 {
			return result, commandconfig.ErrInvalid
		}
		seenScopes := make(map[string]bool, len(result.Diffs))
		for _, diff := range result.Diffs {
			key := "global"
			if diff.Scope.WorkspaceID != nil {
				key = "workspace:" + *diff.Scope.WorkspaceID
			}
			if diff.Scope.UserID != principal.UserID || diff.Operation != operation || seenScopes[key] {
				return result, commandconfig.ErrInvalid
			}
			seenScopes[key] = true
		}
		// O alvo alterado não determina o mapa da tela. Uma alteração global ou
		// em outro workspace precisa preservar a união do workspace ativo.
		publicationScope, err := a.commandMutationCurrentScope(principal)
		if err != nil {
			return result, err
		}
		var built *commandconfig.Snapshot
		var mutationsBuilt []commandconfig.Snapshot
		var version string
		var active []string
		var projectionGuard func(context.Context) error
		authenticate := func(ctx context.Context) (auth.LocalSessionPrincipal, error) {
			current, err := authenticator.AuthenticateLocalAccess(ctx, token)
			if err != nil {
				return current, err
			}
			if !stable() || current.UserID != principal.UserID || current.SessionID != principal.SessionID || !a.commandPrincipalMatches(sessions, manager, current) {
				return current, commandexecution.ErrDenied
			}
			for _, diff := range result.Diffs {
				if err := inputs.Authorize(ctx, current, commandconfig.Scope{UserID: principal.UserID, WorkspaceID: cloneCommandWorkspace(diff.Scope.WorkspaceID)}, operation); err != nil {
					return current, err
				}
			}
			currentScope, err := a.commandMutationCurrentScope(current)
			if err != nil || !sameCommandWorkspace(currentScope.WorkspaceID, publicationScope.WorkspaceID) {
				return current, commandconfig.ErrStale
			}
			if built != nil {
				v, err := inputs.Version(ctx)
				if err != nil {
					return current, err
				}
				options, err := inputs.Projection(ctx, publicationScope)
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
			verified, err := authenticator.AuthenticateLocalAccess(ctx, token)
			if err != nil {
				return current, err
			}
			if verified.UserID != principal.UserID || verified.SessionID != principal.SessionID {
				return current, commandconfig.ErrStale
			}
			currentScope, err = a.commandMutationCurrentScope(verified)
			if err != nil || !sameCommandWorkspace(currentScope.WorkspaceID, publicationScope.WorkspaceID) {
				return current, commandconfig.ErrStale
			}
			// Conferir dados depois das portas: uma revalidação de projeção
			// também pode tornar obsoleto um escopo que não está na tela.
			if built != nil {
				if err := store.CheckCurrent(ctx, *built); err != nil {
					return current, err
				}
				for _, mutated := range mutationsBuilt {
					if err := store.CheckCurrent(ctx, mutated); err != nil {
						return current, err
					}
				}
			}
			return current, nil
		}
		err = state.RebuildUserConfigurationGuarded(ctx, authenticate, func(ctx context.Context, _ auth.LocalSessionPrincipal) (*commandbindings.Configuration, []string, error) {
			version, err = inputs.Version(ctx)
			if err != nil {
				return nil, nil, err
			}
			if version == "" {
				return nil, nil, commandconfig.ErrInvalid
			}
			snapshot, err := store.Load(ctx, publicationScope)
			if err != nil {
				return nil, nil, err
			}
			// O diff público não carrega gerações. A auditoria do mesmo commit
			// fornece o vínculo autenticado, sem inventar uma geração esperada.
			for _, diff := range result.Diffs {
				audit, err := store.GetBindingMutation(ctx, principal.UserID, principal.SessionID, diff.MutationID)
				if err != nil {
					return nil, nil, err
				}
				mutated, err := store.Load(ctx, diff.Scope)
				if err != nil {
					return nil, nil, err
				}
				if audit.Operation != string(operation) || !commandMutationAuditMatchesSnapshot(mutated, audit.GenerationID, audit.AfterGeneration) {
					return nil, nil, commandconfig.ErrStale
				}
				mutationsBuilt = append(mutationsBuilt, mutated)
			}
			options, err := inputs.Projection(ctx, publicationScope)
			if err != nil {
				return nil, nil, err
			}
			active, err = commandMutationActiveLayerSet(options.ActiveUserLayerIDs)
			if err != nil {
				return nil, nil, err
			}
			var configuration *commandbindings.Configuration
			publishedActive := active
			if inputs.BuildConfiguration != nil {
				configuration, publishedActive, projectionGuard, err = inputs.BuildConfiguration(ctx, publicationScope, snapshot, options)
			} else {
				configuration, err = commandconfig.ProjectComplete(ctx, snapshot, options)
			}
			if err != nil {
				return nil, nil, err
			}
			if !stable() {
				return nil, nil, commandconfig.ErrStale
			}
			built = &snapshot
			return configuration, publishedActive, nil
		}, func(ctx context.Context) error {
			if projectionGuard != nil {
				return projectionGuard(ctx)
			}
			return nil
		})
		result.Rebuilt = err == nil
		if result.Rebuilt {
			if p := a.commandProduct.Load(); p != nil {
				p.scheduleCommandManualExpiry()
			}
		}
		if result.Rebuilt && a.emitter != nil {
			a.emitter.Emit("command:keyboard-map-changed", nil)
		}
		return result, err
	}
	return &commandMutationApplier{
		regrantEventRule: func(ctx context.Context, token string, workspace *string, ruleID string) (commandMutationResult, error) {
			target := cloneCommandWorkspace(workspace)
			result, err := run(ctx, token, commandconfig.RuleEnable, func(ctx context.Context) ([]commandconfig.MutationDiff, error) {
				diff, err := service.RegrantEventRule(ctx, token, target, ruleID)
				if err != nil {
					return nil, err
				}
				return []commandconfig.MutationDiff{diff}, nil
			})
			return singleCommandMutationResult(result), err
		},
		upgradeDefaults: func(ctx context.Context, token string, workspace *string) (commandMutationResult, error) {
			target := cloneCommandWorkspace(workspace)
			result, err := run(ctx, token, commandconfig.DefaultUpgrade, func(ctx context.Context) ([]commandconfig.MutationDiff, error) {
				diff, err := service.UpgradeDefaults(ctx, token, target)
				if err != nil {
					return nil, err
				}
				return []commandconfig.MutationDiff{diff}, nil
			})
			return singleCommandMutationResult(result), err
		},
		rebaseDefault: func(ctx context.Context, token string, workspace *string, request commandconfig.DefaultRebaseRequest) (commandMutationResult, error) {
			target := cloneCommandWorkspace(workspace)
			frozen := request
			if request.Condition != nil {
				frozen.Condition = make(commandbindings.Facts, len(request.Condition))
				for key, value := range request.Condition {
					frozen.Condition[key] = value
				}
			}
			result, err := run(ctx, token, commandconfig.DefaultRebase, func(ctx context.Context) ([]commandconfig.MutationDiff, error) {
				diff, err := service.RebaseDefault(ctx, token, target, frozen)
				if err != nil {
					return nil, err
				}
				return []commandconfig.MutationDiff{diff}, nil
			})
			return singleCommandMutationResult(result), err
		},
		apply: func(ctx context.Context, token string, workspace *string, intent commandconfig.MutationIntent) (commandMutationResult, error) {
			target := cloneCommandWorkspace(workspace)
			result, err := run(ctx, token, intent.Operation, func(ctx context.Context) ([]commandconfig.MutationDiff, error) {
				diff, err := service.Apply(ctx, token, target, intent)
				if err != nil {
					return nil, err
				}
				return []commandconfig.MutationDiff{diff}, nil
			})
			return singleCommandMutationResult(result), err
		},
		importEnvelope: func(ctx context.Context, token string, workspace *string, raw []byte, options commandportability.PlanOptions, refs commandportability.ReferencePort) (commandMutationResult, error) {
			frozen := slices.Clone(raw)
			// Identidade e referências privadas são sempre resolvidas no banco
			// do App, sob o usuário autenticado por run, nunca pelo arquivo.
			owner := commandportability.NewStoreOwnership(db)
			options.Name = commandportability.NewStoreLayerName(db)
			refs.CredentialPattern = commandportability.NewCredentialPatternResolver(db)
			target := cloneCommandWorkspace(workspace)
			result, err := run(ctx, token, commandconfig.ConfigImport, func(ctx context.Context) ([]commandconfig.MutationDiff, error) {
				diff, err := portability.ApplyCommandEnvelope(ctx, service, token, target, frozen, options, owner, refs)
				if err != nil {
					return nil, err
				}
				return []commandconfig.MutationDiff{diff}, nil
			})
			return singleCommandMutationResult(result), err
		},
		importEnvelopeBatch: func(ctx context.Context, token string, raw []byte, options commandportability.PlanOptions, refs commandportability.ReferencePort) (commandMutationBatchResult, error) {
			frozen := slices.Clone(raw)
			options.Name = commandportability.NewStoreLayerName(db)
			refs.CredentialPattern = commandportability.NewCredentialPatternResolver(db)
			owner := commandportability.NewStoreOwnership(db)
			var report *commandportability.ImportReport
			result, err := run(ctx, token, commandconfig.ConfigImport, func(ctx context.Context) ([]commandconfig.MutationDiff, error) {
				imported, err := portability.ApplyCommandEnvelopeBatch(ctx, service, token, frozen, options, owner, refs)
				report = imported.Report
				return imported.Diffs, err
			})
			result.Report = report
			return result, err
		},
	}, nil
}

func singleCommandMutationResult(batch commandMutationBatchResult) commandMutationResult {
	result := commandMutationResult{Committed: batch.Committed, Rebuilt: batch.Rebuilt}
	if len(batch.Diffs) == 1 {
		result.Diff = batch.Diffs[0]
	}
	return result
}

func cloneCommandWorkspace(workspace *string) *string {
	if workspace == nil {
		return nil
	}
	value := *workspace
	return &value
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

func (a *App) commandMutationCurrentScope(principal auth.LocalSessionPrincipal) (commandconfig.Scope, error) {
	if a == nil || principal.UserID == "" {
		return commandconfig.Scope{}, commandconfig.ErrInvalid
	}
	a.authMu.RLock()
	defer a.authMu.RUnlock()
	if a.currentAuthUser == nil || a.currentUserID != principal.UserID || a.currentAuthUser.UserID != principal.UserID || a.currentAuthUser.SessionID != principal.SessionID {
		return commandconfig.Scope{}, commandconfig.ErrStale
	}
	scope := commandconfig.Scope{UserID: principal.UserID}
	if a.workspaceMgr == nil {
		return scope, nil
	}
	active, err := a.workspaceMgr.CommandSnapshot()
	if err != nil {
		return commandconfig.Scope{}, err
	}
	workspace := active.WorkspaceID
	scope.WorkspaceID = &workspace
	return scope, nil
}

func sameCommandWorkspace(left, right *string) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

func commandMutationAuditMatchesSnapshot(snapshot commandconfig.Snapshot, generationID string, after int64) bool {
	for _, generation := range snapshot.Generations {
		if generation.ID == generationID && generation.Generation == after && sameCommandWorkspace(generation.WorkspaceID, snapshot.Scope.WorkspaceID) {
			return true
		}
	}
	return false
}
