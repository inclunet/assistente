package app

import (
	"bytes"
	"encoding/json"
	"log"
	"strings"
	"testing"
	"time"

	"assistente/internal/chat"
	"assistente/internal/database"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func setupMessageWindowAppTestDB(t *testing.T) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&database.Conversation{}, &database.ChatMessage{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	database.SetDB(db)
	t.Cleanup(func() {
		sqlDB, _ := db.DB()
		if sqlDB != nil {
			_ = sqlDB.Close()
		}
	})
}

func TestConsolidateTimelineTurnMessages_ToolOnlyPlaceholderOrdersToolCalls(t *testing.T) {
	turnID := "turn-1"
	consolidated := consolidateTimelineTurnMessages([]database.ChatMessage{
		{
			UUIDModel:  database.UUIDModel{ID: "tool-b-message"},
			Role:       "tool",
			Content:    "resultado b",
			TurnID:     &turnID,
			ToolCallID: "tool-b",
		},
		{
			UUIDModel:  database.UUIDModel{ID: "tool-a-message"},
			Role:       "tool",
			Content:    "resultado a",
			TurnID:     &turnID,
			ToolCallID: "tool-a",
		},
	})

	var calls []struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal([]byte(consolidated.ToolCalls), &calls); err != nil {
		t.Fatalf("unmarshal placeholder tool calls: %v", err)
	}
	if len(calls) != 2 || calls[0].ID != "tool-a" || calls[1].ID != "tool-b" {
		t.Fatalf("expected deterministic tool call order by id, got %+v", calls)
	}
}

func TestConsolidateTimelineTurnMessages_OrdersMessagesBeforeChoosingRepresentative(t *testing.T) {
	turnID := "turn-1"
	baseTime := time.Date(2026, 5, 7, 10, 0, 0, 0, time.UTC)
	consolidated := consolidateTimelineTurnMessages([]database.ChatMessage{
		{
			UUIDModel: database.UUIDModel{ID: "assistant-final", CreatedAt: baseTime.Add(2 * time.Minute)},
			Role:      "assistant",
			Content:   "resposta final",
			TurnID:    &turnID,
		},
		{
			UUIDModel: database.UUIDModel{ID: "assistant-intermediate", CreatedAt: baseTime.Add(time.Minute)},
			Role:      "assistant",
			Content:   "resposta intermediária",
			TurnID:    &turnID,
		},
		{
			UUIDModel: database.UUIDModel{ID: "assistant-first", CreatedAt: baseTime},
			Role:      "assistant",
			Content:   "primeira resposta",
			TurnID:    &turnID,
		},
	})

	if consolidated.ID != "assistant-final" {
		t.Fatalf("expected latest assistant as representative, got %s", consolidated.ID)
	}
	if consolidated.Content != "resposta final" {
		t.Fatalf("expected latest non-empty content, got %q", consolidated.Content)
	}
}

func TestConsolidateTimelineTurnMessages_DeduplicatesToolCallsByID(t *testing.T) {
	turnID := "turn-1"
	baseTime := time.Date(2026, 5, 7, 10, 0, 0, 0, time.UTC)
	consolidated := consolidateTimelineTurnMessages([]database.ChatMessage{
		{
			UUIDModel: database.UUIDModel{ID: "assistant-first", CreatedAt: baseTime},
			Role:      "assistant",
			TurnID:    &turnID,
			ToolCalls: `[{"id":"tool-1","type":"function","function":{"name":"search","arguments":"{}"}}]`,
		},
		{
			UUIDModel: database.UUIDModel{ID: "assistant-duplicate", CreatedAt: baseTime.Add(time.Minute)},
			Role:      "assistant",
			TurnID:    &turnID,
			ToolCalls: `[{"id":"tool-1","type":"function","function":{"name":"search_again","arguments":"{}"}}]`,
		},
		{
			UUIDModel:  database.UUIDModel{ID: "tool-result", CreatedAt: baseTime.Add(2 * time.Minute)},
			Role:       "tool",
			Content:    "resultado preservado",
			TurnID:     &turnID,
			ToolCallID: "tool-1",
		},
	})

	var calls []struct {
		ID       string `json:"id"`
		Function struct {
			Name string `json:"name"`
		} `json:"function"`
		Result string `json:"result"`
	}
	if err := json.Unmarshal([]byte(consolidated.ToolCalls), &calls); err != nil {
		t.Fatalf("unmarshal consolidated tool calls: %v", err)
	}
	if len(calls) != 1 {
		t.Fatalf("expected one deduplicated tool call, got %+v", calls)
	}
	if calls[0].ID != "tool-1" || calls[0].Function.Name != "search" || calls[0].Result != "resultado preservado" {
		t.Fatalf("expected first tool call enriched with result, got %+v", calls[0])
	}
}

func TestParseToolCalls_InvalidJSONReturnsNil(t *testing.T) {
	if calls := parseToolCalls("message-invalid", "{invalid"); calls != nil {
		t.Fatalf("expected invalid tool calls JSON to be discarded, got %+v", calls)
	}
}

func TestParseToolCalls_InvalidJSONLogsOncePerMessage(t *testing.T) {
	var buf bytes.Buffer
	previousWriter := log.Writer()
	log.SetOutput(&buf)
	defer log.SetOutput(previousWriter)

	messageID := "message-invalid-log-once"
	parseToolCalls(messageID, "{invalid")
	parseToolCalls(messageID, "{invalid")

	if got := strings.Count(buf.String(), messageID); got != 1 {
		t.Fatalf("expected one invalid tool_calls log for %s, got %d logs: %s", messageID, got, buf.String())
	}
}

func TestMessageTimelineItemKey_UserMessagesIgnoreTurnID(t *testing.T) {
	turnID := "turn-1"
	message := database.ChatMessage{
		UUIDModel: database.UUIDModel{ID: "user-1"},
		Role:      "user",
		TurnID:    &turnID,
	}

	if key := messageTimelineItemKey(message); key != "message:user-1" {
		t.Fatalf("expected user message key to ignore TurnID, got %q", key)
	}
}

func TestGetConversationMessageWindow_ValidatesRequestShape(t *testing.T) {
	setupMessageWindowAppTestDB(t)
	app := &App{}

	conv, err := database.CreateConversation("Conversa", "")
	if err != nil {
		t.Fatalf("create conversation: %v", err)
	}
	if _, err := database.AddMessage(conv.ID, "user", "mensagem"); err != nil {
		t.Fatalf("create message: %v", err)
	}

	_, err = app.GetConversationMessageWindow(chat.MessageWindowRequest{
		ConversationID: conv.ID,
		Scope:          chat.MessageWindowScopeConversation,
		Direction:      "sideways",
		Limit:          10,
	})
	if err == nil || !strings.Contains(err.Error(), "direction") {
		t.Fatalf("expected direction validation error, got %v", err)
	}

	_, err = app.GetConversationMessageWindow(chat.MessageWindowRequest{
		ConversationID: conv.ID,
		Scope:          chat.MessageWindowScopeConversation,
		Direction:      chat.MessageWindowDirectionAround,
		Limit:          10,
	})
	if err == nil || !strings.Contains(err.Error(), "anchorMessageId") {
		t.Fatalf("expected around anchor validation error, got %v", err)
	}
}

func TestGetConversationMessageWindow_RejectsNestedThreadParent(t *testing.T) {
	setupMessageWindowAppTestDB(t)
	app := &App{}

	conv, err := database.CreateConversation("Conversa", "")
	if err != nil {
		t.Fatalf("create conversation: %v", err)
	}
	root, err := database.AddMessage(conv.ID, "assistant", "root")
	if err != nil {
		t.Fatalf("create root: %v", err)
	}
	child, err := database.AddChildMessage(conv.ID, root.ID, "assistant", "child", "")
	if err != nil {
		t.Fatalf("create child: %v", err)
	}

	_, err = app.GetConversationMessageWindow(chat.MessageWindowRequest{
		ConversationID: conv.ID,
		Scope:          chat.MessageWindowScopeThread,
		ThreadParentID: child.ID,
		Anchor:         chat.MessageWindowAnchorStart,
		Direction:      chat.MessageWindowDirectionAfter,
		Limit:          10,
	})
	if err == nil || !strings.Contains(err.Error(), "mensagem raiz") {
		t.Fatalf("expected root thread parent validation error, got %v", err)
	}
}

func TestGetConversationMessageWindow_NormalizesAnchorNotFound(t *testing.T) {
	setupMessageWindowAppTestDB(t)
	app := &App{}

	conv, err := database.CreateConversation("Conversa", "")
	if err != nil {
		t.Fatalf("create conversation: %v", err)
	}
	if _, err := database.AddMessage(conv.ID, "user", "mensagem"); err != nil {
		t.Fatalf("create message: %v", err)
	}

	_, err = app.GetConversationMessageWindow(chat.MessageWindowRequest{
		ConversationID:  conv.ID,
		Scope:           chat.MessageWindowScopeConversation,
		AnchorMessageID: "missing-message",
		Direction:       chat.MessageWindowDirectionAfter,
		Limit:           10,
	})
	if err == nil || !strings.Contains(err.Error(), "anchorMessageId inválido") {
		t.Fatalf("expected normalized anchor error, got %v", err)
	}
}

func TestGetConversationMessageWindow_ClampsOversizedLimit(t *testing.T) {
	setupMessageWindowAppTestDB(t)
	app := &App{}

	conv, err := database.CreateConversation("Conversa", "")
	if err != nil {
		t.Fatalf("create conversation: %v", err)
	}
	for i := 0; i < database.MaxMessageWindowRows+30; i++ {
		if _, err := database.AddMessage(conv.ID, "user", "mensagem"); err != nil {
			t.Fatalf("create message %d: %v", i, err)
		}
	}

	window, err := app.GetConversationMessageWindow(chat.MessageWindowRequest{
		ConversationID: conv.ID,
		Scope:          chat.MessageWindowScopeConversation,
		Anchor:         chat.MessageWindowAnchorEnd,
		Direction:      chat.MessageWindowDirectionBefore,
		Limit:          database.MaxMessageWindowRows + 60,
	})
	if err != nil {
		t.Fatalf("get window: %v", err)
	}
	if len(window.Nodes) != database.MaxMessageWindowRows {
		t.Fatalf("expected clamped window size %d, got %d", database.MaxMessageWindowRows, len(window.Nodes))
	}
}

func TestGetConversationMessageWindow_ReturnsCanonicalTimelineItems(t *testing.T) {
	setupMessageWindowAppTestDB(t)
	app := &App{}

	conv, err := database.CreateConversation("Conversa", "")
	if err != nil {
		t.Fatalf("create conversation: %v", err)
	}
	user, err := database.AddMessage(conv.ID, "user", "pergunta")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	_, err = database.AddAssistantToolMessage(
		conv.ID,
		user.ID,
		"vou buscar",
		`[{"id":"tool-1","type":"function","function":{"name":"search","arguments":"{}"}}]`,
		"",
		"",
	)
	if err != nil {
		t.Fatalf("create assistant tool call: %v", err)
	}
	if _, err := database.AddToolResultMessage(conv.ID, user.ID, "resultado", "tool-1"); err != nil {
		t.Fatalf("create tool result: %v", err)
	}
	finalAssistant, err := database.AddMessageWithTokens(conv.ID, "assistant", "resposta final", 0, 0, 0, "")
	if err != nil {
		t.Fatalf("create final assistant: %v", err)
	}
	finalAssistant.TurnID = &user.ID
	if err := database.DB().Save(finalAssistant).Error; err != nil {
		t.Fatalf("save final assistant turn: %v", err)
	}

	window, err := app.GetConversationMessageWindow(chat.MessageWindowRequest{
		ConversationID: conv.ID,
		Scope:          chat.MessageWindowScopeConversation,
		Anchor:         chat.MessageWindowAnchorEnd,
		Direction:      chat.MessageWindowDirectionBefore,
		Limit:          10,
	})
	if err != nil {
		t.Fatalf("get window: %v", err)
	}

	if window.TotalCount != 2 {
		t.Fatalf("expected user item + consolidated turn item, got total=%d", window.TotalCount)
	}
	if len(window.Nodes) != 2 {
		t.Fatalf("expected 2 rendered timeline nodes, got %d", len(window.Nodes))
	}
	if window.Nodes[0].Message.ID != user.ID {
		t.Fatalf("expected first node to be user item, got %s", window.Nodes[0].Message.ID)
	}
	turnNode := window.Nodes[1]
	if turnNode.Message.ID != finalAssistant.ID {
		t.Fatalf("expected turn representative to be final assistant, got %s", turnNode.Message.ID)
	}
	if turnNode.OriginalIndex == nil || *turnNode.OriginalIndex != 1 {
		t.Fatalf("expected canonical originalIndex=1 for turn item, got %v", turnNode.OriginalIndex)
	}
	if turnNode.Message.Content != "resposta final" {
		t.Fatalf("expected final content, got %q", turnNode.Message.Content)
	}
	if !strings.Contains(turnNode.Message.ToolCalls, "resultado") {
		t.Fatalf("expected enriched tool call result, got %s", turnNode.Message.ToolCalls)
	}
}

func TestGetConversationMessageWindow_AnchorInsideTurnUsesTimelineItem(t *testing.T) {
	setupMessageWindowAppTestDB(t)
	app := &App{}

	conv, err := database.CreateConversation("Conversa", "")
	if err != nil {
		t.Fatalf("create conversation: %v", err)
	}
	user, err := database.AddMessage(conv.ID, "user", "pergunta")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	assistant, err := database.AddAssistantToolMessage(
		conv.ID,
		user.ID,
		"vou buscar",
		`[{"id":"tool-1","type":"function","function":{"name":"search","arguments":"{}"}}]`,
		"",
		"",
	)
	if err != nil {
		t.Fatalf("create assistant tool call: %v", err)
	}
	if _, err := database.AddToolResultMessage(conv.ID, user.ID, "resultado", "tool-1"); err != nil {
		t.Fatalf("create tool result: %v", err)
	}
	nextUser, err := database.AddMessage(conv.ID, "user", "pergunta seguinte")
	if err != nil {
		t.Fatalf("create next user: %v", err)
	}

	window, err := app.GetConversationMessageWindow(chat.MessageWindowRequest{
		ConversationID:  conv.ID,
		Scope:           chat.MessageWindowScopeConversation,
		AnchorMessageID: assistant.ID,
		Direction:       chat.MessageWindowDirectionAfter,
		Limit:           1,
	})
	if err != nil {
		t.Fatalf("get window: %v", err)
	}
	if len(window.Nodes) != 1 || window.Nodes[0].Message.ID != nextUser.ID {
		t.Fatalf("expected anchor inside turn to page after the whole turn, got %+v", window.Nodes)
	}
}

func TestGetConversationMessageWindow_TurnWithoutAssistantReturnsAssistantPlaceholder(t *testing.T) {
	setupMessageWindowAppTestDB(t)
	app := &App{}

	conv, err := database.CreateConversation("Conversa", "")
	if err != nil {
		t.Fatalf("create conversation: %v", err)
	}
	user, err := database.AddMessage(conv.ID, "user", "pergunta")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	tool, err := database.AddToolResultMessage(conv.ID, user.ID, "resultado preservado", "tool-1")
	if err != nil {
		t.Fatalf("create tool-only turn: %v", err)
	}

	window, err := app.GetConversationMessageWindow(chat.MessageWindowRequest{
		ConversationID: conv.ID,
		Scope:          chat.MessageWindowScopeConversation,
		Anchor:         chat.MessageWindowAnchorStart,
		Direction:      chat.MessageWindowDirectionAfter,
		Limit:          10,
	})
	if err != nil {
		t.Fatalf("get window: %v", err)
	}
	if window.TotalCount != 2 || len(window.Nodes) != 2 {
		t.Fatalf("expected user item + tool-only turn item, got total=%d nodes=%d", window.TotalCount, len(window.Nodes))
	}
	turnNode := window.Nodes[1]
	if turnNode.Message.ID != tool.ID {
		t.Fatalf("expected tool message id to remain representative, got %s", turnNode.Message.ID)
	}
	if turnNode.Message.Role != "assistant" || turnNode.Message.Content != "" {
		t.Fatalf("expected assistant placeholder for tool-only turn, got role=%q content=%q", turnNode.Message.Role, turnNode.Message.Content)
	}
	if turnNode.Message.Source != toolOnlyTurnPlaceholderSource {
		t.Fatalf("expected tool-only placeholder source, got %q", turnNode.Message.Source)
	}
	if !strings.Contains(turnNode.Message.ToolCalls, "resultado preservado") {
		t.Fatalf("expected tool result preserved in placeholder tool calls, got %s", turnNode.Message.ToolCalls)
	}
	if turnNode.OriginalIndex == nil || *turnNode.OriginalIndex != 1 {
		t.Fatalf("expected canonical originalIndex=1 for tool-only turn, got %v", turnNode.OriginalIndex)
	}
}
