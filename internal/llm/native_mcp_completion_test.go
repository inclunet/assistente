package llm

import "testing"

func newPendingMCPCall(id, name, server, args string, completed bool) *pendingMCPCall {
	mc := &pendingMCPCall{ID: id, Name: name, ServerLabel: server, Completed: completed}
	mc.Args.WriteString(args)
	return mc
}

// TestFlushPendingCompletedMCPCalls_EmitsCompletedWithoutOutputItemDone cobre o
// caso de regressão: um endpoint/proxy emite response.mcp_call.completed (marca
// Completed=true) mas NÃO emite response.output_item.done para o item mcp_call.
// O flush pós-stream deve emitir o evento de conclusão (com os argumentos
// acumulados), garantindo que a tool nativa seja persistida e apareça no histórico.
func TestFlushPendingCompletedMCPCalls_EmitsCompletedWithoutOutputItemDone(t *testing.T) {
	active := map[string]*pendingMCPCall{
		"mcp_1": newPendingMCPCall("mcp_1", "jira_search", "Atlassian", `{"q":"bug"}`, true),
	}
	handler := &mcpTrackingHandler{}

	emitted := flushPendingCompletedMCPCalls(active, handler)

	if !emitted {
		t.Fatal("esperava emitted=true para item marcado como Completed")
	}
	if len(handler.events) != 1 {
		t.Fatalf("esperado 1 evento, obtido %d", len(handler.events))
	}
	ev := handler.events[0]
	if ev.ID != "mcp_1" || ev.Name != "jira_search" || ev.ServerLabel != "Atlassian" {
		t.Errorf("evento incorreto: %+v", ev)
	}
	if !ev.IsCompleted {
		t.Error("evento deveria ter IsCompleted=true")
	}
	if ev.Arguments != `{"q":"bug"}` {
		t.Errorf("Arguments = %q, esperado os argumentos acumulados", ev.Arguments)
	}
	// O item concluído deve ser removido para evitar reemissão.
	if _, ok := active["mcp_1"]; ok {
		t.Error("item concluído deveria ter sido removido do mapa")
	}
}

// TestFlushPendingCompletedMCPCalls_SkipsItemsNotCompleted garante que itens que
// ainda não receberam o sinal de conclusão NÃO são emitidos (continuam pendentes).
func TestFlushPendingCompletedMCPCalls_SkipsItemsNotCompleted(t *testing.T) {
	active := map[string]*pendingMCPCall{
		"mcp_running": newPendingMCPCall("mcp_running", "search", "Slack", "", false),
	}
	handler := &mcpTrackingHandler{}

	emitted := flushPendingCompletedMCPCalls(active, handler)

	if emitted {
		t.Error("não deveria emitir para item sem Completed")
	}
	if len(handler.events) != 0 {
		t.Fatalf("esperado 0 eventos, obtido %d", len(handler.events))
	}
	if _, ok := active["mcp_running"]; !ok {
		t.Error("item pendente não deveria ser removido do mapa")
	}
}

// TestFlushPendingCompletedMCPCalls_NoDoubleEmission garante que o caminho normal
// (output_item.done já tratou o item e o removeu do mapa) não gera dupla emissão:
// um segundo flush sobre um mapa já esvaziado não emite nada.
func TestFlushPendingCompletedMCPCalls_NoDoubleEmission(t *testing.T) {
	active := map[string]*pendingMCPCall{
		"mcp_1": newPendingMCPCall("mcp_1", "jira_search", "Atlassian", "", true),
	}
	handler := &mcpTrackingHandler{}

	if !flushPendingCompletedMCPCalls(active, handler) {
		t.Fatal("primeiro flush deveria emitir")
	}
	if flushPendingCompletedMCPCalls(active, handler) {
		t.Error("segundo flush não deveria emitir (item já consumido)")
	}
	if len(handler.events) != 1 {
		t.Fatalf("esperado 1 evento no total, obtido %d", len(handler.events))
	}
}
