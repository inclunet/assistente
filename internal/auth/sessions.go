package auth

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"strings"
	"time"

	"assistente/internal/database"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

var (
	ErrInvalidRefreshToken = errors.New("refresh token inválido")
	ErrSessionExpired      = errors.New("sessão expirada")
	ErrSessionRevoked      = errors.New("sessão revogada")
)

// maxClientLabelLength é o limite duro para o clientLabel armazenado em
// `sessions.client_label`. O frontend (Wails) envia valores curtos
// derivados de hostname/UA, mas defesa em profundidade contra payloads
// adversários grandes (Mi4 do review da Fatia 1).
const maxClientLabelLength = 256

type SessionService struct {
	db            *gorm.DB
	signer        *TokenSigner
	issuer        string
	audience      string
	accessTTL     time.Duration
	refreshTTL    time.Duration
	refreshPepper []byte
	now           func() time.Time
}

type SessionConfig struct {
	Issuer     string
	Audience   string
	AccessTTL  time.Duration
	RefreshTTL time.Duration
	Signer     *TokenSigner
	// RefreshTokenPepper é a chave secreta usada para HMAC-SHA256 do
	// refresh secret antes de persistir em DB. Pode ser nil em testes —
	// nesse caso o serviço usa SHA-256 puro (modo legacy compatível).
	// Em produção, App carrega/cria via credentials.Manager
	// (InstanceSecretRefreshTokenPepper) e injeta aqui.
	RefreshTokenPepper []byte
}

type TokenPair struct {
	AccessToken           string    `json:"accessToken"`
	RefreshToken          string    `json:"refreshToken"`
	AccessTokenExpiresAt  time.Time `json:"accessTokenExpiresAt"`
	RefreshTokenExpiresAt time.Time `json:"refreshTokenExpiresAt"`
	SessionID             string    `json:"sessionId"`
}

func NewSessionService(db *gorm.DB, cfg SessionConfig) (*SessionService, error) {
	signer := cfg.Signer
	if signer == nil {
		var err error
		signer, err = NewTokenSigner()
		if err != nil {
			return nil, err
		}
	}
	if cfg.Issuer == "" {
		cfg.Issuer = "assistente"
	}
	if cfg.Audience == "" {
		cfg.Audience = "assistente-client"
	}
	if cfg.AccessTTL == 0 {
		cfg.AccessTTL = 15 * time.Minute
	}
	if cfg.RefreshTTL == 0 {
		cfg.RefreshTTL = 30 * 24 * time.Hour
	}

	return &SessionService{
		db:            db,
		signer:        signer,
		issuer:        cfg.Issuer,
		audience:      cfg.Audience,
		accessTTL:     cfg.AccessTTL,
		refreshTTL:    cfg.RefreshTTL,
		refreshPepper: append([]byte(nil), cfg.RefreshTokenPepper...),
		now:           time.Now,
	}, nil
}

func (s *SessionService) IssueSession(ctx context.Context, user *database.User, clientLabel string) (*TokenPair, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("session service não inicializado")
	}
	if user == nil || strings.TrimSpace(user.ID) == "" {
		return nil, errors.New("usuário obrigatório")
	}

	now := s.now()
	secret, err := newRefreshSecret()
	if err != nil {
		return nil, err
	}

	label := strings.TrimSpace(clientLabel)
	if len(label) > maxClientLabelLength {
		label = label[:maxClientLabelLength]
	}
	session := &database.Session{
		UserID:           user.ID,
		RefreshTokenHash: s.hashRefreshSecret(secret),
		ExpiresAt:        now.Add(s.refreshTTL),
		ClientLabel:      label,
	}
	if err := s.db.WithContext(ctx).Create(session).Error; err != nil {
		return nil, err
	}

	return s.tokenPairForSession(session, user.Role, secret, now)
}

func (s *SessionService) Refresh(ctx context.Context, refreshToken string) (*TokenPair, error) {
	return s.refresh(ctx, refreshToken, true)
}

