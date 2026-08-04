package app

import (
	"testing"

	"assistente/internal/questionnaire"
)

// TestOPedidoDeElevacaoVaiTraduzivelParaATela cobra o diálogo que pede privilégio
// de administrador para substituir o executável. É decisão de segurança: quem o
// lê num idioma que não fala não tem como avaliar o que está autorizando, e a
// resposta padrão para o que não se entende é "sim, tanto faz" (AEP-0085).
func TestOPedidoDeElevacaoVaiTraduzivelParaATela(t *testing.T) {
	payload := updateElevationPayload()

	campos := map[string]questionnaire.Text{
		"title":       payload.Title,
		"description": payload.Description,
		"submitLabel": payload.SubmitLabel,
		"cancelLabel": payload.CancelLabel,
	}
	for _, pergunta := range payload.Questions {
		campos["rótulo de "+pergunta.ID] = pergunta.Prompt
	}

	for nome, texto := range campos {
		if texto.Key == "" {
			t.Errorf("%s = %+v, quer chave de tradução", nome, texto)
		}
		if texto.Fallback == "" {
			t.Errorf("%s = %+v, quer o texto pronto para quem não traduz", nome, texto)
		}
	}

	for nome, esperado := range map[string]string{
		"title":           "Permissão Necessária",
		"description":     "Para atualizar o aplicativo, precisamos de permissões de administrador para substituir o arquivo executável.\n\nDeseja permitir?",
		"submitLabel":     "Permitir",
		"cancelLabel":     "Cancelar",
		"rótulo de allow": "Permitir atualização com privilégios de administrador?",
	} {
		if got := campos[nome].Fallback; got != esperado {
			t.Errorf("%s = %q, quer o texto de antes %q", nome, got, esperado)
		}
	}

	// O pedido é sempre o mesmo: não há dado do ambiente a interpolar, e um
	// parâmetro aqui só poderia vir de fora.
	for nome, texto := range campos {
		if len(texto.Params) != 0 {
			t.Errorf("%s interpola %v, e o pedido de elevação não tem dado a interpolar", nome, texto.Params)
		}
	}
}
