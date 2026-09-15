package commandconfig

import (
	"assistente/internal/commandactivation"
	"assistente/internal/commandautomation"
)

// validateCompleteActivationAggregate fecha a lacuna entre ProjectComplete
// (que deliberadamente projeta apenas bindings) e o callback completo de
// command_config. Regras, layers e grants são todos verificados; nenhum grant
// é criado ou restaurado por esta validação.
func validateCompleteActivationAggregate(snapshot Snapshot, options CompleteProjection) error {
	layers := make(map[string]Layer, len(snapshot.Layers))
	for _, layer := range snapshot.Layers {
		if layer.UserID != snapshot.Scope.UserID || !inScope(layer.WorkspaceID, snapshot.Scope) {
			return ErrInvalid
		}
		layers[layer.ID] = layer
	}
	builtinLayers := make(map[string]bool, len(options.BuiltinLayers))
	for _, layer := range options.BuiltinLayers {
		builtinLayers[layer.ID] = true
	}
	grants := make(map[string]commandautomation.Grant, len(snapshot.AutomationGrants))
	for _, grant := range snapshot.AutomationGrants {
		if err := commandautomation.ValidateGrant(grant); err != nil || grant.Owner.UserID != snapshot.Scope.UserID || !inScope(grant.Owner.WorkspaceID, snapshot.Scope) {
			return ErrInvalid
		}
		grants[grant.ID] = grant
	}
	for _, rule := range snapshot.ActivationRules {
		if err := commandactivation.ValidateRule(rule); err != nil || rule.UserID != snapshot.Scope.UserID || !inScope(rule.WorkspaceID, snapshot.Scope) {
			return ErrInvalid
		}
		switch rule.LayerRefKind {
		case commandactivation.UserRef:
			layer, ok := layers[rule.LayerRef]
			if !ok || !sameWorkspace(layer.WorkspaceID, rule.WorkspaceID) {
				return ErrInvalid
			}
		case commandactivation.BuiltinRef:
			if !builtinLayers[rule.LayerRef] {
				return ErrInvalid
			}
		default:
			return ErrInvalid
		}
		if rule.Mode == commandactivation.ModeEvent {
			if rule.EventName == nil || *rule.EventName != commandautomation.JobRunStateEvent || rule.AllowedInternalProducerTypes == nil || *rule.AllowedInternalProducerTypes != `["jobs.runtime"]` {
				return ErrInvalid
			}
			if !rule.Enabled {
				if rule.AuthorizationDecisionID != nil || rule.AutomationGrantID != nil || rule.AutomationGrantGeneration != nil || rule.AutomationGrantFingerprint != nil {
					return ErrInvalid
				}
				continue
			}
			if rule.AuthorizationDecisionID == nil || rule.AutomationGrantID == nil || rule.AutomationGrantGeneration == nil || rule.AutomationGrantFingerprint == nil {
				return ErrInvalid
			}
			grant, ok := grants[*rule.AutomationGrantID]
			if !ok || grant.RevokedAt != nil || grant.Owner.UserID != rule.UserID || !sameWorkspace(grant.Owner.WorkspaceID, rule.WorkspaceID) || grant.RuleRef.Kind != string(rule.RuleRefKind) || grant.RuleRef.Ref != rule.RuleRef || grant.LayerRef.Kind != string(rule.LayerRefKind) || grant.LayerRef.Ref != rule.LayerRef || grant.AutomationGrantGeneration != *rule.AutomationGrantGeneration || grant.AutomationGrantFingerprint != *rule.AutomationGrantFingerprint || grant.AuthorizationDecisionID != *rule.AuthorizationDecisionID {
				return ErrInvalid
			}
		} else if rule.EventName != nil || rule.AllowedInternalProducerTypes != nil || rule.AuthorizationDecisionID != nil || rule.AutomationGrantID != nil || rule.AutomationGrantGeneration != nil || rule.AutomationGrantFingerprint != nil {
			return ErrInvalid
		}
	}
	for _, grant := range snapshot.AutomationGrants {
		if grant.RevokedAt != nil {
			continue
		}
		matched := false
		for _, rule := range snapshot.ActivationRules {
			if rule.Enabled && rule.Mode == commandactivation.ModeEvent && rule.UserID == grant.Owner.UserID && sameWorkspace(rule.WorkspaceID, grant.Owner.WorkspaceID) && string(rule.RuleRefKind) == grant.RuleRef.Kind && rule.RuleRef == grant.RuleRef.Ref && string(rule.LayerRefKind) == grant.LayerRef.Kind && rule.LayerRef == grant.LayerRef.Ref {
				matched = true
				break
			}
		}
		if !matched {
			return ErrInvalid
		}
	}
	for _, claim := range snapshot.ActivationClaims {
		if err := commandactivation.ValidateClaim(claim); err != nil || claim.UserID != snapshot.Scope.UserID || !inScope(claim.WorkspaceID, snapshot.Scope) {
			return ErrInvalid
		}
	}
	return nil
}
