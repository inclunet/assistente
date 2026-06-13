package http

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"time"
)

// BlockedIPError é devolvido quando uma conexão é recusada por apontar, após a
// resolução de DNS, para um IP local/privado. Tipado para permitir asserções nos
// testes sem casar substrings.
type BlockedIPError struct {
	Host string
	IP   net.IP
}

func (e *BlockedIPError) Error() string {
	if e.Host != "" {
		return fmt.Sprintf("conexão bloqueada (anti-SSRF): host %q resolve para IP local/privado %s", e.Host, e.IP)
	}
	return fmt.Sprintf("conexão bloqueada (anti-SSRF): IP local/privado %s", e.IP)
}

// ipResolver é o subconjunto de *net.Resolver usado pelo guardedDialer. Abstraído
// para permitir injetar um resolver falso nos testes (e validar o caminho pós-DNS
// sem depender de DNS real).
type ipResolver interface {
	LookupIPAddr(ctx context.Context, host string) ([]net.IPAddr, error)
}

// guardedDialer resolve o host, valida CADA IP candidato com isBlockedIP e só então
// conecta — fixando o IP já validado. Isso fecha o TOCTOU/DNS rebinding (não
// re-resolve entre o check e o dial) e cobre formas numéricas não-padrão (o SO as
// normaliza ao resolver). Como o http.Transport é reusado, redirects passam pelo
// mesmo guard automaticamente.
type guardedDialer struct {
	resolver     ipResolver
	dialIP       func(ctx context.Context, network, address string) (net.Conn, error)
	allowPrivate func() bool
}

// DialContext é instalado no http.Transport.DialContext.
func (g *guardedDialer) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, err
	}

	allow := g.allowPrivate != nil && g.allowPrivate()

	// Caso o address já seja um IP literal (inclui IPv4-mapped IPv6 e formas que
	// net.ParseIP normaliza), valida direto sem resolução.
	if ip := net.ParseIP(host); ip != nil {
		if !allow && isBlockedIP(ip) {
			return nil, &BlockedIPError{IP: ip}
		}
		return g.dialIP(ctx, network, net.JoinHostPort(ip.String(), port))
	}

	// Hostname: resolve e valida o IP REAL pós-DNS.
	ips, err := g.resolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, err
	}
	if len(ips) == 0 {
		return nil, fmt.Errorf("nenhum IP resolvido para %q", host)
	}

	// Política fail-closed: se QUALQUER candidato cai em range local/privado, recusa
	// o host inteiro (defesa contra rebinding com múltiplos registros A/AAAA).
	if !allow {
		for _, ipa := range ips {
			if isBlockedIP(ipa.IP) {
				return nil, &BlockedIPError{Host: host, IP: ipa.IP}
			}
		}
	}

	// Conecta fixando os IPs já validados (sem re-resolver), tentando em ordem.
	var firstErr error
	for _, ipa := range ips {
		conn, derr := g.dialIP(ctx, network, net.JoinHostPort(ipa.IP.String(), port))
		if derr != nil {
			if firstErr == nil {
				firstErr = derr
			}
			continue
		}
		return conn, nil
	}
	return nil, firstErr
}

// NewGuardedTransport devolve um *http.Transport (clone dos defaults do net/http)
// cujo DialContext valida o IP real pós-resolução de DNS, barrando ranges
// locais/privados/link-local/CGNAT/etc. allowPrivate libera essa checagem em runtime
// (ex.: testes com httptest, que usam 127.0.0.1); pode ser nil (= sempre barrar).
func NewGuardedTransport(allowPrivate func() bool) *http.Transport {
	// Assertion segura: http.DefaultTransport pode ter sido substituído por outro
	// http.RoundTripper (ex.: em testes), o que faria a assertion direta entrar em
	// panic. Nesse caso partimos de um http.Transport zerado.
	def, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		def = &http.Transport{}
	}
	base := def.Clone()
	// Conexão direta obrigatória (Proxy=nil): o clone dos defaults usa
	// ProxyFromEnvironment, mas com proxy o DialContext validaria/dialaria o IP do
	// PROXY, não o do destino — o que (a) quebraria ambientes com proxy interno
	// (10.x/192.168.x) e (b) abriria um bypass do guard (uma URL para IP privado
	// seria alcançada via proxy público sem o IP do destino ser validado). A política
	// anti-SSRF aqui é conexão direta com o IP real validado pós-DNS.
	base.Proxy = nil
	d := &net.Dialer{Timeout: 30 * time.Second, KeepAlive: 30 * time.Second}
	g := &guardedDialer{
		resolver:     net.DefaultResolver,
		dialIP:       d.DialContext,
		allowPrivate: allowPrivate,
	}
	base.DialContext = g.DialContext
	return base
}

// SetTransportGuard instala o GuardedTransport no *http.Client informado. É a
// barreira anti-SSRF definitiva (pós-DNS) complementar ao IsPrivateHost (pré-dial)
// e ao RedirectGuard. allowPrivate libera hosts privados em runtime (testes).
func SetTransportGuard(bc *http.Client, allowPrivate func() bool) {
	if bc == nil {
		return
	}
	bc.Transport = NewGuardedTransport(allowPrivate)
}
