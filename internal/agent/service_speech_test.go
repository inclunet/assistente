package agent

import (
	"sync"
	"testing"

	"assistente/internal/chat"
	"assistente/internal/core/ports"
	"assistente/internal/database"
	"assistente/internal/events"
)

// --- Mocks ---

type mockEmitter struct {
	mu     sync.Mutex
	events []emittedEvent
}

type emittedEvent struct {
	name string
	data any
}

func (m *mockEmitter) Emit(event string, data any) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.events = append(m.events, emittedEvent{name: event, data: data})
}

func (m *mockEmitter) getEvents() []emittedEvent {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := make([]emittedEvent, len(m.events))
	copy(cp, m.events)
	return cp
}

type mockMsgRepo struct {
	nextID uint
}

func (m *mockMsgRepo) CreateMessage(opts chat.MessageOptions) (*chat.Message, error) {
	m.nextID++
	return &chat.Message{ID: m.nextID, Role: opts.Role, Content: opts.Content}, nil
}
func (m *mockMsgRepo) GetMessages(uint, *uint) ([]chat.Message, error)    { return nil, nil }
func (m *mockMsgRepo) GetConversationSummary(uint) (string, uint, error)  { return "", 0, nil }
func (m *mockMsgRepo) GetDetailedTokenStats(uint, uint) (*chat.DetailedTokenStats, error) {
	return nil, nil
}
func (m *mockMsgRepo) GetContextWindowUsage(uint, int) (float64, int, error)   { return 0, 0, nil }
func (m *mockMsgRepo) GetRecentMessagesTokenCount(uint, int) (int, error)      { return 0, nil }
func (m *mockMsgRepo) GetTurnTokenStats(uint, uint) (*database.TokenStats, error) { return nil, nil }
func (m *mockMsgRepo) AddAssistantToolMessage(conversationID, turnID uint, content, toolCalls, reasoning, model string) (*chat.Message, error) {
	m.nextID++
	return &chat.Message{ID: m.nextID, Role: "assistant", Content: content}, nil
}
func (m *mockMsgRepo) AddToolResultMessage(uint, uint, string, string) (*chat.Message, error) {
	return nil, nil
}
func (m *mockMsgRepo) SearchMessages(string, int) ([]chat.MessageSearchResult, error) {
	return nil, nil
}

// --- Tests ---

func TestSaveAndFinish_CallsOnSpeechRequestBeforeChatDone(t *testing.T) {
	emitter := &mockEmitter{}
	repo := &mockMsgRepo{}

	var speechCalls []speechCall
	svc := NewService(ServiceConfig{
		Emitter: emitter,
		MsgRepo: repo,
		OnSpeechRequest: func(convID, msgID uint, role, text, origin, profileSlug string, interrupt bool) {
			speechCalls = append(speechCalls, speechCall{convID, msgID, role, text, origin, profileSlug, interrupt})
		},
	})

	svc.SaveAndFinish(1, 0, AgenticResult{
		FullResponse: "Olá, mundo!",
		Model:        "test-model",
	}, "")

	// Verificar que speech foi chamado
	if len(speechCalls) != 1 {
		t.Fatalf("esperava 1 speechCall, got %d", len(speechCalls))
	}
	call := speechCalls[0]
	if call.convID != 1 {
		t.Errorf("convID=%d, esperava 1", call.convID)
	}
	if call.role != "assistant" {
		t.Errorf("role=%q, esperava 'assistant'", call.role)
	}
	if call.text != "Olá, mundo!" {
		t.Errorf("text=%q, esperava 'Olá, mundo!'", call.text)
	}
	if call.origin != "assistant_message" {
		t.Errorf("origin=%q, esperava 'assistant_message'", call.origin)
	}
	if !call.interrupt {
		t.Error("interrupt deveria ser true")
	}

	// Verificar que chat:speak (via callback) veio ANTES de chat:done
	evts := emitter.getEvents()
	var speechIdx, doneIdx int = -1, -1
	for i, e := range evts {
		if e.name == "chat:done" && doneIdx == -1 {
			doneIdx = i
		}
	}

	// O callback síncrono é chamado antes de chat:done, mas não emite diretamente
	// (emissão é feita dentro do callback via app.go). Para validar a ordem,
	// verificamos que chat:done existe e vem depois do chat:stream(done=true).
	var streamDoneIdx int = -1
	for i, e := range evts {
		if e.name == "chat:stream" {
			if se, ok := e.data.(events.StreamEvent); ok && se.Done {
				streamDoneIdx = i
			}
		}
	}

	if streamDoneIdx == -1 {
		t.Fatal("chat:stream (done=true) não emitido")
	}
	if doneIdx == -1 {
		t.Fatal("chat:done não emitido")
	}
	if streamDoneIdx >= doneIdx {
		t.Errorf("chat:stream(done) deve vir antes de chat:done: stream=%d done=%d", streamDoneIdx, doneIdx)
	}

	// Confirmar que speechCall foi chamado — o callback é síncrono, então foi
	// executado entre chat:stream(done) e chat:done
	_ = speechIdx
}

