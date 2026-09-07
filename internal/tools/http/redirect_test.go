package http

import (
	"context"
	"errors"
	"net"
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

// Redirect barrado por host privado literal deve devolver o erro acionável
// (*BlockedDestinationError com sugestões), não um erro seco.
func TestRedirectGuard_PrivateHostActionableError(t *testing.T) {
	guard := RedirectGuard(DefaultMaxRedirects, func() bool { return false })

	err := guard(mustReq(t, "http://127.0.0.1/x"), nil)
	var bde *BlockedDestinationError
	if !errors.As(err, &bde) {
		t.Fatalf("esperado *BlockedDestinationError, got %v", err)
	}
	if bde.Category != CategoryLoopback {
		t.Errorf("categoria esperada loopback, got %q", bde.Category)
	}
	if len(bde.Suggestions) == 0 {
		t.Error("erro acionável deveria trazer sugestões")
	}
}

// markRedirected (chamado pelo guard) deve fazer didRedirect(ctx)=true.
func TestRedirectGuard_MarksRedirect(t *testing.T) {
	guard := RedirectGuard(DefaultMaxRedirects, func() bool { return false })
	ctx := withRedirectTracker(context.Background())
	if didRedirect(ctx) {
		t.Fatal("ctx recém-criado não deveria reportar redirect")
	}
	req := mustReq(t, "https://example.com/x").WithContext(ctx)
	if err := guard(req, nil); err != nil {
		t.Fatalf("host público não deveria ser bloqueado: %v", err)
	}
	if !didRedirect(ctx) {
		t.Fatal("após o guard rodar, didRedirect(ctx) deveria ser true")
	}
}

// Com trust por-request (WithTrustedIPs), um redirect para host privado literal
// é permitido pelo guard (a revalidação do IP real fica a cargo do DialContext).
func TestRedirectGuard_TrustedPrivateRedirectAllowed(t *testing.T) {
	guard := RedirectGuard(DefaultMaxRedirects, func() bool { return false })

	// Sem trust: bloqueado.
	if err := guard(mustReq(t, "http://127.0.0.1/x"), nil); err == nil {
		t.Fatal("sem trust, redirect para host privado deveria ser bloqueado")
	}

	// Com trust por-request: permitido.
	ctx := WithTrustedIPs(context.Background(), []net.IP{net.ParseIP("127.0.0.1")}, "80", false)
	req := mustReq(t, "http://127.0.0.1/x").WithContext(ctx)
	if err := guard(req, nil); err != nil {
		t.Fatalf("com trust por-request, redirect para host privado deveria passar: %v", err)
	}
}

// reqWithHeaders cria um request com headers de credencial típicos (injetados pelo
// credmanager e/ou custom passados pelo chamador) mais um header não-sensível.
func reqWithHeaders(t *testing.T, rawURL string) *http.Request {
	t.Helper()
	r := mustReq(t, rawURL)
	r.Header = http.Header{}
	r.Header.Set("Authorization", "Bearer segredo")
	r.Header.Set("X-Api-Key", "chave-secreta") // custom (chamador ou credmanager)
	r.Header.Set("Cookie", "sid=abc")          // sensível
	r.Header.Set("Accept", "application/json") // não-sensível (allowlist)
	return r
}

func TestRedirectGuardStripsHeadersOnUntrustedRedirect(t *testing.T) {
	guard := RedirectGuard(DefaultMaxRedirects, func() bool { return false })
	orig := mustReq(t, "https://api.exemplo.com/start")

	assertStripped := func(t *testing.T, r *http.Request) {
		t.Helper()
		for _, h := range []string{"Authorization", "X-Api-Key", "Cookie"} {
			if r.Header.Get(h) != "" {
				t.Errorf("%s deveria ter sido removido ao cruzar limite de confiança", h)
			}
		}
		if r.Header.Get("Accept") != "application/json" {
			t.Error("headers não-sensíveis (Accept) NÃO deveriam ser removidos")
		}
	}

	// Mudança de host: remove tudo fora da allowlist (credenciais injetadas E custom
	// passadas pelo chamador).
	other := reqWithHeaders(t, "https://atacante.com/loot")
	if err := guard(other, []*http.Request{orig}); err != nil {
		t.Fatalf("redirect para host público não deveria ser bloqueado: %v", err)
	}
	assertStripped(t, other)

	// Mudança apenas de porta também é mudança de host (outro serviço na mesma máquina).
	otherPort := reqWithHeaders(t, "https://api.exemplo.com:8443/x")
	if err := guard(otherPort, []*http.Request{orig}); err != nil {
		t.Fatalf("mudança de porta não deveria ser bloqueada: %v", err)
	}
	assertStripped(t, otherPort)

	// Downgrade https->http no MESMO host: credenciais não podem ir em texto puro.
	downgrade := reqWithHeaders(t, "http://api.exemplo.com/x")
	if err := guard(downgrade, []*http.Request{orig}); err != nil {
		t.Fatalf("downgrade não deveria ser bloqueado pelo guard: %v", err)
	}
	assertStripped(t, downgrade)

	// Mesmo host:port e mesmo scheme: headers são preservados.
	same := reqWithHeaders(t, "https://api.exemplo.com/outro")
	if err := guard(same, []*http.Request{orig}); err != nil {
		t.Fatalf("mesmo host não deveria ser bloqueado: %v", err)
	}
	if same.Header.Get("X-Api-Key") != "chave-secreta" || same.Header.Get("Authorization") == "" {
		t.Error("no mesmo host/scheme os headers deveriam ser preservados")
	}
}
