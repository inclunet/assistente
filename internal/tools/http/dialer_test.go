package http

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

// callControl roda o hook Control do guard para um address (IP:porta) como o
// net.Dialer faria imediatamente antes do connect, devolvendo o erro de política.
func callControl(allowPrivate func() bool, address string) error {
	return ssrfControl(allowPrivate)("tcp", address, nil)
}

func TestSSRFControl_BlocksPrivate(t *testing.T) {
	// IPs reais (já resolvidos) que devem ser barrados no momento do connect. Inclui
	// formas IPv4-mapped IPv6 para garantir a normalização.
	blocked := []string{
		"127.0.0.1:80", "10.1.2.3:443", "192.168.0.1:80", "172.16.0.1:80",
		"169.254.169.254:80", "100.64.0.1:80", "255.255.255.255:80", "0.0.0.0:80",
		"[::1]:80", "[fe80::1]:80",
		// IPv4-mapped IPv6.
		"[::ffff:127.0.0.1]:80", "[::ffff:10.0.0.1]:80", "[::ffff:255.255.255.255]:80",
	}
	for _, addr := range blocked {
		err := callControl(nil, addr)
		var blockErr *BlockedIPError
		if !errors.As(err, &blockErr) {
			t.Errorf("%s: esperado BlockedIPError, got %v", addr, err)
		}
	}
}

func TestSSRFControl_AllowsPublic(t *testing.T) {
	public := []string{"8.8.8.8:80", "1.1.1.1:443", "93.184.216.34:80", "[2606:4700:4700::1111]:443"}
	for _, addr := range public {
		if err := callControl(nil, addr); err != nil {
			t.Errorf("%s: IP público não deveria ser bloqueado: %v", addr, err)
		}
	}
}

func TestSSRFControl_AllowPrivateBypass(t *testing.T) {
	allow := func() bool { return true }
	for _, addr := range []string{"127.0.0.1:80", "10.0.0.1:80", "[::ffff:127.0.0.1]:80"} {
		if err := callControl(allow, addr); err != nil {
			t.Errorf("%s: com allowPrivate=true deveria passar: %v", addr, err)
		}
	}
}

func TestNewGuardedTransport_ProxyDisabled(t *testing.T) {
	// Mesmo com env de proxy configurada, o transport guardado deve ignorá-la: a
	// política anti-SSRF exige conexão direta para validar/dialar o IP do DESTINO
	// real (proxy permitiria bypass do guard pós-DNS).
	t.Setenv("HTTP_PROXY", "http://10.0.0.1:3128")
	t.Setenv("HTTPS_PROXY", "http://10.0.0.1:3128")
	t.Setenv("ALL_PROXY", "http://10.0.0.1:3128")

	tr := NewGuardedTransport(nil)
	if tr.Proxy != nil {
		t.Fatal("GuardedTransport.Proxy deveria ser nil (conexão direta obrigatória)")
	}
}

// rtFunc é um http.RoundTripper arbitrário (não *http.Transport) para simular um
// http.DefaultTransport substituído.
type rtFunc func(*http.Request) (*http.Response, error)

func (f rtFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestNewGuardedTransport_DefaultTransportNotHTTPTransport(t *testing.T) {
	// Se http.DefaultTransport for substituído por outro RoundTripper, a construção
	// não pode entrar em panic: cai num *http.Transport montado com os defaults do
	// net/http (ForceAttemptHTTP2, MaxIdleConns, IdleConnTimeout, TLSHandshakeTimeout,
	// ExpectContinueTimeout), com Proxy=nil e o DialContext de validação do guard.
	orig := http.DefaultTransport
	http.DefaultTransport = rtFunc(func(*http.Request) (*http.Response, error) { return nil, nil })
	defer func() { http.DefaultTransport = orig }()

	var tr *http.Transport
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("NewGuardedTransport entrou em panic: %v", r)
			}
		}()
		tr = NewGuardedTransport(nil)
	}()

	if tr == nil {
		t.Fatal("transport não deveria ser nil")
	}
	if tr.Proxy != nil {
		t.Error("Proxy deveria ser nil (conexão direta obrigatória)")
	}
	if tr.DialContext == nil {
		t.Error("DialContext do guard deveria estar aplicado")
	}
	// No fallback (DefaultTransport customizado), o transport deve ainda ter defaults
	// sensatos do net/http — sem timeouts/limites o risco é hang e consumo de recursos.
	if !tr.ForceAttemptHTTP2 {
		t.Error("ForceAttemptHTTP2 deveria ser true no fallback")
	}
	if tr.MaxIdleConns == 0 {
		t.Error("MaxIdleConns não deveria ser zero no fallback")
	}
	if tr.IdleConnTimeout == 0 {
		t.Error("IdleConnTimeout não deveria ser zero no fallback")
	}
	if tr.TLSHandshakeTimeout == 0 {
		t.Error("TLSHandshakeTimeout não deveria ser zero no fallback")
	}
	if tr.ExpectContinueTimeout == 0 {
		t.Error("ExpectContinueTimeout não deveria ser zero no fallback")
	}
}

// TestGuardedTransport_Integration exercita o caminho real (resolução + Happy
// Eyeballs + Control) com um servidor httptest (que escuta em 127.0.0.1): com
// allowPrivate=false a conexão é barrada; com allowPrivate=true ela funciona. Cobre
// também a ausência de regressão funcional.
func TestGuardedTransport_Integration(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	blockClient := &http.Client{Transport: NewGuardedTransport(func() bool { return false })}
	resp, err := blockClient.Get(srv.URL)
	if err == nil {
		_ = resp.Body.Close()
		t.Fatal("esperado erro ao conectar em 127.0.0.1 com allowPrivate=false")
	}
	var blockErr *BlockedIPError
	if !errors.As(err, &blockErr) {
		t.Fatalf("esperado BlockedIPError na cadeia de erros, got %v", err)
	}

	okClient := &http.Client{Transport: NewGuardedTransport(func() bool { return true })}
	resp2, err := okClient.Get(srv.URL)
	if err != nil {
		t.Fatalf("com allowPrivate=true a conexão deveria funcionar: %v", err)
	}
	defer func() { _ = resp2.Body.Close() }()
	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("status inesperado: %d", resp2.StatusCode)
	}
}
