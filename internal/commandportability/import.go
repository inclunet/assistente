package commandportability

import (
	"context"
	"reflect"
	"slices"

	"assistente/internal/commandactivation"
	"assistente/internal/commandconfig"
)

// ApplyPlanImport rederiva um PlanImport imutável e o entrega ao writer
// confirmado de commandconfig. O chamador fornece somente conteúdo congelado
// e portas confiáveis; não fornece Store, Snapshot, owner, grants ou receipt.
// O plano é calculado novamente no gate final para detectar alteração de
// catálogo, credencial, workspace, trigger ou posse durante a decisão.
//
// O writer conserva o estado do destino que não foi tocado. Uma camada user
// tocada substitui seus filhos user; um delta builtin toca somente seus
// próprios IDs. Grants e claims nunca vêm do plano: o writer conserva o
// histórico do destino e o hook de mutação reconcilia revogações.
func ApplyPlanImport(ctx context.Context, service *commandconfig.CompleteMutationService, token string, workspace *string, layers []LayerExport, options PlanOptions, ownership OwnershipPort, refs ReferencePort) (commandconfig.MutationDiff, error) {
	if ctx == nil || service == nil || len(layers) == 0 {
		return commandconfig.MutationDiff{}, ErrInvalid
	}
	frozenLayers := make([]LayerExport, len(layers))
	for i, layer := range layers {
		frozenLayers[i] = cloneLayerExport(layer)
	}
	frozenOptions := clonePlanOptions(options)
	var planned Plan
	var plannedReady bool
	return service.Import(ctx, token, workspace, func(ctx context.Context, scope commandconfig.Scope, current commandconfig.Snapshot) (commandconfig.ImportedSnapshot, error) {
		candidate, err := PlanImport(ctx, frozenLayers, frozenOptions, ownership, refs)
		if err != nil {
			return commandconfig.ImportedSnapshot{}, err
		}
		built, err := buildImportedSnapshot(candidate, current, scope)
		if err != nil {
			return commandconfig.ImportedSnapshot{}, err
		}
		planned = candidate
		plannedReady = true
		return built, nil
	}, func(ctx context.Context, _ commandconfig.Scope) error {
		if !plannedReady {
			return commandconfig.ErrStale
		}
		candidate, err := PlanImport(ctx, frozenLayers, frozenOptions, ownership, refs)
		if err != nil {
			return err
		}
		if !equivalentImportPlan(planned, candidate) {
			return commandconfig.ErrStale
		}
		return nil
	})
}

func clonePlanOptions(options PlanOptions) PlanOptions {
	cloned := options
	cloned.WorkspaceMap = make(map[string]string, len(options.WorkspaceMap))
	for source, destination := range options.WorkspaceMap {
		cloned.WorkspaceMap[source] = destination
	}
	cloned.RenameByLayer = make(map[string]string, len(options.RenameByLayer))
	for layerID, name := range options.RenameByLayer {
		cloned.RenameByLayer[layerID] = name
	}
	return cloned
}

func equivalentImportPlan(left, right Plan) bool {
	if left.Version != right.Version || len(left.Layers) != len(right.Layers) || len(left.Warnings) != len(right.Warnings) {
		return false
	}
	for i := range left.Warnings {
		if left.Warnings[i] != right.Warnings[i] {
			return false
		}
	}
	for i := range left.Layers {
		a, b := left.Layers[i], right.Layers[i]
		if a.TargetScope != b.TargetScope || a.Action != b.Action || a.Enabled != b.Enabled {
			return false
		}
		if a.Action != CopyMode && a.TargetID != b.TargetID {
			return false
		}
		if !reflect.DeepEqual(normalizePlannedLayer(a), normalizePlannedLayer(b)) {
			return false
		}
	}
	return true
}

func normalizePlannedLayer(item PlannedLayer) LayerExport {
	layer := cloneLayerExport(item.Layer)
	if item.Action != CopyMode {
		return layer
	}
	layer.ID = ""
	for i := range layer.Bindings {
		layer.Bindings[i].ID = ""
		if layer.Bindings[i].LayerRefKind == "user" {
			layer.Bindings[i].LayerRef = ""
		}
	}
	for i := range layer.ActivationRules {
		layer.ActivationRules[i].ID = ""
		if layer.ActivationRules[i].LayerRefKind == "user" {
			layer.ActivationRules[i].LayerRef = ""
		}
		if layer.ActivationRules[i].RuleRefKind == "user" {
			layer.ActivationRules[i].RuleRef = ""
		}
	}
	for i := range layer.BuiltinDeltas {
		layer.BuiltinDeltas[i].ID = ""
	}
	for i := range layer.BuiltinRuleDeltas {
		layer.BuiltinRuleDeltas[i].ID = ""
		if layer.BuiltinRuleDeltas[i].RuleRefKind == "user" {
			layer.BuiltinRuleDeltas[i].RuleRef = ""
		}
	}
	return layer
}

