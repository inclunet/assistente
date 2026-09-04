package history

import (
	"context"
	"encoding/json"
	"testing"

	"assistente/internal/database"
	"assistente/internal/tools/invocationctx"
)

type searchRepoSpy struct {
	globalCalls    int
	scopedCalls    int
	conversationID string
}

func (r *searchRepoSpy) SearchMessages(_ context.Context, _ string, _ int) ([]database.MessageSearchResult, error) {
	r.globalCalls++
	return nil, nil
}

func (r *searchRepoSpy) SearchMessagesInConversation(_ context.Context, _, conversationID string, _ int) ([]database.MessageSearchResult, error) {
	r.scopedCalls++
	r.conversationID = conversationID
	return nil, nil
}

func TestSearchConversations_Name(t *testing.T) {
	tool := NewSearchConversationsForTest(nil)
	if tool.Name() != "search_conversations" {
		t.Errorf("expected 'search_conversations', got '%s'", tool.Name())
	}
}

func TestSearchConversations_Description(t *testing.T) {
	tool := NewSearchConversationsForTest(nil)
	desc := tool.Description()
	if desc == "" {
		t.Error("description should not be empty")
	}
}

func TestSearchConversations_Parameters(t *testing.T) {
	tool := NewSearchConversationsForTest(nil)
	var schema map[string]interface{}
	if err := json.Unmarshal(tool.Parameters(), &schema); err != nil {
		t.Fatalf("Parameters() must return valid JSON: %v", err)
	}
	if schema["type"] != "object" {
		t.Error("schema must have type=object")
	}
	props := schema["properties"].(map[string]interface{})
	if _, ok := props["query"]; !ok {
		t.Error("schema must have 'query' property")
	}
	if _, ok := props["conversation_id"]; !ok {
		t.Error("schema must have optional 'conversation_id' property")
	}
	required := schema["required"].([]interface{})
	found := false
	for _, r := range required {
		if r == "query" {
			found = true
		}
	}
	if !found {
		t.Error("'query' should be required")
	}
	for _, r := range required {
		if r == "conversation_id" {
			t.Error("'conversation_id' must remain optional for backwards compatibility")
		}
	}
}

func TestSearchConversations_OmittedConversationKeepsGlobalSearch(t *testing.T) {
	repo := &searchRepoSpy{}
	tool := NewSearchConversations(repo)

	result, err := tool.Execute(context.Background(), json.RawMessage(`{"query":"token"}`))
	if err != nil || result.IsError {
		t.Fatalf("global search failed: result=%#v err=%v", result, err)
	}
	if repo.globalCalls != 1 || repo.scopedCalls != 0 {
		t.Fatalf("expected global search only, got global=%d scoped=%d", repo.globalCalls, repo.scopedCalls)
	}
	if _, exists := result.Metadata["conversation_id"]; exists {
		t.Fatal("global result metadata must remain backwards compatible")
	}
}

func TestSearchConversations_ExplicitConversationUsesScopedSearch(t *testing.T) {
	repo := &searchRepoSpy{}
	tool := NewSearchConversations(repo)

	result, err := tool.Execute(context.Background(), json.RawMessage(`{"query":"token","conversation_id":"conv-123"}`))
	if err != nil || result.IsError {
		t.Fatalf("scoped search failed: result=%#v err=%v", result, err)
	}
	if repo.globalCalls != 0 || repo.scopedCalls != 1 || repo.conversationID != "conv-123" {
		t.Fatalf("unexpected calls: global=%d scoped=%d conversation=%q", repo.globalCalls, repo.scopedCalls, repo.conversationID)
	}
}

func TestSearchConversations_CurrentConversationUsesInvocationContext(t *testing.T) {
	repo := &searchRepoSpy{}
	tool := NewSearchConversations(repo)
	ctx := invocationctx.With(context.Background(), invocationctx.InvocationContext{ConversationID: "conv-current"})

	result, err := tool.Execute(ctx, json.RawMessage(`{"query":"token","conversation_id":"current"}`))
	if err != nil || result.IsError {
		t.Fatalf("current conversation search failed: result=%#v err=%v", result, err)
	}
	if repo.scopedCalls != 1 || repo.conversationID != "conv-current" {
		t.Fatalf("expected current conversation, got calls=%d conversation=%q", repo.scopedCalls, repo.conversationID)
	}
}

func TestSearchConversations_CurrentConversationFailsClosedWithoutInvocationContext(t *testing.T) {
	repo := &searchRepoSpy{}
	tool := NewSearchConversations(repo)

	result, err := tool.Execute(context.Background(), json.RawMessage(`{"query":"token","conversation_id":"current"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("conversation_id=current without invocation context should fail closed")
	}
	if repo.globalCalls != 0 || repo.scopedCalls != 0 {
		t.Fatalf("repository must not be called, got global=%d scoped=%d", repo.globalCalls, repo.scopedCalls)
	}
}

func TestSearchConversations_EmptyQuery(t *testing.T) {
	tool := NewSearchConversationsForTest(nil)
	args := `{"query": ""}`
	result, err := tool.Execute(context.Background(), json.RawMessage(args))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("empty query should return error result")
	}
}

func TestSearchConversations_InvalidArgs(t *testing.T) {
	tool := NewSearchConversationsForTest(nil)
	args := `{invalid`
	result, err := tool.Execute(context.Background(), json.RawMessage(args))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("invalid args should return error result")
	}
}

// TestSearchConversations_ProductionConstructorRejectsNilRepo garante que
// o construtor de produção não aceita repo == nil. Antes do B13 do AEP-0052
// passar nil silenciosamente caía no fallback database.* fail-open — agora
// é panic explícito no wiring, antes do agente ser registrado.
func TestSearchConversations_ProductionConstructorRejectsNilRepo(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("NewSearchConversations(nil) deveria entrar em panic")
		}
	}()
	_ = NewSearchConversations(nil)
}

// TestSearchConversations_FallbackRejectsContextWithoutUserID confirma que,
// mesmo no caminho de fallback (NewSearchConversationsForTest com repo nil),
// o tool rejeita ctx sem userID antes de chegar ao banco. Defesa em camadas
// para o caso (improvável) de um teste ou bug de wiring chamar Execute com
// ctx pelado em produção.
func TestSearchConversations_FallbackRejectsContextWithoutUserID(t *testing.T) {
	tool := NewSearchConversationsForTest(nil)
	args := `{"query": "qualquer coisa"}`
	result, err := tool.Execute(context.Background(), json.RawMessage(args))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("ctx sem userID no fallback deveria retornar IsError")
	}
}
