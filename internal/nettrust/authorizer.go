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
		switch {
		case a.mgr == nil:
			// Sem manager não há onde persistir: mantém a auditabilidade correta.
			logging.Errorf(ctx, "nettrust.authorizer",
				"[NetTrust] sem manager para persistir allowlist (escopo %s) para host=%s — liberando apenas esta request (once)",
				scope, host)
			effectiveScope = ScopeOnce
		default:
			entry := AllowlistEntry{
				Host:        host,
				Port:        port,
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

// categoryRank ordena as categorias anti-SSRF por sensibilidade (maior = mais
// perigoso). Usado para detectar escalonamento via DNS numa entrada de allowlist
// por host: só liberamos silenciosamente enquanto a categoria atual não for MAIS
// sensível do que a que o usuário efetivamente autorizou.
var categoryRank = map[httpclient.Category]int{
	httpclient.CategoryPublic:         0,
	httpclient.CategoryReserved:       1,
	httpclient.CategoryMulticast:      1,
	httpclient.CategoryLinkLocal:      2,
	httpclient.CategoryCGNAT:          2,
	httpclient.CategoryPrivateRFC1918: 2,
	httpclient.CategoryLoopback:       3,
	httpclient.CategoryLocalhostAlias: 3,
	httpclient.CategoryMetadata:       4,
}

// unknownCategoryRank é o nível assumido quando a categoria é vazia/desconhecida
// (ex.: entradas antigas ou adicionadas programaticamente sem categoria). Trata
// como "privado genérico": permite rotação entre IPs privados/CGNAT, mas ainda
// bloqueia escalonamento para loopback/metadados.
const unknownCategoryRank = 2

func rankOf(cat httpclient.Category) int {
	if r, ok := categoryRank[cat]; ok {
		return r
	}
	return unknownCategoryRank
}

// categoryEscalated reporta se o destino ATUAL é mais sensível do que a entrada
// de allowlist previamente autorizada. Considera tanto a categoria do destino
// (que já captura aliases textuais como "localhost") quanto a reclassificação de
// cada IP resolvido agora — pega o pior caso.
func categoryEscalated(dest httpclient.BlockedDestination, entry *AllowlistEntry) bool {
	stored := unknownCategoryRank
	if entry != nil && entry.Category != "" {
		stored = rankOf(httpclient.Category(entry.Category))
	}

	current := rankOf(dest.Category)
	for _, ip := range dest.IPs {
		if r := rankOf(httpclient.Classify(ip)); r > current {
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

func creatorFor(skillSlug string) string {
	if skillSlug != "" {
		return "skill:" + skillSlug
	}
	return "user"
}
