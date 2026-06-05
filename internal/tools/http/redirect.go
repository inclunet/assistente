package http

import (
	"fmt"
	"net/http"
	"strings"
)

// DefaultMaxRedirects é o número máximo de redirects que as tools de rede seguem.
const DefaultMaxRedirects = 10

// RedirectGuard devolve uma função CheckRedirect (para http.Client) que aplica a
// barreira anti-SSRF em redirects: o net/http segue redirects automaticamente, então
// validar só a URL inicial não basta — uma URL pública pode redirecionar para
// http://127.0.0.1/ ou http://169.254.169.254/ e contornar o bloqueio.
//
// Barra redirects para scheme não-http(s), para hosts locais/privados (via
// IsPrivateHost) e acima de maxRedirects saltos. allowPrivate permite liberar hosts
// privados em runtime (ex.: testes com httptest); pode ser nil.
func RedirectGuard(maxRedirects int, allowPrivate func() bool) func(req *http.Request, via []*http.Request) error {
	return func(req *http.Request, via []*http.Request) error {
		// via contém as requisições já feitas; permitimos até maxRedirects saltos e
		// só barramos a partir do seguinte.
		if len(via) > maxRedirects {
			return fmt.Errorf("excesso de redirects (limite %d)", maxRedirects)
		}
		if req.URL.Scheme != "http" && req.URL.Scheme != "https" {
			return fmt.Errorf("redirect para scheme não suportado: %q", req.URL.Scheme)
		}
		// Remove credenciais quando o redirect sai do host original ou faz downgrade
		// https->http: o net/http copia os headers para o destino e só remove
		// Authorization/Cookie em mudança de host — headers de credencial custom
		// (ex.: X-Api-Key) vazariam para outro domínio (ou em texto puro) sem isto.
		stripCredentialsOnUntrustedRedirect(req, via)
		if allowPrivate != nil && allowPrivate() {
			return nil
		}
		if IsPrivateHost(req.URL.Hostname()) {
			return fmt.Errorf("redirect para host local/privado bloqueado: %s", req.URL.Host)
		}
		return nil
	}
}

// stripCredentialsOnUntrustedRedirect remove headers sensíveis do próximo request
// quando o redirect cruza um limite de confiança, evitando vazamento de credenciais
// (domain-scoped). Cobre os headers padrão (defense-in-depth) e os headers de
// credencial custom injetados pelo cliente, propagados via context.
//
// Considera "não confiável":
//   - mudança de host:port (não só Hostname): example.com -> example.com:8443
//     alcança outro serviço na mesma máquina;
//   - downgrade https->http no mesmo host: exporia as credenciais em texto puro.
func stripCredentialsOnUntrustedRedirect(req *http.Request, via []*http.Request) {
	if len(via) == 0 || via[0] == nil || via[0].URL == nil || req.URL == nil {
		return
	}
	orig := via[0].URL
	sameHost := strings.EqualFold(req.URL.Host, orig.Host)
	downgrade := strings.EqualFold(orig.Scheme, "https") && strings.EqualFold(req.URL.Scheme, "http")
	// Over-strip num caso raro (porta default explícita) é aceitável — segurança
	// acima de conveniência.
	if sameHost && !downgrade {
		return
	}
	// Apenas headers de request: Www-Authenticate é header de resposta e não se
	// aplica aqui.
	for _, h := range []string{"Authorization", "Proxy-Authorization", "Cookie", "Cookie2"} {
		req.Header.Del(h)
	}
	if applied, ok := req.Context().Value(appliedAuthHeadersKey{}).([]string); ok {
		for _, h := range applied {
			req.Header.Del(h)
		}
	}
}
