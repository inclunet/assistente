package agent

import (
	"context"
	"testing"
	"time"

	"assistente/internal/core/ports"
	"assistente/internal/llm"
)

func waitForStreamEvents(t *testing.T, emitter *captureEmitter, count int) []captured {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if events := emitter.find("chat:stream"); len(events) >= count {
			return events
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("chat:stream não chegou; esperava %d evento(s)", count)
	return nil
}

func capturedNames(emitter *captureEmitter) []string {
	emitter.mu.Lock()
	defer emitter.mu.Unlock()
	names := make([]string, len(emitter.events))
	for i, event := range emitter.events {
		names[i] = event.name
	}
	return names
}

func TestBaseStreamHandlerCoalesceDeltasInOrderWithUnicode(t *testing.T) {
	emitter := &captureEmitter{}
	handler := BaseStreamHandler{
		Emitter:            emitter,
		ConversationID:     "conversation-1",
		TurnID:             "turn-1",
		AssistantMessageID: "assistant-1",
	}

	handler.OnChunk("Olá ")
	handler.OnChunk("世界")
	handler.OnChunk(" 👩🏽‍💻")

	events := waitForStreamEvents(t, emitter, 1)
	time.Sleep(StreamCoalesceInterval * 2)
	if got := len(emitter.find("chat:stream")); got != 1 {
		t.Fatalf("coalescing emitiu %d eventos, esperava 1", got)
	}
	stream := events[0].data.(ports.StreamEvent)
	if stream.Delta != "Olá 世界 👩🏽‍💻" {
		t.Fatalf("delta=%q", stream.Delta)
	}
	if !stream.Reset || stream.Sequence != 0 {
		t.Fatalf("primeiro lote precisa iniciar a tentativa: %+v", stream)
	}
	if stream.ConversationId != "conversation-1" || stream.TurnID != "turn-1" || stream.MessageID != "assistant-1" {
		t.Fatalf("correlação perdida: %+v", stream)
	}
}

func TestBaseStreamHandlerFlushPreservesSequenceAndRetryReset(t *testing.T) {
	emitter := &captureEmitter{}
	newHandler := func() *BaseStreamHandler {
		return &BaseStreamHandler{
			Emitter:            emitter,
			ConversationID:     "conversation-1",
			TurnID:             "turn-1",
			AssistantMessageID: "assistant-1",
		}
	}

	firstAttempt := newHandler()
	firstAttempt.OnChunk("primeiro")
	firstAttempt.FlushStream()
	firstAttempt.OnChunk(" segundo")
	firstAttempt.FlushStream()

	secondAttempt := newHandler()
	secondAttempt.OnChunk("recuperado")
	secondAttempt.FlushStream()

	events := emitter.find("chat:stream")
	if len(events) != 3 {
		t.Fatalf("eventos=%d, esperava 3", len(events))
	}
	first := events[0].data.(ports.StreamEvent)
	second := events[1].data.(ports.StreamEvent)
	retry := events[2].data.(ports.StreamEvent)
	if first.Delta != "primeiro" || second.Delta != " segundo" {
		t.Fatalf("buffers de deltas foram alterados após reuso: %+v %+v", first, second)
	}
	if first.Sequence != 0 || !first.Reset || second.Sequence != 1 || second.Reset {
		t.Fatalf("sequência da primeira tentativa inválida: %+v %+v", first, second)
	}
	if retry.Sequence != 0 || !retry.Reset || retry.Delta != "recuperado" {
		t.Fatalf("retry não reiniciou deterministicamente: %+v", retry)
	}
}

func TestBaseStreamHandlerResetStreamAttemptDescartaReasoningAnterior(t *testing.T) {
	emitter := &captureEmitter{}
	handler := &BaseStreamHandler{
		Emitter:            emitter,
		ConversationID:     "conversation-1",
		TurnID:             "turn-1",
		AssistantMessageID: "assistant-1",
	}

	handler.OnThinking("tentativa descartada")
	handler.ResetStreamAttempt()
	handler.OnThinking("tentativa válida")
	handler.OnThinkingDone("tentativa válida")

	_, reasoning := handler.Finalize()
	if reasoning != "tentativa válida" {
		t.Fatalf("reasoning=%q; tentativa descartada vazou para o resultado", reasoning)
	}
	thinkingEvents := emitter.find("chat:thinking")
	if len(thinkingEvents) < 4 {
		t.Fatalf("eventos de thinking=%d, esperava início/fim das duas tentativas", len(thinkingEvents))
	}
	var resetDone *ports.ThinkingEvent
	for _, captured := range thinkingEvents {
		event := captured.data.(ports.ThinkingEvent)
		if event.Done && event.Content == "tentativa descartada" {
			resetDone = &event
			break
		}
	}
	if resetDone == nil {
		t.Fatal("reset não encerrou o thinking descartado")
	}
}

func TestAgenticStreamHandlerOnErrorPreservaParcialEDiagnostico(t *testing.T) {
	handler := NewAgenticStreamHandler(&captureEmitter{}, "conversation-1", 0, nil, "turn-1")
	handler.OnChunk("parcial")
	handler.OnThinking("raciocínio")
	handler.OnFinishReason(llm.FinishInfo{
		Provider:      "provider-1",
		Model:         "model-1",
		OutputLimit:   4096,
		ResponseBytes: 7,
	})
	handler.OnError("streaming_interrupted")

	result := handler.Result()
	if result.FullResponse != "parcial" || result.Reasoning != "raciocínio" {
		t.Fatalf("parcial perdido no erro: %+v", result)
	}
	if result.Finish.Provider != "provider-1" || result.Finish.Model != "model-1" ||
		result.Finish.OutputLimit != 4096 || result.Finish.ResponseBytes != 7 {
		t.Fatalf("diagnóstico perdido no erro: %+v", result.Finish)
	}
}

func TestSimpleStreamHandlerFlushesBeforeToolAndError(t *testing.T) {
	emitter := &captureEmitter{}
	service := NewService(ServiceConfig{Emitter: emitter, MsgRepo: &inMemoryMsgRepo{}})
	handler, err := service.NewSimpleStreamHandler(context.Background(), "conversation-1", "turn-1", "", nil)
	if err != nil {
		t.Fatalf("NewSimpleStreamHandler: %v", err)
	}

	handler.OnChunk("antes da tool")
	handler.OnAgentToolEvent(llm.AgentToolEvent{
		ID:     "call-1",
		Kind:   "execute",
		Status: llm.AgentToolRunning,
	})
	handler.OnChunk(" e antes do erro")
	handler.OnError("falhou")

	names := capturedNames(emitter)
	want := []string{"chat:stream", "chat:tool_start", "chat:stream", "chat:tool_end", "chat:stream"}
	if len(names) != len(want) {
		t.Fatalf("ordem=%v, esperava %v", names, want)
	}
	for i := range want {
		if names[i] != want[i] {
			t.Fatalf("ordem=%v, esperava %v", names, want)
		}
	}
	terminal := emitter.find("chat:stream")[2].data.(ports.StreamEvent)
	if !terminal.Done || terminal.Error != "falhou" || terminal.Delta != "" {
		t.Fatalf("terminal de erro inválido: %+v", terminal)
	}
}

func TestSimpleStreamHandlerFlushesBeforeSegmentAndResetsNextDelta(t *testing.T) {
	emitter := &captureEmitter{}
	service := NewService(ServiceConfig{Emitter: emitter, MsgRepo: &inMemoryMsgRepo{}})
	handler, err := service.NewSimpleStreamHandler(context.Background(), "conversation-1", "turn-1", "", nil)
	if err != nil {
		t.Fatalf("NewSimpleStreamHandler: %v", err)
	}

	handler.OnChunk("segmento um")
	handler.OnSegmentDone()
	handler.OnChunk("segmento dois")
	handler.FlushStream()

	names := capturedNames(emitter)
	want := []string{"chat:stream", "chat:segment_done", "chat:stream"}
	if len(names) != len(want) {
		t.Fatalf("ordem=%v, esperava %v", names, want)
	}
	for i := range want {
		if names[i] != want[i] {
			t.Fatalf("ordem=%v, esperava %v", names, want)
		}
	}
	streams := emitter.find("chat:stream")
	next := streams[1].data.(ports.StreamEvent)
	if !next.Reset || next.Sequence != 0 || next.Delta != "segmento dois" {
		t.Fatalf("novo segmento não reiniciou o acumulador visual: %+v", next)
	}
}

func TestSimpleStreamHandlerFlushesBeforeDoneAndOutputLimit(t *testing.T) {
	emitter := &captureEmitter{}
	service := NewService(ServiceConfig{Emitter: emitter, MsgRepo: &inMemoryMsgRepo{}})
	handler, err := service.NewSimpleStreamHandler(context.Background(), "conversation-1", "turn-1", "", nil)
	if err != nil {
		t.Fatalf("NewSimpleStreamHandler: %v", err)
	}

	handler.OnChunk("parcial")
	handler.OnFinishReason(llm.FinishInfo{Reason: llm.FinishReasonMaxTokens})
	handler.OnDone("parcial", llm.Usage{}, "modelo")

	names := capturedNames(emitter)
	if len(names) < 3 || names[0] != "chat:stream" || names[1] != "chat:stream" || names[len(names)-1] != "chat:done" {
		t.Fatalf("flush/terminais fora de ordem: %v", names)
	}
	streams := emitter.find("chat:stream")
	if delta := streams[0].data.(ports.StreamEvent); delta.Delta != "parcial" || delta.Done {
		t.Fatalf("delta inválido: %+v", delta)
	}
	if terminal := streams[1].data.(ports.StreamEvent); !terminal.Done || terminal.Delta != "" {
		t.Fatalf("terminal inválido: %+v", terminal)
	}
	done := emitter.find("chat:done")[0].data.(ports.DoneEvent)
	if done.Reason != "output_limit" || done.TurnID != "turn-1" {
		t.Fatalf("chat:done inválido: %+v", done)
	}
}
