package commandconfig

import (
	"reflect"
	"slices"
	"time"

	"assistente/internal/commandactivation"
	"github.com/google/uuid"
)

func prepareRuleMutation(after *Snapshot, scope Scope, intent MutationIntent, now time.Time) error {
	if after == nil || intent.Layer != nil || intent.Binding != nil {
		return ErrInvalid
	}
	index := -1
	for i, rule := range after.ActivationRules {
		if rule.ID == intent.ID && sameWorkspace(rule.WorkspaceID, scope.WorkspaceID) {
			if index >= 0 {
				return ErrInvalid
			}
			index = i
		}
	}
	switch intent.Operation {
	case RuleCreate:
		if intent.Rule == nil || intent.ID != "" || intent.Rule.ID != "" {
			return ErrInvalid
		}
		rule := cloneActivationRule(*intent.Rule)
		if (rule.UserID != "" && rule.UserID != scope.UserID) || (rule.WorkspaceID != nil && !sameWorkspace(rule.WorkspaceID, scope.WorkspaceID)) {
			return ErrInvalid
		}
		id, err := uuid.NewV7()
		if err != nil {
			return err
		}
		rule.ID, rule.UserID = id.String(), scope.UserID
		rule.WorkspaceID = cloneWorkspace(scope.WorkspaceID)
		rule.Source = "user"
		if rule.RuleRefKind == commandactivation.UserRef {
			if rule.RuleRef != "" && rule.RuleRef != rule.ID {
				return ErrInvalid
			}
			rule.RuleRef = rule.ID
		}
		if err := validateRuleMutation(rule, true); err != nil {
			return err
		}
		after.ActivationRules = append(after.ActivationRules, rule)
	case RuleUpdate:
		if index < 0 || intent.Rule == nil {
			return ErrInvalid
		}
		old := after.ActivationRules[index]
		candidate := cloneActivationRule(*intent.Rule)
		if candidate.ID != old.ID || candidate.UserID != old.UserID || !sameWorkspace(candidate.WorkspaceID, old.WorkspaceID) ||
			candidate.LayerRefKind != old.LayerRefKind || candidate.LayerRef != old.LayerRef || candidate.RuleRefKind != old.RuleRefKind || candidate.RuleRef != old.RuleRef || candidate.Enabled != old.Enabled {
			return ErrInvalid
		}
		if candidate.Mode == commandactivation.ModeEvent {
			candidate.Enabled = false
		}
		if hasGrantMetadata(candidate) && !sameGrantMetadata(candidate, old) {
			return ErrInvalid
		}
		clearRuleGrant(&candidate)
		previous := cloneActivationRule(old)
		clearRuleGrant(&previous)
		if err := validateRuleMutation(candidate, false); err != nil {
			return err
		}
		if reflect.DeepEqual(candidate, previous) {
			return ErrInvalid
		}
		after.ActivationRules[index] = candidate
	case RuleEnable:
		if index < 0 || after.ActivationRules[index].Enabled {
			return ErrInvalid
		}
		if after.ActivationRules[index].Mode == commandactivation.ModeEvent {
			return commandactivation.ErrGrantUnavailable
		}
		after.ActivationRules[index].Enabled = true
	case RuleDisable:
		if index < 0 || !after.ActivationRules[index].Enabled {
			return ErrInvalid
		}
		after.ActivationRules[index].Enabled = false
		clearRuleGrant(&after.ActivationRules[index])
	case RuleDelete:
		if index < 0 {
			return ErrInvalid
		}
		after.ActivationRules = slices.Delete(after.ActivationRules, index, index+1)
	case RuleRestore:
		if index < 0 || after.ActivationRules[index].ReplacesDefaultID == nil {
			return ErrInvalid
		}
		after.ActivationRules = slices.Delete(after.ActivationRules, index, index+1)
	default:
		return ErrInvalid
	}
	_ = now // kept in the helper signature so future timestamps stay derived
	return nil
}

