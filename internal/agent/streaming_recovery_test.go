package agent

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"assistente/internal/chat"
	"assistente/internal/core/ports"
	"assistente/internal/database"
	"assistente/internal/events"
	"assistente/internal/llm"
)

type captureEmitter struct {
	mu     sync.Mutex
	events []captured
}

type captured struct {
	name string
	data any
}

func (e *captureEmitter) Emit(name string, data any) {
	e.mu.Lock()
	e.events = append(e.events, captured{name: name, data: data})
	e.mu.Unlock()
}

func (e *captureEmitter) find(name string) []captured {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := make([]captured, 0)
	for _, ev := range e.events {
		if ev.name == name {
			out = append(out, ev)
		}
	}
	return out
}

var _ events.Emitter = (*captureEmitter)(nil)

// inMemoryMsgRepo é um MessageRepository mínimo para suportar placeholder+finalize.
// Implementa apenas o necessário para os testes de auto-recovery.
type inMemoryMsgRepo struct {
	nextID             int
	messages           []chat.Message
	createErr          error
	rejectCanceledCtx  bool
	canceledCtxUpdates int
}

func (r *inMemoryMsgRepo) CreateMessage(_ context.Context, opts chat.MessageOptions) (*chat.Message, error) {
	if r.createErr != nil {
		return nil, r.createErr
	}
	r.nextID++
	id := fmt.Sprintf("m%d", r.nextID)
	msg := chat.Message{UUIDModel: database.UUIDModel{ID: id}, ConversationID: opts.ConversationID, Role: opts.Role, Content: opts.Content, TurnID: opts.TurnID}
	r.messages = append(r.messages, msg)
	return &msg, nil
}

func (r *inMemoryMsgRepo) UpdateMessageContentAndReasoning(ctx context.Context, messageID string, content string, reasoning string, promptTokens, completionTokens, totalTokens int, model string) error {
	if err := ctx.Err(); err != nil && r.rejectCanceledCtx {
		r.canceledCtxUpdates++
		return err
	}
	for i := range r.messages {
		if r.messages[i].ID == messageID {
			r.messages[i].Content = content
			r.messages[i].Reasoning = reasoning
			r.messages[i].PromptTokens = promptTokens
			r.messages[i].CompletionTokens = completionTokens
			r.messages[i].TotalTokens = totalTokens
			r.messages[i].Model = model
			return nil
		}
	}
	return nil
}

func (r *inMemoryMsgRepo) GetMessage(ctx context.Context, messageID string) (*chat.Message, error) {
	if err := ctx.Err(); err != nil && r.rejectCanceledCtx {
		return nil, err
	}
	for i := range r.messages {
		if r.messages[i].ID == messageID {
			msg := r.messages[i]
			return &msg, nil
		}
	}
	return nil, nil
}

func (r *inMemoryMsgRepo) GetMessages(_ context.Context, _ string, _ *string) ([]chat.Message, error) {
	return r.messages, nil
}

func (r *inMemoryMsgRepo) GetMessagesByTurnID(_ context.Context, _ string, _ *string, _ string, _ int) ([]chat.Message, error) {
	return r.messages, nil
}

func (r *inMemoryMsgRepo) GetConversationSummary(context.Context, string) (string, string, error) {
	return "", "", nil
}

func (r *inMemoryMsgRepo) GetDetailedTokenStats(context.Context, string, string) (*chat.DetailedTokenStats, error) {
	return nil, nil
}

func (r *inMemoryMsgRepo) GetContextWindowUsage(context.Context, string, int) (float64, int, error) {
	return 0, 0, nil
}

func (r *inMemoryMsgRepo) GetRecentMessagesTokenCount(context.Context, string, int) (int, error) {
	return 0, nil
}

func (r *inMemoryMsgRepo) GetTurnTokenStats(context.Context, string, string) (*database.TokenStats, error) {
	return nil, nil
}

func (r *inMemoryMsgRepo) AddAssistantToolMessage(context.Context, string, string, string, string, string, string) (*chat.Message, error) {
	return nil, nil
}

func (r *inMemoryMsgRepo) AddToolResultMessage(context.Context, string, string, string, string) (*chat.Message, error) {
	return nil, nil
}

func (r *inMemoryMsgRepo) SearchMessages(context.Context, string, int) ([]chat.MessageSearchResult, error) {
	return nil, nil
}

var _ chat.MessageRepository = (*inMemoryMsgRepo)(nil)

type recoveryStreamer struct {
	calls int
	steps []func(handler llm.StreamHandler)
}

