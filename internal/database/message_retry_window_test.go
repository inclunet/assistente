package database

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

func insertRetryWindowConversation(t testing.TB, id, userID, summary, summaryUpTo string) *Conversation {
	t.Helper()
	conversation := &Conversation{
		UUIDModel: UUIDModel{ID: id, CreatedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)},
		UserID:    userID,
		Title:     "retry window",
		Summary:   summary,
	}
	conversation.SummaryUpToMessageID = summaryUpTo
	if err := db.Create(conversation).Error; err != nil {
		t.Fatalf("create conversation %s: %v", id, err)
	}
	return conversation
}

func insertRetryWindowMessage(t testing.TB, conversationID, id string, createdAt time.Time, role, content string, parentID *string) *ChatMessage {
	t.Helper()
	message := &ChatMessage{
		UUIDModel:      UUIDModel{ID: id, CreatedAt: createdAt},
		ConversationID: conversationID,
		ParentID:       parentID,
		Role:           role,
		Content:        content,
	}
	if err := db.Create(message).Error; err != nil {
		t.Fatalf("create message %s: %v", id, err)
	}
	return message
}

func updateRetryWindowSummary(t testing.TB, conversationID, summary, summaryUpTo string) {
	t.Helper()
	if err := db.Model(&Conversation{}).Where("id = ?", conversationID).
		Updates(map[string]any{"summary": summary, "summary_up_to_message_id": summaryUpTo}).Error; err != nil {
		t.Fatalf("update summary: %v", err)
	}
}

func retryWindowIDs(messages []ChatMessage) []string {
	ids := make([]string, 0, len(messages))
	for _, message := range messages {
		ids = append(ids, message.ID)
	}
	return ids
}

func assertRetryWindowIDs(t testing.TB, got []ChatMessage, want ...string) {
	t.Helper()
	gotIDs := retryWindowIDs(got)
	if fmt.Sprint(gotIDs) != fmt.Sprint(want) {
		t.Fatalf("message IDs: got %v, want %v", gotIDs, want)
	}
}

func TestLoadHistoryWindowThroughMessage_SummaryBoundarySemantics(t *testing.T) {
	cases := []struct {
		name             string
		boundary         string
		wantSummary      string
		wantAvailable    bool
		wantMessageIDs   []string
		wantBoundaryEcho string
	}{
		{
			name:             "anterior estrito",
			boundary:         "root-1",
			wantSummary:      "resumo anterior",
			wantAvailable:    true,
			wantMessageIDs:   []string{"target"},
			wantBoundaryEcho: "root-1",
		},
		{
			name:             "igual ao alvo",
			boundary:         "target",
			wantSummary:      "",
			wantAvailable:    false,
			wantMessageIDs:   []string{"root-0", "root-1", "target"},
			wantBoundaryEcho: "target",
		},
		{
			name:             "posterior ao alvo",
			boundary:         "posterior",
			wantSummary:      "",
			wantAvailable:    false,
			wantMessageIDs:   []string{"root-0", "root-1", "target"},
			wantBoundaryEcho: "posterior",
		},
		{
			name:             "deletado",
			boundary:         "missing-boundary",
			wantSummary:      "",
			wantAvailable:    false,
			wantMessageIDs:   []string{"root-0", "root-1", "target"},
			wantBoundaryEcho: "missing-boundary",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			setupChatMessageIndexTestDB(t)
			conversation := insertRetryWindowConversation(t, "conv-summary-"+tc.name, testUserID, "resumo anterior", tc.boundary)
			base := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
			insertRetryWindowMessage(t, conversation.ID, "root-0", base, "user", "antes", nil)
			insertRetryWindowMessage(t, conversation.ID, "root-1", base.Add(time.Minute), "assistant", "resposta", nil)
			insertRetryWindowMessage(t, conversation.ID, "target", base.Add(2*time.Minute), "user", "alvo", nil)
			insertRetryWindowMessage(t, conversation.ID, "posterior", base.Add(3*time.Minute), "assistant", "depois", nil)
			updateRetryWindowSummary(t, conversation.ID, "resumo anterior", tc.boundary)

			result, err := NewMessageRepository(db).LoadHistoryWindowThroughMessageWithContext(testCtx(), conversation.ID, "target", 50)
			if err != nil {
				t.Fatalf("LoadHistoryWindowThroughMessageWithContext: %v", err)
			}
			if result.Summary != tc.wantSummary || result.SummaryBoundaryAvailable != tc.wantAvailable || result.SummaryUpToMessageID != tc.wantBoundaryEcho {
				t.Fatalf("summary metadata: got summary=%q available=%v boundary=%q", result.Summary, result.SummaryBoundaryAvailable, result.SummaryUpToMessageID)
			}
			assertRetryWindowIDs(t, result.Messages, tc.wantMessageIDs...)
		})
	}
}