// RefreshLocalCandidate refreshes a desktop-local candidate token without
// revoking the session on hash mismatch. The desktop app may have multiple
// local stores during migration (keyring and encrypted credential store), so a
// stale candidate must not invalidate the current candidate before it is tried.
func (s *SessionService) RefreshLocalCandidate(ctx context.Context, refreshToken string) (*TokenPair, error) {
	return s.refresh(ctx, refreshToken, false)
}

func (s *SessionService) refresh(ctx context.Context, refreshToken string, revokeOnMismatch bool) (*TokenPair, error) {
	sessionID, secret, err := parseRefreshToken(refreshToken)
	if err != nil {
		return nil, err
	}

	var session database.Session
	err = s.db.WithContext(ctx).First(&session, "id = ?", sessionID).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrInvalidRefreshToken
		}
		return nil, err
	}

	now := s.now()
	if session.RevokedAt != nil {
		return nil, ErrSessionRevoked
	}
	if !now.Before(session.ExpiresAt) {
		return nil, ErrSessionExpired
	}
	if !s.matchRefreshSecret(session.RefreshTokenHash, secret) {
		if revokeOnMismatch {
			revokedAt := now
			_ = s.db.WithContext(ctx).Model(&database.Session{}).Where("id = ?", session.ID).Update("revoked_at", revokedAt).Error
		}
		return nil, ErrInvalidRefreshToken
	}

	var user database.User
	if err := s.db.WithContext(ctx).First(&user, "id = ?", session.UserID).Error; err != nil {
		return nil, err
	}
	if !user.IsActive {
		return nil, ErrInactiveUser
	}

	nextSecret, err := newRefreshSecret()
	if err != nil {
		return nil, err
	}
	lastUsedAt := now
	updates := map[string]interface{}{
		"refresh_token_hash": s.hashRefreshSecret(nextSecret),
		"last_used_at":       lastUsedAt,
		"expires_at":         now.Add(s.refreshTTL),
	}
	if err := s.db.WithContext(ctx).Model(&database.Session{}).Where("id = ?", session.ID).Updates(updates).Error; err != nil {
		return nil, err
	}
	session.RefreshTokenHash = updates["refresh_token_hash"].(string)
	session.LastUsedAt = &lastUsedAt
	session.ExpiresAt = updates["expires_at"].(time.Time)

	return s.tokenPairForSession(&session, user.Role, nextSecret, now)
}

func (s *SessionService) Logout(ctx context.Context, refreshToken string) error {
	sessionID, _, err := parseRefreshToken(refreshToken)
	if err != nil {
		return err
	}

	now := s.now()
	return s.db.WithContext(ctx).Model(&database.Session{}).
		Where("id = ? AND revoked_at IS NULL", sessionID).
		Update("revoked_at", now).Error
}

// PurgeExpiredSessions remove permanentemente do DB sessions que estejam:
//   - expiradas há pelo menos `retention` (i.e., `expires_at < now - retention`); ou
//   - revogadas há pelo menos `retention` (i.e., `revoked_at < now - retention`).
//
// Sessions ainda dentro da janela de retenção são preservadas para que
// auditorias possam confirmar logout/expiração recentes. Retorna o total
// de linhas deletadas.
//
// Decisões (review do AEP-0052, Bloco 6, Mi38):
//
//   - **Retention configurável:** o caller decide (default sugerido: 30 dias).
//     `retention<=0` é tratado como "purga tudo que expirou ou revogou" sem
//     janela de carência (útil em testes ou jobs administrativos).
//   - **Hard delete:** não soft-delete; sessions são append-only e expiradas
//     são permanentemente inúteis (refresh_token_hash não é referenciado).
//   - **Index existente:** `Session.ExpiresAt` e `Session.RevokedAt` já são
//     indexados (tag GORM no model), então o WHERE é eficiente.
//   - **Sem batching:** SQLite single-process é tolerante a DELETE em massa
//     local. Em backends multi-tenant futuros, considerar `LIMIT` + loop.
func (s *SessionService) PurgeExpiredSessions(ctx context.Context, retention time.Duration) (int64, error) {
	if s == nil || s.db == nil {
		return 0, errors.New("session service não inicializado")
	}
	if retention < 0 {
		retention = 0
	}
	cutoff := s.now().Add(-retention)

	res := s.db.WithContext(ctx).
		Where("expires_at < ? OR (revoked_at IS NOT NULL AND revoked_at < ?)", cutoff, cutoff).
		Delete(&database.Session{})
	if res.Error != nil {
		return 0, res.Error
	}
	return res.RowsAffected, nil
}

