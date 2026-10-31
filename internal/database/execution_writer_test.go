package database

import (
	"context"
	"errors"
	"testing"
)

func TestExecutionWritersPreserveRetryHistoryAndRejectStaleSummary(t *testing.T) {
	db := newChatTurnTestDB(t, false)
	seedChatTurnTestMessages(t, db, "conv", "turn", "")
	seedChatTurnTestMessages(t, db, "conv", "later", "later-response")
	if err := MigrateChatTurns(db); err != nil {
		t.Fatal(err)
	}
	if err := MigrateChatExecutionContent(db); err != nil {
		t.Fatal(err)
	}
	ctx := WithUserID(context.Background(), "user")
	repo := NewChatTurnRepository(db)
	first, err := repo.Begin(ctx, "conv", "turn", "", TurnModeNew)
	if err != nil {
		t.Fatal(err)
	}
	response, err := repo.EnsureResponse(ctx, "conv", "turn", first.ExecutionID)
	if err != nil {
		t.Fatal(err)
	}
	turnID := "turn"
	intermediate, err := NewMessageRepository(db).CreateMessageWithContext(ctx, MessageOptions{ConversationID: "conv", TurnID: &turnID, ChatExecutionID: &first.ExecutionID, Role: "assistant", Content: "intermediária anterior"})
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.FinishResponse(ctx, "conv", "turn", first.ExecutionID, TurnCompleted, "completed", response, &MessageOptions{Content: "resposta anterior", TotalTokens: 7}); err != nil {
		t.Fatal(err)
	}
	summaries := NewSummarizationRepository(db)
	if err := summaries.UpdateConversationSummaryWithContext(ctx, "conv", "resumo anterior", "later-response"); err != nil {
		t.Fatal(err)
	}
	next, err := repo.Begin(ctx, "conv", "turn", "", TurnModeRetry)
	if err != nil {
		t.Fatal(err)
	}
	if summary, boundary, err := summaries.GetConversationSummaryWithContext(ctx, "conv"); err != nil || summary != "" || boundary != "" {
		t.Fatalf("resumo antigo mantido: %q %q %v", summary, boundary, err)
	}
	if err := summaries.CommitConversationSummary(ctx, "conv", "resumo atrasado", "later-response", "resumo anterior", []string{"later", "later-response"}); !errors.Is(err, ErrSummarySourceChanged) {
		t.Fatalf("resumo sobre base invalidada aceito: %v", err)
	}
	if err := summaries.CommitConversationSummary(ctx, "conv", "resumo atrasado", response, "", []string{response}); !errors.Is(err, ErrSummarySourceChanged) {
		t.Fatalf("resposta substituída no resumo: %v", err)
	}
	nextResponse, err := repo.EnsureResponse(ctx, "conv", "turn", next.ExecutionID)
	if err != nil {
		t.Fatal(err)
	}
	if nextResponse == response {
		t.Fatal("representante reutilizado")
	}
	if err := repo.FinishResponse(ctx, "conv", "turn", next.ExecutionID, TurnCompleted, "completed", nextResponse, &MessageOptions{Content: "resposta atual"}); err != nil {
		t.Fatal(err)
	}
	visible, err := NewMessageRepository(db).GetMessagesWithContext(ctx, "conv", nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range visible {
		if m.ID == response || m.ID == intermediate.ID {
			t.Fatal("conteúdo anterior visível")
		}
	}
	var old ChatMessage
	if err := db.First(&old, "id=?", response).Error; err != nil {
		t.Fatal(err)
	}
	if old.Content != "resposta anterior" || old.TotalTokens != 7 {
		t.Fatal("conteúdo anterior sobrescrito")
	}
	if err := summaries.CommitConversationSummary(ctx, "conv", "resumo válido", nextResponse, "", []string{"turn", nextResponse}); err != nil {
		t.Fatal(err)
	}
	continuation, err := repo.Begin(ctx, "conv", "turn", "", TurnModeContinue)
	if err != nil {
		t.Fatal(err)
	}
	continuedResponse, err := repo.EnsureResponse(ctx, "conv", "turn", continuation.ExecutionID)
	if err != nil {
		t.Fatal(err)
	}
	var continued ChatMessage
	if err := db.First(&continued, "id=?", continuedResponse).Error; err != nil {
		t.Fatal(err)
	}
	if continued.ID == nextResponse || continued.Content != "resposta atual" || continuation.BaseExecutionID == nil || *continuation.BaseExecutionID != next.ExecutionID || continuation.BaseResponseMessageID == nil || *continuation.BaseResponseMessageID != nextResponse {
		t.Fatalf("base não capturada: %+v %+v", continued, continuation)
	}
	if err := NewMessageRepository(db).UpdateMessageContentReasoningAndUsageWithContext(ctx, nextResponse, "base editada", "", 0, 0, 0, 0, 0, 0, ""); err != nil {
		t.Fatal(err)
	}
	continued = ChatMessage{}
	if err := db.First(&continued, "id=?", continuedResponse).Error; err != nil || continued.Content != "resposta atual" {
		t.Fatalf("prefixo alterado: %+v %v", continued, err)
	}
}
