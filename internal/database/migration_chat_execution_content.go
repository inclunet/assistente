package database

import (
	"fmt"

	"gorm.io/gorm"
)

// MigrateChatExecutionContent aplica a migração aditiva de proveniência.
// A proveniência é nullable para legado: não atribuir conteúdo a uma tentativa
// por horário, igualdade de texto ou posição. A migração precede os novos writers.
func MigrateChatExecutionContent(db *gorm.DB) error {
	return db.Transaction(func(tx *gorm.DB) error {
		columns := []struct{ table, name, definition string }{
			{"chat_messages", "chat_execution_id", "TEXT REFERENCES chat_turn_attempts(execution_id) ON DELETE CASCADE"},
			{"tool_invocations", "chat_execution_id", "TEXT REFERENCES chat_turn_attempts(execution_id) ON DELETE CASCADE"},
			{"chat_turns", "current_execution_id", "TEXT REFERENCES chat_turn_attempts(execution_id) DEFERRABLE INITIALLY DEFERRED"},
			{"chat_turn_attempts", "response_message_id", "TEXT REFERENCES chat_messages(id) ON DELETE SET NULL"},
			{"chat_turn_attempts", "base_execution_id", "TEXT REFERENCES chat_turn_attempts(execution_id) DEFERRABLE INITIALLY DEFERRED"},
			// Exclusão torna a mensagem-base indisponível; modo, execução-base e
			// prefixo já copiado continuam preservados.
			{"chat_turn_attempts", "base_response_message_id", "TEXT REFERENCES chat_messages(id) ON DELETE SET NULL"},
		}
		for _, column := range columns {
			if !tx.Migrator().HasColumn(column.table, column.name) {
				if err := tx.Exec(fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s", column.table, column.name, column.definition)).Error; err != nil {
					return err
				}
			}
		}
		for _, statement := range chatExecutionContentSchema {
			if err := tx.Exec(statement).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

var chatExecutionContentSchema = []string{
	`CREATE INDEX IF NOT EXISTS idx_chat_messages_execution ON chat_messages(chat_execution_id,id)`,
	`CREATE INDEX IF NOT EXISTS idx_tool_invocations_execution ON tool_invocations(chat_execution_id,model_iteration,queued_at,id)`,
	`CREATE TRIGGER IF NOT EXISTS chat_execution_message_insert BEFORE INSERT ON chat_messages
 WHEN NEW.chat_execution_id IS NOT NULL AND NOT EXISTS (
 SELECT 1 FROM chat_turn_attempts a JOIN chat_turns t ON t.user_message_id=a.turn_id
 JOIN chat_messages q ON q.id=t.user_message_id
 WHERE a.execution_id=NEW.chat_execution_id AND a.turn_id=NEW.turn_id
 AND t.active_execution_id=a.execution_id AND t.current_execution_id=a.execution_id AND t.conversation_id=NEW.conversation_id
 AND NEW.role='assistant' AND NEW.parent_id IS q.parent_id)
 BEGIN SELECT RAISE(ABORT,'invalid message execution'); END`,
	`CREATE TRIGGER IF NOT EXISTS chat_execution_message_update BEFORE UPDATE OF chat_execution_id,conversation_id,turn_id,parent_id,role ON chat_messages
 WHEN NEW.chat_execution_id IS NOT OLD.chat_execution_id OR
 (NEW.chat_execution_id IS NOT NULL AND NOT EXISTS (
 SELECT 1 FROM chat_turn_attempts a JOIN chat_turns t ON t.user_message_id=a.turn_id
 JOIN chat_messages q ON q.id=t.user_message_id
 WHERE a.execution_id=NEW.chat_execution_id AND a.turn_id=NEW.turn_id
 AND t.conversation_id=NEW.conversation_id AND NEW.role='assistant' AND NEW.parent_id IS q.parent_id))
 BEGIN SELECT RAISE(ABORT,'immutable message execution'); END`,
	`CREATE TRIGGER IF NOT EXISTS chat_execution_invocation_insert BEFORE INSERT ON tool_invocations
 WHEN NEW.chat_execution_id IS NOT NULL AND NOT EXISTS (
 SELECT 1 FROM chat_turn_attempts a JOIN chat_turns t ON t.user_message_id=a.turn_id
 JOIN conversations c ON c.id=t.conversation_id
 WHERE a.execution_id=NEW.chat_execution_id AND a.turn_id=NEW.turn_id
 AND t.active_execution_id=a.execution_id AND t.current_execution_id=a.execution_id AND t.conversation_id=NEW.conversation_id
 AND c.user_id=NEW.user_id AND NEW.origin_type='chat')
 BEGIN SELECT RAISE(ABORT,'invalid invocation execution'); END`,
	`CREATE TRIGGER IF NOT EXISTS chat_execution_invocation_update BEFORE UPDATE OF chat_execution_id,conversation_id,turn_id,user_id,origin_type ON tool_invocations
 WHEN NEW.chat_execution_id IS NOT OLD.chat_execution_id OR
 (NEW.chat_execution_id IS NOT NULL AND NOT EXISTS (
 SELECT 1 FROM chat_turn_attempts a JOIN chat_turns t ON t.user_message_id=a.turn_id
 JOIN conversations c ON c.id=t.conversation_id
 WHERE a.execution_id=NEW.chat_execution_id AND a.turn_id=NEW.turn_id
 AND t.conversation_id=NEW.conversation_id AND c.user_id=NEW.user_id AND NEW.origin_type='chat'))
 BEGIN SELECT RAISE(ABORT,'immutable invocation execution'); END`,
	`CREATE TRIGGER IF NOT EXISTS chat_execution_current_update BEFORE UPDATE OF current_execution_id,response_message_id,active_execution_id ON chat_turns
 WHEN (OLD.current_execution_id IS NOT NULL AND NEW.current_execution_id IS NULL)
 OR (NEW.current_execution_id IS NOT NULL AND NOT EXISTS (
 SELECT 1 FROM chat_turn_attempts a WHERE a.execution_id=NEW.current_execution_id AND a.turn_id=NEW.user_message_id
 AND (NEW.active_execution_id IS NULL OR NEW.active_execution_id=a.execution_id)
 AND (NEW.response_message_id IS NULL OR a.response_message_id=NEW.response_message_id)))
 BEGIN SELECT RAISE(ABORT,'invalid current execution'); END`,
	`CREATE TRIGGER IF NOT EXISTS chat_execution_current_insert BEFORE INSERT ON chat_turns
 WHEN NEW.current_execution_id IS NOT NULL
 BEGIN SELECT RAISE(ABORT,'current execution requires admitted turn'); END`,
	`CREATE TRIGGER IF NOT EXISTS chat_execution_response_update BEFORE UPDATE OF response_message_id ON chat_turn_attempts
 WHEN (OLD.response_message_id IS NOT NULL AND NEW.response_message_id IS NOT OLD.response_message_id
 AND (NEW.response_message_id IS NOT NULL OR EXISTS (SELECT 1 FROM chat_messages m WHERE m.id=OLD.response_message_id)))
 OR (NEW.response_message_id IS NOT NULL AND NOT EXISTS (
 SELECT 1 FROM chat_messages m WHERE m.id=NEW.response_message_id AND m.chat_execution_id=NEW.execution_id
 AND m.turn_id=NEW.turn_id AND m.role='assistant'))
 BEGIN SELECT RAISE(ABORT,'invalid execution response'); END`,
	`CREATE TRIGGER IF NOT EXISTS chat_execution_attempt_insert BEFORE INSERT ON chat_turn_attempts
 WHEN NEW.response_message_id IS NOT NULL
 OR (NEW.base_execution_id IS NOT NULL AND (NEW.mode<>'continue' OR NOT EXISTS (
 SELECT 1 FROM chat_turn_attempts b WHERE b.execution_id=NEW.base_execution_id AND b.turn_id=NEW.turn_id
 AND b.state NOT IN ('preparing','streaming')
 AND (NEW.base_response_message_id IS NULL OR b.response_message_id=NEW.base_response_message_id))))
 OR (NEW.base_response_message_id IS NOT NULL AND (NEW.mode<>'continue' OR NOT EXISTS (
 SELECT 1 FROM chat_messages m JOIN chat_messages q ON q.id=NEW.turn_id
 WHERE m.id=NEW.base_response_message_id AND m.role='assistant' AND m.turn_id=NEW.turn_id
 AND m.conversation_id=q.conversation_id AND m.parent_id IS q.parent_id
 AND m.chat_execution_id IS NEW.base_execution_id)))
 BEGIN SELECT RAISE(ABORT,'invalid continuation base'); END`,
	`CREATE TRIGGER IF NOT EXISTS chat_execution_base_immutable BEFORE UPDATE OF base_execution_id,base_response_message_id,execution_id,turn_id,mode ON chat_turn_attempts
 WHEN NEW.base_execution_id IS NOT OLD.base_execution_id
 OR (NEW.base_response_message_id IS NOT OLD.base_response_message_id
 AND (NEW.base_response_message_id IS NOT NULL OR EXISTS (SELECT 1 FROM chat_messages m WHERE m.id=OLD.base_response_message_id)))
 OR NEW.mode IS NOT OLD.mode OR NEW.execution_id IS NOT OLD.execution_id OR NEW.turn_id IS NOT OLD.turn_id
 BEGIN SELECT RAISE(ABORT,'immutable execution base'); END`,
	`CREATE TRIGGER IF NOT EXISTS chat_execution_question_scope BEFORE UPDATE OF parent_id ON chat_messages
 WHEN NEW.parent_id IS NOT OLD.parent_id AND EXISTS (
 SELECT 1 FROM chat_messages m WHERE m.turn_id=OLD.id AND m.chat_execution_id IS NOT NULL AND m.parent_id IS NOT NEW.parent_id)
 BEGIN SELECT RAISE(ABORT,'invalid execution scope'); END`,
}
