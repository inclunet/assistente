package agent

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"assistente/internal/chat"
	"assistente/internal/database"
	"assistente/internal/events"
	"assistente/internal/llm"
	"assistente/internal/toolinvocations"
	"assistente/internal/tools"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

type scriptedStreamer struct {
	step int
	call llm.ToolCall
}

func (s *scriptedStreamer) StreamChat(_ context.Context, _ []llm.Message, _ llm.ChatParams, handler llm.StreamHandler, _ ...llm.ToolDefinition) {
	if s.step == 0 {
		handler.OnToolCalls([]llm.ToolCall{s.call}, "", llm.Usage{}, "test-model")
		s.step++
		return
	}
	handler.OnDone("final", llm.Usage{}, "test-model")
	s.step++
}

type testIterationHandler struct {
	res AgenticResult
}

func (h *testIterationHandler) OnChunk(string)                  {}
func (h *testIterationHandler) OnThinking(string)               {}
func (h *testIterationHandler) OnThinkingDone(string)           {}
func (h *testIterationHandler) OnMCPToolEvent(llm.MCPToolEvent) {}
func (h *testIterationHandler) OnError(err string)              { h.res.Error = err }
func (h *testIterationHandler) OnToolCalls(calls []llm.ToolCall, fullResponse string, usage llm.Usage, model string) {
	h.res.FullResponse = fullResponse
	h.res.ToolCalls = calls
	h.res.Usage = usage
	h.res.Model = model
	h.res.IsDone = false
}
func (h *testIterationHandler) OnDone(fullResponse string, usage llm.Usage, model string) {
	h.res.FullResponse = fullResponse
	h.res.Usage = usage
	h.res.Model = model
	h.res.IsDone = true
}
func (h *testIterationHandler) Result() AgenticResult { return h.res }

type toolMsgRepo struct {
	mockMsgRepo
	conversationID     string
	assistantErr       error
	toolResultCount    int
	lastToolResultCall string
}

func (m *toolMsgRepo) GetMessage(_ context.Context, messageID string) (*chat.Message, error) {
	return &chat.Message{UUIDModel: database.UUIDModel{ID: messageID}, ConversationID: m.conversationID}, nil
}

func (m *toolMsgRepo) AddAssistantToolMessage(ctx context.Context, conversationID, turnID string, content, toolCalls, reasoning, model string) (*chat.Message, error) {
	if m.assistantErr != nil {
		return nil, m.assistantErr
	}
	return m.mockMsgRepo.AddAssistantToolMessage(ctx, conversationID, turnID, content, toolCalls, reasoning, model)
}

func (m *toolMsgRepo) AddToolResultMessage(_ context.Context, _ string, _ string, _ string, toolCallID string) (*chat.Message, error) {
	m.toolResultCount++
	m.lastToolResultCall = toolCallID
	return &chat.Message{UUIDModel: database.UUIDModel{ID: "tool-1"}, Role: "tool", ToolCallID: toolCallID}, nil
}

type okTool struct{}

func (okTool) Name() string                { return "ok_tool" }
func (okTool) Description() string         { return "ok" }
func (okTool) Parameters() json.RawMessage { return json.RawMessage(`{"type":"object"}`) }
func (okTool) Execute(context.Context, json.RawMessage) (tools.ToolResult, error) {
	return tools.ToolResult{Content: "ok"}, nil
}

func setupAgenticToolCallDB(t *testing.T) (*gorm.DB, func()) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(
		&database.User{},
		&database.Conversation{},
		&database.ChatMessage{},
		&database.ToolCatalog{},
		&database.ToolInvocation{},
	); err != nil {
		t.Fatalf("automigrate: %v", err)
	}
	prev := database.DB()
	database.SetDB(db)
	cleanup := func() { database.SetDB(prev) }
	return db, cleanup
}

func TestRunAgenticLoop_ToolCalls_SuppressesRoleToolOnSuccessfulPersistence(t *testing.T) {
	db, cleanup := setupAgenticToolCallDB(t)
	t.Cleanup(cleanup)

	ctx := database.WithUserID(context.Background(), "user-1")
	conv, err := database.CreateConversationWithContext(ctx, "t", "")
	if err != nil {
		t.Fatalf("create conv: %v", err)
	}
	turn, err := database.AddMessageWithContext(ctx, conv.ID, "user", "hi")
	if err != nil {
		t.Fatalf("create turn msg: %v", err)
	}

	// Catálogo presente => tool_invocations persiste.
	if err := db.Create(&database.ToolCatalog{
		Name:               "ok_tool",
		DisplayName:        "ok_tool",
		Origin:             tools.ToolOriginBuiltin,
		AvailabilityStatus: tools.ToolAvailabilityAvailable,
	}).Error; err != nil {
		t.Fatalf("seed tool catalog: %v", err)
	}

	repo := toolinvocations.NewDBRepository(db)
	reg := tools.NewRegistry()
	reg.MustRegister(okTool{})
	exec := tools.NewExecutor(reg, tools.DefaultExecutorConfig())
	inv := toolinvocations.NewService(repo, exec)

	msgRepo := &toolMsgRepo{conversationID: conv.ID}
	svc := NewService(ServiceConfig{
		Emitter:         events.NoopEmitter{},
		MsgRepo:         msgRepo,
		ToolExecutor:    exec,
		ToolInvocations: inv,
	})

	streamer := &scriptedStreamer{call: llm.ToolCall{ID: "call-1", Type: "function", Function: llm.FunctionCall{Name: "ok_tool", Arguments: `{}`}}}
	svc.RunAgenticLoop(ctx, []llm.Message{{Role: "user", Content: "hi"}}, llm.ChatParams{MaxAgenticIterations: 2}, conv.ID, turn.ID, nil, streamer, nil, func(string, int) IterationHandler {
		return &testIterationHandler{}
	}, nil, false, 0)

	if msgRepo.toolResultCount != 0 {
		t.Fatalf("expected no role=tool messages, got=%d", msgRepo.toolResultCount)
	}
}

