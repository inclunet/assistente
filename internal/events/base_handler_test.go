package events

import (
	"testing"

	"assistente/internal/core/ports"
)

type baseHandlerCapture struct {
	name string
	data any
}

func (c *baseHandlerCapture) Emit(name string, data any) {
	c.name = name
	c.data = data
}

type retainingBaseHandlerCapture struct {
	events []any
}

func (c *retainingBaseHandlerCapture) Emit(_ string, data any) {
	c.events = append(c.events, data)
}

func TestBaseStreamHandlerPreservaCorrelacaoCompleta(t *testing.T) {
	emitter := &baseHandlerCapture{}
	origin := &ports.ChatSurfaceOrigin{
		SessionKey:     "session-1",
		ConversationID: "conversation-1",
		SurfaceID:      "surface-1",
		SurfaceType:    "page",
	}
	handler := &BaseStreamHandler{
		Emitter:            emitter,
		ConversationID:     "conversation-1",
		TurnID:             "turn-1",
		AssistantMessageID: "assistant-1",
		SurfaceOrigin:      origin,
	}

	handler.OnChunk("delta")

	event, ok := emitter.data.(ports.StreamEvent)
	if emitter.name != "chat:stream" || !ok {
		t.Fatalf("evento inesperado: %q %T", emitter.name, emitter.data)
	}
	if event.ConversationId != "conversation-1" || event.TurnID != "turn-1" || event.MessageID != "assistant-1" {
		t.Fatalf("correlação incompleta: %+v", event)
	}
	if event.SurfaceOrigin != origin {
		t.Fatalf("surfaceOrigin não preservada: %+v", event.SurfaceOrigin)
	}
}

func TestBaseStreamHandlerPreservaDeltasEmitidosAposReusoDoBuffer(t *testing.T) {
	emitter := &retainingBaseHandlerCapture{}
	handler := &BaseStreamHandler{
		Emitter:        emitter,
		ConversationID: "conversation-1",
		TurnID:         "turn-1",
	}

	handler.OnChunk("primeiro")
	handler.OnChunk("segundo!")
	handler.Mu.Lock()
	handler.CancelPendingChunkTimer()
	handler.emitStreamEvent()
	handler.Mu.Unlock()

	if len(emitter.events) != 2 {
		t.Fatalf("eventos=%d, quer 2", len(emitter.events))
	}
	first := emitter.events[0].(ports.StreamEvent)
	second := emitter.events[1].(ports.StreamEvent)
	if first.Delta != "primeiro" || second.Delta != "segundo!" {
		t.Fatalf("deltas corrompidos após reuso do buffer: primeiro=%q segundo=%q", first.Delta, second.Delta)
	}
}

func TestBaseStreamHandlerIgnoraChunkVazioSemAlterarCoalescing(t *testing.T) {
	emitter := &retainingBaseHandlerCapture{}
	handler := &BaseStreamHandler{Emitter: emitter}

	handler.OnChunk("")

	if len(emitter.events) != 0 {
		t.Fatalf("chunk vazio emitiu %d evento(s)", len(emitter.events))
	}
	if !handler.LastEmitTime.IsZero() || handler.PendingEmit || handler.ThrottleTimer != nil {
		t.Fatal("chunk vazio alterou o estado de coalescing")
	}
}

func TestBaseStreamHandlerReiniciaEstadoEntreFasesDeThinking(t *testing.T) {
	emitter := &retainingBaseHandlerCapture{}
	handler := &BaseStreamHandler{
		Emitter:        emitter,
		ConversationID: "conversation-1",
		TurnID:         "turn-1",
	}

	handler.OnThinking("primeira")
	handler.OnThinkingDone("primeira completa")
	handler.OnThinking("segunda")

	handler.Mu.Lock()
	if handler.ThinkingTimer != nil {
		handler.ThinkingTimer.Stop()
		handler.ThinkingTimer = nil
	}
	handler.PendingThinkingEmit = false
	reasoning := handler.AccumulatedReasoning.String()
	handler.Mu.Unlock()

	var started []ports.ThinkingEvent
	for _, data := range emitter.events {
		event, ok := data.(ports.ThinkingEvent)
		if ok && event.Started {
			started = append(started, event)
		}
	}
	if len(started) != 2 {
		t.Fatalf("eventos Started=%d, quer 2", len(started))
	}
	if started[1].Content != "segunda" || reasoning != "segunda" {
		t.Fatalf("segunda fase não reiniciada: Started=%q acumulado=%q", started[1].Content, reasoning)
	}
}
