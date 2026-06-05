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
// 192.168/16, fc00::/7), loopback, link-local (inclui 169.254.169.254, o metadata
// endpoint de nuvem), multicast de escopo local e unspecified, sem o falso positivo
// de um prefix match ingênuo (ex.: 172.2.x.x é público). Também bloqueia "localhost"
// e o TLD reservado ".localhost".
//
// Limitações conhecidas (barreira básica, não proteção anti-SSRF completa):
//   - Só inspeciona IPs literais em notação padrão (o que net.ParseIP aceita) e
//     "localhost". Formas numéricas não-padrão (decimal "2130706433", octal
//     "0177.0.0.1", hex "0x7f.0.0.1") NÃO são reconhecidas como IP aqui.
//   - Um hostname que resolve via DNS para um IP privado (ex.: "internal.example")
//     NÃO é bloqueado: não fazemos resolução de DNS aqui, para evitar lookups
//     extras e o TOCTOU entre o check e o dial.
//
// É uma barreira contra URLs que já apontam diretamente para endereços
// locais/privados em notação usual.
func IsPrivateHost(host string) bool {
	// Normaliza para fechar bypasses clássicos de filtros por string. A ordem
	// importa para ser robusta a combinações.
	host = strings.ToLower(strings.TrimSpace(host))
	host = strings.Trim(host, "[]")
	// 1) Remove o zone identifier de IPv6 link-local (RFC 6874), ex.: "fe80::1%lo0".
	//    url.URL.Hostname() devolve o sufixo "%zone" e net.ParseIP falharia, deixando
	//    passar endereços link-local/loopback.
	if i := strings.IndexByte(host, '%'); i >= 0 {
		host = host[:i]
	}
	// 2) Remove TODOS os pontos finais do FQDN ("localhost.", "localhost.." e
	//    "127.0.0.1." são aceitos por DNS e pelo net/http). Feito após o passo 1
	//    para cobrir também casos como "10.0.0.1.%eth0".
	host = strings.TrimRight(host, ".")

	// Host vazio não é destino válido; bloqueia por segurança (ex.: "http://").
	if host == "" {
		return true
	}
	// "localhost" e o TLD reservado ".localhost" (RFC 6761): em muitos sistemas
	// (ex.: systemd-resolved) qualquer "*.localhost" resolve para loopback, então
	// "evil.localhost" seria um bypass do match exato de "localhost".
	if host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return true
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() || ip.IsInterfaceLocalMulticast() || ip.IsUnspecified()
}
