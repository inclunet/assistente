package commandactivation

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Store struct {
	db              *gorm.DB
	maintenanceSeal *maintenanceSeal
}

// Tx é uma vista da transação fornecida pelo chamador. O tipo impede que a
// integração abra uma segunda transação ao combinar layer enable/disable com
// outras mutações.
type Tx struct {
	db    *gorm.DB
	store *Store
}

func NewStore(db *gorm.DB) (*Store, error) {
	if db == nil {
		return nil, ErrInvalid
	}
	return &Store{db: db, maintenanceSeal: &maintenanceSeal{token: uuid.New()}}, nil
}

func (s *Store) WithTx(ctx context.Context, fn func(*Tx) error) error {
	if s == nil || s.db == nil || ctx == nil || fn == nil {
		return ErrInvalid
	}
	return s.db.WithContext(ctx).Transaction(func(db *gorm.DB) error {
		return fn(&Tx{db: db, store: s})
	})
}

// BindTx adapta uma transação já aberta pelo main. Não verifica nem inicia
// uma transação; o callback deve usar exclusivamente o Tx devolvido.
func (s *Store) BindTx(db *gorm.DB) (*Tx, error) {
	if s == nil || s.db == nil || db == nil || !isTransactionDB(db) {
		return nil, ErrInvalid
	}
	return &Tx{db: db, store: s}, nil
}

func isTransactionDB(db *gorm.DB) bool {
	if db == nil {
		return false
	}
	pool := db.ConnPool
	if db.Statement != nil && db.Statement.ConnPool != nil {
		pool = db.Statement.ConnPool
	}
	_, ok := pool.(gorm.TxCommitter)
	return ok
}

// validateRuleForCreate is stricter than validateRule deliberately. Rows read
// from the database may carry the nullable grant references produced by the
// commandautomation commit protocol, but a caller of CreateRule cannot supply
// those references as authority. Event rules are also created disabled and
// only with the closed event/producer contract; CommitConfirmedGrant is the
// sole path that enables them and fills grant references.
func validateRuleForCreate(rule Rule) error {
	if err := validateRule(rule); err != nil {
		return err
	}
	if rule.AuthorizationDecisionID != nil || rule.AutomationGrantID != nil || rule.AutomationGrantGeneration != nil || rule.AutomationGrantFingerprint != nil {
		return ErrInvalid
	}
	if rule.Mode == ModeEvent {
		if rule.Enabled || rule.EventName == nil || *rule.EventName != "command-context.job-run-state.v1" || rule.AllowedInternalProducerTypes == nil || *rule.AllowedInternalProducerTypes != `["jobs.runtime"]` {
			return ErrInvalid
		}
		return nil
	}
	if rule.EventName != nil || rule.AllowedInternalProducerTypes != nil {
		return ErrInvalid
	}
	return nil
}

func (s *Store) CreateRule(ctx context.Context, rule Rule) error {
	if s == nil || s.db == nil || ctx == nil || validateRuleForCreate(rule) != nil {
		return ErrInvalid
	}
	return s.db.WithContext(ctx).Create(&rule).Error
}

func (s *Store) ResolveRule(ctx context.Context, owner Owner, ref Ref) (Rule, error) {
	if s == nil || s.db == nil || ctx == nil {
		return Rule{}, ErrInvalid
	}
	if err := validateOwner(owner); err != nil {
		return Rule{}, ErrInvalid
	}
	if err := validateRef(ref); err != nil {
		return Rule{}, ErrInvalid
	}
	var rows []Rule
	query := s.db.WithContext(ctx).Where("user_id = ? AND rule_ref_kind = ? AND rule_ref = ?", owner.UserID, ref.Kind, ref.ID)
	query = scopedQuery(query, owner.WorkspaceID)
	if err := query.Order("CASE WHEN workspace_id IS NULL THEN 0 ELSE 1 END, id").Find(&rows).Error; err != nil {
		return Rule{}, err
	}
	for _, row := range rows {
		if err := validateRule(row); err != nil {
			return Rule{}, err
		}
		if sameWorkspace(row.WorkspaceID, owner.WorkspaceID) {
			return row, nil
		}
	}
	return Rule{}, ErrNotFound
}

