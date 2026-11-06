package main

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"assistente/internal/app"
	"assistente/internal/chat"
	"assistente/internal/database"
)

// ---------------------------------------------------------------------------
// Mock historyBackend
// ---------------------------------------------------------------------------

type mockHistoryBackend struct {
	searchResults []database.MessageSearchResult
	searchErr     error
	conversations []app.Conversation
	convsErr      error
	conversation  *app.Conversation
	convErr       error
	messages      []*chat.TimelineItem
	children      []*chat.TimelineItem
	readWindow    func(database.MessageWindowQuery) (*chat.TimelineWindowSnapshot, error)
	messagesErr   error
	deleteErr     error

	// Capture calls
	deletedID string
}

func (m *mockHistoryBackend) SearchConversationHistory(query string, limit int) ([]database.MessageSearchResult, error) {
	return m.searchResults, m.searchErr
}

func (m *mockHistoryBackend) GetConversations() ([]app.Conversation, error) {
	return m.conversations, m.convsErr
}

func (m *mockHistoryBackend) GetConversation(id string) (*app.Conversation, error) {
	return m.conversation, m.convErr
}

func (m *mockHistoryBackend) ReadTimelineWindow(q database.MessageWindowQuery) (*chat.TimelineWindowSnapshot, error) {
	if m.readWindow != nil {
		return m.readWindow(q)
	}
	items := m.messages
	if q.ParentID != nil {
		items = m.children
	}
	return &chat.TimelineWindowSnapshot{Items: items}, m.messagesErr
}

func (m *mockHistoryBackend) DeleteConversation(id string) error {
	m.deletedID = id
	return m.deleteErr
}

// ---------------------------------------------------------------------------
// runHistoryList — no search
// ---------------------------------------------------------------------------

func TestHistoryList_Conversations(t *testing.T) {
	now := time.Now()
	mock := &mockHistoryBackend{
		conversations: []app.Conversation{
			{UUIDModel: database.UUIDModel{ID: "1", UpdatedAt: now}, Title: "Sobre Go", MessageCount: 10},
			{UUIDModel: database.UUIDModel{ID: "2", UpdatedAt: now}, Title: "Projeto React", MessageCount: 5},
		},
	}

	var out bytes.Buffer
	err := runHistoryList(mock, &out, "", 20)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := out.String()
	if !strings.Contains(output, "ID") {
		t.Error("expected header row")
	}
	if !strings.Contains(output, "Sobre Go") {
		t.Error("expected 'Sobre Go' in output")
	}
	if !strings.Contains(output, "Projeto React") {
		t.Error("expected 'Projeto React' in output")
	}
}

func TestHistoryList_Empty(t *testing.T) {
	mock := &mockHistoryBackend{
		conversations: []app.Conversation{},
	}

	var out bytes.Buffer
	err := runHistoryList(mock, &out, "", 20)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out.String(), "Nenhuma conversa no histórico") {
		t.Error("expected empty message")
	}
}

func TestHistoryList_Error(t *testing.T) {
	mock := &mockHistoryBackend{
		convsErr: fmt.Errorf("db error"),
	}

	var out bytes.Buffer
	err := runHistoryList(mock, &out, "", 20)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "erro ao listar conversas") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestHistoryList_WithLimit(t *testing.T) {
	now := time.Now()
	mock := &mockHistoryBackend{
		conversations: []app.Conversation{
			{UUIDModel: database.UUIDModel{ID: "1", UpdatedAt: now}, Title: "First", MessageCount: 3},
			{UUIDModel: database.UUIDModel{ID: "2", UpdatedAt: now}, Title: "Second", MessageCount: 5},
			{UUIDModel: database.UUIDModel{ID: "3", UpdatedAt: now}, Title: "Third", MessageCount: 7},
		},
	}

	var out bytes.Buffer
	err := runHistoryList(mock, &out, "", 2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := out.String()
	if !strings.Contains(output, "First") {
		t.Error("expected 'First' in output")
	}
	if !strings.Contains(output, "Second") {
		t.Error("expected 'Second' in output")
	}
	if strings.Contains(output, "Third") {
		t.Error("should NOT contain 'Third' (limit=2)")
	}
}

// ---------------------------------------------------------------------------
// runHistoryList — with search
// ---------------------------------------------------------------------------

func TestHistoryList_Search(t *testing.T) {
	mock := &mockHistoryBackend{
		searchResults: []database.MessageSearchResult{
			{ConversationID: "1", ConversationTitle: "Sobre Go", Role: "user", Snippet: "como usar goroutines"},
			{ConversationID: "2", ConversationTitle: "Deploy", Role: "assistant", Snippet: "docker compose up"},
		},
	}

	var out bytes.Buffer
	err := runHistoryList(mock, &out, "goroutines", 20)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := out.String()
	if !strings.Contains(output, "CONVERSA") {
		t.Error("expected search header row")
	}
	if !strings.Contains(output, "goroutines") {
		t.Error("expected snippet in output")
	}
}

func TestHistoryList_SearchEmpty(t *testing.T) {
	mock := &mockHistoryBackend{
		searchResults: []database.MessageSearchResult{},
	}

	var out bytes.Buffer
	err := runHistoryList(mock, &out, "nonexistent", 20)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out.String(), "Nenhum resultado encontrado") {
		t.Error("expected empty search message")
	}
}

