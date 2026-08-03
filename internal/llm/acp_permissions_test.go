package llm

import (
	"testing"

	"assistente/internal/acp"
)

// paramsDaTela são os parâmetros de um turno que saiu de uma superfície de
// chat identificada, como o app monta quando a pessoa envia pela tela.
func paramsDaTela(conversationID string) ChatParams {
	return ChatParams{
		ConversationID:    conversationID,
		SurfaceSessionKey: "chat:aba-1",
		SurfaceID:         "chat-principal",
		SurfaceType:       "page",
	}
}

// donoDoTurno monta provider e serviço sobre o mesmo agente falso e devolve o
// que o serviço sabia sobre o turno enquanto o agente o tinha em mãos — que é
// quando ele pergunta por permissão.
func donoDoTurno(t *testing.T, params ChatParams) (acp.TurnOwner, bool) {
	t.Helper()
	sessao := &agenteFalso{updates: []acp.Update{{Kind: acp.UpdateText, Text: "ok"}}}
	agentes := servicoDeAgentes(t, sessao, acp.Capabilities{})
	provider := NewACPChatProvider(&ProviderConfig{
		ID:         "cursor",
		Name:       "Cursor",
		APIFormat:  APIFormatACP,
		ACPCommand: "cursor-agent",
	}, agentes)

	var owner acp.TurnOwner
	var achou bool
	sessao.duranteOTurno = func() { owner, achou = agentes.TurnOwnerOf(sessao.ID()) }

	provider.StreamChat(t.Context(), []Message{{Role: "user", Content: "edite o arquivo"}}, params, &espiao{})
	return owner, achou
}

func TestTurnoDaTelaDizAoAgenteQueTemQuemResponda(t *testing.T) {
	owner, achou := donoDoTurno(t, paramsDaTela("conversa-1"))

	if !achou {
		t.Fatal("o pedido do agente não encontraria o turno em voo")
	}
	if !owner.Interactive {
		t.Error("turno da tela apareceu como se não houvesse quem respondesse: a permissão seria negada na cara da pessoa")
	}
	if owner.ConversationID != "conversa-1" {
		t.Errorf("conversa do turno = %q, quer conversa-1", owner.ConversationID)
	}
}

func TestTurnoSemSuperficieNaoFingeQueTemQuemResponda(t *testing.T) {
	// Canal, job agendado, subagente e CLI chegam sem identidade de
	// superfície: esperar por eles penduraria o agente (AEP-0084 D9).
	owner, achou := donoDoTurno(t, ChatParams{ConversationID: "conversa-1"})

	if !achou {
		t.Fatal("o pedido do agente não encontraria o turno em voo")
	}
	if owner.Interactive {
		t.Error("turno sem superfície apareceu como se tivesse alguém na tela")
	}
}

func TestSuperficieMeioPreenchidaNaoContaComoTela(t *testing.T) {
	// A identidade é a mesma que ports.NewChatSurfaceOrigin exige: faltando
	// uma parte, não há superfície identificada onde abrir diálogo.
	casos := map[string]ChatParams{
		"sem sessão": {ConversationID: "c", SurfaceID: "s", SurfaceType: "page"},
		"sem id":     {ConversationID: "c", SurfaceSessionKey: "k", SurfaceType: "page"},
		"sem tipo":   {ConversationID: "c", SurfaceSessionKey: "k", SurfaceID: "s"},
		"em branco":  {ConversationID: "c", SurfaceSessionKey: " ", SurfaceID: " ", SurfaceType: " "},
	}
	for nome, params := range casos {
		t.Run(nome, func(t *testing.T) {
			if turnHasWatcher(params) {
				t.Error("superfície incompleta passou por tela com gente esperando")
			}
		})
	}
}

func TestTurnoQueAcabouNaoDeixaPedidoSemDono(t *testing.T) {
	sessao := &agenteFalso{updates: []acp.Update{{Kind: acp.UpdateText, Text: "ok"}}}
	agentes := servicoDeAgentes(t, sessao, acp.Capabilities{})
	provider := NewACPChatProvider(&ProviderConfig{ID: "cursor", ACPCommand: "cursor-agent"}, agentes)

	provider.StreamChat(t.Context(), []Message{{Role: "user", Content: "oi"}}, paramsDaTela("conversa-1"), &espiao{})

	// Pedido que chega depois do turno é pedido sem dono: ninguém está diante
	// da tela esperando por ele, e abrir diálogo aí seria pergunta órfã.
	if _, achou := agentes.TurnOwnerOf(sessao.ID()); achou {
		t.Error("o turno terminou e o registro de quem esperava ficou para trás")
	}
}
