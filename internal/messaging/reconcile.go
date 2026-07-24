package messaging

import (
	"assistente/internal/logging"
	"context"
	"time"
)

// ReconcilePending reenvia respostas de canal órfãs após crash (M14).
//
// Para cada pendência no store:
//   - se já existe mensagem assistant após CreatedAt → envia via messenger e apaga
//   - se expirou (> callbackTTL) → apaga
//   - senão → re-registra callback in-memory (espera Notify de run ainda em voo, raro)
func (g *Gateway) ReconcilePending(ctx context.Context, find FindAssistantAfterFn) {
	if g == nil || g.notifier == nil {
		return
	}
	store := g.notifier.pendingStore()
	if store == nil {
		return
	}
	if find == nil {
		find = DefaultFindAssistantAfter
	}

	rows, err := store.List(ctx)
	if err != nil {
		logging.Errorf(ctx, "messaging.gateway", "[Gateway] reconcile: list pending: %v", err)
		return
	}
	if len(rows) == 0 {
		return
	}

	now := time.Now().UTC()
	for _, rec := range rows {
		if rec.CreatedAt.Add(callbackTTL).Before(now) {
			_ = store.Delete(ctx, rec.ConversationID)
			logging.Debugf(ctx, "messaging.gateway", "[Gateway] reconcile: pendência expirada conv=%s channel=%s",
				rec.ConversationID, rec.Channel)
			continue
		}

		content, msgID, ok, ferr := find(ctx, rec.ConversationID, rec.CreatedAt)
		if ferr != nil {
			logging.Warnf(ctx, "messaging.gateway", "[Gateway] reconcile: busca assistant conv=%s: %v", rec.ConversationID, ferr)
			continue
		}
		if ok && content != "" {
			messenger, has := g.GetMessenger(rec.Channel)
			if !has {
				logging.Warnf(ctx, "messaging.gateway", "[Gateway] reconcile: messenger %s ausente conv=%s", rec.Channel, rec.ConversationID)
				continue
			}
			if err := messenger.Send(ctx, OutgoingMessage{ChatID: rec.ChatID, Text: content}); err != nil {
				logging.Errorf(ctx, "messaging.gateway", "[Gateway] reconcile: send falhou conv=%s: %v", rec.ConversationID, err)
				continue
			}
			_ = store.Delete(ctx, rec.ConversationID)
			logging.Infof(ctx, "messaging.gateway", "[Gateway] reconcile: reenviou resposta órfã conv=%s channel=%s msg=%s",
				rec.ConversationID, rec.Channel, msgID)
			continue
		}

		// Sem resposta ainda — re-registra callback in-memory para Notify futuro.
		remaining := time.Until(rec.CreatedAt.Add(callbackTTL))
		if remaining <= 0 {
			_ = store.Delete(ctx, rec.ConversationID)
			continue
		}
		recCopy := rec
		g.notifier.Register(rec.ConversationID, ResponseCallback{
			Channel:     recCopy.Channel,
			ChatID:      recCopy.ChatID,
			OwnerUserID: recCopy.OwnerUserID,
			AudioOnly:   recCopy.AudioOnly,
			TraceID:     recCopy.TraceID,
			TTL:         remaining,
			SkipPersist: true,
			Callback: func(response string, assistantMsgID string) {
				messenger, has := g.GetMessenger(recCopy.Channel)
				if !has {
					return
				}
				_ = messenger.Send(context.Background(), OutgoingMessage{
					ChatID: recCopy.ChatID,
					Text:   response,
				})
			},
		})
	}
}

func (n *ResponseNotifier) pendingStore() ChannelPendingStore {
	if n == nil {
		return nil
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.store
}