func TestHistoryList_SearchError(t *testing.T) {
	mock := &mockHistoryBackend{
		searchErr: fmt.Errorf("search failure"),
	}

	var out bytes.Buffer
	err := runHistoryList(mock, &out, "query", 20)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "erro ao buscar") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestHistoryList_SearchLongSnippet(t *testing.T) {
	long := strings.Repeat("A", 100)
	mock := &mockHistoryBackend{
		searchResults: []database.MessageSearchResult{
			{ConversationID: "1", ConversationTitle: "Long", Role: "user", Snippet: long},
		},
	}

	var out bytes.Buffer
	err := runHistoryList(mock, &out, "query", 20)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out.String(), "...") {
		t.Error("expected truncated snippet")
	}
}

// ---------------------------------------------------------------------------
// runHistoryShow
// ---------------------------------------------------------------------------

func TestHistoryShow_Success(t *testing.T) {
	mock := &mockHistoryBackend{
		conversation: &app.Conversation{UUIDModel: database.UUIDModel{ID: "42"}, Title: "Minha Conversa"},
		messages: []*chat.TimelineItem{
			{Representative: &database.ChatMessage{Role: "user"}, Segments: []chat.ResponseSegment{{Kind: "text", Text: "olá"}}},
			{Representative: &database.ChatMessage{Role: "assistant"}, Segments: []chat.ResponseSegment{{Kind: "text", Text: "oi!"}}},
		},
	}

	var out bytes.Buffer
	err := runHistoryShow(mock, &out, "42")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := out.String()
	if !strings.Contains(output, "Minha Conversa") {
		t.Error("expected conversation title")
	}
	if !strings.Contains(output, "[USER]") {
		t.Error("expected [USER] in output")
	}
	if !strings.Contains(output, "[ASSISTANT]") {
		t.Error("expected [ASSISTANT] in output")
	}
	if !strings.Contains(output, "olá") {
		t.Error("expected user message content")
	}
}

