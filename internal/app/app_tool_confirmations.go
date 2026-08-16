package app

import (
	"fmt"

	"assistente/internal/questionnaire"
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

// shellConfirmationPayload monta a confirmação de execução de comando (AEP-0091).
func shellConfirmationPayload(cmd, workDir string) questionnaire.RequestPayload {
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
		Body:        fmt.Sprintf("%s\n\nem: %s", cmd, workDir),
		AllowCancel: true,
		Actions: []questionnaire.DecisionAction{
			{
				ID:      decisionAllow,
				Label:   questionnaire.Keyed("app.questionnaire.shell.submit", "Permitir"),
				Variant: "primary",
				Primary: true,
			},
			{
				ID:      decisionDeny,
				Label:   questionnaire.Keyed("app.questionnaire.shell.cancel", "Negar"),
				Variant: "outline",
			},
		},
	}
}

// httpConfirmationPayload monta a confirmação de operação HTTP mutável.
// Ainda no formato clássico (Fase 3 do AEP-0091 migra para DecisionDialog).
func httpConfirmationPayload(method, url, bodyPreview string) questionnaire.RequestPayload {
	return questionnaire.RequestPayload{
		Title: questionnaire.KeyedWith(
			"app.questionnaire.http.title",
			map[string]any{"method": method},
			fmt.Sprintf("Confirmar operação %s", method),
		),
		Description: questionnaire.KeyedWith(
			"app.questionnaire.http.description",
			map[string]any{"method": method, "url": url, "body": bodyPreview},
			fmt.Sprintf("O assistente quer executar:\n\n%s %s\n\nBody:\n%s", method, url, bodyPreview),
		),
		AllowCancel: true,
		SubmitLabel: questionnaire.Keyed("app.questionnaire.http.submit", "Permitir"),
		CancelLabel: questionnaire.Keyed("app.questionnaire.http.cancel", "Negar"),
		Questions: []questionnaire.Question{
			{
				ID:   "approve",
				Type: "boolean",
				Prompt: questionnaire.KeyedWith(
					"app.questionnaire.http.prompt",
					map[string]any{"method": method},
					fmt.Sprintf("Permitir esta operação %s?", method),
				),
				Required: true,
			},
		},
	}
}

// approvedFromShellDecision interpreta a resposta kind=decision do shell.
func approvedFromShellDecision(resp questionnaire.Response) (bool, error) {
	if resp.Cancelled {
		return false, nil
	}
	id, ok := questionnaire.DecisionActionID(resp)
	if !ok {
		return false, fmt.Errorf("resposta inválida para aprovação de comando")
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
