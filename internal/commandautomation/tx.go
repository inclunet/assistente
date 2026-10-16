package commandautomation

import (
	"context"
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"
)

func validateActiveTx(ctx context.Context, tx *gorm.DB, owner Owner, key NaturalKey, expected GrantReference) (Grant, error) {
	if ctx == nil || tx == nil || !validOwner(owner) || !validKey(key) || key.Owner.UserID != owner.UserID || !sameScope(key.Owner.WorkspaceID, owner.WorkspaceID) {
		return Grant{}, ErrInvalid
	}
	if expected.ID == "" || !validUUID7(expected.ID) || expected.Generation <= 0 || expected.Fingerprint == "" {
		return Grant{}, ErrInvalid
	}
	var row grantRow
	if err := grantQuery(tx.WithContext(ctx), owner, key).Where("revoked_at IS NULL").Take(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return Grant{}, ErrNotFound
		}
		return Grant{}, err
	}
	if row.ID != expected.ID || row.Generation != expected.Generation || row.GrantFingerprint != expected.Fingerprint || row.EventName != JobRunStateEvent {
		return Grant{}, ErrStale
	}
	return grantFromRow(row), nil
}

func activeGrantTx(ctx context.Context, tx *gorm.DB, owner Owner, key NaturalKey) (Grant, error) {
	if ctx == nil || tx == nil || !validOwner(owner) || !validKey(key) || key.Owner.UserID != owner.UserID || !sameScope(key.Owner.WorkspaceID, owner.WorkspaceID) {
		return Grant{}, ErrInvalid
	}
	var row grantRow
	if err := grantQuery(tx.WithContext(ctx), owner, key).Where("revoked_at IS NULL").Take(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return Grant{}, ErrNotFound
		}
		return Grant{}, err
	}
	return grantFromRow(row), nil
}

// ValidateTx é a porta transacional para o consumidor de claims. A referência
// completa é obrigatória: validar apenas a chave natural permitiria usar uma
// concessão reatribuída depois de revoke/regrant.
func ValidateTx(ctx context.Context, tx *gorm.DB, owner Owner, key NaturalKey, expected GrantReference) (Grant, error) {
	return validateActiveTx(ctx, tx, owner, key, expected)
}

func (s *Store) ValidateTx(ctx context.Context, tx *gorm.DB, owner Owner, key NaturalKey, expected GrantReference) (Grant, error) {
	if s == nil || s.db == nil || tx == nil {
		return Grant{}, ErrInvalid
	}
	if !sameSQLDatabase(s.db, tx) {
		return Grant{}, ErrInvalid
	}
	return validateActiveTx(ctx, tx, owner, key, expected)
}

// Validate é um alias para integração de claims que usa o nome curto.
func (s *Store) Validate(ctx context.Context, tx *gorm.DB, owner Owner, key NaturalKey, expected GrantReference) (Grant, error) {
	return s.ValidateTx(ctx, tx, owner, key, expected)
}

func revokeTx(ctx context.Context, tx *gorm.DB, owner Owner, key NaturalKey, revokedBy, reason string, now time.Time) error {
	if ctx == nil || tx == nil || !validOwner(owner) || !validKey(key) || key.Owner.UserID != owner.UserID || !sameScope(key.Owner.WorkspaceID, owner.WorkspaceID) ||
		strings.TrimSpace(revokedBy) != revokedBy || revokedBy == "" || len(revokedBy) > 256 || strings.TrimSpace(reason) != reason || reason == "" || len(reason) > 256 || now.IsZero() {
		return ErrInvalid
	}
	result := grantQuery(tx.WithContext(ctx), owner, key).Where("revoked_at IS NULL").Updates(map[string]any{"revoked_at": now.UTC(), "revoked_by": revokedBy, "revocation_reason": reason})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return ErrNotFound
	}
	return ctx.Err()
}

// RevokeTx exige uma transação já aberta pelo host/gate e owner derivado de
// autenticação. Não há writer global que aceite payload de job.
func RevokeTx(ctx context.Context, tx *gorm.DB, owner Owner, key NaturalKey, revokedBy, reason string) error {
	return RevokeTxAt(ctx, tx, owner, key, revokedBy, reason, time.Now().UTC())
}

