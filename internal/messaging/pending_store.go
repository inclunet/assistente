package messaging

import (
	"context"
	"time"
)

// ChannelPendingRecord é o espelho persistido de um callback de canal.
type ChannelPendingRecord struct {
	ConversationID string
	Channel        string
	ChatID         string
	AudioOnly      bool
	TraceID        string
	OwnerUserID    string
	CreatedAt      time.Time
}

// ChannelPendingStore persiste intents de resposta outbound (M14).
type ChannelPendingStore interface {
	Upsert(ctx context.Context, rec ChannelPendingRecord) error
	Delete(ctx context.Context, conversationID string) error
	List(ctx context.Context) ([]ChannelPendingRecord, error)
}

func shouldPersistChannelCallback(cb ResponseCallback) bool {
	if cb.Callback == nil || cb.ChatID == "" {
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
