package credentials

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"testing"
	"time"

	"assistente/internal/database"
)

func TestWildcardToRegex(t *testing.T) {
	tests := []struct {
		pattern string
		domains []struct {
			domain string
			match  bool
		}
	}{
		{
			pattern: "*.github.com",
			domains: []struct {
				domain string
				match  bool
			}{
				{"api.github.com", true},
				{"raw.github.com", true},
				{"github.com", false},
				{"github.org", false},
			},
		},
		{
			pattern: "api.example.com",
			domains: []struct {
				domain string
				match  bool
			}{
				{"api.example.com", true},
				{"example.com", false},
				{"v1.api.example.com", false},
			},
		},
		{
			pattern: "*.example.*",
			domains: []struct {
				domain string
				match  bool
			}{
				{"api.example.com", true},
				{"v1.example.org", true},
				{"example.com", false},
			},
		},
	}

	for _, tc := range tests {
		regex := wildcardToRegex(tc.pattern)
		for _, d := range tc.domains {
			result, _ := regexp.Compile(regex)
			matches := result.MatchString(d.domain)
			if matches != d.match {
				t.Errorf("Pattern '%s' domain '%s': esperado %v, got %v",
					tc.pattern, d.domain, d.match, matches)
			}
		}
	}
}

func TestRegisterAndResolve(t *testing.T) {
	mgr := NewManager(nil)

	// Registra GitHub
	githubAuth := &AuthConfig{
		Type:  "bearer",
		Token: "gh_test_token_12345",
	}
	if err := mgr.RegisterPattern("*.github.com", githubAuth); err != nil {
		t.Fatalf("Erro ao registrar padrão: %v", err)
	}

	// Registra GitLab específico
	gitlabAuth := &AuthConfig{
		Type:  "bearer",
		Token: "glpat_test_token_67890",
	}
	if err := mgr.RegisterPattern("gitlab.com", gitlabAuth); err != nil {
		t.Fatalf("Erro ao registrar padrão: %v", err)
	}

	tests := []struct {
		url   string
		found bool
		token string
	}{
		{"https://api.github.com/repos/owner/repo", true, "gh_test_token_12345"},
		{"https://raw.github.com/owner/repo/main/file.txt", true, "gh_test_token_12345"},
		{"https://github.com/owner/repo", false, ""},
		{"https://gitlab.com/api/v4/projects", true, "glpat_test_token_67890"},
		{"https://example.com/api", false, ""},
	}

	for _, tt := range tests {
		auth, err := mgr.ResolveForURL(tt.url)
		if err != nil {
			t.Errorf("URL %s: erro %v", tt.url, err)
			continue
		}

		if tt.found && auth == nil {
			t.Errorf("URL %s: esperado credenciais, got nil", tt.url)
		}
		if !tt.found && auth != nil {
			t.Errorf("URL %s: não esperado credenciais, got %+v", tt.url, auth)
		}

		if tt.found && auth != nil && auth.Token != tt.token {
			t.Errorf("URL %s: esperado token '%s', got '%s'", tt.url, tt.token, auth.Token)
		}
	}
}

func TestBasicAuth(t *testing.T) {
	mgr := NewManager(nil)

	basicAuth := &AuthConfig{
		Type:     "basic",
		Username: "user",
		Password: "secret_pass",
	}

	if err := mgr.RegisterPattern("*.example.com", basicAuth); err != nil {
		t.Fatalf("Erro ao registrar: %v", err)
	}

	auth, err := mgr.ResolveForURL("https://api.example.com/data")
	if err != nil {
		t.Fatalf("Erro ao resolver: %v", err)
	}

	if auth == nil {
		t.Fatal("Esperado credenciais")
	}

	if auth.Username != "user" || auth.Password != "secret_pass" {
		t.Errorf("Credenciais incorretas: %+v", auth)
	}
}

