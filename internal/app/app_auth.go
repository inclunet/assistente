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
		a.authMu.Lock()
		a.sessionSvc = nil
		a.authMu.Unlock()
		return
	}
	sessionSvc, err := auth.NewSessionService(database.DB(), auth.SessionConfig{Signer: signer})
	if err != nil {
		a.authMu.Lock()
		a.sessionSvc = nil
		a.authMu.Unlock()
		return
	}
	a.authMu.Lock()
	a.sessionSvc = sessionSvc
	a.authMu.Unlock()
}

func (a *App) GetAuthStatus() (AuthStatus, error) {
	if err := a.ensureAuthCoreServices(); err != nil {
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
	if err := a.ensureAuthCoreServices(); err != nil {
		return "", err
	}
	return a.vaultSvc.Setup(context.Background(), masterPassword)
}

func (a *App) UnlockVault(kind, secret string) error {
	if err := a.ensureAuthCoreServices(); err != nil {
		return err
	}
	return a.vaultSvc.Unlock(context.Background(), kind, secret)
}

func (a *App) CreateAdminUser(req CreateAdminRequest) (*database.User, error) {
	if err := a.ensureAuthCoreServices(); err != nil {
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

func (a *App) Login(req LoginRequest) (*AuthUser, error) {
	a.authSessionMu.Lock()
	defer a.authSessionMu.Unlock()

	if err := a.ensureAuthCoreServices(); err != nil {
		return nil, err
	}
	if err := a.ensureSessionService(); err != nil {
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
	if err := a.storeAuthRefreshToken(pair.RefreshToken); err != nil {
		return nil, err
	}
	claims, err := a.sessionSvc.VerifyAccessToken(pair.AccessToken)
	if err != nil {
		return nil, err
	}
	a.setCurrentUserID(claims.Subject)
	a.setCurrentAuthUser(&AuthUser{UserID: claims.Subject, SessionID: claims.SessionID, Role: claims.Role})
	if err := a.adoptLegacyDataForUser(claims.Subject); err != nil {
		return nil, err
	}
	a.reloadUserScopedRuntime()
	return a.GetAuthUser()
}

func (a *App) RefreshAuth(req RefreshRequest) (*AuthUser, error) {
	a.authSessionMu.Lock()
	defer a.authSessionMu.Unlock()

	if err := a.ensureAuthCoreServices(); err != nil {
		return nil, err
	}
	if err := a.ensureSessionService(); err != nil {
		return nil, err
	}

	candidates := a.loadAuthRefreshTokenCandidates()
	if reqToken := strings.TrimSpace(req.RefreshToken); reqToken != "" {
		candidates = appendUniqueToken(candidates, reqToken)
	}
	if len(candidates) == 0 {
		return nil, auth.ErrInvalidRefreshToken
	}

	var pair *auth.TokenPair
	var err error
	for _, refreshToken := range candidates {
		pair, err = a.sessionSvc.RefreshLocalCandidate(context.Background(), refreshToken)
		if err == nil {
			break
		}
	}
	if pair == nil {
		_ = a.clearAuthRefreshToken()
		return nil, err
	}
	if err := a.storeAuthRefreshToken(pair.RefreshToken); err != nil {
		return nil, err
	}
	claims, err := a.sessionSvc.VerifyAccessToken(pair.AccessToken)
	if err == nil {
		a.setCurrentUserID(claims.Subject)
		a.setCurrentAuthUser(&AuthUser{UserID: claims.Subject, SessionID: claims.SessionID, Role: claims.Role})
		if err := a.adoptLegacyDataForUser(claims.Subject); err != nil {
			return nil, err
		}
		a.reloadUserScopedRuntime()
	}
	return a.GetAuthUser()
}

func appendUniqueToken(tokens []string, token string) []string {
	token = strings.TrimSpace(token)
	if token == "" {
		return tokens
	}
	for _, existing := range tokens {
		if existing == token {
			return tokens
		}
	}
	return append(tokens, token)
}

func (a *App) loadAuthRefreshTokenCandidates() []string {
	tokens := make([]string, 0, 3)
	if token, err := a.loadAuthRefreshTokenFromKeychain(); err == nil {
		tokens = appendUniqueToken(tokens, token)
	}
	if a.credMgr != nil {
		token, ok, err := a.credMgr.GetInstanceSecret(credentials.InstanceSecretAuthRefreshToken)
		if err == nil && ok {
			tokens = appendUniqueToken(tokens, token)
		}
	}
	return tokens
}

func (a *App) Logout(req LogoutRequest) error {
	a.authSessionMu.Lock()
	defer a.authSessionMu.Unlock()

	if err := a.ensureAuthCoreServices(); err != nil {
		return err
	}
	refreshToken := strings.TrimSpace(req.RefreshToken)
	if refreshToken == "" {
		if stored, ok, loadErr := a.loadAuthRefreshToken(); loadErr == nil && ok {
			refreshToken = stored
		}
	}
	var err error
	if refreshToken != "" && a.ensureSessionService() == nil {
		err = a.sessionSvc.Logout(context.Background(), refreshToken)
	}
	_ = a.clearAuthRefreshToken()
	a.setCurrentUserID("")
	a.setCurrentAuthUser(nil)
	if a.llmRegistry != nil {
		a.llmRegistry.Clear()
	}
	return err
}

func (a *App) loadAuthRefreshToken() (string, bool, error) {
	if a.credMgr != nil {
		token, ok, err := a.credMgr.GetInstanceSecret(credentials.InstanceSecretAuthRefreshToken)
		if err == nil && ok {
			return token, true, nil
		}
	}
	token, err := a.loadAuthRefreshTokenFromKeychain()
	if err != nil {
		if credentials.IsKeychainNotFound(err) {
			return "", false, nil
		}
		return "", false, err
	}
	if strings.TrimSpace(token) == "" {
		return "", false, nil
	}
	if a.credMgr != nil {
		_ = a.credMgr.RegisterInstanceSecret(credentials.InstanceSecretAuthRefreshToken, token)
	}
	return token, true, nil
}

func (a *App) storeAuthRefreshToken(refreshToken string) error {
	refreshToken = strings.TrimSpace(refreshToken)
	if refreshToken == "" {
		return auth.ErrInvalidRefreshToken
	}
	if a.credMgr != nil {
		if err := a.credMgr.RegisterInstanceSecret(credentials.InstanceSecretAuthRefreshToken, refreshToken); err != nil {
			return err
		}
	}
	_ = a.saveAuthRefreshTokenToKeychain(refreshToken)
	return nil
}

func (a *App) clearAuthRefreshToken() error {
	var err error
	if a.credMgr != nil {
		err = a.credMgr.DeleteInstanceSecret(credentials.InstanceSecretAuthRefreshToken)
	}
	keyringErr := a.deleteAuthRefreshTokenFromKeychain()
	if keyringErr != nil && !credentials.IsKeychainNotFound(keyringErr) && err == nil {
		err = keyringErr
	}
	return err
}

func (a *App) loadAuthRefreshTokenFromKeychain() (string, error) {
	if a != nil && a.authKeyringLoad != nil {
		return a.authKeyringLoad()
	}
	return credentials.LoadAuthRefreshTokenFromKeychain()
}

func (a *App) saveAuthRefreshTokenToKeychain(refreshToken string) error {
	if a != nil && a.authKeyringSave != nil {
		return a.authKeyringSave(refreshToken)
	}
	return credentials.SaveAuthRefreshTokenToKeychain(refreshToken)
}

func (a *App) deleteAuthRefreshTokenFromKeychain() error {
	if a != nil && a.authKeyringDelete != nil {
		return a.authKeyringDelete()
	}
	return credentials.DeleteAuthRefreshTokenFromKeychain()
}

func (a *App) GetAuthUser() (*AuthUser, error) {
	if err := a.ensureAuthCoreServices(); err != nil {
		return nil, err
	}
	if err := a.ensureSessionService(); err != nil {
		return nil, err
	}
	a.authMu.RLock()
	defer a.authMu.RUnlock()
	if a.currentAuthUser == nil {
		return nil, auth.ErrInvalidRefreshToken
	}
	user := *a.currentAuthUser
	return &user, nil
}

func (a *App) setCurrentUserID(userID string) {
	a.authMu.Lock()
	defer a.authMu.Unlock()
	a.currentUserID = strings.TrimSpace(userID)
}

func (a *App) setCurrentAuthUser(user *AuthUser) {
	a.authMu.Lock()
	defer a.authMu.Unlock()
	if user == nil {
		a.currentAuthUser = nil
		return
	}
	copy := *user
	a.currentAuthUser = &copy
}

func (a *App) adoptLegacyDataForUser(userID string) error {
	if err := database.AdoptLegacyData(userID); err != nil {
		return err
	}
	if a.credMgr != nil {
		return a.credMgr.LoadFromStore(context.Background())
	}
	return nil
}

// authenticatedContext retorna um context.Context com o userID atual injetado
// (quando há sessão ativa). Quando NÃO há sessão, retorna o ctx base sem userID.
//
// AVISO: NÃO use em bindings Wails ou em qualquer ponto de entrada que processe
// dados de usuário. Use requireAuthenticatedContext() — ele falha-fechado quando
// não há login. authenticatedContext() existe apenas para inicializações internas
// que precisam tolerar a ausência de userID (ex.: registerEnvCredentials, que
// já guarda explicitamente com UserIDFromContext, e fluxos de bootstrap antes
// do primeiro login).
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

// requireAuthenticatedContext retorna o context com userID e um erro
// (ErrUserScopeRequired) quando não há sessão autenticada. É a função correta
// para qualquer binding Wails / handler HTTP / API pública que toque dados
// do usuário.
func (a *App) requireAuthenticatedContext() (context.Context, error) {
	ctx := a.authenticatedContext()
	if _, err := database.RequireUserID(ctx); err != nil {
		return nil, err
	}
	return ctx, nil
}

func (a *App) reloadUserScopedRuntime() {
	if a.llmRegistry != nil {
		a.llmRegistry.Clear()
	}
	a.registerEnvCredentials(a.authenticatedContext(), a.credMgr)
	a.migrateLegacyConfig()
	if a.providerSvc != nil {
		a.initLLMProviders()
	}
	if a.profileManager != nil && a.llmRegistry != nil {
		a.initLLMClient()
	}
}

func (a *App) ensureAuthCoreServices() error {
	if a.identitySvc == nil || a.vaultSvc == nil {
		return errors.New("serviços de autenticação não inicializados")
	}
	return nil
}

func (a *App) ensureSessionService() error {
	a.authMu.RLock()
	if a.sessionSvc != nil {
		a.authMu.RUnlock()
		return nil
	}
	a.authMu.RUnlock()
	a.configureSessionService()
	a.authMu.RLock()
	defer a.authMu.RUnlock()
	if a.sessionSvc == nil {
		return errors.New("sessão de autenticação indisponível: desbloqueie o cofre ou verifique a DEK")
	}
	return nil
}

func (a *App) currentSessionService() *auth.SessionService {
	a.authMu.RLock()
	defer a.authMu.RUnlock()
	return a.sessionSvc
}
