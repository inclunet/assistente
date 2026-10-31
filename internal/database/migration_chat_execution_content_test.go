package database

import (
	"context"
	"testing"
	"time"

	"gorm.io/gorm"
)

func executionSchemaFixture(t *testing.T) (*gorm.DB, *ChatTurnRepository, context.Context, string) {
	t.Helper()
	db := newChatTurnTestDB(t, false)
	seedChatTurnTestMessages(t, db, "conv", "question", "legacy-answer")
	if err := MigrateChatTurns(db); err != nil {
		t.Fatal(err)
	}
	if err := MigrateChatExecutionContent(db); err != nil {
		t.Fatal(err)
	}
	repo := NewChatTurnRepository(db)
	ctx := WithUserID(context.Background(), "user")
	attempt, err := repo.Begin(ctx, "conv", "question", "", TurnModeNew)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("UPDATE chat_turns SET current_execution_id=?,response_message_id=NULL WHERE user_message_id='question'", attempt.ExecutionID).Error; err != nil {
		t.Fatal(err)
	}
	return db, repo, ctx, attempt.ExecutionID
}

func insertExecutionMessage(db *gorm.DB, id, executionID, conversationID, turnID string) error {
	return db.Table("chat_messages").Create(map[string]any{
		"id": id, "conversation_id": conversationID, "turn_id": turnID,
		"role": "assistant", "content": id, "chat_execution_id": executionID,
		"created_at": time.Now(), "updated_at": time.Now(),
	}).Error
}

func TestExecutionContentMigrationKeepsLegacyUnattributed(t *testing.T) {
	db, _, _, _ := executionSchemaFixture(t)
	if err := MigrateChatExecutionContent(db); err != nil {
		t.Fatal(err)
	}
	var row struct {
		Content         string
		ChatExecutionID *string
	}
	if err := db.Table("chat_messages").Where("id='legacy-answer'").Take(&row).Error; err != nil {
		t.Fatal(err)
	}
	if row.Content != "antes" || row.ChatExecutionID != nil {
		t.Fatalf("legado reatribuído: %+v", row)
	}
	var violations []struct{ Table string }
	if err := db.Raw("PRAGMA foreign_key_check").Scan(&violations).Error; err != nil {
		t.Fatal(err)
	}
	if len(violations) != 0 {
		t.Fatalf("vínculos inválidos: %+v", violations)
	}
}

func TestExecutionContentSchemaRejectsCrossScopeAndReassignment(t *testing.T) {
	db, _, _, executionID := executionSchemaFixture(t)
	if err := insertExecutionMessage(db, "answer", executionID, "conv", "question"); err != nil {
		t.Fatal(err)
	}
	for _, sql := range []string{
		"UPDATE chat_messages SET chat_execution_id=NULL WHERE id='answer'",
		"UPDATE chat_messages SET conversation_id='other' WHERE id='answer'",
		"UPDATE chat_messages SET role='user' WHERE id='answer'",
		"UPDATE chat_messages SET parent_id='legacy-answer' WHERE id='answer'",
		"UPDATE chat_turn_attempts SET response_message_id='legacy-answer'",
		"UPDATE chat_turn_attempts SET base_response_message_id='legacy-answer'",
		"UPDATE chat_turn_attempts SET mode='continue'",
		"UPDATE chat_turns SET current_execution_id=NULL WHERE user_message_id='question'",
		"UPDATE chat_messages SET parent_id='legacy-answer' WHERE id='question'",
		"UPDATE chat_turns SET response_message_id='answer' WHERE user_message_id='question'",
	} {
		if err := db.Exec(sql).Error; err == nil {
			t.Fatalf("mutação inválida aceita: %s", sql)
		}
	}
	if err := insertExecutionMessage(db, "foreign", executionID, "other", "question"); err == nil {
		t.Fatal("mensagem cross-conversation aceita")
	}
	if err := db.Exec("UPDATE chat_turn_attempts SET response_message_id='answer' WHERE execution_id=?", executionID).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("UPDATE chat_turns SET response_message_id='answer' WHERE user_message_id='question'").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("UPDATE chat_messages SET content='edição explícita' WHERE id='answer'").Error; err != nil {
		t.Fatal(err)
	}
}

