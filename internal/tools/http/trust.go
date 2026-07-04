package http

import (
	"context"
	"net"
	"strings"
)

// Category classifica por que um IP/host é barrado pela política anti-SSRF. É
// derivada da MESMA fonte de verdade de ranges (isBlockedIP em ssrf.go), então
// nunca diverge do que realmente é bloqueado. Serve para mensagens de erro
// acionáveis, para o pedido de autorização ao usuário e para observabilidade.
type Category string

const (
	// CategoryPublic é um destino roteável normal (não bloqueado).
	CategoryPublic Category = "public"
	// CategoryLoopback é 127.0.0.0/8 ou ::1.
	CategoryLoopback Category = "loopback"
	// CategoryLocalhostAlias é o hostname "localhost" ou o TLD reservado ".localhost".
	CategoryLocalhostAlias Category = "localhost-alias"
	// CategoryPrivateRFC1918 é RFC 1918 (10/8, 172.16/12, 192.168/16) ou ULA IPv6 (fc00::/7).
	CategoryPrivateRFC1918 Category = "private-rfc1918"
	// CategoryCGNAT é o range Carrier-Grade NAT (RFC 6598, 100.64/10).
	CategoryCGNAT Category = "cgnat"
	// CategoryLinkLocal é 169.254/16 ou fe80::/10 (sem o metadata endpoint, tratado à parte).
	CategoryLinkLocal Category = "link-local"
	// CategoryMetadata é o endpoint de metadados de nuvem (169.254.169.254),
	// destacado do link-local por ser o alvo clássico de SSRF em cloud.
	CategoryMetadata Category = "metadata"
	// CategoryMulticast cobre 224.0.0.0/4 e ff00::/8 (inclui SSDP 239.255.255.250).
	CategoryMulticast Category = "multicast"
	// CategoryReserved cobre broadcast limitado (255.255.255.255) e unspecified (0.0.0.0/::).
	CategoryReserved Category = "reserved"
)

// metadataIP é o endpoint de metadados de instância usado por AWS/GCP/Azure/etc.
// (link-local, mas destacado por ser o alvo canônico de SSRF em nuvem).
var metadataIP = net.ParseIP("169.254.169.254")

// Classify devolve a Category de um IP JÁ RESOLVIDO. A ordem das checagens segue
// a especificidade (metadata antes de link-local, cgnat antes de private) para
// que a categoria reportada seja a mais informativa. Espelha isBlockedIP: todo IP
// que isBlockedIP barra recebe uma categoria != CategoryPublic e vice-versa.
func Classify(ip net.IP) Category {
	if ip == nil {
		return CategoryReserved
	}
	if v4 := ip.To4(); v4 != nil {
		ip = v4
	}
	switch {
	case ip.Equal(metadataIP):
		return CategoryMetadata
	case ip.IsLoopback():
		return CategoryLoopback
	case cgnatNet.Contains(ip):
		return CategoryCGNAT
	case ip.IsPrivate():
		return CategoryPrivateRFC1918
	case ip.IsLinkLocalUnicast():
		return CategoryLinkLocal
	case ip.IsMulticast():
		return CategoryMulticast
	case ip.Equal(net.IPv4bcast) || ip.IsUnspecified():
		return CategoryReserved
	default:
		return CategoryPublic
	}
}

// ClassifyDestination classifica o bloqueio de um destino considerando TANTO o
// host textual quanto o IP resolvido. Hosts "localhost"/".localhost" (RFC 6761)
// recebem CategoryLocalhostAlias — mais informativo que o loopback do IP, já que
// a barra é sobre o nome (que em muitos sistemas resolve para loopback). Para os
// demais casos, classifica pelo IP real (Classify).
func ClassifyDestination(host string, ip net.IP) Category {
	if isLocalhostAlias(host) {
		return CategoryLocalhostAlias
	}
	return Classify(ip)
}

