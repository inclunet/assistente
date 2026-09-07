package http

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"assistente/internal/credentials"
)

func TestClient_ApplyAuth_WithCredential(t *testing.T) {
	// Criar um manager vazio
	mgr := credentials.NewManager(nil)

	// Registrar uma credencial
	auth := &credentials.AuthConfig{
		Type:  "bearer",
		Token: "secret-token-123",
	}
	_ = mgr.RegisterPattern("api-token", auth)

	client := New(&Config{
		CredentialManager: mgr,
	}, map[string]string{
		"api.example.com": "api-token",
	})

	req, _ := http.NewRequest("GET", "http://api.example.com/v1/test", nil)
	client.applyAuth(context.Background(), req)

	headerAuth := req.Header.Get("Authorization")
	if headerAuth != "Bearer secret-token-123" {
		t.Errorf("Expected 'Bearer secret-token-123', got '%s'", headerAuth)
	}
}

func TestClient_ApplyAuth_NoCredential(t *testing.T) {
	mgr := credentials.NewManager(nil)

	client := New(&Config{
		CredentialManager: mgr,
	}, map[string]string{
		"api.example.com": "missing-token",
	})

	req, _ := http.NewRequest("GET", "http://api.example.com/v1/test", nil)
	client.applyAuth(context.Background(), req)

	headerAuth := req.Header.Get("Authorization")
	if headerAuth != "" {
		t.Errorf("Expected no Authorization header, got '%s'", headerAuth)
	}
}

func TestClient_ApplyAuth_NoDomainPattern(t *testing.T) {
	mgr := credentials.NewManager(nil)

	client := New(&Config{
		CredentialManager: mgr,
	}, map[string]string{})

	req, _ := http.NewRequest("GET", "http://unknown.example.com/v1/test", nil)
	client.applyAuth(context.Background(), req)

	headerAuth := req.Header.Get("Authorization")
	if headerAuth != "" {
		t.Errorf("Expected no Authorization header, got '%s'", headerAuth)
	}
}

func TestClient_ApplyAuth_WildcardPattern(t *testing.T) {
	mgr := credentials.NewManager(nil)

	auth := &credentials.AuthConfig{
		Type:  "bearer",
		Token: "wildcard-token",
	}
	_ = mgr.RegisterPattern("default-auth", auth)

	client := New(&Config{
		CredentialManager: mgr,
	}, map[string]string{
		"*": "default-auth",
	})

	req, _ := http.NewRequest("GET", "http://any-domain.example.com/v1/test", nil)
	client.applyAuth(context.Background(), req)

	headerAuth := req.Header.Get("Authorization")
	if headerAuth != "Bearer wildcard-token" {
		t.Errorf("Expected 'Bearer wildcard-token', got '%s'", headerAuth)
	}
}

func TestClient_Do_WithServer(t *testing.T) {
	// Criar servidor mock
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		headerAuth := r.Header.Get("Authorization")
		if headerAuth != "Bearer test-token" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	mgr := credentials.NewManager(nil)

	auth := &credentials.AuthConfig{
		Type:  "bearer",
		Token: "test-token",
	}
	_ = mgr.RegisterPattern("test-cred", auth)

	client := New(&Config{
		CredentialManager: mgr,
	}, map[string]string{})

	// Adicionar domínio do servidor dinamicamente
	client.AddDomainPattern(server.URL[7:], "test-cred") // remove "http://"

	req, _ := http.NewRequest("GET", server.URL+"/test", nil)
	resp, err := client.Do(context.Background(), req)

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}
}

func TestClient_ApplyAuth_FallbackResolveForURL(t *testing.T) {
	mgr := credentials.NewManager(nil)

	auth := &credentials.AuthConfig{
		Type:  "bearer",
		Token: "github-token-from-keyring",
	}
	_ = mgr.RegisterPattern("api.github.com", auth)

	// Empty domainPatterns simulates how http_request tool creates the client
	client := New(&Config{
		CredentialManager: mgr,
	}, map[string]string{})

	req, _ := http.NewRequest("GET", "https://api.github.com/user", nil)
	client.applyAuth(context.Background(), req)

	headerAuth := req.Header.Get("Authorization")
	if headerAuth != "Bearer github-token-from-keyring" {
		t.Errorf("Expected 'Bearer github-token-from-keyring', got '%s'", headerAuth)
	}
}

func TestClient_ApplyAuth_FallbackWildcard(t *testing.T) {
	mgr := credentials.NewManager(nil)

	auth := &credentials.AuthConfig{
		Type:  "bearer",
		Token: "wildcard-github-token",
	}
	_ = mgr.RegisterPattern("*.github.com", auth)

	client := New(&Config{
		CredentialManager: mgr,
	}, map[string]string{})

	req, _ := http.NewRequest("GET", "https://api.github.com/repos", nil)
	client.applyAuth(context.Background(), req)

	headerAuth := req.Header.Get("Authorization")
	if headerAuth != "Bearer wildcard-github-token" {
		t.Errorf("Expected 'Bearer wildcard-github-token', got '%s'", headerAuth)
	}
}

func TestClient_ApplyAuth_ExistingAuthNotOverwritten(t *testing.T) {
	mgr := credentials.NewManager(nil)

	auth := &credentials.AuthConfig{
		Type:  "bearer",
		Token: "from-credential-manager",
	}
	_ = mgr.RegisterPattern("api.github.com", auth)

	client := New(&Config{
		CredentialManager: mgr,
	}, map[string]string{})

	req, _ := http.NewRequest("GET", "https://api.github.com/user", nil)
	req.Header.Set("Authorization", "Bearer explicit-user-token")
	client.applyAuth(context.Background(), req)

	headerAuth := req.Header.Get("Authorization")
	if headerAuth != "Bearer explicit-user-token" {
		t.Errorf("Expected explicit token to be preserved, got '%s'", headerAuth)
	}
}

func TestClient_AddDomainPattern(t *testing.T) {
	mgr := credentials.NewManager(nil)

	client := New(&Config{
		CredentialManager: mgr,
	}, map[string]string{})

	client.AddDomainPattern("new-domain.com", "new-pattern")

	if pattern, ok := client.domainPatterns["new-domain.com"]; !ok || pattern != "new-pattern" {
		t.Error("AddDomainPattern failed")
	}
}
