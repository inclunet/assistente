package commandportability

import (
	"context"
	"errors"
	"strings"

	"assistente/internal/credentials"
	"assistente/internal/database"
	"gorm.io/gorm"
)

var (
	ErrCredentialPatternInvalid      = errors.New("referência de credential pattern inválida")
	ErrCredentialPatternManaged      = errors.New("credential pattern gerenciado não pode ser portado")
	ErrCredentialPatternStoreMissing = errors.New("store de credenciais indisponível")
)

// NewCredentialPatternResolver cria a porta de leitura de metadados usada
// pelo planejador de portabilidade. A consulta nunca carrega colunas de
// segredo: pattern só é resolvido por igualdade exata no usuário do contexto.
func NewCredentialPatternResolver(db *gorm.DB) func(context.Context, string) (CredentialStatus, error) {
	return func(ctx context.Context, pattern string) (CredentialStatus, error) {
		if db == nil {
			return CredentialMissing, ErrCredentialPatternStoreMissing
		}
		if ctx == nil {
			return CredentialMissing, database.ErrUserScopeRequired
		}
		userID, ok := database.UserIDFromContext(ctx)
		if !ok || strings.TrimSpace(userID) == "" {
			return CredentialMissing, database.ErrUserScopeRequired
		}
		if pattern == "" || pattern != strings.TrimSpace(pattern) {
			return CredentialMissing, ErrCredentialPatternInvalid
		}
		if credentials.IsManagedPattern(pattern) {
			return CredentialMissing, ErrCredentialPatternManaged
		}

		var owned int64
		if err := db.WithContext(ctx).Model(&database.CredentialEntry{}).
			Where("pattern = ?", pattern).Where("user_id = ?", userID).
			Count(&owned).Error; err != nil {
			return CredentialMissing, err
		}
		if owned == 1 {
			return CredentialAvailable, nil
		}
		if owned > 1 {
			return CredentialAmbiguous, nil
		}

		var foreign int64
		if err := db.WithContext(ctx).Model(&database.CredentialEntry{}).
			Where("pattern = ?", pattern).Where("user_id <> ?", userID).
			Count(&foreign).Error; err != nil {
			return CredentialMissing, err
		}
		if foreign > 0 {
			return CredentialForeign, nil
		}
		return CredentialMissing, nil
	}
}