func (s *recoveryStreamer) StreamChat(_ context.Context, _ []llm.Message, _ llm.ChatParams, handler llm.StreamHandler, _ ...llm.ToolDefinition) {
	s.calls++
	idx := s.calls - 1
	if idx < 0 || idx >= len(s.steps) {
		return
	}
	s.steps[idx](handler)
}

func TestStreamSimpleWithRecovery_RetriesWithoutTerminalError(t *testing.T) {
	em := &captureEmitter{}
	repo := &inMemoryMsgRepo{}
	svc := NewService(ServiceConfig{Emitter: em, MsgRepo: repo})

	streamer := &recoveryStreamer{steps: []func(handler llm.StreamHandler){
		func(h llm.StreamHandler) {
			h.OnChunk("parcial")
			h.OnError("boom")
		},
		func(h llm.StreamHandler) {
			h.OnChunk("ok")
			h.OnDone("ok", llm.Usage{}, "m")
		},
	}}

	svc.StreamSimpleWithRecovery(context.Background(), streamer, []llm.Message{{Role: "user", Content: "hi"}}, llm.ChatParams{}, "c1", "t1", "", nil, true, 3)

	if streamer.calls != 2 {
		t.Fatalf("calls=%d, want 2", streamer.calls)
	}

	streamEvents := em.find("chat:stream")
	if len(streamEvents) == 0 {
		t.Fatalf("expected chat:stream events")
	}
	for _, ev := range streamEvents {
		se, ok := ev.data.(ports.StreamEvent)
		if !ok {
			continue
		}
		if se.Error != "" {
			t.Fatalf("unexpected terminal error emitted during recovery: %q", se.Error)
		}
	}

	// Garante que a mensagem final foi emitida com messageId (placeholder) estável.
	var done *ports.StreamEvent
	for _, ev := range streamEvents {
		se, ok := ev.data.(ports.StreamEvent)
		if ok && se.Done {
			tmp := se
			done = &tmp
		}
	}
	if done == nil {
		t.Fatalf("expected a done chat:stream event")
	}
	if done.MessageID == "" {
		t.Fatalf("expected done messageId to be set")
	}
}

func TestStreamSimpleWithRecovery_StopsWhenAssistantPlaceholderFails(t *testing.T) {
	em := &captureEmitter{}
	repo := &inMemoryMsgRepo{createErr: fmt.Errorf("db down")}
	svc := NewService(ServiceConfig{Emitter: em, MsgRepo: repo})
	streamer := &recoveryStreamer{steps: []func(handler llm.StreamHandler){
		func(h llm.StreamHandler) {
			h.OnChunk("nao deve emitir")
		},
	}}

	svc.StreamSimpleWithRecovery(
		context.Background(),
		streamer,
		nil,
		llm.ChatParams{},
		"conversation-1",
		"user-1",
		"profile",
		nil,
		false,
		1,
	)

	if streamer.calls != 0 {
		t.Fatalf("streamer should not be called without assistant placeholder, got %d calls", streamer.calls)
	}
	if got := em.find("chat:stream"); len(got) != 0 {
		t.Fatalf("expected no chat:stream without assistant placeholder, got %d", len(got))
	}
	doneEvents := em.find("chat:done")
	if len(doneEvents) != 1 {
		t.Fatalf("expected one chat:done, got %d", len(doneEvents))
	}
	done := doneEvents[0].data.(ports.DoneEvent)
	if done.Reason != "error" || done.ErrorMessage == "" {
		t.Fatalf("expected error chat:done, got %+v", done)
	}
}

func TestRunAgenticLoop_StopsWhenAssistantPlaceholderFails(t *testing.T) {
	em := &captureEmitter{}
	repo := &inMemoryMsgRepo{createErr: fmt.Errorf("db down")}
	svc := NewService(ServiceConfig{Emitter: em, MsgRepo: repo})
	streamer := &recoveryStreamer{steps: []func(handler llm.StreamHandler){
		func(h llm.StreamHandler) {
			h.OnChunk("nao deve emitir")
		},
	}}

	svc.RunAgenticLoop(
		context.Background(),
		nil,
		llm.ChatParams{MaxAgenticIterations: 1},
		"conversation-1",
		"user-1",
		nil,
		streamer,
		nil,
		func(string, int) IterationHandler {
			t.Fatal("newHandler should not be called without assistant placeholder")
			return nil
		},
		nil,
		false,
		1,
	)

	if streamer.calls != 0 {
		t.Fatalf("streamer should not be called without assistant placeholder, got %d calls", streamer.calls)
	}
	if got := em.find("chat:stream"); len(got) != 0 {
		t.Fatalf("expected no chat:stream without assistant placeholder, got %d", len(got))
	}
	doneEvents := em.find("chat:done")
	if len(doneEvents) != 1 {
		t.Fatalf("expected one chat:done, got %d", len(doneEvents))
	}
	done := doneEvents[0].data.(ports.DoneEvent)
	if done.Reason != "error" || done.ErrorMessage == "" {
		t.Fatalf("expected error chat:done, got %+v", done)
	}
}

