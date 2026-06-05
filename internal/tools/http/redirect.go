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
		// Remove credenciais ao sair do host original: o net/http copia os headers
		// para o destino do redirect e só remove Authorization/Cookie em mudança de
		// host — headers de credencial custom (ex.: X-Api-Key) vazariam para outro
		// domínio sem isto.
		stripCredentialsOnHostChange(req, via)
		if allowPrivate != nil && allowPrivate() {
			return nil
		}
		if IsPrivateHost(req.URL.Hostname()) {
			return fmt.Errorf("redirect para host local/privado bloqueado: %s", req.URL.Host)
		}
		return nil
	}
}

// stripCredentialsOnHostChange remove headers sensíveis do próximo request quando
// o destino do redirect não é o host original, evitando vazamento de credenciais
// (domain-scoped) para outro domínio. Cobre os headers padrão (defense-in-depth) e
// os headers de credencial custom injetados pelo cliente, propagados via context.
func stripCredentialsOnHostChange(req *http.Request, via []*http.Request) {
	if len(via) == 0 || via[0] == nil || via[0].URL == nil || req.URL == nil {
		return
	}
	// Compara Host (host:port), não só Hostname(): um redirect que muda apenas a
	// porta (ex.: example.com -> example.com:8443) alcança outro serviço na mesma
	// máquina e também deve descartar as credenciais. Over-strip num caso raro
	// (porta default explícita) é aceitável — segurança acima de conveniência.
	if strings.EqualFold(req.URL.Host, via[0].URL.Host) {
		return
	}
	for _, h := range []string{"Authorization", "Proxy-Authorization", "Cookie", "Cookie2", "Www-Authenticate"} {
		req.Header.Del(h)
	}
	if applied, ok := req.Context().Value(appliedAuthHeadersKey{}).([]string); ok {
		for _, h := range applied {
			req.Header.Del(h)
		}
	}
}
