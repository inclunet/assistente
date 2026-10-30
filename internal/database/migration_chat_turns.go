package database

import (
	"context"
	"fmt"
	"os"
	"strings"

	"assistente/internal/logging"
	"gorm.io/gorm"
)

// As tabelas são criadas dentro da transação da migração, não no AutoMigrate:
// criação, constraints e backfill devem reverter juntos (AEP-0116).
var chatTurnSchema = []string{
	`CREATE TABLE IF NOT EXISTS chat_turns (
 user_message_id TEXT PRIMARY KEY NOT NULL REFERENCES chat_messages(id) ON DELETE CASCADE,
 conversation_id TEXT NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
 response_message_id TEXT REFERENCES chat_messages(id) ON DELETE SET NULL,
 state TEXT NOT NULL CHECK (state IN ('preparing','streaming','completed','interrupted','failed','historical_unknown')),
 active_execution_id TEXT,
 origin TEXT NOT NULL CHECK (origin IN ('new','legacy','imported')),
 CHECK ((state IN ('preparing','streaming') AND active_execution_id IS NOT NULL) OR
        (state NOT IN ('preparing','streaming') AND active_execution_id IS NULL)),
 FOREIGN KEY (active_execution_id,user_message_id) REFERENCES chat_turn_attempts(execution_id,turn_id) DEFERRABLE INITIALLY DEFERRED
)`,
	`CREATE TABLE IF NOT EXISTS chat_turn_attempts (
 execution_id TEXT PRIMARY KEY NOT NULL,
 turn_id TEXT NOT NULL REFERENCES chat_turns(user_message_id) ON DELETE CASCADE,
 mode TEXT NOT NULL CHECK (mode IN ('new','retry','continue')),
 state TEXT NOT NULL CHECK (state IN ('preparing','streaming','completed','interrupted','failed')),
 started_at DATETIME NOT NULL,
 finished_at DATETIME,
 terminal_reason TEXT NOT NULL DEFAULT '',
 UNIQUE (execution_id,turn_id),
 CHECK ((state IN ('preparing','streaming') AND finished_at IS NULL AND terminal_reason = '') OR
        (finished_at IS NOT NULL AND (
          (state = 'completed' AND terminal_reason = 'completed') OR
          (state = 'interrupted' AND terminal_reason IN ('cancelled','restart','output_limit','limit_reached')) OR
          (state = 'failed' AND terminal_reason IN ('provider_error','persistence_error','internal_error')))))
)`,
	`CREATE INDEX IF NOT EXISTS idx_chat_turns_conversation ON chat_turns(conversation_id,user_message_id)`,
	`CREATE INDEX IF NOT EXISTS idx_chat_turn_attempts_turn ON chat_turn_attempts(turn_id,started_at,execution_id)`,
}

const chatTurnLinksValid = `EXISTS (SELECT 1 FROM chat_messages q WHERE q.id = NEW.user_message_id
 AND q.conversation_id = NEW.conversation_id AND q.role = 'user')
 AND (NEW.response_message_id IS NULL OR EXISTS (
 SELECT 1 FROM chat_messages r JOIN chat_messages q ON q.id = NEW.user_message_id
 WHERE r.id = NEW.response_message_id AND r.conversation_id = NEW.conversation_id
 AND r.role = 'assistant' AND r.turn_id = NEW.user_message_id AND r.parent_id IS q.parent_id))`

func createChatTurnSchema(tx *gorm.DB) error {
	for _, statement := range chatTurnSchema {
		if err := tx.Exec(statement).Error; err != nil {
			return err
		}
	}
	for _, operation := range []string{"INSERT", "UPDATE"} {
		statement := fmt.Sprintf(`CREATE TRIGGER IF NOT EXISTS chat_turns_validate_%s BEFORE %s ON chat_turns
 WHEN NOT (%s) BEGIN SELECT RAISE(ABORT, 'invalid chat turn links'); END`, strings.ToLower(operation), operation, chatTurnLinksValid)
		if err := tx.Exec(statement).Error; err != nil {
			return err
		}
	}
	// A referência também deve continuar válida se um escritor direto alterar a mensagem.
	return tx.Exec(`CREATE TRIGGER IF NOT EXISTS chat_turns_validate_message_update
 BEFORE UPDATE OF id,conversation_id,role,turn_id,parent_id ON chat_messages
 WHEN EXISTS (SELECT 1 FROM chat_turns t WHERE
 (t.user_message_id = OLD.id AND (NEW.id IS NOT OLD.id OR NEW.conversation_id IS NOT t.conversation_id OR NEW.role <> 'user'
   OR (t.response_message_id IS NOT NULL AND NEW.parent_id IS NOT (SELECT parent_id FROM chat_messages WHERE id=t.response_message_id))))
 OR (t.response_message_id = OLD.id AND (NEW.id IS NOT OLD.id OR NEW.conversation_id IS NOT t.conversation_id OR NEW.role <> 'assistant'
   OR NEW.turn_id IS NOT t.user_message_id OR NEW.parent_id IS NOT (SELECT parent_id FROM chat_messages WHERE id=t.user_message_id))))
 BEGIN SELECT RAISE(ABORT, 'message update invalidates chat turn'); END`).Error
}

