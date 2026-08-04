package app

import (
	"context"
	"strings"
	"testing"

	"assistente/internal/nettrust"
	"assistente/internal/questionnaire"
)

// dialogoDeRede abre o consentimento de rede e devolve o payload que chegaria à
// tela, respondendo o que o teste combinou.
func dialogoDeRede(t *testing.T, req nettrust.PromptRequest, escolha string) (map[string]any, nettrust.PromptDecision) {
	t.Helper()

	var recebido map[string]any
	var mgr *questionnaire.Manager
	mgr = questionnaire.NewManager(func(event string, data any) {
		if event != questionnaire.EventQuestionnaire {
			return
		}
		payload, _ := data.(map[string]any)
		recebido = payload
		id, _ := payload["id"].(string)
		go func() {
			_ = mgr.Respond(id, map[string]any{"scope": escolha, "reason": "API interna"}, false)
		}()
	})

	prompter := &appNetworkPrompter{qm: mgr}
	decision, err := prompter.PromptNetworkAuthorization(context.Background(), req)
	if err != nil {
		t.Fatalf("erro inesperado ao pedir autorização: %v", err)
	}
	if recebido == nil {
		t.Fatal("o diálogo de autorização de rede não chegou à tela")
	}
	return recebido, decision
}

func TestAutorizacaoDeRedeVaiTraduzivelParaATela(t *testing.T) {
	payload, _ := dialogoDeRede(t,
		nettrust.PromptRequest{Host: "interno.local", Category: "private", Reason: "webhook"},
		scopeOptionText(networkScopeOptions[0]),
	)

	for _, campo := range []string{"title", "description", "submitLabel", "cancelLabel"} {
		texto, ok := payload[campo].(questionnaire.Text)
		if !ok {
			t.Fatalf("%s veio como %T, quer questionnaire.Text", campo, payload[campo])
		}
		if texto.Key == "" {
			t.Errorf("%s = %+v, quer chave de tradução", campo, texto)
		}
		if texto.Fallback == "" {
			t.Errorf("%s = %+v, quer o texto pronto para quem não traduz", campo, texto)
		}
	}

	descricao, _ := payload["description"].(questionnaire.Text)
	if got := descricao.Params["category"]; got != "private" {
		t.Errorf("categoria nos params = %v, quer a do pedido", got)
	}
}

func TestOEscopoEscolhidoContinuaSendoParseadoDepoisDaTraducao(t *testing.T) {
	// O rótulo do escopo passa a ser traduzido, mas o valor que volta em
	// Answers é o fallback, com o prefixo estável que o backend parseia. Sem
	// isso, autorizar "durante esta conversa" em inglês viraria escopo inválido.
	for _, opcao := range scopeOptions() {
		if opcao.Key == "" {
			t.Errorf("opção %+v sem chave: o escopo continuaria só em português", opcao)
		}
		escopo, ok := scopeFromOption(opcao.String())
		if !ok {
			t.Errorf("o valor da opção %q não volta a um escopo", opcao.String())
			continue
		}
		if !strings.HasPrefix(opcao.String(), string(escopo)) {
			t.Errorf("valor da opção = %q, quer começar pelo escopo %q", opcao.String(), escopo)
		}
	}
}

func TestADecisaoDeRedeSegueOEscopoQueAPessoaEscolheu(t *testing.T) {
	_, decision := dialogoDeRede(t,
		nettrust.PromptRequest{Host: "interno.local", Category: "private"},
		scopeOptionText(networkScopeOptions[1]),
	)

	if !decision.Approve {
		t.Fatal("a autorização não passou")
	}
	if decision.Scope != nettrust.ScopeSession {
		t.Errorf("escopo = %q, quer %q", decision.Scope, nettrust.ScopeSession)
	}
}
