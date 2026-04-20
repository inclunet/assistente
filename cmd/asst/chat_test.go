package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	cliadapter "assistente/adapters/cli"
	"assistente/internal/app"
	"assistente/internal/core/ports"
)

// ---------------------------------------------------------------------------
// Mock chatBackend
// ---------------------------------------------------------------------------

type mockBackend struct {
	mu          sync.Mutex
	ctx         context.Context
	cancel      context.CancelFunc
	sendFn      func(uint, string, string, app.ChatParams) (uint, error)
	ensureConvFn func(string) (*app.Conversation, error)
	getConvFn   func(uint) (*app.Conversation, error)
	cancelFn    func(uint)
	sendCalls   []sendCall
}

type sendCall struct {
	ConvID  uint
	Content string
	Params  app.ChatParams
}

func newMockBackend() *mockBackend {
	ctx, cancel := context.WithCancel(context.Background())
	return &mockBackend{ctx: ctx, cancel: cancel}
}

func (m *mockBackend) SendMessage(convID uint, content, media string, params app.ChatParams) (uint, error) {
	m.mu.Lock()
	m.sendCalls = append(m.sendCalls, sendCall{ConvID: convID, Content: content, Params: params})
	m.mu.Unlock()
	if m.sendFn != nil {
		return m.sendFn(convID, content, media, params)
	}
	return 1, nil
}

func (m *mockBackend) EnsureConversation(title string) (*app.Conversation, error) {
	if m.ensureConvFn != nil {
		return m.ensureConvFn(title)
	}
	return &app.Conversation{ID: 1, Title: title}, nil
}

func (m *mockBackend) GetConversation(id uint) (*app.Conversation, error) {
	if m.getConvFn != nil {
		return m.getConvFn(id)
	}
	return &app.Conversation{ID: id, Title: "test"}, nil
}

func (m *mockBackend) CancelStreamingForConversation(convID uint) {
	if m.cancelFn != nil {
		m.cancelFn(convID)
	}
}

func (m *mockBackend) Context() context.Context {
	return m.ctx
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

// newTestEmitter creates an EmitterAdapter with captured output buffers.
func newTestEmitter() (*cliadapter.EmitterAdapter, *bytes.Buffer, *bytes.Buffer) {
	var out, errOut bytes.Buffer
	e := cliadapter.NewEmitterAdapter(
		cliadapter.WithOutput(&out),
		cliadapter.WithErrOutput(&errOut),
	)
	return e, &out, &errOut
}

// resetChatState resets global state between tests.
func resetChatState() {
	chatConversationID = 0
	chatModel = ""
	chatProfile = ""
}

// ---------------------------------------------------------------------------
// isTerminal
// ---------------------------------------------------------------------------

func TestIsTerminal_Pipe(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = r.Close() }()
	defer func() { _ = w.Close() }()

	if isTerminal(r) {
		t.Error("pipe reader should not be detected as terminal")
	}
}

// ---------------------------------------------------------------------------
// ensureConversation
// ---------------------------------------------------------------------------

func TestEnsureConversation_NewConversation(t *testing.T) {
	resetChatState()
	mock := newMockBackend()
	defer mock.cancel()

	conv, err := ensureConversation(mock)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if conv.ID != 1 {
		t.Errorf("expected conversation ID 1, got %d", conv.ID)
	}
	if chatConversationID != 1 {
		t.Errorf("expected global chatConversationID=1 after ensure, got %d", chatConversationID)
	}
}

func TestEnsureConversation_ExistingConversation(t *testing.T) {
	resetChatState()
	chatConversationID = 42

	mock := newMockBackend()
	defer mock.cancel()
	mock.getConvFn = func(id uint) (*app.Conversation, error) {
		return &app.Conversation{ID: id, Title: "existente"}, nil
	}

	conv, err := ensureConversation(mock)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if conv.ID != 42 {
		t.Errorf("expected conversation ID 42, got %d", conv.ID)
	}
}

