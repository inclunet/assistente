package agent

import (
	"testing"

	"assistente/internal/core/ports"
	"assistente/internal/llm"
)

// TestSaveAndFinish_DoneEvent_WithLoopStats verifica que chat:done carrega
// os campos IterationCount, ToolCallCount, ToolsUsed e tokens quando loopStats é fornecido.
func TestSaveAndFinish_DoneEvent_WithLoopStats(t *testing.T) {
	emitter := &mockEmitter{}
	repo := &mockMsgRepo{}

	svc := NewService(ServiceConfig{
		Emitter: emitter,
		MsgRepo: repo,
	})

	stats := &LoopStats{
		IterationCount: 3,
		ToolCallCount:  5,
		ToolsUsed: map[string]struct{}{
			"search":    {},
			"read_file": {},
			"write":     {},
		},
		LastUsage: llm.Usage{
			PromptTokens:     1200,
			CompletionTokens: 300,
		},
	}

	svc.SaveAndFinish(42, 1, AgenticResult{
		FullResponse: "Resposta final",
		Model:        "gpt-4",
	}, "", stats)

	evts := emitter.getEvents()
	var done *ports.DoneEvent
	for _, e := range evts {
		if e.name == "chat:done" {
			d := e.data.(ports.DoneEvent)
			done = &d
			break
		}
	}

	if done == nil {
		t.Fatal("chat:done não emitido")
	}

	if done.Reason != "completed" {
		t.Errorf("Reason=%q, esperava 'completed'", done.Reason)
	}
	if done.IterationCount != 3 {
		t.Errorf("IterationCount=%d, esperava 3", done.IterationCount)
	}
	if done.ToolCallCount != 5 {
		t.Errorf("ToolCallCount=%d, esperava 5", done.ToolCallCount)
	}
	if done.PromptTokens != 1200 {
		t.Errorf("PromptTokens=%d, esperava 1200", done.PromptTokens)
	}
	if done.CompletionTokens != 300 {
		t.Errorf("CompletionTokens=%d, esperava 300", done.CompletionTokens)
	}
	if !done.HadToolCalls {
		t.Error("HadToolCalls deveria ser true quando ToolCallCount > 0")
	}
	// ToolsUsed deve estar ordenado
	if len(done.ToolsUsed) != 3 {
		t.Fatalf("ToolsUsed len=%d, esperava 3", len(done.ToolsUsed))
	}
	for i := 1; i < len(done.ToolsUsed); i++ {
		if done.ToolsUsed[i] < done.ToolsUsed[i-1] {
			t.Errorf("ToolsUsed não está ordenado: %v", done.ToolsUsed)
			break
		}
	}
}

// TestSaveAndFinish_DoneEvent_NilLoopStats verifica que chat:done funciona
// sem loopStats (chamada simples sem agentic loop).
func TestSaveAndFinish_DoneEvent_NilLoopStats(t *testing.T) {
	emitter := &mockEmitter{}
	repo := &mockMsgRepo{}

	svc := NewService(ServiceConfig{
		Emitter: emitter,
		MsgRepo: repo,
	})

	svc.SaveAndFinish(1, 0, AgenticResult{
		FullResponse: "Resposta direta",
		Model:        "test",
		Usage: llm.Usage{
			PromptTokens:     500,
			CompletionTokens: 100,
		},
	}, "", nil)

	evts := emitter.getEvents()
	var done *ports.DoneEvent
	for _, e := range evts {
		if e.name == "chat:done" {
			d := e.data.(ports.DoneEvent)
			done = &d
			break
		}
	}

	if done == nil {
		t.Fatal("chat:done não emitido")
	}

	if done.Reason != "completed" {
		t.Errorf("Reason=%q, esperava 'completed'", done.Reason)
	}
	if done.IterationCount != 0 {
		t.Errorf("IterationCount=%d, esperava 0 (nil stats)", done.IterationCount)
	}
	if done.ToolCallCount != 0 {
		t.Errorf("ToolCallCount=%d, esperava 0 (nil stats)", done.ToolCallCount)
	}
	// Tokens devem vir do result.Usage como fallback
	if done.PromptTokens != 500 {
		t.Errorf("PromptTokens=%d, esperava 500 (fallback de result.Usage)", done.PromptTokens)
	}
	if done.CompletionTokens != 100 {
		t.Errorf("CompletionTokens=%d, esperava 100 (fallback de result.Usage)", done.CompletionTokens)
	}
}

// TestSaveAndFinish_DoneEvent_ZeroToolCalls verifica HadToolCalls=false
// quando loopStats tem zero tool calls.
func TestSaveAndFinish_DoneEvent_ZeroToolCalls(t *testing.T) {
	emitter := &mockEmitter{}
	repo := &mockMsgRepo{}

	svc := NewService(ServiceConfig{
		Emitter: emitter,
		MsgRepo: repo,
	})

	stats := &LoopStats{
		IterationCount: 1,
		ToolCallCount:  0,
		ToolsUsed:      map[string]struct{}{},
	}

	svc.SaveAndFinish(1, 1, AgenticResult{
		FullResponse: "Sem tools",
		Model:        "test",
	}, "", stats)

	evts := emitter.getEvents()
	for _, e := range evts {
		if e.name == "chat:done" {
			done := e.data.(ports.DoneEvent)
			if done.HadToolCalls {
				t.Error("HadToolCalls deveria ser false quando ToolCallCount=0")
			}
			if done.ToolCallCount != 0 {
				t.Errorf("ToolCallCount=%d, esperava 0", done.ToolCallCount)
			}
			return
		}
	}
	t.Fatal("chat:done não emitido")
}
