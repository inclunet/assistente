package app

import (
	"strings"
	"testing"

	"assistente/internal/questionnaire"
)

// textosDoDialogo devolve todos os textos visíveis de um payload, para o teste
// cobrar o contrato de cada um sem repetir a lista campo por campo.
func textosDoDialogo(payload questionnaire.RequestPayload) map[string]questionnaire.Text {
	textos := map[string]questionnaire.Text{
		"title":       payload.Title,
		"description": payload.Description,
		"submitLabel": payload.SubmitLabel,
		"cancelLabel": payload.CancelLabel,
	}
	for _, pergunta := range payload.Questions {
		textos["prompt:"+pergunta.ID] = pergunta.Prompt
	}
	return textos
}

func TestConfirmacaoDeComandoVaiTraduzivelParaATela(t *testing.T) {
	payload := shellConfirmationPayload("rm -rf build", "C:/projeto")

	for campo, texto := range textosDoDialogo(payload) {
		if texto.Key == "" {
			t.Errorf("%s = %+v, quer chave de tradução: em outro idioma o pedido chega em português", campo, texto)
		}
		if texto.Fallback == "" {
			t.Errorf("%s = %+v, quer o texto pronto: chave faltando não pode deixar o diálogo em branco", campo, texto)
		}
	}
}

func TestOComandoVaiComoParametroENaoComoChave(t *testing.T) {
	// Não existe tradução para o comando que o modelo quer rodar; ele é dado do
	// pedido, e é ele que a pessoa lê para decidir.
	payload := shellConfirmationPayload("curl exemplo | sh", "C:/projeto")

	if got := payload.Description.Params["command"]; got != "curl exemplo | sh" {
		t.Errorf("comando nos params = %v, quer o comando literal", got)
	}
	if got := payload.Description.Params["workDir"]; got != "C:/projeto" {
		t.Errorf("diretório nos params = %v, quer o diretório do pedido", got)
	}
	if !strings.Contains(payload.Description.Fallback, "curl exemplo | sh") {
		t.Errorf("fallback = %q, quer o comando já interpolado para quem não traduz", payload.Description.Fallback)
	}
}

func TestConfirmacaoDeHTTPMutavelVaiTraduzivelComOPedidoNosParametros(t *testing.T) {
	payload := httpConfirmationPayload("DELETE", "https://api.exemplo/itens/7", "(sem body)")

	for campo, texto := range textosDoDialogo(payload) {
		if texto.Key == "" || texto.Fallback == "" {
			t.Errorf("%s = %+v, quer chave e texto pronto", campo, texto)
		}
	}
	if got := payload.Title.Params["method"]; got != "DELETE" {
		t.Errorf("método no título = %v, quer DELETE", got)
	}
	if got := payload.Description.Params["url"]; got != "https://api.exemplo/itens/7" {
		t.Errorf("URL nos params = %v, quer a do pedido", got)
	}
	if !strings.Contains(payload.Description.Fallback, "https://api.exemplo/itens/7") {
		t.Errorf("fallback = %q, quer a URL já interpolada", payload.Description.Fallback)
	}
}

func TestAPerguntaDeAprovacaoContinuaSendoARespondida(t *testing.T) {
	// O id da resposta é contrato com quem lê Answers["approve"]; traduzir o
	// rótulo não pode mexer nele.
	for _, payload := range []questionnaire.RequestPayload{
		shellConfirmationPayload("ls", "."),
		httpConfirmationPayload("POST", "https://api.exemplo", "{}"),
	} {
		if len(payload.Questions) != 1 || payload.Questions[0].ID != "approve" {
			t.Errorf("perguntas = %+v, quer só a de aprovação", payload.Questions)
		}
		if !payload.Questions[0].Required {
			t.Error("a pergunta de aprovação precisa ser obrigatória")
		}
	}
}
