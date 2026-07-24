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
			if err := store.Delete(ctx, rec.ConversationID); err != nil {
				logging.Warnf(ctx, "messaging.gateway", "[Gateway] reconcile: delete expirado conv=%s: %v", rec.ConversationID, err)
			}
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
			if err := g.deliverChannelResponse(ctx, rec.Channel, rec.ChatID, content, msgID, rec.AudioOnly, rec.ReplyToMsgID, rec.TraceID, rec.ConversationID); err != nil {
				logging.Errorf(ctx, "messaging.gateway", "[Gateway] reconcile: send falhou conv=%s: %v (agendando retry)", rec.ConversationID, err)
				go g.retryReconcileSend(rec, content, msgID)
				continue
			}
			if err := store.Delete(ctx, rec.ConversationID); err != nil {
				logging.Warnf(ctx, "messaging.gateway", "[Gateway] reconcile: delete após send conv=%s: %v", rec.ConversationID, err)
			}
			logging.Infof(ctx, "messaging.gateway", "[Gateway] reconcile: reenviou resposta órfã conv=%s channel=%s msg=%s",
				rec.ConversationID, rec.Channel, msgID)
			continue
		}

		// Sem resposta ainda — re-registra callback in-memory para Notify futuro.
		remaining := time.Until(rec.CreatedAt.Add(callbackTTL))
		if remaining <= 0 {
			if err := store.Delete(ctx, rec.ConversationID); err != nil {
				logging.Warnf(ctx, "messaging.gateway", "[Gateway] reconcile: delete expirado conv=%s: %v", rec.ConversationID, err)
			}
			continue
		}
		recCopy := rec
		g.notifier.Register(rec.ConversationID, ResponseCallback{
			Channel:      recCopy.Channel,
			ChatID:       recCopy.ChatID,
			OwnerUserID:  recCopy.OwnerUserID,
			AudioOnly:    recCopy.AudioOnly,
			ReplyToMsgID: recCopy.ReplyToMsgID,
			TraceID:      recCopy.TraceID,
			TTL:          remaining,
			SkipPersist:  true,
			Callback: func(response string, assistantMsgID string) {
				if err := g.deliverChannelResponse(context.Background(), recCopy.Channel, recCopy.ChatID, response, assistantMsgID, recCopy.AudioOnly, recCopy.ReplyToMsgID, recCopy.TraceID, recCopy.ConversationID); err != nil {
					logging.Errorf(context.Background(), "messaging.gateway", "[Gateway] reconcile callback: send falhou conv=%s channel=%s trace=%s: %v",
						recCopy.ConversationID, recCopy.Channel, recCopy.TraceID, err)
				}
			},
		})
	}
}

// retryReconcileSend tenta reenviar uma resposta órfã após falha no startup
// (adapter ainda conectando). Mantém o pending no store até sucesso ou esgotar.
func (g *Gateway) retryReconcileSend(rec ChannelPendingRecord, content, msgID string) {
	delays := []time.Duration{3 * time.Second, 10 * time.Second, 30 * time.Second}
	for _, wait := range delays {
		time.Sleep(wait)
		if err := g.deliverChannelResponse(context.Background(), rec.Channel, rec.ChatID, content, msgID, rec.AudioOnly, rec.ReplyToMsgID, rec.TraceID, rec.ConversationID); err != nil {
			logging.Warnf(context.Background(), "messaging.gateway", "[Gateway] reconcile retry: send falhou conv=%s: %v", rec.ConversationID, err)
			continue
		}
		if store := g.notifier.pendingStore(); store != nil {
			if err := store.Delete(context.Background(), rec.ConversationID); err != nil {
				logging.Warnf(context.Background(), "messaging.gateway", "[Gateway] reconcile retry: delete conv=%s: %v", rec.ConversationID, err)
			}
		}
		logging.Infof(context.Background(), "messaging.gateway", "[Gateway] reconcile retry: reenviou resposta órfã conv=%s channel=%s msg=%s",
			rec.ConversationID, rec.Channel, msgID)
		return
	}
	logging.Errorf(context.Background(), "messaging.gateway", "[Gateway] reconcile retry: esgotou tentativas conv=%s channel=%s (pending permanece até próximo restart)",
		rec.ConversationID, rec.Channel)
}

func (n *ResponseNotifier) pendingStore() ChannelPendingStore {
	if n == nil {
		return nil
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.store
}
