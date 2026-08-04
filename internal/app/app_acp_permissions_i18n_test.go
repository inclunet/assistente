package app

import (
	"context"
	"testing"

	"assistente/internal/acp"
	"assistente/internal/questionnaire"
)

// O rótulo de opção nos pedidos de permissão é texto que o agente mandou, já
// saneado. Ele nunca pode virar chave de tradução: a chave é decisão do app, e
// uma que viesse de fora exibiria o texto de outro lugar — ou nada, se não
// existisse no locale (AEP-0085).
func TestORotuloDoAgenteNuncaViraChaveDeTraducao(t *testing.T) {
	tela := novaTelaFalsa(func(opcoes []string) string { return opcoes[0] })
	h := handlerCom(tela, acp.TurnOwner{ConversationID: "c", Interactive: true}, true)

	h.RequestPermission(context.Background(), pedidoDeExecucao())

	for _, pergunta := range perguntasDe(tela.ultimaPergunta(t)) {
		for _, opcao := range pergunta.Options {
			if opcao.Key != "" {
				t.Errorf("opção %+v ganhou chave de tradução: o texto é do agente", opcao)
			}
			if opcao.Fallback == "" {
				t.Error("opção sem texto: a pessoa escolheria um rótulo em branco")
			}
		}
	}
}

// A ação pedida continua indo como conteúdo, e não como texto traduzível: é a
// linha de comando que a pessoa lê antes de autorizar.
func TestAAcaoPedidaContinuaSendoConteudoDoBloco(t *testing.T) {
	tela := novaTelaFalsa(func(opcoes []string) string { return opcoes[0] })
	h := handlerCom(tela, acp.TurnOwner{ConversationID: "c", Interactive: true}, true)

	h.RequestPermission(context.Background(), pedidoDeExecucao())

	var bloco questionnaire.Question
	for _, pergunta := range perguntasDe(tela.ultimaPergunta(t)) {
		if pergunta.ID == permissionActionID {
			bloco = pergunta
		}
	}
	if bloco.Content != "rm -rf build" {
		t.Errorf("conteúdo do bloco = %q, quer a ação que o agente pediu", bloco.Content)
	}
	if bloco.Prompt.String() == "" {
		t.Error("o bloco da ação foi para a tela sem rótulo")
	}
}
