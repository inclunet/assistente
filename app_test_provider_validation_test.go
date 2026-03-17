package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"assistente/internal/credentials"
	"assistente/internal/llm"
)

func setupTestApp() *App {
	credMgr := credentials.NewManager([]byte("test-key-exactly-32-bytes-long!!"))
	llmRegistry := llm.NewProviderRegistry()
	return &App{
		ctx:         context.Background(),
		credMgr:     credMgr,
		llmRegistry: llmRegistry,
	}
}

// TestTestLLMProviderValidatesEmptyUrl testa erro quando URL está vazia
func TestTestLLMProviderValidatesEmptyUrl(t *testing.T) {
	app := setupTestApp()

	req := TestLLMProviderRequest{
		Type:    "openai",
		BaseURL: "",
		APIKey:  "sk-test",
	}

	_, err := app.TestLLMProvider(req)
	if err == nil {
		t.Error("Expected error for empty URL, got nil")
	}
	if err.Error() != "base_url é obrigatório" {
		t.Errorf("Expected 'base_url é obrigatório', got: %v", err)
	}
}

// TestTestLLMProviderValidatesInvalidUrl testa erro quando URL é inválida
func TestTestLLMProviderValidatesInvalidUrl(t *testing.T) {
	app := setupTestApp()

	tests := []struct {
		name    string
		baseURL string
	}{
		{"no scheme", "not a valid url"},
		{"ftp scheme", "ftp://example.com"},
		{"empty host", "http://"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := TestLLMProviderRequest{Type: "openai", BaseURL: tt.baseURL, APIKey: "sk-test"}
			_, err := app.TestLLMProvider(req)
			if err == nil {
				t.Errorf("Expected error for %q, got nil", tt.baseURL)
			}
		})
	}
}

// TestTestLLMProviderSuccessfulConnection testa conexão bem-sucedida (HTTP 200)
func TestTestLLMProviderSuccessfulConnection(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"data":[]}`))
	}))
	defer server.Close()

	app := setupTestApp()

	req := TestLLMProviderRequest{
		Type:    "ollama",
		BaseURL: server.URL,
		APIKey:  "",
	}

	result, err := app.TestLLMProvider(req)
	if err != nil {
		t.Errorf("Expected success, got error: %v", err)
	}
	if !result {
		t.Error("Expected true for successful connection, got false")
	}
}

// TestTestLLMProviderHitsModelsEndpoint verifica que o teste usa /models
func TestTestLLMProviderHitsModelsEndpoint(t *testing.T) {
	var requestedPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestedPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"data":[]}`))
	}))
	defer server.Close()

	app := setupTestApp()
	app.TestLLMProvider(TestLLMProviderRequest{Type: "openai", BaseURL: server.URL, APIKey: "sk-test"})

	if requestedPath != "/models" {
		t.Errorf("expected request to /models, got %s", requestedPath)
	}
}

// TestTestLLMProviderWithBearerToken testa autenticação com Bearer Token
func TestTestLLMProviderWithBearerToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if auth != "Bearer sk-test123" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"data":[]}`))
	}))
	defer server.Close()

	app := setupTestApp()

	req := TestLLMProviderRequest{
		Type:    "openai",
		BaseURL: server.URL,
		APIKey:  "sk-test123",
	}

	result, err := app.TestLLMProvider(req)
	if err != nil {
		t.Errorf("Expected success with valid token, got error: %v", err)
	}
	if !result {
		t.Error("Expected true with valid token, got false")
	}
}

// TestTestLLMProviderWithoutApiKey testa conexão sem API Key (provedor local)
func TestTestLLMProviderWithoutApiKey(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if auth != "" {
			t.Errorf("Expected no Authorization header, got: %s", auth)
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"data":[]}`))
	}))
	defer server.Close()

	app := setupTestApp()

	req := TestLLMProviderRequest{
		Type:    "ollama",
		BaseURL: server.URL,
		APIKey:  "",
	}

	result, err := app.TestLLMProvider(req)
	if err != nil {
		t.Errorf("Expected success without API key, got error: %v", err)
	}
	if !result {
		t.Error("Expected true without API key, got false")
	}
}

// TestTestLLMProviderUnauthorized testa erro 401 (API Key inválida)
func TestTestLLMProviderUnauthorized(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte("Unauthorized"))
	}))
	defer server.Close()

	app := setupTestApp()

	req := TestLLMProviderRequest{
		Type:    "openai",
		BaseURL: server.URL,
		APIKey:  "invalid-key",
	}

	_, err := app.TestLLMProvider(req)
	if err == nil {
		t.Error("Expected error for 401, got nil")
	}
	if err.Error() != "API Key inválida ou não autorizada" {
		t.Errorf("Expected 'API Key inválida...', got: %v", err)
	}
}

