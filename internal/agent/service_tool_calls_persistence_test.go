package agent

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

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
func (h *testIterationHandler) OnFinishReason(info llm.FinishInfo) {
	h.res.Finish = info
}
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
	assistantToolCount int
	toolResultCount    int
	lastToolResultCall string
	lastToolResult     string
}

func (m *toolMsgRepo) GetMessage(_ context.Context, messageID string) (*chat.Message, error) {
	return &chat.Message{UUIDModel: database.UUIDModel{ID: messageID}, ConversationID: m.conversationID}, nil
}

func (m *toolMsgRepo) AddAssistantToolMessage(ctx context.Context, conversationID, turnID string, content, toolCalls, reasoning, model string) (*chat.Message, error) {
	m.assistantToolCount++
	if m.assistantErr != nil {
		return nil, m.assistantErr
	}
	return m.mockMsgRepo.AddAssistantToolMessage(ctx, conversationID, turnID, content, toolCalls, reasoning, model)
}

func (m *toolMsgRepo) AddToolResultMessage(_ context.Context, _ string, _ string, content string, toolCallID string) (*chat.Message, error) {
	m.toolResultCount++
	m.lastToolResultCall = toolCallID
	m.lastToolResult = content
	return &chat.Message{UUIDModel: database.UUIDModel{ID: "tool-1"}, Role: "tool", ToolCallID: toolCallID}, nil
}

type okTool struct{}

func (okTool) Name() string                { return "ok_tool" }
func (okTool) Description() string         { return "ok" }
func (okTool) Parameters() json.RawMessage { return json.RawMessage(`{"type":"object"}`) }
func (okTool) Execute(context.Context, json.RawMessage) (tools.ToolResult, error) {
	return tools.ToolResult{Content: "ok"}, nil
}

type largeTextTool struct{}

func (largeTextTool) Name() string                { return "large_text_tool" }
func (largeTextTool) Description() string         { return "large" }
func (largeTextTool) Parameters() json.RawMessage { return json.RawMessage(`{"type":"object"}`) }
func (largeTextTool) Execute(context.Context, json.RawMessage) (tools.ToolResult, error) {
	return tools.ToolResult{Content: strings.Repeat("resultado-", 2_000)}, nil
}

func setupAgenticToolCallDB(t *testing.T) (*gorm.DB, func()) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("sql db: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)
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
	if msgRepo.assistantToolCount != 0 {
		t.Fatalf("iteração só com tools não deve persistir marcador assistant, count=%d", msgRepo.assistantToolCount)
	}
	rows, err := repo.List(ctx, toolinvocations.Filter{OriginType: toolinvocations.OriginChat, OriginID: turn.ID, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("invocação não foi ligada ao marcador da chamada do modelo: %+v", rows)
	}
	if strings.Contains(string(rows[0].Metadata), "assistant_message_id") {
		t.Fatalf("invocação sem texto ainda depende de marcador assistant: %s", rows[0].Metadata)
	}
}

func TestRunAgenticLoop_ToolCalls_NoFallbackRoleToolWhenInvocationPersistenceSucceeds(t *testing.T) {
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

	if msgRepo.toolResultCount != 0 {
		t.Fatalf("expected no fallback role=tool message when tool_invocations persisted, got=%d", msgRepo.toolResultCount)
	}
	rows, err := repo.List(ctx, toolinvocations.Filter{OriginType: toolinvocations.OriginChat, OriginID: turn.ID, Limit: 10})
	if err != nil {
		t.Fatalf("list invocations: %v", err)
	}
	if len(rows) != 1 || rows[0].ToolCallID != "call-1" {
		t.Fatalf("expected persisted invocation for call-1, got %+v", rows)
	}
}

func TestRunAgenticLoop_ToolCalls_CriaCatalogoArchivalSemFallbackRoleTool(t *testing.T) {
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

	// Não semeia tool_catalog: o runtime deve criar entrada archival e manter
	// tool_invocations como única escrita técnica.
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

	if msgRepo.toolResultCount != 0 {
		t.Fatalf("não deveria persistir fallback role=tool, got=%d", msgRepo.toolResultCount)
	}
	var count int64
	if err := database.DB().Model(&database.ToolInvocation{}).Where("user_id = ? AND tool_call_id = ?", "user-1", "call-1").Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("invocação archival=%d, esperado 1", count)
	}
}

