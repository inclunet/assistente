package commandconfig

import (
	"context"
	"slices"
	"time"

	"assistente/internal/commandactivation"
	"assistente/internal/commandautomation"
	"github.com/google/uuid"
)

// PrepareRuleRegrant monta o mesmo documento de mutação usado pelo serviço
// completo, acrescentando somente o grant produzido pelo Store de automação.
// O grant é um artefato privado do fluxo autenticado; não é aceito como
// autoridade de um MutationIntent recebido da UI.
func (s *Store) PrepareRuleRegrant(ctx context.Context, scope Scope, ruleID string, grant commandautomation.Grant, validate MutationValidator) (*PreparedMutation, error) {
	if s == nil || s.db == nil || ctx == nil || !validScope(scope) || !validID(ruleID) || validate == nil || commandautomation.ValidateGrant(grant) != nil {
		return nil, ErrInvalid
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	before, err := s.Load(ctx, scope)
	if err != nil {
		return nil, err
	}
	index := -1
	for i, rule := range before.ActivationRules {
		if rule.ID != ruleID || !sameWorkspace(rule.WorkspaceID, scope.WorkspaceID) {
			continue
		}
		if index >= 0 {
			return nil, ErrInvalid
		}
		index = i
	}
	if index < 0 {
		return nil, ErrInvalid
	}
	old := before.ActivationRules[index]
	if old.Mode != commandactivation.ModeEvent || old.Enabled || old.UserID != scope.UserID ||
		grant.Owner.UserID != scope.UserID || !sameWorkspace(grant.Owner.WorkspaceID, scope.WorkspaceID) ||
		grant.LayerRef.Kind != string(old.LayerRefKind) || grant.LayerRef.Ref != old.LayerRef ||
		grant.RuleRef.Kind != string(old.RuleRefKind) || grant.RuleRef.Ref != old.RuleRef {
		return nil, ErrStale
	}
	for _, existing := range before.AutomationGrants {
		if existing.ID == grant.ID {
			return nil, ErrStale
		}
		if existing.RevokedAt == nil && existing.Owner.UserID == grant.Owner.UserID &&
			sameWorkspace(existing.Owner.WorkspaceID, grant.Owner.WorkspaceID) &&
			existing.LayerRef == grant.LayerRef && existing.RuleRef == grant.RuleRef &&
			existing.LayerRef.Kind == grant.LayerRef.Kind && existing.RuleRef.Kind == grant.RuleRef.Kind {
			return nil, ErrStale
		}
	}
	maximum := int64(0)
	for _, existing := range before.AutomationGrants {
		if existing.Owner.UserID == grant.Owner.UserID && sameWorkspace(existing.Owner.WorkspaceID, grant.Owner.WorkspaceID) &&
			existing.LayerRef == grant.LayerRef && existing.LayerRef.Kind == grant.LayerRef.Kind &&
			existing.RuleRef == grant.RuleRef && existing.RuleRef.Kind == grant.RuleRef.Kind &&
			existing.AutomationGrantGeneration > maximum {
			maximum = existing.AutomationGrantGeneration
		}
	}
	if maximum == int64(^uint64(0)>>1) || grant.AutomationGrantGeneration != maximum+1 {
		return nil, ErrStale
	}

	after := cloneConfigSnapshot(before)
	updated := cloneActivationRule(old)
	updated.AuthorizationDecisionID = cloneWorkspace(&grant.AuthorizationDecisionID)
	updated.AutomationGrantID = cloneWorkspace(&grant.ID)
	generation := grant.AutomationGrantGeneration
	updated.AutomationGrantGeneration = &generation
	updated.AutomationGrantFingerprint = cloneWorkspace(&grant.AutomationGrantFingerprint)
	updated.Enabled = true
	after.ActivationRules[index] = updated
	after.AutomationGrants = append(after.AutomationGrants, cloneAutomationGrant(grant))
	slices.SortFunc(after.AutomationGrants, func(a, b commandautomation.Grant) int {
		if a.ID < b.ID {
			return -1
		}
		if a.ID > b.ID {
			return 1
		}
		return 0
	})
	projectActivationEffects(before, &after, RuleEnable, time.Now().UTC())
	if err := validateSnapshot(after); err != nil {
		return nil, err
	}
	if err := validate(ctx, cloneConfigSnapshot(after)); err != nil {
		return nil, err
	}
	id, err := uuid.NewV7()
	if err != nil {
		return nil, err
	}
	return &PreparedMutation{store: s, before: cloneConfigSnapshot(before), after: cloneConfigSnapshot(after), diff: MutationDiff{
		MutationID: id.String(), Operation: RuleEnable, Scope: cloneScope(scope), RevocationAt: time.Now().UTC(),
	}}, nil
}
