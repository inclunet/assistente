package app

import (
	"context"
	"errors"
	"strings"

	"assistente/internal/auth"
	"assistente/internal/database"
)

type AuthStatus struct {
	VaultConfigured bool `json:"vaultConfigured"`
	VaultUnlocked   bool `json:"vaultUnlocked"`
	HasUsers        bool `json:"hasUsers"`
}

type CreateAdminRequest struct {
	Username    string `json:"username"`
	DisplayName string `json:"displayName"`
	Password    string `json:"password"`
}

type LoginRequest struct {
	Username    string `json:"username"`
	Password    string `json:"password"`
	ClientLabel string `json:"clientLabel"`
}

type RefreshRequest struct {
	RefreshToken string `json:"refreshToken"`
}

type LogoutRequest struct {
	RefreshToken string `json:"refreshToken"`
}

type AuthUser struct {
	UserID    string `json:"userId"`
	SessionID string `json:"sessionId"`
	Role      string `json:"role"`
}

func (a *App) initAuthServices() {
	a.identitySvc = auth.NewIdentityService(database.DB())
	sessionSvc, err := auth.NewSessionService(database.DB(), auth.SessionConfig{})
	if err != nil {
		// Ed25519 key generation should not fail in normal operation. Keep nil so
		// API calls return a clear initialization error if the OS RNG failed.
		a.sessionSvc = nil
	} else {
		a.sessionSvc = sessionSvc
	}
	a.vaultSvc = auth.NewVaultService(a.credStore, func(dek []byte) {
		a.configureCredentialManager(dek, true)
	})
}

func (a *App) GetAuthStatus() (AuthStatus, error) {
	if err := a.ensureAuthServices(); err != nil {
		return AuthStatus{}, err
	}

	vaultStatus, err := a.vaultSvc.Status(context.Background())
	if err != nil {
		return AuthStatus{}, err
	}

	var userCount int64
	if err := database.DB().Model(&database.User{}).Count(&userCount).Error; err != nil {
		return AuthStatus{}, err
	}

	return AuthStatus{
		VaultConfigured: vaultStatus.Configured,
		VaultUnlocked:   vaultStatus.Unlocked,
		HasUsers:        userCount > 0,
	}, nil
}

func (a *App) SetupVault(masterPassword string) (string, error) {
	if err := a.ensureAuthServices(); err != nil {
		return "", err
	}
	return a.vaultSvc.Setup(context.Background(), masterPassword)
}

func (a *App) UnlockVault(kind, secret string) error {
	if err := a.ensureAuthServices(); err != nil {
		return err
	}
	return a.vaultSvc.Unlock(context.Background(), kind, secret)
}

func (a *App) CreateAdminUser(req CreateAdminRequest) (*database.User, error) {
	if err := a.ensureAuthServices(); err != nil {
		return nil, err
	}

	var userCount int64
	if err := database.DB().Model(&database.User{}).Count(&userCount).Error; err != nil {
		return nil, err
	}
	if userCount > 0 {
		return nil, errors.New("admin inicial já foi criado")
	}

	user, err := a.identitySvc.CreateLocalUser(context.Background(), auth.CreateUserParams{
		Username:    req.Username,
		DisplayName: req.DisplayName,
		Password:    req.Password,
		Admin:       true,
	})
	if err != nil {
		return nil, err
	}
	if err := database.AdoptLegacyData(user.ID); err != nil {
		return nil, err
	}
	return user, nil
}

func (a *App) Login(req LoginRequest) (*auth.TokenPair, error) {
	if err := a.ensureAuthServices(); err != nil {
		return nil, err
	}

	user, err := a.identitySvc.AuthenticateLocal(context.Background(), req.Username, req.Password)
	if err != nil {
		return nil, err
	}
	return a.sessionSvc.IssueSession(context.Background(), user, req.ClientLabel)
}

func (a *App) RefreshAuth(req RefreshRequest) (*auth.TokenPair, error) {
	if err := a.ensureAuthServices(); err != nil {
		return nil, err
	}
	return a.sessionSvc.Refresh(context.Background(), req.RefreshToken)
}

func (a *App) Logout(req LogoutRequest) error {
	if err := a.ensureAuthServices(); err != nil {
		return err
	}
	return a.sessionSvc.Logout(context.Background(), req.RefreshToken)
}

func (a *App) GetAuthUser(accessToken string) (*AuthUser, error) {
	if err := a.ensureAuthServices(); err != nil {
		return nil, err
	}
	claims, err := a.sessionSvc.VerifyAccessToken(strings.TrimSpace(accessToken))
	if err != nil {
		return nil, err
	}
	return &AuthUser{
		UserID:    claims.Subject,
		SessionID: claims.SessionID,
		Role:      claims.Role,
	}, nil
}

func (a *App) ensureAuthServices() error {
	if a.identitySvc == nil || a.sessionSvc == nil || a.vaultSvc == nil {
		return errors.New("serviços de autenticação não inicializados")
	}
	return nil
}
