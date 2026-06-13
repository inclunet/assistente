package http

import (
	"context"
	"errors"
	"net"
	"testing"
)

// fakeResolver simula a resolução de DNS, permitindo testar o caminho pós-DNS sem
// depender do resolver real do sistema. Mapeia hostname -> IPs (e o caso de
// getaddrinfo que normaliza formas numéricas como "2130706433" -> 127.0.0.1).
type fakeResolver struct {
	hosts map[string][]net.IPAddr
	err   error
}

func (f *fakeResolver) LookupIPAddr(_ context.Context, host string) ([]net.IPAddr, error) {
	if f.err != nil {
		return nil, f.err
	}
	if ips, ok := f.hosts[host]; ok {
		return ips, nil
	}
	return nil, &net.DNSError{Err: "no such host", Name: host, IsNotFound: true}
}

func ipAddrs(ips ...string) []net.IPAddr {
	out := make([]net.IPAddr, 0, len(ips))
	for _, s := range ips {
		out = append(out, net.IPAddr{IP: net.ParseIP(s)})
	}
	return out
}

// newTestDialer monta um guardedDialer com resolver falso e um dialIP que apenas
// registra qual address recebeu (sem abrir socket de verdade).
func newTestDialer(res ipResolver, allowPrivate func() bool) (*guardedDialer, *string) {
	dialed := new(string)
	g := &guardedDialer{
		resolver:     res,
		allowPrivate: allowPrivate,
		dialIP: func(_ context.Context, _, address string) (net.Conn, error) {
			*dialed = address
			return nil, errors.New("dial-stub: conexão não realizada no teste")
		},
	}
	return g, dialed
}

// blocked roda o DialContext e exige que tenha sido barrado por SSRF (sem dial).
func assertBlocked(t *testing.T, g *guardedDialer, dialed *string, address string) {
	t.Helper()
	_, err := g.DialContext(context.Background(), "tcp", address)
	var blockErr *BlockedIPError
	if !errors.As(err, &blockErr) {
		t.Fatalf("%s: esperado BlockedIPError, got %v", address, err)
	}
	if *dialed != "" {
		t.Fatalf("%s: NÃO deveria ter discado, mas discou para %q", address, *dialed)
	}
}

// allowedDial roda o DialContext e exige que tenha CHEGADO ao dialIP (passou no
// guard). O dialIP stub devolve erro, então só checamos que o address foi discado.
func assertReachedDial(t *testing.T, g *guardedDialer, dialed *string, address, wantDialHostPrefix string) {
	t.Helper()
	_, _ = g.DialContext(context.Background(), "tcp", address)
	if *dialed == "" {
		t.Fatalf("%s: deveria ter chegado ao dialIP, mas foi barrado antes", address)
	}
	host, _, err := net.SplitHostPort(*dialed)
	if err != nil {
		t.Fatalf("address discado inválido %q: %v", *dialed, err)
	}
	if host != wantDialHostPrefix {
		t.Fatalf("%s: discou para IP %q, esperado %q", address, host, wantDialHostPrefix)
	}
}

func TestDialContext_HostnameResolvesToPrivate(t *testing.T) {
	res := &fakeResolver{hosts: map[string][]net.IPAddr{
		// Hostname público que resolve para IP privado (DNS rebinding).
		"rebind.example.com": ipAddrs("10.0.0.5"),
		// CNAME para o metadata endpoint de nuvem.
		"metadata.example.com": ipAddrs("169.254.169.254"),
	}}
	g, dialed := newTestDialer(res, nil)
	assertBlocked(t, g, dialed, "rebind.example.com:80")

	g2, dialed2 := newTestDialer(res, nil)
	assertBlocked(t, g2, dialed2, "metadata.example.com:443")
}

func TestDialContext_NumericFormsBlockedAfterResolution(t *testing.T) {
	// Em SOs cujo resolver (getaddrinfo) normaliza formas numéricas não-padrão, o
	// hostname "2130706433"/"0x7f000001" resolve para 127.0.0.1. O guard pós-DNS
	// barra pelo IP resultante.
	res := &fakeResolver{hosts: map[string][]net.IPAddr{
		"2130706433": ipAddrs("127.0.0.1"),
		"0x7f000001": ipAddrs("127.0.0.1"),
	}}
	for _, host := range []string{"2130706433", "0x7f000001"} {
		g, dialed := newTestDialer(res, nil)
		assertBlocked(t, g, dialed, host+":80")
	}
}

func TestDialContext_LiteralIPsBlocked(t *testing.T) {
	// IPs literais (inclui IPv4-mapped IPv6) são barrados sem resolução.
	res := &fakeResolver{}
	for _, host := range []string{"127.0.0.1", "[::ffff:127.0.0.1]", "169.254.169.254", "10.1.2.3", "100.64.0.1"} {
		g, dialed := newTestDialer(res, nil)
		assertBlocked(t, g, dialed, net.JoinHostPort(trimBrackets(host), "80"))
	}
}

func TestDialContext_PublicAllowed(t *testing.T) {
	res := &fakeResolver{hosts: map[string][]net.IPAddr{
		"example.com": ipAddrs("93.184.216.34"),
	}}
	// Hostname público -> chega ao dial com o IP resolvido.
	g, dialed := newTestDialer(res, nil)
	assertReachedDial(t, g, dialed, "example.com:80", "93.184.216.34")

	// IP público literal -> chega ao dial.
	g2, dialed2 := newTestDialer(res, nil)
	assertReachedDial(t, g2, dialed2, "8.8.8.8:443", "8.8.8.8")
}

func TestDialContext_FailClosedOnMixedRecords(t *testing.T) {
	// Múltiplos registros A: um público e um privado. Fail-closed barra o host.
	res := &fakeResolver{hosts: map[string][]net.IPAddr{
		"mixed.example.com": ipAddrs("93.184.216.34", "127.0.0.1"),
	}}
	g, dialed := newTestDialer(res, nil)
	assertBlocked(t, g, dialed, "mixed.example.com:80")
}

func TestDialContext_AllowPrivateBypasses(t *testing.T) {
	res := &fakeResolver{hosts: map[string][]net.IPAddr{
		"local.test": ipAddrs("127.0.0.1"),
	}}
	allow := func() bool { return true }

	// Hostname privado liberado (ex.: testes com httptest).
	g, dialed := newTestDialer(res, allow)
	assertReachedDial(t, g, dialed, "local.test:80", "127.0.0.1")

	// IP literal privado também liberado.
	g2, dialed2 := newTestDialer(res, allow)
	assertReachedDial(t, g2, dialed2, "127.0.0.1:80", "127.0.0.1")
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

func trimBrackets(h string) string {
	if len(h) >= 2 && h[0] == '[' && h[len(h)-1] == ']' {
		return h[1 : len(h)-1]
	}
	return h
}