func (s *Store) ListRules(ctx context.Context, owner Owner) ([]Rule, error) {
	if s == nil || s.db == nil || ctx == nil {
		return nil, ErrInvalid
	}
	if err := validateOwner(owner); err != nil {
		return nil, ErrInvalid
	}
	var rows []Rule
	if err := scopedQuery(s.db.WithContext(ctx).Where("user_id = ?", owner.UserID), owner.WorkspaceID).
		Order("workspace_id, id").Find(&rows).Error; err != nil {
		return nil, err
	}
	for _, row := range rows {
		if err := validateRule(row); err != nil {
			return nil, err
		}
	}
	return rows, nil
}

func (s *Store) ListClaims(ctx context.Context, owner Owner) ([]Claim, error) {
	if s == nil || s.db == nil || ctx == nil {
		return nil, ErrInvalid
	}
	if err := validateOwner(owner); err != nil {
		return nil, ErrInvalid
	}
	return listClaims(s.db.WithContext(ctx), owner)
}

func (s *Store) SnapshotGeneration(ctx context.Context, owner Owner) (GenerationSnapshot, error) {
	if s == nil || s.db == nil || ctx == nil || validateOwner(owner) != nil {
		return GenerationSnapshot{}, ErrInvalid
	}
	return snapshotGeneration(ctx, s.db, owner)
}

// BumpActiveLayersTx altera o contador somente no TX recebido. A geração
// lógica começa em 1 mesmo antes de existir linha física; a primeira mudança
// cria a linha em 2. Isso mantém leitura sem efeitos colaterais.
func (s *Store) BumpActiveLayersTx(ctx context.Context, db *gorm.DB, owner Owner) (GenerationSnapshot, error) {
	if s == nil || db == nil || ctx == nil || validateOwner(owner) != nil {
		return GenerationSnapshot{}, ErrInvalid
	}
	if err := ctx.Err(); err != nil {
		return GenerationSnapshot{}, err
	}
	var row GenerationSnapshot
	q := db.WithContext(ctx).Table("command_layer_activation_generations").Where("user_id = ?", owner.UserID)
	if owner.WorkspaceID == nil {
		q = q.Where("workspace_id IS NULL")
	} else {
		q = q.Where("workspace_id = ?", *owner.WorkspaceID)
	}
	err := q.First(&row).Error
	now := time.Now().UTC()
	if errors.Is(err, gorm.ErrRecordNotFound) {
		id, idErr := uuid.NewV7()
		if idErr != nil {
			return GenerationSnapshot{}, idErr
		}
		row = GenerationSnapshot{ID: id.String(), UserID: owner.UserID, WorkspaceID: cloneString(owner.WorkspaceID), Generation: 2, UpdatedAt: now}
		if err := db.WithContext(ctx).Table("command_layer_activation_generations").Create(&row).Error; err != nil {
			return GenerationSnapshot{}, err
		}
		return row, nil
	}
	if err != nil {
		return GenerationSnapshot{}, err
	}
	row.Generation++
	row.UpdatedAt = now
	updates := db.WithContext(ctx).Table("command_layer_activation_generations").Where("id = ? AND generation = ?", row.ID, row.Generation-1).Updates(map[string]any{"generation": row.Generation, "updated_at": row.UpdatedAt})
	if updates.Error != nil {
		return GenerationSnapshot{}, updates.Error
	}
	if updates.RowsAffected != 1 {
		return GenerationSnapshot{}, ErrStale
	}
	return row, nil
}

