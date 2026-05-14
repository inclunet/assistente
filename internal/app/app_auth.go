package app

import (
	"context"
	"errors"
	"log"
	"strings"
	"time"

	"assistente/internal/auth"
	"assistente/internal/channels"
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
	pepper, err := auth.LoadOrCreateRefreshTokenPepper(a.credMgr)
	if err != nil {
		log.Printf("[Auth] erro ao carregar pepper de refresh token: %v", err)
		a.authMu.Lock()
		a.sessionSvc = nil
		a.authMu.Unlock()
		return
	}
	sessionSvc, err := auth.NewSessionService(database.DB(), auth.SessionConfig{
		Signer:             signer,
		RefreshTokenPepper: pepper,
	})
	if err != nil {
		a.authMu.Lock()
		a.sessionSvc = nil
		a.authMu.Unlock()
		return
	}
	a.authMu.Lock()
	a.sessionSvc = sessionSvc
	a.authMu.Unlock()

	if deleted, err := sessionSvc.PurgeExpiredSessions(context.Background(), sessionPurgeRetention); err != nil {
		log.Printf("[Auth] purge de sessions expiradas/revogadas falhou: %v", err)
	} else if deleted > 0 {
		log.Printf("[Auth] %d sessions expiradas/revogadas removidas (retention %s)", deleted, sessionPurgeRetention)
	}
}

// sessionPurgeRetention é a janela em que sessions expiradas/revogadas
// permanecem persistidas para auditoria antes de serem hard-deleted no
// próximo boot. 30 dias é suficiente para suportar investigação de
// logout/expiração recentes sem inflar o DB indefinidamente.
const sessionPurgeRetention = 30 * 24 * time.Hour

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
	// B10 (AEP-0052): adoção de canais legados acontece APENAS aqui,
	// no fluxo de criação do primeiro admin. NÃO em Login/RefreshAuth.
	// Antes, o primeiro usuário a logar herdava todos os canais sem
	// dono — em multi-user isso quebrava: o segundo usuário (migrado
	// de outra instância) perdia acesso aos próprios canais. Agora o
	// admin inicial absorve os legados, e usuários adicionais resolvem
	// canais órfãos por fluxo explícito de re-saving (ver
	// app_messaging.go.SaveChannelConfig).
	migrated, adoptErr := channels.AdoptOrphans(user.ID)
	if len(migrated) > 0 {
		log.Printf("[CreateAdminUser] %d canal(is) legado(s) reatribuído(s) ao admin %s: %v", len(migrated), user.ID, migrated)
	}
	if adoptErr != nil {
		// Best-effort: o admin foi criado e é o único que pode adotar
		// canais sem dono — propagamos o erro só em log para não
		// bloquear setup. Canais que ficaram sem dono podem ser
		// reativados manualmente pelas settings.
		log.Printf("[CreateAdminUser] erro best-effort em channels.AdoptOrphans: %v", adoptErr)
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
		// Sessão já existe no DB mas não conseguimos persistir o token
		// localmente — revoga para não deixar sessão "fantasma".
		_ = a.sessionSvc.Logout(context.Background(), pair.RefreshToken)
		return nil, err
	}
	claims, err := a.sessionSvc.VerifyAccessToken(pair.AccessToken)
	if err != nil {
		_ = a.sessionSvc.Logout(context.Background(), pair.RefreshToken)
		_ = a.clearAuthRefreshToken()
		return nil, err
	}
	a.stopUserScopedRuntime()
	a.setCurrentUserID(claims.Subject)
	a.setCurrentAuthUser(&AuthUser{UserID: claims.Subject, SessionID: claims.SessionID, Role: claims.Role})
	if err := a.adoptLegacyDataForUser(claims.Subject); err != nil {
		// B5: failure aqui ANTES era silencioso para o backend — o
		// frontend recebia o erro mas a sessão continuava ativa no DB,
		// o token no keychain, e currentUserID setado. O próximo
		// RefreshAuth funcionaria, criando a impressão de que o Login
		// "funcionou-mas-não-funcionou". Agora reverte estado completo:
		// sessão revogada, token apagado, memória limpa.
		a.rollbackLoginState(pair.RefreshToken)
		return nil, err
	}
	a.reloadUserScopedRuntime()
	return a.GetAuthUser()
}

