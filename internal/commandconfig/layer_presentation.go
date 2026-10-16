package commandconfig

import (
	"assistente/internal/commandactivation"
	"assistente/internal/commandbindings"
)

// projectLayerPresentationTargets keeps action targets and their effective
// activation paths in the same immutable configuration snapshot.
func projectLayerPresentationTargets(snapshot Snapshot, activation map[string]layerActivationProjection) map[string]commandbindings.LayerPresentationState {
	layers := make(map[string]Layer, len(snapshot.Layers))
	for _, layer := range snapshot.Layers {
		layers[layerPresentationKey(layer.ID, layer.WorkspaceID)] = layer
	}
	targets := make(map[string]commandbindings.LayerPresentationState)
	for _, rule := range snapshot.ActivationRules {
		if rule.UserID != snapshot.Scope.UserID || rule.LayerRefKind != commandactivation.UserRef || rule.RuleRefKind != commandactivation.UserRef ||
			(rule.Mode != commandactivation.ModeManual && rule.Mode != commandactivation.ModeToggle) {
			continue
		}
		layer, ok := layers[layerPresentationKey(rule.LayerRef, rule.WorkspaceID)]
		if !ok || layer.UserID != snapshot.Scope.UserID {
			continue
		}
		state := commandbindings.LayerPresentationState{
			LayerID: layer.ID, Scope: layerPresentationScope(rule.WorkspaceID), LayerEnabled: layer.Enabled, RuleEnabled: rule.Enabled,
			RuleMode: string(rule.Mode), RuleCondition: rule.Condition, RuleSource: rule.Source,
			ReviewStatus: rule.ReviewStatus,
		}
		projected := activation[layer.ID]
		if projected.active {
			if len(projected.conditions) == 0 {
				state.AlwaysActive = true
			} else {
				for _, condition := range projected.conditions {
					if len(condition) == 0 {
						state.AlwaysActive = true
					} else {
						state.Contextual = true
					}
				}
				if state.AlwaysActive {
					state.Contextual = false
				}
			}
		}
		if _, duplicate := targets[rule.ID]; duplicate {
			delete(targets, rule.ID)
			continue
		}
		targets[rule.ID] = state
	}
	return targets
}

func layerPresentationScope(workspace *string) string {
	if workspace == nil {
		return "global"
	}
	return "workspace"
}

func layerPresentationKey(layerID string, workspace *string) string {
	if workspace == nil {
		return layerID + "\x00global"
	}
	return layerID + "\x00" + *workspace
}
