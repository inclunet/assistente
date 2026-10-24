package agent

import (
	"context"
	"encoding/json"
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

func (m msgRepoStub) UpdateMessageContentAndReasoning(context.Context, string, string, string, int, int, int, string) error {
	return nil
}

func (m msgRepoStub) GetMessage(_ context.Context, messageID string) (*chat.Message, error) {
	return &chat.Message{UUIDModel: database.UUIDModel{ID: messageID}, ConversationID: m.conversationID}, nil
}

func (m msgRepoStub) GetMessages(context.Context, string, *string) ([]chat.Message, error) {
	return nil, nil
}

func (m msgRepoStub) GetMessagesByTurnID(context.Context, string, *string, string, int) ([]chat.Message, error) {
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

func (m msgRepoStub) SearchMessages(context.Context, string, int) ([]chat.MessageSearchResult, error) {
	return nil, nil
}

type capturingMsgRepo struct {
	conversationID string
}

func (m *capturingMsgRepo) CreateMessage(context.Context, chat.MessageOptions) (*chat.Message, error) {
	return &chat.Message{UUIDModel: database.UUIDModel{ID: "m"}}, nil
}

func (m *capturingMsgRepo) UpdateMessageContentAndReasoning(context.Context, string, string, string, int, int, int, string) error {
	return nil
}

func (m *capturingMsgRepo) GetMessage(_ context.Context, messageID string) (*chat.Message, error) {
	return &chat.Message{UUIDModel: database.UUIDModel{ID: messageID}, ConversationID: m.conversationID}, nil
}

func (m *capturingMsgRepo) GetMessages(context.Context, string, *string) ([]chat.Message, error) {
	return nil, nil
}

func (m *capturingMsgRepo) GetMessagesByTurnID(context.Context, string, *string, string, int) ([]chat.Message, error) {
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

func (m *capturingMsgRepo) SearchMessages(context.Context, string, int) ([]chat.MessageSearchResult, error) {
	return nil, nil
}

func TestPersistNativeMCPCalls_RecordsToolInvocation(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(
		&database.User{}, &database.Conversation{}, &database.ChatMessage{},
		&database.MCPServer{}, &database.ToolCatalog{}, &database.ToolInvocation{},
	); err != nil {
		t.Fatalf("automigrate: %v", err)
	}
	prev := database.DB()
	database.SetDB(db)
	t.Cleanup(func() { database.SetDB(prev) })

	userCtx := database.WithUserID(context.Background(), "user-mcp")
	if err := db.Create(&database.Conversation{
		UUIDModel: database.UUIDModel{ID: "conv-1"},
		UserID:    "user-mcp",
		Title:     "Teste",
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&database.ChatMessage{
		UUIDModel:      database.UUIDModel{ID: "turn-1"},
		ConversationID: "conv-1",
		Role:           "assistant",
	}).Error; err != nil {
		t.Fatal(err)
	}

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
		{ID: "call-2", Name: "unknown", ServerLabel: "Missing Server", Arguments: `{}`, Output: "ok", IsCompleted: true},
		{ID: "call-3", Name: "unknown", ServerLabel: "Missing Server", Arguments: `{}`, Output: "saída original do proxy", Error: "contrato incompatível", IsCompleted: true},
		{ID: "call-4", Name: "unknown", ServerLabel: "Missing Server", Arguments: `{}`, Error: "contrato incompatível", IsCompleted: true},
	}, 0)

	rows, err := repo.List(userCtx, toolinvocations.Filter{OriginType: toolinvocations.OriginChat, OriginID: "turn-1", Limit: 10})
	if err != nil {
		t.Fatalf("list invocations: %v", err)
	}
	if len(rows) != 4 {
		t.Fatalf("expected 4 invocations, got %d", len(rows))
	}
	for _, row := range rows {
		if row.ConversationID != "conv-1" || row.TurnID != "turn-1" ||
			row.InputHash == "" || row.OutputHash == "" {
			t.Fatalf("invocação MCP sem vínculo/projeção: %+v", row)
		}
		if row.ToolCallID == "call-3" || row.ToolCallID == "call-4" {
			stored, err := repo.Get(userCtx, row.ID)
			if err != nil {
				t.Fatal(err)
			}
			var result tools.ToolResult
			if err := json.Unmarshal(stored.Output, &result); err != nil {
				t.Fatal(err)
			}
			wantContent := ""
			if row.ToolCallID == "call-3" {
				wantContent = "saída original do proxy"
			}
			if result.Content != wantContent || !result.IsError || stored.Status != toolinvocations.StatusFailed ||
				stored.ErrorMessage != "contrato incompatível" || stored.ErrorKind != string(tools.ErrorKindUnknown) || stored.Retryable {
				t.Fatalf("saída/diagnóstico persistidos incorretos: result=%+v row=%+v", result, stored)
			}
		}
	}
	var archivalCount int64
	if err := db.Model(&database.ToolCatalog{}).
		Where("user_id = ? AND origin = ?", "user-mcp", toolinvocations.ToolOriginArchival).
		Count(&archivalCount).Error; err != nil {
		t.Fatal(err)
	}
	if archivalCount != 1 {
		t.Fatalf("catálogo archival MCP=%d, esperado 1", archivalCount)
	}
	displays, err := toolinvocations.LoadChatToolInvocationDisplaysForTurnIDsWithUser(userCtx, "user-mcp", []string{"turn-1"})
	if err != nil {
		t.Fatal(err)
	}
	checkedErrors := 0
	for _, display := range displays["turn-1"] {
		if display.ID != "call-3" && display.ID != "call-4" {
			continue
		}
		checkedErrors++
		var projected struct {
			Output string `json:"output"`
			Error  string `json:"error"`
		}
		if err := json.Unmarshal([]byte(display.ModelResult), &projected); err != nil {
			t.Fatal(err)
		}
		wantContent := ""
		if display.ID == "call-3" {
			wantContent = "saída original do proxy"
		}
		if display.Result != wantContent || projected.Output != wantContent || projected.Error != "contrato incompatível" {
			t.Fatalf("diagnóstico/saída perdidos na reidratação: %+v", display)
		}
	}
	if checkedErrors != 2 {
		t.Fatalf("erros não reidratados: %d", checkedErrors)
	}
}

func TestPersistNativeMCPCalls_SemLedgerNaoCriaFallbackEmMensagem(t *testing.T) {
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

}
