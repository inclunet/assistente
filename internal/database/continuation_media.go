package database

import (
	"context"

	"gorm.io/gorm"
)

// ResolveContinuationMedia preenche somente cópias destinadas à apresentação ou
// ao contexto do modelo. A resposta nova referencia a base; não duplica blobs no
// banco. O áudio gerado para o texto anterior não é herdado como áudio do novo.
func ResolveContinuationMedia(ctx context.Context, db *gorm.DB, messages []ChatMessage) error {
	ids := make([]string, 0)
	byID := map[string][]int{}
	for i, m := range messages {
		if m.ChatExecutionID != nil && m.Media == "" {
			if _, ok := byID[m.ID]; !ok {
				ids = append(ids, m.ID)
			}
			byID[m.ID] = append(byID[m.ID], i)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	user, err := RequireUserID(ctx)
	if err != nil {
		return err
	}
	for start := 0; start < len(ids); start += 400 {
		var rows []struct {
			ID     string
			BaseID string
		}
		err := db.WithContext(ctx).Raw(`WITH RECURSIVE bases(id,base_id,depth) AS (
 SELECT m.id,a.base_response_message_id,1 FROM chat_messages m
 JOIN conversations c ON c.id=m.conversation_id
 JOIN chat_turn_attempts a ON a.execution_id=m.chat_execution_id AND a.response_message_id=m.id
 WHERE c.user_id=? AND m.id IN ? AND a.mode='continue' AND a.base_response_message_id IS NOT NULL
 UNION ALL
 SELECT b.id,a.base_response_message_id,b.depth+1 FROM bases b
 JOIN chat_messages m ON m.id=b.base_id
 JOIN chat_turn_attempts a ON a.execution_id=m.chat_execution_id AND a.response_message_id=m.id
 WHERE a.mode='continue' AND a.base_response_message_id IS NOT NULL
 ) SELECT b.id,b.base_id FROM bases b JOIN chat_messages m ON m.id=b.base_id JOIN conversations c ON c.id=m.conversation_id WHERE c.user_id=? AND EXISTS(SELECT 1 FROM chat_message_attachments x WHERE x.message_id=m.id AND x.purpose='media') ORDER BY b.depth`, user, ids[start:min(start+400, len(ids))], user).Scan(&rows).Error
		if err != nil {
			return err
		}
		bases := make([]ChatMessage, len(rows))
		for i, row := range rows {
			bases[i].ID = row.BaseID
		}
		if err := LoadMessageMedia(ctx, db, bases); err != nil {
			return err
		}
		for i, row := range rows {
			for _, index := range byID[row.ID] {
				if messages[index].Media == "" {
					messages[index].Media = bases[i].Media
				}
			}
		}
	}
	return nil
}
