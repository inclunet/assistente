package app

import (
	"context"
	"fmt"
	"strings"

	"assistente/internal/nettrust"
	"assistente/internal/questionnaire"
	"assistente/internal/tools/invocationctx"
)

// ============================================================================
// Network Trust (allowlist anti-SSRF escopável) — Prompter + API
// ============================================================================

// scopeOption associa o rótulo exibido ao usuário ao Scope correspondente. A
// ordem define a apresentação (do mais efêmero ao mais amplo).
type scopeOption struct {
	// key é a chave de tradução do rótulo; label é o texto pronto em pt-BR,
	// que também compõe o valor estável da opção.
	key   string
	label string
	scope nettrust.Scope
}

var networkScopeOptions = []scopeOption{
	{"app.questionnaire.network.scope.once", "Somente esta requisição", nettrust.ScopeOnce},
	{"app.questionnaire.network.scope.session", "Durante esta conversa", nettrust.ScopeSession},
	{"app.questionnaire.network.scope.workspace", "Neste workspace (projeto)", nettrust.ScopeWorkspace},
	{"app.questionnaire.network.scope.profile", "Neste perfil", nettrust.ScopeProfile},
	{"app.questionnaire.network.scope.global", "Global (todos os workspaces e perfis)", nettrust.ScopeGlobal},
}

// scopeOptionSep separa o valor ESTÁVEL do escopo (parseável pelo backend) do
// rótulo humano dentro da mesma option. O parse (scopeFromOption) usa apenas o
// prefixo estável, então o copy pode mudar — ou ganhar i18n — sem quebrar o
// fluxo de consentimento. Ex.: "session — Durante esta conversa".
const scopeOptionSep = " — "

func scopeOptionText(o scopeOption) string {
	return string(o.scope) + scopeOptionSep + o.label
}

// scopeOptions monta as opções para a tela. O valor que volta em Answers é o
// fallback (scopeOptionText), com o prefixo estável que scopeFromOption parseia;
// a tradução mostra só o rótulo humano, porque o prefixo é máquina e quem lê o
// diálogo não tem o que fazer com ele (AEP-0085).
func scopeOptions() []questionnaire.Text {
	out := make([]questionnaire.Text, 0, len(networkScopeOptions))
	for _, o := range networkScopeOptions {
		out = append(out, questionnaire.Keyed(o.key, scopeOptionText(o)))
	}
	return out
}

// scopeFromOption extrai o Scope da option escolhida usando apenas o prefixo
// estável antes de scopeOptionSep (com tolerância a uma resposta que já venha só
// com o valor do escopo). Não depende do rótulo humano.
func scopeFromOption(option string) (nettrust.Scope, bool) {
	value := option
	if i := strings.Index(option, scopeOptionSep); i >= 0 {
		value = option[:i]
	}
	value = strings.TrimSpace(value)
	for _, o := range networkScopeOptions {
		if string(o.scope) == value {
			return o.scope, true
		}
	}
	return "", false
}

// appNetworkPrompter implementa nettrust.Prompter usando o questionnaire manager,
// reutilizando exatamente o mesmo mecanismo de consentimento já usado para
// operações destrutivas de HTTP e execução de comandos shell.
type appNetworkPrompter struct {
	qm *questionnaire.Manager
}

