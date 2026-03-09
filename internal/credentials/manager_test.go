package credentials

import (
	"context"
	"regexp"
	"testing"
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
		mgr.RegisterPattern(p, &AuthConfig{Type: "bearer", Token: "test"})
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

	// Registra padrões em ordem (primeiro vence)
	auth1 := &AuthConfig{Type: "bearer", Token: "token_from_first"}
	auth2 := &AuthConfig{Type: "bearer", Token: "token_from_second"}

	mgr.RegisterPattern("*.github.com", auth1)
	mgr.RegisterPattern("api.*", auth2)

	// api.github.com pode fazer match em ambos - primeira deve vencer
	resolved, _ := mgr.ResolveForURL("https://api.github.com/data")
	if resolved == nil || resolved.Token != "token_from_first" {
		t.Error("Prioridade: primeira pattern deve vencer")
	}
}