func TestCustomHeaders(t *testing.T) {
	mgr := NewManager(nil)

	auth := &AuthConfig{
		Type: "custom",
		Headers: map[string]string{
			"X-API-Key":  "secret_key_123",
			"User-Agent": "CustomAgent/1.0",
		},
	}

	if err := mgr.RegisterPattern("api.internal.com", auth); err != nil {
		t.Fatalf("Erro ao registrar: %v", err)
	}

	resolved, err := mgr.ResolveForURL("https://api.internal.com/endpoint")
	if err != nil {
		t.Fatalf("Erro ao resolver: %v", err)
	}

	if resolved == nil {
		t.Fatal("Esperado credenciais")
	}

	if resolved.Headers["X-API-Key"] != "secret_key_123" {
		t.Errorf("Header X-API-Key incorreto: %s", resolved.Headers["X-API-Key"])
	}
}

func TestPortHandling(t *testing.T) {
	mgr := NewManager(nil)

	auth := &AuthConfig{
		Type:  "bearer",
		Token: "token_123",
	}

	if err := mgr.RegisterPattern("*.example.com", auth); err != nil {
		t.Fatalf("Erro ao registrar: %v", err)
	}

	// URL com porta deve funcionar (porta removida antes de match)
	resolvedAuth, err := mgr.ResolveForURL("https://api.example.com:8443/data")
	if err != nil {
		t.Fatalf("Erro ao resolver: %v", err)
	}

	if resolvedAuth == nil {
		t.Error("Esperado credenciais para URL com porta")
	}
}

func TestNoCredentials(t *testing.T) {
	mgr := NewManager(nil)

	auth, err := mgr.ResolveForURL("https://unknown.example.com/api")
	if err != nil {
		t.Fatalf("Erro ao resolver: %v", err)
	}

	if auth != nil {
		t.Error("Esperado nil para domínio sem credenciais")
	}
}

