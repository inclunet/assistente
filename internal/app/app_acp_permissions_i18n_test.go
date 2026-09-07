package app

import (
	"context"
	"strings"
	"testing"

	"assistente/internal/acp"
	"assistente/internal/questionnaire"
)

// exigirChaveEFallback cobra de cada campo visível do diálogo as duas metades do
// contrato (AEP-0085 D3): sem chave o texto sai em português para quem lê em
// outro idioma, e sem o texto pronto ele sai em branco se a chave faltar num
// locale — e diálogo em branco não é decidível, muito menos por leitor de telas.
func exigirChaveEFallback(t *testing.T, payload map[string]any, campos ...string) {
	t.Helper()
	for _, campo := range campos {
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
}

// exigirRotulosDasPerguntas cobra o mesmo dos rótulos de cada item, e cobra que
// o texto pronto seja o de antes: quem não traduz continua lendo o diálogo
// exatamente como ele era.
func exigirRotulosDasPerguntas(t *testing.T, payload map[string]any, quer map[string]string) {
	t.Helper()
	vistos := make(map[string]bool, len(quer))
	for _, pergunta := range perguntasDe(payload) {
		esperado, conhecido := quer[pergunta.ID]
		if !conhecido {
			t.Errorf("item %q chegou à tela fora do contrato do diálogo", pergunta.ID)
			continue
		}
		vistos[pergunta.ID] = true
		if pergunta.Prompt.Key == "" {
			t.Errorf("rótulo de %q = %+v, quer chave de tradução", pergunta.ID, pergunta.Prompt)
		}
		if pergunta.Prompt.Fallback != esperado {
			t.Errorf("rótulo de %q = %q, quer o texto de antes %q",
				pergunta.ID, pergunta.Prompt.Fallback, esperado)
		}
	}
	for id := range quer {
		if !vistos[id] {
			t.Errorf("o item %q não chegou à tela", id)
		}
	}
}

// dialogoDePermissao abre o pedido e devolve o payload que chegaria à tela.
func dialogoDePermissao(t *testing.T, pedido acp.PermissionRequest) map[string]any {
	t.Helper()
	tela := novaTelaFalsa(func(opcoes []string) string { return opcoes[0] })
	h := handlerCom(tela, acp.TurnOwner{ConversationID: "c", Interactive: true}, true)
	h.RequestPermission(context.Background(), pedido)
	return tela.ultimaPergunta(t)
}

func TestOPedidoDePermissaoVaiTraduzivelParaATela(t *testing.T) {
	payload := dialogoDePermissao(t, pedidoDeExecucao())

	exigirChaveEFallback(t, payload, "title", "description")
	if got := textoDoDialogo(payload, "title"); got != "O agente pede permissão" {
		t.Errorf("title = %q, quer o texto de antes", got)
	}
	if kind, _ := payload["kind"].(string); kind != questionnaire.KindDecision {
		t.Errorf("kind = %q, quer %q", kind, questionnaire.KindDecision)
	}
	if body, _ := payload["body"].(string); body == "" {
		t.Error("body vazio: a ação pedida precisa aparecer no diálogo")
	}
	actions := actionsDe(payload)
	if len(actions) == 0 {
		t.Fatal("sem ações no DecisionDialog")
	}
	for _, action := range actions {
		// Rótulos do agente são Plain (sem chave): traduzir o que vem de fora
		// exibiria texto de outro lugar do app (AEP-0085).
		if action.Label.Fallback == "" && action.Label.String() == "" {
			t.Errorf("ação %q sem rótulo", action.ID)
		}
	}
}

// A classe da ação é enumerável e ganha chave própria, em vez de entrar
// interpolada na frase: o código do protocolo é inglês, e interpolá-lo deixaria
// "o agente quer execute" em qualquer idioma (AEP-0085 D6).
func TestADescricaoDaPermissaoTemUmaChavePorClasseDeAcao(t *testing.T) {
	classes := []string{
		"read", "edit", "delete", "move", "search",
		"execute", "think", "fetch", "switch_mode", "other",
	}
	porChave := make(map[string]string, len(classes))
	for _, classe := range classes {
		pedido := pedidoDeExecucao()
		pedido.ToolCall.Kind = classe

		descricao, _ := dialogoDePermissao(t, pedido)["description"].(questionnaire.Text)
		if descricao.Key == "" {
			t.Fatalf("classe %q: descrição sem chave de tradução", classe)
		}
		if len(descricao.Params) != 0 {
			t.Errorf("classe %q: descrição interpola %v, quer a classe na chave",
				classe, descricao.Params)
		}
		if anterior, repetida := porChave[descricao.Key]; repetida {
			t.Errorf("as classes %q e %q dividem a chave %q: uma delas seria descrita pela outra",
				anterior, classe, descricao.Key)
		}
		porChave[descricao.Key] = classe
	}

	// switch_mode não entra na chave com o underscore do protocolo: o conjunto
	// equivalente do lado da tela (agentPermissions.action.*) nomeia as classes
	// em camelCase, e duas grafias fariam a mesma classe ter dois nomes.
	if classe := porChave["app.questionnaire.agentPermission.descriptionAlways.switchMode"]; classe != "switch_mode" {
		t.Errorf("chaves da descrição = %v, quer switch_mode nomeada como a tela a nomeia", porChave)
	}
}

// Classe que o agente inventou cai na frase genérica, e não numa chave que ele
// escolheu: chave é decisão do app (AEP-0084 D11, AEP-0085 D6).
func TestClasseInventadaPeloAgenteCaiNaChaveGenerica(t *testing.T) {
	pedido := pedidoDeExecucao()
	pedido.ToolCall.Kind = "apagar-tudo"

	descricao, _ := dialogoDePermissao(t, pedido)["description"].(questionnaire.Text)

	if !strings.HasSuffix(descricao.Key, ".other") {
		t.Errorf("chave = %q, quer a da classe genérica", descricao.Key)
	}
	if strings.Contains(descricao.Key, "apagar-tudo") {
		t.Errorf("chave = %q, carrega texto do agente", descricao.Key)
	}
}

// O aviso do "permitir sempre" muda a frase inteira, e não só a acrescenta. As
// duas moram no mesmo campo, então é a chave que diz de qual delas se trata: com
// uma chave só, quem traduz mostraria a descrição sem o aviso, e o alcance da
// autorização — a classe toda, não só o que está na tela — se perderia.
func TestOAvisoDoSempreTrocaAChaveDaDescricao(t *testing.T) {
	comSempre, _ := dialogoDePermissao(t, pedidoDeExecucao())["description"].(questionnaire.Text)

	pedido := pedidoDeExecucao()
	pedido.Options = []acp.PermissionOption{
		{ID: "allow-once", Name: "Permitir uma vez", Kind: "allow_once"},
		{ID: "reject-once", Name: "Negar", Kind: "reject_once"},
	}
	semSempre, _ := dialogoDePermissao(t, pedido)["description"].(questionnaire.Text)

	if comSempre.Key == semSempre.Key {
		t.Errorf("chave = %q nas duas, quer distinguir a descrição que fala do sempre", semSempre.Key)
	}
	if !strings.Contains(comSempre.Fallback, "permitir sempre") {
		t.Errorf("texto pronto = %q, quer dizer o que o sempre abrange", comSempre.Fallback)
	}
	if strings.Contains(semSempre.Fallback, "permitir sempre") {
		t.Errorf("texto pronto = %q, fala de uma opção que o agente não ofereceu", semSempre.Fallback)
	}
}

// O rótulo de opção nos pedidos de permissão é texto que o agente mandou, já
// saneado. Ele nunca pode virar chave de tradução: a chave é decisão do app, e
// uma que viesse de fora exibiria o texto de outro lugar — ou nada, se não
// existisse no locale (AEP-0085).
func TestORotuloDoAgenteNuncaViraChaveDeTraducao(t *testing.T) {
	tela := novaTelaFalsa(func(opcoes []string) string { return opcoes[0] })
	h := handlerCom(tela, acp.TurnOwner{ConversationID: "c", Interactive: true}, true)

	h.RequestPermission(context.Background(), pedidoDeExecucao())

	actions := actionsDe(tela.ultimaPergunta(t))
	if len(actions) == 0 {
		t.Fatal("o diálogo de decisão não trouxe ações")
	}
	for _, action := range actions {
		if action.Label.Key != "" {
			t.Errorf("ação %+v ganhou chave de tradução: o texto é do agente", action)
		}
		if action.Label.Fallback == "" && action.Label.String() == "" {
			t.Error("ação sem texto: a pessoa escolheria um rótulo em branco")
		}
	}
}

// A ação pedida continua indo como Body cru (AEP-0091), e não como texto
// traduzível: é a linha de comando que a pessoa lê antes de autorizar.
func TestAAcaoPedidaContinuaSendoConteudoDoBloco(t *testing.T) {
	tela := novaTelaFalsa(func(opcoes []string) string { return opcoes[0] })
	h := handlerCom(tela, acp.TurnOwner{ConversationID: "c", Interactive: true}, true)

	h.RequestPermission(context.Background(), pedidoDeExecucao())

	payload := tela.ultimaPergunta(t)
	body, _ := payload["body"].(string)
	if body != "rm -rf build" {
		t.Errorf("body = %q, quer a ação que o agente pediu", body)
	}
}
