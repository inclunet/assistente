package commandexecution

import (
	"context"
	"time"

	"assistente/internal/auth"
	"assistente/internal/commandcatalog"
	"assistente/internal/database"
	"gorm.io/gorm"
)

// NewLocalReadAuthorizer constrói a autorização do recorte local de comandos
// read-only. A política é um snapshot de bootstrap; identidade, estado da
// sessão e role são sempre reconsultados no banco durante a autorização. O
// callback é uma leitura pontual e não coordena mutações concorrentes: role,
// revogação e troca de usuário devem ser coordenadas pelo mesmo EpochService
// do executor.
func NewLocalReadAuthorizer(db *gorm.DB, allowedRoles map[string][]string) (func(context.Context, auth.LocalSessionPrincipal, string, commandcatalog.Source) error, error) {
	if db == nil || len(allowedRoles) == 0 {
		return nil, ErrInvalidConfiguration
	}

	policy := make(map[string][]string, len(allowedRoles))
	for commandID, roles := range allowedRoles {
		if !commandIDPattern.MatchString(commandID) || len(roles) == 0 {
			return nil, ErrInvalidConfiguration
		}
		copiedRoles := make([]string, len(roles))
		seen := make(map[string]struct{}, len(roles))
		for i, role := range roles {
			if role != database.UserRoleAdmin && role != database.UserRoleUser {
				return nil, ErrInvalidConfiguration
			}
			if _, duplicate := seen[role]; duplicate {
				return nil, ErrInvalidConfiguration
			}
			seen[role] = struct{}{}
			copiedRoles[i] = role
		}
		policy[commandID] = copiedRoles
	}

	return func(ctx context.Context, principal auth.LocalSessionPrincipal, commandID string, source commandcatalog.Source) error {
		if ctx == nil {
			return ErrDenied
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if !validID(principal.UserID) || !validID(principal.SessionID) ||
			(source != commandcatalog.Palette && source != commandcatalog.UI && source != commandcatalog.CLI) {
			return ErrDenied
		}
		roles, ok := policy[commandID]
		if !ok {
			return ErrDenied
		}

		var row struct {
			Role      string
			ExpiresAt time.Time
		}
		result := db.WithContext(ctx).Table("sessions AS s").
			Select("u.role, s.expires_at").
			Joins("JOIN users AS u ON u.id = s.user_id").
			Where("s.id = ? AND s.user_id = ? AND s.revoked_at IS NULL AND s.expires_at > ? AND u.is_active = ?", principal.SessionID, principal.UserID, time.Now(), true).
			Take(&row)
		if err := ctx.Err(); err != nil {
			return err
		}
		if result.Error != nil {
			return ErrDenied
		}
		// O filtro acima evita consultas evidentemente expiradas, mas não
		// substitui esta checagem: a sessão pode expirar enquanto o DB esperava.
		if !row.ExpiresAt.After(time.Now()) {
			return ErrDenied
		}
		for _, role := range roles {
			if row.Role == role {
				return nil
			}
		}
		return ErrDenied
	}, nil
}
