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
			logging.Infof(ctx, "nettrust.authorizer",
				"[NetTrust] match em allowlist: host=%s port=%s escopo=%s categoria=%s ips=%v",
				host, port, decision.Scope, dest.Category, ipsToStrings(dest.IPs))
			return dest.IPs, true, nil
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

	if scope != ScopeOnce && a.mgr != nil {
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
			logging.Errorf(ctx, "nettrust.authorizer", "[NetTrust] falha ao persistir allowlist (%s): %v", scope, err)
		}
	}

	logging.Infof(ctx, "nettrust.authorizer",
		"[NetTrust] autorização concedida: host=%s escopo=%s ips=%v", host, scope, req.IPs)
	return dest.IPs, true, nil
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