func TestEnsureConversation_GetError(t *testing.T) {
	resetChatState()
	chatConversationID = 99

	mock := newMockBackend()
	defer mock.cancel()
	mock.getConvFn = func(id uint) (*app.Conversation, error) {
		return nil, fmt.Errorf("not found")
	}

	_, err := ensureConversation(mock)
	if err == nil {
		t.Fatal("expected error when conversation not found")
	}
	if !strings.Contains(err.Error(), "conversa 99 não encontrada") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestEnsureConversation_CreateError(t *testing.T) {
	resetChatState()

	mock := newMockBackend()
	defer mock.cancel()
	mock.ensureConvFn = func(title string) (*app.Conversation, error) {
		return nil, fmt.Errorf("db locked")
	}

	_, err := ensureConversation(mock)
	if err == nil {
		t.Fatal("expected error when creation fails")
	}
	if !strings.Contains(err.Error(), "erro ao criar conversa") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestEnsureConversation_ReuseInREPL(t *testing.T) {
	resetChatState()
	mock := newMockBackend()
	defer mock.cancel()
	mock.ensureConvFn = func(title string) (*app.Conversation, error) {
		return &app.Conversation{ID: 7, Title: title}, nil
	}

	// First call creates
	conv1, err := ensureConversation(mock)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if conv1.ID != 7 {
		t.Errorf("expected ID 7, got %d", conv1.ID)
	}

	// chatConversationID should be set
	if chatConversationID != 7 {
		t.Fatalf("expected global to be 7, got %d", chatConversationID)
	}

	// Second call reuses (goes through GetConversation path)
	conv2, err := ensureConversation(mock)
	if err != nil {
		t.Fatalf("unexpected error on reuse: %v", err)
	}
	if conv2.ID != 7 {
		t.Errorf("expected reused ID 7, got %d", conv2.ID)
	}
}

// ---------------------------------------------------------------------------
// sendAndWait
// ---------------------------------------------------------------------------

func TestSendAndWait_Success(t *testing.T) {
	resetChatState()
	emitter, out, _ := newTestEmitter()

	mock := newMockBackend()
	defer mock.cancel()

	// Simulate async streaming completion
	mock.sendFn = func(convID uint, content, media string, params app.ChatParams) (uint, error) {
		go func() {
			time.Sleep(5 * time.Millisecond)
			emitter.Emit("chat:stream", ports.StreamEvent{Content: "Olá"})
			emitter.Emit("chat:stream", ports.StreamEvent{Content: "Olá mundo"})
			emitter.Emit("chat:stream", ports.StreamEvent{Done: true})
		}()
		return 1, nil
	}

	err := sendAndWait(mock, emitter, "teste")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got := out.String()
	if !strings.Contains(got, "Olá") || !strings.Contains(got, "mundo") {
		t.Errorf("expected output to contain streamed content, got %q", got)
	}
}

func TestSendAndWait_SendError(t *testing.T) {
	resetChatState()
	emitter, _, _ := newTestEmitter()

	mock := newMockBackend()
	defer mock.cancel()
	mock.sendFn = func(uint, string, string, app.ChatParams) (uint, error) {
		return 0, fmt.Errorf("provider offline")
	}

	err := sendAndWait(mock, emitter, "teste")
	if err == nil {
		t.Fatal("expected error from SendMessage")
	}
	if !strings.Contains(err.Error(), "erro ao enviar mensagem") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestSendAndWait_EnsureConversationError(t *testing.T) {
	resetChatState()
	emitter, _, _ := newTestEmitter()

	mock := newMockBackend()
	defer mock.cancel()
	mock.ensureConvFn = func(string) (*app.Conversation, error) {
		return nil, fmt.Errorf("db error")
	}

	err := sendAndWait(mock, emitter, "teste")
	if err == nil {
		t.Fatal("expected error from ensureConversation")
	}
	if !strings.Contains(err.Error(), "erro ao criar conversa") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestSendAndWait_ContextCancelled(t *testing.T) {
	resetChatState()
	emitter, _, _ := newTestEmitter()

	mock := newMockBackend()
	// Cancel context immediately after SendMessage
	mock.sendFn = func(uint, string, string, app.ChatParams) (uint, error) {
		mock.cancel()
		return 1, nil
	}

	err := sendAndWait(mock, emitter, "teste")
	if err == nil {
		t.Fatal("expected cancellation error")
	}
	if !strings.Contains(err.Error(), "cancelado") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestSendAndWait_StreamError(t *testing.T) {
	resetChatState()
	emitter, _, errOut := newTestEmitter()

	mock := newMockBackend()
	defer mock.cancel()

	// Simulate async streaming error via emitter
	mock.sendFn = func(convID uint, content, media string, params app.ChatParams) (uint, error) {
		go func() {
			time.Sleep(5 * time.Millisecond)
			emitter.Emit("chat:stream", ports.StreamEvent{Error: "rate limit"})
		}()
		return 1, nil
	}

	err := sendAndWait(mock, emitter, "teste")
	// sendAndWait returns nil because done was signaled (even on stream error)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(errOut.String(), "rate limit") {
		t.Errorf("expected error in stderr, got %q", errOut.String())
	}
}

func TestSendAndWait_PassesModelAndProfile(t *testing.T) {
	resetChatState()
	chatModel = "gpt-4o"
	chatProfile = "dev"
	emitter, _, _ := newTestEmitter()

	mock := newMockBackend()
	defer mock.cancel()
	mock.sendFn = func(convID uint, content, media string, params app.ChatParams) (uint, error) {
		go func() {
			time.Sleep(5 * time.Millisecond)
			emitter.Emit("chat:stream", ports.StreamEvent{Done: true})
		}()
		return 1, nil
	}

	err := sendAndWait(mock, emitter, "hello")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	mock.mu.Lock()
	defer mock.mu.Unlock()
	if len(mock.sendCalls) != 1 {
		t.Fatalf("expected 1 SendMessage call, got %d", len(mock.sendCalls))
	}
	call := mock.sendCalls[0]
	if call.Params.Model != "gpt-4o" {
		t.Errorf("expected model gpt-4o, got %q", call.Params.Model)
	}
	if call.Params.ProfileSlug != "dev" {
		t.Errorf("expected profile dev, got %q", call.Params.ProfileSlug)
	}
}

// ---------------------------------------------------------------------------
// runREPL (simulated via piped stdin)
// ---------------------------------------------------------------------------

func TestRunREPL_ProcessesMultipleLines(t *testing.T) {
	resetChatState()
	emitter, out, _ := newTestEmitter()

	mock := newMockBackend()
	defer mock.cancel()

	callCount := 0
	mock.sendFn = func(convID uint, content, media string, params app.ChatParams) (uint, error) {
		callCount++
		n := callCount
		go func() {
			time.Sleep(2 * time.Millisecond)
			emitter.Emit("chat:stream", ports.StreamEvent{Content: fmt.Sprintf("resp%d", n)})
			emitter.Emit("chat:stream", ports.StreamEvent{Done: true})
		}()
		return 1, nil
	}

	// Simulate piped input with 2 messages
	oldStdin := os.Stdin
	r, w, _ := os.Pipe()
	os.Stdin = r
	defer func() { os.Stdin = oldStdin }()

	go func() {
		_, _ = w.WriteString("primeira\nsegunda\n")
		_ = w.Close()
	}()

	err := runREPL(mock, emitter)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	mock.mu.Lock()
	defer mock.mu.Unlock()
	if len(mock.sendCalls) != 2 {
		t.Fatalf("expected 2 SendMessage calls, got %d", len(mock.sendCalls))
	}
	if mock.sendCalls[0].Content != "primeira" {
		t.Errorf("expected first message 'primeira', got %q", mock.sendCalls[0].Content)
	}
	if mock.sendCalls[1].Content != "segunda" {
		t.Errorf("expected second message 'segunda', got %q", mock.sendCalls[1].Content)
	}
	if !strings.Contains(out.String(), "resp") {
		t.Errorf("expected streamed responses in output, got %q", out.String())
	}
}

func TestRunREPL_SkipsEmptyLines(t *testing.T) {
	resetChatState()
	emitter, _, _ := newTestEmitter()

	mock := newMockBackend()
	defer mock.cancel()
	mock.sendFn = func(uint, string, string, app.ChatParams) (uint, error) {
		go func() {
			time.Sleep(2 * time.Millisecond)
			emitter.Emit("chat:stream", ports.StreamEvent{Done: true})
		}()
		return 1, nil
	}

	oldStdin := os.Stdin
	r, w, _ := os.Pipe()
	os.Stdin = r
	defer func() { os.Stdin = oldStdin }()

	go func() {
		_, _ = w.WriteString("\n  \nhello\n")
		_ = w.Close()
	}()

	err := runREPL(mock, emitter)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	mock.mu.Lock()
	defer mock.mu.Unlock()
	if len(mock.sendCalls) != 1 {
		t.Errorf("expected 1 SendMessage call (empty lines skipped), got %d", len(mock.sendCalls))
	}
}

func TestRunREPL_ContinuesAfterSendError(t *testing.T) {
	resetChatState()
	emitter, _, _ := newTestEmitter()

	mock := newMockBackend()
	defer mock.cancel()

	callN := 0
	mock.sendFn = func(uint, string, string, app.ChatParams) (uint, error) {
		callN++
		if callN == 1 {
			return 0, fmt.Errorf("provider error")
		}
		go func() {
			time.Sleep(2 * time.Millisecond)
			emitter.Emit("chat:stream", ports.StreamEvent{Done: true})
		}()
		return 1, nil
	}

	oldStdin := os.Stdin
	r, w, _ := os.Pipe()
	os.Stdin = r
	defer func() { os.Stdin = oldStdin }()

	go func() {
		_, _ = w.WriteString("first\nsecond\n")
		_ = w.Close()
	}()

	err := runREPL(mock, emitter)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	mock.mu.Lock()
	defer mock.mu.Unlock()
	if len(mock.sendCalls) != 2 {
		t.Errorf("REPL should continue after error; expected 2 calls, got %d", len(mock.sendCalls))
	}
}