func TestRunAgenticLoop_ToolCalls_FallbackRoleToolWhenAssistantToolCallsSaveFails(t *testing.T) {
	db, cleanup := setupAgenticToolCallDB(t)
	t.Cleanup(cleanup)

	ctx := database.WithUserID(context.Background(), "user-1")
	conv, err := database.CreateConversationWithContext(ctx, "t", "")
	if err != nil {
		t.Fatalf("create conv: %v", err)
	}
	turn, err := database.AddMessageWithContext(ctx, conv.ID, "user", "hi")
	if err != nil {
		t.Fatalf("create turn msg: %v", err)
	}

	if err := db.Create(&database.ToolCatalog{
		Name:               "ok_tool",
		DisplayName:        "ok_tool",
		Origin:             tools.ToolOriginBuiltin,
		AvailabilityStatus: tools.ToolAvailabilityAvailable,
	}).Error; err != nil {
		t.Fatalf("seed tool catalog: %v", err)
	}

	repo := toolinvocations.NewDBRepository(db)
	reg := tools.NewRegistry()
	reg.MustRegister(okTool{})
	exec := tools.NewExecutor(reg, tools.DefaultExecutorConfig())
	inv := toolinvocations.NewService(repo, exec)

	msgRepo := &toolMsgRepo{conversationID: conv.ID, assistantErr: errors.New("boom")}
	svc := NewService(ServiceConfig{
		Emitter:         events.NoopEmitter{},
		MsgRepo:         msgRepo,
		ToolExecutor:    exec,
		ToolInvocations: inv,
	})

	streamer := &scriptedStreamer{call: llm.ToolCall{ID: "call-1", Type: "function", Function: llm.FunctionCall{Name: "ok_tool", Arguments: `{}`}}}
	svc.RunAgenticLoop(ctx, []llm.Message{{Role: "user", Content: "hi"}}, llm.ChatParams{MaxAgenticIterations: 2}, conv.ID, turn.ID, nil, streamer, nil, func(string, int) IterationHandler {
		return &testIterationHandler{}
	}, nil, false, 0)

	if msgRepo.toolResultCount != 1 {
		t.Fatalf("expected 1 fallback role=tool message, got=%d", msgRepo.toolResultCount)
	}
	if msgRepo.lastToolResultCall != "call-1" {
		t.Fatalf("toolCallID=%q want call-1", msgRepo.lastToolResultCall)
	}
}

func TestRunAgenticLoop_ToolCalls_FallbackRoleToolWhenInvocationPersistenceFails(t *testing.T) {
	_, cleanup := setupAgenticToolCallDB(t)
	t.Cleanup(cleanup)

	ctx := database.WithUserID(context.Background(), "user-1")
	conv, err := database.CreateConversationWithContext(ctx, "t", "")
	if err != nil {
		t.Fatalf("create conv: %v", err)
	}
	turn, err := database.AddMessageWithContext(ctx, conv.ID, "user", "hi")
	if err != nil {
		t.Fatalf("create turn msg: %v", err)
	}

	// Não semeia tool_catalog => ResolveToolCatalogID falha => Persisted=false => fallback role=tool.
	repo := toolinvocations.NewDBRepository(database.DB())
	reg := tools.NewRegistry()
	reg.MustRegister(okTool{})
	exec := tools.NewExecutor(reg, tools.DefaultExecutorConfig())
	inv := toolinvocations.NewService(repo, exec)

	msgRepo := &toolMsgRepo{conversationID: conv.ID}
	svc := NewService(ServiceConfig{
		Emitter:         events.NoopEmitter{},
		MsgRepo:         msgRepo,
		ToolExecutor:    exec,
		ToolInvocations: inv,
	})

	streamer := &scriptedStreamer{call: llm.ToolCall{ID: "call-1", Type: "function", Function: llm.FunctionCall{Name: "ok_tool", Arguments: `{}`}}}
	svc.RunAgenticLoop(ctx, []llm.Message{{Role: "user", Content: "hi"}}, llm.ChatParams{MaxAgenticIterations: 2}, conv.ID, turn.ID, nil, streamer, nil, func(string, int) IterationHandler {
		return &testIterationHandler{}
	}, nil, false, 0)

	if msgRepo.toolResultCount != 1 {
		t.Fatalf("expected 1 fallback role=tool message, got=%d", msgRepo.toolResultCount)
	}
}