func (p *appNetworkPrompter) PromptNetworkAuthorization(ctx context.Context, req nettrust.PromptRequest) (nettrust.PromptDecision, error) {
	if p.qm == nil {
		return nettrust.PromptDecision{}, fmt.Errorf("questionnaire manager não inicializado")
	}

	var details strings.Builder
	fmt.Fprintf(&details, "Host: %s\n", req.Host)
	if req.Port != "" {
		fmt.Fprintf(&details, "Porta: %s\n", req.Port)
	}
	if len(req.IPs) > 0 {
		fmt.Fprintf(&details, "IP resolvido: %s\n", strings.Join(req.IPs, ", "))
	}
	fmt.Fprintf(&details, "Motivo: %s\n", req.Reason)
	if req.SkillSlug != "" {
		fmt.Fprintf(&details, "Skill: %s\n", req.SkillSlug)
	}
	if len(req.SkillSuggestedHosts) > 0 {
		fmt.Fprintf(&details, "Hosts esperados pelo skill: %s\n", strings.Join(req.SkillSuggestedHosts, ", "))
	}

	// Quando o destino bloqueado é um dos hosts que o skill declarou esperar,
	// dizer isso ao lado dos detalhes evita que a pessoa compare a lista na mão.
	// Continua sendo só informação: a autorização depende da resposta (AEP-0082 D5).
	var skillHostHint questionnaire.Text
	if req.SkillHostMatch != "" {
		skillHostHint = questionnaire.KeyedWith(
			"app.questionnaire.network.skillHostMatch",
			map[string]any{"pattern": req.SkillHostMatch},
			fmt.Sprintf("Este destino casa com %s, declarado pelo skill como host esperado. Isso não dispensa a sua autorização.", req.SkillHostMatch),
		)
	}

	resp, err := p.qm.RequestQuestionnaire(ctx, questionnaire.RequestPayload{
		Title: questionnaire.Keyed(
			"app.questionnaire.network.title",
			"Autorizar acesso a host bloqueado (anti-SSRF)",
		),
		Description: questionnaire.KeyedWith(
			"app.questionnaire.network.description",
			map[string]any{"category": req.Category},
			fmt.Sprintf("O assistente tentou acessar um host que resolve para um endereço interno/privado (%s). Autorize apenas se você confia neste destino.", req.Category),
		),
		AllowCancel: true,
		SubmitLabel: questionnaire.Keyed("app.questionnaire.network.submit", "Autorizar"),
		CancelLabel: questionnaire.Keyed("app.questionnaire.network.cancel", "Negar"),
		Questions: []questionnaire.Question{
			{
				ID:          "details",
				Type:        "readonly_code",
				Prompt:      questionnaire.Keyed("app.questionnaire.network.detailsPrompt", "Detalhes do destino"),
				Description: skillHostHint,
				// Host, porta, IP e motivo são dados do pedido: vão como
				// conteúdo, não como chave.
				Content: details.String(),
			},
			{
				ID:       "scope",
				Type:     "single_choice",
				Prompt:   questionnaire.Keyed("app.questionnaire.network.scopePrompt", "Por quanto tempo autorizar este host?"),
				Required: true,
				Options:  scopeOptions(),
				Default:  scopeOptionText(networkScopeOptions[0]),
			},
			{
				ID:          "reason",
				Type:        "text",
				Prompt:      questionnaire.Keyed("app.questionnaire.network.reasonPrompt", "Observação (opcional)"),
				Placeholder: questionnaire.Keyed("app.questionnaire.network.reasonPlaceholder", "Ex.: API interna de workflows"),
			},
		},
	})
	if err != nil {
		return nettrust.PromptDecision{}, err
	}
	if resp.Cancelled {
		return nettrust.PromptDecision{Approve: false}, nil
	}

	option, _ := resp.Answers["scope"].(string)
	scope, ok := scopeFromOption(option)
	if !ok {
		return nettrust.PromptDecision{}, fmt.Errorf("escopo de autorização inválido: %q", option)
	}
	reason, _ := resp.Answers["reason"].(string)

	return nettrust.PromptDecision{Approve: true, Scope: scope, Reason: reason}, nil
}

// ============================================================================
// API Wails para gestão (base para a futura UI de allowlists de rede)
// ============================================================================

// NetworkAllowlistView é a projeção de uma entrada para o frontend.
type NetworkAllowlistView struct {
	Host        string   `json:"host"`
	Port        string   `json:"port,omitempty"`
	Scope       string   `json:"scope"`
	Category    string   `json:"category,omitempty"`
	ResolvedIPs []string `json:"resolvedIps,omitempty"`
	CreatedBy   string   `json:"createdBy,omitempty"`
	CreatedAt   string   `json:"createdAt"`
	Reason      string   `json:"reason,omitempty"`
}

// networkManagementContext injeta a identidade (perfil ativo) no ctx para que os
// escopos de perfil sejam considerados nas operações de gestão chamadas pela UI.
func (a *App) networkManagementContext() context.Context {
	ctx := a.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	// profileManager pode ser nil em inicialização parcial/testes; sem ele não há
	// perfil ativo a considerar.
	if a.profileManager != nil {
		if slug := a.profileManager.GetActiveSlug(); slug != "" {
			ctx = invocationctx.With(ctx, invocationctx.InvocationContext{ProfileSlug: slug})
		}
	}
	return ctx
}

// GetNetworkAllowlist lista as entradas de allowlist de rede (workspace, global e
// perfil ativo). Base para a futura tela de gestão.
func (a *App) GetNetworkAllowlist() []NetworkAllowlistView {
	if a.netTrustMgr == nil {
		return nil
	}
	entries := a.netTrustMgr.List(a.networkManagementContext())
	views := make([]NetworkAllowlistView, 0, len(entries))
	for _, e := range entries {
		views = append(views, NetworkAllowlistView{
			Host:        e.Host,
			Port:        e.Port,
			Scope:       string(e.Scope),
			Category:    e.Category,
			ResolvedIPs: e.ResolvedIPs,
			CreatedBy:   e.CreatedBy,
			CreatedAt:   e.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
			Reason:      e.Reason,
		})
	}
	return views
}

// RemoveNetworkAllowlistEntry remove uma entrada persistida por (scope, host, port).
// Aceita apenas escopos PERSISTIDOS (workspace/profile/global): once nunca é
// persistido e session vive em memória por conversa, sem relação com esta API de
// gestão — passar esses valores retorna erro de escopo inválido direto, em vez de
// um erro confuso vindo do Manager.
func (a *App) RemoveNetworkAllowlistEntry(scope, host, port string) error {
	if a.netTrustMgr == nil {
		return fmt.Errorf("gerenciador de allowlist de rede não inicializado")
	}
	s := nettrust.Scope(scope)
	if !s.IsPersistent() {
		return fmt.Errorf("escopo inválido para remoção: %q (use workspace, profile ou global)", scope)
	}
	return a.netTrustMgr.Remove(a.networkManagementContext(), s, host, port)
}
