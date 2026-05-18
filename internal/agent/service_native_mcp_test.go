package agent

import (
	"context"
	"strings"
	"testing"

	"assistente/internal/chat"
	"assistente/internal/database"
	"assistente/internal/llm"
	"assistente/internal/toolinvocations"
	"assistente/internal/tools"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

type msgRepoStub struct {
	conversationID string
}

func (m msgRepoStub) CreateMessage(context.Context, chat.MessageOptions) (*chat.Message, error) {
	return &chat.Message{UUIDModel: database.UUIDModel{ID: "m"}}, nil
}

func (m msgRepoStub) GetMessage(_ context.Context, messageID string) (*chat.Message, error) {
	return &chat.Message{UUIDModel: database.UUIDModel{ID: messageID}, ConversationID: m.conversationID}, nil
}

func (m msgRepoStub) GetMessages(context.Context, string, *string) ([]chat.Message, error) {
	return nil, nil
}

func (m msgRepoStub) GetConversationSummary(context.Context, string) (string, string, error) {
	return "", "", nil
}

func (m msgRepoStub) GetDetailedTokenStats(context.Context, string, string) (*chat.DetailedTokenStats, error) {
	return nil, nil
}

func (m msgRepoStub) GetContextWindowUsage(context.Context, string, int) (float64, int, error) {
	return 0, 0, nil
}

func (m msgRepoStub) GetRecentMessagesTokenCount(context.Context, string, int) (int, error) {
	return 0, nil
}

func (m msgRepoStub) GetTurnTokenStats(context.Context, string, string) (*database.TokenStats, error) {
	return nil, nil
}

func (msgRepoStub) AddAssistantToolMessage(_ context.Context, conversationID, turnID string, content, toolCalls, reasoning, model string) (*chat.Message, error) {
	return &chat.Message{UUIDModel: database.UUIDModel{ID: "m"}, Role: "assistant", Content: content}, nil
}

func (msgRepoStub) AddToolResultMessage(context.Context, string, string, string, string) (*chat.Message, error) {
	return nil, nil
}

func (m msgRepoStub) SearchMessages(context.Context, string, int) ([]chat.MessageSearchResult, error) {
	return nil, nil
}

type capturingMsgRepo struct {
	conversationID string
	lastContent  string
	lastToolCall string
}

func (m *capturingMsgRepo) CreateMessage(context.Context, chat.MessageOptions) (*chat.Message, error) {
	return &chat.Message{UUIDModel: database.UUIDModel{ID: "m"}}, nil
}

func (m *capturingMsgRepo) GetMessage(_ context.Context, messageID string) (*chat.Message, error) {
	return &chat.Message{UUIDModel: database.UUIDModel{ID: messageID}, ConversationID: m.conversationID}, nil
}

func (m *capturingMsgRepo) GetMessages(context.Context, string, *string) ([]chat.Message, error) {
	return nil, nil
}

func (m *capturingMsgRepo) GetConversationSummary(context.Context, string) (string, string, error) {
	return "", "", nil
}

func (m *capturingMsgRepo) GetDetailedTokenStats(context.Context, string, string) (*chat.DetailedTokenStats, error) {
	return nil, nil
}

func (m *capturingMsgRepo) GetContextWindowUsage(context.Context, string, int) (float64, int, error) {
	return 0, 0, nil
}

func (m *capturingMsgRepo) GetRecentMessagesTokenCount(context.Context, string, int) (int, error) {
	return 0, nil
}

func (m *capturingMsgRepo) GetTurnTokenStats(context.Context, string, string) (*database.TokenStats, error) {
	return nil, nil
}

func (m *capturingMsgRepo) AddAssistantToolMessage(_ context.Context, conversationID, turnID string, content, toolCalls, reasoning, model string) (*chat.Message, error) {
	return &chat.Message{UUIDModel: database.UUIDModel{ID: "m"}, Role: "assistant", Content: content}, nil
}

func (m *capturingMsgRepo) AddToolResultMessage(_ context.Context, conversationID, turnID string, content, toolCallID string) (*chat.Message, error) {
	m.lastContent = content
	m.lastToolCall = toolCallID
	return &chat.Message{UUIDModel: database.UUIDModel{ID: "t"}, Role: "tool", Content: content, ToolCallID: toolCallID}, nil
}

func (m *capturingMsgRepo) SearchMessages(context.Context, string, int) ([]chat.MessageSearchResult, error) {
	return nil, nil
}

func TestPersistNativeMCPCalls_RecordsToolInvocation(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&database.User{}, &database.MCPServer{}, &database.ToolCatalog{}, &database.ToolInvocation{}); err != nil {
		t.Fatalf("automigrate: %v", err)
	}
	prev := database.DB()
	database.SetDB(db)
	t.Cleanup(func() { database.SetDB(prev) })

	userCtx := database.WithUserID(context.Background(), "user-mcp")

	// Seed MCP server + catalog entry for the namespaced bridge tool.
	server := database.MCPServer{UserID: "user-mcp", Slug: "srv", Name: "Server One", Enabled: true}
	if err := db.Create(&server).Error; err != nil {
		t.Fatalf("seed server: %v", err)
	}
	toolName := "mcp_srv__ping"
	if err := db.Create(&database.ToolCatalog{
		UserID:             &server.UserID,
		MCPServerID:        &server.ID,
		Name:               toolName,
		DisplayName:        "Ping",
		Origin:             "mcp",
		AvailabilityStatus: tools.ToolAvailabilityAvailable,
	}).Error; err != nil {
		t.Fatalf("seed tool catalog: %v", err)
	}

	repo := toolinvocations.NewDBRepository(db)
	registry := tools.NewRegistry()
	exec := tools.NewExecutor(registry, tools.DefaultExecutorConfig())
	invSvc := toolinvocations.NewService(repo, exec)

	svc := NewService(ServiceConfig{
		MsgRepo:         msgRepoStub{conversationID: "conv-1"},
		ToolInvocations: invSvc,
	})

	svc.persistNativeMCPCalls(userCtx, "conv-1", "turn-1", []llm.MCPToolEvent{
		{ID: "call-1", Name: "ping", ServerLabel: "Server One", Arguments: `{"x":1}`, Output: "ok", IsCompleted: true},
	}, 0)

	rows, err := repo.List(userCtx, toolinvocations.Filter{OriginType: toolinvocations.OriginChat, OriginID: "turn-1", Limit: 10})
	if err != nil {
		t.Fatalf("list invocations: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 invocation, got %d", len(rows))
	}
	if rows[0].ToolCallID != "call-1" {
		t.Fatalf("unexpected tool_call_id: %s", rows[0].ToolCallID)
	}
	if rows[0].ToolCatalogID == "" {
		t.Fatalf("expected tool_catalog_id to be set")
	}
}

func TestPersistNativeMCPCalls_FallbackToolMessageMarksError(t *testing.T) {
	// Sem toolInvocations => persistNativeMCPCalls deve cair no fallback role=tool.
	msgRepo := &capturingMsgRepo{conversationID: "conv-1"}
	svc := NewService(ServiceConfig{MsgRepo: msgRepo})

	ctx := database.WithUserID(context.Background(), "user-mcp")
	svc.persistNativeMCPCalls(ctx, "conv-1", "turn-1", []llm.MCPToolEvent{{
		ID:          "call-1",
		Name:        "ping",
		ServerLabel: "Server One",
		Arguments:   `{}`,
		Output:      "",
		Error:       "boom",
		IsCompleted: true,
	}}, 0)

	if msgRepo.lastToolCall != "call-1" {
		t.Fatalf("toolCallID = %q, want call-1", msgRepo.lastToolCall)
	}
	if !strings.Contains(msgRepo.lastContent, "Error:") {
		t.Fatalf("fallback content = %q, want error marker", msgRepo.lastContent)
	}
}