func isRuleOperation(operation Operation) bool {
	switch operation {
	case RuleCreate, RuleUpdate, RuleDelete, RuleEnable, RuleDisable, RuleRestore:
		return true
	default:
		return false
	}
}

func validateRuleMutation(rule commandactivation.Rule, creating bool) error {
	if err := commandactivation.ValidateRule(rule); err != nil {
		return err
	}
	if rule.AuthorizationDecisionID != nil || rule.AutomationGrantID != nil || rule.AutomationGrantGeneration != nil || rule.AutomationGrantFingerprint != nil {
		return ErrInvalid
	}
	if rule.Mode == commandactivation.ModeEvent {
		if rule.Enabled || rule.EventName == nil || *rule.EventName != "command-context.job-run-state.v1" || rule.AllowedInternalProducerTypes == nil || *rule.AllowedInternalProducerTypes != `["jobs.runtime"]` {
			return commandactivation.ErrGrantUnavailable
		}
	} else if rule.EventName != nil || rule.AllowedInternalProducerTypes != nil {
		return ErrInvalid
	}
	if creating && rule.Source != "user" {
		return ErrInvalid
	}
	return nil
}

func clearRuleGrant(rule *commandactivation.Rule) {
	if rule == nil {
		return
	}
	rule.AuthorizationDecisionID = nil
	rule.AutomationGrantID = nil
	rule.AutomationGrantGeneration = nil
	rule.AutomationGrantFingerprint = nil
	if rule.Mode == commandactivation.ModeEvent {
		rule.Enabled = false
	}
}

func hasGrantMetadata(rule commandactivation.Rule) bool {
	return rule.AuthorizationDecisionID != nil || rule.AutomationGrantID != nil || rule.AutomationGrantGeneration != nil || rule.AutomationGrantFingerprint != nil
}

func sameGrantMetadata(left, right commandactivation.Rule) bool {
	return reflect.DeepEqual(left.AuthorizationDecisionID, right.AuthorizationDecisionID) &&
		reflect.DeepEqual(left.AutomationGrantID, right.AutomationGrantID) &&
		reflect.DeepEqual(left.AutomationGrantGeneration, right.AutomationGrantGeneration) &&
		reflect.DeepEqual(left.AutomationGrantFingerprint, right.AutomationGrantFingerprint)
}

func ruleSemanticEqual(left, right commandactivation.Rule) bool {
	a, b := cloneActivationRule(left), cloneActivationRule(right)
	a.Enabled, b.Enabled = false, false
	clearRuleGrant(&a)
	clearRuleGrant(&b)
	return reflect.DeepEqual(a, b)
}

