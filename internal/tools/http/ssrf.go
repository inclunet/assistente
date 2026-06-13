package http

import (
	"net"
	"strings"
)

// IsPrivateHost reporta se um host aponta para um endereço local/privado, servindo
// como primeira barreira anti-SSRF compartilhada pelas tools de rede (web_fetch,
// http_request, feed_read) para evitar divergência de política e duplicação.
//
// Usa net.ParseIP para cobrir corretamente os ranges privados (10/8, 172.16/12,
// 192.168/16, fc00::/7), CGNAT (100.64/10), loopback, link-local (inclui
// 169.254.169.254, o metadata endpoint de nuvem), multicast (inclui
// 239.255.255.250/SSDP), broadcast IPv4 e unspecified, sem o falso positivo de um
// prefix match ingênuo (ex.: 172.2.x.x é público). Também bloqueia "localhost" e o
// TLD reservado ".localhost".
//
// Cobertura em duas camadas (ver dialer.go):
//   - Esta função é a checagem ANTES do dial, sobre o host textual da URL. Ela só
//     reconhece IPs literais na notação que net.ParseIP aceita; formas numéricas
//     não-padrão (decimal "2130706433", octal "0177.0.0.1", hex "0x7f.0.0.1") e
//     hostnames que resolvem para IP privado (DNS rebinding) passam por aqui.
//   - A barreira definitiva é o GuardedTransport: o DialContext valida o IP REAL
//     APÓS a resolução de DNS, fechando o TOCTOU e cobrindo tanto as formas
//     numéricas alternativas (o SO as normaliza ao resolver) quanto os redirects
//     (que reusam o mesmo transport).
//
// Mantida como rejeição rápida e legível de URLs que já apontam diretamente para
// endereços locais/privados em notação usual.
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
	return isBlockedIP(ip)
}

// cgnatNet é o range CGNAT (Carrier-Grade NAT) RFC 6598, que net.IP.IsPrivate NÃO
// cobre mas é alcançável em redes internas/operadoras e relevante para SSRF.
var cgnatNet = &net.IPNet{IP: net.IPv4(100, 64, 0, 0), Mask: net.CIDRMask(10, 32)}

// isBlockedIP reporta se um IP já resolvido cai em range não-roteável/local que
// deve ser barrado por SSRF: loopback, privado (RFC 1918 / fc00::/7), CGNAT
// (100.64/10), link-local (inclui 169.254.169.254), multicast, broadcast IPv4 e
// unspecified. É o ponto único de política de ranges, usado tanto pela checagem
// textual (IsPrivateHost) quanto pela validação pós-DNS no DialContext.
func isBlockedIP(ip net.IP) bool {
	if ip == nil {
		return true
	}
	// Normaliza IPv4-mapped IPv6 (ex.: ::ffff:255.255.255.255, ::ffff:10.0.0.1) para
	// a forma IPv4 de 4 bytes. Sem isto, comparações IPv4 explícitas (como o broadcast
	// abaixo) poderiam não bater para a representação mapeada, abrindo um bypass.
	if v4 := ip.To4(); v4 != nil {
		ip = v4
	}
	// Broadcast IPv4 limitado (255.255.255.255): alcança toda a rede local.
	if ip.Equal(net.IPv4bcast) {
		return true
	}
	if cgnatNet.Contains(ip) {
		return true
	}
	// IsMulticast cobre todo o range multicast (224.0.0.0/4 e ff00::/8), incluindo
	// destinos de descoberta de rede local como 239.255.255.250 (SSDP), não só o
	// escopo link-local.
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() ||
		ip.IsMulticast() || ip.IsUnspecified()
}
