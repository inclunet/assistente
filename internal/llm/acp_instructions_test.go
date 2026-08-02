package llm

import (
	"strings"
	"testing"

	"assistente/internal/acp"
)

// sistema monta a mensagem que o pipeline injeta antes do turno, com a mesma
// fronteira que o app marca para cache de prompt.
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

func TestInstrucoesDoPerfilChegamAoAgente(t *testing.T) {
	sessao := &agenteFalso{}
	provider := providerDeAgente(t, sessao)

	provider.StreamChat(t.Context(), []Message{
		sistema("Você é a Ana, assistente de acessibilidade.", "\n<conversation_summary>até aqui falamos de CSS</conversation_summary>"),
		{Role: "user", Content: "e agora?"},
	}, ChatParams{ConversationID: "conversa-1"}, &espiao{})

	turnos := sessao.turnos()
	if len(turnos) != 1 {
		t.Fatalf("esperava um turno, obtive %d", len(turnos))
	}
	texto := textoDoTurno(t, turnos[0])
	// O agente não tem papel de sistema: as instruções vão delimitadas, para
	// não se confundirem com o que a pessoa escreveu.
	if !strings.Contains(texto, acpProfileOpen) || !strings.Contains(texto, "Você é a Ana") {
		t.Errorf("o perfil não chegou ao agente: %q", texto)
	}
	if !strings.Contains(texto, acpContextOpen) || !strings.Contains(texto, "falamos de CSS") {
		t.Errorf("o contexto do turno não chegou ao agente: %q", texto)
	}
	// A mensagem da pessoa é o último bloco: as instruções vêm antes dela.
	ultimo := turnos[0][len(turnos[0])-1]
	if ultimo.Text != "e agora?" {
		t.Errorf("último bloco = %q, esperava a mensagem da pessoa", ultimo.Text)
	}
}

func TestPerfilNaoERepetidoATodoTurnoDaMesmaSessao(t *testing.T) {
	sessao := &agenteFalso{}
	provider := providerDeAgente(t, sessao)
	perfil := "Você é a Ana, assistente de acessibilidade."
	contexto := "\n<conversation_summary>até aqui falamos de CSS</conversation_summary>"

	for _, pergunta := range []string{"e agora?", "e depois?"} {
		provider.StreamChat(t.Context(), []Message{
			sistema(perfil, contexto),
			{Role: "user", Content: pergunta},
		}, ChatParams{ConversationID: "conversa-1"}, &espiao{})
	}

	turnos := sessao.turnos()
	if len(turnos) != 2 {
		t.Fatalf("esperava dois turnos, obtive %d", len(turnos))
	}
	segundo := textoDoTurno(t, turnos[1])
	// O histórico vive na sessão do agente: repetir a persona a cada turno
	// gastaria contexto dele com o que ele acabou de ouvir.
	if strings.Contains(segundo, acpProfileOpen) {
		t.Errorf("o perfil foi repetido no segundo turno: %q", segundo)
	}
	if strings.Contains(segundo, acpContextOpen) {
		t.Errorf("o contexto inalterado foi repetido no segundo turno: %q", segundo)
	}
	if len(turnos[1]) != 1 || turnos[1][0].Text != "e depois?" {
		t.Errorf("segundo turno = %+v, esperava só a mensagem da pessoa", turnos[1])
	}
}

func TestContextoQueMudouVoltaAoAgenteSemArrastarOPerfil(t *testing.T) {
	sessao := &agenteFalso{}
	provider := providerDeAgente(t, sessao)
	perfil := "Você é a Ana, assistente de acessibilidade."

	provider.StreamChat(t.Context(), []Message{
		sistema(perfil, "\n<conversation_summary>falamos de CSS</conversation_summary>"),
		{Role: "user", Content: "e agora?"},
	}, ChatParams{ConversationID: "conversa-1"}, &espiao{})
	provider.StreamChat(t.Context(), []Message{
		sistema(perfil, "\n<conversation_summary>agora falamos de ARIA</conversation_summary>"),
		{Role: "user", Content: "e depois?"},
	}, ChatParams{ConversationID: "conversa-1"}, &espiao{})

	segundo := textoDoTurno(t, sessao.turnos()[1])
	if !strings.Contains(segundo, "falamos de ARIA") {
		t.Errorf("o resumo novo não chegou ao agente: %q", segundo)
	}
	if strings.Contains(segundo, acpProfileOpen) {
		t.Errorf("o perfil inalterado voltou junto: %q", segundo)
	}
}

func TestTrocaDePerfilNoMeioDaConversaReenviaAsInstrucoes(t *testing.T) {
	sessao := &agenteFalso{}
	provider := providerDeAgente(t, sessao)

	provider.StreamChat(t.Context(), []Message{
		sistema("Você é a Ana, assistente de acessibilidade.", ""),
		{Role: "user", Content: "e agora?"},
	}, ChatParams{ConversationID: "conversa-1"}, &espiao{})
	provider.StreamChat(t.Context(), []Message{
		sistema("Você é o Beto, revisor de código.", ""),
		{Role: "user", Content: "e depois?"},
	}, ChatParams{ConversationID: "conversa-1"}, &espiao{})

	segundo := textoDoTurno(t, sessao.turnos()[1])
	// Instruções passaram a ser outras: a sessão só ouviu as antigas.
	if !strings.Contains(segundo, "Você é o Beto") {
		t.Errorf("o perfil novo não chegou ao agente: %q", segundo)
	}
}

