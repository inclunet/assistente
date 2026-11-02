package toolinvocations

import (
	"context"
	"errors"
	"slices"

	"assistente/internal/database"
	"gorm.io/gorm"
)

var errMaintenanceScopeChanged = errors.New("conversas afetadas pela manutenção mudaram")

// Descobre somente identidades, sem materializar payloads. Revalida sob writer
// lock antes de alterar qualquer linha, incluindo filhos que serão desvinculados.
// Nenhum mutex SQLite é mantido enquanto se adquire a barreira do projetor.
func (r *DBRepository) mutateChatMaintenance(ctx context.Context, operation string, target func(*gorm.DB) *gorm.DB, write func(*gorm.DB) error) error {
	return database.WithSQLiteMaintenance(ctx, func() error {
		if r.maintenanceCoordinator == nil {
			return database.WithSQLiteImmediateTransaction(ctx, r.db, "toolinvocations."+operation, write)
		}
		for attempt := 0; attempt < 8; attempt++ {
			ids, err := maintenanceConversationIDs(ctx, r.db, target)
			if err != nil {
				return err
			}
			err = r.maintenanceCoordinator(ctx, ids, func() error {
				return database.WithSQLiteImmediateTransaction(ctx, r.db, "toolinvocations."+operation, func(tx *gorm.DB) error {
					current, err := maintenanceConversationIDs(ctx, tx, target)
					if err != nil {
						return err
					}
					if !slices.Equal(ids, current) {
						return errMaintenanceScopeChanged
					}
					return write(tx)
				})
			})
			if !errors.Is(err, errMaintenanceScopeChanged) {
				return err
			}
		}
		return errMaintenanceScopeChanged
	})
}

func maintenanceConversationIDs(ctx context.Context, db *gorm.DB, target func(*gorm.DB) *gorm.DB) ([]string, error) {
	db = db.WithContext(ctx)
	parents := target(db).Select("tool_invocations.id")
	affected := database.ScopeByUser(ctx, db.Model(&database.ToolInvocation{}), "user_id").
		Select("user_id", "conversation_id", "turn_id", "origin_id").
		Where("id IN (?) OR parent_invocation_id IN (?)", parents, parents)
	var ids []string
	// A mesma união de vínculos dos triggers v43: conversation_id sozinho não
	// representa todos os leitores de linhas legadas. Cada ramo usa PK/turn_id,
	// sem cruzar cada invocação com todas as conversas. OriginID cobre pré-v18.
	err := db.Raw(`WITH affected AS (?)
 SELECT c.id FROM affected i JOIN conversations c ON c.id=i.conversation_id AND c.user_id=i.user_id
 UNION SELECT c.id FROM affected i JOIN chat_messages m ON m.id=i.turn_id
   JOIN conversations c ON c.id=m.conversation_id AND c.user_id=i.user_id
 UNION SELECT c.id FROM affected i JOIN chat_messages m ON m.turn_id=i.turn_id
   JOIN conversations c ON c.id=m.conversation_id AND c.user_id=i.user_id
 UNION SELECT c.id FROM affected i JOIN chat_messages m ON m.id=i.origin_id
   JOIN conversations c ON c.id=m.conversation_id AND c.user_id=i.user_id
   WHERE i.conversation_id IS NULL
 ORDER BY 1`, affected).Scan(&ids).Error
	return ids, err
}
