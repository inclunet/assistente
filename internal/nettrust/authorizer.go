package nettrust

import (
	"context"
	"net"

	"assistente/internal/logging"
	"assistente/internal/tools"
	httpclient "assistente/internal/tools/http"
)

// PromptRequest é o pedido de consentimento apresentado ao usuário quando um
// destino barrado por anti-SSRF não está em nenhuma allowlist.
type PromptRequest struct {
	Host                string
	Port                string
	URL                 string
	IPs                 []string
	Category            string
	Reason              string
	SkillSlug           string   // skill que originou a chamada, se houver
	SkillSuggestedHosts []string // hosts declarados pelo skill (melhora UX; não substitui consentimento)
	// SkillHostMatch é o padrão declarado pelo skill que casa com o destino
	// bloqueado (vazio quando nenhum casa). Serve para o diálogo destacar que
	// este é o host esperado — o que não dispensa o consentimento (AEP-0082 D5),
	// só evita que a pessoa precise comparar a lista de hosts na mão.
	SkillHostMatch string
}

// PromptDecision é a resposta do usuário ao pedido de autorização.
type PromptDecision struct {
	Approve bool
	Scope   Scope
	Reason  string
}

// Prompter apresenta o pedido de consentimento e devolve a decisão. É
// implementado pela camada de app (usando o questionnaire manager / UI), o que
// mantém o pacote nettrust livre de dependências de UI.
type Prompter interface {
	PromptNetworkAuthorization(ctx context.Context, req PromptRequest) (PromptDecision, error)
}

// Authorizer conecta o Manager (allowlist) ao Prompter (consentimento) e
// implementa httpclient.NetworkAuthorizer, sendo instalado no Client HTTP.
type Authorizer struct {
	mgr    *Manager
	prompt Prompter
}

// NewAuthorizer cria o authorizer. prompt pode ser nil (só allowlist, sem UI).
func NewAuthorizer(mgr *Manager, prompt Prompter) *Authorizer {
	return &Authorizer{mgr: mgr, prompt: prompt}
}

// Authorize decide se um destino barrado por anti-SSRF pode ser liberado.
// Ordem: (1) allowlist existente → libera sem perguntar; (2) consentimento
// explícito → persiste no escopo escolhido e libera. Sempre devolve os IPs
// resolvidos do destino como trust por-request (nunca a faixa inteira).
func (a *Authorizer) Authorize(ctx context.Context, dest httpclient.BlockedDestination) ([]net.IP, bool, error) {
	host := dest.Host
	port := dest.Port

	// 1) Allowlist existente. mgr pode ser nil se o authorizer foi mal
	// inicializado; nesse caso não há allowlist para consultar (nem persistir).
	if a.mgr != nil {
		if decision := a.mgr.Match(ctx, host, port); decision.Allowed {
			// Proteção contra DNS rebinding: uma entrada é por HOST, mas os IPs
			// resolvidos podem ter mudado desde a autorização. Se o DNS passou a
			// apontar para uma categoria MAIS sensível (ex.: era CGNAT e agora
			// resolve para o endpoint de metadados/loopback), não liberamos
			// silenciosamente — caímos para novo consentimento explícito.
			if !categoryEscalated(dest, decision.Entry) {
				logging.Infof(ctx, "nettrust.authorizer",
					"[NetTrust] match em allowlist: host=%s port=%s escopo=%s categoria=%s ips=%v",
					host, port, decision.Scope, dest.Category, ipsToStrings(dest.IPs))
				return dest.IPs, true, nil
			}
			logging.Infof(ctx, "nettrust.authorizer",
				"[NetTrust] allowlist ignorada por escalonamento de categoria via DNS: host=%s escopo=%s categoria_atual=%s ips=%v — exigindo novo consentimento",
				host, decision.Scope, dest.Category, ipsToStrings(dest.IPs))
		}
	}

	// 2) Consentimento explícito
	if a.prompt == nil {
		logging.Infof(ctx, "nettrust.authorizer",
			"[NetTrust] bloqueio sem authorizer interativo: host=%s categoria=%s", host, dest.Category)
		return nil, false, nil
	}

	skillSlug, suggested := skillNetworkHints(ctx)
	req := PromptRequest{
		Host:                host,
		Port:                port,
		URL:                 dest.URL,
		IPs:                 ipsToStrings(dest.IPs),
		Category:            string(dest.Category),
		Reason:              dest.Reason,
		SkillSlug:           skillSlug,
		SkillSuggestedHosts: suggested,
		SkillHostMatch:      matchingSuggestedHost(suggested, host),
	}

	logging.Infof(ctx, "nettrust.authorizer",
		"[NetTrust] solicitando autorização: host=%s port=%s categoria=%s ips=%v skill=%s",
		host, port, dest.Category, req.IPs, skillSlug)

	decision, err := a.prompt.PromptNetworkAuthorization(ctx, req)
	if err != nil {
		return nil, false, err
	}
	if !decision.Approve {
		logging.Infof(ctx, "nettrust.authorizer", "[NetTrust] autorização negada: host=%s", host)
		return nil, false, nil
	}

	scope := decision.Scope
	if !ValidScope(scope) {
		scope = ScopeOnce
	}

	// effectiveScope reflete o que de fato aconteceu: se a entrada não puder ser
	// persistida, a liberação vale só para esta request (degrada para once) — o
	// log não deve afirmar que foi salva num escopo persistente.
	effectiveScope := scope
	if scope != ScopeOnce {
		if a.mgr == nil {
			// Sem manager não há onde persistir: mantém a auditabilidade correta.
			logging.Errorf(ctx, "nettrust.authorizer",
				"[NetTrust] sem manager para persistir allowlist (escopo %s) para host=%s — liberando apenas esta request (once)",
				scope, host)
			effectiveScope = ScopeOnce
		} else {
			// A autorização é por HOST: só gravamos a porta quando ela veio
			// EXPLÍCITA na URL. Porta derivada do scheme (443/80) não deve tornar
			// a entrada específica de porta — senão o mesmo host via http vs https
			// (ou outra porta default) voltaria a ser bloqueado/promptado.
			entryPort := ""
			if dest.PortExplicit {
				entryPort = port
			}
			entry := AllowlistEntry{
				Host:        host,
				Port:        entryPort,
				Scope:       scope,
				Category:    string(dest.Category),
				ResolvedIPs: ipsToStrings(dest.IPs),
				CreatedBy:   creatorFor(skillSlug),
				Reason:      decision.Reason,
			}
			if err := a.mgr.Add(ctx, entry); err != nil {
				// Falha ao persistir não deve abortar o acesso já autorizado nesta
				// request: logamos e seguimos como ScopeOnce.
				logging.Errorf(ctx, "nettrust.authorizer",
					"[NetTrust] falha ao persistir allowlist (escopo %s) para host=%s: %v — liberando apenas esta request (once)",
					scope, host, err)
				effectiveScope = ScopeOnce
			}
		}
	}

	logging.Infof(ctx, "nettrust.authorizer",
		"[NetTrust] autorização concedida: host=%s escopo=%s ips=%v", host, effectiveScope, req.IPs)
	return dest.IPs, true, nil
}