// logLogoutError distingue os 3 cenários relevantes de falha em
// `sessionSvc.Logout` para incident response (P1-4 do re-review do
// PR #94). Antes desta separação, qualquer erro caía na mesma linha
// genérica e era impossível diferenciar "token mal formatado" de
// "DB indisponível" só pelos logs.
func (a *App) logLogoutError(err error) {
	switch {
	case errors.Is(err, auth.ErrInvalidRefreshToken):
		log.Printf("[Auth] Logout: refresh token mal formatado (possível tampering ou sessão local corrompida): %v", err)
	default:
		log.Printf("[Auth] Logout: erro ao revogar sessão remota (estado local já foi limpo, sessão remota expira em RefreshTTL): %v", err)
	}
}

// rollbackLogoutTimeout é o tempo máximo aceitável para revogar uma
// sessão durante rollback de login. Em DB saudável o UPDATE responde em
// <50ms; 2s é um teto generoso para covers de variação. Acima disso é
// problema maior (DB sob lock global, deadlock, IO travado) e não vale
// segurar o usuário no caminho de erro: o estado local já foi limpo,
// e a sessão remota expira pelo TTL natural mesmo se o Update não rodar
// (P1-2 do re-review do PR #94).
const rollbackLogoutTimeout = 2 * time.Second

// rollbackLoginState desfaz o estado parcial deixado por um Login
// que iniciou bem mas falhou em uma etapa pós-IssueSession. Idempotente
// e melhor-esforço: cada limpeza é tentada independentemente.
func (a *App) rollbackLoginState(refreshToken string) {
	if a.sessionSvc != nil && refreshToken != "" {
		ctx, cancel := context.WithTimeout(context.Background(), rollbackLogoutTimeout)
		err := a.sessionSvc.Logout(ctx, refreshToken)
		cancel()
		if err != nil {
			log.Printf("[Auth] rollback: erro ao revogar sessão (ctx<=%s): %v", rollbackLogoutTimeout, err)
		}
	}
	if err := a.clearAuthRefreshToken(); err != nil {
		log.Printf("[Auth] rollback: erro ao apagar refresh token local: %v", err)
	}
	if a.jobMgr != nil {
		a.jobMgr.Stop()
	}
	a.setCurrentUserID("")
	a.setCurrentAuthUser(nil)
	if a.llmRegistry != nil {
		a.llmRegistry.Clear()
	}
	if a.mcpMgr != nil {
		a.mcpMgr.DisconnectAll()
	}
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
		_ = a.sessionSvc.Logout(context.Background(), pair.RefreshToken)
		return nil, err
	}
	claims, err := a.sessionSvc.VerifyAccessToken(pair.AccessToken)
	if err != nil {
		// Refresh produziu um access token que não verifica — situação
		// muito anormal (signer mudou mid-flight?). Reverte.
		a.rollbackLoginState(pair.RefreshToken)
		return nil, err
	}
	a.stopUserScopedRuntime()
	a.setCurrentUserID(claims.Subject)
	a.setCurrentAuthUser(&AuthUser{UserID: claims.Subject, SessionID: claims.SessionID, Role: claims.Role})
	if err := a.adoptLegacyDataForUser(claims.Subject); err != nil {
		// Mesmo padrão do Login: rollback completo se a adoção falhar.
		a.rollbackLoginState(pair.RefreshToken)
		return nil, err
	}
	a.reloadUserScopedRuntime()
	return a.GetAuthUser()
}

// appendUniqueToken adiciona `token` em `tokens` se ainda não estiver
// presente, ignorando trim de espaços. É O(n²) por design — N é
// tipicamente <=3 candidates por Refresh (request body + keychain +
// fallback do credMgr) e a economia de uma struct/map nessa hot path
// não compensa o overhead. Se a lista crescer (>10), migrar para
// `map[string]struct{}` (P2-3 do re-review do PR #94).
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

// Logout é sempre best-effort do ponto de vista do caller (M6 do review
// da Fatia 1). Tentamos revogar a sessão remota e limpar todo estado
// local; falhas no revoke remoto são logadas mas NÃO retornam erro,
// porque o usuário fez "logout" e o estado local já foi limpo —
// retornar erro confunde a UI sem nenhum ganho prático (a sessão
// remota expira sozinha em até RefreshTTL). Isso elimina o cenário
// inconsistente onde local-clean + erro retornado fazia o frontend
// pensar que o logout falhou enquanto, do ponto de vista do app, ele
// já tinha completado.
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
	if refreshToken != "" && a.ensureSessionService() == nil {
		ctx, cancel := context.WithTimeout(context.Background(), rollbackLogoutTimeout)
		err := a.sessionSvc.Logout(ctx, refreshToken)
		cancel()
		if err != nil {
			a.logLogoutError(err)
		}
	}
	if a.jobMgr != nil {
		a.jobMgr.Stop()
	}
	_ = a.clearAuthRefreshToken()
	a.setCurrentUserID("")
	a.setCurrentAuthUser(nil)
	if a.llmRegistry != nil {
		a.llmRegistry.Clear()
	}
	if a.mcpMgr != nil {
		// Solta as conexões MCP do user que está saindo. As credenciais
		// `mcp-tokens:*` desse user não estão mais válidas neste
		// processo (o vault vai ser locked logo abaixo); deixar as
		// conexões abertas é (a) leak de file descriptors, (b) risco
		// de que requests pendentes vazem para o user seguinte.
		a.mcpMgr.DisconnectAll()
	}
	if a.vaultSvc != nil {
		a.vaultSvc.Lock()
	}
	return nil
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

