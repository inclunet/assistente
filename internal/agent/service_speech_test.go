package agent

import (
	"fmt"
	"strings"
	"sync"
	"testing"

	"assistente/internal/chat"
	"assistente/internal/core/ports"
	"assistente/internal/database"
	"assistente/internal/llm"
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
	nextID                int
	lastCreateMessageOpts *chat.MessageOptions
}

func (m *mockMsgRepo) CreateMessage(opts chat.MessageOptions) (*chat.Message, error) {
	m.nextID++
	m.lastCreateMessageOpts = &opts
	id := fmt.Sprintf("%d", m.nextID)
	return &chat.Message{UUIDModel: database.UUIDModel{ID: id}, Role: opts.Role, Content: opts.Content}, nil
}
func (m *mockMsgRepo) GetMessage(messageID string) (*chat.Message, error) {
	return &chat.Message{UUIDModel: database.UUIDModel{ID: messageID}}, nil
}
func (m *mockMsgRepo) GetMessages(string, *string) ([]chat.Message, error)   { return nil, nil }
func (m *mockMsgRepo) GetConversationSummary(string) (string, string, error) { return "", "", nil }
func (m *mockMsgRepo) GetDetailedTokenStats(string, string) (*chat.DetailedTokenStats, error) {
	return nil, nil
}
func (m *mockMsgRepo) GetContextWindowUsage(string, int) (float64, int, error) { return 0, 0, nil }
func (m *mockMsgRepo) GetRecentMessagesTokenCount(string, int) (int, error)    { return 0, nil }
func (m *mockMsgRepo) GetTurnTokenStats(string, string) (*database.TokenStats, error) {
	return nil, nil
}
func (m *mockMsgRepo) AddAssistantToolMessage(conversationID, turnID string, content, toolCalls, reasoning, model string) (*chat.Message, error) {
	m.nextID++
	id := fmt.Sprintf("%d", m.nextID)
	return &chat.Message{UUIDModel: database.UUIDModel{ID: id}, Role: "assistant", Content: content}, nil
}
func (m *mockMsgRepo) AddToolResultMessage(string, string, string, string) (*chat.Message, error) {
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
	var speechAtEventCount int
	svc := NewService(ServiceConfig{
		Emitter: emitter,
		MsgRepo: repo,
		OnSpeechRequest: func(convID, msgID string, role, text, origin, profileSlug string, interrupt bool) {
			speechAtEventCount = len(emitter.getEvents())
			speechCalls = append(speechCalls, speechCall{convID, msgID, role, text, origin, profileSlug, interrupt})
		},
	})

	svc.SaveAndFinish("1", "", AgenticResult{
		FullResponse: "Olá, mundo!",
		Model:        "test-model",
	}, "", nil, nil)

	// Verificar que speech foi chamado
	if len(speechCalls) != 1 {
		t.Fatalf("esperava 1 speechCall, got %d", len(speechCalls))
	}
	call := speechCalls[0]
	if call.convID != "1" {
		t.Errorf("convID=%s, esperava 1", call.convID)
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

	// Verificar a ordem: OnSpeechRequest (síncrono) deve ser chamado ANTES de chat:done
	evts := emitter.getEvents()
	doneIdx := -1
	for i, e := range evts {
		if e.name == "chat:done" && doneIdx == -1 {
			doneIdx = i
		}
	}

	if doneIdx == -1 {
		t.Fatal("chat:done não emitido")
	}
	// speechAtEventCount captura quantos eventos existiam quando OnSpeechRequest foi chamado.
	// Se chat:done está no índice doneIdx, o callback deve ter sido chamado quando
	// havia menos eventos (ou seja, antes de chat:done ser emitido).
	if speechAtEventCount > doneIdx {
		t.Errorf("OnSpeechRequest deve ser chamado antes de chat:done: speechAtEvents=%d doneIdx=%d", speechAtEventCount, doneIdx)
	}
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
	svc.SaveAndFinish("1", "", AgenticResult{
		FullResponse: "Sem TTS",
		Model:        "test-model",
	}, "", nil, nil)

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
		OnSpeechRequest: func(string, string, string, string, string, string, bool) {
			called = true
		},
	})

	svc.SaveAndFinish("1", "", AgenticResult{
		FullResponse: "",
		Model:        "test-model",
	}, "", nil, nil)

	if called {
		t.Error("OnSpeechRequest não deveria ser chamado com resposta vazia")
	}
}

