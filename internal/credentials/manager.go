package credentials

import (
	"assistente/internal/logging"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/url"
	"regexp"
	"strings"
	"sync"

	"assistente/internal/database"
)

// AuthConfig descreve como autenticar em um domínio
type AuthConfig struct {
	Type         string            // "bearer", "basic", "oauth2", "custom", "none"
	Token        string            // para bearer, oauth2
	Username     string            // para basic auth
	Password     string            // para basic auth
	Headers      map[string]string // headers customizados (já com valores)
	ExpiresAt    int64             // unix timestamp, 0 = sem expiração
	RefreshURL   string            // para oauth2 refresh
	ClientSecret string            // para oauth2 client credentials (criptografado)
	ClientID     string            // para oauth2 DCR (dynamic client registration)
}

// DomainCredential mapeia um padrão de domínio a credenciais
type DomainCredential struct {
	ID      string
	UserID  string
	Pattern string // "*.github.com", "api.example.com", etc
	regex   *regexp.Regexp
	Auth    *AuthConfig
}

// Manager armazena e resolve credenciais por domínio.
type Manager struct {
	mu          sync.RWMutex
	credentials []*DomainCredential
	encKey      []byte // para criptografar credenciais em memória
	store       Store
	persist     bool

	// integrity guarda o último resultado de `verifyDEKConsistency`,
	// consultável via `IntegrityStatus()`. Atualizado em
	// `LoadInstanceSecrets` e `Reset`.
	integrity vaultIntegrity
}

type instanceCredentialStore interface {
	ListInstanceCredentials(ctx context.Context) ([]StoredCredential, error)
}

// NewManager cria novo credential manager (sem persistência).
func NewManager(encryptionKey []byte) *Manager {
	return NewManagerWithStoreAndPersistence(encryptionKey, nil, false)
}

// NewManagerWithStore cria manager com store configurado (persistência opcional).
func NewManagerWithStore(encryptionKey []byte, store Store, persist bool) *Manager {
	return NewManagerWithStoreAndPersistence(encryptionKey, store, persist)
}

// NewManagerWithStoreAndPersistence cria manager com controle explícito de persistência.
func NewManagerWithStoreAndPersistence(encryptionKey []byte, store Store, persist bool) *Manager {
	// Se key não fornecida, gera aleatória
	if len(encryptionKey) == 0 {
		encryptionKey = make([]byte, 32) // AES-256
		rand.Read(encryptionKey)
	}
	return &Manager{
		credentials: make([]*DomainCredential, 0),
		encKey:      encryptionKey,
		store:       store,
		persist:     persist && store != nil,
	}
}

// RegisterPattern registra credenciais para um padrão de domínio
// Exemplos: "*.github.com", "api.example.com", "github.com"
func (m *Manager) RegisterPattern(pattern string, auth *AuthConfig) error {
	return m.RegisterStoredCredentialWithContext(context.Background(), StoredCredential{
		Pattern: pattern,
		Auth:    auth,
	})
}

// RegisterPatternWithContext registra credenciais com contexto (persistência).
func (m *Manager) RegisterPatternWithContext(ctx context.Context, pattern string, auth *AuthConfig) error {
	return m.RegisterStoredCredentialWithContext(ctx, StoredCredential{
		Pattern: pattern,
		Auth:    auth,
	})
}

