package app

import (
	"context"
	"fmt"

	"assistente/internal/questionnaire"
	"assistente/internal/shelltrust"
)

// Diálogos de decisão crítica das tools: o que a pessoa lê antes de deixar o
// assistente rodar um comando na máquina dela ou disparar uma requisição que
// muda estado do outro lado.
//
// Os textos vão com chave de tradução e com o texto pronto em pt-BR (AEP-0085):
// quem usa o app em inglês ou espanhol precisa entender o pedido — é justamente
// aqui que não entender custa caro. O que é dado do pedido (comando, diretório,
// URL, body) vai como parâmetro da tradução ou como Body cru (AEP-0091).

const (
	decisionAllow = "allow"
	decisionDeny  = "deny"
)

func askShellConfirmation(ctx context.Context, router *questionnaire.Router, surface questionnaire.Surface, req shelltrust.PromptRequest) (shelltrust.PromptDecision, error) {
	payload, actions := shellConfirmationPayload(req)
	if receipt := shelltrust.ConfirmationReceipt(ctx); receipt != nil {
		payload = payload.WithAcceptGuard(func(resp questionnaire.Response) bool {
			choice, err := actions.Parse(resp)
			return receipt.TryAccept(err == nil && choice.Effect == "deny" && choice.Scope == "once")
		})
	}
	resp, err := router.Ask(ctx, surface, payload)
	if err != nil {
		return shelltrust.PromptDecision{}, err
	}
	choice, err := actions.Parse(resp)
	return shelltrust.PromptDecision{Approve: choice.Effect == "allow", Scope: choice.Scope}, err
}

// shellConfirmationPayload monta a confirmação de execução de comando (AEP-0091).
func shellConfirmationPayload(req shelltrust.PromptRequest) (questionnaire.RequestPayload, questionnaire.AuthorizationActions) {
	actions := questionnaire.AuthorizationActions{Scopes: req.AvailableScopes}
	if req.ProtectedCommand {
		description := questionnaire.Keyed("app.questionnaire.shell.protectedPrompt", "Este comando de job usa um valor protegido. Os detalhes ficam ocultos. Permitir uma única execução no shell?")
		if req.LiteralPrefix != "" {
			description = questionnaire.KeyedWith("app.questionnaire.shell.protectedPrefixPrompt", map[string]any{"prefix": req.LiteralPrefix},
				fmt.Sprintf("O comando de job começa com %s e usa um valor protegido. Os demais detalhes ficam ocultos. Permitir uma única execução no shell?", req.LiteralPrefix))
		}
		var bodyLabel questionnaire.Text
		var body string
		if req.PublicTemplate != "" && req.LiteralPrefix != "" {
			body = req.PublicTemplate
			description = questionnaire.KeyedWith("app.questionnaire.shell.protectedTemplatePrompt", map[string]any{"prefix": req.LiteralPrefix},
				fmt.Sprintf("O comando de job começa com %s. O template mostra a referência ao cofre. Permitir uma única execução no shell?", req.LiteralPrefix))
			bodyLabel = questionnaire.Keyed("app.questionnaire.shell.templateLabel", "Template original do comando")
		}
		var hint questionnaire.Text
		if req.PersistentTerminal {
			hint = questionnaire.Keyed("app.questionnaire.shell.protectedPersistentHint", "Terminal persistente. O diretório atual pode ter mudado.")
		}
		return questionnaire.RequestPayload{
			Hint: hint, Body: body, BodyLabel: bodyLabel,
			Kind: questionnaire.KindDecision, Title: questionnaire.Keyed("app.questionnaire.shell.title", "Confirmar execução de comando"),
			Description: description, AllowCancel: true, Actions: actions.Actions(),
		}, actions
	}
	hint := questionnaire.KeyedWith(
		"app.questionnaire.shell.workDir",
		map[string]any{"workDir": req.Directory},
		fmt.Sprintf("Diretório: %s", req.Directory),
	)
	if req.PersistentTerminal {
		hint = questionnaire.KeyedWith(
			"app.questionnaire.shell.persistentWorkDir",
			map[string]any{"workDir": req.Directory},
			fmt.Sprintf("Terminal persistente. Diretório inicial: %s; o diretório atual pode ter mudado. A autorização vale só para esta execução.", req.Directory),
		)
	}
	return questionnaire.RequestPayload{
		Kind: questionnaire.KindDecision,
		Title: questionnaire.Keyed(
			"app.questionnaire.shell.title",
			"Confirmar execução de comando",
		),
		Description: questionnaire.Keyed(
			"app.questionnaire.shell.prompt",
			"Permitir a execução deste comando?",
		),
		// Diretório vai como Hint traduzível (rótulo "Diretório:" localizado);
		// Body fica só com o comando cru, sem conectivo em pt-BR (AEP-0091).
		Hint: hint,
		Body: req.Command,
		BodyLabel: questionnaire.Keyed(
			"app.questionnaire.shell.bodyLabel",
			"Comando solicitado",
		),
		AllowCancel: true,
		Actions:     actions.Actions(),
	}, actions
}

// httpConfirmationPayload monta a confirmação de operação HTTP mutável (AEP-0091).
func httpConfirmationPayload(method, url, bodyPreview string) questionnaire.RequestPayload {
	return questionnaire.RequestPayload{
		Kind: questionnaire.KindDecision,
		Title: questionnaire.KeyedWith(
			"app.questionnaire.http.title",
			map[string]any{"method": method},
			fmt.Sprintf("Confirmar operação %s", method),
		),
		Description: questionnaire.KeyedWith(
			"app.questionnaire.http.prompt",
			map[string]any{"method": method},
			fmt.Sprintf("Permitir esta operação %s?", method),
		),
		// Pedido cru no Body (método, URL e body); rótulos localizados ficam
		// fora para não vazar pt-BR em en/es.
		Body: fmt.Sprintf("%s %s\n\n%s", method, url, bodyPreview),
		BodyLabel: questionnaire.Keyed(
			"app.questionnaire.http.bodyLabel",
			"Detalhes da operação solicitada",
		),
		AllowCancel: true,
		Actions: []questionnaire.DecisionAction{
			{
				ID:       decisionAllow,
				Label:    questionnaire.Keyed("app.questionnaire.http.submit", "Permitir"),
				Variant:  "primary",
				Primary:  true,
				Polarity: questionnaire.DecisionPolarityAffirmative,
				Scope:    questionnaire.DecisionScopeCurrent,
			},
			{
				ID:       decisionDeny,
				Label:    questionnaire.Keyed("app.questionnaire.http.cancel", "Negar"),
				Variant:  "outline",
				Polarity: questionnaire.DecisionPolarityNegative,
				Scope:    questionnaire.DecisionScopeCurrent,
			},
		},
	}
}

// approvedFromHTTPDecision valida a confirmação pontual de uma mutação HTTP.
func approvedFromHTTPDecision(resp questionnaire.Response) (bool, error) {
	if resp.Cancelled {
		return false, nil
	}
	id, ok := questionnaire.DecisionActionID(resp)
	if !ok {
		return false, fmt.Errorf("resposta inválida para aprovação HTTP")
	}
	switch id {
	case decisionAllow:
		return true, nil
	case decisionDeny:
		return false, nil
	default:
		return false, fmt.Errorf("ação de decisão desconhecida: %q", id)
	}
}