func TestSaveAndFinish_SpeechGetsCorrectMessageID(t *testing.T) {
	emitter := &mockEmitter{}
	repo := &mockMsgRepo{}

	var gotMsgID string
	svc := NewService(ServiceConfig{
		Emitter: emitter,
		MsgRepo: repo,
		OnSpeechRequest: func(convID, msgID string, role, text, origin, profileSlug string, interrupt bool) {
			gotMsgID = msgID
		},
	})

	svc.SaveAndFinish("42", "", AgenticResult{
		FullResponse: "Resposta com ID",
		Model:        "test-model",
	}, "", nil, nil)

	// msgRepo.CreateMessage retorna nextID=1
	if gotMsgID != "1" {
		t.Errorf("messageID=%s, esperava 1 (do mockMsgRepo)", gotMsgID)
	}

	// Confirma que chat:done também carrega o mesmo ID
	evts := emitter.getEvents()
	for _, e := range evts {
		if e.name == "chat:done" {
			done := e.data.(ports.DoneEvent)
			if done.AssistantMessageID != gotMsgID {
				t.Errorf("chat:done msgID=%s, speech msgID=%s — devem ser iguais",
					done.AssistantMessageID, gotMsgID)
			}
		}
	}
}

func TestSimpleStreamHandler_PersistsAssistantWithTurnID(t *testing.T) {
	emitter := &mockEmitter{}
	repo := &mockMsgRepo{}
	svc := NewService(ServiceConfig{
		Emitter: emitter,
		MsgRepo: repo,
	})

	handler := svc.NewSimpleStreamHandler("conversation-1", "user-1", "profile", nil)
	handler.OnDone("Resposta simples", llm.Usage{}, "test-model")

	if repo.lastCreateMessageOpts == nil {
		t.Fatal("expected assistant message to be saved")
	}
	if repo.lastCreateMessageOpts.TurnID == nil || *repo.lastCreateMessageOpts.TurnID != "user-1" {
		t.Fatalf("expected assistant turnID=user-1, got %v", repo.lastCreateMessageOpts.TurnID)
	}

	for _, event := range emitter.getEvents() {
		if event.name == "chat:done" {
			done := event.data.(ports.DoneEvent)
			if done.TurnID != "user-1" {
				t.Fatalf("expected chat:done turnID=user-1, got %q", done.TurnID)
			}
			return
		}
	}
	t.Fatal("chat:done não emitido")
}

func TestSurfacePayloadPrefixesIdentifyField(t *testing.T) {
	var output strings.Builder
	logger := func(format string, args ...any) {
		_, _ = fmt.Fprintf(&output, format, args...)
		_, _ = output.WriteString("\n")
	}

	state := chat.DecodeSurfaceJSONMapWithLogger("{", "[agent] surface state payload", logger)
	context := chat.DecodeSurfaceJSONMapWithLogger("{", "[agent] surface context payload", logger)

	if state != nil || context != nil {
		t.Fatalf("expected invalid payloads to decode as nil, got state=%v context=%v", state, context)
	}
	logs := output.String()
	if !strings.Contains(logs, "[agent] surface state payload inválido") {
		t.Fatalf("esperava log do state, recebeu: %s", logs)
	}
	if !strings.Contains(logs, "[agent] surface context payload inválido") {
		t.Fatalf("esperava log do context, recebeu: %s", logs)
	}
}

// speechCall captura os parâmetros de uma invocação do OnSpeechRequest.
type speechCall struct {
	convID      string
	msgID       string
	role        string
	text        string
	origin      string
	profileSlug string
	interrupt   bool
}