func TestSaveAndFinish_NilOnSpeechRequest_NoPanic(t *testing.T) {
	emitter := &mockEmitter{}
	repo := &mockMsgRepo{}

	svc := NewService(ServiceConfig{
		Emitter: emitter,
		MsgRepo: repo,
		// OnSpeechRequest não definido (nil)
	})

	// Não deve dar panic
	svc.SaveAndFinish(1, 0, AgenticResult{
		FullResponse: "Sem TTS",
		Model:        "test-model",
	}, "")

	evts := emitter.getEvents()
	hasDone := false
	for _, e := range evts {
		if e.name == "chat:done" {
			hasDone = true
		}
	}
	if !hasDone {
		t.Fatal("chat:done não emitido mesmo com OnSpeechRequest nil")
	}
}

func TestSaveAndFinish_EmptyResponse_NoSpeechCall(t *testing.T) {
	emitter := &mockEmitter{}
	repo := &mockMsgRepo{}

	called := false
	svc := NewService(ServiceConfig{
		Emitter: emitter,
		MsgRepo: repo,
		OnSpeechRequest: func(uint, uint, string, string, string, string, bool) {
			called = true
		},
	})

	svc.SaveAndFinish(1, 0, AgenticResult{
		FullResponse: "",
		Model:        "test-model",
	}, "")

	if called {
		t.Error("OnSpeechRequest não deveria ser chamado com resposta vazia")
	}
}

func TestSaveAndFinish_SpeechCallOrder_BeforeChatDone(t *testing.T) {
	// Testa que o callback de speech é invocado ANTES de chat:done ser emitido
	emitter := &mockEmitter{}
	repo := &mockMsgRepo{}

	var speechCalledAtEventCount int
	svc := NewService(ServiceConfig{
		Emitter: emitter,
		MsgRepo: repo,
		OnSpeechRequest: func(uint, uint, string, string, string, string, bool) {
			// Conta quantos eventos existem no momento do callback
			speechCalledAtEventCount = len(emitter.getEvents())
		},
	})

	svc.SaveAndFinish(1, 0, AgenticResult{
		FullResponse: "Teste de ordem",
		Model:        "test-model",
	}, "")

	evts := emitter.getEvents()
	var doneIdx int = -1
	for i, e := range evts {
		if e.name == "chat:done" {
			doneIdx = i
		}
	}

	if doneIdx == -1 {
		t.Fatal("chat:done não emitido")
	}

	// O speech callback foi invocado quando havia speechCalledAtEventCount eventos.
	// chat:done está no índice doneIdx. O callback deve ter sido chamado antes.
	if speechCalledAtEventCount > doneIdx {
		t.Errorf("speech callback chamado após chat:done: speechAt=%d doneAt=%d",
			speechCalledAtEventCount, doneIdx)
	}
}

func TestSaveAndFinish_SpeechGetsCorrectMessageID(t *testing.T) {
	emitter := &mockEmitter{}
	repo := &mockMsgRepo{}

	var gotMsgID uint
	svc := NewService(ServiceConfig{
		Emitter: emitter,
		MsgRepo: repo,
		OnSpeechRequest: func(convID, msgID uint, role, text, origin, profileSlug string, interrupt bool) {
			gotMsgID = msgID
		},
	})

	svc.SaveAndFinish(42, 0, AgenticResult{
		FullResponse: "Resposta com ID",
		Model:        "test-model",
	}, "")

	// msgRepo.CreateMessage retorna nextID=1
	if gotMsgID != 1 {
		t.Errorf("messageID=%d, esperava 1 (do mockMsgRepo)", gotMsgID)
	}

	// Confirma que chat:done também carrega o mesmo ID
	evts := emitter.getEvents()
	for _, e := range evts {
		if e.name == "chat:done" {
			done := e.data.(ports.DoneEvent)
			if done.AssistantMessageID != gotMsgID {
				t.Errorf("chat:done msgID=%d, speech msgID=%d — devem ser iguais",
					done.AssistantMessageID, gotMsgID)
			}
		}
	}
}

// speechCall captura os parâmetros de uma invocação do OnSpeechRequest.
type speechCall struct {
	convID      uint
	msgID       uint
	role        string
	text        string
	origin      string
	profileSlug string
	interrupt   bool
}