func buildImportedSnapshot(plan Plan, before commandconfig.Snapshot, scope commandconfig.Scope) (commandconfig.ImportedSnapshot, error) {
	incoming, err := plan.Snapshot(scope.UserID)
	if err != nil {
		return commandconfig.ImportedSnapshot{}, err
	}
	incoming.Scope = scope

	touchedLayers := make(map[string]PortableScope)
	touchedUserBindings := make(map[string]PortableScope)
	touchedUserRules := make(map[string]PortableScope)
	touchedBuiltinBindings := make(map[string]PortableScope)
	touchedBuiltinRules := make(map[string]PortableScope)
	touchedLayerIDs := make([]string, 0, len(plan.Layers))
	touchedBindingIDs := make([]string, 0)
	touchedRuleIDs := make([]string, 0)
	for _, item := range plan.Layers {
		if item.Layer.DeltaOnly {
			for _, binding := range item.Layer.BuiltinDeltas {
				if item.Action == KeepMode && hasBeforeBinding(before, binding.ID, item.TargetScope) {
					continue
				}
				touchedBuiltinBindings[binding.ID] = item.TargetScope
				touchedBindingIDs = appendUnique(touchedBindingIDs, binding.ID)
			}
			for _, rule := range item.Layer.BuiltinRuleDeltas {
				if item.Action == KeepMode && hasBeforeRule(before, rule.ID, item.TargetScope) {
					continue
				}
				touchedBuiltinRules[rule.ID] = item.TargetScope
				touchedRuleIDs = appendUnique(touchedRuleIDs, rule.ID)
			}
			continue
		}
		keepExisting := item.Action == KeepMode && hasBeforeLayer(before, item.TargetID, item.TargetScope)
		if !keepExisting {
			touchedLayers[item.TargetID] = item.TargetScope
			touchedLayerIDs = appendUnique(touchedLayerIDs, item.TargetID)
		}
		for _, row := range item.Layer.Bindings {
			if keepExisting && hasBeforeBinding(before, row.ID, item.TargetScope) {
				continue
			}
			if keepExisting {
				touchedUserBindings[row.ID] = item.TargetScope
			}
			touchedBindingIDs = appendUnique(touchedBindingIDs, row.ID)
		}
		for _, row := range item.Layer.BuiltinDeltas {
			if keepExisting {
				return commandconfig.ImportedSnapshot{}, ErrInvalid
			}
			touchedBindingIDs = appendUnique(touchedBindingIDs, row.ID)
		}
		for _, row := range item.Layer.ActivationRules {
			if keepExisting && hasBeforeRule(before, row.ID, item.TargetScope) {
				continue
			}
			if keepExisting {
				touchedUserRules[row.ID] = item.TargetScope
			}
			touchedRuleIDs = appendUnique(touchedRuleIDs, row.ID)
		}
		for _, row := range item.Layer.BuiltinRuleDeltas {
			if keepExisting {
				return commandconfig.ImportedSnapshot{}, ErrInvalid
			}
			touchedRuleIDs = appendUnique(touchedRuleIDs, row.ID)
		}
	}
	// Replacing a user layer also replaces its existing children. Mark those
	// old IDs touched so the commandconfig boundary can prove that every
	// unmentioned row was preserved, including event grants on other layers.
	for _, binding := range before.Bindings {
		if binding.LayerRefKind != "user" {
			continue
		}
		if target, ok := touchedLayers[binding.LayerRef]; ok && sameImportWorkspace(binding.WorkspaceID, target) {
			touchedBindingIDs = appendUnique(touchedBindingIDs, binding.ID)
		}
	}
	for _, rule := range before.ActivationRules {
		if rule.LayerRefKind != commandactivation.UserRef {
			continue
		}
		if target, ok := touchedLayers[rule.LayerRef]; ok && sameImportWorkspace(rule.WorkspaceID, target) {
			touchedRuleIDs = appendUnique(touchedRuleIDs, rule.ID)
		}
	}

	after := before
	after.Scope = scope
	after.Layers = slices.DeleteFunc(slices.Clone(before.Layers), func(layer commandconfig.Layer) bool {
		target, ok := touchedLayers[layer.ID]
		return ok && sameImportWorkspace(layer.WorkspaceID, target)
	})
	after.Bindings = slices.DeleteFunc(slices.Clone(before.Bindings), func(binding commandconfig.Binding) bool {
		if binding.LayerRefKind == "user" {
			target, ok := touchedLayers[binding.LayerRef]
			return ok && sameImportWorkspace(binding.WorkspaceID, target)
		}
		if binding.LayerRefKind == "builtin" {
			target, ok := touchedBuiltinBindings[binding.ID]
			return ok && sameImportWorkspace(binding.WorkspaceID, target)
		}
		return false
	})
	after.ActivationRules = slices.DeleteFunc(slices.Clone(before.ActivationRules), func(rule commandactivation.Rule) bool {
		if rule.LayerRefKind == commandactivation.UserRef {
			target, ok := touchedLayers[rule.LayerRef]
			return ok && sameImportWorkspace(rule.WorkspaceID, target)
		}
		if rule.LayerRefKind == commandactivation.BuiltinRef {
			target, ok := touchedBuiltinRules[rule.ID]
			return ok && sameImportWorkspace(rule.WorkspaceID, target)
		}
		return false
	})

	for _, layer := range incoming.Layers {
		if target, ok := touchedLayers[layer.ID]; ok && sameImportWorkspace(layer.WorkspaceID, target) {
			after.Layers = append(after.Layers, layer)
		}
	}
	for _, binding := range incoming.Bindings {
		if binding.LayerRefKind == "user" {
			target, ok := touchedLayers[binding.LayerRef]
			if ok && sameImportWorkspace(binding.WorkspaceID, target) {
				after.Bindings = append(after.Bindings, binding)
				continue
			}
			if target, ok := touchedUserBindings[binding.ID]; ok && sameImportWorkspace(binding.WorkspaceID, target) {
				after.Bindings = append(after.Bindings, binding)
			}
			continue
		}
		if target, ok := touchedBuiltinBindings[binding.ID]; ok && sameImportWorkspace(binding.WorkspaceID, target) {
			after.Bindings = append(after.Bindings, binding)
		}
	}
	for _, rule := range incoming.ActivationRules {
		if rule.LayerRefKind == commandactivation.UserRef {
			target, ok := touchedLayers[rule.LayerRef]
			if ok && sameImportWorkspace(rule.WorkspaceID, target) {
				after.ActivationRules = append(after.ActivationRules, rule)
				continue
			}
			if target, ok := touchedUserRules[rule.ID]; ok && sameImportWorkspace(rule.WorkspaceID, target) {
				after.ActivationRules = append(after.ActivationRules, rule)
			}
			continue
		}
		if target, ok := touchedBuiltinRules[rule.ID]; ok && sameImportWorkspace(rule.WorkspaceID, target) {
			after.ActivationRules = append(after.ActivationRules, rule)
		}
	}
	return commandconfig.ImportedSnapshot{Snapshot: after, TouchedLayerIDs: touchedLayerIDs, TouchedBindingIDs: touchedBindingIDs, TouchedRuleIDs: touchedRuleIDs}, nil
}

func appendUnique(ids []string, id string) []string {
	if id == "" {
		return ids
	}
	for _, existing := range ids {
		if existing == id {
			return ids
		}
	}
	return append(ids, id)
}

func hasBeforeLayer(snapshot commandconfig.Snapshot, id string, scope PortableScope) bool {
	for _, row := range snapshot.Layers {
		if row.ID == id && sameImportWorkspace(row.WorkspaceID, scope) {
			return true
		}
	}
	return false
}

func hasBeforeBinding(snapshot commandconfig.Snapshot, id string, scope PortableScope) bool {
	for _, row := range snapshot.Bindings {
		if row.ID == id && sameImportWorkspace(row.WorkspaceID, scope) {
			return true
		}
	}
	return false
}

func hasBeforeRule(snapshot commandconfig.Snapshot, id string, scope PortableScope) bool {
	for _, row := range snapshot.ActivationRules {
		if row.ID == id && sameImportWorkspace(row.WorkspaceID, scope) {
			return true
		}
	}
	return false
}

func sameImportWorkspace(workspace *string, scope PortableScope) bool {
	if scope.Kind == GlobalScope {
		return workspace == nil
	}
	return workspace != nil && *workspace == scope.WorkspaceID
}
