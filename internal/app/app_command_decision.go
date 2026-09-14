package app

import (
	"context"
	"time"

	"assistente/internal/commanddecision"
	"assistente/internal/questionnaire"
)

// commandDecisionPresenter adapta a porta backend ao gerenciador desktop.
// O gerenciador controla o ID curto da UI; o adapter associa a resposta ao
// ID de decisão criado pelo backend, sem aceitar essa identidade do cliente.
type commandDecisionPresenter struct {
	manager *questionnaire.Manager
}

var _ commanddecision.Presenter = (*commandDecisionPresenter)(nil)

func (p *commandDecisionPresenter) Present(ctx context.Context, req commanddecision.Request) (commanddecision.Response, error) {
	if p == nil || p.manager == nil || ctx == nil {
		return commanddecision.Response{}, commanddecision.ErrInvalid
	}
	if err := ctx.Err(); err != nil {
		return commanddecision.Response{}, err
	}

	remaining := time.Until(req.ExpiresAt)
	if remaining <= 0 {
		return commanddecision.Response{}, commanddecision.ErrInvalid
	}

	// O prazo absoluto inclui a fila de diálogos. WithDeadline preserva um
	// deadline anterior do chamador quando ele existir.
	deadlineCtx, cancel := context.WithDeadline(ctx, req.ExpiresAt)
	defer cancel()

	resp, err := p.manager.RequestQuestionnaire(deadlineCtx, questionnaire.RequestPayload{
		Kind:        questionnaire.KindDecision,
		Severity:    questionnaire.DecisionSeverityPermission,
		Title:       questionnaire.Keyed("app.questionnaire.commandBinding.title", "Aplicar alteração de comandos?"),
		Description: questionnaire.Keyed("app.questionnaire.commandBinding.description", "Uma alteração na configuração de comandos está pronta para ser aplicada. Deseja aplicar?"),
		Body:        req.Body,
		BodyLabel:   questionnaire.Keyed("app.questionnaire.commandBinding.bodyLabel", "Alteração solicitada"),
		Actions: []questionnaire.DecisionAction{
			{
				ID:       commanddecision.ApplyAction,
				Label:    questionnaire.Keyed("app.questionnaire.commandBinding.apply", "Aplicar"),
				Variant:  "primary",
				Primary:  true,
				Polarity: questionnaire.DecisionPolarityAffirmative,
				Scope:    questionnaire.DecisionScopePersistent,
			},
			{
				ID:       commanddecision.DenyAction,
				Label:    questionnaire.Keyed("app.questionnaire.commandBinding.deny", "Negar"),
				Variant:  "outline",
				Polarity: questionnaire.DecisionPolarityNegative,
				Scope:    questionnaire.DecisionScopeCurrent,
			},
		},
		AllowCancel: true,
	})
	if err != nil {
		return commanddecision.Response{}, err
	}

	result := commanddecision.Response{DecisionID: req.DecisionID}
	if resp.Cancelled {
		result.Cancelled = true
		return result, nil
	}

	actionID, ok := questionnaire.DecisionActionID(resp)
	if !ok {
		return commanddecision.Response{}, commanddecision.ErrInvalid
	}
	switch actionID {
	case commanddecision.ApplyAction, commanddecision.DenyAction:
		result.ActionID = actionID
		return result, nil
	default:
		return commanddecision.Response{}, commanddecision.ErrInvalid
	}
}
