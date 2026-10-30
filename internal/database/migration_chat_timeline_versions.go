package database

import (
	"fmt"
	"strings"

	"gorm.io/gorm"
)

// Metadado de invalidação de storage, não sequência dos eventos de streaming.
// Nenhum writer precisa lembrar de invalidar separadamente do seu commit.
func migrateChatTimelineVersions(database *gorm.DB) error {
	return database.Transaction(func(tx *gorm.DB) error {
		statements := []string{
			`CREATE INDEX IF NOT EXISTS idx_timeline_catalog_chat ON tool_invocations(tool_catalog_id) WHERE origin_type='chat'`,
			`CREATE TABLE IF NOT EXISTS chat_timeline_versions (
    conversation_id TEXT PRIMARY KEY NOT NULL REFERENCES conversations(id) ON DELETE CASCADE ON UPDATE CASCADE,
    incarnation TEXT NOT NULL CHECK(length(incarnation) = 32),
    revision INTEGER NOT NULL DEFAULT 0 CHECK(typeof(revision) = 'integer' AND revision >= 0)
   )`,
			`INSERT INTO chat_timeline_versions(conversation_id, incarnation, revision)
    SELECT id, lower(hex(randomblob(16))), 0 FROM conversations
    WHERE id NOT IN (SELECT conversation_id FROM chat_timeline_versions)`,
			`CREATE TRIGGER IF NOT EXISTS chat_timeline_conversation_insert AFTER INSERT ON conversations
    BEGIN
     INSERT INTO chat_timeline_versions(conversation_id, incarnation, revision)
     VALUES (NEW.id, lower(hex(randomblob(16))), 0)
     ON CONFLICT(conversation_id) DO UPDATE SET incarnation=excluded.incarnation, revision=0;
    END`,
			`CREATE TRIGGER IF NOT EXISTS chat_timeline_conversation_delete AFTER DELETE ON conversations
    BEGIN DELETE FROM chat_timeline_versions WHERE conversation_id=OLD.id; END`,
		}
		for _, statement := range statements {
			if err := tx.Exec(statement).Error; err != nil {
				return err
			}
		}
		// Predicados usam somente tabelas/expressões internas. UPDATE invalida uma vez
		// cada conversa afetada, mesmo quando origem e destino são iguais.
		sources := []struct{ table, oldScope, newScope string }{
			{"conversations", "conversation_id=OLD.id", "conversation_id=NEW.id"},
			{"chat_messages", "conversation_id=OLD.conversation_id", "conversation_id=NEW.conversation_id"},
			{"tool_invocations", "OLD.origin_type='chat' AND conversation_id IN (" + timelineInvocationConversations("OLD") + ")", "NEW.origin_type='chat' AND conversation_id IN (" + timelineInvocationConversations("NEW") + ")"},
			{"tool_catalog", timelineCatalogScope("OLD"), timelineCatalogScope("NEW")},
			{"chat_turns", "conversation_id=OLD.conversation_id", "conversation_id=NEW.conversation_id"},
			{"chat_turn_attempts", "conversation_id=(SELECT conversation_id FROM chat_turns WHERE user_message_id=OLD.turn_id)", "conversation_id=(SELECT conversation_id FROM chat_turns WHERE user_message_id=NEW.turn_id)"},
		}
		for _, source := range sources {
			// Enumerar o schema real também cobre campos legados preservados no upgrade.
			// Migrações futuras que acrescentem campos devem reinstalar estes gatilhos.
			columns, err := tx.Migrator().ColumnTypes(source.table)
			if err != nil {
				return err
			}
			changes := make([]string, 0, len(columns))
			for _, column := range columns {
				// Descoberta de ferramentas renova updated_at mesmo sem mudança
				// visual. Somente estes campos do catálogo entram nos resumos.
				if source.table == "tool_catalog" && column.Name() != "id" && column.Name() != "name" && column.Name() != "display_name" && column.Name() != "origin" {
					continue
				}
				name := `"` + strings.ReplaceAll(column.Name(), `"`, `""`) + `"`
				changes = append(changes, "OLD."+name+" IS NOT NEW."+name)
			}
			for _, operation := range []string{"insert", "update", "delete"} {
				if source.table == "conversations" && operation != "update" {
					continue
				}
				predicate, condition := source.newScope, ""
				switch operation {
				case "delete":
					predicate = source.oldScope
				case "update":
					predicate = "(" + source.oldScope + ") OR (" + source.newScope + ")"
					condition = " WHEN " + strings.Join(changes, " OR ")
				}
				name := "chat_timeline_" + source.table + "_" + operation
				// Reinstalação idempotente conserva incarnation/revision existentes.
				if err := tx.Exec("DROP TRIGGER IF EXISTS " + name).Error; err != nil {
					return err
				}
				statement := fmt.Sprintf(`CREATE TRIGGER %s AFTER %s ON %s%s
     BEGIN UPDATE chat_timeline_versions SET revision=revision+1 WHERE %s; END`, name, operation, source.table, condition, predicate)
				if err := tx.Exec(statement).Error; err != nil {
					return err
				}
			}
		}
		return nil
	})
}

// O leitor de resumos usa user_id+turn_id, inclusive em registros cujo vínculo
// conversation_id é nulo. Invalidar todas as conversas que podem projetá-los.
func timelineInvocationConversations(alias string) string {
	return `SELECT c.id FROM conversations c WHERE c.id=` + alias + `.conversation_id AND c.user_id=` + alias + `.user_id
  UNION SELECT c.id FROM chat_messages m JOIN conversations c ON c.id=m.conversation_id
   WHERE m.id=` + alias + `.turn_id AND c.user_id=` + alias + `.user_id
  UNION SELECT c.id FROM chat_messages m JOIN conversations c ON c.id=m.conversation_id
   WHERE m.turn_id=` + alias + `.turn_id AND c.user_id=` + alias + `.user_id`
}

func timelineCatalogScope(alias string) string {
	// UNION elimina conversas repetidas antes de incrementar. Separar os vínculos
	// permite buscas por PK/turn_id; não cruzar cada invocação com todas as conversas.
	filter := ` WHERE i.origin_type='chat' AND i.tool_catalog_id=` + alias + `.id`
	return `conversation_id IN (
  SELECT c.id FROM tool_invocations i JOIN conversations c ON c.id=i.conversation_id AND c.user_id=i.user_id` + filter + `
  UNION SELECT c.id FROM tool_invocations i JOIN chat_messages m ON m.id=i.turn_id
   JOIN conversations c ON c.id=m.conversation_id AND c.user_id=i.user_id` + filter + `
  UNION SELECT c.id FROM tool_invocations i JOIN chat_messages m ON m.turn_id=i.turn_id
   JOIN conversations c ON c.id=m.conversation_id AND c.user_id=i.user_id` + filter + `)`
}
