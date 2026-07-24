package messaging

import (
	"context"
	"time"

	"assistente/internal/database"
)

// DBChannelPendingStore adapta as funções de database para ChannelPendingStore.
type DBChannelPendingStore struct{}

func NewDBChannelPendingStore() *DBChannelPendingStore {
	return &DBChannelPendingStore{}
}

func (s *DBChannelPendingStore) Upsert(ctx context.Context, rec ChannelPendingRecord) error {
	return database.UpsertChannelResponsePending(ctx, &database.ChannelResponsePending{
		ConversationID: rec.ConversationID,
		Channel:        rec.Channel,
		ChatID:         rec.ChatID,
		AudioOnly:      rec.AudioOnly,
		TraceID:        rec.TraceID,
		OwnerUserID:    rec.OwnerUserID,
		ReplyToMsgID:   rec.ReplyToMsgID,
		CreatedAt:      rec.CreatedAt,
	})
}

func (s *DBChannelPendingStore) Delete(ctx context.Context, conversationID string) error {
	return database.DeleteChannelResponsePending(ctx, conversationID)
}

func (s *DBChannelPendingStore) List(ctx context.Context) ([]ChannelPendingRecord, error) {
	rows, err := database.ListChannelResponsePending(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]ChannelPendingRecord, 0, len(rows))
	for _, r := range rows {
		out = append(out, ChannelPendingRecord{
			ConversationID: r.ConversationID,
			Channel:        r.Channel,
			ChatID:         r.ChatID,
			AudioOnly:      r.AudioOnly,
			TraceID:        r.TraceID,
			OwnerUserID:    r.OwnerUserID,
			ReplyToMsgID:   r.ReplyToMsgID,
			CreatedAt:      r.CreatedAt,
		})
	}
	return out, nil
}

// FindAssistantAfterFn busca a resposta assistant já salva após o pending.
type FindAssistantAfterFn func(ctx context.Context, conversationID string, after time.Time) (content string, messageID string, ok bool, err error)

// DefaultFindAssistantAfter usa o repositório de mensagens do database.
func DefaultFindAssistantAfter(ctx context.Context, conversationID string, after time.Time) (string, string, bool, error) {
	msg, err := database.FindLatestAssistantMessageAfter(ctx, conversationID, after)
	if err != nil || msg == nil {
		return "", "", false, err
	}
	return msg.Content, msg.ID, true, nil
}
