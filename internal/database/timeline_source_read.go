package database

import (
	"database/sql"
	"slices"
	"strings"

	"gorm.io/gorm"
)

func timelineSourceBytesSQL(table string, columns ...string) string {
	parts := make([]string, len(columns))
	for i, column := range columns {
		parts[i] = "COALESCE(LENGTH(CAST(" + table + "." + column + " AS BLOB)),0)"
	}
	return strings.Join(parts, "+")
}

// Uma única seleção de identidades é compartilhada pela soma e pelo payload.
// A materialização intermediária não contém texto, raciocínio nem blobs.
var timelineSourceReadCTE = func() string {
	messageBytes := timelineSourceBytesSQL("chat_messages", "id", "conversation_id", "parent_id", "turn_id", "chat_execution_id", "role", "content", "reasoning", "model", "source", "created_at", "updated_at")
	scopeBytes := timelineSourceBytesSQL("chat_messages", "id", "conversation_id", "role", "parent_id", "created_at")
	turnBytes := timelineSourceBytesSQL("chat_turns", "user_message_id", "conversation_id", "response_message_id", "current_execution_id", "active_execution_id", "state", "origin")
	callBytes := timelineSourceBytesSQL("tool_invocations", "id", "user_id", "conversation_id", "turn_id", "chat_execution_id", "tool_catalog_id", "origin_id", "parent_invocation_id", "metadata", "input_preview", "output_preview", "display_name", "tool_call_id", "result_availability", "status") + "+" + timelineSourceBytesSQL("selected_chat_turn", "response_message_id")
	catalogBytes := timelineSourceBytesSQL("tool_catalog", "name", "display_name", "origin")
	selection := ChatInvocationSelection{}
	return `WITH authorized_conversation AS MATERIALIZED (
 SELECT id FROM conversations WHERE id=? AND user_id=?
 ), admitted_sources AS MATERIALIZED (
 SELECT chat_messages.id,1024+` + messageBytes + ` AS source_bytes FROM (
 SELECT id FROM chat_messages WHERE id IN (SELECT CAST(unhex(value) AS TEXT) FROM json_each(?))
 UNION SELECT m.id FROM chat_messages m CROSS JOIN chat_messages q ON q.id=m.turn_id
 WHERE m.turn_id IN (SELECT CAST(unhex(value) AS TEXT) FROM json_each(?))
 AND m.conversation_id=q.conversation_id AND m.parent_id IS q.parent_id AND m.role<>'user' AND q.role='user'
 ) selected CROSS JOIN chat_messages ON chat_messages.id=selected.id
 WHERE chat_messages.conversation_id=(SELECT id FROM authorized_conversation) AND ` + CurrentChatMessageSQL("chat_messages") + `
 ), message_budget(size) AS MATERIALIZED (
 SELECT (SELECT COALESCE(SUM(source_bytes),0) FROM admitted_sources)
 + (SELECT COALESCE(SUM(1024+` + scopeBytes + `),0) FROM
 (SELECT CAST(unhex(value) AS TEXT) AS scope_id FROM json_each(?)) selected_scope
 CROSS JOIN chat_messages ON chat_messages.id=selected_scope.scope_id
 WHERE conversation_id=authorized_conversation.id)
 + (SELECT COALESCE(SUM(1024+` + turnBytes + `),0) FROM chat_turns
 WHERE conversation_id=authorized_conversation.id AND user_message_id IN (SELECT CAST(unhex(value) AS TEXT) FROM json_each(?)))
 FROM authorized_conversation
 ), total_budget(size) AS MATERIALIZED (
 SELECT CASE WHEN size > ? THEN size ELSE size + (
 SELECT COALESCE(SUM(1024+` + callBytes + `+` + catalogBytes + `),0)
 FROM tool_invocations LEFT JOIN tool_catalog ON tool_catalog.id=tool_invocations.tool_catalog_id
 ` + selection.Join("tool_invocations") + ` WHERE ` + selection.Predicate("tool_invocations") + `
 AND tool_invocations.user_id=? AND tool_invocations.origin_type='chat'
 AND tool_invocations.turn_id IN (SELECT CAST(unhex(value) AS TEXT) FROM json_each(?))
 ) END FROM message_budget
 ) `
}()

