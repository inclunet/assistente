package database

import (
	"strings"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

// setupOrderingTestDB creates an in-memory SQLite database for ordering tests
func setupOrderingTestDB(t *testing.T) {
	t.Helper()
	var err error
	db, err = gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}

	if err := db.AutoMigrate(&Conversation{}, &ChatMessage{}); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}

	t.Cleanup(func() {
		sqlDB, _ := db.DB()
		if sqlDB != nil {
			_ = sqlDB.Close()
		}
		db = nil
	})
}

// createConvWithOrderedMessages creates a conversation with messages that have
// UUIDv7 IDs in reverse lexicographic order but ascending created_at.
// This simulates the real scenario where UUIDs generated in the same millisecond
// can be lexicographically "out of order" relative to insertion order.
func createConvWithOrderedMessages(t *testing.T) (convID string, msgIDs []string) {
	t.Helper()

	conv := &Conversation{Title: "ordering-test"}
	if err := db.Create(conv).Error; err != nil {
		t.Fatalf("failed to create conversation: %v", err)
	}
	convID = conv.ID

	// UUIDs chosen so that lexicographic order (id-A < id-B < id-C) differs from
	// insertion order (id-C first, id-B second, id-A third) — i.e. created_at
	// order is the reverse of lexicographic order. This catches any code that
	// accidentally compares UUIDs lexicographically instead of using created_at.
	ids := []string{
		"01970fff-0001-7000-8000-000000000001", // created first  — lex "last" (fff)
		"01970aaa-0002-7000-8000-000000000002", // created second — lex "middle" (aaa)
		"019700bb-0003-7000-8000-000000000003", // created third  — lex "first" (0bb)
	}

	baseTime := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	for i, id := range ids {
		msg := ChatMessage{
			UUIDModel: UUIDModel{
				ID:        id,
				CreatedAt: baseTime.Add(time.Duration(i) * time.Minute),
			},
			ConversationID: convID,
			Role:           "user",
			Content:        "message " + id,
			TotalTokens:    (i + 1) * 10,
		}
		if err := db.Create(&msg).Error; err != nil {
			t.Fatalf("failed to create message %d: %v", i, err)
		}
	}

	return convID, ids
}

func TestGetMessagesAfterID_UsesCreatedAtNotLex(t *testing.T) {
	setupOrderingTestDB(t)
	convID, ids := createConvWithOrderedMessages(t)

	// Ask for messages after the first one (by created_at order).
	msgs, err := GetMessagesAfterID(convID, ids[0])
	if err != nil {
		t.Fatalf("GetMessagesAfterID: %v", err)
	}
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages after first, got %d", len(msgs))
	}
	if msgs[0].ID != ids[1] {
		t.Errorf("expected first result %s, got %s", ids[1], msgs[0].ID)
	}
	if msgs[1].ID != ids[2] {
		t.Errorf("expected second result %s, got %s", ids[2], msgs[1].ID)
	}
}

func TestGetMessagesAfterID_EmptyAfterID(t *testing.T) {
	setupOrderingTestDB(t)
	convID, ids := createConvWithOrderedMessages(t)

	msgs, err := GetMessagesAfterID(convID, "")
	if err != nil {
		t.Fatalf("GetMessagesAfterID empty: %v", err)
	}
	if len(msgs) != 3 {
		t.Fatalf("expected all 3 messages, got %d", len(msgs))
	}
	// Should be in created_at order
	for i, id := range ids {
		if msgs[i].ID != id {
			t.Errorf("msgs[%d]: expected %s, got %s", i, id, msgs[i].ID)
		}
	}
}

func TestGetMessagesBetweenIDs_UsesCreatedAtNotLex(t *testing.T) {
	setupOrderingTestDB(t)
	convID, ids := createConvWithOrderedMessages(t)

	msgs, err := GetMessagesBetweenIDs(convID, ids[0], ids[2])
	if err != nil {
		t.Fatalf("GetMessagesBetweenIDs: %v", err)
	}
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages between first and last, got %d", len(msgs))
	}
	if msgs[0].ID != ids[1] {
		t.Errorf("expected first %s, got %s", ids[1], msgs[0].ID)
	}
	if msgs[1].ID != ids[2] {
		t.Errorf("expected last %s, got %s", ids[2], msgs[1].ID)
	}
}

