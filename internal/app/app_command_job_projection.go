package app

import (
	"context"
	"errors"
	"slices"
	"time"

	"assistente/internal/auth"
	"assistente/internal/commandactivation"
	"assistente/internal/commandbindings"
	"assistente/internal/commandconfig"
	"assistente/internal/commandexecution"
	"assistente/internal/commandsecurity"
)

// commandJobLayerProjection carrega a prova durável somente na reconstrução.
// O guard publicado com o mapa relê apenas fontes locais em memória. Não é
// uma porta pública e não aceita claims nem identidades fornecidas pela UI.
type commandJobProjection struct {
	layers  []string
	sources map[string][]commandbindings.LayerProvenance
	current func() bool
}

type commandProjectionRecoveryProofKey struct{}

func (a *App) commandJobLayerProjection(ctx context.Context, principal auth.LocalSessionPrincipal, scope commandconfig.Scope) (*commandJobProjection, func(context.Context) error, error) {
	mounted := a.commandMaintenance.Load()
	if mounted == nil {
		projection := &commandJobProjection{current: func() bool { return a.commandMaintenance.Load() == nil }}
		return projection, func(ctx context.Context) error {
			if err := ctx.Err(); err != nil {
				return err
			}
			if a.commandMaintenance.Load() != nil {
				return commandexecution.ErrStale
			}
			return nil
		}, nil
	}
	a.authMu.RLock()
	manager := a.workspaceMgr
	a.authMu.RUnlock()
	if manager == nil {
		return nil, nil, commandexecution.ErrStale
	}
	workspace, err := manager.CommandSnapshot()
	if err != nil || scope.WorkspaceID == nil || workspace.WorkspaceID != *scope.WorkspaceID {
		return nil, nil, commandexecution.ErrStale
	}
	core, err := a.commandSecurityService()
	if err != nil {
		return nil, nil, err
	}
	epoch, err := core.Capture(ctx, principal.UserID, principal.SessionID)
	if err != nil {
		return nil, nil, err
	}
	owner := commandactivation.Owner{Scope: commandactivation.Scope{UserID: principal.UserID, WorkspaceID: cloneCommandWorkspace(scope.WorkspaceID)},
		AuthContextType: "local_session", AuthContextID: principal.SessionID,
		AuthGeneration: epoch.AuthGeneration, SecurityGeneration: epoch.SecurityGeneration}
	proof, err := mounted.consumer.Projection(ctx, owner)
	if err != nil {
		return nil, nil, err
	}
	var active []string
	sources := make(map[string][]commandbindings.LayerProvenance)
	for _, entry := range proof.Claims {
		if entry.Claim.LayerRefKind == commandactivation.UserRef {
			active = append(active, entry.Claim.LayerRef)
			sources[entry.Claim.LayerRef] = append(sources[entry.Claim.LayerRef], commandbindings.LayerProvenance{
				SourceID: entry.Claim.ActivationID, Provenance: entry.Provenance,
			})
		}
	}
	guard := func(ctx context.Context) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		// HostState executa este guard fora de seu mutex. Mantemos a ordem
		// auth -> workspace/runtime e a identidade da fonte, não apenas o
		// conteúdo/version de um Manager que pode já ter sido substituído.
		a.authMu.RLock()
		defer a.authMu.RUnlock()
		if a.workspaceMgr != manager || a.currentAuthUser == nil || a.currentUserID != principal.UserID ||
			a.currentAuthUser.UserID != principal.UserID || a.currentAuthUser.SessionID != principal.SessionID {
			return commandexecution.ErrStale
		}
		if a.commandMaintenance.Load() != mounted || mounted.consumer.ProjectionRevision() != proof.Revision ||
			!proof.ValidUntil.IsZero() && !time.Now().Before(proof.ValidUntil) {
			return commandexecution.ErrStale
		}
		current, err := manager.CommandSnapshot()
		if err != nil || current.Version != workspace.Version {
			return commandexecution.ErrStale
		}
		for _, entry := range proof.Claims {
			if err := mounted.manager.ValidateCommandRuntimeProjection(ctx, entry.RunID, entry.Runtime); err != nil {
				if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
					return err
				}
				return commandexecution.ErrStale
			}
		}
		return nil
	}
	current := func() bool {
		return a.commandMaintenance.Load() == mounted && mounted.consumer.ProjectionRevision() == proof.Revision &&
			(proof.ValidUntil.IsZero() || time.Now().Before(proof.ValidUntil))
	}
	if err := guard(ctx); err != nil {
		return nil, nil, err
	}
	return &commandJobProjection{layers: mergeCommandLayerIDs(nil, active), sources: sources, current: current}, guard, nil
}