func snapshotGeneration(ctx context.Context, db *gorm.DB, owner Owner) (GenerationSnapshot, error) {
	var row GenerationSnapshot
	q := db.WithContext(ctx).Table("command_layer_activation_generations").Where("user_id = ?", owner.UserID)
	if owner.WorkspaceID == nil {
		q = q.Where("workspace_id IS NULL")
	} else {
		q = q.Where("workspace_id = ?", *owner.WorkspaceID)
	}
	err := q.First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return GenerationSnapshot{UserID: owner.UserID, WorkspaceID: cloneString(owner.WorkspaceID), Generation: 1}, nil
	}
	if err != nil {
		return GenerationSnapshot{}, err
	}
	return row, nil
}

func (s *Store) GetClaim(ctx context.Context, owner Owner, activationID string) (Claim, error) {
	if s == nil || s.db == nil || ctx == nil || !validUUID7(activationID) {
		return Claim{}, ErrNotFound
	}
	if err := validateOwner(owner); err != nil {
		return Claim{}, ErrNotFound
	}
	var row Claim
	err := s.db.WithContext(ctx).Where("activation_id = ? AND user_id = ?", activationID, owner.UserID).
		Where(scopePredicate(owner.WorkspaceID), scopeArg(owner.WorkspaceID)...).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return Claim{}, ErrNotFound
	}
	if err != nil {
		return Claim{}, err
	}
	if err := validateClaim(row); err != nil {
		return Claim{}, err
	}
	return row, nil
}

func (t *Tx) InsertClaim(ctx context.Context, claim Claim) error {
	if t == nil || t.db == nil || ctx == nil || !validClaim(claim) {
		return ErrInvalid
	}
	return t.db.WithContext(ctx).Create(&claim).Error
}

func (t *Tx) ListClaims(ctx context.Context, owner Owner) ([]Claim, error) {
	if t == nil || t.db == nil || ctx == nil {
		return nil, ErrInvalid
	}
	if err := validateOwner(owner); err != nil {
		return nil, ErrInvalid
	}
	return listClaims(t.db.WithContext(ctx), owner)
}

func (t *Tx) ResolveRule(ctx context.Context, owner Owner, ref Ref) (Rule, error) {
	if t == nil || t.db == nil || ctx == nil {
		return Rule{}, ErrInvalid
	}
	if err := validateOwner(owner); err != nil {
		return Rule{}, ErrInvalid
	}
	if err := validateRef(ref); err != nil {
		return Rule{}, ErrInvalid
	}
	var rows []Rule
	q := t.db.WithContext(ctx).Where("user_id = ? AND rule_ref_kind = ? AND rule_ref = ?", owner.UserID, ref.Kind, ref.ID)
	q = scopedQuery(q, owner.WorkspaceID)
	if err := q.Order("CASE WHEN workspace_id IS NULL THEN 0 ELSE 1 END, id").Find(&rows).Error; err != nil {
		return Rule{}, err
	}
	for _, row := range rows {
		if err := validateRule(row); err != nil {
			return Rule{}, err
		}
		if sameWorkspace(row.WorkspaceID, owner.WorkspaceID) {
			return row, nil
		}
	}
	return Rule{}, ErrNotFound
}

func (t *Tx) ListRules(ctx context.Context, owner Owner) ([]Rule, error) {
	if t == nil || t.db == nil || ctx == nil || validateOwner(owner) != nil {
		return nil, ErrInvalid
	}
	return listRules(t.db.WithContext(ctx), owner)
}

