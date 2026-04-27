package main

import (
	"bytes"
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
	messages      []chat.MessageNode
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

func (m *mockHistoryBackend) GetMessages(conversationID string, parentID *string) ([]chat.MessageNode, error) {
	return m.messages, m.messagesErr
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
		messages: []chat.MessageNode{
			{
				Message: chat.EnrichedMessage{Role: "user", Content: "olá"},
			},
			{
				Message: chat.EnrichedMessage{Role: "assistant", Content: "oi!"},
			},
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
		messages: []chat.MessageNode{
			{
				Message: chat.EnrichedMessage{Role: "user", Content: "pergunta"},
				Children: []chat.MessageNode{
					{Message: chat.EnrichedMessage{Role: "assistant", Content: "resposta"}},
				},
			},
		},
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
