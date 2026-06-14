package http

import (
	"net"
	"testing"
)

// TestIsBlockedIP_CGNATBoundaries valida diretamente isBlockedIP nos limites do
// range CGNAT (100.64.0.0/10). Pega regressões na construção do cgnatNet: com um
// *net.IPNet mal formado, IPNet.Contains classificaria IPs públicos como CGNAT e o
// caso 8.8.8.8/1.1.1.1 falharia.
func TestIsBlockedIP_CGNATBoundaries(t *testing.T) {
	blocked := []string{"100.64.0.1", "100.127.255.255", "100.64.0.0", "100.100.50.25"}
	for _, s := range blocked {
		if !isBlockedIP(net.ParseIP(s)) {
			t.Errorf("%s deveria ser bloqueado (CGNAT 100.64.0.0/10)", s)
		}
	}

	// IPs públicos e os limites JUST-OUTSIDE do range CGNAT NÃO podem ser bloqueados.
	notBlocked := []string{"8.8.8.8", "1.1.1.1", "100.63.255.255", "100.128.0.0"}
	for _, s := range notBlocked {
		if isBlockedIP(net.ParseIP(s)) {
			t.Errorf("%s NÃO deveria ser bloqueado", s)
		}
	}
}

func TestIsPrivateHost(t *testing.T) {
	blocked := []string{
		"localhost", "127.0.0.1", "0.0.0.0", "::1", "[::1]",
		"10.0.0.1", "192.168.1.1", "172.16.0.1", "172.31.255.255", "169.254.0.1",
		// Variantes com ponto(s) final(is) (FQDN) e espaços: bypass clássico de filtros.
		"localhost.", "127.0.0.1.", "  localhost  ", "10.0.0.1.", "LOCALHOST.",
		"localhost..", "127.0.0.1...", "10.0.0.1.%eth0",
		// IPv6 link-local/loopback com zone identifier (RFC 6874): url.Hostname()
		// devolve o sufixo "%zone".
		"fe80::1%lo0", "fe80::1", "::1%eth0",
		// TLD reservado .localhost (RFC 6761): resolve para loopback em muitos
		// sistemas, então "*.localhost" seria um bypass do match exato.
		"foo.localhost", "evil.LOCALHOST", "a.b.localhost.",
		// Cloud metadata endpoint e variantes IPv4-mapped/multicast de escopo local.
		"169.254.169.254", "::ffff:169.254.169.254", "::ffff:127.0.0.1", "224.0.0.1",
		// IPv4-mapped IPv6 de broadcast e privado: a normalização via To4() deve
		// fazê-los baterem nos checks IPv4 (broadcast/RFC 1918).
		"::ffff:255.255.255.255", "::ffff:10.0.0.1", "::ffff:192.168.1.1",
		// Multicast fora do escopo link-local (SSDP), broadcast limitado e multicast IPv6.
		"239.255.255.250", "255.255.255.255", "ff02::c", "ff0e::1",
		// CGNAT (100.64.0.0/10, RFC 6598): alcançável em redes internas/operadoras.
		"100.64.0.1", "100.127.255.255", "100.100.100.100",
		// Host vazio não é destino válido.
		"", "   ",
	}
	for _, h := range blocked {
		if !IsPrivateHost(h) {
			t.Errorf("%q deveria ser bloqueado", h)
		}
	}

	// 172.2.x.x e 172.32.x.x são públicos (fora do range 172.16/12); o prefix
	// match ingênuo bloqueava 172.2.* indevidamente.
	// Inclui hosts que apenas CONTÊM "localhost" mas não são o TLD reservado, para
	// garantir que o HasSuffix exige o ponto separador.
	public := []string{
		"172.2.3.4", "172.32.0.1", "8.8.8.8", "example.com", "1.1.1.1", "google.com",
		"mylocalhost", "localhost.example.com", "notlocalhost",
		// Limites do CGNAT: 100.63.x e 100.128.x estão FORA do 100.64/10.
		"100.63.255.255", "100.128.0.1",
	}
	for _, h := range public {
		if IsPrivateHost(h) {
			t.Errorf("%q NÃO deveria ser bloqueado", h)
		}
	}
}
