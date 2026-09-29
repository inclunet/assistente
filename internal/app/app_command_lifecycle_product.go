package app

import (
	"context"
	"errors"
	"time"

	"assistente/internal/auth"
	"assistente/internal/commandactivation"
	"assistente/internal/commandbindings"
	"assistente/internal/commandconfig"
	"assistente/internal/commandexecution"
	"assistente/internal/commandsecurity"
	"assistente/internal/database"
)

// ensureCommandLifecycleMountedForCurrentUser instala o executor operacional
// e o catálogo de produto, sem comando sentinela ou callback permissivo.
func (a *App) ensureCommandLifecycleMountedForCurrentUser(ctx context.Context) error {
	return a.mountCommandProduct(ctx)
}

func (a *App) rebuildCommandLifecyclePersistedConfiguration(ctx context.Context) error {
	return a.rebuildCommandLifecycleProjection(ctx, true)
}

func (a *App) rebuildCommandLifecycleProjection(ctx context.Context, restoreManual bool) error {
	return a.rebuildCommandLifecycleProjectionMode(ctx, restoreManual, false)
}

func (a *App) rebuildCommandLifecycleProjectionMode(ctx context.Context, restoreManual, jobClaimProjection bool) error {
	if a == nil || ctx == nil {
		return commandexecution.ErrInvalidConfiguration
	}
	product := a.commandProduct.Load()
	if product == nil {
		return commandexecution.ErrStale
	}
	return a.rebuildCommandLifecycleProjectionAtReset(ctx, restoreManual, jobClaimProjection, product, product.projectionResetRevision.Load())
}

