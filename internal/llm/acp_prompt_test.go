package llm

import (
	"strings"
	"testing"

	"assistente/internal/acp"
)

// sistema monta a mensagem que o pipeline injeta antes do turno, com a mesma
// fronteira que o app marca para cache de prompt. Um turno de agente não
// deveria receber nenhuma delas — é justamente o que os testes daqui vigiam.
func sistema(estavel, dinamico string) Message {
	return Message{
		Role:                        "system",
		Content:                     estavel + dinamico,
		SystemCacheControlPrefixLen: len(estavel),
	}
}

func textoDoTurno(t *testing.T, turno []acp.Content) string {
	t.Helper()
	var partes []string
	for _, bloco := range turno {
		partes = append(partes, bloco.Text)
	}
	return strings.Join(partes, "\n")
}

// O pipeline não monta prompt de sistema para um turno de agente, mas o
// provider é chamado de outros lugares e o contrato precisa valer aqui também:
// o que o app tem a dizer não vira bloco no turno (AEP-0084 D4, revisto na
// Fase 8).
func TestOTurnoDoAgenteLevaSoAMensagemDaPessoa(t *testing.T) {
	sessao := &agenteFalso{}
	provider := providerDeAgente(t, sessao)

	provider.StreamChat(t.Context(), []Message{
		sistema("Você é a Ana, assistente de acessibilidade.",
			"\n<conversation_summary>até aqui falamos de CSS</conversation_summary>"),
		{Role: "user", Content: "e agora?"},
	}, ChatParams{ConversationID: "conversa-1"}, &espiao{})

	turnos := sessao.turnos()
	if len(turnos) != 1 {
		t.Fatalf("esperava um turno, obtive %d", len(turnos))
	}
	if len(turnos[0]) != 1 || turnos[0][0].Text != "e agora?" {
		t.Fatalf("turno = %+v, esperava só a mensagem da pessoa", turnos[0])
	}
	texto := textoDoTurno(t, turnos[0])
	if strings.Contains(texto, "Você é a Ana") {
		t.Errorf("a persona do perfil foi parar no turno do agente: %q", texto)
	}
	if strings.Contains(texto, "falamos de CSS") {
		t.Errorf("o contexto do app foi parar no turno do agente: %q", texto)
	}
}

// Sem a fronteira de cache, o caminho antigo mandava a mensagem de sistema
// inteira como perfil. Agora ela não vai de jeito nenhum, marcada ou não.
func TestSistemaSemFronteiraMarcadaTambemFicaDeFora(t *testing.T) {
	sessao := &agenteFalso{}
	provider := providerDeAgente(t, sessao)

	provider.StreamChat(t.Context(), []Message{
		{Role: "system", Content: "instruções sem marca de prefixo"},
		{Role: "user", Content: "oi"},
	}, ChatParams{ConversationID: "conversa-1"}, &espiao{})

	texto := textoDoTurno(t, sessao.turnos()[0])
	if texto != "oi" {
		t.Errorf("turno = %q, esperava só a mensagem da pessoa", texto)
	}
}

// A conversa segue com o agente turno após turno, e nenhum deles carrega o que
// o app sabe — nem o primeiro, que antes levava o prefixo estável.
func TestNenhumTurnoDaSessaoCarregaOQueOAppSabe(t *testing.T) {
	sessao := &agenteFalso{}
	provider := providerDeAgente(t, sessao)
	perfil := "Você é a Ana, assistente de acessibilidade."

	for _, pergunta := range []string{"e agora?", "e depois?"} {
		provider.StreamChat(t.Context(), []Message{
			sistema(perfil, "\n<user_memory>a pessoa usa NVDA</user_memory>"),
			{Role: "user", Content: pergunta},
		}, ChatParams{ConversationID: "conversa-1"}, &espiao{})
	}

	turnos := sessao.turnos()
	if len(turnos) != 2 {
		t.Fatalf("esperava dois turnos, obtive %d", len(turnos))
	}
	for i, turno := range turnos {
		texto := textoDoTurno(t, turno)
		if strings.Contains(texto, "Você é a Ana") || strings.Contains(texto, "usa NVDA") {
			t.Errorf("turno %d levou o que o app sabe: %q", i+1, texto)
		}
	}
}