func TestLoadHistoryWindowThroughMessage_RejectsThreadForeignAndWrongRoleTargets(t *testing.T) {
	setupChatMessageIndexTestDB(t)
	base := time.Date(2026, 2, 2, 0, 0, 0, 0, time.UTC)
	owner := insertRetryWindowConversation(t, "conv-owner", testUserID, "", "")
	other := insertRetryWindowConversation(t, "conv-other", "other-user", "", "")
	target := insertRetryWindowMessage(t, owner.ID, "owner-target", base, "user", "alvo", nil)
	threadUser := insertRetryWindowMessage(t, owner.ID, "thread-user", base.Add(time.Minute), "user", "thread", &target.ID)
	insertRetryWindowMessage(t, owner.ID, "root-assistant", base.Add(2*time.Minute), "assistant", "não é alvo", nil)
	insertRetryWindowMessage(t, other.ID, "foreign-target", base, "user", "outro usuário", nil)

	tests := []struct {
		name           string
		ctx            context.Context
		conversationID string
		messageID      string
	}{
		{name: "mensagem thread", ctx: testCtx(), conversationID: owner.ID, messageID: threadUser.ID},
		{name: "assistant raiz", ctx: testCtx(), conversationID: owner.ID, messageID: "root-assistant"},
		{name: "mensagem de outra conversa", ctx: testCtx(), conversationID: owner.ID, messageID: "foreign-target"},
		{name: "conversa de outro usuário", ctx: testCtx(), conversationID: other.ID, messageID: "foreign-target"},
		{name: "sem autenticação", ctx: context.Background(), conversationID: owner.ID, messageID: target.ID},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NewMessageRepository(db).LoadHistoryWindowThroughMessageWithContext(tc.ctx, tc.conversationID, tc.messageID, 50)
			if err == nil {
				t.Fatal("expected target validation/authentication error")
			}
		})
	}
}

func TestLoadHistoryWindowThroughMessage_LimitsAndCreatedAtIDTieBreak(t *testing.T) {
	setupChatMessageIndexTestDB(t)
	conversation := insertRetryWindowConversation(t, "conv-ties", testUserID, "", "")
	createdAt := time.Date(2026, 2, 3, 0, 0, 0, 0, time.UTC)
	ids := []string{"tie-a", "tie-b", "tie-c", "tie-d", "tie-e"}
	for _, id := range ids {
		insertRetryWindowMessage(t, conversation.ID, id, createdAt, "user", id, nil)
	}

	for _, tc := range []struct {
		max  int
		want []string
	}{
		{max: 2, want: []string{"tie-a", "tie-b", "tie-d", "tie-e"}},
		{max: 3, want: []string{"tie-a", "tie-b", "tie-c", "tie-d", "tie-e"}},
	} {
		t.Run(fmt.Sprintf("max-%d", tc.max), func(t *testing.T) {
			result, err := NewMessageRepository(db).LoadHistoryWindowThroughMessageWithContext(testCtx(), conversation.ID, "tie-e", tc.max)
			if err != nil {
				t.Fatalf("LoadHistoryWindowThroughMessageWithContext: %v", err)
			}
			assertRetryWindowIDs(t, result.Messages, tc.want...)
		})
	}
}

