package auth

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
)

var ErrUnauthenticatedLocalSession = errors.New("sessão local não autenticada")

// LocalSessionPrincipal contém somente identidade derivada pelo backend.
// Não transporta role, token ou gerações; não é autorização de comando.
type LocalSessionPrincipal struct{ UserID, SessionID string }

// AuthenticateLocalAccess revalida assinatura e estado autoritativo da sessão
// local em cada consulta. Não modifica VerifyAccessToken nem autenticação externa.
// O executor futuro deve reconsultar sob DispatchGate e coordenar revogações
// com o mesmo gate: esta consulta isolada não elimina corridas após seu retorno.
func (s *SessionService) AuthenticateLocalAccess(ctx context.Context, token string) (LocalSessionPrincipal, error) {
	if s == nil || s.db == nil || s.signer == nil || s.now == nil || ctx == nil {
		return LocalSessionPrincipal{}, ErrUnauthenticatedLocalSession
	}
	if err := ctx.Err(); err != nil {
		return LocalSessionPrincipal{}, err
	}
	claims, err := s.VerifyAccessToken(token)
	if err != nil || claims == nil || !canonicalSessionUUID(claims.Subject) || !canonicalSessionUUID(claims.SessionID) {
		return LocalSessionPrincipal{}, ErrUnauthenticatedLocalSession
	}
	var row struct {
		UserID    string
		SessionID string
		ExpiresAt time.Time
	}
	// Uma única leitura relaciona a sessão assinada à conta ainda ativa. Não
	// consulta por ID recebido separadamente nem usa role antiga contida no JWT.
	result := s.db.WithContext(ctx).Table("sessions AS s").Select("s.user_id, s.id AS session_id, s.expires_at").Joins("JOIN users AS u ON u.id = s.user_id").Where("s.id = ? AND s.user_id = ? AND s.revoked_at IS NULL AND u.is_active = ?", claims.SessionID, claims.Subject, true).Take(&row)
	if err := ctx.Err(); err != nil {
		return LocalSessionPrincipal{}, err
	}
	if result.Error != nil {
		return LocalSessionPrincipal{}, ErrUnauthenticatedLocalSession
	}
	now := s.now()
	if now.IsZero() || !row.ExpiresAt.After(now) || !time.Unix(claims.ExpiresAt, 0).After(now) {
		return LocalSessionPrincipal{}, ErrUnauthenticatedLocalSession
	}
	return LocalSessionPrincipal{UserID: row.UserID, SessionID: row.SessionID}, nil
}

func canonicalSessionUUID(value string) bool {
	id, err := uuid.Parse(value)
	return err == nil && id.Version() == 7 && id.Variant() == uuid.RFC4122 && id.String() == value
}