// RevokeTxAt usa o instante já vinculado ao diff/receipt pelo host.
func RevokeTxAt(ctx context.Context, tx *gorm.DB, owner Owner, key NaturalKey, revokedBy, reason string, now time.Time) error {
	return revokeTx(ctx, tx, owner, key, revokedBy, reason, now)
}

func (s *Store) RevokeTx(ctx context.Context, tx *gorm.DB, owner Owner, key NaturalKey, revokedBy, reason string) error {
	return s.RevokeTxAt(ctx, tx, owner, key, revokedBy, reason, s.now())
}

// RevokeTxAt mantém o instante de revogação já preparado pelo host. Isso
// permite que o documento after e a linha efetivamente revogada compartilhem
// o mesmo valor, sem fazer o hook alegações baseadas em um relógio posterior.
func (s *Store) RevokeTxAt(ctx context.Context, tx *gorm.DB, owner Owner, key NaturalKey, revokedBy, reason string, now time.Time) error {
	if s == nil || s.db == nil || tx == nil || !sameSQLDatabase(s.db, tx) {
		return ErrInvalid
	}
	return revokeTx(ctx, tx, owner, key, revokedBy, reason, now)
}

func revokeLayerTx(ctx context.Context, tx *gorm.DB, owner Owner, layerRef RuleRef, revokedBy, reason string, now time.Time) error {
	if ctx == nil || tx == nil || !validOwner(owner) || !validRef(layerRef) || strings.TrimSpace(revokedBy) != revokedBy || revokedBy == "" || len(revokedBy) > 256 || strings.TrimSpace(reason) != reason || reason == "" || len(reason) > 256 || now.IsZero() {
		return ErrInvalid
	}
	result := scopeWhere(tx.WithContext(ctx).Model(&grantRow{}), owner).
		Where("layer_ref_kind = ? AND layer_ref = ? AND revoked_at IS NULL", layerRef.Kind, layerRef.Ref).
		Updates(map[string]any{"revoked_at": now.UTC(), "revoked_by": revokedBy, "revocation_reason": reason})
	if result.Error != nil {
		return result.Error
	}
	return ctx.Err()
}

// RevokeLayerTx revoga todos os grants ativos da layer no único owner/escopo
// fornecido. É idempotente quando não há grant e nunca abre/fecha transação;
// o chamador deve compô-la no TX do delete/disable/restore.
func RevokeLayerTx(ctx context.Context, tx *gorm.DB, owner Owner, layerRef RuleRef, revokedBy, reason string) error {
	return RevokeLayerTxAt(ctx, tx, owner, layerRef, revokedBy, reason, time.Now().UTC())
}

// RevokeLayerTxAt é a variante clock-bound da revogação agregada por layer.
func RevokeLayerTxAt(ctx context.Context, tx *gorm.DB, owner Owner, layerRef RuleRef, revokedBy, reason string, now time.Time) error {
	return revokeLayerTx(ctx, tx, owner, layerRef, revokedBy, reason, now)
}

func (s *Store) RevokeLayerTx(ctx context.Context, tx *gorm.DB, owner Owner, layerRef RuleRef, revokedBy, reason string) error {
	return s.RevokeLayerTxAt(ctx, tx, owner, layerRef, revokedBy, reason, s.now())
}

// RevokeLayerTxAt é a variante clock-bound usada por mutações compostas.
func (s *Store) RevokeLayerTxAt(ctx context.Context, tx *gorm.DB, owner Owner, layerRef RuleRef, revokedBy, reason string, now time.Time) error {
	if s == nil || s.db == nil || tx == nil || !sameSQLDatabase(s.db, tx) {
		return ErrInvalid
	}
	return revokeLayerTx(ctx, tx, owner, layerRef, revokedBy, reason, now)
}

func sameSQLDatabase(left, right *gorm.DB) bool {
	if left == nil || right == nil {
		return false
	}
	leftSQL, leftErr := left.DB()
	rightSQL, rightErr := right.DB()
	return leftErr == nil && rightErr == nil && leftSQL != nil && leftSQL == rightSQL
}