func TestExecutionContentSchemaMarksDeletedBaseUnavailable(t *testing.T) {
	db, repo, ctx, first := executionSchemaFixture(t)
	if err := insertExecutionMessage(db, "answer", first, "conv", "question"); err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("UPDATE chat_turn_attempts SET response_message_id='answer' WHERE execution_id=?", first).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("UPDATE chat_turns SET response_message_id='answer' WHERE user_message_id='question'").Error; err != nil {
		t.Fatal(err)
	}
	if err := repo.Finish(ctx, "conv", "question", first, TurnCompleted, "completed"); err != nil {
		t.Fatal(err)
	}
	if err := insertExecutionMessage(db, "late", first, "conv", "question"); err == nil {
		t.Fatal("escrita em execução encerrada aceita")
	}
	insert := `INSERT INTO chat_turn_attempts(execution_id,turn_id,mode,state,started_at,base_execution_id,base_response_message_id) VALUES (?, 'question','continue','preparing',CURRENT_TIMESTAMP,?,?)`
	if err := db.Exec(insert, "next", first, "answer").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("UPDATE chat_turn_attempts SET mode='retry' WHERE execution_id='next'").Error; err == nil {
		t.Fatal("continuação virou retry mantendo base")
	}
	if err := db.Exec("UPDATE chat_turn_attempts SET response_message_id=NULL WHERE execution_id=?", first).Error; err == nil {
		t.Fatal("representante limpo explicitamente")
	}
	if err := db.Exec("UPDATE chat_turn_attempts SET base_response_message_id=NULL WHERE execution_id='next'").Error; err == nil {
		t.Fatal("base limpa explicitamente")
	}

	if err := db.Exec(insert, "wrong", first, "legacy-answer").Error; err == nil {
		t.Fatal("base não corresponde à execução")
	}
	if err := db.Exec(insert, "cycle", "cycle", "answer").Error; err == nil {
		t.Fatal("base cíclica aceita")
	}
	if err := db.Exec("UPDATE chat_turns SET current_execution_id='next',active_execution_id='next',response_message_id=NULL,state='preparing' WHERE user_message_id='question'").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("DELETE FROM chat_messages WHERE id='answer'").Error; err != nil {
		t.Fatal(err)
	}
	var base struct {
		BaseResponseMessageID *string
		BaseExecutionID       *string
		Mode                  string
	}
	if err := db.Raw("SELECT base_response_message_id,base_execution_id,mode FROM chat_turn_attempts WHERE execution_id='next'").Scan(&base).Error; err != nil {
		t.Fatal(err)
	}
	if base.BaseResponseMessageID != nil || base.BaseExecutionID == nil || *base.BaseExecutionID != first || base.Mode != TurnModeContinue {
		t.Fatalf("base indisponível perdeu semântica: %+v", base)
	}
	// Restauração histórica pode declarar a mensagem-base indisponível,
	// mantendo a referência inequívoca à execução e sem escolher outro texto.
	if err := db.Exec(insert, "restored", first, nil).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("INSERT INTO chat_turn_attempts(execution_id,turn_id,mode,state,started_at,base_execution_id) VALUES ('invalid-retry','question','retry','preparing',CURRENT_TIMESTAMP,?)", first).Error; err == nil {
		t.Fatal("retry com cadeia-base aceito")
	}
	if err := db.Exec("DELETE FROM chat_messages WHERE id='question'").Error; err != nil {
		t.Fatal(err)
	}
}

