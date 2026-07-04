package http

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// spyAuthorizer registra chamadas e devolve uma decisão fixa.
type spyAuthorizer struct {
	called     int
	trustedIPs []net.IP
	ok         bool
	err        error
	lastDest   BlockedDestination
}

func (s *spyAuthorizer) Authorize(_ context.Context, dest BlockedDestination) ([]net.IP, bool, error) {
	s.called++
	s.lastDest = dest
	return s.trustedIPs, s.ok, s.err
}

// newTestClient monta um Client com o guard pós-DNS instalado. allowPrivate
// controla se 127.0.0.1 (usado pelo httptest) é tratado como "público".
func newTestClient(allowPrivate bool, auth NetworkAuthorizer) *Client {
	c := New(&Config{}, map[string]string{})
	if bc := c.GetBaseClient(); bc != nil {
		bc.CheckRedirect = RedirectGuard(DefaultMaxRedirects, func() bool { return allowPrivate })
		SetTransportGuard(bc, func() bool { return allowPrivate })
	}
	c.SetNetworkAuthorizer(auth)
	return c
}

func serverIP(t *testing.T, srv *httptest.Server) net.IP {
	t.Helper()
	u := srv.URL
	host := strings.TrimPrefix(u, "http://")
	h, _, err := net.SplitHostPort(host)
	if err != nil {
		t.Fatalf("split host: %v", err)
	}
	ip := net.ParseIP(h)
	if ip == nil {
		t.Fatalf("host do httptest não é IP: %q", h)
	}
	return ip
}

