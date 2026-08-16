package app

import (
	"context"
	"fmt"
	"strings"

	"assistente/internal/nettrust"
	"assistente/internal/questionnaire"
)

// ============================================================================
// Network Trust (allowlist anti-SSRF escopável) — Prompter
// ============================================================================

// scopeOption associa a chave de tradução ao Scope correspondente. A ordem
// define a apresentação (do mais efêmero ao mais amplo). O id da ação no
// DecisionDialog é o próprio Scope (AEP-0091) — sem prefixo "session — …".
type scopeOption struct {
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

func networkDecisionActions() []questionnaire.DecisionAction {
	out := make([]questionnaire.DecisionAction, 0, len(networkScopeOptions)+1)
	for i, o := range networkScopeOptions {
		out = append(out, questionnaire.DecisionAction{
			ID:      string(o.scope),
			Label:   questionnaire.Keyed(o.key, o.label),
			Variant: "secondary",
			Primary: i == 0,
		})
	}
	if len(out) > 0 {
		out[0].Variant = "primary"
	}
	out = append(out, questionnaire.DecisionAction{
		ID:      decisionDeny,
		Label:   questionnaire.Keyed("app.questionnaire.network.cancel", "Negar"),
		Variant: "outline",
	})
	return out
}

// scopeFromActionID resolve o Scope pelo id da ação do DecisionDialog.
func scopeFromActionID(actionID string) (nettrust.Scope, bool) {
	value := strings.TrimSpace(actionID)
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

func networkConfirmationPayload(req nettrust.PromptRequest) questionnaire.RequestPayload {
	// Body só com valores e chaves de protocolo (host/port/…), sem prosa em
	// pt-BR — o título/descrição/hint carregam o texto traduzível (AEP-0085).
	var details strings.Builder
	fmt.Fprintf(&details, "host: %s\n", req.Host)
	if req.Port != "" {
		fmt.Fprintf(&details, "port: %s\n", req.Port)
	}
	if len(req.IPs) > 0 {
		fmt.Fprintf(&details, "ips: %s\n", strings.Join(req.IPs, ", "))
	}
	if req.Reason != "" {
		fmt.Fprintf(&details, "reason: %s\n", req.Reason)
	}
	if req.SkillSlug != "" {
		fmt.Fprintf(&details, "skill: %s\n", req.SkillSlug)
	}
	if len(req.SkillSuggestedHosts) > 0 {
		fmt.Fprintf(&details, "expected_hosts: %s\n", strings.Join(req.SkillSuggestedHosts, ", "))
	}

	var skillHostHint questionnaire.Text
	if req.SkillHostMatch != "" {
		skillHostHint = questionnaire.KeyedWith(
			"app.questionnaire.network.skillHostMatch",
			map[string]any{"pattern": req.SkillHostMatch},
			fmt.Sprintf("Este destino casa com %s, declarado pelo skill como host esperado. Isso não dispensa a sua autorização.", req.SkillHostMatch),
		)
	}

	return questionnaire.RequestPayload{
		Kind: questionnaire.KindDecision,
		Title: questionnaire.Keyed(
			"app.questionnaire.network.title",
			"Autorizar acesso a host bloqueado (anti-SSRF)",
		),
		Description: questionnaire.KeyedWith(
			"app.questionnaire.network.description",
			map[string]any{"category": req.Category},
			fmt.Sprintf("O assistente tentou acessar um host que resolve para um endereço interno/privado (%s). Autorize apenas se você confia neste destino.", req.Category),
		),
		Hint:        skillHostHint,
		Body:        details.String(),
		AllowCancel: true,
		Actions:     networkDecisionActions(),
	}
}

func (p *appNetworkPrompter) PromptNetworkAuthorization(ctx context.Context, req nettrust.PromptRequest) (nettrust.PromptDecision, error) {
	if p.qm == nil {
		return nettrust.PromptDecision{}, fmt.Errorf("questionnaire manager não inicializado")
	}

	resp, err := p.qm.RequestQuestionnaire(ctx, networkConfirmationPayload(req))
	if err != nil {
		return nettrust.PromptDecision{}, err
	}
	if resp.Cancelled {
		return nettrust.PromptDecision{Approve: false}, nil
	}

	actionID, ok := questionnaire.DecisionActionID(resp)
	if !ok {
		return nettrust.PromptDecision{}, fmt.Errorf("resposta de autorização de rede sem ação")
	}
	if actionID == decisionDeny {
		return nettrust.PromptDecision{Approve: false}, nil
	}

	scope, ok := scopeFromActionID(actionID)
	if !ok {
		return nettrust.PromptDecision{}, fmt.Errorf("escopo de autorização inválido: %q", actionID)
	}

	// Observação livre saiu com o DecisionDialog (só botões). Reason vazio
	// até haver campo opcional sem voltar ao híbrido rádio+texto (AEP-0091).
	return nettrust.PromptDecision{Approve: true, Scope: scope}, nil
}