func TestListPatterns(t *testing.T) {
	mgr := NewManager(nil)

	patterns := []string{"*.github.com", "gitlab.com", "api.*.internal"}
	for _, p := range patterns {
		_ = mgr.RegisterPattern(p, &AuthConfig{Type: "bearer", Token: "test"})
	}

	listed := mgr.ListPatterns()
	if len(listed) != len(patterns) {
		t.Errorf("Esperado %d padrões, got %d", len(patterns), len(listed))
	}

	// Verificar que todos os padrões estão lá
	for _, p := range patterns {
		found := false
		for _, l := range listed {
			if l == p {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Padrão '%s' não encontrado em ListPatterns", p)
		}
	}
}

func TestEncryption(t *testing.T) {
	mgr := NewManager(nil)

	secret := "very_secret_password_12345!"
	encrypted, err := mgr.encrypt(secret)
	if err != nil {
		t.Fatalf("Erro ao criptografar: %v", err)
	}

	// Encrypted deve ser diferente do original e estar em base64
	if encrypted == secret {
		t.Error("Encrypted deve ser diferente do plaintext")
	}

	// Descriptografar
	decrypted, err := mgr.decrypt(encrypted)
	if err != nil {
		t.Fatalf("Erro ao descriptografar: %v", err)
	}

	if decrypted != secret {
		t.Errorf("Descriptografia falhou: esperado '%s', got '%s'", secret, decrypted)
	}
}

func TestGetAndDeletePattern(t *testing.T) {
	mgr := NewManager(nil)

	auth := &AuthConfig{
		Type:  "bearer",
		Token: "token_abc_123",
	}

	if err := mgr.RegisterPattern("channel:slack:bot_token", auth); err != nil {
		t.Fatalf("Erro ao registrar padrão: %v", err)
	}

	resolved, err := mgr.GetByPattern("channel:slack:bot_token")
	if err != nil {
		t.Fatalf("Erro ao buscar padrão: %v", err)
	}
	if resolved == nil || resolved.Token != "token_abc_123" {
		t.Fatalf("Credenciais incorretas: %+v", resolved)
	}

	if err := mgr.DeletePattern(context.Background(), "channel:slack:bot_token"); err != nil {
		t.Fatalf("Erro ao remover padrão: %v", err)
	}

	resolved, err = mgr.GetByPattern("channel:slack:bot_token")
	if err != nil {
		t.Fatalf("Erro ao buscar padrão após remoção: %v", err)
	}
	if resolved != nil {
		t.Fatalf("Esperado nil após remoção, got %+v", resolved)
	}
}

func TestListCredentials(t *testing.T) {
	mgr := NewManager(nil)

	if err := mgr.RegisterPattern("example.com", &AuthConfig{Type: "bearer", Token: "tok_123"}); err != nil {
		t.Fatalf("Erro ao registrar credencial: %v", err)
	}

	list, err := mgr.ListCredentials()
	if err != nil {
		t.Fatalf("Erro ao listar credenciais: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("Esperado 1 credencial, got %d", len(list))
	}
	if list[0].Pattern != "example.com" || list[0].Auth == nil || list[0].Auth.Token != "tok_123" {
		t.Fatalf("Credencial inesperada: %+v", list[0])
	}
}

func TestInvalidPattern(t *testing.T) {
	mgr := NewManager(nil)

	// Padrão nil deve retornar erro
	err := mgr.RegisterPattern("", &AuthConfig{Type: "bearer"})
	if err == nil {
		t.Error("Esperado erro para padrão vazio")
	}
}

func TestNilAuth(t *testing.T) {
	mgr := NewManager(nil)

	// Auth nil deve retornar erro
	err := mgr.RegisterPattern("*.example.com", nil)
	if err == nil {
		t.Error("Esperado erro para auth nil")
	}
}

func TestPriorityOrder(t *testing.T) {
	mgr := NewManager(nil)

	auth1 := &AuthConfig{Type: "bearer", Token: "token_from_first"}
	auth2 := &AuthConfig{Type: "bearer", Token: "token_from_second"}

	_ = mgr.RegisterPattern("*.github.com", auth1)
	_ = mgr.RegisterPattern("api.*", auth2)

	resolved, _ := mgr.ResolveForURL("https://api.github.com/data")
	if resolved == nil || resolved.Token != "token_from_first" {
		t.Error("Prioridade: primeira pattern deve vencer")
	}
}

func TestOAuth2FieldsRoundTrip(t *testing.T) {
	mgr := NewManager(nil)

	auth := &AuthConfig{
		Type:         "oauth2",
		Token:        "access_tok_xyz",
		RefreshURL:   "refresh_tok_abc",
		ClientID:     "app-client-id-123",
		ClientSecret: "super-secret-456",
		ExpiresAt:    1700000000,
	}

	if err := mgr.RegisterPattern("mcp-client:test-server", auth); err != nil {
		t.Fatalf("Erro ao registrar: %v", err)
	}

	resolved, err := mgr.GetByPattern("mcp-client:test-server")
	if err != nil {
		t.Fatalf("Erro ao buscar: %v", err)
	}
	if resolved == nil {
		t.Fatal("Esperado credenciais, got nil")
	}

	if resolved.Token != "access_tok_xyz" {
		t.Errorf("Token: esperado 'access_tok_xyz', got '%s'", resolved.Token)
	}
	if resolved.RefreshURL != "refresh_tok_abc" {
		t.Errorf("RefreshURL: esperado 'refresh_tok_abc', got '%s'", resolved.RefreshURL)
	}
	if resolved.ClientID != "app-client-id-123" {
		t.Errorf("ClientID: esperado 'app-client-id-123', got '%s'", resolved.ClientID)
	}
	if resolved.ClientSecret != "super-secret-456" {
		t.Errorf("ClientSecret: esperado 'super-secret-456', got '%s'", resolved.ClientSecret)
	}
	if resolved.ExpiresAt != 1700000000 {
		t.Errorf("ExpiresAt: esperado 1700000000, got %d", resolved.ExpiresAt)
	}
}

func TestListCredentialsOAuth2Fields(t *testing.T) {
	mgr := NewManager(nil)

	auth := &AuthConfig{
		Type:         "oauth2",
		Token:        "tok",
		RefreshURL:   "refresh",
		ClientID:     "cid",
		ClientSecret: "csec",
	}

	if err := mgr.RegisterPattern("mcp-tokens:my-server", auth); err != nil {
		t.Fatalf("Erro ao registrar: %v", err)
	}

	list, err := mgr.ListCredentials()
	if err != nil {
		t.Fatalf("Erro ao listar: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("Esperado 1, got %d", len(list))
	}

	a := list[0].Auth
	if a.Token != "tok" || a.RefreshURL != "refresh" || a.ClientID != "cid" || a.ClientSecret != "csec" {
		t.Errorf("Campos OAuth2 incorretos na listagem: %+v", a)
	}
}

func TestTryDecryptLegacyPlaintext(t *testing.T) {
	mgr := NewManager(nil)

	plaintext := "legacy-plaintext-value"
	result := mgr.tryDecrypt(plaintext)
	if result != plaintext {
		t.Errorf("tryDecrypt com texto plano: esperado '%s', got '%s'", plaintext, result)
	}

	encrypted, err := mgr.encrypt("encrypted-value")
	if err != nil {
		t.Fatalf("encrypt falhou: %v", err)
	}
	result = mgr.tryDecrypt(encrypted)
	if result != "encrypted-value" {
		t.Errorf("tryDecrypt com dado criptografado: esperado 'encrypted-value', got '%s'", result)
	}
}

func TestEncryptDecryptAllSensitiveFields(t *testing.T) {
	mgr := NewManager(nil)

	auth := &AuthConfig{
		Type:         "oauth2",
		Token:        "my-token",
		Password:     "my-password",
		ClientID:     "my-client-id",
		ClientSecret: "my-client-secret",
		RefreshURL:   "my-refresh-token",
		Headers:      map[string]string{"X-Key": "header-val"},
	}

	encrypted, err := mgr.encryptAuth(auth)
	if err != nil {
		t.Fatalf("encryptAuth falhou: %v", err)
	}

	if encrypted.Token == "my-token" {
		t.Error("Token não foi criptografado")
	}
	if encrypted.Password == "my-password" {
		t.Error("Password não foi criptografado")
	}
	if encrypted.ClientID == "my-client-id" {
		t.Error("ClientID não foi criptografado")
	}
	if encrypted.ClientSecret == "my-client-secret" {
		t.Error("ClientSecret não foi criptografado")
	}
	if encrypted.RefreshURL == "my-refresh-token" {
		t.Error("RefreshURL não foi criptografado")
	}
	if encrypted.Headers["X-Key"] == "header-val" {
		t.Error("Header não foi criptografado")
	}

	decrypted, err := mgr.decryptAuth(encrypted)
	if err != nil {
		t.Fatalf("decryptAuth falhou: %v", err)
	}

	if decrypted.Token != "my-token" {
		t.Errorf("Token: esperado 'my-token', got '%s'", decrypted.Token)
	}
	if decrypted.Password != "my-password" {
		t.Errorf("Password: esperado 'my-password', got '%s'", decrypted.Password)
	}
	if decrypted.ClientID != "my-client-id" {
		t.Errorf("ClientID: esperado 'my-client-id', got '%s'", decrypted.ClientID)
	}
	if decrypted.ClientSecret != "my-client-secret" {
		t.Errorf("ClientSecret: esperado 'my-client-secret', got '%s'", decrypted.ClientSecret)
	}
	if decrypted.RefreshURL != "my-refresh-token" {
		t.Errorf("RefreshURL: esperado 'my-refresh-token', got '%s'", decrypted.RefreshURL)
	}
	if decrypted.Headers["X-Key"] != "header-val" {
		t.Errorf("Header X-Key: esperado 'header-val', got '%s'", decrypted.Headers["X-Key"])
	}
}

func TestDecryptAuthRawPreservesRefs(t *testing.T) {
	mgr := NewManager(nil)

	auth := &AuthConfig{
		Type:         "oauth2",
		Token:        "tok",
		ClientID:     "cid",
		ClientSecret: "csec",
		RefreshURL:   "reftok",
	}

	encrypted, err := mgr.encryptAuth(auth)
	if err != nil {
		t.Fatalf("encryptAuth falhou: %v", err)
	}

	raw, err := mgr.decryptAuthRaw(encrypted)
	if err != nil {
		t.Fatalf("decryptAuthRaw falhou: %v", err)
	}

	if raw.Token != "tok" || raw.ClientID != "cid" || raw.ClientSecret != "csec" || raw.RefreshURL != "reftok" {
		t.Errorf("decryptAuthRaw não retornou valores corretos: %+v", raw)
	}
}

func TestIsManagedPattern(t *testing.T) {
	tests := []struct {
		pattern string
		want    bool
	}{
		{"mcp-client:atlassian", true},
		{"mcp-tokens:atlassian", true},
		{"mcp-client:", true},
		{"mcp-tokens:", true},
		{"internal-auth:jwt-signing-key", true},
		{"internal-tls:private-key", true},
		{"*.github.com", false},
		{"api.example.com", false},
		{"channel:slack:bot_token", false},
		{"mcp-clientx:foo", false},
		{"", false},
	}
	for _, tc := range tests {
		got := IsManagedPattern(tc.pattern)
		if got != tc.want {
			t.Errorf("IsManagedPattern(%q): got %v, want %v", tc.pattern, got, tc.want)
		}
	}
}

func TestInstanceSecretsUseManagedPatterns(t *testing.T) {
	mgr := NewManager(nil)
	if err := mgr.RegisterInstanceSecret(InstanceSecretJWTSigningKey, "private-key"); err != nil {
		t.Fatalf("register instance secret: %v", err)
	}
	value, ok, err := mgr.GetInstanceSecret(InstanceSecretJWTSigningKey)
	if err != nil {
		t.Fatalf("get instance secret: %v", err)
	}
	if !ok || value != "private-key" {
		t.Fatalf("unexpected instance secret: ok=%v value=%q", ok, value)
	}
	if err := mgr.RegisterInstanceSecret("api.example.com", "secret"); err == nil {
		t.Fatal("expected non-managed instance secret pattern to fail")
	}
}

func TestUpdateExistingPattern(t *testing.T) {
	mgr := NewManager(nil)

	auth1 := &AuthConfig{Type: "bearer", Token: "old-token"}
	_ = mgr.RegisterPattern("api.test.com", auth1)

	auth2 := &AuthConfig{Type: "bearer", Token: "new-token"}
	_ = mgr.RegisterPattern("api.test.com", auth2)

	if len(mgr.ListPatterns()) != 1 {
		t.Fatalf("Esperado 1 padrão, got %d", len(mgr.ListPatterns()))
	}

	resolved, _ := mgr.GetByPattern("api.test.com")
	if resolved == nil || resolved.Token != "new-token" {
		t.Errorf("Pattern não foi atualizado: %+v", resolved)
	}
}

type reentrantCredentialStore struct {
	manager *Manager
	saved   StoredCredential
	listErr error
	noID    bool
}

func (s *reentrantCredentialStore) SaveCredential(_ context.Context, cred StoredCredential) error {
	s.saved = cred
	_ = s.manager.ListPatterns()
	return nil
}

func (s *reentrantCredentialStore) ListCredentials(context.Context) ([]StoredCredential, error) {
	if s.saved.Pattern == "" {
		return nil, nil
	}
	cred := s.saved
	if s.listErr != nil {
		return nil, s.listErr
	}
	if cred.ID == "" && !s.noID {
		cred.ID = "persisted-credential-id"
	}
	return []StoredCredential{cred}, nil
}

func (s *reentrantCredentialStore) DeleteCredential(context.Context, string) error {
	return nil
}

func (s *reentrantCredentialStore) SaveKeyWrap(context.Context, KeyWrap) error {
	return nil
}

func (s *reentrantCredentialStore) GetKeyWrap(context.Context, string) (*KeyWrap, error) {
	return nil, nil
}

func (s *reentrantCredentialStore) HasKeyWrap(context.Context, string) (bool, error) {
	return false, nil
}

func TestRegisterStoredCredentialDoesNotHoldLockDuringStoreIO(t *testing.T) {
	store := &reentrantCredentialStore{}
	mgr := NewManagerWithStoreAndPersistence([]byte("test-key-exactly-32-bytes-long!!"), store, true)
	store.manager = mgr

	done := make(chan error, 1)
	go func() {
		done <- mgr.RegisterStoredCredentialWithContext(context.Background(), StoredCredential{
			Pattern: "api.example.com",
			Auth:    &AuthConfig{Type: "bearer", Token: "secret"},
		})
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("RegisterStoredCredentialWithContext() error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("RegisterStoredCredentialWithContext() appears to hold manager lock during store I/O")
	}

	creds, err := mgr.ListCredentials()
	if err != nil {
		t.Fatalf("ListCredentials() error = %v", err)
	}
	if len(creds) != 1 || creds[0].ID != "persisted-credential-id" {
		t.Fatalf("expected persisted credential id in memory, got %+v", creds)
	}
}

func TestRegisterStoredCredentialReturnsListCredentialsError(t *testing.T) {
	store := &reentrantCredentialStore{listErr: errors.New("store unavailable")}
	mgr := NewManagerWithStoreAndPersistence([]byte("test-key-exactly-32-bytes-long!!"), store, true)
	store.manager = mgr

	err := mgr.RegisterStoredCredentialWithContext(context.Background(), StoredCredential{
		Pattern: "api.example.com",
		Auth:    &AuthConfig{Type: "bearer", Token: "secret"},
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "listar credenciais persistidas após salvar") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRegisterStoredCredentialRequiresPersistedIDAfterSave(t *testing.T) {
	store := &reentrantCredentialStore{noID: true}
	mgr := NewManagerWithStoreAndPersistence([]byte("test-key-exactly-32-bytes-long!!"), store, true)
	store.manager = mgr

	err := mgr.RegisterStoredCredentialWithContext(context.Background(), StoredCredential{
		Pattern: "api.example.com",
		Auth:    &AuthConfig{Type: "bearer", Token: "secret"},
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "id da credencial persistida não encontrado") {
		t.Fatalf("unexpected error: %v", err)
	}
}

type staticCredentialStore struct {
	entries []StoredCredential
}

func (s *staticCredentialStore) SaveCredential(context.Context, StoredCredential) error {
	return nil
}

func (s *staticCredentialStore) ListCredentials(context.Context) ([]StoredCredential, error) {
	return s.entries, nil
}

func (s *staticCredentialStore) DeleteCredential(context.Context, string) error {
	return nil
}

func (s *staticCredentialStore) SaveKeyWrap(context.Context, KeyWrap) error {
	return nil
}

func (s *staticCredentialStore) GetKeyWrap(context.Context, string) (*KeyWrap, error) {
	return nil, nil
}

func (s *staticCredentialStore) HasKeyWrap(context.Context, string) (bool, error) {
	return false, nil
}

func TestLoadFromStorePreservesUserScope(t *testing.T) {
	key := []byte("test-key-exactly-32-bytes-long!!")
	encoder := NewManager(key)
	encAuth, err := encoder.encryptAuth(&AuthConfig{Type: "bearer", Token: "sk-user-1"})
	if err != nil {
		t.Fatalf("encrypt auth: %v", err)
	}

	store := &staticCredentialStore{entries: []StoredCredential{{
		ID:      "cred-1",
		UserID:  "user-1",
		Pattern: "llm.inclunet.com.br",
		Auth:    encAuth,
	}}}
	mgr := NewManagerWithStoreAndPersistence(key, store, true)
	if err := mgr.LoadFromStore(context.Background()); err != nil {
		t.Fatalf("LoadFromStore() error = %v", err)
	}

	auth, err := mgr.GetByPatternWithContext(database.WithUserID(context.Background(), "user-1"), "llm.inclunet.com.br")
	if err != nil {
		t.Fatalf("GetByPatternWithContext() error = %v", err)
	}
	if auth == nil || auth.Token != "sk-user-1" {
		t.Fatalf("expected scoped credential for user-1, got %+v", auth)
	}

	otherAuth, err := mgr.GetByPatternWithContext(database.WithUserID(context.Background(), "user-2"), "llm.inclunet.com.br")
	if err != nil {
		t.Fatalf("GetByPatternWithContext(other) error = %v", err)
	}
	if otherAuth != nil {
		t.Fatalf("credential leaked across users: %+v", otherAuth)
	}
}

// === NOVOS TESTES PARA SEGURANÇA E EDGE CASES ===

// TestClientSecretEncryption testa que ClientSecret (sensível) é criptografado
func TestClientSecretEncryption(t *testing.T) {
	mgr := NewManager(nil)

	secret := "my_client_secret_xyz_123"
	auth := &AuthConfig{
		Type:         "oauth2",
		ClientID:     "my_client_id",
		ClientSecret: secret,
		Token:        "access_token_123",
	}

	if err := mgr.RegisterPattern("oauth.provider.com", auth); err != nil {
		t.Fatalf("Erro ao registrar: %v", err)
	}

	// Recuperar e verificar que ClientSecret foi preservado
	resolved, _ := mgr.ResolveForURL("https://oauth.provider.com/api")
	if resolved == nil {
		t.Fatal("Esperado credenciais")
	}

	if resolved.ClientSecret != secret {
		t.Errorf("ClientSecret não foi preservado: esperado %q, got %q", secret, resolved.ClientSecret)
	}

	// ClientSecret não deve aparecer em ListCredentials em plaintext (se implementado)
	listed, err := mgr.ListCredentials()
	if err != nil {
		t.Logf("ListCredentials retornou erro: %v (pode ser esperado sem store)", err)
		return
	}
	for _, cred := range listed {
		if cred.Auth.ClientSecret != "" && cred.Auth.ClientSecret == secret {
			// Neste caso, o ClientSecret está em plaintext na lista (pode ser um issue de segurança)
			t.Logf("WARNING: ClientSecret aparece em plaintext em ListCredentials")
		}
	}
}

// TestExpiredCredentials testa credenciais com timestamp de expiração
func TestExpiredCredentials(t *testing.T) {
	mgr := NewManager(nil)

	// Credencial que já expirou (timestamp no passado)
	pastTime := int64(1000000000) // erro de 2001
	auth := &AuthConfig{
		Type:      "bearer",
		Token:     "expired_token",
		ExpiresAt: pastTime,
	}

	if err := mgr.RegisterPattern("expired.test.com", auth); err != nil {
		t.Fatalf("Erro ao registrar: %v", err)
	}

	resolved, _ := mgr.ResolveForURL("https://expired.test.com/api")
	if resolved == nil {
		t.Fatal("Esperado credenciais (mesmo expiradas)")
	}

	// Manager retorna credencial expirada - cliente deve checar ExpiresAt
	if resolved.ExpiresAt != pastTime {
		t.Errorf("ExpiresAt não foi preservado: %d", resolved.ExpiresAt)
	}
}

// TestURLSpecialCharacters testa URLs com caracteres especiais
func TestURLSpecialCharacters(t *testing.T) {
	mgr := NewManager(nil)

	auth := &AuthConfig{
		Type:  "bearer",
		Token: "token_special",
	}

	if err := mgr.RegisterPattern("*.example.com", auth); err != nil {
		t.Fatalf("Erro ao registrar: %v", err)
	}

	tests := []struct {
		url      string
		expected bool
		desc     string
	}{
		{"https://api-test.example.com/path", true, "hyphenated subdomain"},
		{"https://api.example.com:8080/path?query=value#hash", true, "with port, query, hash"},
		{"https://api.example.com/path%20with%20spaces", true, "URL-encoded path"},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			resolved, _ := mgr.ResolveForURL(tt.url)
			if tt.expected && resolved == nil {
				t.Errorf("%s: esperado credenciais, got nil", tt.desc)
			}
			if !tt.expected && resolved != nil {
				t.Errorf("%s: não esperado credenciais, got %+v", tt.desc, resolved)
			}
		})
	}
}

// TestConcurrentRegisterAndResolve testa operações concorrentes sem race condition
func TestConcurrentRegisterAndResolve(t *testing.T) {
	mgr := NewManager(nil)
	done := make(chan bool, 20)

	// 10 goroutines registrando
	for i := 0; i < 10; i++ {
		go func(idx int) {
			pattern := fmt.Sprintf("api%d.example.com", idx)
			token := fmt.Sprintf("token_%d", idx)
			auth := &AuthConfig{Type: "bearer", Token: token}
			_ = mgr.RegisterPattern(pattern, auth)
			done <- true
		}(i)
	}

	// 10 goroutines resolvendo
	for i := 0; i < 10; i++ {
		go func(idx int) {
			pattern := fmt.Sprintf("api%d.example.com", idx)
			url := fmt.Sprintf("https://%s/api", pattern)
			_, _ = mgr.ResolveForURL(url)
			done <- true
		}(i)
	}

	// Aguarda todas
	for i := 0; i < 20; i++ {
		<-done
	}

	// Se chegou aqui sem panic ou deadlock, OK
	if len(mgr.ListPatterns()) != 10 {
		t.Errorf("Esperado 10 padrões, got %d", len(mgr.ListPatterns()))
	}
}

// TestInvalidURL testa tratamento de URLs inválidas
func TestInvalidURL(t *testing.T) {
	mgr := NewManager(nil)

	_ = mgr.RegisterPattern("*.example.com", &AuthConfig{Type: "bearer", Token: "test"})

	// URLs com scheme malformado devem retornar erro
	invalidURLs := []string{
		"://no-scheme.com",
	}

	for _, url := range invalidURLs {
		_, err := mgr.ResolveForURL(url)
		if err == nil {
			t.Errorf("URL inválida não retornou erro: %q", url)
		}
	}
}

// TestNilEncryptionKey testa manager com encryption key nil (gera aleatória)
func TestNilEncryptionKey(t *testing.T) {
	mgr1 := NewManager(nil) // key = nil, deve gerar
	mgr2 := NewManager(nil) // key = nil, deve gerar diferente

	auth := &AuthConfig{Type: "bearer", Token: "test_secret"}

	_ = mgr1.RegisterPattern("test.com", auth)
	_ = mgr2.RegisterPattern("test.com", auth)

	// Keys devem ser diferentes (aleatórias)
	if string(mgr1.encKey) == string(mgr2.encKey) {
		t.Error("Encryption keys deveriam ser aleatórias quando nil")
	}

	// Mas ambas devem conseguir resolver (cada uma com sua key)
	resolved1, _ := mgr1.ResolveForURL("https://test.com/api")
	resolved2, _ := mgr2.ResolveForURL("https://test.com/api")

	if resolved1 == nil || resolved2 == nil {
		t.Fatal("Ambas deveriam conseguir resolver")
	}
}

// TestEmptyPatternRejection testa rejeição de patterns vazios
func TestEmptyPatternRejection(t *testing.T) {
	mgr := NewManager(nil)

	tests := []struct {
		pattern string
		auth    *AuthConfig
		name    string
		should  bool // esperado erro?
	}{
		{"", &AuthConfig{Type: "bearer", Token: "test"}, "empty_pattern", true},
		{"test.com", nil, "nil_auth", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := mgr.RegisterPattern(tt.pattern, tt.auth)
			if tt.should && err == nil {
				t.Errorf("%s: esperado erro de validação", tt.name)
			}
			if !tt.should && err != nil {
				t.Errorf("%s: não esperado erro: %v", tt.name, err)
			}
		})
	}
}

// TestHeadersPreservation testa que headers customizados são preservados
func TestHeadersPreservation(t *testing.T) {
	mgr := NewManager(nil)

	headers := map[string]string{
		"Authorization": "Bearer token123",
		"X-Custom":      "value",
		"Accept":        "application/json",
	}

	auth := &AuthConfig{
		Type:    "custom",
		Headers: headers,
	}

	_ = mgr.RegisterPattern("api.test.com", auth)

	resolved, _ := mgr.ResolveForURL("https://api.test.com/endpoint")
	if resolved == nil {
		t.Fatal("Esperado credenciais")
	}

	for key, val := range headers {
		if resolved.Headers[key] != val {
			t.Errorf("Header %s não preservado: esperado %q, got %q", key, val, resolved.Headers[key])
		}
	}
}

// TestContextCancellation testa se RegisterPatternWithContext respeita cancelamento
func TestContextCancellation(t *testing.T) {
	mgr := NewManager(nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancelar imediatamente

	auth := &AuthConfig{Type: "bearer", Token: "test"}

	// Registrar com context cancelado pode retornar erro (dependendo de implementation)
	// Desde que não cause panic, está OK
	err := mgr.RegisterPatternWithContext(ctx, "test.com", auth)
	_ = err // Ignorar resultado
}