func (s *SessionService) VerifyAccessToken(token string) (*AccessClaims, error) {
	return s.signer.VerifyAccessToken(token, s.issuer, s.audience, s.now(), time.Minute)
}

func (s *SessionService) JWKSet() JWKSet {
	return s.signer.JWKSet()
}

func (s *SessionService) tokenPairForSession(session *database.Session, role, secret string, now time.Time) (*TokenPair, error) {
	jti, err := uuid.NewV7()
	if err != nil {
		return nil, err
	}
	accessExpiresAt := now.Add(s.accessTTL)
	accessToken, err := s.signer.SignAccessToken(AccessClaims{
		Issuer:    s.issuer,
		Audience:  s.audience,
		Subject:   session.UserID,
		SessionID: session.ID,
		JTI:       jti.String(),
		IssuedAt:  now.Unix(),
		ExpiresAt: accessExpiresAt.Unix(),
		Role:      role,
	})
	if err != nil {
		return nil, err
	}

	return &TokenPair{
		AccessToken:           accessToken,
		RefreshToken:          formatRefreshToken(session.ID, secret),
		AccessTokenExpiresAt:  accessExpiresAt,
		RefreshTokenExpiresAt: session.ExpiresAt,
		SessionID:             session.ID,
	}, nil
}

func newRefreshSecret() (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

func formatRefreshToken(sessionID, secret string) string {
	return "v1." + sessionID + "." + secret
}

func parseRefreshToken(token string) (sessionID string, secret string, err error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 || parts[0] != "v1" || parts[1] == "" || parts[2] == "" {
		return "", "", ErrInvalidRefreshToken
	}
	return parts[1], parts[2], nil
}

// hashRefreshSecret produz o hash persistido em sessions.refresh_token_hash.
// Em produção usa HMAC-SHA256(pepper, secret) — defesa em camadas contra
// recovery por DB leak (B2 do review da Fatia 1). Em testes onde pepper
// é nil, fallback SHA-256 puro mantém compatibilidade.
func (s *SessionService) hashRefreshSecret(secret string) string {
	if len(s.refreshPepper) > 0 {
		mac := hmac.New(sha256.New, s.refreshPepper)
		mac.Write([]byte(secret))
		return "h1:" + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	}
	sum := sha256.Sum256([]byte(secret))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

// matchRefreshSecret verifica se `stored` (lido do DB) corresponde ao
// `secret` apresentado pelo cliente. Aceita BOTH:
//   - hashes legacy SHA-256 puro (instalações pré-pepper);
//   - hashes novos HMAC-SHA256 com pepper (prefixo "h1:").
//
// A migração é transparente: a sessão é re-hashada com o formato corrente
// no próximo refresh (ver `refresh()`), então o hash legacy desaparece
// naturalmente em até `RefreshTTL`. Compara em tempo constante para evitar
// timing attacks.
func (s *SessionService) matchRefreshSecret(stored, secret string) bool {
	if strings.HasPrefix(stored, "h1:") {
		if len(s.refreshPepper) == 0 {
			return false
		}
		mac := hmac.New(sha256.New, s.refreshPepper)
		mac.Write([]byte(secret))
		expected := "h1:" + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
		return subtle.ConstantTimeCompare([]byte(stored), []byte(expected)) == 1
	}
	sum := sha256.Sum256([]byte(secret))
	expected := base64.RawURLEncoding.EncodeToString(sum[:])
	return subtle.ConstantTimeCompare([]byte(stored), []byte(expected)) == 1
}