func TestExecutionContentMigrationRollsBackOnFailure(t *testing.T) {
	db := newChatTurnTestDB(t, false)
	seedChatTurnTestMessages(t, db, "conv", "question", "answer")
	if err := MigrateChatTurns(db); err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("ALTER TABLE tool_invocations DROP COLUMN model_iteration").Error; err != nil {
		t.Fatal(err)
	}
	if err := MigrateChatExecutionContent(db); err == nil {
		t.Fatal("migração inválida aceita")
	}
	if db.Migrator().HasColumn("chat_messages", "chat_execution_id") || db.Migrator().HasColumn("chat_turns", "current_execution_id") {
		t.Fatal("migração parcial persistiu")
	}
}

func TestExecutionContentSchemaScopesInvocationsAndKeepsTerminalResults(t *testing.T) {
	db, repo, ctx, executionID := executionSchemaFixture(t)
	catalog := ToolCatalog{Name: "execution-probe", DisplayName: "Probe", Origin: "builtin"}
	if err := db.Create(&catalog).Error; err != nil {
		t.Fatal(err)
	}
	insert := func(id, userID, origin string) error {
		return db.Table("tool_invocations").Create(map[string]any{
			"id": id, "user_id": userID, "tool_catalog_id": catalog.ID,
			"origin_type": origin, "origin_id": "question", "conversation_id": "conv",
			"turn_id": "question", "tool_call_id": id, "status": "queued",
			"queued_at": time.Now(), "chat_execution_id": executionID,
		}).Error
	}
	if err := insert("invocation", "user", "chat"); err != nil {
		t.Fatal(err)
	}
	if err := insert("foreign", "other-user", "chat"); err == nil {
		t.Fatal("invocação de outro usuário aceita")
	}
	if err := insert("job", "user", "job_run"); err == nil {
		t.Fatal("execução do chat usada por job")
	}
	for _, statement := range []string{
		"UPDATE tool_invocations SET chat_execution_id=NULL WHERE id='invocation'",
		"UPDATE tool_invocations SET user_id='other-user' WHERE id='invocation'",
		"UPDATE tool_invocations SET turn_id='legacy-answer' WHERE id='invocation'",
	} {
		if err := db.Exec(statement).Error; err == nil {
			t.Fatalf("reatribuição aceita: %s", statement)
		}
	}
	if err := repo.Finish(ctx, "conv", "question", executionID, TurnCompleted, "completed"); err != nil {
		t.Fatal(err)
	}
	if err := insert("late", "user", "chat"); err == nil {
		t.Fatal("nova chamada de execução encerrada aceita")
	}
	// O resultado de uma chamada já registrada mantém sua identidade e pode
	// terminar depois do lifecycle; não vira chamada de uma execução nova.
	if err := db.Exec("UPDATE tool_invocations SET status='succeeded',output_preview='resultado' WHERE id='invocation'").Error; err != nil {
		t.Fatal(err)
	}
}

func TestExecutionContentSchemaSurvivesAutoMigrate(t *testing.T) {
	db, _, _, executionID := executionSchemaFixture(t)
	if err := insertExecutionMessage(db, "answer", executionID, "conv", "question"); err != nil {
		t.Fatal(err)
	}
	// O boot seguinte ainda usa os modelos anteriores à ativação dos writers.
	if err := db.AutoMigrate(&Conversation{}, &ChatMessage{}, &ToolCatalog{}, &ToolInvocation{}); err != nil {
		t.Fatal(err)
	}
	var stored string
	if err := db.Raw("SELECT chat_execution_id FROM chat_messages WHERE id='answer'").Scan(&stored).Error; err != nil {
		t.Fatal(err)
	}
	if stored != executionID {
		t.Fatal("AutoMigrate removeu proveniência")
	}
	if err := db.Exec("UPDATE chat_messages SET chat_execution_id=NULL WHERE id='answer'").Error; err == nil {
		t.Fatal("AutoMigrate removeu constraint")
	}
}