func (t *Tx) ActiveManualClaims(ctx context.Context, owner Owner, layer, rule Ref, stackKey string) ([]Claim, error) {
	if t == nil || t.db == nil || ctx == nil || strings.TrimSpace(stackKey) == "" {
		return nil, ErrInvalid
	}
	if err := validateOwner(owner); err != nil {
		return nil, ErrInvalid
	}
	if err := validateRef(layer); err != nil {
		return nil, ErrInvalid
	}
	if err := validateRef(rule); err != nil {
		return nil, ErrInvalid
	}
	var rows []Claim
	q := t.db.WithContext(ctx).Where("user_id = ? AND layer_ref_kind = ? AND layer_ref = ? AND rule_ref_kind = ? AND rule_ref = ? AND source_type = ? AND manual_stack_key = ? AND state = ?", owner.UserID, layer.Kind, layer.ID, rule.Kind, rule.ID, "manual", stackKey, StateActive)
	q = q.Where(scopePredicate(owner.WorkspaceID), scopeArg(owner.WorkspaceID)...)
	if err := q.Order("activated_at DESC, activation_id DESC").Find(&rows).Error; err != nil {
		return nil, err
	}
	for _, row := range rows {
		if err := validateClaim(row); err != nil {
			return nil, err
		}
	}
	return rows, nil
}

func (t *Tx) LatestManualClaim(ctx context.Context, owner Owner, layer, rule Ref, stackKey string) (Claim, error) {
	rows, err := t.ActiveManualClaims(ctx, owner, layer, rule, stackKey)
	if err != nil {
		return Claim{}, err
	}
	if len(rows) == 0 {
		return Claim{}, ErrNotFound
	}
	return rows[0], nil
}

func (t *Tx) latestManualClaimAtScope(ctx context.Context, owner Owner, layer, rule Ref, stackKey string, workspace *string) (Claim, error) {
	if t == nil || t.db == nil || ctx == nil || strings.TrimSpace(stackKey) == "" || validateOwner(owner) != nil || validateRef(layer) != nil || validateRef(rule) != nil {
		return Claim{}, ErrInvalid
	}
	var row Claim
	q := t.db.WithContext(ctx).Where("user_id = ? AND layer_ref_kind = ? AND layer_ref = ? AND rule_ref_kind = ? AND rule_ref = ? AND source_type = ? AND manual_stack_key = ? AND state = ?", owner.UserID, layer.Kind, layer.ID, rule.Kind, rule.ID, "manual", stackKey, StateActive)
	if workspace == nil {
		q = q.Where("workspace_id IS NULL")
	} else {
		q = q.Where("workspace_id = ?", *workspace)
	}
	err := q.Order("activated_at DESC, activation_id DESC").First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return Claim{}, ErrNotFound
	}
	if err != nil {
		return Claim{}, err
	}
	if err := validateClaim(row); err != nil {
		return Claim{}, err
	}
	return row, nil
}

func (t *Tx) ActiveContextClaim(ctx context.Context, owner Owner, layer, rule Ref) (Claim, error) {
	if t == nil || t.db == nil || ctx == nil {
		return Claim{}, ErrInvalid
	}
	if err := validateOwner(owner); err != nil {
		return Claim{}, ErrInvalid
	}
	if err := validateRef(layer); err != nil {
		return Claim{}, ErrInvalid
	}
	if err := validateRef(rule); err != nil {
		return Claim{}, ErrInvalid
	}
	var row Claim
	q := t.db.WithContext(ctx).Where("user_id = ? AND layer_ref_kind = ? AND layer_ref = ? AND rule_ref_kind = ? AND rule_ref = ? AND source_type = ? AND state = ?", owner.UserID, layer.Kind, layer.ID, rule.Kind, rule.ID, "context", StateActive)
	q = q.Where(scopePredicate(owner.WorkspaceID), scopeArg(owner.WorkspaceID)...).Order("activated_at DESC, activation_id DESC")
	err := q.First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return Claim{}, ErrNotFound
	}
	if err != nil {
		return Claim{}, err
	}
	if err := validateClaim(row); err != nil {
		return Claim{}, err
	}
	return row, nil
}

