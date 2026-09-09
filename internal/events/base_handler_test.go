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
