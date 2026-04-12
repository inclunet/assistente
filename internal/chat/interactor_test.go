package chat

import (
	"context"
	"strings"
	"testing"

	"assistente/internal/core/ports"
	"assistente/internal/events"
)

// spyEmitter captures emitted events for assertions.
type spyEmitter struct {
	emitted []emittedEvent
}

type emittedEvent struct {
	name string
	data any
}

func (s *spyEmitter) Emit(event string, data any) {
	s.emitted = append(s.emitted, emittedEvent{name: event, data: data})
}

func (s *spyEmitter) findError() *ports.ErrorEvent {
	for _, e := range s.emitted {
		if e.name == "chat:error" {
			if ev, ok := e.data.(ports.ErrorEvent); ok {
				return &ev
			}
		}
	}
	return nil
}

var _ events.Emitter = (*spyEmitter)(nil)

// noopConvRepo is a minimal ConversationRepository for tests.
type noopConvRepo struct{}

func (noopConvRepo) GetConversationInfo(_ uint) (*Conversation, error) { return nil, nil }
func (noopConvRepo) UpdateConversation(_ uint, _, _ string) error      { return nil }
func (noopConvRepo) UpdateConversationChannel(_ uint, _, _ string) error {
	return nil
}

func newTestInteractor(em events.Emitter) *Interactor {
	return NewInteractor(InteractorConfig{
		Emitter:  em,
		ConvRepo: noopConvRepo{},
	})
}

func TestPrepareContext_RejectsContentExceedingMaxSize(t *testing.T) {
	spy := &spyEmitter{}
	inter := newTestInteractor(spy)

	bigContent := strings.Repeat("x", MaxMessageContentSize+1)

	_, err := inter.PrepareContext(context.Background(), PrepareContextRequest{
		ConversationID: 1,
		UserContent:    bigContent,
	})

	if err == nil {
		t.Fatal("expected error for content exceeding max size")
	}
	if !strings.Contains(err.Error(), "Mensagem muito grande") {
		t.Errorf("unexpected error: %q", err.Error())
	}

	ev := spy.findError()
	if ev == nil {
		t.Fatal("expected chat:error event to be emitted")
	}
	if ev.ConversationID != 1 {
		t.Errorf("expected conversationId=1, got %d", ev.ConversationID)
	}
}

func TestPrepareContext_AcceptsContentAtExactMaxSize(t *testing.T) {
	spy := &spyEmitter{}
	inter := newTestInteractor(spy)

	exactContent := strings.Repeat("x", MaxMessageContentSize)

	// Should fail after size check (at provider check), not AT the size check
	_, err := inter.PrepareContext(context.Background(), PrepareContextRequest{
		ConversationID: 1,
		UserContent:    exactContent,
	})

	// Should NOT fail with "Mensagem muito grande"
	if err != nil && strings.Contains(err.Error(), "Mensagem muito grande") {
		t.Errorf("content at exact max size should not be rejected: %q", err.Error())
	}
}

func TestPrepareContext_RejectsMediaExceedingMaxSize(t *testing.T) {
	spy := &spyEmitter{}
	inter := newTestInteractor(spy)

	bigMedia := strings.Repeat("x", MaxMediaSize+1)

	_, err := inter.PrepareContext(context.Background(), PrepareContextRequest{
		ConversationID: 1,
		UserContent:    "hello",
		UserMedia:      bigMedia,
	})

	if err == nil {
		t.Fatal("expected error for media exceeding max size")
	}
	if !strings.Contains(err.Error(), "Mídia muito grande") {
		t.Errorf("unexpected error: %q", err.Error())
	}

	ev := spy.findError()
	if ev == nil {
		t.Fatal("expected chat:error event to be emitted")
	}
}

func TestPrepareContext_RejectsConversationIDZero(t *testing.T) {
	spy := &spyEmitter{}
	inter := newTestInteractor(spy)

	_, err := inter.PrepareContext(context.Background(), PrepareContextRequest{
		ConversationID: 0,
		UserContent:    "hello",
	})

	if err == nil {
		t.Fatal("expected error for conversationID=0")
	}
	if !strings.Contains(err.Error(), "conversationID") {
		t.Errorf("unexpected error: %q", err.Error())
	}

	ev := spy.findError()
	if ev == nil {
		t.Fatal("expected chat:error event to be emitted")
	}
	if ev.ConversationID != 0 {
		t.Errorf("expected conversationId=0, got %d", ev.ConversationID)
	}
}
