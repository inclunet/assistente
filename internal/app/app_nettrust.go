package app

import (
	"assistente/internal/authorization"
	"assistente/internal/database"
	"assistente/internal/userctx"
	"context"
	"fmt"
	"strings"

	"assistente/internal/nettrust"
	"assistente/internal/questionnaire"
)

// ============================================================================
// Network Trust (allowlist anti-SSRF escopável) — Prompter
// ============================================================================

// appNetworkPrompter implementa nettrust.Prompter usando o questionnaire manager,
// reutilizando exatamente o mesmo mecanismo de consentimento já usado para
// operações destrutivas de HTTP e execução de comandos shell.
type appNetworkPrompter struct {
	ask func(context.Context, questionnaire.RequestPayload) (questionnaire.Response, error)
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

	description := questionnaire.KeyedWith(
		"app.questionnaire.network.description",
		map[string]any{"category": req.Category},
		fmt.Sprintf("O assistente tentou acessar um host que resolve para um endereço interno/privado (%s). Autorize apenas se você confia neste destino.", req.Category),
	)
	if len(req.AvailableScopes) > 1 && req.Port != "" {
		ports := req.Port
		if !req.PortExplicit {
			ports = "80, 443"
		}
		description = questionnaire.KeyedWith("app.questionnaire.network.descriptionRememberedPorts",
			map[string]any{"category": req.Category, "ports": ports},
			fmt.Sprintf("O destino está em uma rede interna ou privada (%s). Se lembrar a autorização, ela valerá para estas portas do host: %s.", req.Category, ports))
	}
	return questionnaire.RequestPayload{
		Kind: questionnaire.KindDecision,
		Title: questionnaire.Keyed(
			"app.questionnaire.network.title",
			"Autorizar acesso a host bloqueado (anti-SSRF)",
		),
		Description: description,
		Hint:        skillHostHint,
		Body:        details.String(),
		BodyLabel: questionnaire.Keyed(
			"app.questionnaire.network.bodyLabel",
			"Detalhes do destino",
		),
		AllowCancel: true,
		Actions:     (questionnaire.AuthorizationActions{Scopes: req.AvailableScopes, PersistentShortcut: authorization.ScopeGlobal}).Actions(),
	}
}

func (p *appNetworkPrompter) PromptNetworkAuthorization(ctx context.Context, req nettrust.PromptRequest) (nettrust.PromptDecision, error) {
	if p.ask == nil {
		return nettrust.PromptDecision{}, fmt.Errorf("questionnaire manager não inicializado")
	}

	resp, err := p.ask(ctx, networkConfirmationPayload(req))
	if err != nil {
		return nettrust.PromptDecision{}, err
	}
	choice, err := (questionnaire.AuthorizationActions{Scopes: req.AvailableScopes, PersistentShortcut: authorization.ScopeGlobal}).Parse(resp)
	if err != nil {
		return nettrust.PromptDecision{}, err
	}
	return nettrust.PromptDecision{Approve: choice.Effect == "allow", Scope: choice.Scope}, nil
}

// The desktop repository manager owns this decision even without an active chat.
func (a *App) repositoryNetworkAuthorizer() *nettrust.Authorizer {
	return a.newNetworkAuthorizer(&appNetworkPrompter{ask: a.questionnaireMgr.RequestQuestionnaire})
}

// Keep the decision, vault access and HTTP transfer bound to the desktop login
// that submitted the request, including time spent waiting for another dialog.
func (a *App) watchRepositorySession(ctx context.Context) (context.Context, func(), error) {
	owner, err := database.RequireUserID(ctx)
	if err != nil {
		return nil, nil, err
	}
	session, ok := userctx.SessionIDFromContext(ctx)
	if !ok {
		return nil, nil, database.ErrUserScopeRequired
	}
	epochs, err := a.commandSecurityService()
	if err != nil {
		return nil, nil, err
	}
	snapshot, err := epochs.CaptureAuthenticated(ctx, func(context.Context) (string, string, error) {
		a.authMu.RLock()
		defer a.authMu.RUnlock()
		if a.currentAuthUser == nil || a.currentUserID != owner || a.currentAuthUser.UserID != owner || a.currentAuthUser.SessionID != session {
			return "", "", database.ErrUserScopeRequired
		}
		return owner, session, nil
	})
	if err != nil {
		return nil, nil, err
	}
	return epochs.WatchSecurityEpoch(ctx, snapshot)
}
