package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
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

type SessionService struct {
	db         *gorm.DB
	signer     *TokenSigner
	issuer     string
	audience   string
	accessTTL  time.Duration
	refreshTTL time.Duration
	now        func() time.Time
}

type SessionConfig struct {
	Issuer     string
	Audience   string
	AccessTTL  time.Duration
	RefreshTTL time.Duration
	Signer     *TokenSigner
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
		db:         db,
		signer:     signer,
		issuer:     cfg.Issuer,
		audience:   cfg.Audience,
		accessTTL:  cfg.AccessTTL,
		refreshTTL: cfg.RefreshTTL,
		now:        time.Now,
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

	session := &database.Session{
		UserID:           user.ID,
		RefreshTokenHash: hashRefreshSecret(secret),
		ExpiresAt:        now.Add(s.refreshTTL),
		ClientLabel:      strings.TrimSpace(clientLabel),
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
	if session.RefreshTokenHash != hashRefreshSecret(secret) {
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
		"refresh_token_hash": hashRefreshSecret(nextSecret),
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

func hashRefreshSecret(secret string) string {
	sum := sha256.Sum256([]byte(secret))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}
