package messaging

import (
	"context"
	"time"
)

// ChannelPendingRecord é o espelho persistido de um callback de canal.
type ChannelPendingRecord struct {
	ConversationID       string
	Channel              string
	ChatID               string
	AudioOnly            bool
	TraceID              string
	OwnerUserID          string
	ReplyToMsgID         string
	DeliveredAssistantID string
	CreatedAt            time.Time
}

// ChannelPendingStore persiste intents de resposta outbound (M14).
type ChannelPendingStore interface {
	Upsert(ctx context.Context, rec ChannelPendingRecord) error
	Delete(ctx context.Context, conversationID string) error
	// DeleteIfTrace remove só se o TraceID atual ainda corresponde ao turno entregue.
	DeleteIfTrace(ctx context.Context, conversationID, traceID string) error
	// MarkDelivered grava assistantMsgID após Send OK (antes do DeleteIfTrace).
	MarkDelivered(ctx context.Context, conversationID, traceID, assistantMsgID string) error
	List(ctx context.Context) ([]ChannelPendingRecord, error)
}

func shouldPersistChannelCallback(cb ResponseCallback) bool {
	if cb.Callback == nil || cb.ChatID == "" || cb.OwnerUserID == "" {
		// Sem OwnerUserID o reconcile não consegue buscar assistant (AEP-0052).
		return false
	}
	switch cb.Channel {
	case "telegram", "signal", "slack":
		return true
	default:
		// Callbacks internos (subagent, sip, etc.) ficam só em memória.
		return false
	}
}