// RegisterStoredCredentialWithContext registra credenciais preservando o ID persistido, quando informado.
func (m *Manager) RegisterStoredCredentialWithContext(ctx context.Context, cred StoredCredential) error {
	pattern := cred.Pattern
	auth := cred.Auth
	if pattern == "" || auth == nil {
		return errors.New("pattern e auth não podem ser vazios")
	}

	// Converter padrão wildcard em regex
	regexStr := wildcardToRegex(pattern)
	regex, err := regexp.Compile(regexStr)
	if err != nil {
		return fmt.Errorf("padrão inválido '%s': %w", pattern, err)
	}

	// Criptografar credenciais sensíveis
	encAuth, err := m.encryptAuth(auth)
	if err != nil {
		return err
	}

	persistedID := cred.ID
	userID := cred.UserID
	if userID == "" {
		if scopedUserID, ok := database.UserIDFromContext(ctx); ok {
			userID = scopedUserID
		}
	}
	if userID == "" && !IsInstanceSecretPattern(pattern) && m.persist {
		return database.ErrUserScopeRequired
	}
	if m.persist && m.store != nil {
		if err := m.store.SaveCredential(ctx, StoredCredential{ID: cred.ID, UserID: userID, Pattern: pattern, Auth: encAuth}); err != nil {
			return err
		}
		if cred.ID == "" {
			persisted, err := m.lookupPersistedByScope(ctx, userID)
			if err != nil {
				return fmt.Errorf("listar credenciais persistidas após salvar: %w", err)
			}
			for _, entry := range persisted {
				if entry.Pattern == pattern && entry.UserID == userID && entry.ID != "" {
					persistedID = entry.ID
					userID = entry.UserID
					break
				}
			}
			if persistedID == "" {
				return fmt.Errorf("id da credencial persistida não encontrado após salvar pattern %q", pattern)
			}
		}
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	for i, existing := range m.credentials {
		sameStoredCredential := persistedID != "" && existing.ID == persistedID
		sameScopedPattern := existing.Pattern == pattern && existing.UserID == userID
		if sameStoredCredential || sameScopedPattern {
			m.credentials[i] = &DomainCredential{ID: persistedID, UserID: userID, Pattern: pattern, regex: regex, Auth: encAuth}
			return nil
		}
	}
	m.credentials = append(m.credentials, &DomainCredential{ID: persistedID, UserID: userID, Pattern: pattern, regex: regex, Auth: encAuth})

	return nil
}

// ResolveForURL resolve credenciais para uma URL
// Retorna nil se não encontrar
func (m *Manager) ResolveForURL(urlStr string) (*AuthConfig, error) {
	return m.ResolveForURLWithContext(context.Background(), urlStr)
}

func (m *Manager) ResolveForURLWithContext(ctx context.Context, urlStr string) (*AuthConfig, error) {
	u, err := url.Parse(urlStr)
	if err != nil {
		return nil, fmt.Errorf("URL inválida: %w", err)
	}

	domain := strings.ToLower(u.Host)
	// Remove porta se houver
	if idx := strings.LastIndex(domain, ":"); idx >= 0 {
		domain = domain[:idx]
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	userID := ""
	if scopedUser, ok := database.UserIDFromContext(ctx); ok {
		userID = scopedUser
	}
	// Procura em ordem (primeira match vence)
	for _, dc := range m.credentials {
		if userID != "" && dc.UserID != userID {
			continue
		}
		if userID == "" && dc.UserID != "" {
			continue
		}
		if dc.regex.MatchString(domain) {
			// Descriptografar antes de retornar
			auth, err := m.decryptAuth(dc.Auth)
			if err != nil {
				return nil, fmt.Errorf("erro ao descriptografar credenciais: %w", err)
			}
			return auth, nil
		}
	}

	return nil, nil // sem credenciais para este domínio
}

// ListPatterns retorna padrões registrados (sem credenciais sensíveis)
func (m *Manager) ListPatterns() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	patterns := make([]string, len(m.credentials))
	for i, dc := range m.credentials {
		patterns[i] = dc.Pattern
	}
	return patterns
}

// ListCredentials retorna credenciais descriptografadas sem resolver referências externas.
// Refs como keyring://... e env://... ficam visíveis para exibição na UI.
func (m *Manager) ListCredentials() ([]StoredCredential, error) {
	return m.ListCredentialsWithContext(context.Background())
}

func (m *Manager) ListCredentialsWithContext(ctx context.Context) ([]StoredCredential, error) {
	return m.listCredentialsWithContext(ctx, false)
}

// ListVisibleCredentialsWithContext retorna apenas credenciais editáveis/visíveis
// ao usuário. Patterns gerenciados são filtrados antes de descriptografar para
// que segredos internos ilegíveis não bloqueiem a tela de credenciais.
func (m *Manager) ListVisibleCredentialsWithContext(ctx context.Context) ([]StoredCredential, error) {
	return m.listCredentialsWithContext(ctx, true)
}

func (m *Manager) listCredentialsWithContext(ctx context.Context, skipManaged bool) ([]StoredCredential, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]StoredCredential, 0, len(m.credentials))
	userID := ""
	if scopedUser, ok := database.UserIDFromContext(ctx); ok {
		userID = scopedUser
	}
	for _, dc := range m.credentials {
		if userID != "" && dc.UserID != userID {
			continue
		}
		if skipManaged && IsManagedPattern(dc.Pattern) {
			continue
		}
		auth, err := m.decryptAuthRaw(dc.Auth)
		if err != nil {
			if skipManaged {
				result = append(result, StoredCredential{
					ID:         dc.ID,
					UserID:     dc.UserID,
					Pattern:    dc.Pattern,
					Auth:       &AuthConfig{Type: dc.Auth.Type},
					Unreadable: true,
				})
				continue
			}
			return nil, err
		}
		result = append(result, StoredCredential{ID: dc.ID, UserID: dc.UserID, Pattern: dc.Pattern, Auth: auth})
	}

	return result, nil
}

