package database

import (
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

// setupTestDB creates an in-memory SQLite database for testing
func setupTestDB(t *testing.T) {
	t.Helper()
	var err error
	db, err = gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}

	if err := db.AutoMigrate(&Conversation{}, &ChatMessage{}, &SubAgentRun{}); err != nil {
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

func createTestConversation(t *testing.T, title string) string {
	t.Helper()
	conv := &Conversation{Title: title, UserID: testUserID}
	if err := db.Create(conv).Error; err != nil {
		t.Fatalf("failed to create conversation: %v", err)
	}
	return conv.ID
}

func createTestMessage(t *testing.T, convID string, role, content string) string {
	t.Helper()
	msg := &ChatMessage{
		ConversationID: convID,
		Role:           role,
		Content:        content,
	}
	if err := db.Create(msg).Error; err != nil {
		t.Fatalf("failed to create message: %v", err)
	}
	return msg.ID
}

// ==================== GetConversationSummary ====================

func TestGetConversationSummary_Empty(t *testing.T) {
	setupTestDB(t)
	convID := createTestConversation(t, "Test")

	summary, upToID, err := GetConversationSummaryWithContext(testCtx(), convID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if summary != "" {
		t.Errorf("expected empty summary, got %q", summary)
	}
	if upToID != "" {
		t.Errorf("expected empty upToID, got %s", upToID)
	}
}

func TestGetConversationSummary_NotFound(t *testing.T) {
	setupTestDB(t)

	_, _, err := GetConversationSummaryWithContext(testCtx(), "99999")
	if err == nil {
		t.Error("expected error for non-existent conversation")
	}
}

// ==================== UpdateConversationSummary ====================

func TestUpdateConversationSummary(t *testing.T) {
	setupTestDB(t)
	convID := createTestConversation(t, "Test")

	err := UpdateConversationSummaryWithContext(testCtx(), convID, "This is a summary", "42")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	summary, upToID, err := GetConversationSummaryWithContext(testCtx(), convID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if summary != "This is a summary" {
		t.Errorf("expected 'This is a summary', got %q", summary)
	}
	if upToID != "42" {
		t.Errorf("expected upToID 42, got %s", upToID)
	}
}

func TestUpdateConversationSummary_ClearsSummarizingFlag(t *testing.T) {
	setupTestDB(t)
	convID := createTestConversation(t, "Test")

	if err := SetSummarizingInProgressWithContext(testCtx(), convID, true); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if err := UpdateConversationSummaryWithContext(testCtx(), convID, "Done", "10"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	inProgress, err := IsSummarizingInProgressWithContext(testCtx(), convID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if inProgress {
		t.Error("expected summarizing_in_progress to be false after UpdateConversationSummary")
	}
}

func TestUpdateConversationSummary_Incremental(t *testing.T) {
	setupTestDB(t)
	convID := createTestConversation(t, "Test")

	if err := UpdateConversationSummaryWithContext(testCtx(), convID, "First summary", "5"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if err := UpdateConversationSummaryWithContext(testCtx(), convID, "Extended summary with more context", "15"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	summary, upToID, err := GetConversationSummaryWithContext(testCtx(), convID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if summary != "Extended summary with more context" {
		t.Errorf("expected updated summary, got %q", summary)
	}
	if upToID != "15" {
		t.Errorf("expected upToID 15, got %s", upToID)
	}
}

// ==================== SetSummarizingInProgress / IsSummarizingInProgress ====================

func TestSummarizingInProgress(t *testing.T) {
	setupTestDB(t)
	convID := createTestConversation(t, "Test")

	inProgress, err := IsSummarizingInProgressWithContext(testCtx(), convID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if inProgress {
		t.Error("expected false initially")
	}

	if err := SetSummarizingInProgressWithContext(testCtx(), convID, true); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	inProgress, err = IsSummarizingInProgressWithContext(testCtx(), convID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !inProgress {
		t.Error("expected true after setting")
	}

	if err := SetSummarizingInProgressWithContext(testCtx(), convID, false); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	inProgress, err = IsSummarizingInProgressWithContext(testCtx(), convID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if inProgress {
		t.Error("expected false after clearing")
	}
}

func TestIsSummarizingInProgress_NotFound(t *testing.T) {
	setupTestDB(t)

	_, err := IsSummarizingInProgressWithContext(testCtx(), "99999")
	if err == nil {
		t.Error("expected error for non-existent conversation")
	}
}

// ==================== GetMessagesAfterID ====================

func TestGetMessagesAfterID(t *testing.T) {
	setupTestDB(t)
	convID := createTestConversation(t, "Test")

	id1 := createTestMessage(t, convID, "user", "msg1")
	createTestMessage(t, convID, "assistant", "msg2")
	createTestMessage(t, convID, "user", "msg3")

	msgs, err := GetMessagesAfterIDWithContext(testCtx(), convID, id1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages after ID %s, got %d", id1, len(msgs))
	}
	if msgs[0].Content != "msg2" {
		t.Errorf("expected 'msg2', got %q", msgs[0].Content)
	}
	if msgs[1].Content != "msg3" {
		t.Errorf("expected 'msg3', got %q", msgs[1].Content)
	}
}

func TestGetMessagesAfterID_AllAfterZero(t *testing.T) {
	setupTestDB(t)
	convID := createTestConversation(t, "Test")

	createTestMessage(t, convID, "user", "a")
	createTestMessage(t, convID, "assistant", "b")

	msgs, err := GetMessagesAfterIDWithContext(testCtx(), convID, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(msgs) != 2 {
		t.Errorf("expected 2 messages, got %d", len(msgs))
	}
}

func TestGetMessagesAfterID_ExcludesChildMessages(t *testing.T) {
	setupTestDB(t)
	convID := createTestConversation(t, "Test")

	parentID := createTestMessage(t, convID, "user", "root msg")
	// Child message (has ParentID)
	child := &ChatMessage{
		ConversationID: convID,
		ParentID:       &parentID,
		Role:           "assistant",
		Content:        "child msg",
	}
	if err := db.Create(child).Error; err != nil {
		t.Fatalf("failed to create child: %v", err)
	}

	msgs, err := GetMessagesAfterIDWithContext(testCtx(), convID, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(msgs) != 1 {
		t.Errorf("expected 1 root message, got %d", len(msgs))
	}
}

func TestGetMessagesAfterID_EmptyResult(t *testing.T) {
	setupTestDB(t)
	convID := createTestConversation(t, "Test")

	id := createTestMessage(t, convID, "user", "only")

	msgs, err := GetMessagesAfterIDWithContext(testCtx(), convID, id)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(msgs) != 0 {
		t.Errorf("expected 0 messages, got %d", len(msgs))
	}
}

// ==================== GetMessagesBetweenIDs ====================

func TestGetMessagesBetweenIDs(t *testing.T) {
	setupTestDB(t)
	convID := createTestConversation(t, "Test")

	id1 := createTestMessage(t, convID, "user", "msg1")
	id2 := createTestMessage(t, convID, "assistant", "msg2")
	id3 := createTestMessage(t, convID, "user", "msg3")
	createTestMessage(t, convID, "assistant", "msg4")

	msgs, err := GetMessagesBetweenIDsWithContext(testCtx(), convID, id1, id3)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages between %s and %s, got %d", id1, id3, len(msgs))
	}
	if msgs[0].ID != id2 {
		t.Errorf("expected msg ID %s, got %s", id2, msgs[0].ID)
	}
	if msgs[1].ID != id3 {
		t.Errorf("expected msg ID %s, got %s", id3, msgs[1].ID)
	}
}

func TestGetMessagesBetweenIDs_EmptyRange(t *testing.T) {
	setupTestDB(t)
	convID := createTestConversation(t, "Test")

	id1 := createTestMessage(t, convID, "user", "msg1")

	msgs, err := GetMessagesBetweenIDsWithContext(testCtx(), convID, id1, id1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(msgs) != 0 {
		t.Errorf("expected 0 messages for same start/end, got %d", len(msgs))
	}
}

func TestGetMessagesBetweenIDs_DifferentConversation(t *testing.T) {
	setupTestDB(t)
	conv1 := createTestConversation(t, "Conv1")
	conv2 := createTestConversation(t, "Conv2")

	createTestMessage(t, conv1, "user", "conv1 msg")
	id2 := createTestMessage(t, conv2, "user", "conv2 msg")

	msgs, err := GetMessagesBetweenIDsWithContext(testCtx(), conv1, "", id2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(msgs) != 1 {
		t.Errorf("expected 1 message from conv1 only, got %d", len(msgs))
	}
}

// ==================== Integration: Summary workflow ====================

func TestSummaryWorkflow(t *testing.T) {
	setupTestDB(t)
	convID := createTestConversation(t, "Workflow Test")

	for i := 0; i < 10; i++ {
		role := "user"
		if i%2 == 1 {
			role = "assistant"
		}
		createTestMessage(t, convID, role, "message content")
	}

	summary, upToID, err := GetConversationSummaryWithContext(testCtx(), convID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if summary != "" || upToID != "" {
		t.Error("expected empty initial summary")
	}

	inProgress, _ := IsSummarizingInProgressWithContext(testCtx(), convID)
	if inProgress {
		t.Error("expected not in progress initially")
	}

	if err := SetSummarizingInProgressWithContext(testCtx(), convID, true); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	msgs, err := GetMessagesAfterIDWithContext(testCtx(), convID, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(msgs) != 10 {
		t.Fatalf("expected 10 messages, got %d", len(msgs))
	}

	cutoffID := msgs[5].ID
	if err := UpdateConversationSummaryWithContext(testCtx(), convID, "Summary of first 6 messages", cutoffID); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	summary, upToID, err = GetConversationSummaryWithContext(testCtx(), convID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if summary != "Summary of first 6 messages" {
		t.Errorf("unexpected summary: %q", summary)
	}
	if upToID != cutoffID {
		t.Errorf("expected upToID %s, got %s", cutoffID, upToID)
	}

	inProgress, _ = IsSummarizingInProgressWithContext(testCtx(), convID)
	if inProgress {
		t.Error("summarizing should be cleared after UpdateConversationSummary")
	}

	remainingMsgs, err := GetMessagesAfterIDWithContext(testCtx(), convID, cutoffID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(remainingMsgs) != 4 {
		t.Errorf("expected 4 remaining messages after summary, got %d", len(remainingMsgs))
	}
}
