package http

import (
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