func TestRunAgenticLoop_ToolCalls_ResultLargeFicaSomenteNoLedger(t *testing.T) {
	_, cleanup := setupAgenticToolCallDB(t)
	t.Cleanup(cleanup)

	ctx := database.WithUserID(context.Background(), "user-large-fallback")
	conv, err := database.CreateConversationWithContext(ctx, "t", "")
	if err != nil {
		t.Fatalf("create conv: %v", err)
	}
	turn, err := database.AddMessageWithContext(ctx, conv.ID, "user", "hi")
	if err != nil {
		t.Fatalf("create turn msg: %v", err)
	}

	// Sem catálogo, cria entrada archival; o resultado limitado continua apenas
	// no ledger.
	repo := toolinvocations.NewDBRepository(database.DB())
	reg := tools.NewRegistry()
	reg.MustRegister(largeTextTool{})
	cfg := tools.DefaultExecutorConfig()
	cfg.MaxResultSize = 1024
	exec := tools.NewExecutor(reg, cfg)
	inv := toolinvocations.NewService(repo, exec)

	msgRepo := &toolMsgRepo{conversationID: conv.ID}
	svc := NewService(ServiceConfig{
		Emitter: events.NoopEmitter{}, MsgRepo: msgRepo,
		ToolExecutor: exec, ToolInvocations: inv,
	})
	streamer := &scriptedStreamer{call: llm.ToolCall{
		ID: "call-large", Type: "function",
		Function: llm.FunctionCall{Name: "large_text_tool", Arguments: `{}`},
	}}
	svc.RunAgenticLoop(ctx, []llm.Message{{Role: "user", Content: "hi"}},
		llm.ChatParams{MaxAgenticIterations: 2}, conv.ID, turn.ID, nil, streamer, nil,
		func(string, int) IterationHandler { return &testIterationHandler{} },
		nil, false, 0)

	if msgRepo.toolResultCount != 0 {
		t.Fatalf("não deveria persistir fallback role=tool, got=%d", msgRepo.toolResultCount)
	}
	var invocation database.ToolInvocation
	if err := database.DB().Where("user_id = ? AND tool_call_id = ?", "user-large-fallback", "call-large").First(&invocation).Error; err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(invocation.Output, "result_omitted_for_persistence") ||
		strings.Contains(invocation.Output, `"result_id"`) ||
		strings.Contains(invocation.Output, "resultado-resultado-") {
		t.Fatalf("ledger persistiu prévia ou ID efêmero: %q", invocation.Output)
	}
	if len(invocation.Output) > cfg.MaxResultSize*2 {
		t.Fatalf("ledger persistiu envelope muito acima do limite: %d", len(invocation.Output))
	}
}

func TestTagChatToolInvocationsWithAssistantMessage_SkipsAlreadyTaggedMetadata(t *testing.T) {
	db, cleanup := setupAgenticToolCallDB(t)
	t.Cleanup(cleanup)

	ctx := database.WithUserID(context.Background(), "user-1")
	tool := database.ToolCatalog{
		Name:               "ok_tool",
		DisplayName:        "ok_tool",
		Origin:             tools.ToolOriginBuiltin,
		AvailabilityStatus: tools.ToolAvailabilityAvailable,
	}
	if err := db.Create(&tool).Error; err != nil {
		t.Fatalf("seed tool catalog: %v", err)
	}

	originalMetadata := `{"extra":true,"display":{"version":1,"assistant_message_id":"assistant-1"}}`
	invocation := database.ToolInvocation{
		UserID:        "user-1",
		ToolCatalogID: tool.ID,
		OriginType:    toolinvocations.OriginChat,
		OriginID:      "turn-1",
		ToolCallID:    "call-1",
		Status:        toolinvocations.StatusSucceeded,
		Metadata:      originalMetadata,
		QueuedAt:      time.Now(),
	}
	if err := db.Create(&invocation).Error; err != nil {
		t.Fatalf("seed invocation: %v", err)
	}

	svc := NewService(ServiceConfig{})
	svc.tagChatToolInvocationsWithAssistantMessage(ctx, "turn-1", []tools.ToolExecutionResult{{CallID: "call-1"}}, "assistant-1")

	var got database.ToolInvocation
	if err := db.First(&got, "id = ?", invocation.ID).Error; err != nil {
		t.Fatalf("load invocation: %v", err)
	}
	if got.Metadata != originalMetadata {
		t.Fatalf("expected metadata to remain untouched, got %s", got.Metadata)
	}
}
