package http

import (
	"net"
	"strings"
)

// IsPrivateHost reporta se um host aponta para um endereço local/privado, servindo
// como barreira básica anti-SSRF compartilhada pelas tools de rede (web_fetch,
// http_request, feed_read) para evitar divergência de política e duplicação.
//
// Usa net.ParseIP para cobrir corretamente os ranges privados (10/8, 172.16/12,
// 192.168/16), loopback, link-local e unspecified, sem o falso positivo de um
// prefix match ingênuo (ex.: 172.2.x.x é público).
//
// Limitação conhecida: só inspeciona IPs literais e "localhost". Um hostname que
// resolve via DNS para um IP privado (ex.: "internal.example") NÃO é bloqueado,
// pois não fazemos resolução de DNS aqui (evita lookups extras e o TOCTOU entre o
// check e o dial). Portanto não é proteção anti-SSRF completa, apenas uma barreira
// contra URLs que já apontam diretamente para endereços locais/privados.
func IsPrivateHost(host string) bool {
	// Normaliza: remove espaços, colchetes de IPv6 e o ponto final do FQDN
	// ("localhost." / "127.0.0.1." são aceitos por DNS e pelo net/http e seriam
	// um bypass clássico de filtros por string).
	host = strings.ToLower(strings.TrimSpace(host))
	host = strings.Trim(host, "[]")
	host = strings.TrimSuffix(host, ".")
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsUnspecified()
}
