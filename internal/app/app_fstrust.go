package app

import (
	"assistente/internal/authorization"
	"context"
	"fmt"
	"strings"

	"assistente/internal/fstrust"
	"assistente/internal/questionnaire"
)

// ============================================================================
// Filesystem Path Trust (allowlist fora do sandbox) — Prompter (AEP-0092)
// ============================================================================

type appFSPrompter struct {
	ask func(context.Context, questionnaire.RequestPayload) (questionnaire.Response, error)
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
		Kind:     questionnaire.KindDecision,
		Severity: questionnaire.DecisionSeverityPermission,
		Title: questionnaire.Keyed(
			"app.questionnaire.fstrust.title",
			"Autorizar acesso a caminho fora do workspace",
		),
		Description: questionnaire.KeyedWith(
			"app.questionnaire.fstrust.description",
			map[string]any{"operation": req.Operation},
			fmt.Sprintf("O assistente pediu a operação \"%s\" em um caminho fora do workspace ativo e de ~/.assistente. Permita o path ou a pasta pai, negue esta tentativa ou lembre a negação no escopo escolhido.", req.Operation),
		),
		Body: details.String(),
		BodyLabel: questionnaire.Keyed(
			"app.questionnaire.fstrust.bodyLabel",
			"Caminho e detalhes solicitados",
		),
		AllowCancel: true,
		Actions:     (questionnaire.AuthorizationActions{Scopes: req.AvailableScopes, Directory: true, RememberDenial: true, PersistentShortcut: authorization.ScopeWorkspace}).Actions(),
	}
}

func (p *appFSPrompter) PromptPathAuthorization(ctx context.Context, req fstrust.PromptRequest) (fstrust.PromptDecision, error) {
	if p.ask == nil {
		return fstrust.PromptDecision{}, fmt.Errorf("questionnaire manager não inicializado")
	}

	resp, err := p.ask(ctx, pathConfirmationPayload(req))
	if err != nil {
		return fstrust.PromptDecision{}, err
	}
	choice, err := (questionnaire.AuthorizationActions{Scopes: req.AvailableScopes, Directory: true, RememberDenial: true, PersistentShortcut: authorization.ScopeWorkspace}).Parse(resp)
	if err != nil {
		return fstrust.PromptDecision{}, err
	}
	kind := fstrust.KindFile
	if choice.Directory {
		kind = fstrust.KindDir
	}
	return fstrust.PromptDecision{Approve: choice.Effect == "allow", Scope: choice.Scope, Kind: kind, Effect: fstrust.Effect(choice.Effect)}, nil
}
