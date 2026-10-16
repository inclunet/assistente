package commandportability

import (
	"context"
	"errors"
	"strings"

	"assistente/internal/database"
	"gorm.io/gorm"
)

var ErrReferenceStoreUnavailable = errors.New("store de referências de portabilidade indisponível")

var ownershipReferenceTables = [...]string{
	"command_layers",
	"command_bindings",
	"command_layer_activation_rules",
}

// NewStoreOwnership cria uma porta somente leitura. As tabelas são uma
// allowlist fixa: nenhum nome vindo do arquivo de portabilidade chega ao SQL.
func NewStoreOwnership(db *gorm.DB) OwnershipPort {
	return func(ctx context.Context, workspaceID, id string) (Ownership, error) {
		if db == nil || ctx == nil || strings.TrimSpace(id) == "" || strings.TrimSpace(id) != id {
			return AbsentOwner, ErrReferenceStoreUnavailable
		}
		userID, err := database.RequireUserID(ctx)
		if err != nil {
			return AbsentOwner, err
		}
		if err := ctx.Err(); err != nil {
			return AbsentOwner, err
		}
		exact, current, any, err := ownershipCounts(ctx, db, userID, workspaceID, id)
		if err != nil {
			return AbsentOwner, err
		}
		if exact > 0 {
			if exact > 1 || any != exact {
				return AmbiguousOwnership, nil
			}
			return CurrentUserOwner, nil
		}
		if current > 0 {
			return AmbiguousOwnership, nil
		}
		if any > 0 {
			return ForeignUserOwner, nil
		}
		return AbsentOwner, nil
	}
}

func ownershipCounts(ctx context.Context, db *gorm.DB, userID, workspaceID, id string) (exact, current, any int, err error) {
	for _, table := range ownershipReferenceTables {
		var rowCount int64
		query := db.WithContext(ctx).Table(table).Where("id = ?", id)
		if err = query.Count(&rowCount).Error; err != nil {
			return 0, 0, 0, err
		}
		any += int(rowCount)

		var currentCount int64
		currentQuery := db.WithContext(ctx).Table(table).Where("id = ? AND user_id = ?", id, userID)
		if err = currentQuery.Count(&currentCount).Error; err != nil {
			return 0, 0, 0, err
		}
		current += int(currentCount)

		exactQuery := currentQuery
		if workspaceID == "" {
			exactQuery = exactQuery.Where("workspace_id IS NULL")
		} else {
			exactQuery = exactQuery.Where("workspace_id = ?", workspaceID)
		}
		var exactCount int64
		if err = exactQuery.Count(&exactCount).Error; err != nil {
			return 0, 0, 0, err
		}
		exact += int(exactCount)
	}
	return exact, current, any, nil
}

// NewStoreLayerName cria a porta de conflito de nome. Nomes pertencem apenas
// a camadas; a consulta nunca carrega a linha nem expõe seu owner.
func NewStoreLayerName(db *gorm.DB) NamePort {
	return func(ctx context.Context, workspaceID, name string) (Ownership, error) {
		if db == nil || ctx == nil || strings.TrimSpace(name) == "" || strings.TrimSpace(name) != name {
			return AbsentOwner, ErrReferenceStoreUnavailable
		}
		userID, err := database.RequireUserID(ctx)
		if err != nil {
			return AbsentOwner, err
		}
		if err := ctx.Err(); err != nil {
			return AbsentOwner, err
		}
		base := db.WithContext(ctx).Table("command_layers").Where("name = ? AND user_id = ?", name, userID)
		if workspaceID == "" {
			base = base.Where("workspace_id IS NULL")
		} else {
			base = base.Where("workspace_id = ?", workspaceID)
		}
		var count int64
		if err := base.Count(&count).Error; err != nil {
			return AbsentOwner, err
		}
		switch {
		case count > 1:
			return AmbiguousOwnership, nil
		case count == 1:
			return CurrentUserOwner, nil
		default:
			return AbsentOwner, nil
		}
	}
}
