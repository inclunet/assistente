package app

import (
	"strings"
	"testing"

	"assistente/internal/questionnaire"
)

// textosDoDialogo devolve todos os textos visíveis de um payload clássico.
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

	if payload.Kind != questionnaire.KindDecision {
		t.Errorf("kind = %q, quer %q", payload.Kind, questionnaire.KindDecision)
	}
	for _, campo := range []struct {
		nome  string
		texto questionnaire.Text
	}{
		{"title", payload.Title},
		{"description", payload.Description},
	} {
		if campo.texto.Key == "" {
			t.Errorf("%s = %+v, quer chave de tradução", campo.nome, campo.texto)
		}
		if campo.texto.Fallback == "" {
			t.Errorf("%s = %+v, quer o texto pronto", campo.nome, campo.texto)
		}
	}
	for _, action := range payload.Actions {
		if action.Label.Key == "" || action.Label.Fallback == "" {
			t.Errorf("ação %q = %+v, quer chave e fallback", action.ID, action.Label)
		}
	}
}

func TestOComandoVaiNoBodyENaoComoChave(t *testing.T) {
	payload := shellConfirmationPayload("curl exemplo | sh", "C:/projeto")

	if !strings.Contains(payload.Body, "curl exemplo | sh") {
		t.Errorf("body = %q, quer o comando literal", payload.Body)
	}
	if !strings.Contains(payload.Body, "C:/projeto") {
		t.Errorf("body = %q, quer o diretório do pedido", payload.Body)
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

func TestAcaoDeShellContinuaSendoARespondida(t *testing.T) {
	payload := shellConfirmationPayload("ls", ".")
	ids := make(map[string]bool, len(payload.Actions))
	for _, action := range payload.Actions {
		ids[action.ID] = true
	}
	if !ids[decisionAllow] || !ids[decisionDeny] {
		t.Errorf("ações = %+v, quer allow e deny", payload.Actions)
	}
}

func TestAPerguntaHTTPDeAprovacaoContinuaSendoARespondida(t *testing.T) {
	payload := httpConfirmationPayload("POST", "https://api.exemplo", "{}")
	if len(payload.Questions) != 1 || payload.Questions[0].ID != "approve" {
		t.Errorf("perguntas = %+v, quer só a de aprovação", payload.Questions)
	}
	if !payload.Questions[0].Required {
		t.Error("a pergunta de aprovação precisa ser obrigatória")
	}
}

func TestApprovedFromShellDecision(t *testing.T) {
	ok, err := approvedFromShellDecision(questionnaire.Response{
		Answers: map[string]any{questionnaire.AnswerActionID: decisionAllow},
	})
	if err != nil || !ok {
		t.Fatalf("allow: ok=%v err=%v", ok, err)
	}
	ok, err = approvedFromShellDecision(questionnaire.Response{
		Answers: map[string]any{questionnaire.AnswerActionID: decisionDeny},
	})
	if err != nil || ok {
		t.Fatalf("deny: ok=%v err=%v", ok, err)
	}
	ok, err = approvedFromShellDecision(questionnaire.Response{Cancelled: true})
	if err != nil || ok {
		t.Fatalf("cancel: ok=%v err=%v", ok, err)
	}
}