func mergeCommandLayerIDs(a, b []string) []string {
	merged := append(append([]string(nil), a...), b...)
	slices.Sort(merged)
	return slices.Compact(merged)
}

// Uma notificação perdida não mantém mapa antigo: cada entrada consulta o
// guard local e reconstrói apenas se ele envelheceu. A admissão do executor
// relê o mesmo guard sob seu gate; não se repete uma execução já iniciada.
func (p *commandProductRuntime) refreshCommandJobProjection(ctx context.Context) error {
	if p == nil || p.app == nil || ctx == nil {
		return commandexecution.ErrStale
	}
	resetRevision := p.projectionResetRevision.Load()
	if p.app.commandProduct.Load() != p || !p.dependenciesMatch(p.app) {
		return commandexecution.ErrStale
	}
	_, err := p.host.Snapshot(ctx, p.principal)
	if !commandProjectionNeedsRebuild(err) {
		return err
	}
	p.projectionMu.Lock()
	defer p.projectionMu.Unlock()
	_, err = p.host.Snapshot(ctx, p.principal)
	if !commandProjectionNeedsRebuild(err) {
		return err
	}
	if p.app.commandProduct.Load() != p || !p.dependenciesMatch(p.app) || p.projectionResetRevision.Load() != resetRevision {
		return commandexecution.ErrStale
	}
	if errors.Is(err, commandexecution.ErrHostUserNotPublished) {
		return p.recoverCommandProjectionAtReset(ctx, false, resetRevision)
	}
	// A prova anterior autoriza somente a reconstrução contextual de job; essa
	// modalidade preserva execuções e heartbeat quando a projeção está stale.
	return p.recoverCommandProjectionAtReset(ctx, true, resetRevision)
}

func (p *commandProductRuntime) withCommandProjectionRecoveryProof(ctx context.Context, resetRevision uint64) (context.Context, func(), error) {
	if ctx == nil || p == nil || p.app == nil || p.app.commandProduct.Load() != p ||
		!p.dependenciesMatch(p.app) || p.projectionResetRevision.Load() != resetRevision {
		return nil, nil, commandexecution.ErrStale
	}
	p.persistedConfigMu.RLock()
	epoch, published := p.persistedConfigEpoch, p.hasPersistedSnapshot
	proof := p.projectionRecoveryContext
	p.persistedConfigMu.RUnlock()
	if !published || proof == nil || proof.Err() != nil {
		return nil, nil, commandexecution.ErrStale
	}
	bounded, cancel := context.WithCancel(ctx)
	stop := context.AfterFunc(proof, cancel)
	recoveryCtx, release, err := p.epochs.WatchSecurityEpoch(bounded, epoch)
	if err != nil {
		stop()
		cancel()
		return nil, nil, err
	}
	cleanup := func() {
		stop()
		cancel()
		release()
	}
	if err := ctx.Err(); err != nil {
		cleanup()
		return nil, nil, err
	}
	if proof.Err() != nil || p.app.commandProduct.Load() != p || p.projectionResetRevision.Load() != resetRevision {
		cleanup()
		return nil, nil, commandexecution.ErrStale
	}
	return context.WithValue(recoveryCtx, commandProjectionRecoveryProofKey{}, proof), cleanup, nil
}

// Chamado sob projectionMu também pelo worker de expiração. A revisão é
// fixada antes de ler prova/epochs; um reset posterior invalida todo o rebuild.
func (p *commandProductRuntime) recoverCommandProjection(ctx context.Context) error {
	if p == nil {
		return commandexecution.ErrStale
	}
	return p.recoverCommandProjectionAtReset(ctx, false, p.projectionResetRevision.Load())
}

func (p *commandProductRuntime) recoverCommandProjectionAtReset(ctx context.Context, jobClaimProjection bool, resetRevision uint64) error {
	recoveryCtx, cleanup, err := p.withCommandProjectionRecoveryProof(ctx, resetRevision)
	if err != nil {
		return err
	}
	defer cleanup()
	if transition := p.currentCommandClaimTransition(); transition != nil {
		if transition.resetRevision != resetRevision || p.projectionResetRevision.Load() != resetRevision {
			return commandexecution.ErrStale
		}
		if err := p.app.reconcileCommandLifecycleClaimsAtReset(recoveryCtx, transition.restore, transition.workspaceSwitch, p, transition.resetRevision); err != nil {
			return err
		}
	}
	err = p.app.rebuildCommandLifecycleProjectionAtReset(recoveryCtx, false, jobClaimProjection, p, resetRevision)
	if jobClaimProjection && errors.Is(err, commandexecution.ErrJobProjectionBaseChanged) {
		// A mudança da base de configuração não pode usar o caminho que preserva
		// execuções; mantém a revisão pinada e a mesma prova para o rebuild normal.
		return p.app.rebuildCommandLifecycleProjectionAtReset(recoveryCtx, false, false, p, resetRevision)
	}
	return err
}

