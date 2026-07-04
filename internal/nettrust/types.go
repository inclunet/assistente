// Package nettrust implementa a allowlist escopável e o fluxo de consentimento
// para destinos barrados pela política anti-SSRF das tools de rede
// (http_request, web_fetch, feed_read).
//
// Ele NÃO afrouxa a barreira anti-SSRF: apenas registra decisões explícitas do
// usuário (por host/padrão, com escopo e IPs resolvidos auditáveis) e as
// disponibiliza para o Client HTTP reexecutar a request liberando exatamente os
// IPs autorizados. A classificação de faixas de IP continua sendo a fonte única
// de verdade em internal/tools/http (isBlockedIP/Classify).
package nettrust

import (
	"strings"
	"time"
)

// Scope define o alcance de uma autorização de rede, do mais efêmero ao mais
// amplo. A ordem de match no Manager é sessão → perfil → workspace → global.
type Scope string

const (
	// ScopeOnce libera apenas a requisição atual; nunca é persistida.
	ScopeOnce Scope = "once"
	// ScopeSession vale enquanto durar a conversa/sessão (em memória).
	ScopeSession Scope = "session"
	// ScopeWorkspace persiste no .assistente/ do diretório de trabalho (projeto).
	ScopeWorkspace Scope = "workspace"
	// ScopeProfile persiste por perfil ativo (arquivo por slug em ~/.assistente/).
	ScopeProfile Scope = "profile"
	// ScopeGlobal persiste globalmente em ~/.assistente/.
	ScopeGlobal Scope = "global"
)

// ValidScope reporta se s é um escopo conhecido.
func ValidScope(s Scope) bool {
	switch s {
	case ScopeOnce, ScopeSession, ScopeWorkspace, ScopeProfile, ScopeGlobal:
		return true
	default:
		return false
	}
}

// IsPersistent reporta se o escopo é gravado em disco (workspace/profile/global).
func (s Scope) IsPersistent() bool {
	return s == ScopeWorkspace || s == ScopeProfile || s == ScopeGlobal
}

// AllowlistEntry é uma autorização de rede para um host/padrão barrado por
// anti-SSRF. Guarda metadados para auditoria: quem criou, quando, por quê, os
// IP(s) resolvidos no momento da autorização e a categoria do bloqueio.
type AllowlistEntry struct {
	// Host é o host exato (api.nu.workflows.dev) OU um padrão de domínio
	// (*.nu.workflows.dev). Comparação case-insensitive, sem ponto final.
	Host string `json:"host"`
	// Port, quando não-vazio, restringe a autorização àquela porta.
	Port string `json:"port,omitempty"`
	// Scope é o alcance desta entrada (ver Scope). Entradas persistidas nunca têm
	// ScopeOnce.
	Scope Scope `json:"scope"`
	// Category é a categoria do bloqueio no momento da autorização (cgnat,
	// loopback, metadata, ...). Informativa/auditoria.
	Category string `json:"category,omitempty"`
	// ResolvedIPs registra os IP(s) que o host resolvia quando foi autorizado —
	// para auditoria e para deixar explícito que a decisão foi sobre IPs reais.
	ResolvedIPs []string `json:"resolved_ips,omitempty"`
	// CreatedBy identifica a origem da autorização (ex.: "user", ou o skill slug).
	CreatedBy string `json:"created_by,omitempty"`
	// CreatedAt é o timestamp de criação (UTC).
	CreatedAt time.Time `json:"created_at"`
	// Reason é uma observação/motivo livre.
	Reason string `json:"reason,omitempty"`
}

// Matches reporta se esta entrada autoriza um dado host(:port).
//
// Semântica de porta (AEP-0082):
//   - Port preenchida: casa exatamente aquela porta (autorização explícita da
//     porta que veio na URL).
//   - Port vazia: autorização "por host", que cobre APENAS portas default
//     (80/443 ou porta ausente). Isso evita que uma autorização implícita para
//     https://host (443) libere serviços diferentes no MESMO host em portas
//     não-default (ex.: 8443) — o que seria um afrouxamento não-intencional.
//     Portas não-default sempre exigem autorização explícita daquela porta.
func (e AllowlistEntry) Matches(host, port string) bool {
	if e.Port != "" {
		if !strings.EqualFold(e.Port, port) {
			return false
		}
	} else if !isDefaultPort(port) {
		return false
	}
	return hostMatchesPattern(e.Host, host)
}

// isDefaultPort reporta se port é uma porta default de HTTP(S) — ou ausente.
// Uma autorização por host (sem porta) só vale para essas portas.
func isDefaultPort(port string) bool {
	switch port {
	case "", "80", "443":
		return true
	default:
		return false
	}
}

// NetworkTrustDecision é o resultado de uma consulta de autorização: se o destino
// pode ser acessado, por qual escopo/entrada, e se veio de consentimento novo.
type NetworkTrustDecision struct {
	Allowed  bool
	Scope    Scope
	Entry    *AllowlistEntry
	Prompted bool // true quando exigiu consentimento novo do usuário
}

// normalizeHost normaliza um host para comparação: minúsculas, sem espaços, sem
// colchetes de IPv6, sem zone id e sem ponto final de FQDN. Mesma semântica de
// internal/tools/http para manter consistência entre as duas camadas.
func normalizeHost(host string) string {
	host = strings.ToLower(strings.TrimSpace(host))
	host = strings.Trim(host, "[]")
	if i := strings.IndexByte(host, '%'); i >= 0 {
		host = host[:i]
	}
	return strings.TrimRight(host, ".")
}

// hostMatchesPattern implementa match exato + wildcard "*.dominio" (que NÃO casa
// o apex). Espelha internal/tools/http.hostMatchesPattern.
func hostMatchesPattern(pattern, host string) bool {
	pattern = normalizeHost(pattern)
	host = normalizeHost(host)
	if pattern == "" || host == "" {
		return false
	}
	if pattern == host {
		return true
	}
	if strings.HasPrefix(pattern, "*.") {
		suffix := strings.TrimPrefix(pattern, "*.")
		return host != suffix && strings.HasSuffix(host, "."+suffix)
	}
	return false
}
