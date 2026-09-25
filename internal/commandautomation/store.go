package commandautomation

import (
	"context"
	"database/sql"
	"errors"
	"math"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type grantRow struct {
	ID                       string     `gorm:"column:id;primaryKey"`
	UserID                   string     `gorm:"column:user_id"`
	WorkspaceID              *string    `gorm:"column:workspace_id"`
	LayerRefKind             string     `gorm:"column:layer_ref_kind"`
	LayerRef                 string     `gorm:"column:layer_ref"`
	RuleRefKind              string     `gorm:"column:rule_ref_kind"`
	RuleRef                  string     `gorm:"column:rule_ref"`
	RuleFingerprint          string     `gorm:"column:rule_fingerprint"`
	EventName                string     `gorm:"column:event_name"`
	ProducerTypesFingerprint string     `gorm:"column:producer_types_fingerprint"`
	Generation               int64      `gorm:"column:automation_grant_generation"`
	GrantFingerprint         string     `gorm:"column:automation_grant_fingerprint"`
	AuthorizationDecisionID  string     `gorm:"column:authorization_decision_id"`
	GrantedAt                time.Time  `gorm:"column:granted_at"`
	GrantedBy                string     `gorm:"column:granted_by"`
	RevokedAt                *time.Time `gorm:"column:revoked_at"`
	RevokedBy                *string    `gorm:"column:revoked_by"`
	RevocationReason         *string    `gorm:"column:revocation_reason"`
}

func (grantRow) TableName() string { return "command_layer_automation_grants" }

type ruleRow struct {
	ID                         string  `gorm:"column:id"`
	UserID                     string  `gorm:"column:user_id"`
	WorkspaceID                *string `gorm:"column:workspace_id"`
	LayerRefKind               string  `gorm:"column:layer_ref_kind"`
	LayerRef                   string  `gorm:"column:layer_ref"`
	RuleRefKind                string  `gorm:"column:rule_ref_kind"`
	RuleRef                    string  `gorm:"column:rule_ref"`
	Mode                       string  `gorm:"column:mode"`
	Condition                  string  `gorm:"column:condition"`
	Lifecycle                  string  `gorm:"column:lifecycle"`
	EventName                  string  `gorm:"column:event_name"`
	AllowedProducerTypes       string  `gorm:"column:allowed_internal_producer_types"`
	AuthorizationDecisionID    *string `gorm:"column:authorization_decision_id"`
	AutomationGrantID          *string `gorm:"column:automation_grant_id"`
	AutomationGrantGeneration  *int64  `gorm:"column:automation_grant_generation"`
	AutomationGrantFingerprint *string `gorm:"column:automation_grant_fingerprint"`
	Enabled                    bool    `gorm:"column:enabled"`
	Source                     string  `gorm:"column:source"`
	ReviewStatus               string  `gorm:"column:review_status"`
}

func (ruleRow) TableName() string { return "command_layer_activation_rules" }

type Store struct {
	db  *gorm.DB
	now func() time.Time
}

func New(db *gorm.DB, now func() time.Time) (*Store, error) {
	if db == nil || now == nil {
		return nil, ErrInvalid
	}
	return &Store{db: db, now: now}, nil
}

func validUUID7(value string) bool {
	id, err := uuid.Parse(value)
	return err == nil && id.Version() == 7 && id.Variant() == uuid.RFC4122 && id.String() == value
}

func validOwner(owner Owner) bool {
	if !validUUID7(owner.UserID) {
		return false
	}
	if owner.WorkspaceID == nil {
		return true
	}
	w := *owner.WorkspaceID
	return len(w) > 0 && len(w) <= 256 && utf8.ValidString(w) && strings.TrimSpace(w) == w && !strings.ContainsRune(w, '\x00')
}

func validRef(ref RuleRef) bool {
	if (ref.Kind != "builtin" && ref.Kind != "user") || strings.TrimSpace(ref.Ref) != ref.Ref || ref.Ref == "" || len(ref.Ref) > 256 || !utf8.ValidString(ref.Ref) {
		return false
	}
	return ref.Kind != "user" || validUUID7(ref.Ref)
}

func validKey(key NaturalKey) bool {
	return validOwner(key.Owner) && validRef(key.LayerRef) && validRef(key.RuleRef)
}

func keyFromRule(rule Rule) NaturalKey {
	return NaturalKey{Owner: rule.Owner, LayerRef: rule.LayerRef, RuleRef: rule.RuleRef}
}

func sameScope(left, right *string) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

func scopeWhere(query *gorm.DB, owner Owner) *gorm.DB {
	query = query.Where("user_id = ?", owner.UserID)
	if owner.WorkspaceID == nil {
		return query.Where("workspace_id IS NULL")
	}
	return query.Where("workspace_id = ?", *owner.WorkspaceID)
}

func ruleQuery(db *gorm.DB, owner Owner, ref RuleRef) *gorm.DB {
	query := scopeWhere(db.Model(&ruleRow{}), owner).Where("rule_ref_kind = ? AND rule_ref = ?", ref.Kind, ref.Ref)
	return query
}

func grantQuery(db *gorm.DB, owner Owner, key NaturalKey) *gorm.DB {
	query := scopeWhere(db.Model(&grantRow{}), owner).
		Where("layer_ref_kind = ? AND layer_ref = ? AND rule_ref_kind = ? AND rule_ref = ?", key.LayerRef.Kind, key.LayerRef.Ref, key.RuleRef.Kind, key.RuleRef.Ref)
	return query
}

func cloneString(value *string) *string {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func cloneTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func ruleFromRow(row ruleRow) (Rule, error) {
	producers, err := parseProducerTypes(row.AllowedProducerTypes)
	if err != nil {
		return Rule{}, err
	}
	return Rule{ID: row.ID, Owner: Owner{UserID: row.UserID, WorkspaceID: cloneString(row.WorkspaceID)},
		LayerRef: RuleRef{Kind: row.LayerRefKind, Ref: row.LayerRef}, RuleRef: RuleRef{Kind: row.RuleRefKind, Ref: row.RuleRef},
		Mode: row.Mode, Condition: row.Condition, Lifecycle: row.Lifecycle, EventName: row.EventName,
		AllowedInternalProducerTypes: producers, AuthorizationDecisionID: cloneString(row.AuthorizationDecisionID),
		AutomationGrantID: cloneString(row.AutomationGrantID), AutomationGrantGeneration: row.AutomationGrantGeneration,
		AutomationGrantFingerprint: cloneString(row.AutomationGrantFingerprint), Enabled: row.Enabled, Source: row.Source, ReviewStatus: row.ReviewStatus}, nil
}

func grantFromRow(row grantRow) Grant {
	return Grant{ID: row.ID, Owner: Owner{UserID: row.UserID, WorkspaceID: cloneString(row.WorkspaceID)},
		LayerRef: RuleRef{Kind: row.LayerRefKind, Ref: row.LayerRef}, RuleRef: RuleRef{Kind: row.RuleRefKind, Ref: row.RuleRef},
		RuleFingerprint: row.RuleFingerprint, EventName: row.EventName, ProducerTypesFingerprint: row.ProducerTypesFingerprint,
		AutomationGrantGeneration: row.Generation, AutomationGrantFingerprint: row.GrantFingerprint,
		AuthorizationDecisionID: row.AuthorizationDecisionID, GrantedAt: row.GrantedAt, GrantedBy: row.GrantedBy,
		RevokedAt: cloneTime(row.RevokedAt), RevokedBy: cloneString(row.RevokedBy), RevocationReason: cloneString(row.RevocationReason)}
}

func (s *Store) LoadRule(ctx context.Context, owner Owner, ref RuleRef) (Rule, error) {
	if s == nil || s.db == nil || ctx == nil || !validOwner(owner) || !validRef(ref) {
		return Rule{}, ErrInvalid
	}
	if err := ctx.Err(); err != nil {
		return Rule{}, err
	}
	var row ruleRow
	if err := ruleQuery(s.db.WithContext(ctx), owner, ref).Take(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return Rule{}, ErrNotFound
		}
		return Rule{}, err
	}
	rule, err := ruleFromRow(row)
	if err != nil {
		return Rule{}, err
	}
	if rule.Owner.UserID != owner.UserID || !sameScope(rule.Owner.WorkspaceID, owner.WorkspaceID) {
		return Rule{}, ErrForeignScope
	}
	return rule, nil
}

func (s *Store) LoadActive(ctx context.Context, owner Owner, key NaturalKey) (Grant, error) {
	if s == nil || s.db == nil || ctx == nil || !validOwner(owner) || !validKey(key) || key.Owner.UserID != owner.UserID || !sameScope(key.Owner.WorkspaceID, owner.WorkspaceID) {
		return Grant{}, ErrInvalid
	}
	var row grantRow
	if err := grantQuery(s.db.WithContext(ctx), owner, key).Where("revoked_at IS NULL").Take(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return Grant{}, ErrNotFound
		}
		return Grant{}, err
	}
	return grantFromRow(row), nil
}