func TestGetDetailedTokenStats_CutoffByCreatedAt(t *testing.T) {
	setupOrderingTestDB(t)
	convID, ids := createConvWithOrderedMessages(t)

	// ids[0] has 10 tokens, ids[1] has 20 tokens, ids[2] has 30 tokens
	// summaryUpToMessageID = ids[1] → out-of-context: ids[0]+ids[1] = 30 tokens,
	// in-context: ids[2] = 30 tokens
	stats, err := GetDetailedTokenStats(convID, ids[1])
	if err != nil {
		t.Fatalf("GetDetailedTokenStats: %v", err)
	}
	if stats.MessagesOutOfContextCount != 2 {
		t.Errorf("out-of-context count: expected 2, got %d", stats.MessagesOutOfContextCount)
	}
	if stats.MessagesOutOfContextTokens != 30 {
		t.Errorf("out-of-context tokens: expected 30, got %d", stats.MessagesOutOfContextTokens)
	}
	if stats.MessagesInContextCount != 1 {
		t.Errorf("in-context count: expected 1, got %d", stats.MessagesInContextCount)
	}
	if stats.MessagesInContextTokens != 30 {
		t.Errorf("in-context tokens: expected 30, got %d", stats.MessagesInContextTokens)
	}
}

func TestGetDetailedTokenStats_NoSummary(t *testing.T) {
	setupOrderingTestDB(t)
	convID, _ := createConvWithOrderedMessages(t)

	// No summary → all messages are in-context
	stats, err := GetDetailedTokenStats(convID, "")
	if err != nil {
		t.Fatalf("GetDetailedTokenStats: %v", err)
	}
	if stats.MessagesInContextCount != 3 {
		t.Errorf("in-context count: expected 3, got %d", stats.MessagesInContextCount)
	}
	if stats.MessagesInContextTokens != 60 {
		t.Errorf("in-context tokens: expected 60, got %d", stats.MessagesInContextTokens)
	}
	if stats.MessagesOutOfContextCount != 0 {
		t.Errorf("out-of-context count: expected 0, got %d", stats.MessagesOutOfContextCount)
	}
}

func createConvWithSameTimestampRootMessages(t *testing.T) (convID string, msgIDs []string) {
	t.Helper()

	conv := &Conversation{Title: "pagination-test"}
	if err := db.Create(conv).Error; err != nil {
		t.Fatalf("failed to create conversation: %v", err)
	}
	convID = conv.ID

	createdAt := time.Date(2025, 2, 1, 0, 0, 0, 0, time.UTC)
	ids := []string{
		"01971000-0000-7000-8000-000000000001",
		"01971000-0000-7000-8000-000000000002",
		"01971000-0000-7000-8000-000000000003",
		"01971000-0000-7000-8000-000000000004",
		"01971000-0000-7000-8000-000000000005",
	}

	for i, id := range ids {
		msg := ChatMessage{
			UUIDModel: UUIDModel{
				ID:        id,
				CreatedAt: createdAt,
			},
			ConversationID: convID,
			Role:           "user",
			Content:        "root message",
			TotalTokens:    i + 1,
		}
		if err := db.Create(&msg).Error; err != nil {
			t.Fatalf("failed to create root message %d: %v", i, err)
		}
	}

	childID := "01971000-0000-7000-8000-000000000099"
	child := ChatMessage{
		UUIDModel: UUIDModel{
			ID:        childID,
			CreatedAt: createdAt,
		},
		ConversationID: convID,
		ParentID:       &ids[2],
		Role:           "assistant",
		Content:        "child message",
	}
	if err := db.Create(&child).Error; err != nil {
		t.Fatalf("failed to create child message: %v", err)
	}

	return convID, ids
}