func TestLoadHistoryWindowThroughMessage_LongHistoriesKeepFirstTwoAndRecentTail(t *testing.T) {
	for _, size := range []int{100, 500, 1000} {
		t.Run(fmt.Sprintf("messages-%d", size), func(t *testing.T) {
			setupChatMessageIndexTestDB(t)
			conversation := insertRetryWindowConversation(t, fmt.Sprintf("conv-long-%d", size), testUserID, "", "")
			base := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
			messages := make([]ChatMessage, 0, size)
			for i := 0; i < size; i++ {
				role := "assistant"
				if i%2 == 0 {
					role = "user"
				}
				messages = append(messages, ChatMessage{
					UUIDModel:      UUIDModel{ID: fmt.Sprintf("long-%04d", i), CreatedAt: base.Add(time.Duration(i) * time.Second)},
					ConversationID: conversation.ID,
					Role:           role,
					Content:        strings.Repeat("context ", 12),
				})
			}
			if err := db.CreateInBatches(messages, 100).Error; err != nil {
				t.Fatalf("create %d messages: %v", size, err)
			}

			targetID := fmt.Sprintf("long-%04d", size-2)
			result, err := NewMessageRepository(db).LoadHistoryWindowThroughMessageWithContext(testCtx(), conversation.ID, targetID, 50)
			if err != nil {
				t.Fatalf("LoadHistoryWindowThroughMessageWithContext: %v", err)
			}
			if len(result.Messages) != 52 {
				t.Fatalf("expected first two plus recent 50, got %d", len(result.Messages))
			}
			if result.Messages[0].ID != "long-0000" || result.Messages[1].ID != "long-0001" || result.Messages[len(result.Messages)-1].ID != targetID {
				t.Fatalf("unexpected long-history anchors: first=%s,%s last=%s", result.Messages[0].ID, result.Messages[1].ID, result.Messages[len(result.Messages)-1].ID)
			}
		})
	}
}

func TestLoadHistoryWindowThroughMessage_ContextAndLimitsErrors(t *testing.T) {
	setupChatMessageIndexTestDB(t)
	conversation := insertRetryWindowConversation(t, "conv-errors", testUserID, "", "")
	insertRetryWindowMessage(t, conversation.ID, "error-target", time.Now().UTC(), "user", "alvo", nil)
	repo := NewMessageRepository(db)

	for _, maxMessages := range []int{0, -1} {
		if _, err := repo.LoadHistoryWindowThroughMessageWithContext(testCtx(), conversation.ID, "error-target", maxMessages); err == nil {
			t.Fatalf("maxMessages=%d should fail", maxMessages)
		}
	}

	canceled, cancel := context.WithCancel(testCtx())
	cancel()
	_, err := repo.LoadHistoryWindowThroughMessageWithContext(canceled, conversation.ID, "error-target", 3)
	if err == nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}

func TestLoadHistoryWindowThroughMessage_QueryPlanUsesWindowIndex(t *testing.T) {
	setupChatMessageIndexTestDB(t)
	conversation := insertRetryWindowConversation(t, "conv-plan", testUserID, "", "")
	base := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 100; i++ {
		insertRetryWindowMessage(t, conversation.ID, fmt.Sprintf("plan-%04d", i), base.Add(time.Duration(i)*time.Second), "user", "plan", nil)
	}
	targetTime := base.Add(99 * time.Second)

	queries := []struct {
		name  string
		query string
		args  []any
	}{
		{
			name: "first-two",
			query: `SELECT id FROM chat_messages
				WHERE conversation_id = ? AND parent_id IS NULL
				  AND (created_at, id) <= (?, ?)
				ORDER BY created_at ASC, id ASC LIMIT 2`,
			args: []any{conversation.ID, targetTime, "plan-0099"},
		},
		{
			name: "recent-tail",
			query: `SELECT id FROM chat_messages
				WHERE conversation_id = ? AND parent_id IS NULL
				  AND (created_at, id) <= (?, ?)
				ORDER BY created_at DESC, id DESC LIMIT 50`,
			args: []any{conversation.ID, targetTime, "plan-0099"},
		},
	}

	for _, tc := range queries {
		t.Run(tc.name, func(t *testing.T) {
			rows, err := db.Raw("EXPLAIN QUERY PLAN "+tc.query, tc.args...).Rows()
			if err != nil {
				t.Fatalf("EXPLAIN QUERY PLAN: %v", err)
			}
			defer func() { _ = rows.Close() }()
			var details []string
			for rows.Next() {
				var id, parent, notUsed int
				var detail string
				if err := rows.Scan(&id, &parent, &notUsed, &detail); err != nil {
					t.Fatalf("scan query plan: %v", err)
				}
				details = append(details, detail)
			}
			if err := rows.Err(); err != nil {
				t.Fatalf("query plan rows: %v", err)
			}
			plan := strings.Join(details, " | ")
			if !strings.Contains(plan, "idx_chat_messages_window") {
				t.Fatalf("query plan did not use idx_chat_messages_window: %s", plan)
			}
			upper := strings.ToUpper(plan)
			if strings.Contains(upper, "SCAN CHAT_MESSAGES") || strings.Contains(upper, "USE TEMP B-TREE") {
				t.Fatalf("query plan has full scan or temporary sort: %s", plan)
			}
		})
	}
}