func commandProjectionNeedsRebuild(err error) bool {
	return errors.Is(err, commandexecution.ErrStale) || errors.Is(err, commandexecution.ErrHostUserNotPublished)
}

// checkPersistedCommandConfiguration catches a durable configuration write
// that did not arrive through the mounted App mutation path. It runs before a
// command is admitted (never from the projection guard/gate); a stale stamp
// gets the ordinary, canceling lifecycle rebuild rather than the claim-only
// preservation path.
func (p *commandProductRuntime) checkPersistedCommandConfiguration(ctx context.Context) error {
	if p == nil || p.app == nil || ctx == nil {
		return commandexecution.ErrStale
	}
	resetRevision := p.projectionResetRevision.Load()
	if p.app.commandProduct.Load() != p || !p.dependenciesMatch(p.app) {
		return commandexecution.ErrStale
	}
	p.persistedConfigMu.RLock()
	store, snapshot, hasSnapshot := p.persistedConfigStore, p.persistedConfigSnapshot, p.hasPersistedSnapshot
	p.persistedConfigMu.RUnlock()
	if !hasSnapshot || store == nil {
		return commandexecution.ErrStale
	}
	if err := store.CheckCurrent(ctx, snapshot); err != nil {
		if !errors.Is(err, commandconfig.ErrStale) {
			return err
		}
		if p.app.commandProduct.Load() != p || !p.dependenciesMatch(p.app) || p.projectionResetRevision.Load() != resetRevision {
			return commandexecution.ErrStale
		}
		p.projectionMu.Lock()
		defer p.projectionMu.Unlock()
		if p.app.commandProduct.Load() != p || !p.dependenciesMatch(p.app) || p.projectionResetRevision.Load() != resetRevision {
			return commandexecution.ErrStale
		}
		p.persistedConfigMu.RLock()
		store, snapshot, hasSnapshot = p.persistedConfigStore, p.persistedConfigSnapshot, p.hasPersistedSnapshot
		p.persistedConfigMu.RUnlock()
		if !hasSnapshot || store == nil {
			return commandexecution.ErrStale
		}
		if err := store.CheckCurrent(ctx, snapshot); err == nil {
			return nil
		} else if !errors.Is(err, commandconfig.ErrStale) {
			return err
		}
		return p.recoverCommandProjectionAtReset(ctx, false, resetRevision)
	}
	return nil
}

func (p *commandProductRuntime) rememberPersistedCommandConfiguration(store *commandconfig.Store, snapshot commandconfig.Snapshot, epoch commandsecurity.EpochSnapshot, resetRevision uint64) {
	if p == nil || store == nil {
		return
	}
	p.persistedConfigMu.Lock()
	defer p.persistedConfigMu.Unlock()
	if p.projectionResetRevision.Load() != resetRevision {
		return
	}
	if p.projectionRecoveryCancel != nil {
		p.projectionRecoveryCancel()
	}
	p.projectionRecoveryContext, p.projectionRecoveryCancel = context.WithCancel(p.app.commandBridgeContext())
	p.persistedConfigStore = store
	p.persistedConfigEpoch = epoch
	p.persistedConfigSnapshot = snapshot
	p.hasPersistedSnapshot = true
}

// Retirada deliberada não é uma falha recuperável de publicação.
func (a *App) invalidateCommandProjectionRecovery() {
	if a == nil {
		return
	}
	p := a.commandProduct.Load()
	p.invalidateCommandProjectionRecovery()
}

func (p *commandProductRuntime) invalidateCommandProjectionRecovery() {
	if p == nil {
		return
	}
	p.persistedConfigMu.Lock()
	defer p.persistedConfigMu.Unlock()
	p.projectionResetRevision.Add(1)
	if p.projectionRecoveryCancel != nil {
		p.projectionRecoveryCancel()
	}
	p.projectionRecoveryContext, p.projectionRecoveryCancel = nil, nil
}
