package agent

import (
	"assistente/internal/llm"
	"context"
	"testing"
)

type nativeMismatchStreamer struct {
	calls int
	fail  bool
}

func (s *nativeMismatchStreamer) StreamChat(_ context.Context, _ []llm.Message, p llm.ChatParams, h llm.StreamHandler, _ ...llm.ToolDefinition) {
	s.calls++
	p.NativeMCPFallback.TriggerAfterResponse()
	h.OnChunk("conteúdo já recebido")
	h.OnMCPToolEvent(llm.MCPToolEvent{ID: "remote1", Name: "tool_catalog", Output: "resultado remoto", Error: "contrato incompatível", IsCompleted: true})
	if s.fail {
		if barrier, ok := h.(interface{ MarkErrorNotRetryable() }); ok {
			barrier.MarkErrorNotRetryable()
		}
		h.OnError("interrupção após execução")
		return
	}
	h.OnToolCalls([]llm.ToolCall{{ID: "local1", Function: llm.FunctionCall{Name: "tool_catalog", Arguments: `{}`}}}, "conteúdo já recebido", llm.Usage{}, "model")
}

func TestNativeMCPMismatchSwitchesWithoutDiscardingResult(t *testing.T) {
	for _, withResolver := range []bool{false, true} {
		t.Run(map[bool]string{false: "sem resolver", true: "com resolver"}[withResolver], func(t *testing.T) {
			native := &nativeMismatchStreamer{}
			adapter := &recordingAdapterStreamer{}
			fb := &llm.NativeMCPAdapterFallback{Streamer: adapter, ToolDefs: []llm.ToolDefinition{{Function: llm.FunctionDefinition{Name: "mcp_remote__query"}}}}
			resolved := 0
			if withResolver {
				fb.ResolveToolDefs = func(active []llm.ToolDefinition, names []string) []llm.ToolDefinition {
					resolved++
					if len(active) != 1 || active[0].Function.Name != "loaded_later" {
						t.Fatal("tools dinâmicas perdidas")
					}
					if len(names) != 1 || names[0] != "mcp_remote__query" {
						t.Fatal("bridge não restaurada")
					}
					return append(active, fb.ToolDefs...)
				}
			}
			em := &captureEmitter{}
			r := &agenticLoopRunner{svc: NewService(ServiceConfig{Emitter: em, MsgRepo: &inMemoryMsgRepo{}}), params: llm.ChatParams{NativeMCPFallback: fb}, activeStreamer: native, activeToolDefs: []llm.ToolDefinition{{Function: llm.FunctionDefinition{Name: "loaded_later"}}}, newHandler: func(c string, i int) IterationHandler { return NewAgenticStreamHandler(em, c, i, nil, "t") }}
			result, errText, stop := r.streamIteration(context.Background(), 0)
			if stop || errText != "" || native.calls != 1 || adapter.calls != 0 {
				t.Fatalf("replay inesperado: native=%d adapter=%d err=%s", native.calls, adapter.calls, errText)
			}
			if result.FullResponse != "conteúdo já recebido" || len(result.NativeMCPEvents) != 1 || result.NativeMCPEvents[0].Output != "resultado remoto" || len(result.ToolCalls) != 1 || result.ToolCalls[0].ID != "local1" {
				t.Fatalf("resultado descartado: %+v", result)
			}
			if len(r.activeToolDefs) != 2 || (withResolver && resolved != 1) {
				t.Fatalf("tools/resolver: %+v/%d", r.activeToolDefs, resolved)
			}
			r.streamIteration(context.Background(), 1)
			if native.calls != 1 || adapter.calls != 1 {
				t.Fatalf("próxima chamada não usou adapter: %d/%d", native.calls, adapter.calls)
			}
		})
	}
}

func TestNativeMCPFallbackAfterResponseTakesPrecedence(t *testing.T) {
	fb := &llm.NativeMCPAdapterFallback{}
	fb.TriggerAfterResponse()
	fb.Trigger()
	if fb.Consume() || !fb.ConsumeAfterResponse() || fb.ConsumeAfterResponse() {
		t.Fatal("troca posterior deve impedir replay e ser consumida uma vez")
	}
}

func TestNativeMCPMismatchDoesNotRetryInterruptedResponse(t *testing.T) {
	native := &nativeMismatchStreamer{fail: true}
	adapter := &recordingAdapterStreamer{}
	em := &captureEmitter{}
	r := &agenticLoopRunner{svc: NewService(ServiceConfig{Emitter: em, MsgRepo: &inMemoryMsgRepo{}}), params: llm.ChatParams{NativeMCPFallback: &llm.NativeMCPAdapterFallback{Streamer: adapter}}, activeStreamer: native, streamingRecoveryEnabled: true, maxRecoveryAttempts: 3, newHandler: func(c string, i int) IterationHandler { return NewAgenticStreamHandler(em, c, i, nil, "t") }}
	result, errText, _ := r.streamIteration(context.Background(), 0)
	if native.calls != 1 || adapter.calls != 0 || errText != "interrupção após execução" || len(result.NativeMCPEvents) != 1 {
		t.Fatalf("erro ou efeitos descartados/repetidos: %d/%d %+v %q", native.calls, adapter.calls, result, errText)
	}
}