func TestTurnoSemMensagemDeSistemaVaiSoComOTextoDaPessoa(t *testing.T) {
	sessao := &agenteFalso{}
	provider := providerDeAgente(t, sessao)

	provider.StreamChat(t.Context(),
		[]Message{{Role: "user", Content: "oi"}},
		ChatParams{ConversationID: "conversa-1"}, &espiao{})

	turnos := sessao.turnos()
	if len(turnos) != 1 || len(turnos[0]) != 1 || turnos[0][0].Text != "oi" {
		t.Errorf("turno = %+v, esperava só a mensagem da pessoa", turnos)
	}
}

func TestSistemaSemFronteiraMarcadaVaiInteiroComoPerfil(t *testing.T) {
	sessao := &agenteFalso{}
	provider := providerDeAgente(t, sessao)

	provider.StreamChat(t.Context(), []Message{
		{Role: "system", Content: "instruções sem marca de prefixo"},
		{Role: "user", Content: "oi"},
	}, ChatParams{ConversationID: "conversa-1"}, &espiao{})

	texto := textoDoTurno(t, sessao.turnos()[0])
	if !strings.Contains(texto, acpProfileOpen) || !strings.Contains(texto, "sem marca de prefixo") {
		t.Errorf("sem a marca, as instruções ainda precisam chegar: %q", texto)
	}
	if strings.Contains(texto, acpContextOpen) {
		t.Errorf("sem fronteira não há contexto separado: %q", texto)
	}
}

func TestSemFronteiraMarcadaAMudancaAindaChegaAoAgente(t *testing.T) {
	sessao := &agenteFalso{}
	provider := providerDeAgente(t, sessao)

	provider.StreamChat(t.Context(), []Message{
		{Role: "system", Content: "persona\n<conversation_summary>falamos de CSS</conversation_summary>"},
		{Role: "user", Content: "e agora?"},
	}, ChatParams{ConversationID: "conversa-1"}, &espiao{})
	provider.StreamChat(t.Context(), []Message{
		{Role: "system", Content: "persona\n<conversation_summary>agora falamos de ARIA</conversation_summary>"},
		{Role: "user", Content: "e depois?"},
	}, ChatParams{ConversationID: "conversa-1"}, &espiao{})

	// Sem a marca não dá para saber onde termina o que se repete, e o conjunto
	// inteiro vira o prefixo. Mudar qualquer parte dele muda o hash: o agente
	// ouve tudo de novo, e não deixa de ouvir o que mudou.
	segundo := textoDoTurno(t, sessao.turnos()[1])
	if !strings.Contains(segundo, "falamos de ARIA") {
		t.Errorf("o resumo novo não chegou ao agente: %q", segundo)
	}
}

func TestTurnoQueFalhouNaoContaAsInstrucoesComoOuvidas(t *testing.T) {
	sessao := &agenteFalso{err: acp.ErrSessionLost}
	provider := providerDeAgente(t, sessao)
	perfil := "Você é a Ana, assistente de acessibilidade."

	provider.StreamChat(t.Context(), []Message{
		sistema(perfil, ""),
		{Role: "user", Content: "e agora?"},
	}, ChatParams{ConversationID: "conversa-1"}, &espiao{})
	sessao.err = nil
	provider.StreamChat(t.Context(), []Message{
		sistema(perfil, ""),
		{Role: "user", Content: "e agora?"},
	}, ChatParams{ConversationID: "conversa-1"}, &espiao{})

	segundo := textoDoTurno(t, sessao.turnos()[1])
	// O turno morreu no transporte: dar o perfil por entregue deixaria o agente
	// sem ele pelo resto da conversa.
	if !strings.Contains(segundo, acpProfileOpen) {
		t.Errorf("o perfil não foi reenviado depois da falha: %q", segundo)
	}
}

func TestSeparacaoDaMensagemDeSistema(t *testing.T) {
	casos := []struct {
		nome      string
		mensagens []Message
		estavel   string
		dinamico  string
	}{
		{
			nome:      "fronteira marcada",
			mensagens: []Message{sistema("persona", "\ncontexto")},
			estavel:   "persona",
			dinamico:  "contexto",
		},
		{
			nome:      "sem mensagem de sistema",
			mensagens: []Message{{Role: "user", Content: "oi"}},
		},
		{
			// Marca maior que o texto não pode virar índice fora do intervalo.
			nome:      "marca maior que a mensagem",
			mensagens: []Message{{Role: "system", Content: "curta", SystemCacheControlPrefixLen: 500}},
			estavel:   "curta",
		},
		{
			nome:      "só contexto",
			mensagens: []Message{{Role: "system", Content: "contexto", SystemCacheControlPrefixLen: 0}},
			estavel:   "contexto",
		},
	}
	for _, caso := range casos {
		t.Run(caso.nome, func(t *testing.T) {
			estavel, dinamico := splitSystemPrompt(caso.mensagens)
			if estavel != caso.estavel || dinamico != caso.dinamico {
				t.Errorf("estável=%q dinâmico=%q, quer %q e %q", estavel, dinamico, caso.estavel, caso.dinamico)
			}
		})
	}
}