func projectActivationEffects(before Snapshot, after *Snapshot, operation Operation, now time.Time) {
	if after == nil || now.IsZero() {
		return
	}
	beforeRules := make(map[string]commandactivation.Rule, len(before.ActivationRules))
	for _, rule := range before.ActivationRules {
		beforeRules[rule.ID] = rule
	}
	removedLayers := map[string]bool{}
	disabledLayers := map[string]bool{}
	for _, layer := range before.Layers {
		found := false
		for _, candidate := range after.Layers {
			if candidate.ID == layer.ID {
				found = true
				if !candidate.Enabled {
					disabledLayers[layer.ID] = true
				}
				break
			}
		}
		if !found {
			removedLayers[layer.ID] = true
		}
	}
	for i, rule := range after.ActivationRules {
		if rule.LayerRefKind != commandactivation.UserRef || !sameWorkspace(rule.WorkspaceID, after.Scope.WorkspaceID) {
			continue
		}
		if rule.Mode == commandactivation.ModeEvent && (removedLayers[rule.LayerRef] || disabledLayers[rule.LayerRef]) {
			// A complete mutation cannot leave an enabled event rule pointing
			// at a disabled/deleted layer with a revoked grant. Disable and
			// clear the rule in the same projected state the hook will commit.
			rule.Enabled = false
			clearRuleGrant(&rule)
			after.ActivationRules[i] = rule
		}
	}
	for i, grant := range after.AutomationGrants {
		if grant.RevokedAt != nil {
			continue
		}
		// A leitura local inclui o histórico global, mas a mutação só pode
		// projetar revogação no owner/workspace exato do diff.
		if !sameWorkspace(grant.Owner.WorkspaceID, after.Scope.WorkspaceID) {
			continue
		}
		var rule commandactivation.Rule
		exists := false
		for _, candidate := range after.ActivationRules {
			if string(candidate.RuleRefKind) == grant.RuleRef.Kind && candidate.RuleRef == grant.RuleRef.Ref && string(candidate.LayerRefKind) == grant.LayerRef.Kind && candidate.LayerRef == grant.LayerRef.Ref {
				rule, exists = candidate, true
				break
			}
		}
		shouldRevoke := !exists || !rule.Enabled || !ruleSemanticEqual(beforeRules[rule.ID], rule)
		if grant.LayerRef.Kind == "user" && (removedLayers[grant.LayerRef.Ref] || disabledLayers[grant.LayerRef.Ref]) {
			shouldRevoke = true
		}
		if shouldRevoke {
			grant.RevokedAt = cloneTime(&now)
			by, reason := after.Scope.UserID, "rule_disabled"
			if grant.LayerRef.Kind == "user" && removedLayers[grant.LayerRef.Ref] {
				reason = "layer_deleted"
			} else if grant.LayerRef.Kind == "user" && disabledLayers[grant.LayerRef.Ref] {
				reason = "layer_disabled"
			} else if !exists {
				reason = "rule_deleted"
			}
			grant.RevokedBy, grant.RevocationReason = &by, &reason
			after.AutomationGrants[i] = grant
		}
	}
	for i, claim := range after.ActivationClaims {
		if claim.State != commandactivation.StateActive {
			continue
		}
		// A leitura local inclui claims globais herdadas, mas uma mutação
		// local nunca pode projetar terminalidade nesse outro escopo.
		if !sameWorkspace(claim.WorkspaceID, after.Scope.WorkspaceID) {
			continue
		}
		rule, exists := ruleForClaim(after.ActivationRules, claim)
		layerDeleted := claim.LayerRefKind == commandactivation.UserRef && removedLayers[claim.LayerRef]
		layerDisabled := claim.LayerRefKind == commandactivation.UserRef && disabledLayers[claim.LayerRef]
		claimInvalidatedByRule := !exists || !rule.Enabled
		// LayerDisable itself preserves claims. In the complete pipeline the
		// exact rule was disabled above, so its rule invalidation is the
		// deliberate terminal transition; an orphan inherited claim remains
		// untouched rather than being attributed to this local mutation.
		if layerDisabled && !exists {
			claimInvalidatedByRule = false
		}
		if layerDeleted || claimInvalidatedByRule {
			state, reason := commandactivation.StateStale, "rule_disabled"
			if layerDeleted {
				state, reason = commandactivation.StateInactive, "layer_deleted"
			} else if !exists {
				state, reason = commandactivation.StateInactive, "rule_deleted"
			}
			claim.State = state
			claim.TerminalReason = cloneWorkspace(&reason)
			claim.UpdatedAt = now
			after.ActivationClaims[i] = claim
		}
	}
	_ = operation
}

func ruleForClaim(rows []commandactivation.Rule, claim commandactivation.Claim) (commandactivation.Rule, bool) {
	for _, rule := range rows {
		if rule.RuleRefKind == claim.RuleRefKind && rule.RuleRef == claim.RuleRef && rule.LayerRefKind == claim.LayerRefKind && rule.LayerRef == claim.LayerRef && sameWorkspace(rule.WorkspaceID, claim.WorkspaceID) {
			return rule, true
		}
	}
	return commandactivation.Rule{}, false
}
