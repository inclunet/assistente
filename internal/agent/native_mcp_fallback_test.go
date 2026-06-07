package agent

import (
	"context"
	"testing"

	"assistente/internal/llm"
)

// nativeUnsupportedStreamer emula um provider que rejeita MCP nativo: dispara o
// hook de persistência e o Trigger do fallback, depois aborta sem emitir (como o
// provider real faz quando params.NativeMCPFallback != nil).
type nativeUnsupportedStreamer struct {
	calls     int
	hookCalls int
}

func (s *nativeUnsupportedStreamer) StreamChat(_ context.Context, _ []llm.Message, params llm.ChatParams, _ llm.StreamHandler, _ ...llm.ToolDefinition) {
	s.calls++
	if params.OnNativeMCPUnsupported != nil {
		params.OnNativeMCPUnsupported()
		s.hookCalls++
	}
	if params.NativeMCPFallback != nil {
		params.NativeMCPFallback.Trigger()
	}
	// aborta sem emitir — o loop agêntico deve re-tentar em modo adapter.
}

// recordingAdapterStreamer registra as tools recebidas e conclui normalmente.
type recordingAdapterStreamer struct {
	calls    int
	gotTools [][]string
}

func (s *recordingAdapterStreamer) StreamChat(_ context.Context, _ []llm.Message, _ llm.ChatParams, handler llm.StreamHandler, tools ...llm.ToolDefinition) {
	s.calls++
	names := make([]string, 0, len(tools))
	for _, td := range tools {
		names = append(names, td.Function.Name)
	}
	s.gotTools = append(s.gotTools, names)
	handler.OnChunk("ok")
	handler.OnDone("ok", llm.Usage{}, "m")
}

// TestRunAgenticLoop_NativeMCPUnsupportedRetriesSameTurnWithBridgeTools prova que,
// ao detectar não-suporte a MCP nativo, o loop re-tenta o MESMO turno em modo
// adapter COM as bridge tools presentes (não fica sem tools nem espera o próximo
// turno).
func TestRunAgenticLoop_NativeMCPUnsupportedRetriesSameTurnWithBridgeTools(t *testing.T) {
	em := &captureEmitter{}
	repo := &inMemoryMsgRepo{}
	svc := NewService(ServiceConfig{Emitter: em, MsgRepo: repo})

	native := &nativeUnsupportedStreamer{}
	adapter := &recordingAdapterStreamer{}

	// Tools nativas (bridges removidas): só "read_file". Tools adapter (com bridge MCP).
	nativeToolDefs := []llm.ToolDefinition{{Function: llm.FunctionDefinition{Name: "read_file"}}}
	bridgeToolDefs := []llm.ToolDefinition{
		{Function: llm.FunctionDefinition{Name: "mcp_github__list"}},
		{Function: llm.FunctionDefinition{Name: "read_file"}},
	}

	params := llm.ChatParams{
		MaxAgenticIterations: 1,
		NativeMCPFallback: &llm.NativeMCPAdapterFallback{
			Streamer: adapter,
			ToolDefs: bridgeToolDefs,
		},
	}

	svc.RunAgenticLoop(
		context.Background(),
		[]llm.Message{{Role: "user", Content: "hi"}},
		params,
		"c1",
		"t1",
		nativeToolDefs,
		native,
		nil,
		func(convID string, iter int) IterationHandler {
			return NewAgenticStreamHandler(em, convID, iter, nil, "t1")
		},
		nil,
		true,
		2,
	)

	if native.calls != 1 {
		t.Fatalf("streamer nativo: calls=%d, want 1", native.calls)
	}
	if adapter.calls != 1 {
		t.Fatalf("streamer adapter: calls=%d, want 1 (retry no mesmo turno)", adapter.calls)
	}
	if len(adapter.gotTools) != 1 {
		t.Fatalf("adapter gotTools=%#v", adapter.gotTools)
	}
	// O retry adapter DEVE incluir a bridge tool MCP (tools preservadas no turno).
	foundBridge := false
	for _, n := range adapter.gotTools[0] {
		if n == "mcp_github__list" {
			foundBridge = true
		}
	}
	if !foundBridge {
		t.Fatalf("retry adapter não recebeu a bridge tool MCP: %#v", adapter.gotTools[0])
	}

	doneEvents := em.find("chat:done")
	if len(doneEvents) != 1 {
		t.Fatalf("esperava 1 chat:done, got %d", len(doneEvents))
	}
}

// TestNativeMCPAdapterFallback_ConsumeOnce garante que Consume só dispara uma vez.
func TestNativeMCPAdapterFallback_ConsumeOnce(t *testing.T) {
	fb := &llm.NativeMCPAdapterFallback{}
	if fb.Consume() {
		t.Fatal("Consume não deveria retornar true antes de Trigger")
	}
	fb.Trigger()
	if !fb.Consume() {
		t.Fatal("Consume deveria retornar true após Trigger")
	}
	if fb.Consume() {
		t.Fatal("Consume deveria retornar false na segunda chamada (idempotente)")
	}
}