// adoptLegacyDataForUser executa as migrações pré-AEP-0052 para o
// usuário recém-autenticado:
//
//  1. database.AdoptLegacyData: atribui registros órfãos (user_id="")
//     do banco ao userID. Falha aqui é HARD: se não conseguimos escrever
//     no DB, qualquer operação subsequente vai falhar de qualquer jeito,
//     então propaga.
//  2. credMgr.LoadUserCredentials: hidrata o cache em memória do
//     Manager com as credenciais do user. Sem esse passo,
//     ResolveForURLWithContext não acha as credenciais user-scoped
//     mesmo com elas gravadas no DB, e todo request HTTP do user
//     bate em `unresolvedCredentialError`.
//
// NOTA (B10 / AEP-0052): channels.AdoptOrphans NÃO é chamado aqui.
// Foi movido para CreateAdminUser exclusivamente. Em multi-user, fazer
// "first-login-takes-all" sobre canais sem dono prejudicava o segundo
// usuário, que perderia acesso aos canais migrados de instâncias
// single-user prévias. Canais que ficarem sem OwnerUserID continuam
// rejeitando mensagens entrantes (ver gateway.handleIncoming) e o
// usuário precisa reabrir as settings para reatribuir explicitamente.
func (a *App) adoptLegacyDataForUser(userID string) error {
	if err := database.AdoptLegacyData(userID); err != nil {
		return err
	}
	if a.credMgr != nil {
		return a.credMgr.LoadUserCredentials(context.Background(), userID)
	}
	return nil
}

