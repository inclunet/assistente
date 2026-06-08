package providers

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"assistente/internal/credentials"
	"assistente/internal/llm"
	"assistente/internal/profiles"
)

func newHealthService(t *testing.T, provider *llm.ProviderConfig) *Service {
	t.Helper()
	registry := llm.NewProviderRegistry()
	if provider != nil {
		if err := registry.Register(provider); err != nil {
			t.Fatalf("falha ao registrar provider: %v", err)
		}
	}
	return NewService(ServiceConfig{
		Registry: registry,
		CredMgr:  credentials.NewManager(nil),
	})
}

func profileForProvider(providerID string) *profiles.Profile {
	p := &profiles.Profile{}
	p.Chat.LLMProvider = providerID
	p.Chat.Model = "modelo-x"
	return p
}

// TestCheckHealth_Online: endpoint acessível e respondendo 200 em /models →
// estado online e latência medida (>= 0).
func TestCheckHealth_Online(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"object":"list","data":[{"id":"gpt-test"}]}`)
	}))
	defer srv.Close()

	provider := &llm.ProviderConfig{
		ID:       "local-test",
		Name:     "Local Test",
		Type:     llm.ProviderOpenAI,
		BaseURL:  srv.URL + "/v1",
		AuthMode: llm.AuthModeNone,
	}
	svc := newHealthService(t, provider)

	res := svc.CheckHealth(context.Background(), profileForProvider("local-test"))
	if res.State != HealthOnline {
		t.Fatalf("estado esperado online, got %q (err=%q)", res.State, res.Error)
	}
	if !res.Reachable || !res.AuthOK {
		t.Fatalf("esperava reachable+authOK, got reachable=%v authOK=%v", res.Reachable, res.AuthOK)
	}
	if res.ProviderID != "local-test" || res.ProviderName != "Local Test" {
		t.Fatalf("metadados do provider incorretos: %+v", res)
	}
	if res.LatencyMs < 0 {
		t.Fatalf("latência inválida: %d", res.LatencyMs)
	}
}

// TestCheckHealth_OfflineUnreachable: servidor fechado → URL inacessível →
// estado offline.
func TestCheckHealth_OfflineUnreachable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	url := srv.URL
	srv.Close() // garante que a porta está fechada

	provider := &llm.ProviderConfig{
		ID:       "dead-test",
		Name:     "Dead Test",
		Type:     llm.ProviderOpenAI,
		BaseURL:  url + "/v1",
		AuthMode: llm.AuthModeNone,
	}
	svc := newHealthService(t, provider)

	res := svc.CheckHealth(context.Background(), profileForProvider("dead-test"))
	if res.State != HealthOffline {
		t.Fatalf("estado esperado offline, got %q", res.State)
	}
	if res.Reachable {
		t.Fatalf("não deveria estar reachable")
	}
	if res.Error == "" {
		t.Fatalf("offline deveria carregar um detalhe de erro")
	}
}

// TestCheckHealth_OfflineAuth: servidor responde 401 → autenticação falhou →
// estado offline mesmo com URL acessível.
func TestCheckHealth_OfflineAuth(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	provider := &llm.ProviderConfig{
		ID:      "auth-test",
		Name:    "Auth Test",
		Type:    llm.ProviderOpenAI,
		BaseURL: srv.URL + "/v1",
	}
	svc := newHealthService(t, provider)

	res := svc.CheckHealth(context.Background(), profileForProvider("auth-test"))
	if res.State != HealthOffline {
		t.Fatalf("estado esperado offline, got %q", res.State)
	}
	if !res.Reachable {
		t.Fatalf("URL deveria estar reachable (servidor respondeu 401)")
	}
	if res.AuthOK {
		t.Fatalf("authOK não deveria ser true com 401")
	}
}

// TestCheckHealth_ProviderMissing: perfil aponta para provider inexistente →
// offline com ErrorType provider_missing.
func TestCheckHealth_ProviderMissing(t *testing.T) {
	svc := newHealthService(t, nil)

	res := svc.CheckHealth(context.Background(), profileForProvider("nao-existe"))
	if res.State != HealthOffline {
		t.Fatalf("estado esperado offline, got %q", res.State)
	}
	if res.ErrorType != "provider_missing" {
		t.Fatalf("ErrorType esperado provider_missing, got %q", res.ErrorType)
	}
}

// TestCheckHealth_NilProfile: sem perfil ativo → offline.
func TestCheckHealth_NilProfile(t *testing.T) {
	svc := newHealthService(t, nil)
	res := svc.CheckHealth(context.Background(), nil)
	if res.State != HealthOffline {
		t.Fatalf("estado esperado offline para perfil nil, got %q", res.State)
	}
}