func (t *Tx) DeactivateClaim(ctx context.Context, owner Owner, activationID, reason string) (bool, error) {
	if t == nil || t.db == nil || ctx == nil || !validUUID7(activationID) || !validText(reason) {
		return false, ErrInvalid
	}
	if err := validateOwner(owner); err != nil {
		return false, ErrInvalid
	}
	result := t.db.WithContext(ctx).Model(&Claim{}).
		Where("activation_id = ? AND user_id = ? AND state = ?", activationID, owner.UserID, StateActive).
		Where(scopePredicate(owner.WorkspaceID), scopeArg(owner.WorkspaceID)...).
		Updates(map[string]any{"state": StateDeactivated, "terminal_reason": reason, "updated_at": time.Now().UTC()})
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected == 1, nil
}

func (t *Tx) deactivateClaimAt(ctx context.Context, owner Owner, activationID, reason string, now time.Time) (bool, error) {
	if now.IsZero() || !validText(reason) {
		return false, ErrInvalid
	}
	result := t.db.WithContext(ctx).Model(&Claim{}).
		Where("activation_id = ? AND user_id = ? AND state = ?", activationID, owner.UserID, StateActive).
		Where(scopePredicate(owner.WorkspaceID), scopeArg(owner.WorkspaceID)...).
		Updates(map[string]any{"state": StateDeactivated, "terminal_reason": reason, "updated_at": now.UTC()})
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected == 1, nil
}

func (t *Tx) expireClaimAt(ctx context.Context, owner Owner, activationID string, now time.Time) (bool, error) {
	if now.IsZero() {
		return false, ErrInvalid
	}
	result := t.db.WithContext(ctx).Model(&Claim{}).
		Where("activation_id = ? AND user_id = ? AND state = ? AND expires_at IS NOT NULL AND expires_at <= ?", activationID, owner.UserID, StateActive, now.UTC()).
		Where(scopePredicate(owner.WorkspaceID), scopeArg(owner.WorkspaceID)...).
		Updates(map[string]any{"state": StateExpired, "terminal_reason": "expiry", "updated_at": now.UTC()})
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected == 1, nil
}

func (t *Tx) updateClaim(ctx context.Context, owner Owner, activationID string, values map[string]any) error {
	if t == nil || t.db == nil || ctx == nil || !validUUID7(activationID) || len(values) == 0 {
		return ErrInvalid
	}
	if err := validateOwner(owner); err != nil {
		return ErrInvalid
	}
	result := t.db.WithContext(ctx).Model(&Claim{}).Where("activation_id = ? AND user_id = ?", activationID, owner.UserID).
		Where(scopePredicate(owner.WorkspaceID), scopeArg(owner.WorkspaceID)...).Updates(values)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return ErrStale
	}
	return nil
}

// ExpireDue marca claims temporárias vencidas por CAS. Claims terminais nunca
// voltam a active, inclusive se o scheduler for executado novamente.
func (t *Tx) ExpireDue(ctx context.Context, owner Owner, now time.Time) (int, error) {
	if t == nil || t.db == nil || ctx == nil || now.IsZero() {
		return 0, ErrInvalid
	}
	if err := validateOwner(owner); err != nil {
		return 0, ErrInvalid
	}
	result := t.db.WithContext(ctx).Model(&Claim{}).
		Where("user_id = ? AND state = ? AND expires_at IS NOT NULL AND expires_at <= ?", owner.UserID, StateActive, now.UTC()).
		Where(scopePredicate(owner.WorkspaceID), scopeArg(owner.WorkspaceID)...).
		Updates(map[string]any{"state": StateExpired, "terminal_reason": "expiry", "updated_at": now.UTC()})
	if result.Error != nil {
		return 0, result.Error
	}
	return int(result.RowsAffected), nil
}