// TestTestLLMProviderForbidden testa erro 403
func TestTestLLMProviderForbidden(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer server.Close()

	app := setupTestApp()
	_, err := app.TestLLMProvider(TestLLMProviderRequest{Type: "openai", BaseURL: server.URL, APIKey: "key"})
	if err == nil {
		t.Error("Expected error for 403, got nil")
	}
	if !strings.Contains(err.Error(), "403") {
		t.Errorf("Expected error mentioning 403, got: %v", err)
	}
}

// TestTestLLMProviderServerError testa erro 500 do servidor
func TestTestLLMProviderServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("Internal Server Error"))
	}))
	defer server.Close()

	app := setupTestApp()

	req := TestLLMProviderRequest{
		Type:    "openai",
		BaseURL: server.URL,
		APIKey:  "sk-test",
	}

	_, err := app.TestLLMProvider(req)
	if err == nil {
		t.Error("Expected error for 500, got nil")
	}
	if err.Error() != "servidor retornou erro: 500" {
		t.Errorf("Expected 'servidor retornou erro: 500', got: %v", err)
	}
}

// TestTestLLMProviderNotFound testa resposta 404 (ainda considera sucesso)
func TestTestLLMProviderNotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte("Not Found"))
	}))
	defer server.Close()

	app := setupTestApp()

	req := TestLLMProviderRequest{
		Type:    "custom",
		BaseURL: server.URL,
		APIKey:  "",
	}

	result, err := app.TestLLMProvider(req)
	if err != nil {
		t.Errorf("404 should be success (server responded), got error: %v", err)
	}
	if !result {
		t.Error("404 should return true (server responded)")
	}
}

// TestTestLLMProviderConnectionRefused testa erro de conexão recusada
func TestTestLLMProviderConnectionRefused(t *testing.T) {
	app := setupTestApp()

	req := TestLLMProviderRequest{
		Type:    "ollama",
		BaseURL: "http://localhost:99999",
		APIKey:  "",
	}

	_, err := app.TestLLMProvider(req)
	if err == nil {
		t.Error("Expected error for connection refused, got nil")
	}
}

// TestTestLLMProviderURLTrailingSlash testa normalização de URL
func TestTestLLMProviderURLTrailingSlash(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"data":[]}`))
	}))
	defer server.Close()

	app := setupTestApp()

	req := TestLLMProviderRequest{
		Type:    "ollama",
		BaseURL: server.URL + "/",
		APIKey:  "",
	}

	result, err := app.TestLLMProvider(req)
	if err != nil {
		t.Errorf("Expected success, got error: %v", err)
	}
	if !result {
		t.Error("Expected true, got false")
	}
}

// TestTestLLMProviderUsesExistingCredential verifica que provider_id faz lookup da credencial
func TestTestLLMProviderUsesExistingCredential(t *testing.T) {
	var receivedAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedAuth = r.Header.Get("Authorization")
		if receivedAuth != "Bearer sk-existing-secret" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"data":[{"id":"gpt-4o"}]}`))
	}))
	defer server.Close()

	app := setupTestApp()

	// Registrar provider e credencial no registry/credMgr
	hostname := strings.TrimPrefix(server.URL, "http://")
	provider := &llm.ProviderConfig{
		ID:                "test-provider",
		Name:              "Test",
		Type:              llm.ProviderOpenAI,
		BaseURL:           server.URL,
		CredentialPattern: hostname,
	}
	_ = app.llmRegistry.Register(provider)
	_ = app.credMgr.RegisterPatternWithContext(context.Background(), hostname, &credentials.AuthConfig{
		Type:  "bearer",
		Token: "sk-existing-secret",
	})

	// Testar SEM api_key mas COM provider_id — deve usar a credencial existente
	result, err := app.TestLLMProvider(TestLLMProviderRequest{
		Type:       "openai",
		BaseURL:    server.URL,
		APIKey:     "",
		ProviderID: "test-provider",
	})

	if err != nil {
		t.Fatalf("Expected success with existing credential, got: %v", err)
	}
	if !result {
		t.Error("Expected true")
	}
	if receivedAuth != "Bearer sk-existing-secret" {
		t.Errorf("Expected Bearer header from existing credential, got %q", receivedAuth)
	}
}

// TestTestLLMProviderWithoutProviderID_NoAuth verifica que sem provider_id não busca credencial
func TestTestLLMProviderWithoutProviderID_NoAuth(t *testing.T) {
	var receivedAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"data":[]}`))
	}))
	defer server.Close()

	app := setupTestApp()

	// Sem provider_id e sem api_key → não deve enviar auth
	app.TestLLMProvider(TestLLMProviderRequest{
		Type:    "ollama",
		BaseURL: server.URL,
		APIKey:  "",
	})

	if receivedAuth != "" {
		t.Errorf("Expected no auth header, got %q", receivedAuth)
	}
}
