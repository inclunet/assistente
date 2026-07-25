package database

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
)

// ChannelResponsePending persiste a intenção de entregar uma resposta de canal
// externo após o Register do ResponseNotifier (mitigação M14 / AEP-0052).
// Se o app crashar entre Register e Notify, o startup reconcilia e reenvia.
//
// Modelo: no máximo 1 pendência por conversa (PK = ConversationID). Um novo
// Register sobrescreve a anterior (turno novo supersede). Deletes pós-Send
// usam TraceID para não apagar a linha de um turno mais recente.
type ChannelResponsePending struct {
	ConversationID string    `gorm:"type:text;primaryKey" json:"conversation_id"`
	Channel        string    `gorm:"type:text;not null;index" json:"channel"`
	ChatID         string    `gorm:"type:text;not null" json:"chat_id"`
	AudioOnly      bool      `json:"audio_only"`
	TraceID        string    `gorm:"type:text" json:"trace_id"`
	OwnerUserID    string    `gorm:"type:text;index" json:"owner_user_id"`
	ReplyToMsgID   string    `gorm:"type:text" json:"reply_to_msg_id"`
	CreatedAt      time.Time `json:"created_at"`
}

var errDBNotInitialized = errors.New("banco de dados não inicializado")

// UpsertChannelResponsePending cria ou atualiza a pendência de uma conversa.
func UpsertChannelResponsePending(ctx context.Context, p *ChannelResponsePending) error {
	if p == nil || p.ConversationID == "" {
		return nil
	}
	if p.CreatedAt.IsZero() {
		p.CreatedAt = time.Now().UTC()
	}
	db := DB()
	if db == nil {
		return errDBNotInitialized
	}
	return db.WithContext(ctx).Save(p).Error
}

// DeleteChannelResponsePending remove a pendência (Cancel/TTL — incondicional).
func DeleteChannelResponsePending(ctx context.Context, conversationID string) error {
	if conversationID == "" {
		return nil
	}
	db := DB()
	if db == nil {
		return errDBNotInitialized
	}
	return db.WithContext(ctx).Where("conversation_id = ?", conversationID).Delete(&ChannelResponsePending{}).Error
}

// DeleteChannelResponsePendingIfTrace remove a pendência só se o TraceID ainda
// bate. Evita que um retry atrasado de um turno antigo apague a pendência de
// um turno mais novo (Upsert por conversationID sobrescreve a linha).
// TraceID vazio só remove linha que também está sem TraceID (legado/teste).
func DeleteChannelResponsePendingIfTrace(ctx context.Context, conversationID, traceID string) error {
	if conversationID == "" {
		return nil
	}
	db := DB()
	if db == nil {
		return errDBNotInitialized
	}
	q := db.WithContext(ctx).Where("conversation_id = ?", conversationID)
	if traceID == "" {
		q = q.Where("trace_id = '' OR trace_id IS NULL")
	} else {
		q = q.Where("trace_id = ?", traceID)
	}
	return q.Delete(&ChannelResponsePending{}).Error
}

// ListChannelResponsePending retorna todas as pendências (startup reconcile).
func ListChannelResponsePending(ctx context.Context) ([]ChannelResponsePending, error) {
	db := DB()
	if db == nil {
		return nil, errDBNotInitialized
	}
	var rows []ChannelResponsePending
	if err := db.WithContext(ctx).Order("created_at ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

// FindFirstAssistantMessageAfter retorna a primeira mensagem assistant raiz
// criada após after (inclusive), ou nil se não houver.
// Usado pelo reconcile M14: a pendência corresponde ao turno que a criou;
// pegar a mais recente poderia reenviar resposta de outro turno na mesma conversa.
// SECURITY: fail-closed (AEP-0052). Requer userID no ctx e usa scopedMessageQuery.
func FindFirstAssistantMessageAfter(ctx context.Context, conversationID string, after time.Time) (*ChatMessage, error) {
	if _, err := RequireUserID(ctx); err != nil {
		return nil, err
	}
	db := DB()
	if db == nil {
		return nil, errDBNotInitialized
	}
	var msg ChatMessage
	err := scopedMessageQuery(ctx, db.Model(&ChatMessage{})).
		Where("chat_messages.conversation_id = ? AND chat_messages.role = ? AND (chat_messages.parent_id IS NULL OR chat_messages.parent_id = '') AND chat_messages.created_at >= ?",
			conversationID, "assistant", after).
		Order("chat_messages.created_at ASC, chat_messages.id ASC").
		Limit(1).
		First(&msg).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &msg, nil
}

// FindLatestAssistantMessageAfter é alias histórico; preferir FindFirstAssistantMessageAfter
// no reconcile (M14). Mantido para callers/testes que querem a mais recente.
func FindLatestAssistantMessageAfter(ctx context.Context, conversationID string, after time.Time) (*ChatMessage, error) {
	if _, err := RequireUserID(ctx); err != nil {
		return nil, err
	}
	db := DB()
	if db == nil {
		return nil, errDBNotInitialized
	}
	var msg ChatMessage
	err := scopedMessageQuery(ctx, db.Model(&ChatMessage{})).
		Where("chat_messages.conversation_id = ? AND chat_messages.role = ? AND (chat_messages.parent_id IS NULL OR chat_messages.parent_id = '') AND chat_messages.created_at >= ?",
			conversationID, "assistant", after).
		Order("chat_messages.created_at DESC").
		Limit(1).
		First(&msg).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &msg, nil
}
