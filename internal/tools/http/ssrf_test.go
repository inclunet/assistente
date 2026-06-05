package http

import "testing"

func TestIsPrivateHost(t *testing.T) {
	blocked := []string{
		"localhost", "127.0.0.1", "0.0.0.0", "::1", "[::1]",
		"10.0.0.1", "192.168.1.1", "172.16.0.1", "172.31.255.255", "169.254.0.1",
		// Variantes com ponto final (FQDN) e espaços: bypass clássico de filtros.
		"localhost.", "127.0.0.1.", "  localhost  ", "10.0.0.1.", "LOCALHOST.",
		// IPv6 link-local/loopback com zone identifier (RFC 6874): url.Hostname()
		// devolve o sufixo "%zone".
		"fe80::1%lo0", "fe80::1", "::1%eth0",
	}
	for _, h := range blocked {
		if !IsPrivateHost(h) {
			t.Errorf("%q deveria ser bloqueado", h)
		}
	}

	// 172.2.x.x e 172.32.x.x são públicos (fora do range 172.16/12); o prefix
	// match ingênuo bloqueava 172.2.* indevidamente.
	public := []string{"172.2.3.4", "172.32.0.1", "8.8.8.8", "example.com", "1.1.1.1", "google.com"}
	for _, h := range public {
		if IsPrivateHost(h) {
			t.Errorf("%q NÃO deveria ser bloqueado", h)
		}
	}
}