func TestHistoryShow_NotFound(t *testing.T) {
	mock := &mockHistoryBackend{
		convErr: fmt.Errorf("not found"),
	}

	var out bytes.Buffer
	err := runHistoryShow(mock, &out, "999")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "conversa não encontrada") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestHistoryShow_MessagesError(t *testing.T) {
	mock := &mockHistoryBackend{
		conversation: &app.Conversation{UUIDModel: database.UUIDModel{ID: "1"}, Title: "Test"},
		messagesErr:  fmt.Errorf("load error"),
	}

	var out bytes.Buffer
	err := runHistoryShow(mock, &out, "1")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "erro ao carregar mensagens") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestHistoryShow_NestedMessages(t *testing.T) {
	mock := &mockHistoryBackend{
		conversation: &app.Conversation{UUIDModel: database.UUIDModel{ID: "1"}, Title: "Nested"},
		messages: []*chat.TimelineItem{
			{Representative: &database.ChatMessage{UUIDModel: database.UUIDModel{ID: "parent"}, Role: "user", Presentation: &database.MessagePresentation{ChildCount: 1}}, Segments: []chat.ResponseSegment{{Kind: "text", Text: "pergunta"}}},
		},
		children: []*chat.TimelineItem{{Representative: &database.ChatMessage{Role: "assistant"}, Segments: []chat.ResponseSegment{{Kind: "text", Text: "resposta"}}}},
	}

	var out bytes.Buffer
	err := runHistoryShow(mock, &out, "1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := out.String()
	if !strings.Contains(output, "pergunta") {
		t.Error("expected parent message")
	}
	if !strings.Contains(output, "resposta") {
		t.Error("expected child message")
	}
}

// ---------------------------------------------------------------------------
// runHistoryDelete
// ---------------------------------------------------------------------------

func TestHistoryDelete_Success(t *testing.T) {
	mock := &mockHistoryBackend{}

	var out bytes.Buffer
	err := runHistoryDelete(mock, &out, "42")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mock.deletedID != "42" {
		t.Errorf("expected deleted ID 42, got %s", mock.deletedID)
	}
	if !strings.Contains(out.String(), "Conversa 42 removida") {
		t.Error("expected success message")
	}
}

func TestHistoryDelete_Error(t *testing.T) {
	mock := &mockHistoryBackend{
		deleteErr: fmt.Errorf("not found"),
	}

	var out bytes.Buffer
	err := runHistoryDelete(mock, &out, "999")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "erro ao remover conversa") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestHistoryShowPaginatesRootAndThread(t *testing.T) {
	version := database.ChatTimelineVersion{Incarnation: "conversation", Revision: 1}
	calls := 0
	svc := &mockHistoryBackend{conversation: &app.Conversation{UUIDModel: database.UUIDModel{ID: "c"}}}
	svc.readWindow = func(q database.MessageWindowQuery) (*chat.TimelineWindowSnapshot, error) {
		calls++
		if q.ConversationID != "c" || q.Direction != "after" || q.Limit != database.MaxMessageWindowRows {
			t.Fatalf("consulta incorreta: %+v", q)
		}
		first := q.Cursor == ""
		if first && q.Anchor != "start" || !first && q.Anchor != "" {
			t.Fatalf("âncora incorreta: %+v", q)
		}
		prefix := "root"
		if q.ParentID != nil {
			if *q.ParentID != "parent" {
				t.Fatal("pai incorreto", *q.ParentID)
			}
			prefix = "thread"
		}
		snapshot := &chat.TimelineWindowSnapshot{Version: version}
		start, end := 0, database.MaxMessageWindowRows
		if !first {
			if q.Cursor != prefix+"-next" {
				t.Fatal("cursor incorreto", q.Cursor)
			}
			start, end = end, end+1
		}
		for i := start; i < end; i++ {
			item := &chat.TimelineItem{Segments: []chat.ResponseSegment{{Kind: "text", Text: fmt.Sprintf("%s-%03d", prefix, i)}}}
			if prefix == "root" && i == 0 {
				item.Representative = &database.ChatMessage{UUIDModel: database.UUIDModel{ID: "parent"}, Presentation: &database.MessagePresentation{ChildCount: 1}}
			}
			snapshot.Items = append(snapshot.Items, item)
		}
		snapshot.Window.HasAfter = first
		if first {
			snapshot.AfterCursor = prefix + "-next"
		}
		return snapshot, nil
	}
	var out bytes.Buffer
	if err := runHistoryShow(svc, &out, "c"); err != nil {
		t.Fatal(err)
	}
	if calls != 4 {
		t.Fatal("quantidade de páginas", calls)
	}
	for _, prefix := range []string{"root", "thread"} {
		for i := 0; i <= database.MaxMessageWindowRows; i++ {
			if strings.Count(out.String(), fmt.Sprintf("%s-%03d", prefix, i)) != 1 {
				t.Fatalf("item omitido ou repetido: %s-%03d", prefix, i)
			}
		}
	}
}

func TestHistoryShowRejectsVersionChangesAndBrokenCursors(t *testing.T) {
	for _, mode := range []string{"version", "incarnation", "empty-cursor", "repeated-cursor", "thread-version"} {
		t.Run(mode, func(t *testing.T) {
			calls := 0
			svc := &mockHistoryBackend{conversation: &app.Conversation{}}
			svc.readWindow = func(q database.MessageWindowQuery) (*chat.TimelineWindowSnapshot, error) {
				calls++
				if calls > 2 {
					t.Fatal("leitura não encerrou após recusa")
				}
				snapshot := &chat.TimelineWindowSnapshot{Version: database.ChatTimelineVersion{Incarnation: "a", Revision: 1}, AfterCursor: "next"}
				snapshot.Window.HasAfter = true
				if mode == "empty-cursor" {
					snapshot.AfterCursor = ""
				}
				if calls == 1 && mode == "thread-version" {
					snapshot.Items = []*chat.TimelineItem{{Representative: &database.ChatMessage{UUIDModel: database.UUIDModel{ID: "p"}, Presentation: &database.MessagePresentation{ChildCount: 1}}}}
				}
				if calls == 2 {
					snapshot.Items = []*chat.TimelineItem{{Segments: []chat.ResponseSegment{{Kind: "text", Text: "conteúdo divergente"}}}}
					switch mode {
					case "version", "thread-version":
						snapshot.Version.Revision = 2
					case "incarnation":
						snapshot.Version.Incarnation = "b"
					}
				}
				return snapshot, nil
			}
			var out bytes.Buffer
			err := runHistoryShow(svc, &out, "c")
			if err == nil {
				t.Fatal("histórico inconsistente aceito")
			}
			if strings.Contains(out.String(), "conteúdo divergente") {
				t.Fatal("página divergente foi impressa")
			}
		})
	}
}

type historyFailingWriter struct{ err error }

func (w historyFailingWriter) Write([]byte) (int, error) { return 0, w.err }

func TestHistoryShowEmptyPropagatesOutputFailure(t *testing.T) {
	expected := errors.New("saída indisponível")
	backend := &mockHistoryBackend{conversation: &app.Conversation{Title: "vazia"}}
	if err := runHistoryShow(backend, historyFailingWriter{expected}, "empty"); !errors.Is(err, expected) {
		t.Fatalf("falha de saída perdida: %v", err)
	}
}

func TestHistoryShowIncludesIntermediateThreads(t *testing.T) {
	turn := "q"
	messages := []chat.Message{
		{UUIDModel: database.UUIDModel{ID: "intermediate"}, ConversationID: "c", TurnID: &turn, Role: "assistant", Content: "parcial", Presentation: &database.MessagePresentation{ChildCount: 1}},
		{UUIDModel: database.UUIDModel{ID: "final"}, ConversationID: "c", TurnID: &turn, Role: "assistant", Content: "final", Presentation: &database.MessagePresentation{ChildCount: 0}},
	}
	response, err := chat.ProjectPersistedResponse("c", turn, nil, messages, nil, &database.ChatTurn{UserMessageID: turn, ConversationID: "c", ResponseMessageID: &messages[1].ID})
	if err != nil {
		t.Fatal(err)
	}
	reads := 0
	backend := &mockHistoryBackend{conversation: &app.Conversation{}}
	backend.readWindow = func(q database.MessageWindowQuery) (*chat.TimelineWindowSnapshot, error) {
		reads++
		if q.ParentID == nil {
			return &chat.TimelineWindowSnapshot{Items: []*chat.TimelineItem{response}}, nil
		}
		if *q.ParentID != "intermediate" {
			t.Fatal("consulta desnecessária ou alvo incorreto", *q.ParentID)
		}
		return &chat.TimelineWindowSnapshot{Items: []*chat.TimelineItem{{Segments: []chat.ResponseSegment{{Kind: "text", Text: "thread intermediária"}}}}}, nil
	}
	var out bytes.Buffer
	if err := runHistoryShow(backend, &out, "c"); err != nil {
		t.Fatal(err)
	}
	if reads != 2 || strings.Count(out.String(), "thread intermediária") != 1 {
		t.Fatal("thread perdida ou repetida", reads, out.String())
	}
}

func TestHistoryShowPreservesTextSourceBoundaries(t *testing.T) {
	backend := &mockHistoryBackend{conversation: &app.Conversation{}, messages: []*chat.TimelineItem{{Segments: []chat.ResponseSegment{
		{Kind: "text", SourceMessageID: "intermediate", Text: "Vou "},
		{Kind: "text", SourceMessageID: "intermediate", Text: "consultar."},
		{Kind: "tool_invocations"},
		{Kind: "reasoning", SourceMessageID: "final", Text: "raciocínio"},
		{Kind: "text", SourceMessageID: "final", Text: "Concluído."},
	}}}}
	var out bytes.Buffer
	if err := runHistoryShow(backend, &out, "c"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "Vou consultar.\n\nConcluído.") || strings.Contains(out.String(), "raciocínio") {
		t.Fatal("limites de fontes alterados", out.String())
	}
}