// ReconcileLayer é a ponte transacional para layer_disable/layer_enable. A
// camada é dona de commandconfig; aqui claims são apenas relidas. Disable não
// encerra claims persistidas, e Enable não as reativa: o resolvedor deve usar
// Layer.Enabled e este resultado para publicar a geração efetiva.
func (t *Tx) ReconcileLayer(ctx context.Context, owner Owner, layer Ref, enabled bool) (Mutation, error) {
	if t == nil || t.db == nil || ctx == nil {
		return Mutation{}, ErrInvalid
	}
	if err := validateOwner(owner); err != nil {
		return Mutation{}, ErrInvalid
	}
	if err := validateRef(layer); err != nil {
		return Mutation{}, ErrInvalid
	}
	var count int64
	q := t.db.WithContext(ctx).Model(&Claim{}).Where("user_id = ? AND layer_ref_kind = ? AND layer_ref = ? AND state = ?", owner.UserID, layer.Kind, layer.ID, StateActive)
	q = q.Where(scopePredicate(owner.WorkspaceID), scopeArg(owner.WorkspaceID)...)
	if err := q.Count(&count).Error; err != nil {
		return Mutation{}, err
	}
	return Mutation{Changed: count > 0 && !enabled, EffectiveClaims: func() int {
		if enabled {
			return int(count)
		}
		return 0
	}(), ActiveLayersChanged: count > 0}, nil
}

func listClaims(db *gorm.DB, owner Owner) ([]Claim, error) {
	var rows []Claim
	q := db.Where("user_id = ?", owner.UserID)
	q = q.Where(scopePredicate(owner.WorkspaceID), scopeArg(owner.WorkspaceID)...)
	if err := q.Order("activated_at, activation_id").Find(&rows).Error; err != nil {
		return nil, err
	}
	for _, row := range rows {
		if err := validateClaim(row); err != nil {
			return nil, err
		}
	}
	return rows, nil
}

func listRules(db *gorm.DB, owner Owner) ([]Rule, error) {
	var rows []Rule
	if err := scopedQuery(db.Where("user_id = ?", owner.UserID), owner.WorkspaceID).
		Order("workspace_id, id").Find(&rows).Error; err != nil {
		return nil, err
	}
	for _, row := range rows {
		if err := validateRule(row); err != nil {
			return nil, err
		}
	}
	return rows, nil
}

func scopedQuery(query *gorm.DB, workspace *string) *gorm.DB {
	return query.Where(scopePredicate(workspace), scopeArg(workspace)...)
}

func scopePredicate(workspace *string) string {
	if workspace == nil {
		return "workspace_id IS NULL"
	}
	return "(workspace_id IS NULL OR workspace_id = ?)"
}

func scopeArg(workspace *string) []any {
	if workspace == nil {
		return nil
	}
	return []any{*workspace}
}

func validClaim(claim Claim) bool { return validateClaim(claim) == nil }

func validateRule(rule Rule) error {
	if !validUUID7(rule.ID) || !validUUID7(rule.UserID) || !validRef(rule.LayerRefKind, rule.LayerRef) || !validRef(rule.RuleRefKind, rule.RuleRef) || rule.RuleRefKind == UserRef && rule.RuleRef != rule.ID {
		return ErrInvalid
	}
	if rule.WorkspaceID != nil && !validOpaque(*rule.WorkspaceID) {
		return ErrInvalid
	}
	if rule.LayerRefKind == UserRef && !validUUID7(rule.LayerRef) {
		return ErrInvalid
	}
	if rule.Mode != ModeAlways && rule.Mode != ModeContext && rule.Mode != ModeCondition && rule.Mode != ModeManual && rule.Mode != ModeToggle && rule.Mode != ModeEvent {
		return ErrInvalid
	}
	if rule.Lifecycle != LifecyclePersistent && rule.Lifecycle != LifecycleSession && rule.Lifecycle != LifecycleTemporary {
		return ErrInvalid
	}
	if !objectJSON(rule.Condition) || !validText(rule.Source) || (rule.ReviewStatus != "active" && rule.ReviewStatus != "needs_review") {
		return ErrInvalid
	}
	for _, value := range []*string{rule.EventName, rule.AllowedInternalProducerTypes, rule.AuthorizationDecisionID, rule.AutomationGrantID, rule.AutomationGrantFingerprint} {
		if value != nil && !validText(*value) {
			return ErrInvalid
		}
	}
	if rule.AutomationGrantGeneration != nil && *rule.AutomationGrantGeneration < 0 {
		return ErrInvalid
	}
	if rule.ReplacesDefaultID == nil && rule.ReplacesDefaultVersion == nil && rule.ReplacesDefaultFingerprint == nil {
		return nil
	}
	if rule.ReplacesDefaultID == nil || rule.ReplacesDefaultVersion == nil || rule.ReplacesDefaultFingerprint == nil || !validText(*rule.ReplacesDefaultID) || !validText(*rule.ReplacesDefaultVersion) || !validText(*rule.ReplacesDefaultFingerprint) {
		return ErrInvalid
	}
	return nil
}

