package llm

import (
	"errors"
	"testing"

	"assistente/internal/acp"
)

// providerComModelo monta o provider como a configuração de um agente o monta, e
// devolve o transporte para os testes que precisam contar consultas. O modelo do
// provider fica vazio de propósito: quem escolhe é o perfil, e um padrão aqui
// esconderia o caminho que se quer cobrir.
func providerComModelo(t *testing.T, sessao *agenteFalso) (*ACPChatProvider, *clienteFalso) {
	t.Helper()
	mgr, cliente := servicoComTransporte(t, sessao, acp.Capabilities{})
	provider := NewACPChatProvider(&ProviderConfig{
		ID:         "cursor",
		Name:       "Cursor",
		APIFormat:  APIFormatACP,
		ACPCommand: "cursor-agent",
	}, mgr)
	return provider, cliente
}

func TestGetModelsListaOQueOAgenteOferece(t *testing.T) {
	sessao := &agenteFalso{opcoes: []acp.ConfigOption{opcaoDeModelo("modelo-a", "modelo-a", "modelo-b")}}
	provider, _ := providerComModelo(t, sessao)

	modelos, err := provider.GetModels(t.Context())
	if err != nil {
		t.Fatalf("listar modelos: %v", err)
	}
	if len(modelos) != 2 || modelos[0] != "modelo-a" || modelos[1] != "modelo-b" {
		t.Errorf("modelos = %v", modelos)
	}
}

func TestGetModelsDeAgenteSemEscolhaDeModeloNaoEFalha(t *testing.T) {
	provider, _ := providerComModelo(t, &agenteFalso{})

	modelos, err := provider.GetModels(t.Context())
	if err != nil {
		t.Fatalf("um agente que escolhe o próprio modelo não é erro: %v", err)
	}
	if len(modelos) != 0 {
		t.Errorf("modelos = %v; esperava lista vazia", modelos)
	}
}

func TestRefreshModelsDescartaOQueEstavaGuardado(t *testing.T) {
	sessao := &agenteFalso{opcoes: []acp.ConfigOption{opcaoDeModelo("modelo-a", "modelo-a")}}
	provider, cliente := providerComModelo(t, sessao)
	ctx := t.Context()

	if _, err := provider.GetModels(ctx); err != nil {
		t.Fatalf("primeira listagem: %v", err)
	}

	// O agente passa a oferecer outro modelo — quem instalou um modelo novo nele
	// precisa poder vê-lo aparecer sem reiniciar o app.
	sessao.mu.Lock()
	sessao.opcoes = []acp.ConfigOption{opcaoDeModelo("modelo-a", "modelo-a", "modelo-novo")}
	sessao.mu.Unlock()

	guardados, err := provider.GetModels(ctx)
	if err != nil {
		t.Fatalf("segunda listagem: %v", err)
	}
	if len(guardados) != 1 {
		t.Fatalf("a segunda listagem foi ao agente em vez de usar o que já tinha: %v", guardados)
	}

	frescos, err := provider.RefreshModels(ctx)
	if err != nil {
		t.Fatalf("recarregar modelos: %v", err)
	}
	if len(frescos) != 2 || frescos[1] != "modelo-novo" {
		t.Errorf("modelos depois do refresh = %v", frescos)
	}
	if cliente.consultas != 3 {
		t.Errorf("consultas ao transporte = %d; esperava 3", cliente.consultas)
	}
}

func TestModeloDoPerfilValeParaOTurno(t *testing.T) {
	sessao := &agenteFalso{
		opcoes:  []acp.ConfigOption{opcaoDeModelo("modelo-a", "modelo-a", "modelo-b")},
		updates: []acp.Update{{Kind: acp.UpdateText, Text: "pronto"}},
	}
	provider, _ := providerComModelo(t, sessao)
	handler := &espiao{}

	// É o caminho da pessoa: o modelo escolhido no perfil chega ao turno pelos
	// parâmetros, como o interactor o entrega.
	provider.StreamChat(t.Context(),
		[]Message{{Role: "user", Content: "oi"}},
		ChatParams{ConversationID: "conversa-1", Model: "modelo-b"}, handler)

	if handler.erro != "" {
		t.Fatalf("turno falhou: %s", handler.erro)
	}
	if trocas := sessao.trocasPedidas(); len(trocas) != 1 || trocas[0] != "model=modelo-b" {
		t.Fatalf("o que chegou ao agente = %v", trocas)
	}
	// A troca precisa ter valido antes do turno sair: aplicá-la depois deixaria
	// a escolha da pessoa para o turno seguinte ao seguinte.
	if modelos := sessao.modelosDosTurnos(); len(modelos) != 1 || modelos[0] != "modelo-b" {
		t.Errorf("o turno rodou nos modelos %v; esperava modelo-b", modelos)
	}
	// O modelo relatado é o que atendeu, e é ele que fica salvo na mensagem.
	if handler.modeloFim != "modelo-b" {
		t.Errorf("modelo relatado no fim = %q", handler.modeloFim)
	}
	if len(handler.avisos) != 0 {
		t.Errorf("uma troca bem-sucedida não precisa de aviso: %+v", handler.avisos)
	}
}