// GetByPattern retorna credenciais para um padrão exato.
func (m *Manager) GetByPattern(pattern string) (*AuthConfig, error) {
	return m.GetByPatternWithContext(context.Background(), pattern)
}

func (m *Manager) GetByPatternWithContext(ctx context.Context, pattern string) (*AuthConfig, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	userID := ""
	if scopedUser, ok := database.UserIDFromContext(ctx); ok {
		userID = scopedUser
	}
	for _, dc := range m.credentials {
		if userID != "" && dc.UserID != userID {
			continue
		}
		if userID == "" && dc.UserID != "" {
			continue
		}
		if dc.Pattern == pattern {
			auth, err := m.decryptAuth(dc.Auth)
			if err != nil {
				scope := userID
				if scope == "" {
					scope = "<sem usuario autenticado>"
				}
				return nil, fmt.Errorf("credencial %q ilegível para usuário %q: %w", pattern, scope, err)
			}
			return auth, nil
		}
	}

	return nil, nil
}

// DeletePattern remove credenciais de um padrão e persiste a exclusão.
func (m *Manager) DeletePattern(ctx context.Context, pattern string) error {
	if pattern == "" {
		return errors.New("pattern não pode ser vazio")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	userID := ""
	if scopedUserID, ok := database.UserIDFromContext(ctx); ok {
		userID = scopedUserID
	}
	if userID == "" && !IsInstanceSecretPattern(pattern) && m.persist {
		return database.ErrUserScopeRequired
	}
	filtered := m.credentials[:0]
	for _, dc := range m.credentials {
		if dc.Pattern != pattern || (userID != "" && dc.UserID != userID) {
			filtered = append(filtered, dc)
		}
	}
	// evita manter referências antigas
	m.credentials = append([]*DomainCredential(nil), filtered...)

	if m.persist && m.store != nil {
		if err := m.store.DeleteCredential(ctx, pattern); err != nil {
			return err
		}
	}

	return nil
}

// CanPersist indica se o manager está configurado para persistir credenciais.
func (m *Manager) CanPersist() bool {
	return m.persist && m.store != nil
}

// LoadInstanceSecrets carrega para a memória APENAS os segredos
// instance-scoped (`internal-auth:*`, `internal-tls:*`). É o caminho
// chamado no boot pré-login: ainda não há sessão, então as credenciais
// user-scoped não podem (e não devem) entrar em memória.
//
// Para credenciais user-scoped use `LoadUserCredentials` após o
// Login/RefreshAuth.
func (m *Manager) LoadInstanceSecrets(ctx context.Context) error {
	if m.store == nil {
		return nil
	}
	// Verificação de consistência DEK_keychain ↔ DEK_wraps DEVE rodar
	// antes de qualquer escrita (e antes mesmo de aceitar carregar
	// segredos, para evitar populá-los em memória se houver
	// divergência crítica que vá impactar serviços que dependem
	// deles). Se o store não está pronto a verificação é no-op.
	if err := m.verifyDEKConsistency(ctx); err != nil {
		// Falhas de leitura do wrap não bloqueiam o carregamento
		// (instance secrets não dependem de wraps), mas são logadas
		// dentro de verifyDEKConsistency.
		_ = err
	}
	if !m.persist {
		return nil
	}
	// Re-cifragem one-shot de refresh tokens legados em texto plano
	// (issue #236). Roda aqui — e não na migração de schema do pacote
	// database — porque a DEK só está disponível depois que o Manager
	// é configurado com a chave do keychain. Guardada por `m.persist`
	// (garante que `m.encKey` é a DEK real, não a aleatória de
	// fallback) e pela integridade do vault (não cifrar nada com uma
	// DEK divergente dos wraps, ver AEP-0061).
	if m.integrity.get().OK {
		n, err := m.reencryptLegacyPlaintextRefreshTokens(ctx)
		if err != nil {
			return fmt.Errorf("re-cifrar refresh tokens legados: %w", err)
		}
		if n > 0 {
			logging.Infof(ctx, "credentials.manager", "[Credentials] %d refresh tokens legados re-cifrados com a DEK atual", n)
		}
	}
	entries, err := m.lookupPersistedByScope(ctx, "")
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if err := m.registerEncryptedPattern(entry.ID, entry.UserID, entry.Pattern, entry.Auth); err != nil {
			return err
		}
	}
	return nil
}

