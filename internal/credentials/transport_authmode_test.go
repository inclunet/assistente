package credentials

import (
	"net/http/httptest"
	"strings"
	"testing"
)

// Bug 7 (regressão): provedor AuthModeNone (Ollama, llama.cpp puros) NÃO
// deve disparar erro de credencial ausente nem deixar o placeholder
// `managed-by-credential-transport` chegar ao upstream. O transport
// remove qualquer header Authorization residual antes de enviar.
func TestTransport_AuthNone_RemovePlaceholderEAfere(t *testing.T) {
	capture := &captureTransport{}
	transport := &CredentialTransport{
		Base:        capture,
		CredMgr:     nil, // sem credMgr — válido pra AuthNone
		CredPattern: "",
		AuthMode:    AuthNone,
	}

	req := httptest.NewRequest("POST", "http://localhost:11434/api/chat", nil)
	req.Header.Set("Authorization", "Bearer managed-by-credential-transport")

	resp, err := transport.RoundTrip(req)
	if err != nil {
		t.Fatalf("AuthNone não deveria erro, got %v", err)
	}
	if resp == nil {
		t.Fatal("response nil")
	}

	got := capture.captured.Header.Get("Authorization")
	if got != "" {
		t.Errorf("AuthNone deveria remover Authorization residual; veio %q ao upstream", got)
	}
}

// Bug 7 (regressão): provedor AuthModeOptional sem credencial cadastrada
// também segue adiante sem header — diferente do AuthModeRequired que
// dispararia "credencial gerenciada não resolvida".
func TestTransport_AuthOptional_SemCredencial_SegueSemHeader(t *testing.T) {
	mgr := newTestManager(t)
	capture := &captureTransport{}
	transport := &CredentialTransport{
		Base:        capture,
		CredMgr:     mgr,
		CredPattern: "localhost:8080",
		AuthMode:    AuthOptional,
	}

	req := httptest.NewRequest("POST", "http://localhost:8080/v1/chat/completions", nil)
	req.Header.Set("Authorization", "Bearer managed-by-credential-transport")

	resp, err := transport.RoundTrip(req)
	if err != nil {
		t.Fatalf("AuthOptional não deveria erro sem credencial, got %v", err)
	}
	if resp == nil {
		t.Fatal("response nil")
	}
	if got := capture.captured.Header.Get("Authorization"); got != "" {
		t.Errorf("AuthOptional sem credencial deveria limpar Authorization; veio %q", got)
	}
}

// Quando AuthOptional TEM credencial, o token é injetado normalmente
// (mesmo caminho do AuthRequired).
func TestTransport_AuthOptional_ComCredencial_InjetaNormal(t *testing.T) {
	mgr := newTestManager(t)
	if err := mgr.RegisterPattern("localai.local", &AuthConfig{
		Type:  "bearer",
		Token: "optional-token",
	}); err != nil {
		t.Fatalf("RegisterPattern: %v", err)
	}
	capture := &captureTransport{}
	transport := &CredentialTransport{
		Base:        capture,
		CredMgr:     mgr,
		CredPattern: "localai.local",
		AuthMode:    AuthOptional,
	}

	req := httptest.NewRequest("POST", "http://localai.local/v1/chat", nil)
	req.Header.Set("Authorization", "Bearer managed-by-credential-transport")

	if _, err := transport.RoundTrip(req); err != nil {
		t.Fatal(err)
	}
	if got := capture.captured.Header.Get("Authorization"); got != "Bearer optional-token" {
		t.Errorf("Authorization = %q; esperado %q", got, "Bearer optional-token")
	}
}

// Sanidade: AuthRequired sem credencial continua disparando erro
// "credencial gerenciada não resolvida" — o contrato existente para
// providers cloud não pode regredir.
func TestTransport_AuthRequired_SemCredencial_DisparaErro(t *testing.T) {
	mgr := newTestManager(t)
	transport := &CredentialTransport{
		Base:        &captureTransport{},
		CredMgr:     mgr,
		CredPattern: "api.cloud.com",
		AuthMode:    AuthRequired,
	}

	req := httptest.NewRequest("POST", "https://api.cloud.com/v1/x", nil)
	req.Header.Set("Authorization", "Bearer managed-by-credential-transport")

	if _, err := transport.RoundTrip(req); err == nil {
		t.Fatal("AuthRequired sem credencial deveria erro")
	} else if !strings.Contains(err.Error(), "credencial gerenciada não resolvida") {
		t.Errorf("erro inesperado: %v", err)
	}
}
