package main

import (
	"context"
	"net/http"
	"net/http/httptest"
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

	req := TestLLMProviderRequest{
		Type:    "openai",
		BaseURL: "not a valid url",
		APIKey:  "sk-test",
	}

	_, err := app.TestLLMProvider(req)
	if err == nil {
		t.Error("Expected error for invalid URL, got nil")
	}
}

// TestTestLLMProviderSuccessfulConnection testa conexão bem-sucedida (HTTP 200)
func TestTestLLMProviderSuccessfulConnection(t *testing.T) {

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
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

// TestTestLLMProviderWithBearerToken testa autenticação com Bearer Token
func TestTestLLMProviderWithBearerToken(t *testing.T) {

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if auth != "Bearer sk-test123" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
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
		w.Write([]byte("OK"))
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

	// URL inválida que vai dar erro de conexão
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
		w.Write([]byte("OK"))
	}))
	defer server.Close()

	app := setupTestApp()

	// Testar com e sem trailing slash
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
