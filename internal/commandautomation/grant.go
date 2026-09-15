package commandautomation

import (
	"context"
	"errors"
	"reflect"
	"time"

	"assistente/internal/commanddecision"
	"assistente/internal/commandsecurity"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func (s *Store) Prepare(ctx context.Context, owner Owner, ref RuleRef) (*GrantChange, error) {
	if s == nil || s.db == nil || ctx == nil || !validOwner(owner) || !validRef(ref) {
		return nil, ErrInvalid
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	rule, err := s.LoadRule(ctx, owner, ref)
	if err != nil {
		return nil, err
	}
	if err := validateRule(rule); err != nil || rule.Enabled {
		return nil, ErrInvalid
	}
	key := keyFromRule(rule)
	if key.Owner.UserID != owner.UserID || !sameScope(key.Owner.WorkspaceID, owner.WorkspaceID) {
		return nil, ErrForeignScope
	}
	maximum, err := maxGeneration(ctx, s.db, key)
	if err != nil {
		return nil, err
	}
	if !validateGeneration(maximum) {
		return nil, ErrInvalid
	}
	if _, err := s.LoadActive(ctx, owner, key); err == nil {
		return nil, ErrInvalid
	} else if !errors.Is(err, ErrNotFound) {
		return nil, err
	}
	return &GrantChange{store: s, owner: cloneOwner(owner), rule: cloneRule(rule), maxGeneration: maximum}, nil
}

// PrepareGrant é um nome explícito equivalente à primitiva de preparação.
func (s *Store) PrepareGrant(ctx context.Context, owner Owner, ref RuleRef) (*GrantChange, error) {
	return s.Prepare(ctx, owner, ref)
}

func (s *Store) ConfirmGrant(ctx context.Context, change *GrantChange, epoch commandsecurity.EpochSnapshot,
	receipts *commanddecision.Store, version string, keys FingerprintKeyProvider, expiresAt time.Time,
	render func(Rule) (string, error)) (*ConfirmedGrantChange, error) {
	if s == nil || s.db == nil || ctx == nil || change == nil || change.store != s || receipts == nil || keys == nil || render == nil ||
		!validOwner(change.owner) || change.owner.UserID != epoch.UserID || !validRuleChange(change) || !expiresAt.After(s.now()) {
		return nil, ErrInvalid
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	generation := change.maxGeneration + 1
	if !validateGeneration(generation) || generation == 0 {
		return nil, ErrInvalid
	}
	ruleFingerprint, err := FingerprintRule(ctx, change.rule, version, keys)
	if err != nil {
		return nil, err
	}
	producerFingerprint, err := producerTypesFingerprint(ctx, version, keys)
	if err != nil {
		return nil, err
	}
	grantFingerprint, err := FingerprintGrant(ctx, keyFromRule(change.rule), ruleFingerprint, producerFingerprint, generation, version, keys)
	if err != nil {
		return nil, err
	}
	body, err := render(cloneRule(change.rule))
	if err != nil || body == "" {
		if err != nil {
			return nil, err
		}
		return nil, ErrInvalid
	}
	decisionID, err := uuid.NewV7()
	if err != nil {
		return nil, err
	}
	mutationID, err := uuid.NewV7()
	if err != nil {
		return nil, err
	}
	request := commanddecision.Request{DecisionID: decisionID.String(), MutationID: mutationID.String(), UserID: epoch.UserID,
		SessionID: epoch.SessionID, Fingerprint: grantFingerprint, AuthGeneration: epoch.AuthGeneration,
		SecurityGeneration: epoch.SecurityGeneration, ExpiresAt: time.UnixMilli(expiresAt.UnixMilli()).UTC(),
		Body: body, SubjectType: "config_mutation"}
	status, err := receipts.Decide(ctx, request)
	if err != nil {
		return nil, err
	}
	if status != commanddecision.Accepted {
		return nil, commanddecision.ErrStale
	}
	grantID, err := uuid.NewV7()
	if err != nil {
		return nil, err
	}
	request.Body = ""
	return &ConfirmedGrantChange{store: s, change: change, receipts: receipts, epoch: epoch, request: request,
		grant: Grant{ID: grantID.String(), Owner: change.owner, LayerRef: change.rule.LayerRef, RuleRef: change.rule.RuleRef,
			RuleFingerprint: ruleFingerprint, EventName: JobRunStateEvent, ProducerTypesFingerprint: producerFingerprint,
			AutomationGrantGeneration: generation, AutomationGrantFingerprint: grantFingerprint,
			AuthorizationDecisionID: decisionID.String(), GrantedAt: s.now().UTC(), GrantedBy: epoch.UserID}}, nil
}

func validRuleChange(change *GrantChange) bool {
	return change != nil && validateRule(change.rule) == nil && change.rule.Owner.UserID == change.owner.UserID &&
		sameScope(change.rule.Owner.WorkspaceID, change.owner.WorkspaceID) && validateGeneration(change.maxGeneration)
}

// Confirm é um alias curto para hosts que tratam a operação como uma
// confirmação de configuração.
func (s *Store) Confirm(ctx context.Context, change *GrantChange, epoch commandsecurity.EpochSnapshot,
	receipts *commanddecision.Store, version string, keys FingerprintKeyProvider, expiresAt time.Time,
	render func(Rule) (string, error)) (*ConfirmedGrantChange, error) {
	return s.ConfirmGrant(ctx, change, epoch, receipts, version, keys, expiresAt, render)
}

// CommitConfirmedGrant consome a receipt, insere o grant e atualiza a regra
// dentro da mesma transação. O validator é opcional apenas porque a política
// estrutural fechada (evento/produtor/refs/JSON) sempre é revalidada aqui;
// quando fornecido, ele é chamado dentro da mesma tx antes do CAS.
func (s *Store) CommitConfirmedGrant(ctx context.Context, confirmed *ConfirmedGrantChange, epoch commandsecurity.EpochSnapshot, validators ...PolicyValidator) error {
	if s == nil || s.db == nil || ctx == nil || confirmed == nil || confirmed.store != s || confirmed.change == nil ||
		confirmed.receipts == nil || epoch != confirmed.epoch || !validRuleChange(confirmed.change) {
		return ErrInvalid
	}
	return confirmed.receipts.ConsumeForDatabase(ctx, s.db, confirmed.request, func(tx *gorm.DB) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		var row ruleRow
		if err := ruleQuery(tx, confirmed.change.owner, confirmed.change.rule.RuleRef).Take(&row).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrStale
			}
			return err
		}
		current, err := ruleFromRow(row)
		if err != nil {
			return err
		}
		if err := validateRule(current); err != nil || current.Enabled || !reflect.DeepEqual(current, confirmed.change.rule) {
			return ErrStale
		}
		if len(validators) > 1 {
			return ErrInvalid
		}
		if len(validators) == 1 && validators[0] != nil {
			if err := validators[0](ctx, tx, cloneOwner(confirmed.change.owner), cloneRule(current)); err != nil {
				return err
			}
		}
		key := keyFromRule(current)
		maximum, err := maxGeneration(ctx, tx, key)
		if err != nil {
			return err
		}
		if maximum != confirmed.change.maxGeneration || maximum >= mathMaxInt64 {
			return ErrStale
		}
		if _, err := activeGrantTx(ctx, tx, confirmed.change.owner, key); err == nil {
			return ErrStale
		} else if !errors.Is(err, ErrNotFound) {
			return err
		}
		if confirmed.grant.AutomationGrantGeneration != maximum+1 {
			return ErrStale
		}
		grant := grantRow{ID: confirmed.grant.ID, UserID: key.Owner.UserID, WorkspaceID: cloneString(key.Owner.WorkspaceID),
			LayerRefKind: key.LayerRef.Kind, LayerRef: key.LayerRef.Ref, RuleRefKind: key.RuleRef.Kind, RuleRef: key.RuleRef.Ref,
			RuleFingerprint: confirmed.grant.RuleFingerprint, EventName: JobRunStateEvent,
			ProducerTypesFingerprint: confirmed.grant.ProducerTypesFingerprint, Generation: confirmed.grant.AutomationGrantGeneration,
			GrantFingerprint: confirmed.grant.AutomationGrantFingerprint, AuthorizationDecisionID: confirmed.grant.AuthorizationDecisionID,
			GrantedAt: confirmed.grant.GrantedAt, GrantedBy: confirmed.grant.GrantedBy}
		if err := tx.Create(&grant).Error; err != nil {
			return err
		}
		updates := map[string]any{"authorization_decision_id": confirmed.grant.AuthorizationDecisionID, "automation_grant_id": confirmed.grant.ID,
			"automation_grant_generation": confirmed.grant.AutomationGrantGeneration, "automation_grant_fingerprint": confirmed.grant.AutomationGrantFingerprint, "enabled": true}
		result := ruleQuery(tx, key.Owner, key.RuleRef).Where("id = ? AND enabled = ?", current.ID, false).Updates(updates)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrStale
		}
		return ctx.Err()
	})
}

func (s *Store) Commit(ctx context.Context, confirmed *ConfirmedGrantChange, epoch commandsecurity.EpochSnapshot, validators ...PolicyValidator) error {
	return s.CommitConfirmedGrant(ctx, confirmed, epoch, validators...)
}

const mathMaxInt64 = int64(^uint64(0) >> 1)
