package app

import (
	"context"
	"strings"
	"testing"

	"assistente/internal/nettrust"
	"assistente/internal/questionnaire"
)

// dialogoDeRede abre o consentimento de rede e devolve o payload que chegaria à
// tela, respondendo o que o teste combinou (id de ação ou valor legado de escopo).
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
			actionID := escolha
			if escopo, ok := scopeFromOption(escolha); ok {
				actionID = string(escopo)
			}
			_ = mgr.Respond(id, map[string]any{questionnaire.AnswerActionID: actionID}, false)
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
		string(nettrust.ScopeOnce),
	)

	for _, campo := range []string{"title", "description"} {
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

	if kind, _ := payload["kind"].(string); kind != questionnaire.KindDecision {
		t.Errorf("kind = %q, quer %q", kind, questionnaire.KindDecision)
	}

	actions, ok := payload["actions"].([]questionnaire.DecisionAction)
	if !ok || len(actions) == 0 {
		t.Fatalf("actions = %T %#v, quer []DecisionAction", payload["actions"], payload["actions"])
	}
	for _, action := range actions {
		if action.Label.Key == "" || action.Label.Fallback == "" {
			t.Errorf("ação %q = %+v, quer chave e fallback", action.ID, action.Label)
		}
	}

	descricao, _ := payload["description"].(questionnaire.Text)
	if got := descricao.Params["category"]; got != "private" {
		t.Errorf("categoria nos params = %v, quer a do pedido", got)
	}
}

func TestOEscopoEscolhidoContinuaSendoParseadoDepoisDaTraducao(t *testing.T) {
	// Os ids das ações são os escopos estáveis; labels traduzem sem afetar o parse.
	for _, action := range networkDecisionActions() {
		if action.ID == decisionDeny {
			continue
		}
		if action.Label.Key == "" {
			t.Errorf("ação %+v sem chave: o escopo continuaria só em português", action)
		}
		escopo, ok := scopeFromOption(action.ID)
		if !ok {
			t.Errorf("o id da ação %q não volta a um escopo", action.ID)
			continue
		}
		if string(escopo) != action.ID {
			t.Errorf("escopo = %q, quer %q", escopo, action.ID)
		}
	}
	// Compat: valores legados "session — …" ainda parseiam.
	for _, opcao := range scopeOptions() {
		if _, ok := scopeFromOption(opcao.String()); !ok {
			t.Errorf("valor legado %q não parseia", opcao.String())
		}
	}
}

func TestODialogoDestacaOHostQueOSkillDeclarou(t *testing.T) {
	payload, _ := dialogoDeRede(t,
		nettrust.PromptRequest{
			Host:                "api.nu.workflows.dev",
			Category:            "cgnat",
			SkillSlug:           "workflows-api",
			SkillSuggestedHosts: []string{"*.nu.workflows.dev"},
			SkillHostMatch:      "*.nu.workflows.dev",
		},
		string(nettrust.ScopeOnce),
	)

	body, _ := payload["body"].(string)
	if !strings.Contains(body, "*.nu.workflows.dev") {
		t.Errorf("body = %q, quer nomear o host declarado", body)
	}
}

func TestODialogoNaoDestacaHostQuandoNenhumCasa(t *testing.T) {
	payload, _ := dialogoDeRede(t,
		nettrust.PromptRequest{
			Host:                "api.nu.workflows.dev",
			Category:            "cgnat",
			SkillSlug:           "workflows-api",
			SkillSuggestedHosts: []string{"outra.coisa.dev"},
		},
		string(nettrust.ScopeOnce),
	)

	body, _ := payload["body"].(string)
	if strings.Contains(body, "casa com") {
		t.Errorf("body = %q, não quer destaque de match", body)
	}
}

func TestADecisaoDeRedeSegueOEscopoQueAPessoaEscolheu(t *testing.T) {
	_, decision := dialogoDeRede(t,
		nettrust.PromptRequest{Host: "interno.local", Category: "private"},
		string(nettrust.ScopeSession),
	)

	if !decision.Approve {
		t.Fatal("a autorização não passou")
	}
	if decision.Scope != nettrust.ScopeSession {
		t.Errorf("escopo = %q, quer %q", decision.Scope, nettrust.ScopeSession)
	}
}
