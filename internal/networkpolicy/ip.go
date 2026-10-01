package networkpolicy

import "net"

// cgnatNet é o range CGNAT (Carrier-Grade NAT) RFC 6598, que net.IP.IsPrivate NÃO
// cobre mas é alcançável em redes internas/operadoras e relevante para SSRF.
//
// Construído via net.ParseCIDR para garantir um *net.IPNet consistente: net.IPv4()
// devolve um IP de 16 bytes (IPv4-mapped) que, combinado com uma máscara de 4 bytes,
// pode fazer IPNet.Contains comparar bytes errados e classificar IPs públicos como
// CGNAT. O ParseCIDR retorna IP/máscara já normalizados (4 bytes).
var cgnatNet = mustCIDR("100.64.0.0/10")

// mustCIDR parseia um CIDR literal (constante de código) e entra em panic se for
// inválido — falha de programação, detectada na inicialização.
func mustCIDR(s string) *net.IPNet {
	_, n, err := net.ParseCIDR(s)
	if err != nil {
		panic("ssrf: CIDR inválido " + s + ": " + err.Error())
	}
	return n
}

// IsBlockedIP reporta se um IP já resolvido cai em range não-roteável/local que
// deve ser barrado por SSRF: loopback, privado (RFC 1918 / fc00::/7), CGNAT
// (100.64/10), link-local (inclui 169.254.169.254), multicast, broadcast IPv4 e
// unspecified. É o ponto único de política de ranges, usado tanto pela checagem
// textual (IsPrivateHost) quanto pela validação pós-DNS no DialContext.
func IsBlockedIP(ip net.IP) bool {
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

// IsCGNAT identifica enderecos compartilhados de operadoras.
func IsCGNAT(ip net.IP) bool { return cgnatNet.Contains(ip) }