func TestGetRecentRootMessages_LimitAndStableOrder(t *testing.T) {
	setupOrderingTestDB(t)
	convID, ids := createConvWithSameTimestampRootMessages(t)

	msgs, err := GetRecentRootMessages(convID, 3)
	if err != nil {
		t.Fatalf("GetRecentRootMessages: %v", err)
	}
	if len(msgs) != 3 {
		t.Fatalf("expected 3 recent root messages, got %d", len(msgs))
	}

	expected := ids[2:5]
	for i, id := range expected {
		if msgs[i].ID != id {
			t.Errorf("msgs[%d]: expected %s, got %s", i, id, msgs[i].ID)
		}
		if msgs[i].ParentID != nil {
			t.Errorf("msgs[%d]: expected root message, got parent %s", i, *msgs[i].ParentID)
		}
	}
}

func TestGetRootMessagesBefore_CursorLimitAndStableOrder(t *testing.T) {
	setupOrderingTestDB(t)
	convID, ids := createConvWithSameTimestampRootMessages(t)

	msgs, err := GetRootMessagesBefore(convID, ids[3], 2)
	if err != nil {
		t.Fatalf("GetRootMessagesBefore: %v", err)
	}
	if len(msgs) != 2 {
		t.Fatalf("expected 2 root messages before cursor, got %d", len(msgs))
	}

	expected := []string{ids[1], ids[2]}
	for i, id := range expected {
		if msgs[i].ID != id {
			t.Errorf("msgs[%d]: expected %s, got %s", i, id, msgs[i].ID)
		}
		if msgs[i].ParentID != nil {
			t.Errorf("msgs[%d]: expected root message, got parent %s", i, *msgs[i].ParentID)
		}
	}
}

func TestGetMessageWindow_EndBeforeReturnsAbsoluteMetadata(t *testing.T) {
	setupOrderingTestDB(t)
	convID, ids := createConvWithSameTimestampRootMessages(t)

	window, err := GetMessageWindow(MessageWindowQuery{
		ConversationID: convID,
		Anchor:         "end",
		Direction:      "before",
		Limit:          2,
	})
	if err != nil {
		t.Fatalf("GetMessageWindow: %v", err)
	}
	if window.TotalCount != 5 {
		t.Fatalf("total count: expected 5, got %d", window.TotalCount)
	}
	if window.StartIndex != 3 || window.EndIndex != 4 {
		t.Fatalf("indices: expected 3..4, got %d..%d", window.StartIndex, window.EndIndex)
	}
	if !window.HasBefore || window.HasAfter {
		t.Fatalf("flags: expected hasBefore=true hasAfter=false, got %v/%v", window.HasBefore, window.HasAfter)
	}

	expected := []string{ids[3], ids[4]}
	for i, id := range expected {
		if window.Messages[i].ID != id {
			t.Errorf("messages[%d]: expected %s, got %s", i, id, window.Messages[i].ID)
		}
	}
}

func TestGetMessageWindow_RejectsInvalidCursorShape(t *testing.T) {
	setupOrderingTestDB(t)
	convID, ids := createConvWithSameTimestampRootMessages(t)

	cases := []struct {
		name  string
		query MessageWindowQuery
		want  string
	}{
		{
			name: "invalid direction",
			query: MessageWindowQuery{
				ConversationID: convID,
				Direction:      "sideways",
				Limit:          2,
			},
			want: "direction",
		},
		{
			name: "invalid anchor",
			query: MessageWindowQuery{
				ConversationID: convID,
				Anchor:         "middle",
				Direction:      "before",
				Limit:          2,
			},
			want: "anchor",
		},
		{
			name: "anchor and anchor message",
			query: MessageWindowQuery{
				ConversationID:  convID,
				Anchor:          "end",
				AnchorMessageID: ids[0],
				Direction:       "before",
				Limit:           2,
			},
			want: "mutuamente exclusivos",
		},
		{
			name: "start before",
			query: MessageWindowQuery{
				ConversationID: convID,
				Anchor:         "start",
				Direction:      "before",
				Limit:          2,
			},
			want: "anchor=start",
		},
		{
			name: "end after",
			query: MessageWindowQuery{
				ConversationID: convID,
				Anchor:         "end",
				Direction:      "after",
				Limit:          2,
			},
			want: "anchor=end",
		},
		{
			name: "around without anchor message",
			query: MessageWindowQuery{
				ConversationID: convID,
				Direction:      "around",
				Limit:          2,
			},
			want: "anchorMessageId",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := GetMessageWindow(tc.query)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("expected error containing %q, got %v", tc.want, err)
			}
		})
	}
}