func TestStreamSimpleWithRecovery_EmitsTerminalErrorWhenExhausted(t *testing.T) {
	em := &captureEmitter{}
	repo := &inMemoryMsgRepo{}
	svc := NewService(ServiceConfig{Emitter: em, MsgRepo: repo})

	streamer := &recoveryStreamer{steps: []func(handler llm.StreamHandler){
		func(h llm.StreamHandler) {
			h.OnChunk("parcial")
			h.OnError("boom")
		},
	}}

	svc.StreamSimpleWithRecovery(context.Background(), streamer, []llm.Message{{Role: "user", Content: "hi"}}, llm.ChatParams{}, "c1", "t1", "", nil, true, 1)

	if streamer.calls != 1 {
		t.Fatalf("calls=%d, want 1", streamer.calls)
	}
	streamEvents := em.find("chat:stream")
	var sawErr bool
	for _, ev := range streamEvents {
		se, ok := ev.data.(ports.StreamEvent)
		if ok && se.Error != "" {
			sawErr = true
			break
		}
	}
	if !sawErr {
		t.Fatalf("expected terminal error event")
	}
	if len(repo.messages) != 1 {
		t.Fatalf("expected 1 placeholder message, got %d", len(repo.messages))
	}
	if repo.messages[0].Content != "parcial" {
		t.Fatalf("expected partial content to be persisted, got %q", repo.messages[0].Content)
	}
}

func TestStreamSimpleWithRecovery_ContextCancelEmitsDone(t *testing.T) {
	em := &captureEmitter{}
	repo := &inMemoryMsgRepo{rejectCanceledCtx: true}
	svc := NewService(ServiceConfig{Emitter: em, MsgRepo: repo})
	ctx, cancel := context.WithCancel(context.Background())
	streamer := &recoveryStreamer{steps: []func(handler llm.StreamHandler){
		func(h llm.StreamHandler) {
			h.OnChunk("parcial")
			cancel()
		},
	}}

	svc.StreamSimpleWithRecovery(ctx, streamer, []llm.Message{{Role: "user", Content: "hi"}}, llm.ChatParams{}, "c1", "t1", "", nil, true, 3)

	if streamer.calls != 1 {
		t.Fatalf("calls=%d, want 1", streamer.calls)
	}
	doneEvents := em.find("chat:done")
	if len(doneEvents) != 1 {
		t.Fatalf("expected 1 chat:done, got %d", len(doneEvents))
	}
	de, ok := doneEvents[0].data.(ports.DoneEvent)
	if !ok {
		t.Fatalf("chat:done payload type mismatch")
	}
	if de.Reason != "error" || de.ErrorMessage == "" {
		t.Fatalf("expected cancellation chat:done, got reason=%q error=%q", de.Reason, de.ErrorMessage)
	}
	if de.AssistantMessageID == "" {
		t.Fatalf("expected assistantMessageId in simple cancellation chat:done")
	}
	if len(repo.messages) != 1 {
		t.Fatalf("expected 1 placeholder message, got %d", len(repo.messages))
	}
	if repo.messages[0].Content != "parcial" {
		t.Fatalf("expected partial content to be persisted with non-canceled context, got %q", repo.messages[0].Content)
	}
	if repo.canceledCtxUpdates != 0 {
		t.Fatalf("partial persistence used canceled context %d time(s)", repo.canceledCtxUpdates)
	}
}

func TestStreamSimpleWithRecovery_ConversationGoneDoesNotStream(t *testing.T) {
	em := &captureEmitter{}
	repo := &inMemoryMsgRepo{createErr: chat.ErrConversationDeleted}
	svc := NewService(ServiceConfig{Emitter: em, MsgRepo: repo})
	streamer := &recoveryStreamer{steps: []func(handler llm.StreamHandler){
		func(h llm.StreamHandler) {
			h.OnChunk("nao deve chamar")
		},
	}}

	svc.StreamSimpleWithRecovery(context.Background(), streamer, []llm.Message{{Role: "user", Content: "hi"}}, llm.ChatParams{}, "c1", "t1", "", nil, true, 3)

	if streamer.calls != 0 {
		t.Fatalf("streamer should not be called when conversation is gone, got %d calls", streamer.calls)
	}
	if len(em.find("chat:stream")) != 0 {
		t.Fatalf("conversation gone should not emit chat:stream")
	}
	if len(em.find("chat:done")) != 0 {
		t.Fatalf("conversation gone should abort silently")
	}
}

