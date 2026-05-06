package app

import (
	"context"
	"errors"
	"strings"

	"assistente/internal/auth"
	"assistente/internal/credentials"
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
	a.configureSessionService()
	a.vaultSvc = auth.NewVaultService(a.credStore, func(dek []byte) {
		a.configureCredentialManager(dek, true)
		a.configureSessionService()
	})
}

func (a *App) configureSessionService() {
	signer, err := auth.LoadOrCreateTokenSigner(a.credMgr)
	if err != nil {
		a.sessionSvc = nil
		return
	}
	sessionSvc, err := auth.NewSessionService(database.DB(), auth.SessionConfig{Signer: signer})
	if err != nil {
		a.sessionSvc = nil
		return
	}
	a.sessionSvc = sessionSvc
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
	pair, err := a.sessionSvc.IssueSession(context.Background(), user, req.ClientLabel)
	if err != nil {
		return nil, err
	}
	a.setCurrentUserID(user.ID)
	a.reloadUserScopedRuntime()
	return pair, nil
}

func (a *App) RefreshAuth(req RefreshRequest) (*auth.TokenPair, error) {
	if err := a.ensureAuthServices(); err != nil {
		return nil, err
	}
	pair, err := a.sessionSvc.Refresh(context.Background(), req.RefreshToken)
	if err != nil {
		return nil, err
	}
	claims, err := a.sessionSvc.VerifyAccessToken(pair.AccessToken)
	if err == nil {
		a.setCurrentUserID(claims.Subject)
		a.reloadUserScopedRuntime()
	}
	return pair, nil
}

func (a *App) Logout(req LogoutRequest) error {
	if err := a.ensureAuthServices(); err != nil {
		return err
	}
	err := a.sessionSvc.Logout(context.Background(), req.RefreshToken)
	a.setCurrentUserID("")
	if a.llmRegistry != nil {
		a.llmRegistry.Clear()
	}
	return err
}

func (a *App) LoadAuthRefreshToken() (string, error) {
	token, err := credentials.LoadAuthRefreshTokenFromKeychain()
	if err != nil {
		if credentials.IsKeychainNotFound(err) {
			return "", nil
		}
		return "", err
	}
	return token, nil
}

func (a *App) StoreAuthRefreshToken(refreshToken string) error {
	return credentials.SaveAuthRefreshTokenToKeychain(strings.TrimSpace(refreshToken))
}

func (a *App) ClearAuthRefreshToken() error {
	err := credentials.DeleteAuthRefreshTokenFromKeychain()
	if err != nil && credentials.IsKeychainNotFound(err) {
		return nil
	}
	return err
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

func (a *App) setCurrentUserID(userID string) {
	a.authMu.Lock()
	defer a.authMu.Unlock()
	a.currentUserID = strings.TrimSpace(userID)
}

func (a *App) authenticatedContext() context.Context {
	ctx := context.Background()
	if a != nil && a.ctx != nil {
		ctx = a.ctx
	}
	a.authMu.RLock()
	defer a.authMu.RUnlock()
	if a.currentUserID == "" {
		return ctx
	}
	return database.WithUserID(ctx, a.currentUserID)
}

func (a *App) reloadUserScopedRuntime() {
	if a.llmRegistry != nil {
		a.llmRegistry.Clear()
	}
	a.registerEnvCredentials(a.authenticatedContext(), a.credMgr)
	a.migrateLegacyConfig()
	a.initLLMProviders()
	a.initLLMClient()
}

func (a *App) ensureAuthServices() error {
	if a.identitySvc == nil || a.sessionSvc == nil || a.vaultSvc == nil {
		return errors.New("serviços de autenticação não inicializados")
	}
	return nil
}
