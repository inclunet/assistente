package database

import (
	"context"
	"errors"

	"gorm.io/gorm"
)

// Boot com adoção pendente mantém o login disponível, mas não autoriza uma
// leitura alternativa do histórico. Só consulta readiness no caminho de erro;
// leitores saudáveis não acrescentam uma consulta de schema a cada página.
func chatSelectionReadError(ctx context.Context, db *gorm.DB, readErr error) error {
	if readErr == nil {
		return nil
	}
	if errors.Is(NewChatTurnRepository(db).CheckReady(ctx), ErrChatTurnMigrationPending) {
		return errors.Join(ErrChatTurnMigrationPending, readErr)
	}
	return readErr
}
