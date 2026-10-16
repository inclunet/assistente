package commandportability

import (
	"context"
	"errors"
	"slices"

	"assistente/internal/commandconfig"
)

// BatchImportResult mantém o diff privado separado do relatório redigido.
// Report só existe em commit confirmado ou no-op; não prova publicação no host.
type BatchImportResult struct {
	Diffs  []commandconfig.MutationDiff
	Report *ImportReport
}

// ApplyPlanImportBatch aplica um envelope em todos os escopos que o plano
// autoritativo descobrir. O serviço cria uma decisão por escopo e mantém uma
// única transação para o lote; esta camada conserva o plano e separa o merge
// por escopo, sem compartilhar slices mutáveis com o chamador.
func ApplyPlanImportBatch(ctx context.Context, service *commandconfig.CompleteMutationService, token string, layers []LayerExport, options PlanOptions, ownership OwnershipPort, refs ReferencePort) (BatchImportResult, error) {
	if ctx == nil || service == nil || len(layers) == 0 || ownership == nil {
		return BatchImportResult{}, ErrInvalid
	}
	frozenLayers := make([]LayerExport, len(layers))
	for i, layer := range layers {
		frozenLayers[i] = cloneLayerExport(layer)
	}
	frozenOptions := clonePlanOptions(options)
	initial, err := PlanImport(ctx, frozenLayers, frozenOptions, ownership, refs)
	if err != nil {
		return BatchImportResult{}, err
	}
	workspaces, err := importBatchScopes(initial)
	if err != nil {
		return BatchImportResult{}, err
	}

	planned := Plan{}
	plannedReady := false
	provider := func(ctx context.Context, scope commandconfig.Scope, current commandconfig.Snapshot) (commandconfig.ImportedSnapshot, error) {
		if !plannedReady {
			candidate, planErr := PlanImport(ctx, frozenLayers, frozenOptions, ownership, refs)
			if planErr != nil {
				return commandconfig.ImportedSnapshot{}, planErr
			}
			if !equivalentImportPlan(initial, candidate) {
				return commandconfig.ImportedSnapshot{}, commandconfig.ErrStale
			}
			planned = candidate
			plannedReady = true
		}
		candidate := planned
		filtered, ok := importBatchPlanForScope(candidate, scope)
		if !ok {
			return commandconfig.ImportedSnapshot{}, commandconfig.ErrStale
		}
		return buildImportedSnapshot(filtered, current, scope)
	}
	revalidate := func(ctx context.Context, scope commandconfig.Scope) error {
		candidate, err := PlanImport(ctx, frozenLayers, frozenOptions, ownership, refs)
		if err != nil {
			return err
		}
		if !equivalentImportPlan(initial, candidate) {
			return commandconfig.ErrStale
		}
		if _, ok := importBatchPlanForScope(candidate, scope); !ok {
			return commandconfig.ErrStale
		}
		return nil
	}
	diffs, err := service.ImportBatch(ctx, token, workspaces, provider, revalidate)
	if err != nil && !errors.Is(err, ErrNoChanges) {
		// Plano recusado/cancelado/obsoleto não é relatório de importação.
		return BatchImportResult{}, err
	}
	if !plannedReady {
		return BatchImportResult{}, commandconfig.ErrStale
	}
	// O plano que alimentou o writer contém os IDs da cópia efetivamente
	// persistida. Replanejar aqui produziria outros IDs e avisos diferentes.
	report := buildImportReport(planned, errors.Is(err, ErrNoChanges))
	return BatchImportResult{Diffs: diffs, Report: &report}, err
}

func importBatchScopes(plan Plan) ([]*string, error) {
	seen := make(map[PortableScope]struct{}, len(plan.Layers))
	scopes := make([]PortableScope, 0, len(plan.Layers))
	for _, item := range plan.Layers {
		if _, ok := seen[item.TargetScope]; ok {
			continue
		}
		seen[item.TargetScope] = struct{}{}
		scopes = append(scopes, item.TargetScope)
	}
	if len(scopes) == 0 {
		return nil, ErrInvalid
	}
	slices.SortFunc(scopes, func(a, b PortableScope) int {
		if a.Kind != b.Kind {
			if a.Kind == GlobalScope {
				return -1
			}
			return 1
		}
		if a.WorkspaceID < b.WorkspaceID {
			return -1
		}
		if a.WorkspaceID > b.WorkspaceID {
			return 1
		}
		return 0
	})
	result := make([]*string, len(scopes))
	for i, scope := range scopes {
		switch scope.Kind {
		case GlobalScope:
			if scope.WorkspaceID != "" {
				return nil, ErrInvalid
			}
		case WorkspaceScope:
			if scope.WorkspaceID == "" {
				return nil, ErrInvalid
			}
			workspace := scope.WorkspaceID
			result[i] = &workspace
		default:
			return nil, ErrInvalid
		}
	}
	return result, nil
}

func importBatchPlanForScope(plan Plan, scope commandconfig.Scope) (Plan, bool) {
	want := PortableScope{Kind: GlobalScope}
	if scope.WorkspaceID != nil {
		want = PortableScope{Kind: WorkspaceScope, WorkspaceID: *scope.WorkspaceID}
	}
	filtered := Plan{Version: plan.Version, Warnings: slices.Clone(plan.Warnings)}
	for _, item := range plan.Layers {
		if item.TargetScope != want {
			continue
		}
		copy := item
		copy.Layer = cloneLayerExport(item.Layer)
		filtered.Layers = append(filtered.Layers, copy)
	}
	return filtered, len(filtered.Layers) != 0
}
