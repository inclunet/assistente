package history

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"assistente/internal/database"
	"assistente/internal/tools/invocationctx"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

const itUserID = "conv-info-user"

// setupConvInfoDB sobe um SQLite em memória e o injeta no pacote database via
// SetDB, migrando apenas as tabelas que get_conversation_info consulta. Restaura
// o estado (db=nil) no cleanup para não vazar entre os testes do pacote.
func setupConvInfoDB(t *testing.T) {
	t.Helper()
	gdb, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}
	if err := gdb.AutoMigrate(
		&database.Conversation{},
		&database.ChatMessage{},
		&database.TaskListWorkflow{},
		&database.TaskList{},
		&database.Task{},
	); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}
	database.SetDB(gdb)
	t.Cleanup(func() {
		if sqlDB, err := gdb.DB(); err == nil && sqlDB != nil {
			_ = sqlDB.Close()
		}
		database.SetDB(nil)
	})
}

func itCtx(convID string) context.Context {
	ctx := database.WithUserID(context.Background(), itUserID)
	return invocationctx.With(ctx, invocationctx.InvocationContext{ConversationID: convID})
}

func seedConversation(t *testing.T, title string) string {
	t.Helper()
	conv := &database.Conversation{Title: title, UserID: itUserID}
	if err := database.DB().Create(conv).Error; err != nil {
		t.Fatalf("failed to create conversation: %v", err)
	}
	return conv.ID
}

func seedRootMessage(t *testing.T, convID, role, content string) {
	t.Helper()
	msg := &database.ChatMessage{ConversationID: convID, Role: role, Content: content}
	if err := database.DB().Create(msg).Error; err != nil {
		t.Fatalf("failed to create message: %v", err)
	}
}

func TestGetConversationInfo_Integration_FullPayload(t *testing.T) {
	setupConvInfoDB(t)
	tool := NewGetConversationInfo()

	convID := seedConversation(t, "Suporte do cliente X")
	seedRootMessage(t, convID, "user", "tenho um problema")
	seedRootMessage(t, convID, "assistant", "vamos investigar")

	if err := database.UpdateConversationSummaryWithContext(itCtx(convID), convID, "Resumo da conversa", ""); err != nil {
		t.Fatalf("failed to set summary: %v", err)
	}

	// Lista e task vinculadas à conversa.
	tl := &database.TaskList{Title: "Plano de ação", UserID: itUserID, ConversationID: &convID}
	if err := database.DB().Create(tl).Error; err != nil {
		t.Fatalf("failed to create task list: %v", err)
	}
	tk := &database.Task{TaskListID: tl.ID, Title: "Passo 1", StatusID: 1, ConversationID: &convID}
	if err := database.DB().Create(tk).Error; err != nil {
		t.Fatalf("failed to create task: %v", err)
	}

	// Sem include_messages: deve trazer metadados, summary e vínculos, mas não mensagens.
	res, err := tool.Execute(itCtx(convID), json.RawMessage(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError {
		t.Fatalf("unexpected error: %s", res.Content)
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(res.Content), &payload); err != nil {
		t.Fatalf("content should be valid JSON: %v\n%s", err, res.Content)
	}
	if payload["conversation_id"] != convID {
		t.Errorf("expected conversation_id %s, got %v", convID, payload["conversation_id"])
	}
	if payload["title"] != "Suporte do cliente X" {
		t.Errorf("expected title, got %v", payload["title"])
	}
	if payload["summary"] != "Resumo da conversa" {
		t.Errorf("expected summary, got %v", payload["summary"])
	}
	if _, ok := payload["linked_task_lists"]; !ok {
		t.Errorf("expected linked_task_lists in payload: %s", res.Content)
	}
	if _, ok := payload["linked_tasks"]; !ok {
		t.Errorf("expected linked_tasks in payload: %s", res.Content)
	}
	if _, ok := payload["recent_messages"]; ok {
		t.Errorf("recent_messages should be absent when include_messages is false: %s", res.Content)
	}
	if md := res.Metadata; md == nil || md["conversation_id"] != convID {
		t.Errorf("expected metadata conversation_id %s, got %v", convID, res.Metadata)
	}
}

func TestGetConversationInfo_Integration_IncludeMessages(t *testing.T) {
	setupConvInfoDB(t)
	tool := NewGetConversationInfo()

	convID := seedConversation(t, "Com mensagens")
	seedRootMessage(t, convID, "user", "primeira")
	seedRootMessage(t, convID, "assistant", "segunda")

	res, err := tool.Execute(itCtx(convID), json.RawMessage(`{"include_messages": true, "message_limit": 5}`))
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError {
		t.Fatalf("unexpected error: %s", res.Content)
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(res.Content), &payload); err != nil {
		t.Fatalf("content should be valid JSON: %v", err)
	}
	msgs, ok := payload["recent_messages"].([]any)
	if !ok {
		t.Fatalf("expected recent_messages array, got: %s", res.Content)
	}
	if len(msgs) != 2 {
		t.Errorf("expected 2 recent messages, got %d", len(msgs))
	}
}

// Com conversation_id explícito a tool não depende do InvocationContext.
func TestGetConversationInfo_Integration_ExplicitID(t *testing.T) {
	setupConvInfoDB(t)
	tool := NewGetConversationInfo()

	convID := seedConversation(t, "Explícita")

	ctx := database.WithUserID(context.Background(), itUserID)
	args, _ := json.Marshal(map[string]any{"conversation_id": convID})
	res, err := tool.Execute(ctx, args)
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError {
		t.Fatalf("unexpected error: %s", res.Content)
	}
	if !strings.Contains(res.Content, convID) {
		t.Errorf("expected conversation_id in content, got: %s", res.Content)
	}
}

func TestGetConversationInfo_Integration_NotFound(t *testing.T) {
	setupConvInfoDB(t)
	tool := NewGetConversationInfo()

	res, err := tool.Execute(itCtx("does-not-exist"), json.RawMessage(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	if !res.IsError {
		t.Fatalf("expected IsError for non-existent conversation, got: %s", res.Content)
	}
}

// Conversa de outro usuário não deve ser acessível (escopo por user_id).
func TestGetConversationInfo_Integration_CrossUserDenied(t *testing.T) {
	setupConvInfoDB(t)
	tool := NewGetConversationInfo()

	other := &database.Conversation{Title: "De outro usuário", UserID: "someone-else"}
	if err := database.DB().Create(other).Error; err != nil {
		t.Fatalf("failed to create conversation: %v", err)
	}

	res, err := tool.Execute(itCtx(other.ID), json.RawMessage(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	if !res.IsError {
		t.Fatalf("expected IsError accessing another user's conversation, got: %s", res.Content)
	}
}
