package credentials

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"assistente/internal/database"
)

// captureTransport é um RoundTripper de teste que captura o request
// em vez de enviá-lo pela rede, para inspecionar headers.
type captureTransport struct {
	captured *http.Request
}

func (c *captureTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	c.captured = req
	return &http.Response{StatusCode: 200, Body: http.NoBody}, nil
}

func newTestManager(t *testing.T) *Manager {
	t.Helper()
	dek := make([]byte, 32) // zero key, suficiente para testes
	mgr := NewManager(dek)
	return mgr
}

func TestTransport_BearerTokenInjected(t *testing.T) {
	mgr := newTestManager(t)
	if err := mgr.RegisterPattern("api.openai.com", &AuthConfig{
		Type:  "bearer",
		Token: "sk-real-key-12345",
	}); err != nil {
		t.Fatal(err)
	}

	capture := &captureTransport{}
	transport := &CredentialTransport{
		Base:        capture,
		CredMgr:     mgr,
		CredPattern: "api.openai.com",
	}

	req := httptest.NewRequest("POST", "https://api.openai.com/v1/responses", nil)
	// Simular o que o SDK faz: coloca o placeholder
	req.Header.Set("Authorization", "Bearer managed-by-credential-transport")

	_, err := transport.RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}

	got := capture.captured.Header.Get("Authorization")
	if got != "Bearer sk-real-key-12345" {
		t.Errorf("Authorization header = %q, esperado %q", got, "Bearer sk-real-key-12345")
	}
}

func TestTransport_PlaceholderReplacedNotSent(t *testing.T) {
	mgr := newTestManager(t)
	if err := mgr.RegisterPattern("llm.inclunet.com.br", &AuthConfig{
		Type:  "bearer",
		Token: "sk-litellm-key",
	}); err != nil {
		t.Fatal(err)
	}

	capture := &captureTransport{}
	transport := &CredentialTransport{
		Base:        capture,
		CredMgr:     mgr,
		CredPattern: "llm.inclunet.com.br",
	}

	req := httptest.NewRequest("POST", "http://llm.inclunet.com.br/v1/responses", nil)
	req.Header.Set("Authorization", "Bearer managed-by-credential-transport")

	_, err := transport.RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}

	got := capture.captured.Header.Get("Authorization")
	if got == "Bearer managed-by-credential-transport" {
		t.Fatal("placeholder 'managed-by-credential-transport' NÃO foi substituído pelo token real — este é o bug de auth que causa 401")
	}
	if got != "Bearer sk-litellm-key" {
		t.Errorf("Authorization = %q, esperado %q", got, "Bearer sk-litellm-key")
	}
}

func TestTransport_UsesRequestContextUserScope(t *testing.T) {
	mgr := newTestManager(t)
	userCtx := database.WithUserID(t.Context(), "user-1")
	otherCtx := database.WithUserID(t.Context(), "user-2")
	if err := mgr.RegisterPatternWithContext(userCtx, "llm.inclunet.com.br", &AuthConfig{
		Type:  "bearer",
		Token: "sk-user-1",
	}); err != nil {
		t.Fatal(err)
	}
	if err := mgr.RegisterPatternWithContext(otherCtx, "llm.inclunet.com.br", &AuthConfig{
		Type:  "bearer",
		Token: "sk-user-2",
	}); err != nil {
		t.Fatal(err)
	}

	capture := &captureTransport{}
	transport := &CredentialTransport{
		Base:        capture,
		CredMgr:     mgr,
		CredPattern: "llm.inclunet.com.br",
	}

	req := httptest.NewRequest("POST", "http://llm.inclunet.com.br/v1/responses", nil).WithContext(userCtx)
	req.Header.Set("Authorization", "Bearer managed-by-credential-transport")

	_, err := transport.RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}

	got := capture.captured.Header.Get("Authorization")
	if got != "Bearer sk-user-1" {
		t.Errorf("Authorization = %q, esperado %q", got, "Bearer sk-user-1")
	}
}

func TestTransport_BasicAuth(t *testing.T) {
	mgr := newTestManager(t)
	if err := mgr.RegisterPattern("basic.example.com", &AuthConfig{
		Type:     "basic",
		Username: "user",
		Password: "pass",
	}); err != nil {
		t.Fatal(err)
	}

	capture := &captureTransport{}
	transport := &CredentialTransport{
		Base:        capture,
		CredMgr:     mgr,
		CredPattern: "basic.example.com",
	}

	req := httptest.NewRequest("GET", "https://basic.example.com/api", nil)
	_, err := transport.RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}

	user, pass, ok := capture.captured.BasicAuth()
	if !ok {
		t.Fatal("Basic auth não foi injetado")
	}
	if user != "user" || pass != "pass" {
		t.Errorf("basic auth = %q:%q, esperado user:pass", user, pass)
	}
}

