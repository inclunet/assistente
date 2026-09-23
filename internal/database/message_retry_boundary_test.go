package database

import (
	"testing"
	"time"
)

func TestAnchoredRetryDiscardsUnscopedSummaryBoundary(t *testing.T) {
	setupChatMessageIndexTestDB(t)
	owner := insertRetryWindowConversation(t, "boundary-owner", testUserID, "", "")
	other := insertRetryWindowConversation(t, "boundary-other", "other-user", "", "")
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	first := insertRetryWindowMessage(t, owner.ID, "first", base, "user", "início", nil)
	thread := insertRetryWindowMessage(t, owner.ID, "thread", base.Add(time.Second), "assistant", "filha", &first.ID)
	foreign := insertRetryWindowMessage(t, other.ID, "foreign", base.Add(time.Second), "user", "outra conversa", nil)
	target := insertRetryWindowMessage(t, owner.ID, "target", base.Add(2*time.Second), "user", "alvo", nil)
	for _, boundary := range []string{"", thread.ID, foreign.ID} {
		updateRetryWindowSummary(t, owner.ID, "resumo que não pode entrar", boundary)
		window, err := NewMessageRepository(db).LoadHistoryWindowThroughMessageWithContext(testCtx(), owner.ID, target.ID, 50)
		if err != nil {
			t.Fatal(err)
		}
		if window.Summary != "" || window.SummaryBoundaryAvailable {
			t.Fatalf("boundary=%q leaked summary: %+v", boundary, window)
		}
		assertRetryWindowIDs(t, window.Messages, first.ID, target.ID)
	}
}