// internalBootstrapCtx retorna o ctx do app com o userID atual injetado
// (quando há sessão ativa). Quando NÃO há sessão, retorna o ctx base sem
// userID. É FAIL-OPEN e EXISTE APENAS para os poucos caminhos legítimos
// de bootstrap pré-login que o Wails dispara ANTES de qualquer Login —
// ler/gravar credenciais via OnStartup e processar mensagens entrantes
// na UI antes da primeira sessão. Para tudo o mais use
// requireAuthenticatedContext (fail-closed).
//
// O nome é propositalmente assustador (Blocker C do re-review do AEP-0052)
// para evitar o vetor de autocomplete que dois reviews seguidos
// flagaram: bootstrapAwareCtx parecia "consciente" e respeitável; o
// reviewer disse "se vai chamar isso, prove que cabe num dos call sites
// listados abaixo, ou está com bug".
//
// USO PERMITIDO (e nada mais):
//
//   - initCredentialManager / configureCredentialManager (OnStartup):
//     registerEnvCredentials já se guarda com UserIDFromContext.
//   - MCP SetAuthContextProvider: o manager precisa ser usável
//     pré-login (descoberta de servidores) e pós-login (credenciais
//     escopadas). Os escritores reais dentro do MCP fail-close por conta.
//
// Para qualquer outro lugar (binding Wails, helper interno chamado
// pós-login, CRUD de dados de usuário, NeedsWelcomeWizard, qualquer
// resolveX/loadX/saveX) use requireAuthenticatedContext.
func (a *App) internalBootstrapCtx() context.Context {
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
// do usuário e para todos os helpers internos chamados a partir de fluxos
// pós-login.
func (a *App) requireAuthenticatedContext() (context.Context, error) {
	ctx := a.internalBootstrapCtx()
	if _, err := database.RequireUserID(ctx); err != nil {
		return nil, err
	}
	return ctx, nil
}

// ErrAdminRequired é retornado quando uma operação privilegiada é chamada
// por usuário não-admin (ou pré-login). Operações instance-wide
// (ResetDatabase, gestão de outros usuários, dump global) exigem role admin
// para evitar que qualquer usuário autenticado destrua dados de toda a
// instância em deployment multi-user.
var ErrAdminRequired = errors.New("admin role required")

// requireAdminContext devolve um context autenticado e exige que o usuário
// atual tenha role admin. Combina `requireAuthenticatedContext` com checagem
// de `currentAuthUser.Role` sob lock — sem race com Login/Logout
// concorrentes. É a função correta para bindings/handlers que executam
// ações instance-wide irreversíveis em deployments multi-user.
func (a *App) requireAdminContext() (context.Context, error) {
	ctx, err := a.requireAuthenticatedContext()
	if err != nil {
		return nil, err
	}
	a.authMu.RLock()
	role := ""
	if a.currentAuthUser != nil {
		role = a.currentAuthUser.Role
	}
	a.authMu.RUnlock()
	if role != database.UserRoleAdmin {
		return nil, ErrAdminRequired
	}
	return ctx, nil
}

// reloadUserScopedRuntimeTimeout é o teto agregado para o
// reload do runtime (registerEnvCredentials + migrateLegacyConfig +
// runPostLoginLegacyImports + initLLMProviders + initLLMClient). 10s é generoso para DB local
// + provider init em ambiente saudável; acima disso o caminho
// crítico do Login fica preso em UX-critical e o usuário força quit
// deixando estado parcial. P1-2 do re-review do PR #94.
const reloadUserScopedRuntimeTimeout = 10 * time.Second

func (a *App) reloadUserScopedRuntime() {
	if a.llmRegistry != nil {
		a.llmRegistry.Clear()
	}
	authedCtx, err := a.requireAuthenticatedContext()
	if err != nil {
		log.Printf("[reloadUserScopedRuntime] sem sessão autenticada, abortando: %v", err)
		return
	}
	ctx, cancel := context.WithTimeout(authedCtx, reloadUserScopedRuntimeTimeout)
	defer cancel()
	userID, _ := database.UserIDFromContext(authedCtx)
	startJobsForCurrentUser := func() {
		if a.jobMgr == nil {
			return
		}
		currentCtx, err := a.requireAuthenticatedContext()
		if err != nil {
			log.Printf("[reloadUserScopedRuntime] jobs não iniciados sem sessão autenticada: %v", err)
			return
		}
		currentUserID, _ := database.UserIDFromContext(currentCtx)
		if currentUserID != userID {
			log.Printf("[reloadUserScopedRuntime] jobs não iniciados: sessão mudou durante reload")
			return
		}
		if err := a.jobMgr.Start(); err != nil {
			log.Printf("[reloadUserScopedRuntime] erro ao iniciar jobs do usuário: %v", err)
		}
	}

	if a.jobMgr != nil {
		a.jobMgr.Stop()
	}
	a.registerEnvCredentials(ctx, a.credMgr)
	a.migrateLegacyConfig(ctx)
	a.runPostLoginLegacyImports(ctx)
	if a.providerSvc != nil {
		a.initLLMProviders(ctx)
	}
	if a.profileManager != nil && a.llmRegistry != nil {
		a.initLLMClient()
	}
	if a.mcpMgr != nil {
		if err := a.mcpMgr.LoadConfigs(); err != nil {
			log.Printf("[reloadUserScopedRuntime] erro ao carregar MCP servers do usuário: %v", err)
		}
		// Auto-connect MCP só agora: depois de adoptLegacyDataForUser →
		// LoadUserCredentials, as credenciais user-scoped (incluindo os
		// tokens OAuth `mcp-tokens:*` / `mcp-client:*`) estão em memória.
		// Sem isso o auto-connect cairia em fallback "sem token" e
		// abriria o navegador em paralelo para todos (ver AEP-0061).
		//
		// Não herda do `ctx` local (que tem timeout de 10s para o reload
		// inteiro); o AutoConnectAll é serial e demora N×handshake, e
		// cada Connect tem seu próprio timeout interno. Herda só o
		// userID do contexto autenticado vigente.
		go func() {
			a.mcpMgr.AutoConnectAll(database.WithUserID(context.Background(), userID))
			startJobsForCurrentUser()
		}()
	} else {
		startJobsForCurrentUser()
	}
	if err := ctx.Err(); err != nil {
		log.Printf("[reloadUserScopedRuntime] timeout/cancel atingido (%s): %v — runtime pode estar parcialmente inicializado", reloadUserScopedRuntimeTimeout, err)
	}
}

func (a *App) stopUserScopedRuntime() {
	if a.jobMgr != nil {
		a.jobMgr.Stop()
	}
	if a.llmRegistry != nil {
		a.llmRegistry.Clear()
	}
	if a.mcpMgr != nil {
		a.mcpMgr.DisconnectAll()
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