// LoadUserCredentials carrega para a memória as credenciais do
// usuário informado. É o caminho que precisa rodar pós-Login para que
// `ResolveForURLWithContext` encontre as credenciais user-scoped no
// cache do Manager — sem isso, todo request HTTP do user falha com
// `unresolvedCredentialError` mesmo com a credencial gravada no DB.
//
// `userID` é obrigatório (não há "carregar credenciais do usuário sem
// usuário"). O ctx é usado apenas para deadline/cancel; o escopo de
// query é construído explicitamente a partir do `userID`.
func (m *Manager) LoadUserCredentials(ctx context.Context, userID string) error {
	if m.store == nil || !m.persist {
		return nil
	}
	if strings.TrimSpace(userID) == "" {
		return database.ErrUserScopeRequired
	}
	entries, err := m.lookupPersistedByScope(ctx, userID)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if err := m.registerEncryptedPattern(entry.ID, entry.UserID, entry.Pattern, entry.Auth); err != nil {
			return err
		}
	}
	return nil
}

// lookupPersistedByScope é o ÚNICO lugar do Manager que decide
// "user-scoped vs instance-scoped" para leituras do store, e ele faz
// isso a partir do `userID` recebido (não do ctx). Centralizar aqui
// elimina o ramo mágico que a versão anterior (`persistedCredentials`)
// tinha — onde o callsite passava `context.Background()` achando que
// "carregava tudo" e silenciosamente caía em instance-only.
//
//   - userID == ""   → instance secrets (`internal-auth:*`/`internal-tls:*`)
//   - userID != ""   → todas as credenciais daquele user
func (m *Manager) lookupPersistedByScope(ctx context.Context, userID string) ([]StoredCredential, error) {
	if userID == "" {
		instanceStore, ok := m.store.(instanceCredentialStore)
		if !ok {
			return nil, nil
		}
		return instanceStore.ListInstanceCredentials(ctx)
	}
	scoped := database.WithUserID(ctx, userID)
	return m.store.ListCredentials(scoped)
}