func TestModeloJaCorrenteNaoViraPedidoAoAgente(t *testing.T) {
	sessao := &agenteFalso{
		opcoes:  []acp.ConfigOption{opcaoDeModelo("modelo-a", "modelo-a", "modelo-b")},
		updates: []acp.Update{{Kind: acp.UpdateText, Text: "pronto"}},
	}
	provider, _ := providerComModelo(t, sessao)

	provider.StreamChat(t.Context(),
		[]Message{{Role: "user", Content: "oi"}},
		ChatParams{ConversationID: "conversa-1", Model: "modelo-a"}, &espiao{})

	if trocas := sessao.trocasPedidas(); len(trocas) != 0 {
		t.Errorf("pedido redundante foi ao agente: %v", trocas)
	}
}

// O que o agente manda é dado de fora, e pode vir com espaço nas pontas
// (AEP-0084 D11). A lista que a pessoa escolhe sai aparada, então é o valor
// aparado que fica no perfil e volta no turno: comparando cru, o app diria que o
// agente não oferece justamente o modelo que ele acabou de listar, e o turno
// sairia no modelo errado com um aviso mentiroso.
func TestModeloListadoPelaTelaValeMesmoComEspacoNaResposta(t *testing.T) {
	sessao := &agenteFalso{
		opcoes:  []acp.ConfigOption{opcaoDeModelo(" modelo-a ", " modelo-a ", " modelo-b ")},
		updates: []acp.Update{{Kind: acp.UpdateText, Text: "pronto"}},
	}
	provider, _ := providerComModelo(t, sessao)
	ctx := t.Context()

	modelos, err := provider.GetModels(ctx)
	if err != nil {
		t.Fatalf("listar modelos: %v", err)
	}
	if len(modelos) != 2 || modelos[1] != "modelo-b" {
		t.Fatalf("a lista mostrada à pessoa = %v", modelos)
	}

	handler := &espiao{}
	// O perfil guarda exatamente o que a pessoa escolheu na lista.
	provider.StreamChat(ctx,
		[]Message{{Role: "user", Content: "oi"}},
		ChatParams{ConversationID: "conversa-1", Model: modelos[1]}, handler)

	if len(handler.avisos) != 0 {
		t.Fatalf("o modelo estava na lista do agente e não devia render aviso: %+v", handler.avisos)
	}
	if trocas := sessao.trocasPedidas(); len(trocas) != 1 || trocas[0] != "model=modelo-b" {
		t.Fatalf("o que chegou ao agente = %v", trocas)
	}
	if handler.modeloFim != "modelo-b" {
		t.Errorf("modelo relatado no fim = %q", handler.modeloFim)
	}
}

// E o contrário: o modelo pedido já é o corrente, e só o espaço na resposta do
// agente os faz parecer diferentes. Pedir a troca de novo seria gastar uma ida ao
// agente por nada, a cada turno.
func TestModeloCorrenteComEspacoNaoViraPedidoRedundante(t *testing.T) {
	sessao := &agenteFalso{
		opcoes:  []acp.ConfigOption{opcaoDeModelo(" modelo-a ", " modelo-a ", "modelo-b")},
		updates: []acp.Update{{Kind: acp.UpdateText, Text: "pronto"}},
	}
	provider, _ := providerComModelo(t, sessao)
	handler := &espiao{}

	provider.StreamChat(t.Context(),
		[]Message{{Role: "user", Content: "oi"}},
		ChatParams{ConversationID: "conversa-1", Model: "modelo-a"}, handler)

	if trocas := sessao.trocasPedidas(); len(trocas) != 0 {
		t.Errorf("pedido redundante foi ao agente: %v", trocas)
	}
	if len(handler.avisos) != 0 {
		t.Errorf("nada mudou e nada precisava ser avisado: %+v", handler.avisos)
	}
}