func TestGetMessageWindow_BeforeAnchorUsesAbsoluteIndex(t *testing.T) {
	setupOrderingTestDB(t)
	convID, ids := createConvWithSameTimestampRootMessages(t)

	window, err := GetMessageWindow(MessageWindowQuery{
		ConversationID:  convID,
		AnchorMessageID: ids[4],
		Direction:       "before",
		Limit:           3,
	})
	if err != nil {
		t.Fatalf("GetMessageWindow before anchor: %v", err)
	}
	if window.StartIndex != 1 || window.EndIndex != 3 {
		t.Fatalf("indices: expected 1..3, got %d..%d", window.StartIndex, window.EndIndex)
	}
	if !window.HasBefore || !window.HasAfter {
		t.Fatalf("flags: expected hasBefore=true hasAfter=true, got %v/%v", window.HasBefore, window.HasAfter)
	}

	expected := []string{ids[1], ids[2], ids[3]}
	for i, id := range expected {
		if window.Messages[i].ID != id {
			t.Errorf("messages[%d]: expected %s, got %s", i, id, window.Messages[i].ID)
		}
	}
}

func TestGetMessageWindow_BeforeAnchorNearStartDoesNotIncludeAnchor(t *testing.T) {
	setupOrderingTestDB(t)
	convID, ids := createConvWithSameTimestampRootMessages(t)

	window, err := GetMessageWindow(MessageWindowQuery{
		ConversationID:  convID,
		AnchorMessageID: ids[0],
		Direction:       "before",
		Limit:           3,
	})
	if err != nil {
		t.Fatalf("GetMessageWindow before first anchor: %v", err)
	}
	if len(window.Messages) != 0 {
		t.Fatalf("expected empty window before first message, got %d messages", len(window.Messages))
	}
	if window.TotalCount != len(ids) {
		t.Fatalf("total count: expected %d, got %d", len(ids), window.TotalCount)
	}
	if window.StartIndex != 0 || window.EndIndex != -1 {
		t.Fatalf("indices: expected 0..-1, got %d..%d", window.StartIndex, window.EndIndex)
	}
	if window.HasBefore || !window.HasAfter {
		t.Fatalf("flags: expected hasBefore=false hasAfter=true, got %v/%v", window.HasBefore, window.HasAfter)
	}
}

func TestGetMessageWindow_AroundRebalancesAtEnd(t *testing.T) {
	setupOrderingTestDB(t)
	convID, ids := createConvWithSameTimestampRootMessages(t)

	window, err := GetMessageWindow(MessageWindowQuery{
		ConversationID:  convID,
		AnchorMessageID: ids[4],
		Direction:       "around",
		Limit:           4,
	})
	if err != nil {
		t.Fatalf("GetMessageWindow around anchor: %v", err)
	}
	if len(window.Messages) != 4 {
		t.Fatalf("expected 4 messages, got %d", len(window.Messages))
	}
	if window.StartIndex != 1 || window.EndIndex != 4 {
		t.Fatalf("indices: expected 1..4, got %d..%d", window.StartIndex, window.EndIndex)
	}
}