// categoryEscalated reporta se o destino ATUAL é mais sensível do que a entrada
// de allowlist previamente autorizada. Considera tanto a categoria do destino
// (que já captura aliases textuais como "localhost") quanto a reclassificação de
// cada IP resolvido agora — pega o pior caso. O rank de sensibilidade é a fonte
// única em internal/tools/http (httpclient.RankOf).
func categoryEscalated(dest httpclient.BlockedDestination, entry *AllowlistEntry) bool {
	stored := httpclient.UnknownCategoryRank
	if entry != nil && entry.Category != "" {
		stored = httpclient.RankOf(httpclient.Category(entry.Category))
	}

	current := httpclient.RankOf(dest.Category)
	for _, ip := range dest.IPs {
		if r := httpclient.RankOf(httpclient.Classify(ip)); r > current {
			current = r
		}
	}
	return current > stored
}

func ipsToStrings(ips []net.IP) []string {
	out := make([]string, 0, len(ips))
	for _, ip := range ips {
		out = append(out, ip.String())
	}
	return out
}

// skillNetworkHints extrai o slug do skill invocado e os hosts que ele declarou
// como permitidos (NetworkAllowedHost) — usados apenas para melhorar a UX do
// pedido (sugerir o host esperado), nunca para dispensar o consentimento.
func skillNetworkHints(ctx context.Context) (slug string, suggested []string) {
	if ec, ok := tools.GetExecutionContext(ctx); ok {
		return ec.InvokedSkillSlug, append([]string(nil), ec.NetworkAllowedHost...)
	}
	return "", nil
}

// matchingSuggestedHost devolve o primeiro padrão declarado pelo skill que casa
// com o host bloqueado, usando a MESMA regra de match da allowlist (exato ou
// wildcard "*.dominio", que não casa o apex). Vazio quando nenhum casa.
func matchingSuggestedHost(suggested []string, host string) string {
	for _, pattern := range suggested {
		if hostMatchesPattern(pattern, host) {
			return pattern
		}
	}
	return ""
}

func creatorFor(skillSlug string) string {
	if skillSlug != "" {
		return "skill:" + skillSlug
	}
	return "user"
}