func TestAgenteQueNaoOfereceOModeloDoPerfilAvisaSemDerrubarOTurno(t *testing.T) {
	sessao := &agenteFalso{
		opcoes:  []acp.ConfigOption{opcaoDeModelo("modelo-a", "modelo-a", "modelo-b")},
		updates: []acp.Update{{Kind: acp.UpdateText, Text: "pronto"}},
	}
	provider, _ := providerComModelo(t, sessao)
	handler := &espiao{}

	// Acontece ao trocar de provedor sem trocar o modelo no perfil.
	provider.StreamChat(t.Context(),
		[]Message{{Role: "user", Content: "oi"}},
		ChatParams{ConversationID: "conversa-1", Model: "gpt-4o"}, handler)

	if handler.erro != "" {
		t.Fatalf("o turno não deveria falhar por causa do modelo: %s", handler.erro)
	}
	if handler.texto() != "pronto" {
		t.Errorf("a resposta não chegou: %q", handler.texto())
	}
	if trocas := sessao.trocasPedidas(); len(trocas) != 0 {
		t.Errorf("um modelo que o agente não tem não deve ser pedido: %v", trocas)
	}
	if len(handler.avisos) != 1 || handler.avisos[0].Kind != TurnNoticeModelNotOffered {
		t.Fatalf("avisos = %+v", handler.avisos)
	}
	// O aviso diz quem atendeu de verdade, senão a pessoa lê a resposta
	// atribuindo-a a um modelo que não a escreveu.
	if handler.avisos[0].Model != "modelo-a" {
		t.Errorf("o aviso não disse o modelo que atendeu: %+v", handler.avisos[0])
	}
	if handler.modeloFim != "modelo-a" {
		t.Errorf("modelo relatado no fim = %q; esperava o que atendeu", handler.modeloFim)
	}
}

func TestRecusaDoAgenteEmTrocarDeModeloAvisaSemDerrubarOTurno(t *testing.T) {
	sessao := &agenteFalso{
		opcoes:      []acp.ConfigOption{opcaoDeModelo("modelo-a", "modelo-a", "modelo-b")},
		updates:     []acp.Update{{Kind: acp.UpdateText, Text: "pronto"}},
		erroDaTroca: errors.New("modelo indisponível"),
	}
	provider, _ := providerComModelo(t, sessao)
	handler := &espiao{}

	provider.StreamChat(t.Context(),
		[]Message{{Role: "user", Content: "oi"}},
		ChatParams{ConversationID: "conversa-1", Model: "modelo-b"}, handler)

	if handler.erro != "" {
		t.Fatalf("o turno não deveria falhar por causa do modelo: %s", handler.erro)
	}
	if len(handler.avisos) != 1 || handler.avisos[0].Kind != TurnNoticeModelNotApplied {
		t.Fatalf("avisos = %+v", handler.avisos)
	}
	if handler.avisos[0].Model != "modelo-a" {
		t.Errorf("o aviso não disse o modelo que atendeu: %+v", handler.avisos[0])
	}
}

func TestAgenteSemEscolhaDeModeloNaoAvisaNadaACadaTurno(t *testing.T) {
	sessao := &agenteFalso{updates: []acp.Update{{Kind: acp.UpdateText, Text: "pronto"}}}
	provider, _ := providerComModelo(t, sessao)
	handler := &espiao{}

	provider.StreamChat(t.Context(),
		[]Message{{Role: "user", Content: "oi"}},
		ChatParams{ConversationID: "conversa-1", Model: "modelo-b"}, handler)

	if len(handler.avisos) != 0 {
		t.Errorf("um agente que escolhe o próprio modelo não é anomalia do turno: %+v", handler.avisos)
	}
	// Sem escolha exposta, o melhor que se sabe é o que o perfil pediu — e é o
	// mesmo que os outros provedores relatam.
	if handler.modeloFim != "modelo-b" {
		t.Errorf("modelo relatado no fim = %q", handler.modeloFim)
	}
}

func TestTurnoQueNaoSaiuNaoAvisaSobreModelo(t *testing.T) {
	sessao := &agenteFalso{
		opcoes: []acp.ConfigOption{opcaoDeModelo("modelo-a", "modelo-a", "modelo-b")},
		// O agente recusou o pedido sem começar: não há resposta a caminho.
		err: &acp.PromptError{Err: acp.ErrSessionClosed},
	}
	provider, _ := providerComModelo(t, sessao)
	handler := &espiao{}

	provider.StreamChat(t.Context(),
		[]Message{{Role: "user", Content: "oi"}},
		ChatParams{ConversationID: "conversa-1", Model: "gpt-4o"}, handler)

	if handler.erro == "" {
		t.Fatal("o turno deveria falhar")
	}
	// Sem resposta não há sobre o que avisar: falar do modelo de uma resposta
	// que não veio é ruído em cima de um erro.
	if len(handler.avisos) != 0 {
		t.Errorf("avisos num turno que falhou: %+v", handler.avisos)
	}
}
