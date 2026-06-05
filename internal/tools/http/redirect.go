package http

import (
	"fmt"
	"net/http"
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
		if allowPrivate != nil && allowPrivate() {
			return nil
		}
		if IsPrivateHost(req.URL.Hostname()) {
			return fmt.Errorf("redirect para host local/privado bloqueado: %s", req.URL.Host)
		}
		return nil
	}
}
