package nettrust

import (
	"assistente/internal/authorization"
	"context"
	"net"

	"assistente/internal/logging"
	"assistente/internal/tools"
	httpclient "assistente/internal/tools/http"
)

// PromptRequest é o pedido de consentimento apresentado ao usuário quando um
// destino barrado por anti-SSRF não está em nenhuma allowlist.
type PromptRequest struct {
	AvailableScopes     []Scope
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
	ScopeFilter       func(context.Context, []Scope) []Scope
	PersistenceFailed func(context.Context, authorization.Request, error)
	mgr               *Manager
	prompt            Prompter
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
	if a.mgr != nil {
		ctx = a.mgr.Capture(ctx)
	}
	skillSlug, suggested := skillNetworkHints(ctx)
	scopes := []Scope{ScopeOnce}
	if a.mgr != nil {
		var err error
		scopes, err = a.mgr.AvailableScopes(ctx)
		if err != nil {
			return nil, false, err
		}
	}
	if a.ScopeFilter != nil {
		scopes = a.ScopeFilter(ctx, scopes)
	}
	flow := authorization.Flow[AllowlistEntry]{
		Evaluate: func(ctx context.Context) (authorization.Decision, error) {
			if a.mgr != nil {
				match := a.mgr.Match(ctx, dest.Host, dest.Port)
				if match.Err != nil {
					return authorization.Decision{}, match.Err
				}
				if match.Allowed && !categoryEscalated(dest, match.Entry) {
					return authorization.Decision{Effect: "allow", Scope: match.Scope, Reason: "concessão compatível com o destino resolvido"}, nil
				}
			}
			return authorization.Decision{Effect: "ask"}, nil
		},
		PersistenceFailed: func(ctx context.Context, req authorization.Request, err error) {
			if a.PersistenceFailed != nil {
				a.PersistenceFailed(ctx, req, err)
			}
			logging.Errorf(ctx, "nettrust.authorizer", "[NetTrust] falha ao lembrar autorização host=%s: %v — válida somente nesta tentativa", dest.Host, err)
		},
	}
	if a.mgr != nil {
		flow.Remember = func(ctx context.Context, choice authorization.Consent[AllowlistEntry]) error {
			return a.mgr.Add(ctx, choice.Value)
		}
	}
	if a.prompt != nil {
		flow.Ask = func(ctx context.Context) (authorization.Consent[AllowlistEntry], error) {
			req := PromptRequest{AvailableScopes: scopes, Host: dest.Host, Port: dest.Port, URL: dest.URL, IPs: ipsToStrings(dest.IPs), Category: string(dest.Category), Reason: dest.Reason, SkillSlug: skillSlug, SkillSuggestedHosts: suggested, SkillHostMatch: matchingSuggestedHost(suggested, dest.Host)}
			decision, err := a.prompt.PromptNetworkAuthorization(ctx, req)
			if err != nil {
				return authorization.Consent[AllowlistEntry]{}, err
			}
			if !decision.Approve {
				return authorization.Consent[AllowlistEntry]{Effect: "deny", Scope: "once"}, nil
			}
			scope := decision.Scope
			if !ValidScope(scope) {
				scope = ScopeOnce
			}
			port := ""
			if dest.PortExplicit {
				port = dest.Port
			}
			entry := AllowlistEntry{Host: dest.Host, Port: port, Scope: scope, Category: string(dest.Category), ResolvedIPs: ipsToStrings(dest.IPs), CreatedBy: creatorFor(skillSlug), Reason: decision.Reason}
			return authorization.Consent[AllowlistEntry]{Effect: "allow", Scope: scope, Value: entry}, nil
		}
	}
	result, err := authorization.Resolve(ctx, authorization.Request{Domain: "network", Operation: "connect", Resource: dest.Host, AvailableScopes: scopes}, flow)
	if err != nil {
		return nil, false, err
	}
	if result.Effect != "allow" {
		return nil, false, nil
	}
	logging.Infof(ctx, "nettrust.authorizer", "[NetTrust] autorização concedida: host=%s escopo=%s ips=%v", dest.Host, result.Scope, ipsToStrings(dest.IPs))
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
