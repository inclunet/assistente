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
	return s.prepareRule(ctx, owner, rule)
}

// PrepareRule prepara uma concessão usando o ID da regra carregada no banco.
// O chamador não fornece a chave natural: ela é derivada da regra autoritativa.
func (s *Store) PrepareRule(ctx context.Context, owner Owner, ruleID string) (*GrantChange, error) {
	if s == nil || s.db == nil || ctx == nil || !validOwner(owner) || !validUUID7(ruleID) {
		return nil, ErrInvalid
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var row ruleRow
	if err := scopeWhere(s.db.WithContext(ctx).Model(&ruleRow{}), owner).Where("id = ?", ruleID).Take(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	rule, err := ruleFromRow(row)
	if err != nil {
		return nil, err
	}
	return s.prepareRule(ctx, owner, rule)
}

func (s *Store) prepareRule(ctx context.Context, owner Owner, rule Rule) (*GrantChange, error) {
	if s == nil || s.db == nil || ctx == nil || !validOwner(owner) || !validRuleChangeOwner(owner, rule) {
		return nil, ErrInvalid
	}
	if err := ctx.Err(); err != nil {
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

func validRuleChangeOwner(owner Owner, rule Rule) bool {
	return rule.Owner.UserID == owner.UserID && sameScope(rule.Owner.WorkspaceID, owner.WorkspaceID)
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

// DecisionRequest retorna a cópia da decisão que vincula exatamente o
// fingerprint da regra, dos produtores e da geração do grant. O corpo já foi
// removido antes da confirmação e nunca é reintroduzido.
func (c *ConfirmedGrantChange) DecisionRequest() (commanddecision.Request, error) {
	if c == nil || c.store == nil || c.change == nil || c.receipts == nil || c.request.Fingerprint == "" || c.request.MutationID == "" {
		return commanddecision.Request{}, ErrInvalid
	}
	request := c.request
	request.Body = ""
	return request, nil
}

// Grant retorna uma cópia do grant produzido pelo protocolo de confirmação.
func (c *ConfirmedGrantChange) Grant() (Grant, error) {
	if c == nil || c.store == nil || c.change == nil || ValidateGrant(c.grant) != nil {
		return Grant{}, ErrInvalid
	}
	return cloneGrant(c.grant), nil
}

// Rule retorna a regra lida durante a preparação. Ela não inclui a autoridade
// do grant e serve apenas para construir uma prévia; o commit relê a regra.
func (c *ConfirmedGrantChange) Rule() (Rule, error) {
	if c == nil || c.store == nil || !validRuleChange(c.change) {
		return Rule{}, ErrInvalid
	}
	return cloneRule(c.change.rule), nil
}

// Rule retorna uma cópia da regra carregada durante a preparação, antes da
// confirmação. O método é somente para montar uma prévia; o commit relê o
// estado autoritativo dentro do TX.
func (c *GrantChange) Rule() (Rule, error) {
	if c == nil || c.store == nil || !validRuleChange(c) {
		return Rule{}, ErrInvalid
	}
	return cloneRule(c.rule), nil
}

func cloneGrant(grant Grant) Grant {
	grant.Owner = cloneOwner(grant.Owner)
	grant.RevokedAt = cloneTime(grant.RevokedAt)
	grant.RevokedBy = cloneString(grant.RevokedBy)
	grant.RevocationReason = cloneString(grant.RevocationReason)
	return grant
}

// ApplyConfirmedGrantTx insere o grant confirmado depois que o writer comum
// aplicou a regra habilitada no mesmo TX. A regra é comparada novamente por
// identidade semântica e pelos quatro vínculos de concessão; não abre outra
// transação nem consome uma segunda receipt.
func (s *Store) ApplyConfirmedGrantTx(ctx context.Context, tx *gorm.DB, confirmed *ConfirmedGrantChange, epoch commandsecurity.EpochSnapshot) error {
	if s == nil || s.db == nil || s.db.Config == nil || tx == nil || tx == s.db || tx.Config == nil || !isTransactionDB(tx) || ctx == nil || confirmed == nil || confirmed.store != s || confirmed.receipts == nil || epoch != confirmed.epoch || !sameSQLDatabase(s.db, tx) || !validRuleChange(confirmed.change) {
		return ErrInvalid
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if confirmed.request.Fingerprint != confirmed.grant.AutomationGrantFingerprint || confirmed.request.UserID != epoch.UserID || confirmed.request.SubjectType != "config_mutation" {
		return ErrStale
	}
	var status string
	if err := tx.Table("command_decision_receipts").Select("status").Where(
		"decision_id = ? AND subject_id = ? AND user_id = ? AND auth_context_id = ? AND request_fingerprint = ? AND auth_generation = ? AND security_generation = ? AND expires_at = ? AND status = ? AND auth_context_type = ? AND subject_type = ? AND allowed_action_ids = ? AND accepted_action_id = ? AND responded_at IS NOT NULL AND consumed_at IS NOT NULL",
		confirmed.request.DecisionID, confirmed.request.MutationID, confirmed.request.UserID, confirmed.request.SessionID,
		confirmed.request.Fingerprint, confirmed.request.AuthGeneration, confirmed.request.SecurityGeneration, confirmed.request.ExpiresAt.UnixMilli(),
		commanddecision.Consumed, "local_session", confirmed.request.SubjectType, `["apply","deny"]`, commanddecision.ApplyAction).Scan(&status).Error; err != nil {
		return err
	}
	if status != commanddecision.Consumed {
		return ErrStale
	}
	var row ruleRow
	if err := ruleQuery(tx.WithContext(ctx), confirmed.change.owner, confirmed.change.rule.RuleRef).Take(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrStale
		}
		return err
	}
	current, err := ruleFromRow(row)
	if err != nil {
		return err
	}
	if !sameRuleSemantics(current, confirmed.change.rule) || !current.Enabled || !sameRuleGrantMetadata(current, confirmed.grant) {
		return ErrStale
	}
	key := keyFromRule(current)
	maximum, err := maxGeneration(ctx, tx, key)
	if err != nil {
		return err
	}
	if maximum >= mathMaxInt64 || confirmed.grant.AutomationGrantGeneration != maximum+1 {
		return ErrStale
	}
	if _, err := activeGrantTx(ctx, tx, confirmed.change.owner, key); err == nil {
		return ErrStale
	} else if !errors.Is(err, ErrNotFound) {
		return err
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
	return ctx.Err()
}

func isTransactionDB(db *gorm.DB) bool {
	if db == nil {
		return false
	}
	connPool := db.ConnPool
	if db.Statement != nil && db.Statement.ConnPool != nil {
		connPool = db.Statement.ConnPool
	}
	_, ok := connPool.(gorm.TxCommitter)
	return ok
}

func sameRuleSemantics(left, right Rule) bool {
	left, right = cloneRule(left), cloneRule(right)
	left.Enabled, right.Enabled = false, false
	left.AuthorizationDecisionID, right.AuthorizationDecisionID = nil, nil
	left.AutomationGrantID, right.AutomationGrantID = nil, nil
	left.AutomationGrantGeneration, right.AutomationGrantGeneration = nil, nil
	left.AutomationGrantFingerprint, right.AutomationGrantFingerprint = nil, nil
	return reflect.DeepEqual(left, right)
}

func sameRuleGrantMetadata(rule Rule, grant Grant) bool {
	return rule.AuthorizationDecisionID != nil && *rule.AuthorizationDecisionID == grant.AuthorizationDecisionID &&
		rule.AutomationGrantID != nil && *rule.AutomationGrantID == grant.ID &&
		rule.AutomationGrantGeneration != nil && *rule.AutomationGrantGeneration == grant.AutomationGrantGeneration &&
		rule.AutomationGrantFingerprint != nil && *rule.AutomationGrantFingerprint == grant.AutomationGrantFingerprint
}

const mathMaxInt64 = int64(^uint64(0) >> 1)
