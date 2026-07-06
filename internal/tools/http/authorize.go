package http

import (
	"context"
	"fmt"
	"net"
	"strings"
)

// BlockedDestination descreve um destino barrado pela política anti-SSRF de forma
// estruturada, para (a) montar o pedido de autorização ao usuário e (b) montar a
// mensagem de erro acionável. É produzida pelo Client quando o guard pós-DNS
// devolve BlockedIPError e há um NetworkAuthorizer configurado.
type BlockedDestination struct {
	Host string // hostname da URL (ex.: api.nu.workflows.dev)
	Port string // porta efetiva (ex.: 443), vazio se não determinada
	// PortExplicit indica se a porta veio EXPLÍCITA na URL (host:porta) e não
	// apenas derivada do scheme (443/80). A allowlist persistida só grava a porta
	// quando ela é explícita — caso contrário a autorização é por host (qualquer
	// porta default), como descreve o AEP-0082.
	PortExplicit bool
	URL          string   // URL solicitada sanitizada: scheme://host/path (sem query/fragment/userinfo)
	IPs          []net.IP // IP(s) resolvido(s) do host
	Category     Category // categoria do bloqueio (cgnat, loopback, metadata, ...)
	Reason       string   // motivo textual legível
}

// HostPort devolve "host:port" quando há porta, senão só o host. Útil como chave
// de allowlist quando o escopo inclui porta.
func (d BlockedDestination) HostPort() string {
	if d.Port == "" {
		return d.Host
	}
	return net.JoinHostPort(d.Host, d.Port)
}

// NetworkAuthorizer decide, em runtime, se um destino barrado por anti-SSRF pode
// ser liberado — consultando allowlist persistida e/ou pedindo consentimento
// explícito ao usuário. É injetado no Client pela camada de app (que conhece a
// UI de consentimento e o armazenamento de allowlist), mantendo o pacote http
// livre de dependências de UI/persistência.
//
// Retorno:
//   - trustedIPs: IPs que devem ser liberados para a reexecução desta request
//     (tipicamente os IPs resolvidos do host autorizado). Vazio => manter bloqueio.
//   - ok: true se autorizado.
//   - err: falha operacional (ex.: timeout aguardando o usuário).
type NetworkAuthorizer interface {
	Authorize(ctx context.Context, dest BlockedDestination) (trustedIPs []net.IP, ok bool, err error)
}

// BlockedDestinationError é o erro acionável devolvido quando um destino é barrado
// por anti-SSRF e NÃO houve autorização (sem authorizer configurado ou usuário
// negou). Diferente do BlockedIPError seco, expõe host, IP, categoria e as ações
// possíveis — sem esconder que houve bloqueio por política.
type BlockedDestinationError struct {
	Host        string
	URL         string
	IPs         []net.IP
	Category    Category
	Reason      string
	Suggestions []string
}

func (e *BlockedDestinationError) Error() string {
	var b strings.Builder
	b.WriteString("acesso bloqueado pela política anti-SSRF")
	if e.Host != "" {
		fmt.Fprintf(&b, "\n- Host: %s", e.Host)
	}
	if len(e.IPs) > 0 {
		fmt.Fprintf(&b, "\n- IP resolvido: %s", joinIPs(e.IPs))
	}
	rule := string(e.Category)
	if e.Reason != "" {
		rule = fmt.Sprintf("%s (%s)", e.Category, e.Reason)
	}
	fmt.Fprintf(&b, "\n- Regra: %s", rule)
	if len(e.Suggestions) > 0 {
		label := "Ação possível"
		if len(e.Suggestions) > 1 {
			label = "Ações possíveis"
		}
		fmt.Fprintf(&b, "\n- %s: %s", label, strings.Join(e.Suggestions, " | "))
	}
	return b.String()
}

func joinIPs(ips []net.IP) string {
	parts := make([]string, 0, len(ips))
	for _, ip := range ips {
		parts = append(parts, ip.String())
	}
	return strings.Join(parts, ", ")
}

// defaultBlockSuggestions são as ações sugeridas quando não há autorização.
var defaultBlockSuggestions = []string{
	"autorizar temporariamente esta requisição",
	"adicionar o host à allowlist (sessão / workspace / perfil / global)",
}