// Cenário 1: host público permitido sem prompt.
func TestClientDo_PublicAllowed_NoPrompt(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	auth := &spyAuthorizer{}
	c := newTestClient(true, auth) // allowPrivate=true simula destino "público"

	req, _ := http.NewRequest("GET", srv.URL, nil)
	resp, err := c.Do(context.Background(), req)
	if err != nil {
		t.Fatalf("request pública deveria funcionar: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if auth.called != 0 {
		t.Fatal("authorizer não deveria ser consultado para destino permitido")
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status inesperado: %d", resp.StatusCode)
	}
}

// Cenário 2: host privado bloqueado sem allowlist (sem authorizer) -> erro estruturado.
func TestClientDo_PrivateBlocked_NoAuthorizer(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := newTestClient(false, nil) // sem authorizer

	req, _ := http.NewRequest("GET", srv.URL, nil)
	_, err := c.Do(context.Background(), req)
	if err == nil {
		t.Fatal("esperado bloqueio anti-SSRF")
	}
	var bde *BlockedDestinationError
	if !errors.As(err, &bde) {
		t.Fatalf("esperado BlockedDestinationError, got %v", err)
	}
	if bde.Host == "" || len(bde.IPs) == 0 || bde.Category == "" {
		t.Fatalf("erro estruturado incompleto: %+v", bde)
	}
}

// Cenário 3 e 7: host privado com autorização temporária -> reexecuta e sucede.
func TestClientDo_PrivateAuthorized_Retries(t *testing.T) {
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits++
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	auth := &spyAuthorizer{trustedIPs: []net.IP{serverIP(t, srv)}, ok: true}
	c := newTestClient(false, auth)

	req, _ := http.NewRequest("GET", srv.URL, nil)
	resp, err := c.Do(context.Background(), req)
	if err != nil {
		t.Fatalf("após autorização a request deveria suceder: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if auth.called != 1 {
		t.Fatalf("authorizer deveria ter sido consultado 1x, got %d", auth.called)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status inesperado: %d", resp.StatusCode)
	}
	if hits != 1 {
		t.Fatalf("servidor deveria receber a request reexecutada 1x, got %d", hits)
	}
	// A decisão foi tomada sobre o IP real resolvido.
	if len(auth.lastDest.IPs) == 0 {
		t.Fatal("dest deveria conter o IP resolvido")
	}
}

// Cenário 7 (com body): a reexecução após autorização deve reenviar o body.
func TestClientDo_PrivateAuthorized_RetriesWithBody(t *testing.T) {
	var received string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		received = string(b)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	auth := &spyAuthorizer{trustedIPs: []net.IP{serverIP(t, srv)}, ok: true}
	c := newTestClient(false, auth)

	req, _ := http.NewRequest("POST", srv.URL, strings.NewReader("payload-123"))
	resp, err := c.Do(context.Background(), req)
	if err != nil {
		t.Fatalf("POST autorizado deveria suceder: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if received != "payload-123" {
		t.Fatalf("body deveria ser reenviado na reexecução, got %q", received)
	}
}

// Cenário 2 (negado explícito): authorizer devolve ok=false -> erro estruturado.
func TestClientDo_AuthorizerDenies(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	auth := &spyAuthorizer{ok: false}
	c := newTestClient(false, auth)

	req, _ := http.NewRequest("GET", srv.URL, nil)
	_, err := c.Do(context.Background(), req)
	var bde *BlockedDestinationError
	if !errors.As(err, &bde) {
		t.Fatalf("negado deveria devolver BlockedDestinationError, got %v", err)
	}
}

// Cenário 6: trust é por IP:porta exato — outro IP privado (e outra porta no
// mesmo IP) continua bloqueado.
func TestSSRFControlWithTrust_OnlyTrustsExactIP(t *testing.T) {
	trusted := map[string]bool{trustKey(normalizeIPKey(net.ParseIP("100.64.1.112")), "443"): true}
	ctl := ssrfControlWithTrust(nil, trusted)

	if err := ctl("tcp", "100.64.1.112:443", nil); err != nil {
		t.Fatalf("IP:porta confiável deveria passar: %v", err)
	}
	// Mesmo IP confiável, mas em porta NÃO autorizada -> bloqueado (fecha o
	// bypass de porta via redirect).
	if err := ctl("tcp", "100.64.1.112:8443", nil); err == nil {
		t.Fatal("porta não autorizada no mesmo IP deveria continuar bloqueada")
	}
	// Outro IP CGNAT (mesma faixa) NÃO está no trust -> bloqueado.
	if err := ctl("tcp", "100.64.1.113:443", nil); err == nil {
		t.Fatal("IP não confiável da mesma faixa deveria continuar bloqueado")
	}
	// IP público sem trust -> passa normalmente.
	if err := ctl("tcp", "8.8.8.8:443", nil); err != nil {
		t.Fatalf("IP público deveria passar: %v", err)
	}
}

// IPv6 link-local com zone id (RFC 6874) deve ser barrado como *BlockedIPError
// (não erro genérico), preservando o fluxo de autorização/erro acionável.
func TestSSRFControlWithTrust_IPv6ZoneID(t *testing.T) {
	ctl := ssrfControlWithTrust(nil, nil)

	err := ctl("tcp", "[fe80::1%en0]:443", nil)
	var blocked *BlockedIPError
	if !errors.As(err, &blocked) {
		t.Fatalf("link-local com zone id deveria virar *BlockedIPError, got %v", err)
	}

	// Mesmo endereço, se estiver no trust (IP sem zona), deve passar.
	trusted := map[string]bool{trustKey(normalizeIPKey(net.ParseIP("fe80::1")), "443"): true}
	if err := ssrfControlWithTrust(nil, trusted)("tcp", "[fe80::1%en0]:443", nil); err != nil {
		t.Fatalf("link-local confiável (com zone id) deveria passar: %v", err)
	}
}

// MostSensitiveCategory deve escolher o pior caso entre todos os IPs (evita que
// o prompt descreva categoria branda enquanto o trust libera IP mais crítico).
func TestMostSensitiveCategory(t *testing.T) {
	// cgnat + metadata na mesma resposta -> metadata (pior caso).
	ips := []net.IP{net.ParseIP("100.64.1.10"), net.ParseIP("169.254.169.254")}
	if got := MostSensitiveCategory("host.example", ips); got != CategoryMetadata {
		t.Fatalf("esperado metadata (pior caso), got %q", got)
	}
	// Só cgnat -> cgnat.
	if got := MostSensitiveCategory("host.example", []net.IP{net.ParseIP("100.64.1.10")}); got != CategoryCGNAT {
		t.Fatalf("esperado cgnat, got %q", got)
	}
	// Host localhost textual prevalece como localhost-alias quando não há IP mais sensível.
	if got := MostSensitiveCategory("localhost", []net.IP{net.ParseIP("127.0.0.1")}); got != CategoryLocalhostAlias {
		t.Fatalf("esperado localhost-alias, got %q", got)
	}
}

// allowedTrustPorts: porta explícita libera só ela; host-level libera default.
func TestAllowedTrustPorts(t *testing.T) {
	if got := allowedTrustPorts("8443", true); len(got) != 1 || got[0] != "8443" {
		t.Fatalf("porta explícita deveria liberar apenas ela, got %v", got)
	}
	got := allowedTrustPorts("443", false)
	has80, has443 := false, false
	for _, p := range got {
		switch p {
		case "80":
			has80 = true
		case "443":
			has443 = true
		}
	}
	if !has80 || !has443 {
		t.Fatalf("host-level deveria liberar portas default (80/443), got %v", got)
	}
}

// WithTrustedIPs (host-level) confia nas portas default; porta não-default no
// mesmo IP não entra no trust.
func TestWithTrustedIPs_HostLevelDefaultPortsOnly(t *testing.T) {
	ip := net.ParseIP("100.64.1.112")
	ctx := WithTrustedIPs(context.Background(), []net.IP{ip}, "443", false)
	set := trustedIPSet(ctx)
	key := normalizeIPKey(ip)
	if !set[trustKey(key, "443")] || !set[trustKey(key, "80")] {
		t.Fatalf("host-level deveria confiar em 80/443, got %v", set)
	}
	if set[trustKey(key, "8443")] {
		t.Fatal("host-level não deveria confiar em porta não-default")
	}
}

// didRedirect distingue um bloqueio na URL diretamente requisitada (sem
// redirect) de um bloqueio ocorrido após um salto de redirect (não abre prompt).
func TestRedirectTracker(t *testing.T) {
	base := context.Background()

	// Sem tracker no ctx → false.
	if didRedirect(base) {
		t.Fatal("ctx sem tracker não deveria reportar redirect")
	}

	// Com tracker mas sem marcação → false.
	ctx := withRedirectTracker(base)
	if didRedirect(ctx) {
		t.Fatal("tracker não marcado não deveria reportar redirect")
	}

	// Após marcar (como faz o RedirectGuard) → true.
	markRedirected(ctx)
	if !didRedirect(ctx) {
		t.Fatal("após markRedirected deveria reportar redirect")
	}
}

// redirectBlockedError não deve atribuir o IP interno a um host, mas ainda
// classifica e traz o IP para a mensagem acionável.
func TestRedirectBlockedError(t *testing.T) {
	err := redirectBlockedError(net.ParseIP("169.254.169.254"))
	if err.Host != "" {
		t.Fatalf("não deveria atribuir host, got %q", err.Host)
	}
	if err.Category != CategoryMetadata {
		t.Fatalf("categoria esperada metadata, got %q", err.Category)
	}
	if len(err.IPs) != 1 || !err.IPs[0].Equal(net.ParseIP("169.254.169.254")) {
		t.Fatalf("IP esperado no erro, got %v", err.IPs)
	}
}

// appendUniqueIP garante que o IP barrado (o que falhou no dial) sempre entra no
// trust, sem duplicar, mesmo quando a resolução DNS devolve um conjunto diferente.
func TestAppendUniqueIP(t *testing.T) {
	base := []net.IP{net.ParseIP("100.64.1.112")}

	// IP diferente é acrescentado.
	got := appendUniqueIP(base, net.ParseIP("10.0.0.1"))
	if len(got) != 2 {
		t.Fatalf("esperado 2 IPs, got %v", got)
	}

	// IP já presente não duplica (inclusive forma IPv4-in-IPv6).
	got = appendUniqueIP(base, net.ParseIP("100.64.1.112"))
	if len(got) != 1 {
		t.Fatalf("IP duplicado não deveria ser acrescentado, got %v", got)
	}

	// nil é ignorado.
	if got := appendUniqueIP(base, nil); len(got) != 1 {
		t.Fatalf("nil deveria ser ignorado, got %v", got)
	}
}

// Cenário 8: mensagem de erro estruturada contém host, IP e categoria.
func TestBlockedDestinationError_Message(t *testing.T) {
	err := &BlockedDestinationError{
		Host:        "api.nu.workflows.dev",
		IPs:         []net.IP{net.ParseIP("100.64.1.112")},
		Category:    CategoryCGNAT,
		Reason:      "cgnat address blocked by anti-SSRF policy",
		Suggestions: defaultBlockSuggestions,
	}
	msg := err.Error()
	for _, want := range []string{"api.nu.workflows.dev", "100.64.1.112", string(CategoryCGNAT)} {
		if !strings.Contains(msg, want) {
			t.Fatalf("mensagem deveria conter %q, got:\n%s", want, msg)
		}
	}
}

// Classify deve mapear as categorias exigidas.
func TestClassify_Categories(t *testing.T) {
	cases := map[string]Category{
		"127.0.0.1":       CategoryLoopback,
		"10.0.0.1":        CategoryPrivateRFC1918,
		"192.168.1.1":     CategoryPrivateRFC1918,
		"100.64.1.112":    CategoryCGNAT,
		"169.254.1.1":     CategoryLinkLocal,
		"169.254.169.254": CategoryMetadata,
		"239.255.255.250": CategoryMulticast,
		"255.255.255.255": CategoryReserved,
		"8.8.8.8":         CategoryPublic,
	}
	for ipStr, want := range cases {
		if got := Classify(net.ParseIP(ipStr)); got != want {
			t.Errorf("Classify(%s) = %q, want %q", ipStr, got, want)
		}
	}
}

// ClassifyDestination deve priorizar o host textual "localhost"/".localhost"
// (RFC 6761) sobre o IP loopback, para que a categoria reportada seja a mais
// informativa e a constante CategoryLocalhostAlias seja de fato exercida.
func TestClassifyDestination_LocalhostAlias(t *testing.T) {
	cases := []struct {
		host string
		ip   net.IP
		want Category
	}{
		{"localhost", net.ParseIP("127.0.0.1"), CategoryLocalhostAlias},
		{"LOCALHOST.", net.ParseIP("127.0.0.1"), CategoryLocalhostAlias},
		{"api.localhost", net.ParseIP("127.0.0.1"), CategoryLocalhostAlias},
		// Host não-alias cai na classificação por IP.
		{"127.0.0.1", net.ParseIP("127.0.0.1"), CategoryLoopback},
		{"internal.corp", net.ParseIP("10.0.0.1"), CategoryPrivateRFC1918},
		{"api.nu.workflows.dev", net.ParseIP("100.64.1.112"), CategoryCGNAT},
	}
	for _, c := range cases {
		if got := ClassifyDestination(c.host, c.ip); got != c.want {
			t.Errorf("ClassifyDestination(%q, %s) = %q, want %q", c.host, c.ip, got, c.want)
		}
	}
}