func validateClaim(claim Claim) error {
	if !validUUID7(claim.ActivationID) || !validUUID7(claim.UserID) || !validRef(claim.LayerRefKind, claim.LayerRef) || !validRef(claim.RuleRefKind, claim.RuleRef) || !validText(claim.AuthContextType) || !validText(claim.AuthContextID) || !validText(claim.AuthGeneration) || !validText(claim.SecurityGeneration) || !validText(claim.SourceType) || claim.ActivatedAt.IsZero() || claim.UpdatedAt.IsZero() {
		return ErrInvalid
	}
	if claim.WorkspaceID != nil && !validOpaque(*claim.WorkspaceID) {
		return ErrInvalid
	}
	if claim.State != StateActive && claim.State != StateInactive && claim.State != StateDeactivated && claim.State != StateExpired && claim.State != StateStale {
		return ErrInvalid
	}
	if claim.SourceType == "manual" {
		if claim.ManualStackKey == nil || !validText(*claim.ManualStackKey) {
			return ErrInvalid
		}
		if claim.SourceEventID != nil || claim.Sequence != nil {
			return ErrInvalid
		}
	}
	if claim.ExpiresAt != nil && !claim.ExpiresAt.After(claim.ActivatedAt) {
		return ErrInvalid
	}
	return nil
}

func validateOwner(owner Owner) error {
	if !validUUID7(owner.UserID) || !validText(owner.AuthContextType) || !validOpaque(owner.AuthContextID) || !validText(owner.AuthGeneration) || !validText(owner.SecurityGeneration) {
		return ErrInvalid
	}
	if owner.WorkspaceID != nil && !validOpaque(*owner.WorkspaceID) {
		return ErrInvalid
	}
	return nil
}

func validateRef(ref Ref) error {
	if !validRef(ref.Kind, ref.ID) {
		return ErrInvalid
	}
	return nil
}

func validRef(kind RefKind, value string) bool {
	if !validText(value) {
		return false
	}
	if kind == UserRef {
		return validUUID7(value)
	}
	return kind == BuiltinRef && !strings.ContainsAny(value, "\r\n\t")
}

func validUUID7(value string) bool {
	id, err := uuid.Parse(value)
	return err == nil && id.Version() == 7 && id.Variant() == uuid.RFC4122 && id.String() == value
}

func validOpaque(value string) bool {
	return value != "" && len(value) <= 256 && utf8.ValidString(value) && strings.TrimSpace(value) == value && !strings.ContainsRune(value, '\x00')
}
func validText(value string) bool {
	return value != "" && len(value) <= 4096 && utf8.ValidString(value) && strings.TrimSpace(value) == value && !strings.ContainsAny(value, "\r\n\t")
}
func objectJSON(value string) bool {
	var decoded map[string]json.RawMessage
	return strings.TrimSpace(value) == value && json.Unmarshal([]byte(value), &decoded) == nil && decoded != nil
}
func sameWorkspace(a, b *string) bool {
	return (a == nil && b == nil) || (a != nil && b != nil && *a == *b)
}
