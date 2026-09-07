package llm

import (
	"testing"

	"assistente/internal/acp"
)

// O agente batiza a sessão dele no meio do turno, e esse nome descreve o que foi
// pedido melhor do que o recorte da primeira mensagem que o app usa como rótulo
// (AEP-0084 D8). Ele chega ao handler, que é quem sabe renomear a conversa.
func TestTituloDaSessaoChegaAoHandler(t *testing.T) {
	sessao := &agenteFalso{updates: []acp.Update{
		{Kind: acp.UpdateTitle, Title: "Corrigir o teste de anexos"},
		{Kind: acp.UpdateText, Text: "pronto"},
	}}
	provider := providerDeAgente(t, sessao)
	handler := &espiao{}

	provider.StreamChat(t.Context(),
		[]Message{{Role: "user", Content: "arruma o teste"}},
		ChatParams{ConversationID: "conversa-1"}, handler)

	if handler.erro != "" {
		t.Fatalf("turno falhou: %s", handler.erro)
	}
	if len(handler.titulos) != 1 || handler.titulos[0] != "Corrigir o teste de anexos" {
		t.Fatalf("títulos entregues = %v", handler.titulos)
	}
}

// O agente refina o nome conforme entende o pedido, e quem manda é o último: o
// primeiro palpite dele descreve menos do que ele já sabe no fim do turno.
func TestUltimoTituloDoTurnoEhOQueVale(t *testing.T) {
	sessao := &agenteFalso{updates: []acp.Update{
		{Kind: acp.UpdateTitle, Title: "Investigar teste"},
		{Kind: acp.UpdateText, Text: "achei"},
		{Kind: acp.UpdateTitle, Title: "Corrigir o teste de anexos"},
	}}
	provider := providerDeAgente(t, sessao)
	handler := &espiao{}

	provider.StreamChat(t.Context(),
		[]Message{{Role: "user", Content: "arruma o teste"}},
		ChatParams{ConversationID: "conversa-1"}, handler)

	if len(handler.titulos) != 1 || handler.titulos[0] != "Corrigir o teste de anexos" {
		t.Fatalf("títulos entregues = %v", handler.titulos)
	}
}

// O título é entregue depois que o turno termina, e não no meio dele: este sink
// roda na goroutine que entrega as atualizações do protocolo, e renomear é
// escrita no banco — segurá-la aqui deixaria o agente parado esperando o disco.
func TestTituloEhEntregueDepoisDoTextoDoTurno(t *testing.T) {
	sessao := &agenteFalso{updates: []acp.Update{
		{Kind: acp.UpdateTitle, Title: "Corrigir o teste"},
		{Kind: acp.UpdateText, Text: "pronto"},
	}}
	provider := providerDeAgente(t, sessao)
	handler := &espiao{}

	provider.StreamChat(t.Context(),
		[]Message{{Role: "user", Content: "arruma o teste"}},
		ChatParams{ConversationID: "conversa-1"}, handler)

	posTitulo, posChunk := -1, -1
	for i, evento := range handler.ordem {
		switch evento {
		case "title":
			posTitulo = i
		case "chunk":
			if posChunk < 0 {
				posChunk = i
			}
		}
	}
	if posTitulo < 0 || posChunk < 0 {
		t.Fatalf("faltou evento na ordem: %v", handler.ordem)
	}
	if posTitulo < posChunk {
		t.Fatalf("o título foi entregue no meio do turno: %v", handler.ordem)
	}
}

// Título só de espaço, ou sem título nenhum, não renomeia coisa alguma: a aba
// ficaria sem rótulo e o leitor de telas sem o que anunciar.
func TestTituloEmBrancoNaoEhEntregue(t *testing.T) {
	sessao := &agenteFalso{updates: []acp.Update{
		{Kind: acp.UpdateTitle, Title: "   "},
		{Kind: acp.UpdateText, Text: "pronto"},
	}}
	provider := providerDeAgente(t, sessao)
	handler := &espiao{}

	provider.StreamChat(t.Context(),
		[]Message{{Role: "user", Content: "arruma o teste"}},
		ChatParams{ConversationID: "conversa-1"}, handler)

	if len(handler.titulos) != 0 {
		t.Fatalf("entregou título em branco: %q", handler.titulos)
	}
}

// O título vem do agente, que manda o que quiser: escape de terminal e marca
// invisível viram rótulo de aba e texto de leitor de telas, e precisam sair
// antes (AEP-0084 D11).
func TestTituloDoAgenteEhSaneadoAntesDeVirarRotulo(t *testing.T) {
	sessao := &agenteFalso{updates: []acp.Update{
		{Kind: acp.UpdateTitle, Title: "\x1b[31mCorrigir\x1b[0m o\nteste"},
		{Kind: acp.UpdateText, Text: "pronto"},
	}}
	provider := providerDeAgente(t, sessao)
	handler := &espiao{}

	provider.StreamChat(t.Context(),
		[]Message{{Role: "user", Content: "arruma o teste"}},
		ChatParams{ConversationID: "conversa-1"}, handler)

	if len(handler.titulos) != 1 || handler.titulos[0] != "Corrigir o teste" {
		t.Fatalf("título entregue = %q", handler.titulos)
	}
}

// Handler que não sabe receber título segue recebendo o turno inteiro: renomear
// é conveniência, e nenhuma resposta pode depender dela.
func TestHandlerSemTituloRecebeOTurnoNormalmente(t *testing.T) {
	sessao := &agenteFalso{updates: []acp.Update{
		{Kind: acp.UpdateTitle, Title: "Corrigir o teste"},
		{Kind: acp.UpdateText, Text: "pronto"},
	}}
	provider := providerDeAgente(t, sessao)
	handler := &espiaoSurdo{}

	provider.StreamChat(t.Context(),
		[]Message{{Role: "user", Content: "arruma o teste"}},
		ChatParams{ConversationID: "conversa-1"}, handler)

	if !handler.pronto {
		t.Fatal("o turno não terminou para o handler que ignora título")
	}
}