// LoadGrantsTx lê o histórico completo de grants no TX fornecido. O escopo de
// workspace inclui o global e o workspace local, como o projetor de
// commandconfig; grants revogados permanecem no resultado para o diff de
// restore e para auditoria. A função não abre transação nem altera estado.
func LoadGrantsTx(ctx context.Context, tx *gorm.DB, owner Owner) ([]Grant, error) {
	if ctx == nil || tx == nil || !validOwner(owner) {
		return nil, ErrInvalid
	}
	query := tx.WithContext(ctx).Model(&grantRow{}).Where("user_id = ?", owner.UserID)
	if owner.WorkspaceID == nil {
		query = query.Where("workspace_id IS NULL")
	} else {
		query = query.Where("workspace_id IS NULL OR workspace_id = ?", *owner.WorkspaceID)
	}
	var rows []grantRow
	if err := query.Order("id").Find(&rows).Error; err != nil {
		return nil, err
	}
	result := make([]Grant, len(rows))
	for i, row := range rows {
		grant := grantFromRow(row)
		if err := ValidateGrant(grant); err != nil {
			return nil, err
		}
		result[i] = grant
	}
	return result, nil
}

// ListGrants expõe a mesma leitura fora de uma transação para diagnósticos e
// integração; mutações continuam restritas às primitivas de grant.
func (s *Store) ListGrants(ctx context.Context, owner Owner) ([]Grant, error) {
	if s == nil || s.db == nil {
		return nil, ErrInvalid
	}
	return LoadGrantsTx(ctx, s.db, owner)
}

func maxGeneration(ctx context.Context, tx *gorm.DB, key NaturalKey) (int64, error) {
	var value sql.NullInt64
	if err := grantQuery(tx.WithContext(ctx), key.Owner, key).Select("MAX(automation_grant_generation)").Scan(&value).Error; err != nil {
		return 0, err
	}
	if !value.Valid {
		return 0, nil
	}
	return value.Int64, nil
}

func validateGeneration(value int64) bool { return value >= 0 && value < math.MaxInt64 }