func TestTransport_CustomHeaders(t *testing.T) {
	mgr := newTestManager(t)
	if err := mgr.RegisterPattern("custom.example.com", &AuthConfig{
		Type: "custom",
		Headers: map[string]string{
			"X-Api-Key":     "key123",
			"X-Custom-Auth": "secret",
		},
	}); err != nil {
		t.Fatal(err)
	}

	capture := &captureTransport{}
	transport := &CredentialTransport{
		Base:        capture,
		CredMgr:     mgr,
		CredPattern: "custom.example.com",
	}

	req := httptest.NewRequest("GET", "https://custom.example.com/api", nil)
	_, err := transport.RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}

	if v := capture.captured.Header.Get("X-Api-Key"); v != "key123" {
		t.Errorf("X-Api-Key = %q, esperado 'key123'", v)
	}
	if v := capture.captured.Header.Get("X-Custom-Auth"); v != "secret" {
		t.Errorf("X-Custom-Auth = %q, esperado 'secret'", v)
	}
}

func TestTransport_NoCredentialFallthrough(t *testing.T) {
	mgr := newTestManager(t)
	// NÃO registrar nenhuma credencial

	capture := &captureTransport{}
	transport := &CredentialTransport{
		Base:        capture,
		CredMgr:     mgr,
		CredPattern: "unknown.example.com",
	}

	req := httptest.NewRequest("GET", "https://unknown.example.com/api", nil)
	req.Header.Set("Authorization", "Bearer original-header")

	_, err := transport.RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}

	// Sem credencial registrada, o header original deve passar inalterado
	got := capture.captured.Header.Get("Authorization")
	if got != "Bearer original-header" {
		t.Errorf("sem credencial registrada, header deveria passar inalterado, obteve %q", got)
	}
}

func TestTransport_NoCredentialWithManagedPlaceholderReturnsError(t *testing.T) {
	mgr := newTestManager(t)
	capture := &captureTransport{}
	transport := &CredentialTransport{
		Base:        capture,
		CredMgr:     mgr,
		CredPattern: "llm.inclunet.com.br",
	}

	req := httptest.NewRequest("POST", "http://llm.inclunet.com.br/v1/responses", nil)
	req.Header.Set("Authorization", "Bearer managed-by-credential-transport")

	resp, err := transport.RoundTrip(req)
	if err == nil {
		t.Fatal("expected unresolved managed credential error")
	}
	if resp != nil {
		t.Fatalf("expected nil response, got %+v", resp)
	}
	if capture.captured != nil {
		t.Fatal("request with unresolved managed credential should not reach base transport")
	}
}

func TestTransport_NilManagerFallthrough(t *testing.T) {
	capture := &captureTransport{}
	transport := &CredentialTransport{
		Base:        capture,
		CredMgr:     nil,
		CredPattern: "api.example.com",
	}

	req := httptest.NewRequest("GET", "https://api.example.com/api", nil)
	req.Header.Set("Authorization", "Bearer original")

	_, err := transport.RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}

	got := capture.captured.Header.Get("Authorization")
	if got != "Bearer original" {
		t.Errorf("com nil manager, header deveria passar inalterado, obteve %q", got)
	}
}

func TestTransport_EmptyTokenNotInjected(t *testing.T) {
	mgr := newTestManager(t)
	if err := mgr.RegisterPattern("empty.example.com", &AuthConfig{
		Type:  "bearer",
		Token: "", // token vazio — não deve sobrescrever
	}); err != nil {
		t.Fatal(err)
	}

	capture := &captureTransport{}
	transport := &CredentialTransport{
		Base:        capture,
		CredMgr:     mgr,
		CredPattern: "empty.example.com",
	}

	req := httptest.NewRequest("GET", "https://empty.example.com/api", nil)
	req.Header.Set("Authorization", "Bearer managed-by-credential-transport")

	resp, err := transport.RoundTrip(req)
	if err == nil {
		t.Fatal("expected unresolved managed credential error for empty token")
	}
	if resp != nil {
		t.Fatalf("expected nil response, got %+v", resp)
	}
	if capture.captured != nil {
		t.Fatal("request with empty managed credential should not reach base transport")
	}
}

func TestTransport_BearerPrefixNotDuplicated(t *testing.T) {
	mgr := newTestManager(t)
	if err := mgr.RegisterPattern("prefixed.example.com", &AuthConfig{
		Type:  "bearer",
		Token: "Bearer already-prefixed",
	}); err != nil {
		t.Fatal(err)
	}

	capture := &captureTransport{}
	transport := &CredentialTransport{
		Base:        capture,
		CredMgr:     mgr,
		CredPattern: "prefixed.example.com",
	}

	req := httptest.NewRequest("GET", "https://prefixed.example.com/api", nil)
	_, err := transport.RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}

	got := capture.captured.Header.Get("Authorization")
	if got != "Bearer already-prefixed" {
		t.Errorf("Authorization = %q, esperado 'Bearer already-prefixed' (sem duplicar prefixo)", got)
	}
}