func timelineSourceReadArgs(tx *gorm.DB, conversationID string, messageIDs, turnIDs []string) ([]interface{}, error) {
	if err := CheckTimelineIdentityBudget(messageIDs, turnIDs); err != nil {
		return nil, err
	}
	userID, err := RequireUserID(tx.Statement.Context)
	if err != nil {
		return nil, err
	}
	turns := EncodeTimelineIDSet(turnIDs)
	messages, metadata := turns, turns
	if !slices.Equal(messageIDs, turnIDs) {
		messages = EncodeTimelineIDSet(messageIDs)
		ids := make([]string, 0, len(messageIDs)+len(turnIDs))
		seen := make(map[string]bool, len(messageIDs)+len(turnIDs))
		for _, set := range [][]string{messageIDs, turnIDs} {
			for _, id := range set {
				if !seen[id] {
					seen[id] = true
					ids = append(ids, id)
				}
			}
		}
		metadata = EncodeTimelineIDSet(ids)
	}
	return []interface{}{conversationID, userID, messages, turns, turns, metadata, MaxTimelineSourceBytes, userID, turns}, nil
}

var timelineBoundedSourceSQL = timelineSourceReadCTE + `SELECT total_budget.size, chat_messages.id IS NOT NULL,
 COALESCE(chat_messages.id,''), chat_messages.created_at, chat_messages.updated_at,
 COALESCE(chat_messages.conversation_id,''), chat_messages.parent_id, chat_messages.turn_id,
 chat_messages.chat_execution_id, COALESCE(chat_messages.role,''), COALESCE(chat_messages.content,''),
 COALESCE(chat_messages.reasoning,''), COALESCE(chat_messages.prompt_tokens,0),
 COALESCE(chat_messages.completion_tokens,0), COALESCE(chat_messages.total_tokens,0),
 COALESCE(chat_messages.cache_read_tokens,0), COALESCE(chat_messages.cache_write_tokens,0),
 COALESCE(chat_messages.cache_miss_tokens,0), COALESCE(chat_messages.model,''),
 COALESCE(chat_messages.source,''), COALESCE(chat_messages.pinned,0)
 FROM total_budget LEFT JOIN admitted_sources ON total_budget.size <= ?
 LEFT JOIN chat_messages ON chat_messages.id=admitted_sources.id
 ORDER BY chat_messages.created_at,chat_messages.id`

// ReadBoundedTimelineSources não transporta conteúdo acima da cota. O LEFT
// JOIN preserva um resultado de orçamento mesmo quando não existe mensagem.
func ReadBoundedTimelineSources(tx *gorm.DB, conversationID string, messageIDs, turnIDs []string) ([]ChatMessage, error) {
	args, err := timelineSourceReadArgs(tx, conversationID, messageIDs, turnIDs)
	if err != nil {
		return nil, err
	}
	args = append(args, MaxTimelineSourceBytes)

	rows, err := tx.Raw(timelineBoundedSourceSQL, args...).Rows()
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	result := make([]ChatMessage, 0, len(messageIDs)+len(turnIDs))
	authorized := false
	for rows.Next() {
		authorized = true
		var size int64
		var present bool
		result = append(result, ChatMessage{})
		message := &result[len(result)-1]
		var created, updated sql.NullTime
		if err := rows.Scan(&size, &present, &message.ID, &created, &updated, &message.ConversationID,
			&message.ParentID, &message.TurnID, &message.ChatExecutionID, &message.Role, &message.Content, &message.Reasoning,
			&message.PromptTokens, &message.CompletionTokens, &message.TotalTokens, &message.CacheReadTokens, &message.CacheWriteTokens,
			&message.CacheMissTokens, &message.Model, &message.Source, &message.Pinned); err != nil {
			return nil, err
		}
		if size > MaxTimelineSourceBytes {
			return nil, ErrTimelineSourceLimit
		}
		if present {
			message.CreatedAt, message.UpdatedAt = created.Time, updated.Time
		} else {
			result = result[:len(result)-1]
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if !authorized {
		return nil, gorm.ErrRecordNotFound
	}
	return result, nil
}

// ApplyTimelineSources associa somente as fontes dos itens já selecionados.
// Todas as fontes vêm da leitura global limitada, na mesma transação.
func (r *MessageWindowResult) ApplyTimelineSources(conversationID string, parentID *string, sources []ChatMessage) {
	messages := make(map[string]bool, len(r.Items))
	turns := make(map[string]bool, len(r.Items))
	for _, item := range r.Items {
		if item.Kind == MessageWindowItemKindTurn {
			turns[item.TurnID] = true
		} else {
			messages[item.MessageID] = item.InvalidTurnReference
		}
	}
	r.Messages = make([]ChatMessage, 0, len(r.Items))
	for _, message := range sources {
		if message.ConversationID != conversationID || (message.ParentID == nil) != (parentID == nil) || (parentID != nil && *message.ParentID != *parentID) {
			continue
		}
		invalid, direct := messages[message.ID]
		if !direct && (message.Role == "user" || message.TurnID == nil || !turns[*message.TurnID]) {
			continue
		}
		if invalid {
			message.TurnID = nil
		}
		r.Messages = append(r.Messages, message)
	}
}
