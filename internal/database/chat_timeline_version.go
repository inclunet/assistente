package database

import (
	"context"

	"gorm.io/gorm"
)

// ChatTimelineVersion identifica uma encarnação da conversa e suas alterações
// persistidas. Não é credencial, revisão de comando, epoch nem sequence wire.
// Consumidores comparam o par inteiro; revision só é ordenável na mesma incarnation.
type ChatTimelineVersion struct {
	Incarnation string
	Revision    int64
}

// ReadChatTimelineVersion deve receber a mesma transação de leitura que captura
// mensagens/ledger/turnos. Não inicia snapshot nem consulta conteúdo por token.
// Ausência, falta de autorização e schema incompleto nunca viram versão zero.
func ReadChatTimelineVersion(ctx context.Context, tx *gorm.DB, conversationID string) (ChatTimelineVersion, error) {
	var version ChatTimelineVersion
	userID, err := RequireUserID(ctx)
	if err != nil {
		return version, err
	}
	err = tx.WithContext(ctx).Table("chat_timeline_versions v").Select("v.incarnation, v.revision").
		Joins("JOIN conversations c ON c.id=v.conversation_id").
		Where("c.id=? AND c.user_id=?", conversationID, userID).Take(&version).Error
	return version, err
}
