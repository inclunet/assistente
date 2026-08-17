package app

import (
	"context"
	"fmt"
	"strings"

	"assistente/internal/fstrust"
	"assistente/internal/questionnaire"
)

// ============================================================================
// Filesystem Path Trust (allowlist fora do sandbox) — Prompter (AEP-0092)
// ============================================================================

type fsScopeOption struct {
	key   string
	label string
	scope fstrust.Scope
}

var fsScopeOptions = []fsScopeOption{
	{"app.questionnaire.fstrust.scope.once", "Somente esta tentativa", fstrust.ScopeOnce},
	{"app.questionnaire.fstrust.scope.session", "Durante esta conversa", fstrust.ScopeSession},
	{"app.questionnaire.fstrust.scope.workspace", "Neste workspace (projeto)", fstrust.ScopeWorkspace},
	{"app.questionnaire.fstrust.scope.profile", "Neste perfil", fstrust.ScopeProfile},
	{"app.questionnaire.fstrust.scope.global", "Global (todos os workspaces e perfis)", fstrust.ScopeGlobal},
}

const (
	fsActionDenyPrefix = "deny"
	fsActionDirPrefix  = "dir-"
)

func fsDecisionActions() []questionnaire.DecisionAction {
	out := make([]questionnaire.DecisionAction, 0, len(fsScopeOptions)*2+1)
	for i, o := range fsScopeOptions {
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
	// Ações explícitas para liberar a pasta pai (D-Q1): nunca automáticas.
	for _, o := range fsScopeOptions {
		out = append(out, questionnaire.DecisionAction{
			ID: fsActionDirPrefix + string(o.scope),
			Label: questionnaire.Keyed(
				"app.questionnaire.fstrust.scope.dir."+string(o.scope),
				fmt.Sprintf("Pasta pai — %s", o.label),
			),
			Variant: "secondary",
		})
	}
	out = append(out, questionnaire.DecisionAction{
		ID:      fsActionDenyPrefix,
		Label:   questionnaire.Keyed("app.questionnaire.fstrust.cancel", "Negar"),
		Variant: "outline",
	})
	return out
}

func fsScopeFromActionID(actionID string) (scope fstrust.Scope, kind fstrust.Kind, ok bool) {
	value := strings.TrimSpace(actionID)
	if value == "" || value == fsActionDenyPrefix {
		return "", "", false
	}
	kind = fstrust.KindFile
	if strings.HasPrefix(value, fsActionDirPrefix) {
		kind = fstrust.KindDir
		value = strings.TrimPrefix(value, fsActionDirPrefix)
	}
	for _, o := range fsScopeOptions {
		if string(o.scope) == value {
			return o.scope, kind, true
		}
	}
	return "", "", false
}

type appFSPrompter struct {
	qm *questionnaire.Manager
}

func pathConfirmationPayload(req fstrust.PromptRequest) questionnaire.RequestPayload {
	var details strings.Builder
	fmt.Fprintf(&details, "path: %s\n", req.Path)
	if req.ResolvedPath != "" && req.ResolvedPath != req.Path {
		fmt.Fprintf(&details, "resolved: %s\n", req.ResolvedPath)
	}
	fmt.Fprintf(&details, "operation: %s\n", req.Operation)
	if req.SkillSlug != "" {
		fmt.Fprintf(&details, "skill: %s\n", req.SkillSlug)
	}

	return questionnaire.RequestPayload{
		Kind: questionnaire.KindDecision,
		Title: questionnaire.Keyed(
			"app.questionnaire.fstrust.title",
			"Autorizar acesso a caminho fora do workspace",
		),
		Description: questionnaire.KeyedWith(
			"app.questionnaire.fstrust.description",
			map[string]any{"operation": req.Operation},
			fmt.Sprintf("O assistente pediu a operação \"%s\" em um caminho fora do workspace ativo e de ~/.assistente. Autorize apenas o path exato desta tentativa, ou escolha explicitamente liberar a pasta pai.", req.Operation),
		),
		Body:        details.String(),
		AllowCancel: true,
		Actions:     fsDecisionActions(),
	}
}

func (p *appFSPrompter) PromptPathAuthorization(ctx context.Context, req fstrust.PromptRequest) (fstrust.PromptDecision, error) {
	if p.qm == nil {
		return fstrust.PromptDecision{}, fmt.Errorf("questionnaire manager não inicializado")
	}

	resp, err := p.qm.RequestQuestionnaire(ctx, pathConfirmationPayload(req))
	if err != nil {
		return fstrust.PromptDecision{}, err
	}
	if resp.Cancelled {
		return fstrust.PromptDecision{Approve: false}, nil
	}

	actionID, ok := questionnaire.DecisionActionID(resp)
	if !ok {
		return fstrust.PromptDecision{}, fmt.Errorf("resposta de autorização de path sem ação")
	}
	actionID = strings.TrimSpace(actionID)
	if actionID == fsActionDenyPrefix {
		return fstrust.PromptDecision{Approve: false}, nil
	}

	scope, kind, ok := fsScopeFromActionID(actionID)
	if !ok {
		return fstrust.PromptDecision{}, fmt.Errorf("ação de autorização de path inválida: %q", actionID)
	}

	return fstrust.PromptDecision{Approve: true, Scope: scope, Kind: kind}, nil
}
