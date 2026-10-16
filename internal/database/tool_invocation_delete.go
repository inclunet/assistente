package database

import (
	"context"

	"gorm.io/gorm"
)

// DeleteToolInvocationsWithChildrenDetachedTx removes the invocations selected
// by target after clearing same-user children that survive the deletion. The
// caller must pass a query scoped to the user in ctx and invoke this from the
// transaction that owns the related cleanup.
func DeleteToolInvocationsWithChildrenDetachedTx(ctx context.Context, tx *gorm.DB, target *gorm.DB) (int64, error) {
	if _, err := RequireUserID(ctx); err != nil {
		return 0, err
	}
	parentIDs := target.Session(&gorm.Session{}).Select("tool_invocations.id")
	children := ScopeByUser(ctx, tx.Model(&ToolInvocation{}), "user_id").
		Where("parent_invocation_id IN (?)", parentIDs)
	if err := children.Update("parent_invocation_id", nil).Error; err != nil {
		return 0, err
	}
	result := target.Session(&gorm.Session{}).Delete(&ToolInvocation{})
	if result.Error != nil {
		return 0, result.Error
	}
	return result.RowsAffected, nil
}
