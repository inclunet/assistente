package auth

import (
	"context"
	"errors"
	"time"

	"assistente/internal/database"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

var ErrUnauthenticatedLocalSession = errors.New("sessão local não autenticada")

var ErrActiveUserNotFound = errors.New("usuário local ativo não encontrado")

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
	return s.revalidateLocalSession(ctx, LocalSessionPrincipal{UserID: claims.Subject, SessionID: claims.SessionID}, time.Time{}, claims.ExpiresAt)
}

// RevalidateLocalSession reconsulta uma identidade local capturada pelo
// backend, sem aceitar token, role ou qualquer dado vindo de bindings Wails.
// O chamador deve obter o principal de uma sessão desktop já autenticada; este
// método apenas confirma que a sessão e a conta continuam autorizadas.
func (s *SessionService) RevalidateLocalSession(ctx context.Context, principal LocalSessionPrincipal) (LocalSessionPrincipal, error) {
	if s == nil || s.db == nil || s.now == nil || ctx == nil ||
		!canonicalSessionUUID(principal.UserID) || !canonicalSessionUUID(principal.SessionID) {
		return LocalSessionPrincipal{}, ErrUnauthenticatedLocalSession
	}
	if err := ctx.Err(); err != nil {
		return LocalSessionPrincipal{}, err
	}
	before := s.now()
	if before.IsZero() {
		return LocalSessionPrincipal{}, ErrUnauthenticatedLocalSession
	}
	return s.revalidateLocalSession(ctx, principal, before, 0)
}

func (s *SessionService) revalidateLocalSession(ctx context.Context, principal LocalSessionPrincipal, before time.Time, jwtExpiresAt int64) (LocalSessionPrincipal, error) {
	validated, err := s.revalidateLocalSessionDB(ctx, s.db, principal, before, jwtExpiresAt)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return LocalSessionPrincipal{}, err
		}
		return LocalSessionPrincipal{}, ErrUnauthenticatedLocalSession
	}
	return validated, nil
}

// RevalidateLocalSessionTx revalida a sessão usando exclusivamente a
// transação recebida. O consumidor de comandos já está dentro dessa
// transação; consultar s.db aqui poderia tentar obter outra conexão e
// bloquear quando o pool tem capacidade um.
func (s *SessionService) RevalidateLocalSessionTx(ctx context.Context, tx *gorm.DB, principal LocalSessionPrincipal) (LocalSessionPrincipal, error) {
	if s == nil || s.db == nil || tx == nil || s.now == nil || ctx == nil ||
		!canonicalSessionUUID(principal.UserID) || !canonicalSessionUUID(principal.SessionID) {
		return LocalSessionPrincipal{}, ErrUnauthenticatedLocalSession
	}
	if err := ctx.Err(); err != nil {
		return LocalSessionPrincipal{}, err
	}
	rootDB, err := s.db.DB()
	if err != nil {
		return LocalSessionPrincipal{}, err
	}
	txDB, err := tx.DB()
	if err != nil {
		return LocalSessionPrincipal{}, err
	}
	if rootDB == nil || txDB == nil || rootDB != txDB {
		return LocalSessionPrincipal{}, ErrUnauthenticatedLocalSession
	}
	before := s.now()
	if before.IsZero() {
		return LocalSessionPrincipal{}, ErrUnauthenticatedLocalSession
	}
	validated, err := s.revalidateLocalSessionDB(ctx, tx, principal, before, 0)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return LocalSessionPrincipal{}, ErrUnauthenticatedLocalSession
	}
	if err != nil {
		return LocalSessionPrincipal{}, err
	}
	return validated, nil
}

func (s *SessionService) revalidateLocalSessionDB(ctx context.Context, db *gorm.DB, principal LocalSessionPrincipal, before time.Time, jwtExpiresAt int64) (LocalSessionPrincipal, error) {
	var row struct {
		UserID    string
		SessionID string
		ExpiresAt time.Time
	}
	// Uma única leitura relaciona a sessão assinada à conta ainda ativa. Não
	// consulta por ID recebido separadamente nem usa role antiga contida no JWT.
	result := db.WithContext(ctx).Table("sessions AS s").Select("s.user_id, s.id AS session_id, s.expires_at").Joins("JOIN users AS u ON u.id = s.user_id").Where("s.id = ? AND s.user_id = ? AND s.revoked_at IS NULL AND u.is_active = ?", principal.SessionID, principal.UserID, true).Take(&row)
	if err := ctx.Err(); err != nil {
		return LocalSessionPrincipal{}, err
	}
	if result.Error != nil {
		return LocalSessionPrincipal{}, result.Error
	}
	now := s.now()
	if now.IsZero() || !row.ExpiresAt.After(now) || (!before.IsZero() && !row.ExpiresAt.After(before)) ||
		(jwtExpiresAt != 0 && !time.Unix(jwtExpiresAt, 0).After(now)) {
		return LocalSessionPrincipal{}, ErrUnauthenticatedLocalSession
	}
	return LocalSessionPrincipal{UserID: row.UserID, SessionID: row.SessionID}, nil
}

// ActiveUserRole relê o role atual da conta depois de autenticar a sessão.
// Claims antigas nunca são usadas como autorização de comando.
func (s *SessionService) ActiveUserRole(ctx context.Context, userID string) (string, error) {
	if s == nil || s.db == nil || ctx == nil || !canonicalSessionUUID(userID) {
		return "", ErrActiveUserNotFound
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	var user database.User
	if err := s.db.WithContext(ctx).Where("id = ? AND is_active = ?", userID, true).First(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", ErrActiveUserNotFound
		}
		return "", err
	}
	return user.Role, nil
}

func canonicalSessionUUID(value string) bool {
	id, err := uuid.Parse(value)
	return err == nil && id.Version() == 7 && id.Variant() == uuid.RFC4122 && id.String() == value
}