func (a *App) rebuildCommandLifecycleProjectionAtReset(ctx context.Context, restoreManual, jobClaimProjection bool, product *commandProductRuntime, resetRevision uint64) error {
	// Só repetimos leituras/projeção antes de qualquer execução de comando.
	// Restore concluído não é repetido; suas transações abortadas por BUSY
	// usam retry próprio. Concorrência
	// contínua permanece indisponível após três tentativas, sem loop infinito.
	for attempt := 0; attempt < 3; attempt++ {
		err := a.buildCommandLifecycleProjectionAtReset(ctx, restoreManual && attempt == 0, jobClaimProjection, product, resetRevision)
		if !errors.Is(err, commandexecution.ErrStale) && !errors.Is(err, commandconfig.ErrStale) {
			return err
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
	}
	return commandexecution.ErrStale
}

func (a *App) buildCommandLifecycleProjection(ctx context.Context, restoreManual, jobClaimProjection bool) error {
	if a == nil || ctx == nil {
		return commandexecution.ErrInvalidConfiguration
	}
	product := a.commandProduct.Load()
	if product == nil {
		return commandexecution.ErrStale
	}
	resetRevision := product.projectionResetRevision.Load()
	return a.buildCommandLifecycleProjectionAtReset(ctx, restoreManual, jobClaimProjection, product, resetRevision)
}

func (a *App) buildCommandLifecycleProjectionAtReset(ctx context.Context, restoreManual, jobClaimProjection bool, product *commandProductRuntime, resetRevision uint64) error {
	if a.commandProduct.Load() != product || product.projectionResetRevision.Load() != resetRevision {
		return commandexecution.ErrStale
	}
	store, err := commandconfig.New(database.DB())
	if err != nil {
		return err
	}
	registry, _, err := a.commandProductCatalog()
	if err != nil {
		return err
	}
	principal, err := a.currentCommandPrincipal()
	if err != nil {
		return err
	}
	scope, err := a.commandMutationCurrentScope(principal)
	if err != nil {
		return err
	}
	if err := database.WithSQLiteBusyRetry(ctx, "command lifecycle ensure scope", func() error {
		return store.EnsureScope(ctx, scope)
	}); err != nil {
		return err
	}
	if restoreManual {
		if err := a.restoreCommandLifecyclePersistentClaims(ctx); err != nil {
			return err
		}
	}
	claimRevision := product.claimTransitionRevision.Load()
	if product.currentCommandClaimTransition() != nil {
		return commandexecution.ErrStale
	}
	jobProjection, guard, err := a.commandJobLayerProjection(ctx, principal, scope)
	if err != nil {
		return err
	}
	snapshot, err := store.Load(ctx, scope)
	if err != nil {
		return err
	}
	if commandManualClaimsDue(snapshot, time.Now()) {
		if err := a.reconcileCommandLifecycleClaims(ctx, false); err != nil {
			return err
		}
		snapshot, err = store.Load(ctx, scope)
		if err != nil {
			return err
		}
	}
	deadline := commandManualClaimsDeadline(snapshot, principal, time.Now())
	epoch, manualGuard, err := a.commandProduct.Load().commandManualClaimAuthority(ctx)
	if err != nil {
		return err
	}
	active := commandCurrentManualLayerIDs(snapshot, principal, time.Now(), epoch)
	if jobProjection != nil {
		active = mergeCommandLayerIDs(active, jobProjection.layers)
	}
	projection, err := a.commandProductGlobalProjection(ctx, registry, active)
	if err != nil {
		return err
	}
	configuration, err := commandconfig.ProjectComplete(ctx, snapshot, projection)
	if err != nil {
		return err
	}
	if jobProjection != nil {
		configuration, err = configuration.WithLayerProvenance(jobProjection.sources)
		if err != nil {
			return err
		}
	}
	persistedBaseline, err := snapshot.PersistedFingerprint()
	if err != nil {
		return err
	}
	configuration, err = configuration.WithPersistedBaseline(persistedBaseline)
	if err != nil {
		return err
	}
	configuration = configuration.WithValidityDeadline(deadline)
	jobGuard := guard
	guard = func(ctx context.Context) error {
		if a.commandProduct.Load() != product || product.projectionResetRevision.Load() != resetRevision || product.currentCommandClaimTransition() != nil || product.claimTransitionRevision.Load() != claimRevision {
			return commandexecution.ErrStale
		}
		if err := manualGuard(ctx); err != nil {
			return err
		}
		if !deadline.IsZero() && !time.Now().Before(deadline) {
			return commandexecution.ErrStale
		}
		if jobGuard != nil {
			return jobGuard(ctx)
		}
		return ctx.Err()
	}
	return (commandLifecycleLoadedConfiguration{app: a, store: store, principal: principal, workspaceID: cloneCommandWorkspace(scope.WorkspaceID), snapshot: snapshot, configuration: configuration, activeLayers: active, guard: guard, jobClaimProjection: jobClaimProjection, jobProjectionCurrent: jobProjection.current, publicationResetRevision: resetRevision}).publish(ctx)
}

type commandLifecycleLoadedConfiguration struct {
	app                      *App
	store                    *commandconfig.Store
	principal                auth.LocalSessionPrincipal
	workspaceID              *string
	snapshot                 commandconfig.Snapshot
	configuration            *commandbindings.Configuration
	activeLayers             []string
	guard                    func(context.Context) error
	jobClaimProjection       bool
	jobProjectionCurrent     func() bool
	publicationEpoch         commandsecurity.EpochSnapshot
	publicationResetRevision uint64
}

func (a *App) restoreCommandLifecyclePersistentClaims(ctx context.Context) error {
	return a.reconcileCommandLifecycleClaims(ctx, true)
}

func (a *App) reconcileCommandLifecycleClaims(ctx context.Context, restore bool) error {
	return a.reconcileCommandLifecycleClaimsForTransition(ctx, restore, false)
}

// Uma transição que já suspendeu o mapa precisa terminar antes da republicação.
// Guardamos a operação, não uma configuração antiga a ser reutilizada.
type commandClaimTransition struct {
	restore         bool
	workspaceSwitch bool
	resetRevision   uint64
}

func (p *commandProductRuntime) currentCommandClaimTransition() *commandClaimTransition {
	pending := p.claimTransition.Load()
	if pending != nil && pending.resetRevision != p.projectionResetRevision.Load() {
		return nil
	}
	return pending
}

func (a *App) reconcileCommandLifecycleClaimsForTransition(ctx context.Context, restore, workspaceSwitch bool) error {
	if a == nil || ctx == nil {
		return commandexecution.ErrInvalidConfiguration
	}
	original, err := a.currentCommandPrincipal()
	if err != nil {
		return err
	}
	product := a.commandProduct.Load()
	if product == nil || product.principal != original {
		return commandexecution.ErrStale
	}
	transitionRevision := product.projectionResetRevision.Load()
	return a.reconcileCommandLifecycleClaimsAtReset(ctx, restore, workspaceSwitch, product, transitionRevision)
}

func (a *App) reconcileCommandLifecycleClaimsAtReset(ctx context.Context, restore, workspaceSwitch bool, product *commandProductRuntime, transitionRevision uint64) error {
	if a.commandProduct.Load() != product || product.projectionResetRevision.Load() != transitionRevision {
		return commandexecution.ErrStale
	}
	original := product.principal
	a.authMu.RLock()
	state := a.commandHost
	sessions, credentials := a.sessionSvc, a.credMgr
	a.authMu.RUnlock()
	if state == nil || sessions == nil || credentials == nil {
		return commandexecution.ErrInvalidConfiguration
	}
	// Cada falha BUSY desfaz a transação de claims. A espera fica fora do
	// gate de segurança; cada tentativa captura epochs novos e revalida a
	// mesma sessão. Nunca repetimos execução de comandos neste caminho.
	return database.WithSQLiteBusyRetry(ctx, "command lifecycle restore persistent claims", func() error {
		authenticate := func(authCtx context.Context) (auth.LocalSessionPrincipal, error) {
			if !a.commandPrincipalMatches(sessions, credentials, original) {
				return auth.LocalSessionPrincipal{}, commandexecution.ErrDenied
			}
			validated, err := sessions.RevalidateLocalSession(authCtx, original)
			if err != nil {
				return auth.LocalSessionPrincipal{}, err
			}
			if validated != original || !a.commandPrincipalMatches(sessions, credentials, original) {
				return auth.LocalSessionPrincipal{}, commandexecution.ErrDenied
			}
			return original, nil
		}
		return state.ChangeUserConfigurationWithEpoch(ctx, authenticate, func(prepareCtx context.Context, principal auth.LocalSessionPrincipal, epoch commandsecurity.EpochSnapshot) (func(context.Context) error, error) {
			if principal != original || !a.commandPrincipalMatches(sessions, credentials, original) {
				return nil, commandexecution.ErrDenied
			}
			scope, err := a.commandMutationCurrentScope(principal)
			if err != nil {
				return nil, err
			}
			owner := commandactivation.Owner{
				Scope:           commandactivation.Scope{UserID: scope.UserID, WorkspaceID: scope.WorkspaceID},
				AuthContextType: "local_session", AuthContextID: principal.SessionID,
				AuthGeneration: epoch.AuthGeneration, SecurityGeneration: epoch.SecurityGeneration,
			}
			return func(commitCtx context.Context) error {
				if a.commandProduct.Load() != product || product.projectionResetRevision.Load() != transitionRevision {
					return commandexecution.ErrStale
				}
				if pending := product.currentCommandClaimTransition(); pending != nil &&
					(pending.restore != restore || pending.workspaceSwitch != workspaceSwitch) {
					// Outra operação não pode apagar uma restauração incompleta.
					return commandexecution.ErrStale
				}
				transition := &commandClaimTransition{restore: restore, workspaceSwitch: workspaceSwitch, resetRevision: transitionRevision}
				// A revisão cobre inclusive leitura que começou antes do Store e
				// terminou depois do commit/CAS, quando a marca já está limpa.
				product.claimTransitionRevision.Add(1)
				defer product.claimTransitionRevision.Add(1)
				product.claimTransition.Store(transition)
				service, err := newCommandLifecycleActivationService(owner)
				if err != nil {
					return err
				}
				if workspaceSwitch {
					_, err = service.RestoreForWorkspace(commitCtx, owner, commandLifecycleRestoreOrigin(principal))
				} else if restore {
					_, err = service.RestorePersistent(commitCtx, owner, commandLifecycleRestoreOrigin(principal))
				} else {
					_, err = service.Expire(commitCtx, owner)
				}
				if err == nil {
					product.claimTransition.CompareAndSwap(transition, nil)
				}
				return err
			}, nil
		})
	})
}

func newCommandLifecycleActivationService(owner commandactivation.Owner) (*commandactivation.Service, error) {
	store, err := commandactivation.NewStore(database.DB())
	if err != nil {
		return nil, err
	}
	return commandactivation.New(database.DB(), &commandsecurity.DispatchGate{}, commandactivation.Ports{
		Owner: commandactivation.OwnerPortFunc(func(context.Context, commandactivation.Owner) (commandactivation.Owner, error) {
			return owner, nil
		}),
		Layer: commandLifecycleActivationLayerPort{},
		Origin: commandactivation.OriginPortFunc(func(_ context.Context, _ commandactivation.Owner, origin commandactivation.Origin) (commandactivation.Origin, error) {
			return origin, nil
		}),
		Rule:         store,
		GenerationTx: store,
	}, time.Now)
}

type commandLifecycleActivationLayerPort struct{}

func (commandLifecycleActivationLayerPort) ResolveLayer(ctx context.Context, owner commandactivation.Owner, ref commandactivation.Ref) (commandactivation.Layer, error) {
	if ref.Kind != commandactivation.UserRef {
		return commandactivation.Layer{}, commandactivation.ErrNotFound
	}
	var layer commandconfig.Layer
	query := database.DB().WithContext(ctx).Where("id = ? AND user_id = ?", ref.ID, owner.UserID)
	if owner.WorkspaceID == nil {
		query = query.Where("workspace_id IS NULL")
	} else {
		query = query.Where("workspace_id IS NULL OR workspace_id = ?", *owner.WorkspaceID).Order("CASE WHEN workspace_id IS NULL THEN 1 ELSE 0 END")
	}
	if err := query.First(&layer).Error; err != nil {
		return commandactivation.Layer{}, commandactivation.ErrNotFound
	}
	return commandactivation.Layer{Ref: commandactivation.Ref{Kind: commandactivation.UserRef, ID: layer.ID}, UserID: layer.UserID, WorkspaceID: layer.WorkspaceID, Enabled: layer.Enabled}, nil
}

func commandLifecycleRestoreOrigin(principal auth.LocalSessionPrincipal) commandactivation.Origin {
	return commandactivation.Origin{Type: "ui_action", SessionID: principal.SessionID, DeviceID: "command-lifecycle"}
}

func commandLifecycleActiveUserLayerIDs(snapshot commandconfig.Snapshot, principal auth.LocalSessionPrincipal, now time.Time) []string {
	rules := map[string]commandactivation.Rule{}
	for _, rule := range snapshot.ActivationRules {
		if rule.UserID != snapshot.Scope.UserID || !commandLifecycleInScope(rule.WorkspaceID, snapshot.Scope.WorkspaceID) || !rule.Enabled || rule.ReviewStatus != "active" || !commandManualLifecycleSupported(rule.Lifecycle) {
			continue
		}
		key := commandLifecycleRuleKey(rule.LayerRefKind, rule.LayerRef, rule.RuleRefKind, rule.RuleRef, rule.WorkspaceID)
		if previous, exists := rules[key]; !exists || previous.WorkspaceID == nil && rule.WorkspaceID != nil {
			rules[key] = rule
		}
	}
	layers := map[string]commandconfig.Layer{}
	for _, layer := range snapshot.Layers {
		if layer.UserID == snapshot.Scope.UserID && commandLifecycleInScope(layer.WorkspaceID, snapshot.Scope.WorkspaceID) && layer.Enabled {
			layers[commandLifecycleLayerKey(layer.ID, layer.WorkspaceID)] = layer
		}
	}
	active := map[string]bool{}
	var ids []string
	for _, claim := range snapshot.ActivationClaims {
		if claim.UserID != snapshot.Scope.UserID || !commandLifecycleInScope(claim.WorkspaceID, snapshot.Scope.WorkspaceID) || claim.State != commandactivation.StateActive ||
			claim.SourceType != "manual" || claim.AuthContextID != principal.SessionID ||
			claim.LayerRefKind != commandactivation.UserRef || claim.ExpiresAt != nil && !claim.ExpiresAt.After(now) {
			continue
		}
		key := commandLifecycleRuleKey(claim.LayerRefKind, claim.LayerRef, claim.RuleRefKind, claim.RuleRef, claim.WorkspaceID)
		rule, ok := rules[key]
		if !ok || rule.Lifecycle == commandactivation.LifecycleTemporary && claim.ExpiresAt == nil {
			continue
		}
		if _, ok := layers[commandLifecycleLayerKey(claim.LayerRef, claim.WorkspaceID)]; !ok || active[claim.LayerRef] {
			continue
		}
		active[claim.LayerRef] = true
		ids = append(ids, claim.LayerRef)
	}
	return ids
}

func commandLifecycleInScope(workspace, current *string) bool {
	return workspace == nil || current != nil && *workspace == *current
}

func commandLifecycleLayerKey(id string, workspace *string) string {
	if workspace == nil {
		return id + "\x00global"
	}
	return id + "\x00" + *workspace
}

func commandLifecycleRuleKey(layerKind commandactivation.RefKind, layer string, ruleKind commandactivation.RefKind, rule string, workspace *string) string {
	workspaceKey := "global"
	if workspace != nil {
		workspaceKey = *workspace
	}
	return string(layerKind) + "\x00" + layer + "\x00" + string(ruleKind) + "\x00" + rule + "\x00" + workspaceKey
}

func (loaded commandLifecycleLoadedConfiguration) publish(ctx context.Context) error {
	if ctx == nil || loaded.app == nil || loaded.store == nil || loaded.configuration == nil || loaded.principal.UserID == "" || loaded.principal.SessionID == "" {
		return commandexecution.ErrInvalidConfiguration
	}
	product := loaded.app.commandProduct.Load()
	if product == nil || product.principal != loaded.principal {
		return commandexecution.ErrStale
	}
	if product.projectionResetRevision.Load() != loaded.publicationResetRevision {
		return commandexecution.ErrStale
	}
	recoveryProof, _ := ctx.Value(commandProjectionRecoveryProofKey{}).(context.Context)
	epoch, err := product.epochs.Capture(ctx, loaded.principal.UserID, loaded.principal.SessionID)
	if err != nil {
		return err
	}
	publicationCtx, release, err := product.epochs.WatchSecurityEpoch(ctx, epoch)
	if err != nil {
		return err
	}
	defer release()
	ctx = publicationCtx
	loaded.publicationEpoch = epoch
	if loaded.jobClaimProjection {
		return loaded.publishJobClaimProjection(ctx)
	}
	err = loaded.app.rebuildCommandLifecycleConfigurationChecked(ctx, func(ctx context.Context, principal auth.LocalSessionPrincipal) (*commandbindings.Configuration, []string, error) {
		if principal != loaded.principal || principal.UserID != loaded.snapshot.Scope.UserID {
			return nil, nil, commandexecution.ErrDenied
		}
		if err := loaded.store.CheckCurrent(ctx, loaded.snapshot); err != nil {
			return nil, nil, err
		}
		currentScope, err := loaded.app.commandMutationCurrentScope(principal)
		if err != nil || !sameCommandWorkspace(currentScope.WorkspaceID, loaded.workspaceID) {
			return nil, nil, commandexecution.ErrStale
		}
		return loaded.configuration, loaded.activeLayers, nil
	}, func(ctx context.Context) error {
		if product.projectionResetRevision.Load() != loaded.publicationResetRevision {
			return commandexecution.ErrStale
		}
		// Revalidado também dentro do gate de publicação. Não depender da
		// entrega assíncrona de AfterFunc para impedir publicação após reset.
		if recoveryProof != nil && recoveryProof.Err() != nil {
			return commandexecution.ErrStale
		}
		principal, err := loaded.app.currentCommandPrincipal()
		if err != nil || principal != loaded.principal {
			return commandexecution.ErrDenied
		}
		if err := loaded.store.CheckCurrent(ctx, loaded.snapshot); err != nil {
			return err
		}
		currentScope, err := loaded.app.commandMutationCurrentScope(principal)
		if err != nil || !sameCommandWorkspace(currentScope.WorkspaceID, loaded.workspaceID) {
			return commandexecution.ErrStale
		}
		return nil
	}, loaded.guard)
	if err == nil && loaded.app.emitter != nil {
		loaded.app.emitter.Emit("command:keyboard-map-changed", nil)
	}
	if err == nil {
		if p := loaded.app.commandProduct.Load(); p != nil && p.principal == loaded.principal {
			p.rememberPersistedCommandConfiguration(loaded.store, loaded.snapshot, loaded.publicationEpoch, loaded.publicationResetRevision)
			p.scheduleCommandManualExpiry()
		}
	}
	return err
}

func (loaded commandLifecycleLoadedConfiguration) publishJobClaimProjection(ctx context.Context) error {
	if ctx == nil {
		return commandexecution.ErrInvalidConfiguration
	}
	if loaded.jobProjectionCurrent == nil || !loaded.jobProjectionCurrent() {
		return commandexecution.ErrStale
	}
	recoveryProof, _ := ctx.Value(commandProjectionRecoveryProofKey{}).(context.Context)
	loaded.app.authMu.RLock()
	state := loaded.app.commandHost
	sessions, credentials := loaded.app.sessionSvc, loaded.app.credMgr
	loaded.app.authMu.RUnlock()
	product := loaded.app.commandProduct.Load()
	if state == nil || product == nil || product.principal != loaded.principal || sessions == nil || credentials == nil {
		return commandexecution.ErrInvalidConfiguration
	}
	err := state.RebuildUserConfigurationForJobClaimProjection(ctx,
		func(ctx context.Context) (auth.LocalSessionPrincipal, error) {
			if recoveryProof != nil && recoveryProof.Err() != nil {
				return auth.LocalSessionPrincipal{}, commandexecution.ErrStale
			}
			principal, err := loaded.app.currentCommandPrincipal()
			if err != nil || principal != loaded.principal || loaded.app.commandProduct.Load() != product || product.projectionResetRevision.Load() != loaded.publicationResetRevision || !product.dependenciesMatch(loaded.app) ||
				!loaded.app.commandPrincipalMatches(sessions, credentials, principal) {
				return auth.LocalSessionPrincipal{}, commandexecution.ErrDenied
			}
			validated, err := sessions.RevalidateLocalSession(ctx, principal)
			if err != nil {
				return auth.LocalSessionPrincipal{}, commandPrincipalRevalidationFailure(ctx, err)
			}
			if ctxErr := ctx.Err(); ctxErr != nil {
				return auth.LocalSessionPrincipal{}, ctxErr
			}
			if recoveryProof != nil && recoveryProof.Err() != nil {
				return auth.LocalSessionPrincipal{}, commandexecution.ErrStale
			}
			if validated != principal || !loaded.app.commandPrincipalMatches(sessions, credentials, principal) {
				return auth.LocalSessionPrincipal{}, commandexecution.ErrDenied
			}
			if err := loaded.store.CheckCurrent(ctx, loaded.snapshot); err != nil {
				return auth.LocalSessionPrincipal{}, err
			}
			currentScope, err := loaded.app.commandMutationCurrentScope(principal)
			if err != nil || !sameCommandWorkspace(currentScope.WorkspaceID, loaded.workspaceID) {
				return auth.LocalSessionPrincipal{}, commandexecution.ErrStale
			}
			if loaded.guard != nil {
				if err := loaded.guard(ctx); err != nil {
					return auth.LocalSessionPrincipal{}, err
				}
			}
			return principal, nil
		},
		func(context.Context, auth.LocalSessionPrincipal) (*commandbindings.Configuration, []string, error) {
			return loaded.configuration, loaded.activeLayers, nil
		}, loaded.guard, loaded.jobProjectionCurrent)
	if err == nil && loaded.app.emitter != nil && !product.localKeyboardProjectionCurrent(ctx) {
		loaded.app.emitter.Emit("command:keyboard-map-changed", nil)
	}
	if err == nil {
		if p := loaded.app.commandProduct.Load(); p != nil && p.principal == loaded.principal {
			p.rememberPersistedCommandConfiguration(loaded.store, loaded.snapshot, loaded.publicationEpoch, loaded.publicationResetRevision)
			p.scheduleCommandManualExpiry()
		}
	}
	return err
}

func commandPrincipalRevalidationFailure(ctx context.Context, err error) error {
	if ctx != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	return commandexecution.ErrDenied
}

func (a *App) rebuildCommandLifecycleConfigurationChecked(ctx context.Context,
	build func(context.Context, auth.LocalSessionPrincipal) (*commandbindings.Configuration, []string, error),
	check func(context.Context) error,
	guard func(context.Context) error,
) error {
	if a == nil {
		return commandexecution.ErrInvalidConfiguration
	}
	a.authMu.RLock()
	state := a.commandHost
	a.authMu.RUnlock()
	if state == nil {
		return commandexecution.ErrInvalidConfiguration
	}
	return state.RebuildUserConfigurationGuarded(ctx, func(ctx context.Context) (auth.LocalSessionPrincipal, error) {
		principal, err := a.currentCommandPrincipal()
		if err != nil {
			return auth.LocalSessionPrincipal{}, err
		}
		if check != nil {
			if err := check(ctx); err != nil {
				return auth.LocalSessionPrincipal{}, err
			}
		}
		return principal, nil
	}, build, guard)
}
