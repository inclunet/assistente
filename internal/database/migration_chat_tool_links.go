package database

import (
	"fmt"
	"strings"

	"gorm.io/gorm"
)

// migrateChatToolLinks completa vínculos que a v18 não alcançou em invocações
// preexistentes. Falha é bloqueante: leitores canônicos não podem abrir um
// histórico parcialmente convertido. A transação preserva tudo em caso de erro.
func migrateChatToolLinks(db *gorm.DB) error {
	return db.Transaction(func(tx *gorm.DB) error {
		for _, sql := range []string{
			`CREATE INDEX IF NOT EXISTS idx_tool_invocations_chat_conversation ON tool_invocations(user_id, origin_type, conversation_id)`,
			`CREATE INDEX IF NOT EXISTS idx_tool_invocations_chat_turn ON tool_invocations(user_id, origin_type, turn_id, queued_at, id)`,
		} {
			if err := tx.Exec(sql).Error; err != nil {
				return err
			}
		}
		cursor := ""
		const batchSize = 256
		for {
			var rows []struct {
				ID                     string
				UserID                 string
				ConversationID         string
				TurnID                 string
				OriginMessageID        string
				OriginConversationID   string
				OriginOwner            string
				ResolvedTurnID         string
				ResolvedConversationID string
				ResolvedOwner          string
			}
			err := tx.Raw(`SELECT t.id, t.user_id, COALESCE(t.conversation_id, '') AS conversation_id,
    COALESCE(t.turn_id, '') AS turn_id, COALESCE(m.id, '') AS origin_message_id,
    COALESCE(m.conversation_id, '') AS origin_conversation_id, COALESCE(c.user_id, '') AS origin_owner,
    COALESCE(tm.id, '') AS resolved_turn_id, COALESCE(tm.conversation_id, '') AS resolved_conversation_id,
    COALESCE(tc.user_id, '') AS resolved_owner
    FROM tool_invocations t
    LEFT JOIN chat_messages m ON m.id = t.origin_id
    LEFT JOIN conversations c ON c.id = m.conversation_id
    LEFT JOIN chat_messages tm ON tm.id = COALESCE(NULLIF(t.turn_id, ''), NULLIF(m.turn_id, ''), m.id)
    LEFT JOIN conversations tc ON tc.id = tm.conversation_id
    WHERE t.id > ? AND t.origin_type = 'chat'
     AND (t.conversation_id IS NULL OR t.conversation_id = '' OR t.turn_id IS NULL OR t.turn_id = '')
    ORDER BY t.id LIMIT ?`, cursor, batchSize).Scan(&rows).Error
			if err != nil {
				return err
			}
			if len(rows) == 0 {
				return nil
			}
			for _, row := range rows {
				if strings.TrimSpace(row.UserID) == "" || row.ResolvedTurnID == "" || row.ResolvedConversationID == "" || row.ResolvedOwner != row.UserID ||
					(row.ConversationID != "" && row.ConversationID != row.ResolvedConversationID) ||
					(row.TurnID == "" && (row.OriginMessageID == "" || row.OriginOwner != row.UserID || row.OriginConversationID != row.ResolvedConversationID)) {
					return fmt.Errorf("vínculo de chat não recuperável na invocação %s; migração revertida, histórico preservado", row.ID)
				}
				// Só os vínculos mudam: timestamps, payloads e identidade permanecem intactos.
				if err := tx.Exec(`UPDATE tool_invocations SET conversation_id = ?, turn_id = ? WHERE id = ? AND user_id = ?`,
					row.ResolvedConversationID, row.ResolvedTurnID, row.ID, row.UserID).Error; err != nil {
					return err
				}
			}
			cursor = rows[len(rows)-1].ID
			if len(rows) < batchSize {
				return nil
			}
		}
	})
}
