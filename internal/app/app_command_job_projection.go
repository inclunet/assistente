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
)

// commandJobLayerProjection carrega a prova durável somente na reconstrução.
// O guard publicado com o mapa relê apenas fontes locais em memória. Não é
// uma porta pública e não aceita claims nem identidades fornecidas pela UI.
type commandJobProjection struct {
	layers  []string
	sources map[string][]commandbindings.LayerProvenance
}

func (a *App) commandJobLayerProjection(ctx context.Context, principal auth.LocalSessionPrincipal, scope commandconfig.Scope) (*commandJobProjection, func(context.Context) error, error) {
	mounted := a.commandMaintenance.Load()
	if mounted == nil {
		return nil, func(ctx context.Context) error {
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
				return commandexecution.ErrStale
			}
		}
		return nil
	}
	if err := guard(ctx); err != nil {
		return nil, nil, err
	}
	return &commandJobProjection{layers: mergeCommandLayerIDs(nil, active), sources: sources}, guard, nil
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
	if p == nil || p.app.commandProduct.Load() != p || !p.dependenciesMatch(p.app) {
		return commandexecution.ErrStale
	}
	_, err := p.host.Snapshot(ctx, p.principal)
	if !errors.Is(err, commandexecution.ErrStale) {
		return err
	}
	p.projectionMu.Lock()
	defer p.projectionMu.Unlock()
	_, err = p.host.Snapshot(ctx, p.principal)
	if !errors.Is(err, commandexecution.ErrStale) {
		return err
	}
	if p.app.commandProduct.Load() != p || !p.dependenciesMatch(p.app) {
		return commandexecution.ErrStale
	}
	// Reconciliação contextual não restaura claims manuais nem suspende o
	// mapa previamente publicado; o guard já impede seu uso se estiver velho.
	return p.app.rebuildCommandLifecycleProjection(ctx, false)
}
