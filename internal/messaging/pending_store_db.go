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
		ConversationID:       rec.ConversationID,
		Channel:              rec.Channel,
		ChatID:               rec.ChatID,
		AudioOnly:            rec.AudioOnly,
		TraceID:              rec.TraceID,
		OwnerUserID:          rec.OwnerUserID,
		ReplyToMsgID:         rec.ReplyToMsgID,
		DeliveredAssistantID: rec.DeliveredAssistantID,
		CreatedAt:            rec.CreatedAt,
	})
}

func (s *DBChannelPendingStore) Get(ctx context.Context, conversationID string) (ChannelPendingRecord, bool, error) {
	row, err := database.GetChannelResponsePending(ctx, conversationID)
	if err != nil {
		return ChannelPendingRecord{}, false, err
	}
	if row == nil {
		return ChannelPendingRecord{}, false, nil
	}
	return ChannelPendingRecord{
		ConversationID:       row.ConversationID,
		Channel:              row.Channel,
		ChatID:               row.ChatID,
		AudioOnly:            row.AudioOnly,
		TraceID:              row.TraceID,
		OwnerUserID:          row.OwnerUserID,
		ReplyToMsgID:         row.ReplyToMsgID,
		DeliveredAssistantID: row.DeliveredAssistantID,
		CreatedAt:            row.CreatedAt,
	}, true, nil
}

func (s *DBChannelPendingStore) Delete(ctx context.Context, conversationID string) error {
	return database.DeleteChannelResponsePending(ctx, conversationID)
}

func (s *DBChannelPendingStore) DeleteIfTrace(ctx context.Context, conversationID, traceID string) error {
	return database.DeleteChannelResponsePendingIfTrace(ctx, conversationID, traceID)
}

func (s *DBChannelPendingStore) MarkDelivered(ctx context.Context, conversationID, traceID, assistantMsgID string) error {
	return database.MarkChannelResponsePendingDelivered(ctx, conversationID, traceID, assistantMsgID)
}

func (s *DBChannelPendingStore) List(ctx context.Context) ([]ChannelPendingRecord, error) {
	rows, err := database.ListChannelResponsePending(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]ChannelPendingRecord, 0, len(rows))
	for _, r := range rows {
		out = append(out, ChannelPendingRecord{
			ConversationID:       r.ConversationID,
			Channel:              r.Channel,
			ChatID:               r.ChatID,
			AudioOnly:            r.AudioOnly,
			TraceID:              r.TraceID,
			OwnerUserID:          r.OwnerUserID,
			ReplyToMsgID:         r.ReplyToMsgID,
			DeliveredAssistantID: r.DeliveredAssistantID,
			CreatedAt:            r.CreatedAt,
		})
	}
	return out, nil
}

// FindAssistantAfterFn busca a resposta assistant já salva após o pending.
type FindAssistantAfterFn func(ctx context.Context, conversationID string, after time.Time) (content string, messageID string, ok bool, err error)

// DefaultFindAssistantAfter usa o repositório de mensagens do database.
// Resolve o turno pela primeira user após CreatedAt e a assistant com o mesmo turn_id.
func DefaultFindAssistantAfter(ctx context.Context, conversationID string, after time.Time) (string, string, bool, error) {
	msg, err := database.FindFirstAssistantMessageAfter(ctx, conversationID, after)
	if err != nil || msg == nil {
		return "", "", false, err
	}
	return msg.Content, msg.ID, true, nil
}