func migrateChatTurns(db *gorm.DB) error {
	// Falha no backup bloqueia a migração. Bancos em memória não têm arquivo a copiar.
	if err := backupChatTurns(db); err != nil {
		return err
	}
	return db.Transaction(func(tx *gorm.DB) error {
		if err := createChatTurnSchema(tx); err != nil {
			return err
		}
		if err := backfillChatTurns(tx); err != nil {
			return err
		}
		for _, table := range []string{"chat_turns", "chat_turn_attempts"} {
			var violations []map[string]any
			if err := tx.Raw("PRAGMA foreign_key_check(" + table + ")").Scan(&violations).Error; err != nil {
				return err
			}
			if len(violations) != 0 {
				return fmt.Errorf("vínculos inválidos em %s; migração de turnos revertida", table)
			}
		}
		return nil
	})
}

func backupChatTurns(db *gorm.DB) error {
	sqlDB, err := db.DB()
	if err != nil {
		return err
	}
	path, err := sqliteMainDatabasePath(sqlDB)
	if err != nil || path == "" || path == ":memory:" {
		return err
	}
	backup := path + ".pre-chat-turns-v41.bak"
	if err := archivePreviousToolLedgerBackup(backup); err != nil {
		return err
	}
	if err := verifyToolLedgerBackupSpace(path, backup); err != nil {
		return err
	}
	if _, err := sqlDB.Exec(`VACUUM INTO '` + strings.ReplaceAll(backup, "'", "''") + `'`); err != nil {
		return err
	}
	if err := os.Chmod(backup, 0600); err != nil {
		return err
	}
	digest, size, err := fileSHA256(backup)
	if err != nil {
		return err
	}
	if size == 0 {
		return fmt.Errorf("backup de turnos vazio")
	}
	return os.WriteFile(backup+".manifest", []byte(fmt.Sprintf("sha256=%s\nbytes=%d\n", digest, size)), 0600)
}

