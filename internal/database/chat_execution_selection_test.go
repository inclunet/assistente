package database

import (
	"reflect"
	"testing"
	"time"

	"gorm.io/gorm"
)

func selectExecutionFixture(t *testing.T, db *gorm.DB, id, mode string, baseExecution, baseMessage any) {
	t.Helper()
	if err := db.Exec(`INSERT INTO chat_turn_attempts(execution_id,turn_id,mode,state,started_at,base_execution_id,base_response_message_id)
 VALUES (?,'question',?,'preparing',CURRENT_TIMESTAMP,?,?)`, id, mode, baseExecution, baseMessage).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("UPDATE chat_turns SET current_execution_id=?,active_execution_id=?,response_message_id=NULL,state='preparing' WHERE user_message_id='question'", id, id).Error; err != nil {
		t.Fatal(err)
	}
}

func bindExecutionFixture(t *testing.T, db *gorm.DB, executionID, responseID string) {
	t.Helper()
	if err := db.Exec("UPDATE chat_turn_attempts SET response_message_id=? WHERE execution_id=?", responseID, executionID).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("UPDATE chat_turns SET response_message_id=? WHERE user_message_id='question'", responseID).Error; err != nil {
		t.Fatal(err)
	}
}

func selectedMessageIDs(messages []ChatMessage) []string {
	ids := make([]string, 0, len(messages))
	for _, message := range messages {
		ids = append(ids, message.ID)
	}
	return ids
}

func TestExecutionSelectionRetryAndContinuationBeforeLimits(t *testing.T) {
	db, turns, ctx, first := executionSchemaFixture(t)
	for _, id := range []string{"a-mid", "a-final"} {
		if err := insertExecutionMessage(db, id, first, "conv", "question"); err != nil {
			t.Fatal(err)
		}
	}
	bindExecutionFixture(t, db, first, "a-final")
	if err := turns.Finish(ctx, "conv", "question", first, TurnCompleted, "completed"); err != nil {
		t.Fatal(err)
	}
	selectExecutionFixture(t, db, "retry", TurnModeRetry, nil, nil)
	for _, id := range []string{"b-mid", "b-final"} {
		if err := insertExecutionMessage(db, id, "retry", "conv", "question"); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Exec("UPDATE chat_messages SET created_at=? WHERE id='b-final'", time.Now().Add(time.Second)).Error; err != nil {
		t.Fatal(err)
	}
	bindExecutionFixture(t, db, "retry", "b-final")
	if err := turns.Finish(ctx, "conv", "question", "retry", TurnCompleted, "completed"); err != nil {
		t.Fatal(err)
	}
	repo := NewMessageRepository(db)
	messages, err := repo.GetMessagesWithContext(ctx, "conv", nil)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(selectedMessageIDs(messages), []string{"question", "b-mid", "b-final"}) {
		t.Fatalf("retry misturou execuções: %v", selectedMessageIDs(messages))
	}
	all, err := repo.GetAllConversationMessagesWithContext(ctx, "conv")
	if err != nil || len(all) != 6 {
		t.Fatalf("auditoria perdeu conteúdo: %v %v", selectedMessageIDs(all), err)
	}
	selectExecutionFixture(t, db, "continue", TurnModeContinue, "retry", "b-final")
	if err := insertExecutionMessage(db, "c-final", "continue", "conv", "question"); err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("UPDATE chat_messages SET created_at=? WHERE id='c-final'", time.Now().Add(2*time.Second)).Error; err != nil {
		t.Fatal(err)
	}
	bindExecutionFixture(t, db, "continue", "c-final")
	messages, err = repo.GetRecentRootMessagesWithContext(ctx, "conv", 2)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(selectedMessageIDs(messages), []string{"b-mid", "c-final"}) {
		t.Fatalf("limite aplicado antes da seleção: %v", selectedMessageIDs(messages))
	}
	window, err := repo.LoadHistoryWindowWithContext(ctx, "conv", 2)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(selectedMessageIDs(window.Messages), []string{"question", "b-mid", "c-final"}) {
		t.Fatalf("contexto divergiu: %v", selectedMessageIDs(window.Messages))
	}
	page, err := repo.GetMessageWindowWithContext(ctx, MessageWindowQuery{ConversationID: "conv", Anchor: "end", Direction: "before", Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	if page.TotalCount != 2 || !reflect.DeepEqual(selectedMessageIDs(page.Messages), []string{"question", "b-mid", "c-final"}) {
		t.Fatalf("janela divergiu: %+v %v", page.Items, selectedMessageIDs(page.Messages))
	}
	if err := turns.Finish(ctx, "conv", "question", "continue", TurnCompleted, "completed"); err != nil {
		t.Fatal(err)
	}
	selectExecutionFixture(t, db, "chain", TurnModeContinue, "continue", "c-final")
	if err := insertExecutionMessage(db, "d-final", "chain", "conv", "question"); err != nil {
		t.Fatal(err)
	}
	bindExecutionFixture(t, db, "chain", "d-final")
	messages, err = repo.GetMessagesByTurnIDWithContext(ctx, "conv", nil, "question", 0)
	if err != nil || !reflect.DeepEqual(selectedMessageIDs(messages), []string{"b-mid", "d-final"}) {
		t.Fatalf("cadeia duplicou prefixos: %v %v", selectedMessageIDs(messages), err)
	}
}

func TestExecutionSelectionLegacyBaseAndUnavailableBase(t *testing.T) {
	db, turns, ctx, first := executionSchemaFixture(t)
	if err := turns.Finish(ctx, "conv", "question", first, TurnFailed, "provider_error"); err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&ChatMessage{UUIDModel: UUIDModel{ID: "legacy-mid"}, ConversationID: "conv", TurnID: stringPointer("question"), Role: "assistant", Content: "legacy"}).Error; err != nil {
		t.Fatal(err)
	}
	selectExecutionFixture(t, db, "legacy-continuation", TurnModeContinue, nil, "legacy-answer")
	if err := insertExecutionMessage(db, "new-final", "legacy-continuation", "conv", "question"); err != nil {
		t.Fatal(err)
	}
	bindExecutionFixture(t, db, "legacy-continuation", "new-final")
	repo := NewMessageRepository(db)
	messages, err := repo.GetMessagesByTurnIDWithContext(ctx, "conv", nil, "question", 0)
	if err != nil || !reflect.DeepEqual(selectedMessageIDs(messages), []string{"legacy-mid", "new-final"}) {
		t.Fatalf("base legada: %v %v", selectedMessageIDs(messages), err)
	}
	if err := db.Exec("DELETE FROM chat_messages WHERE id='legacy-answer'").Error; err != nil {
		t.Fatal(err)
	}
	messages, err = repo.GetMessagesByTurnIDWithContext(ctx, "conv", nil, "question", 0)
	if err != nil || !reflect.DeepEqual(selectedMessageIDs(messages), []string{"new-final"}) {
		t.Fatalf("base indisponível inferiu herança: %v %v", selectedMessageIDs(messages), err)
	}
}

// As fixtures de leitores precisam das mesmas dependências de schema que o boot.
func migrateExecutionSelectionFixture(t testing.TB, db *gorm.DB) {
	t.Helper()
	if err := db.AutoMigrate(&ToolCatalog{}, &ToolInvocation{}); err != nil {
		t.Fatal(err)
	}
	if err := MigrateChatTurns(db); err != nil {
		t.Fatal(err)
	}
	if err := MigrateChatExecutionContent(db); err != nil {
		t.Fatal(err)
	}
}
