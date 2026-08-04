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
// URL, body) vai como parâmetro da tradução, porque não existe tradução para o
// comando que o modelo quer rodar.

// shellConfirmationPayload monta a confirmação de execução de comando.
func shellConfirmationPayload(cmd, workDir string) questionnaire.RequestPayload {
	return questionnaire.RequestPayload{
		Title: questionnaire.Keyed(
			"app.questionnaire.shell.title",
			"Confirmar execução de comando",
		),
		Description: questionnaire.KeyedWith(
			"app.questionnaire.shell.description",
			map[string]any{"command": cmd, "workDir": workDir},
			fmt.Sprintf("O assistente quer executar:\n\n%s\n\nem: %s", cmd, workDir),
		),
		AllowCancel: true,
		SubmitLabel: questionnaire.Keyed("app.questionnaire.shell.submit", "Permitir"),
		CancelLabel: questionnaire.Keyed("app.questionnaire.shell.cancel", "Negar"),
		Questions: []questionnaire.Question{
			{
				ID:       "approve",
				Type:     "boolean",
				Prompt:   questionnaire.Keyed("app.questionnaire.shell.prompt", "Permitir a execução deste comando?"),
				Required: true,
			},
		},
	}
}

// httpConfirmationPayload monta a confirmação de operação HTTP mutável.
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