func TestComputeMessageWindowHasAfter(t *testing.T) {
	cases := []struct {
		name       string
		total      int
		startIndex int
		endIndex   int
		itemCount  int
		want       bool
	}{
		{name: "empty total", total: 0, startIndex: 0, endIndex: -1, itemCount: 0, want: false},
		{name: "empty window before remaining items", total: 5, startIndex: 3, endIndex: -1, itemCount: 0, want: true},
		{name: "empty window at end", total: 5, startIndex: 5, endIndex: -1, itemCount: 0, want: false},
		{name: "non-empty window before tail", total: 5, startIndex: 1, endIndex: 3, itemCount: 3, want: true},
		{name: "non-empty window at tail", total: 5, startIndex: 3, endIndex: 4, itemCount: 2, want: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := computeMessageWindowHasAfter(tc.total, tc.startIndex, tc.endIndex, tc.itemCount)
			if got != tc.want {
				t.Fatalf("computeMessageWindowHasAfter() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestGetMessageWindow_CountsTurnAsTimelineItem(t *testing.T) {
	setupOrderingTestDB(t)
	conv, err := CreateConversation("timeline-items", "")
	if err != nil {
		t.Fatalf("create conversation: %v", err)
	}
	user, err := AddMessage(conv.ID, "user", "pergunta")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	assistant, err := AddAssistantToolMessage(conv.ID, user.ID, "vou buscar", `[{"id":"tool-1"}]`, "", "")
	if err != nil {
		t.Fatalf("create assistant: %v", err)
	}
	tool, err := AddToolResultMessage(conv.ID, user.ID, "resultado", "tool-1")
	if err != nil {
		t.Fatalf("create tool: %v", err)
	}
	nextUser, err := AddMessage(conv.ID, "user", "pergunta seguinte")
	if err != nil {
		t.Fatalf("create next user: %v", err)
	}

	window, err := GetMessageWindow(MessageWindowQuery{
		ConversationID: conv.ID,
		Anchor:         "start",
		Direction:      "after",
		Limit:          10,
	})
	if err != nil {
		t.Fatalf("GetMessageWindow: %v", err)
	}
	if window.TotalCount != 3 {
		t.Fatalf("expected 3 timeline items, got %d", window.TotalCount)
	}
	if len(window.Items) != 3 {
		t.Fatalf("expected 3 selected items, got %d", len(window.Items))
	}
	if window.Items[0].Kind != MessageWindowItemKindMessage || window.Items[0].MessageID != user.ID {
		t.Fatalf("expected first user message item, got %+v", window.Items[0])
	}
	if window.Items[1].Kind != MessageWindowItemKindTurn || window.Items[1].TurnID != user.ID {
		t.Fatalf("expected consolidated turn item, got %+v", window.Items[1])
	}
	if window.Items[2].Kind != MessageWindowItemKindMessage || window.Items[2].MessageID != nextUser.ID {
		t.Fatalf("expected next user message item, got %+v", window.Items[2])
	}

	messageIDs := map[string]bool{}
	for _, message := range window.Messages {
		messageIDs[message.ID] = true
	}
	if !messageIDs[assistant.ID] || !messageIDs[tool.ID] {
		t.Fatalf("expected selected turn messages to be fetched in batch, got ids=%v", messageIDs)
	}
}

func TestGetMessageWindow_AnchorInsideTurnPagesByWholeItem(t *testing.T) {
	setupOrderingTestDB(t)
	conv, err := CreateConversation("timeline-anchor", "")
	if err != nil {
		t.Fatalf("create conversation: %v", err)
	}
	user, err := AddMessage(conv.ID, "user", "pergunta")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	assistant, err := AddAssistantToolMessage(conv.ID, user.ID, "vou buscar", `[{"id":"tool-1"}]`, "", "")
	if err != nil {
		t.Fatalf("create assistant: %v", err)
	}
	if _, err := AddToolResultMessage(conv.ID, user.ID, "resultado", "tool-1"); err != nil {
		t.Fatalf("create tool: %v", err)
	}
	nextUser, err := AddMessage(conv.ID, "user", "pergunta seguinte")
	if err != nil {
		t.Fatalf("create next user: %v", err)
	}

	window, err := GetMessageWindow(MessageWindowQuery{
		ConversationID:  conv.ID,
		AnchorMessageID: assistant.ID,
		Direction:       "after",
		Limit:           1,
	})
	if err != nil {
		t.Fatalf("GetMessageWindow: %v", err)
	}
	if window.StartIndex != 2 || window.EndIndex != 2 {
		t.Fatalf("expected indices 2..2 after turn item, got %d..%d", window.StartIndex, window.EndIndex)
	}
	if len(window.Items) != 1 || window.Items[0].MessageID != nextUser.ID {
		t.Fatalf("expected next item after turn, got %+v", window.Items)
	}
}

func TestGetMessageWindow_BeforeAnchorInsideTurnPagesBeforeWholeItem(t *testing.T) {
	setupOrderingTestDB(t)
	conv, err := CreateConversation("timeline-anchor-before", "")
	if err != nil {
		t.Fatalf("create conversation: %v", err)
	}
	beforeUser, err := AddMessage(conv.ID, "user", "pergunta anterior")
	if err != nil {
		t.Fatalf("create before user: %v", err)
	}
	user, err := AddMessage(conv.ID, "user", "pergunta")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	assistant, err := AddAssistantToolMessage(conv.ID, user.ID, "vou buscar", `[{"id":"tool-1"}]`, "", "")
	if err != nil {
		t.Fatalf("create assistant: %v", err)
	}
	if _, err := AddToolResultMessage(conv.ID, user.ID, "resultado", "tool-1"); err != nil {
		t.Fatalf("create tool: %v", err)
	}

	window, err := GetMessageWindow(MessageWindowQuery{
		ConversationID:  conv.ID,
		AnchorMessageID: assistant.ID,
		Direction:       "before",
		Limit:           2,
	})
	if err != nil {
		t.Fatalf("GetMessageWindow: %v", err)
	}
	if window.StartIndex != 0 || window.EndIndex != 1 {
		t.Fatalf("expected indices 0..1 before turn item, got %d..%d", window.StartIndex, window.EndIndex)
	}
	if len(window.Items) != 2 || window.Items[0].MessageID != beforeUser.ID || window.Items[1].MessageID != user.ID {
		t.Fatalf("expected item before whole turn, got %+v", window.Items)
	}
}

func TestGetMessageWindow_AroundAnchorInsideToolCentersWholeTurn(t *testing.T) {
	setupOrderingTestDB(t)
	conv, err := CreateConversation("timeline-anchor-around", "")
	if err != nil {
		t.Fatalf("create conversation: %v", err)
	}
	if _, err := AddMessage(conv.ID, "user", "pergunta anterior"); err != nil {
		t.Fatalf("create before user: %v", err)
	}
	user, err := AddMessage(conv.ID, "user", "pergunta")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	if _, err := AddAssistantToolMessage(conv.ID, user.ID, "vou buscar", `[{"id":"tool-1"}]`, "", ""); err != nil {
		t.Fatalf("create assistant: %v", err)
	}
	tool, err := AddToolResultMessage(conv.ID, user.ID, "resultado", "tool-1")
	if err != nil {
		t.Fatalf("create tool: %v", err)
	}
	nextUser, err := AddMessage(conv.ID, "user", "pergunta seguinte")
	if err != nil {
		t.Fatalf("create next user: %v", err)
	}

	window, err := GetMessageWindow(MessageWindowQuery{
		ConversationID:  conv.ID,
		AnchorMessageID: tool.ID,
		Direction:       "around",
		Limit:           3,
	})
	if err != nil {
		t.Fatalf("GetMessageWindow: %v", err)
	}
	if window.StartIndex != 1 || window.EndIndex != 3 {
		t.Fatalf("expected indices 1..3 around turn item, got %d..%d", window.StartIndex, window.EndIndex)
	}
	if len(window.Items) != 3 {
		t.Fatalf("expected 3 timeline items, got %+v", window.Items)
	}
	if window.Items[0].MessageID != user.ID || window.Items[1].TurnID != user.ID || window.Items[2].MessageID != nextUser.ID {
		t.Fatalf("expected user, whole turn, next user; got %+v", window.Items)
	}
}

func TestGetMessageWindow_ThreadScopeCountsChildren(t *testing.T) {
	setupOrderingTestDB(t)
	convID, ids := createConvWithSameTimestampRootMessages(t)

	parentID := ids[2]
	window, err := GetMessageWindow(MessageWindowQuery{
		ConversationID: convID,
		ParentID:       &parentID,
		Anchor:         "start",
		Direction:      "after",
		Limit:          10,
	})
	if err != nil {
		t.Fatalf("GetMessageWindow thread: %v", err)
	}
	if window.TotalCount != 1 {
		t.Fatalf("thread total: expected 1, got %d", window.TotalCount)
	}
	if window.StartIndex != 0 || window.EndIndex != 0 {
		t.Fatalf("thread indices: expected 0..0, got %d..%d", window.StartIndex, window.EndIndex)
	}
	if window.Messages[0].ParentID == nil || *window.Messages[0].ParentID != parentID {
		t.Fatalf("expected child of %s", parentID)
	}
}