func backfillChatTurns(tx *gorm.DB) error {
	const batchSize = 128
	cursor := ""
	var orphaned int64
	if err := tx.Model(&ChatMessage{}).Where("role = 'user' AND NOT EXISTS (SELECT 1 FROM conversations c WHERE c.id = chat_messages.conversation_id)").Count(&orphaned).Error; err != nil {
		return err
	}
	if orphaned > 0 {
		logging.Warnf(context.Background(), "database.chat-turns", "%d perguntas sem conversa preservadas fora dos metadados de turno", orphaned)
	}
	for {
		var questions []ChatMessage
		if err := tx.Select("id,conversation_id,parent_id").Where("role = 'user' AND id > ? AND EXISTS (SELECT 1 FROM conversations c WHERE c.id = chat_messages.conversation_id)", cursor).
			Order("id").Limit(batchSize).Find(&questions).Error; err != nil {
			return err
		}
		if len(questions) == 0 {
			break
		}
		ids := make([]string, len(questions))
		for i, question := range questions {
			ids[i] = question.ID
		}
		var messages []ChatMessage
		if err := tx.Select("id,conversation_id,parent_id,turn_id,role,created_at,updated_at,total_tokens").
			Where("turn_id IN ? AND role <> 'user'", ids).Order("created_at,id").Find(&messages).Error; err != nil {
			return err
		}
		intermediate, err := legacyTurnIntermediateIDs(tx, ids)
		if err != nil {
			return err
		}
		byTurn := map[string][]ChatMessage{}
		for _, message := range messages {
			byTurn[*message.TurnID] = append(byTurn[*message.TurnID], message)
		}
		for _, question := range questions {
			valid := make([]ChatMessage, 0, len(byTurn[question.ID]))
			for _, message := range byTurn[question.ID] {
				if message.ConversationID == question.ConversationID && equalNullableID(message.ParentID, question.ParentID) {
					valid = append(valid, message)
				}
			}
			representative := SelectLegacyTurnRepresentative(valid, intermediate[question.ID])
			var responseID *string
			if representative.Role == "assistant" {
				responseID = &representative.ID
			}
			// Reexecução após crash entre commit e schema_migrations não reclassifica estado ativo.
			if err := tx.Exec(`INSERT INTO chat_turns(user_message_id,conversation_id,response_message_id,state,origin)
 VALUES (?,?,?,'historical_unknown','legacy') ON CONFLICT(user_message_id) DO NOTHING`, question.ID, question.ConversationID, responseID).Error; err != nil {
				return err
			}
		}
		cursor = questions[len(questions)-1].ID
	}
	var invalid int64
	if err := tx.Raw(`SELECT count(*) FROM chat_messages m LEFT JOIN chat_messages q ON q.id=m.turn_id
 WHERE m.role <> 'user' AND m.turn_id IS NOT NULL AND trim(m.turn_id) <> '' AND
 (q.id IS NULL OR q.role <> 'user' OR q.conversation_id IS NOT m.conversation_id OR q.parent_id IS NOT m.parent_id)`).Scan(&invalid).Error; err != nil {
		return err
	}
	if invalid > 0 {
		logging.Warnf(context.Background(), "database.chat-turns", "%d mensagens com vínculo legado inválido preservadas fora dos metadados de turno", invalid)
	}
	return nil
}

func equalNullableID(a, b *string) bool {
	return (a == nil && b == nil) || (a != nil && b != nil && *a == *b)
}

// Espelha somente a seleção das referências leves da projeção legada: nenhuma
// leitura de input/output. A maior tentativa vence antes de classificar a mensagem.
func legacyTurnIntermediateIDs(tx *gorm.DB, ids []string) (map[string]map[string]bool, error) {
	var rows []struct {
		TurnID, AssistantID, Origin, CatalogOrigin string
		Offset                                     *int
	}
	err := tx.Raw(`WITH ranked AS (
 SELECT t.turn_id,t.metadata, c.origin AS catalog_origin,
 row_number() OVER (PARTITION BY t.turn_id,t.tool_call_id ORDER BY t.attempt DESC,t.queued_at DESC,t.id DESC) AS rank
 FROM tool_invocations t JOIN chat_messages q ON q.id=t.turn_id JOIN conversations v ON v.id=q.conversation_id
 LEFT JOIN tool_catalog c ON c.id=t.tool_catalog_id
 WHERE t.origin_type='chat' AND t.user_id=v.user_id AND t.turn_id IN ? AND trim(t.tool_call_id) <> ''
 AND (t.completed_at IS NOT NULL OR t.status IN ('queued','running','succeeded','failed','cancelled','timed_out')))
 SELECT turn_id,
 CASE WHEN json_valid(metadata) THEN COALESCE(CAST(json_extract(metadata,'$.display.assistant_message_id') AS TEXT),'') ELSE '' END AS assistant_id,
 CASE WHEN json_valid(metadata) THEN COALESCE(CAST(json_extract(metadata,'$.display.origin') AS TEXT),'') ELSE '' END AS origin,
 COALESCE(catalog_origin,'') AS catalog_origin,
 CASE WHEN json_valid(metadata) THEN CASE WHEN json_type(metadata,'$.display.acp_text_offset')='integer' THEN json_extract(metadata,'$.display.acp_text_offset') END END AS offset
 FROM ranked WHERE rank=1`, ids).Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	result := map[string]map[string]bool{}
	for _, row := range rows {
		id := strings.TrimSpace(row.AssistantID) // compatibilidade exata com o consolidador legado
		origin := strings.TrimSpace(row.Origin)
		if origin == "" {
			origin = strings.TrimSpace(row.CatalogOrigin)
		}
		if id == "" || origin == "mcp_native" || (origin == "acp_agent" && row.Offset != nil) {
			continue
		}
		if result[row.TurnID] == nil {
			result[row.TurnID] = map[string]bool{}
		}
		result[row.TurnID][id] = true
	}
	return result, nil
}
