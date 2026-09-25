package commandconfig

import (
	"context"
	"time"

	"gorm.io/gorm"
)

// BumpGenerationTx incrementa somente a geração do escopo exato já criado.
// É destinado a consumidores que invalidam estado derivado dentro do próprio
// TX/ledger; não cria linha, não abre transação e usa CAS contra a geração
// observada. O chamador deve compor a chamada sob seu gate transacional.
func (s *Store) BumpGenerationTx(ctx context.Context, tx *gorm.DB, scope Scope) (Generation, error) {
	if s == nil || s.db == nil || ctx == nil || tx == nil || !validScope(scope) || !isTransactionDB(tx) || !sameSQLDatabase(s.db, tx) {
		return Generation{}, ErrInvalid
	}
	if err := ctx.Err(); err != nil {
		return Generation{}, err
	}
	rows, err := readGenerations(tx, scope)
	if err != nil {
		return Generation{}, err
	}
	var current Generation
	for _, row := range rows {
		if sameWorkspace(row.WorkspaceID, scope.WorkspaceID) {
			current = row
			break
		}
	}
	if current.ID == "" || current.Generation < 1 {
		return Generation{}, ErrInvalid
	}
	updatedAt := time.Now().UTC()
	result := tx.WithContext(ctx).Model(&Generation{}).
		Where("id = ? AND user_id = ? AND generation = ?", current.ID, current.UserID, current.Generation).
		Updates(map[string]any{"generation": current.Generation + 1, "updated_at": updatedAt})
	if result.Error != nil {
		return Generation{}, result.Error
	}
	if result.RowsAffected != 1 {
		return Generation{}, ErrStale
	}
	current.Generation++
	current.UpdatedAt = updatedAt
	return current, ctx.Err()
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

func sameSQLDatabase(left, right *gorm.DB) bool {
	if left == nil || right == nil {
		return false
	}
	leftSQL, leftErr := left.DB()
	rightSQL, rightErr := right.DB()
	return leftErr == nil && rightErr == nil && leftSQL != nil && leftSQL == rightSQL
}
