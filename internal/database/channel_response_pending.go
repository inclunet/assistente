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

// DeleteChannelResponsePending remove a pendência (Notify/Cancel/reconcile).
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

// FindLatestAssistantMessageAfter retorna a última mensagem assistant raiz
// criada após after (inclusive), ou nil se não houver.
func FindLatestAssistantMessageAfter(ctx context.Context, conversationID string, after time.Time) (*ChatMessage, error) {
	db := DB()
	if db == nil {
		return nil, errDBNotInitialized
	}
	var msg ChatMessage
	err := db.WithContext(ctx).
		Where("conversation_id = ? AND role = ? AND (parent_id IS NULL OR parent_id = '') AND created_at >= ?",
			conversationID, "assistant", after).
		Order("created_at DESC").
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