// Reset redefine a chave de criptografia e limpa credenciais em memória.
func (m *Manager) Reset(encryptionKey []byte, persist bool) {
	if len(encryptionKey) == 0 {
		encryptionKey = make([]byte, 32)
		rand.Read(encryptionKey)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	m.encKey = encryptionKey
	m.credentials = make([]*DomainCredential, 0)
	m.persist = persist && m.store != nil
}

func (m *Manager) registerEncryptedPattern(id, userID, pattern string, encAuth *AuthConfig) error {
	if pattern == "" || encAuth == nil {
		return errors.New("pattern e auth não podem ser vazios")
	}

	regexStr := wildcardToRegex(pattern)
	regex, err := regexp.Compile(regexStr)
	if err != nil {
		return fmt.Errorf("padrão inválido '%s': %w", pattern, err)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	updated := false
	for i, existing := range m.credentials {
		sameStoredCredential := id != "" && existing.ID == id
		sameScopedPattern := existing.Pattern == pattern && existing.UserID == userID
		if sameStoredCredential || sameScopedPattern {
			m.credentials[i] = &DomainCredential{ID: id, UserID: userID, Pattern: pattern, regex: regex, Auth: encAuth}
			updated = true
			break
		}
	}
	if !updated {
		m.credentials = append(m.credentials, &DomainCredential{ID: id, UserID: userID, Pattern: pattern, regex: regex, Auth: encAuth})
	}

	return nil
}

// ========== Criptografia ==========

// encryptAuth criptografa credenciais sensíveis
func (m *Manager) encryptAuth(auth *AuthConfig) (*AuthConfig, error) {
	if auth == nil {
		return nil, nil
	}

	encrypted := *auth

	if auth.Token != "" {
		token, err := m.encrypt(auth.Token)
		if err != nil {
			return nil, err
		}
		encrypted.Token = token
	}

	if auth.Password != "" {
		pwd, err := m.encrypt(auth.Password)
		if err != nil {
			return nil, err
		}
		encrypted.Password = pwd
	}

	if auth.ClientID != "" {
		cid, err := m.encrypt(auth.ClientID)
		if err != nil {
			return nil, err
		}
		encrypted.ClientID = cid
	}

	if auth.ClientSecret != "" {
		cs, err := m.encrypt(auth.ClientSecret)
		if err != nil {
			return nil, err
		}
		encrypted.ClientSecret = cs
	}

	if auth.RefreshURL != "" {
		rt, err := m.encrypt(auth.RefreshURL)
		if err != nil {
			return nil, err
		}
		encrypted.RefreshURL = rt
	}

	if len(auth.Headers) > 0 {
		encrypted.Headers = make(map[string]string)
		for k, v := range auth.Headers {
			encV, err := m.encrypt(v)
			if err != nil {
				return nil, err
			}
			encrypted.Headers[k] = encV
		}
	}

	return &encrypted, nil
}

// decryptAuth descriptografa credenciais
func (m *Manager) decryptAuth(auth *AuthConfig) (*AuthConfig, error) {
	if auth == nil {
		return nil, nil
	}

	decrypted := *auth

	if auth.Token != "" {
		token, err := m.decrypt(auth.Token)
		if err != nil {
			return nil, err
		}
		token, err = ResolveExternalRef(token)
		if err != nil {
			return nil, fmt.Errorf("erro ao resolver referência do token: %w", err)
		}
		decrypted.Token = token
	}

	if auth.Password != "" {
		pwd, err := m.decrypt(auth.Password)
		if err != nil {
			return nil, err
		}
		pwd, err = ResolveExternalRef(pwd)
		if err != nil {
			return nil, fmt.Errorf("erro ao resolver referência da senha: %w", err)
		}
		decrypted.Password = pwd
	}

	if auth.ClientID != "" {
		cid := m.tryDecrypt(auth.ClientID)
		cid, err := ResolveExternalRef(cid)
		if err != nil {
			return nil, fmt.Errorf("erro ao resolver referência do client_id: %w", err)
		}
		decrypted.ClientID = cid
	}

	if auth.ClientSecret != "" {
		cs := m.tryDecrypt(auth.ClientSecret)
		cs, err := ResolveExternalRef(cs)
		if err != nil {
			return nil, fmt.Errorf("erro ao resolver referência do client_secret: %w", err)
		}
		decrypted.ClientSecret = cs
	}

	if auth.RefreshURL != "" {
		rt := m.tryDecrypt(auth.RefreshURL)
		rt, err := ResolveExternalRef(rt)
		if err != nil {
			return nil, fmt.Errorf("erro ao resolver referência do refresh token: %w", err)
		}
		decrypted.RefreshURL = rt
	}

	if len(auth.Headers) > 0 {
		decrypted.Headers = make(map[string]string)
		for k, v := range auth.Headers {
			decV, err := m.decrypt(v)
			if err != nil {
				return nil, err
			}
			decV, err = ResolveExternalRef(decV)
			if err != nil {
				return nil, fmt.Errorf("erro ao resolver referência do header %s: %w", k, err)
			}
			decrypted.Headers[k] = decV
		}
	}

	return &decrypted, nil
}

// decryptAuthRaw descriptografa credenciais sem resolver referências externas.
// Usado para listagem/exibição onde queremos ver a ref original (keyring://..., env://...).
func (m *Manager) decryptAuthRaw(auth *AuthConfig) (*AuthConfig, error) {
	if auth == nil {
		return nil, nil
	}
	decrypted := *auth

	if auth.Token != "" {
		token, err := m.decrypt(auth.Token)
		if err != nil {
			return nil, err
		}
		decrypted.Token = token
	}
	if auth.Password != "" {
		pwd, err := m.decrypt(auth.Password)
		if err != nil {
			return nil, err
		}
		decrypted.Password = pwd
	}
	if auth.ClientID != "" {
		decrypted.ClientID = m.tryDecrypt(auth.ClientID)
	}
	if auth.ClientSecret != "" {
		decrypted.ClientSecret = m.tryDecrypt(auth.ClientSecret)
	}
	if auth.RefreshURL != "" {
		decrypted.RefreshURL = m.tryDecrypt(auth.RefreshURL)
	}
	if len(auth.Headers) > 0 {
		decrypted.Headers = make(map[string]string)
		for k, v := range auth.Headers {
			decV, err := m.decrypt(v)
			if err != nil {
				return nil, err
			}
			decrypted.Headers[k] = decV
		}
	}
	return &decrypted, nil
}

// encrypt criptografa string com AES-GCM
func (m *Manager) encrypt(plaintext string) (string, error) {
	block, err := aes.NewCipher(m.encKey)
	if err != nil {
		return "", err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	nonce := make([]byte, gcm.NonceSize())
	_, err = io.ReadFull(rand.Reader, nonce)
	if err != nil {
		return "", err
	}

	ciphertext := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

// tryDecrypt tenta descriptografar; se falhar, retorna o valor original.
// Usado para campos que antes não eram criptografados (dados legados).
func (m *Manager) tryDecrypt(value string) string {
	result, err := m.decrypt(value)
	if err != nil {
		return value
	}
	return result
}

// decrypt descriptografa string com AES-GCM
func (m *Manager) decrypt(ciphertext string) (string, error) {
	data, err := base64.StdEncoding.DecodeString(ciphertext)
	if err != nil {
		return "", err
	}

	block, err := aes.NewCipher(m.encKey)
	if err != nil {
		return "", err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize {
		return "", errors.New("ciphertext muito curto")
	}

	nonce, data := data[:nonceSize], data[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, data, nil)
	if err != nil {
		return "", err
	}

	return string(plaintext), nil
}

// ========== Patterns gerenciados ==========

// managedPrefixes contém prefixos de patterns gerenciados automaticamente pelo sistema.
// Credenciais com esses prefixos não devem ser editáveis pelo usuário.
var managedPrefixes = []string{
	"mcp-client:",
	"mcp-tokens:",
	"internal-auth:",
	"internal-tls:",
}

// IsManagedPattern retorna true se o pattern pertence a uma credencial gerenciada
// automaticamente pelo sistema (ex: OAuth MCP), e não deve ser editada manualmente.
func IsManagedPattern(pattern string) bool {
	for _, prefix := range managedPrefixes {
		if strings.HasPrefix(pattern, prefix) {
			return true
		}
	}
	return false
}

// ========== Helpers ==========

// wildcardToRegex converte padrão wildcard para regex
// Exemplos:
//
//	"*.github.com" -> "^[^.]+\.github\.com$"
//	"api.example.com" -> "^api\.example\.com$"
//	"example.com" -> "^example\.com$"
func wildcardToRegex(pattern string) string {
	// Escape dots e outros caracteres especiais
	escaped := regexp.QuoteMeta(pattern)
	// * se torna [^.] (qualquer coisa menos ponto) com +
	escaped = strings.ReplaceAll(escaped, `\*`, `[^.]+`)
	// Anchor no início e fim
	return "^" + escaped + "$"
}