// isLocalhostAlias reporta se o host textual é "localhost" ou o TLD reservado
// ".localhost" (após normalização). Mesma regra de IsPrivateHost.
func isLocalhostAlias(host string) bool {
	host = normalizeNetworkHost(host)
	return host == "localhost" || strings.HasSuffix(host, ".localhost")
}

// normalizeIPKey devolve uma chave canônica de comparação para um IP, colapsando
// IPv4-mapped IPv6 (::ffff:10.0.0.1) para a forma IPv4 — sem isto o mesmo IP
// poderia gerar duas chaves e furar a checagem de trust.
func normalizeIPKey(ip net.IP) string {
	if ip == nil {
		return ""
	}
	if v4 := ip.To4(); v4 != nil {
		return v4.String()
	}
	return ip.String()
}

type trustedIPsKey struct{}

// trustKey monta a chave canônica de um destino confiável: IP normalizado +
// porta. O trust é por IP:porta (não só por IP) para honrar a semântica de porta
// do AEP-0082 — uma autorização por host (portas default) não deve liberar portas
// não-default no mesmo IP, inclusive via redirect.
func trustKey(ipKey, port string) string {
	return ipKey + "|" + port
}

// allowedTrustPorts devolve as portas que uma autorização cobre. Porta explícita
// libera exatamente aquela porta; porta derivada do scheme (host-level) libera
// apenas as portas default (80/443), espelhando AllowlistEntry.Matches.
func allowedTrustPorts(port string, explicit bool) []string {
	if explicit && port != "" {
		return []string{port}
	}
	return []string{"80", "443"}
}

// WithTrustedIPs injeta no ctx o conjunto de destinos (IP:porta) que podem ser
// conectados NESTA request apesar de caírem em faixa bloqueada — o resultado de
// uma autorização explícita do usuário (ver NetworkAuthorizer). O trust é por IP
// RESOLVIDO + PORTA (não por hostname): amarra a permissão ao endereço concreto
// autorizado, mantendo fechados DNS rebinding, o acesso a outros hosts/IPs da
// mesma faixa e o acesso a portas não autorizadas no mesmo IP (ex.: via redirect).
//
// port é a porta efetiva do destino e portExplicit indica se ela veio explícita
// na URL (ver BlockedDestination): host-level cobre portas default (80/443),
// porta explícita cobre apenas ela.
func WithTrustedIPs(ctx context.Context, ips []net.IP, port string, portExplicit bool) context.Context {
	if len(ips) == 0 {
		return ctx
	}
	ports := allowedTrustPorts(port, portExplicit)
	set := make(map[string]bool, len(ips)*len(ports))
	for _, ip := range ips {
		key := normalizeIPKey(ip)
		if key == "" {
			continue
		}
		for _, p := range ports {
			set[trustKey(key, p)] = true
		}
	}
	if len(set) == 0 {
		return ctx
	}
	// Mescla com um trust preexistente no ctx (ex.: redirect dentro da mesma
	// request que já herdou destinos confiáveis).
	if existing, ok := ctx.Value(trustedIPsKey{}).(map[string]bool); ok {
		for k := range existing {
			set[k] = true
		}
	}
	return context.WithValue(ctx, trustedIPsKey{}, set)
}

// trustedIPSet devolve o conjunto de destinos confiáveis (IP:porta) do ctx (nil
// se nenhum). As chaves seguem trustKey (ver WithTrustedIPs).
func trustedIPSet(ctx context.Context) map[string]bool {
	if ctx == nil {
		return nil
	}
	set, _ := ctx.Value(trustedIPsKey{}).(map[string]bool)
	return set
}

// hasTrustedIPs reporta se a request carrega QUALQUER destino confiável por-request,
// ou seja, se algum destino foi explicitamente autorizado. Usado para delegar a
// decisão final à barreira pós-DNS (DialContext), que revalida o IP:porta real.
func hasTrustedIPs(ctx context.Context) bool {
	return len(trustedIPSet(ctx)) > 0
}
