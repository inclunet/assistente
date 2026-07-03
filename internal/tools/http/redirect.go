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
		// Sinaliza que a request seguiu um redirect (o dial inicial teve sucesso).
		// handleBlocked usa isso para não abrir prompt em bloqueios pós-redirect.
		markRedirected(req.Context())
		// via contém as requisições já feitas; permitimos até maxRedirects saltos e
		// só barramos a partir do seguinte.
		if len(via) > maxRedirects {
			return fmt.Errorf("excesso de redirects (limite %d)", maxRedirects)
		}
		if req.URL.Scheme != "http" && req.URL.Scheme != "https" {
			return fmt.Errorf("redirect para scheme não suportado: %q", req.URL.Scheme)
		}
		// Remove headers sensíveis quando o redirect cruza um limite de confiança:
		// o net/http copia os headers para o destino e só remove Authorization/Cookie
		// em mudança de host — qualquer outro header (credencial injetada pelo
		// credmanager OU custom passado pelo chamador, ex.: X-Api-Key) vazaria para
		// outro domínio (ou em texto puro) sem isto.
		stripUnsafeHeadersOnUntrustedRedirect(req, via)
		if allowPrivate != nil && allowPrivate() {
			return ValidateNetworkScope(req.Context(), req.URL.Hostname())
		}
		if IsPrivateHost(req.URL.Hostname()) {
			// Se a request carrega trust por-request (autorização explícita),
			// delega a decisão à barreira pós-DNS (DialContext), que revalida o
			// IP real e só libera IPs exatamente confiáveis. Isso cobre tanto IP
			// literal quanto hosts textuais como "localhost"/".localhost" (que o
			// net.ParseIP não reconhece) sem afrouxar a proteção: um redirect
			// para um IP não confiável continua barrado no dial.
			if hasTrustedIPs(req.Context()) {
				return ValidateNetworkScope(req.Context(), req.URL.Hostname())
			}
			return fmt.Errorf("redirect para host local/privado bloqueado: %s", req.URL.Host)
		}
		return ValidateNetworkScope(req.Context(), req.URL.Hostname())
	}
}

// redirectSafeHeaders é a allowlist de headers que podem atravessar um redirect que
// cruza um limite de confiança. Política deny-by-default: qualquer header fora desta
// lista é removido nesse caso, fechando o vazamento independentemente da origem do
// header (credencial do credmanager, header custom do chamador, etc.).
var redirectSafeHeaders = map[string]bool{
	"User-Agent":      true,
	"Accept":          true,
	"Accept-Language": true,
	"Accept-Encoding": true,
	"Content-Type":    true,
	"Content-Length":  true,
}

// stripUnsafeHeadersOnUntrustedRedirect remove, quando o redirect cruza um limite de
// confiança, todos os headers que não estão na allowlist — evitando vazamento de
// credenciais (injetadas ou passadas pelo chamador) para outro destino.
//
// Considera "não confiável":
//   - mudança de host:port (não só Hostname): example.com -> example.com:8443
//     alcança outro serviço na mesma máquina;
//   - downgrade https->http no mesmo host: exporia as credenciais em texto puro.
func stripUnsafeHeadersOnUntrustedRedirect(req *http.Request, via []*http.Request) {
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
	for name := range req.Header {
		if !redirectSafeHeaders[name] {
			req.Header.Del(name)
		}
	}
}