func TestRunAgenticLoop_RetriesIterationBeforeChatDoneError(t *testing.T) {
	em := &captureEmitter{}
	repo := &inMemoryMsgRepo{}
	svc := NewService(ServiceConfig{Emitter: em, MsgRepo: repo})

	streamer := &recoveryStreamer{steps: []func(handler llm.StreamHandler){
		func(h llm.StreamHandler) {
			h.OnChunk("parcial")
			h.OnError("boom")
		},
		func(h llm.StreamHandler) {
			h.OnChunk("ok")
			h.OnDone("ok", llm.Usage{}, "m")
		},
	}}

	svc.RunAgenticLoop(
		context.Background(),
		[]llm.Message{{Role: "user", Content: "hi"}},
		llm.ChatParams{MaxAgenticIterations: 1},
		"c1",
		"t1",
		nil,
		streamer,
		nil,
		func(convID string, iter int) IterationHandler {
			return NewAgenticStreamHandler(em, convID, iter, nil, "t1")
		},
		nil,
		true,
		2,
	)

	if streamer.calls != 2 {
		t.Fatalf("calls=%d, want 2", streamer.calls)
	}

	doneEvents := em.find("chat:done")
	if len(doneEvents) != 1 {
		t.Fatalf("expected 1 chat:done, got %d", len(doneEvents))
	}
	de, ok := doneEvents[0].data.(ports.DoneEvent)
	if !ok {
		t.Fatalf("chat:done payload type mismatch")
	}
	if de.ErrorMessage != "" {
		t.Fatalf("unexpected chat:done error after recovery: %q", de.ErrorMessage)
	}
}

func TestRunAgenticLoop_ErrorIncludesAssistantMessageIDAndPersistsPartial(t *testing.T) {
	em := &captureEmitter{}
	repo := &inMemoryMsgRepo{}
	svc := NewService(ServiceConfig{Emitter: em, MsgRepo: repo})

	streamer := &recoveryStreamer{steps: []func(handler llm.StreamHandler){
		func(h llm.StreamHandler) {
			h.OnChunk("parcial")
			h.OnError("boom")
		},
	}}

	svc.RunAgenticLoop(
		context.Background(),
		[]llm.Message{{Role: "user", Content: "hi"}},
		llm.ChatParams{MaxAgenticIterations: 1},
		"c1",
		"t1",
		nil,
		streamer,
		nil,
		func(convID string, iter int) IterationHandler {
			return NewAgenticStreamHandler(em, convID, iter, nil, "t1")
		},
		nil,
		true,
		1,
	)

	doneEvents := em.find("chat:done")
	if len(doneEvents) != 1 {
		t.Fatalf("expected 1 chat:done, got %d", len(doneEvents))
	}
	de, ok := doneEvents[0].data.(ports.DoneEvent)
	if !ok {
		t.Fatalf("chat:done payload type mismatch")
	}
	if de.ErrorMessage == "" {
		t.Fatalf("expected error in chat:done")
	}
	if de.AssistantMessageID == "" {
		t.Fatalf("expected assistantMessageId in chat:done error")
	}

	if len(repo.messages) != 1 {
		t.Fatalf("expected 1 placeholder message, got %d", len(repo.messages))
	}
	if repo.messages[0].ID != de.AssistantMessageID {
		t.Fatalf("assistantMessageId mismatch: done=%s repo=%s", de.AssistantMessageID, repo.messages[0].ID)
	}
	if repo.messages[0].Content != "parcial" {
		t.Fatalf("expected partial content to be persisted, got %q", repo.messages[0].Content)
	}
}

