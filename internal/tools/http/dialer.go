package http

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"syscall"
	"time"
)

// BlockedIPError é devolvido quando uma conexão é recusada por apontar, no momento
// do connect (já com o IP real resolvido), para um IP local/privado. Tipado para
// permitir asserções nos testes sem casar substrings.
type BlockedIPError struct {
	IP net.IP
}

func (e *BlockedIPError) Error() string {
	// e.IP.String() explicitamente: net.IP tem tipo subjacente []byte e passá-lo
	// direto ao fmt poderia imprimir bytes crus em vez do endereço legível.
	return "conexão bloqueada (anti-SSRF): IP local/privado " + e.IP.String()
}

// ssrfControl devolve a função Control de net.Dialer que valida o IP REAL no momento
// da conexão. O net/http resolve o host (Happy Eyeballs) e, para CADA tentativa de
// socket, chama Control com o `address` já no formato IP:porta concreto ao qual o
// connect() vai ser feito. Validar aqui:
//   - cobre a validação pós-DNS do IP real, sem TOCTOU (o IP checado é exatamente o
//     que será conectado);
//   - cobre DNS rebinding, formas numéricas não-padrão (o SO as normaliza ao
//     resolver) e redirects (o http.Transport é reusado);
//   - é fail-closed: retornar erro aborta aquela tentativa de connect (um host que
//     só resolve para IPs privados falha por completo);
//   - preserva o Happy Eyeballs nativo do net.Dialer (tentativas IPv6/IPv4
//     concorrentes com fallback), sem a regressão de latência de dialar IPs em série.
//
// allowPrivate libera a checagem em runtime (ex.: testes com httptest, que usam
// 127.0.0.1); pode ser nil (= sempre barrar).
func ssrfControl(allowPrivate func() bool) func(network, address string, c syscall.RawConn) error {
	return ssrfControlWithTrust(allowPrivate, nil)
}

// ssrfControlWithTrust é o ssrfControl com um conjunto adicional de IPs
// confiáveis (chaves normalizadas via normalizeIPKey) liberados apesar de caírem
// em faixa bloqueada. É o resultado de uma autorização explícita do usuário para
// ESTA request (ver NetworkAuthorizer / WithTrustedIPs). trusted pode ser nil.
//
// A ordem importa para segurança: a checagem de trust é por IP EXATO já resolvido
// (o mesmo que será conectado), então não afrouxa a faixa inteira — outros IPs
// privados/CGNAT continuam barrados, fechando SSRF por hosts vizinhos.
func ssrfControlWithTrust(allowPrivate func() bool, trusted map[string]bool) func(network, address string, c syscall.RawConn) error {
	return func(_, address string, _ syscall.RawConn) error {
		if allowPrivate != nil && allowPrivate() {
			return nil
		}
		host, _, err := net.SplitHostPort(address)
		if err != nil {
			host = address
		}
		ip := net.ParseIP(host)
		if ip == nil {
			return fmt.Errorf("anti-SSRF: endereço de conexão sem IP válido: %q", address)
		}
		if len(trusted) > 0 && trusted[normalizeIPKey(ip)] {
			return nil
		}
		if isBlockedIP(ip) {
			return &BlockedIPError{IP: ip}
		}
		return nil
	}
}

// NewGuardedTransport devolve um *http.Transport (clone dos defaults do net/http)
// cujo dialer valida, via hook Control, o IP real pós-resolução de DNS — barrando
// ranges locais/privados/link-local/CGNAT/etc. allowPrivate libera essa checagem em
// runtime (ex.: testes com httptest, que usam 127.0.0.1); pode ser nil (= sempre
// barrar).
func NewGuardedTransport(allowPrivate func() bool) *http.Transport {
	// Assertion segura: http.DefaultTransport pode ter sido substituído por outro
	// http.RoundTripper (ex.: em testes), o que faria a assertion direta entrar em
	// panic. No fallback NÃO usamos um http.Transport zerado (perderia timeouts e
	// limites importantes, arriscando hangs/consumo de recursos); construímos um
	// transport com os mesmos defaults do net/http (exceto Proxy, que fica nil pela
	// política anti-SSRF de conexão direta).
	def, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		def = &http.Transport{
			ForceAttemptHTTP2:     true,
			MaxIdleConns:          100,
			IdleConnTimeout:       90 * time.Second,
			TLSHandshakeTimeout:   10 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
		}
	}
	base := def.Clone()
	// Conexão direta obrigatória (Proxy=nil): o clone dos defaults usa
	// ProxyFromEnvironment, mas com proxy o Control validaria o IP do PROXY, não o do
	// destino — o que (a) quebraria ambientes com proxy interno (10.x/192.168.x) e
	// (b) abriria um bypass do guard (uma URL para IP privado seria alcançada via
	// proxy público sem o IP do destino ser validado). A política anti-SSRF aqui é
	// conexão direta com o IP real validado no momento do connect.
	base.Proxy = nil
	// DialContext embrulhado para ler o trust POR-REQUEST do ctx: o hook Control
	// de um net.Dialer não recebe ctx, mas o DialContext sim. Reconstruímos o
	// net.Dialer por chamada com um Control ciente dos IPs confiáveis daquela
	// request (autorizados pelo usuário). Preserva Happy Eyeballs (o net.Dialer
	// nativo resolve e dialha IPv6/IPv4 concorrentes, com o Control validando cada
	// IP concreto antes do connect).
	base.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		d := &net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
			Control:   ssrfControlWithTrust(allowPrivate, trustedIPSet(ctx)),
		}
		return d.DialContext(ctx, network, address)
	}
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
