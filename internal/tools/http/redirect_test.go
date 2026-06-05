package http

import (
	"context"
	"net/http"
	"net/url"
	"testing"
)

func mustReq(t *testing.T, rawURL string) *http.Request {
	t.Helper()
	u, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("url.Parse(%q): %v", rawURL, err)
	}
	return &http.Request{URL: u}
}

func TestRedirectGuard(t *testing.T) {
	denyPrivate := RedirectGuard(DefaultMaxRedirects, func() bool { return false })

	// Redirect para host privado é bloqueado quando allowPrivate=false.
	if err := denyPrivate(mustReq(t, "http://127.0.0.1/x"), nil); err == nil {
		t.Error("esperado erro ao redirecionar para host privado")
	}
	// Scheme não-http(s) é sempre bloqueado.
	if err := denyPrivate(mustReq(t, "ftp://example.com/x"), nil); err == nil {
		t.Error("esperado erro ao redirecionar para scheme não-http")
	}
	// Host público é permitido.
	if err := denyPrivate(mustReq(t, "https://example.com/x"), nil); err != nil {
		t.Errorf("host público não deveria ser bloqueado: %v", err)
	}

	// Quando allowPrivate=true (ex.: testes), host privado é liberado.
	allowPrivate := RedirectGuard(DefaultMaxRedirects, func() bool { return true })
	if err := allowPrivate(mustReq(t, "http://127.0.0.1/x"), nil); err != nil {
		t.Errorf("com allowPrivate=true, host privado deveria passar: %v", err)
	}

	// Limite de redirects: até DefaultMaxRedirects saltos passam; acima falha.
	atLimit := make([]*http.Request, DefaultMaxRedirects)
	if err := denyPrivate(mustReq(t, "https://example.com/x"), atLimit); err != nil {
		t.Errorf("exatamente %d saltos deveria passar: %v", DefaultMaxRedirects, err)
	}
	over := make([]*http.Request, DefaultMaxRedirects+1)
	if err := denyPrivate(mustReq(t, "https://example.com/x"), over); err == nil {
		t.Error("acima do limite de redirects deveria falhar")
	}
}

func TestRedirectGuardStripsCredentialsOnHostChange(t *testing.T) {
	guard := RedirectGuard(DefaultMaxRedirects, func() bool { return false })
	orig := mustReq(t, "https://api.exemplo.com/start")

	// Próximo request com credenciais herdadas, indo para outro host.
	next := mustReq(t, "https://atacante.com/loot")
	next.Header = http.Header{}
	next.Header.Set("Authorization", "Bearer segredo")
	next.Header.Set("X-Api-Key", "chave-secreta")
	next.Header.Set("Accept", "application/json")
	// Propaga, como o cliente faria, os headers de credencial custom injetados.
	ctx := context.WithValue(context.Background(), appliedAuthHeadersKey{}, []string{"X-Api-Key"})
	next = next.WithContext(ctx)

	if err := guard(next, []*http.Request{orig}); err != nil {
		t.Fatalf("redirect para host público não deveria ser bloqueado: %v", err)
	}
	if next.Header.Get("Authorization") != "" {
		t.Error("Authorization deveria ter sido removido ao mudar de host")
	}
	if next.Header.Get("X-Api-Key") != "" {
		t.Error("header de credencial custom deveria ter sido removido ao mudar de host")
	}
	if next.Header.Get("Accept") != "application/json" {
		t.Error("headers não-sensíveis (Accept) NÃO deveriam ser removidos")
	}

	// Mesmo host (subpath): credenciais são preservadas.
	same := mustReq(t, "https://api.exemplo.com/outro")
	same.Header = http.Header{}
	same.Header.Set("X-Api-Key", "chave-secreta")
	same = same.WithContext(context.WithValue(context.Background(), appliedAuthHeadersKey{}, []string{"X-Api-Key"}))
	if err := guard(same, []*http.Request{orig}); err != nil {
		t.Fatalf("mesmo host não deveria ser bloqueado: %v", err)
	}
	if same.Header.Get("X-Api-Key") != "chave-secreta" {
		t.Error("no mesmo host as credenciais deveriam ser preservadas")
	}

	// Mudança apenas de porta também é mudança de host: outro serviço na mesma
	// máquina não deve receber as credenciais.
	otherPort := mustReq(t, "https://api.exemplo.com:8443/x")
	otherPort.Header = http.Header{}
	otherPort.Header.Set("X-Api-Key", "chave-secreta")
	otherPort = otherPort.WithContext(context.WithValue(context.Background(), appliedAuthHeadersKey{}, []string{"X-Api-Key"}))
	if err := guard(otherPort, []*http.Request{orig}); err != nil {
		t.Fatalf("mudança de porta para host público não deveria ser bloqueada: %v", err)
	}
	if otherPort.Header.Get("X-Api-Key") != "" {
		t.Error("credencial deveria ser removida ao mudar a porta (host:port diferente)")
	}
}