func TestRunAgenticLoop_ContextCancelPersistsPartialWithNonCanceledContext(t *testing.T) {
	em := &captureEmitter{}
	repo := &inMemoryMsgRepo{rejectCanceledCtx: true}
	svc := NewService(ServiceConfig{Emitter: em, MsgRepo: repo})
	ctx, cancel := context.WithCancel(context.Background())

	streamer := &recoveryStreamer{steps: []func(handler llm.StreamHandler){
		func(h llm.StreamHandler) {
			h.OnChunk("parcial")
			cancel()
		},
	}}

	svc.RunAgenticLoop(
		ctx,
		[]llm.Message{{Role: "user", Content: "hi"}},
		llm.ChatParams{MaxAgenticIterations: 1},
		"c1",
		"t1",
		nil,
		streamer,
		nil,
		func(convID string, iter int) IterationHandler {
			return NewAgenticStreamHandler(em, convID, iter, nil, "t1")
		},
		nil,
		true,
		1,
	)

	if streamer.calls != 1 {
		t.Fatalf("calls=%d, want 1", streamer.calls)
	}
	doneEvents := em.find("chat:done")
	if len(doneEvents) != 1 {
		t.Fatalf("expected 1 chat:done, got %d", len(doneEvents))
	}
	de, ok := doneEvents[0].data.(ports.DoneEvent)
	if !ok {
		t.Fatalf("chat:done payload type mismatch")
	}
	if de.Reason != "error" || de.ErrorMessage == "" {
		t.Fatalf("expected cancellation chat:done, got reason=%q error=%q", de.Reason, de.ErrorMessage)
	}
	if len(repo.messages) != 1 {
		t.Fatalf("expected 1 placeholder message, got %d", len(repo.messages))
	}
	if repo.messages[0].ID != de.AssistantMessageID {
		t.Fatalf("assistantMessageId mismatch: done=%s repo=%s", de.AssistantMessageID, repo.messages[0].ID)
	}
	if repo.messages[0].Content != "parcial" {
		t.Fatalf("expected partial content to be persisted with non-canceled context, got %q", repo.messages[0].Content)
	}
	if repo.canceledCtxUpdates != 0 {
		t.Fatalf("partial persistence used canceled context %d time(s)", repo.canceledCtxUpdates)
	}
}

func TestRunAgenticLoop_CanceledBeforeIterationEmitsDone(t *testing.T) {
	em := &captureEmitter{}
	repo := &inMemoryMsgRepo{}
	svc := NewService(ServiceConfig{Emitter: em, MsgRepo: repo})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	streamer := &recoveryStreamer{steps: []func(handler llm.StreamHandler){
		func(h llm.StreamHandler) {
			h.OnChunk("nao deve chamar")
		},
	}}

	svc.RunAgenticLoop(
		ctx,
		[]llm.Message{{Role: "user", Content: "hi"}},
		llm.ChatParams{MaxAgenticIterations: 1},
		"c1",
		"t1",
		nil,
		streamer,
		nil,
		func(convID string, iter int) IterationHandler {
			return NewAgenticStreamHandler(em, convID, iter, nil, "t1")
		},
		nil,
		true,
		1,
	)

	if streamer.calls != 0 {
		t.Fatalf("streamer should not be called after cancellation, got %d calls", streamer.calls)
	}
	doneEvents := em.find("chat:done")
	if len(doneEvents) != 1 {
		t.Fatalf("expected 1 chat:done, got %d", len(doneEvents))
	}
	de, ok := doneEvents[0].data.(ports.DoneEvent)
	if !ok {
		t.Fatalf("chat:done payload type mismatch")
	}
	if de.Reason != "error" || de.ErrorMessage == "" {
		t.Fatalf("expected terminal cancellation error, got reason=%q error=%q", de.Reason, de.ErrorMessage)
	}
	if de.IterationCount != 0 {
		t.Fatalf("expected 0 started iterations, got %d", de.IterationCount)
	}
	if de.AssistantMessageID == "" {
		t.Fatalf("expected assistantMessageId in cancellation chat:done")
	}
}

func TestSimpleStreamHandler_SuppressedErrorFinishesThinking(t *testing.T) {
	em := &captureEmitter{}
	handler := &SimpleStreamHandler{
		BaseStreamHandler: BaseStreamHandler{
			Emitter:        em,
			ConversationID: "c1",
			TurnID:         "t1",
		},
	}
	handler.SuppressTerminalError(true)
	handler.OnThinking("raciocinando")
	handler.OnError("boom")

	thinkingEvents := em.find("chat:thinking")
	if len(thinkingEvents) < 2 {
		t.Fatalf("expected thinking start/update and done events, got %d", len(thinkingEvents))
	}
	done, ok := thinkingEvents[len(thinkingEvents)-1].data.(ports.ThinkingEvent)
	if !ok {
		t.Fatalf("chat:thinking payload type mismatch")
	}
	if !done.Done {
		t.Fatalf("expected thinking done event before suppressed error returns")
	}
	if len(em.find("chat:stream")) != 0 {
		t.Fatalf("suppressed error should not emit terminal chat:stream")
	}
}
