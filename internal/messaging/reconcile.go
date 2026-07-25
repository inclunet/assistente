package messaging

import (
	"assistente/internal/database"
	"assistente/internal/logging"
	"context"
	"time"
)

// ReconcilePending reenvia respostas de canal órfãs após crash (M14).
//
// Para cada pendência no store:
//  1. busca a primeira assistant após CreatedAt (turno da pendência)
//  2. se houver → envia via messenger; apaga só após Send OK
//  3. se não houver e expirou (> callbackTTL) → apaga
//  4. senão → limpa callbacks in-memory e re-registra (espera Notify)
//
// Importante: NÃO descartar por TTL antes do find — se a assistant já
// foi salva e o crash ocorreu antes do Notify, a pendência ainda deve
// reenviar mesmo após 5 minutos (exatamente o buraco que M14 fecha).
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
		recCtx := ctx
		if rec.OwnerUserID != "" {
			recCtx = database.WithUserID(ctx, rec.OwnerUserID)
		}

		content, msgID, ok, ferr := find(recCtx, rec.ConversationID, rec.CreatedAt)
		if ferr != nil {
			logging.Warnf(recCtx, "messaging.gateway", "[Gateway] reconcile: busca assistant conv=%s: %v", rec.ConversationID, ferr)
			continue
		}
		if ok && content != "" {
			if err := g.deliverChannelResponse(recCtx, rec.Channel, rec.ChatID, content, msgID, rec.AudioOnly, rec.ReplyToMsgID, rec.TraceID, rec.ConversationID); err != nil {
				logging.Errorf(recCtx, "messaging.gateway", "[Gateway] reconcile: send falhou conv=%s: %v (agendando retry)", rec.ConversationID, err)
				go g.retryReconcileSend(rec, content, msgID)
				continue
			}
			// deliverChannelResponse já remove o pending após Send OK.
			logging.Infof(recCtx, "messaging.gateway", "[Gateway] reconcile: reenviou resposta órfã conv=%s channel=%s msg=%s",
				rec.ConversationID, rec.Channel, msgID)
			continue
		}

		expired := rec.CreatedAt.Add(callbackTTL).Before(now)
		if expired {
			if err := store.Delete(recCtx, rec.ConversationID); err != nil {
				logging.Warnf(recCtx, "messaging.gateway", "[Gateway] reconcile: delete expirado conv=%s: %v", rec.ConversationID, err)
			}
			logging.Debugf(recCtx, "messaging.gateway", "[Gateway] reconcile: pendência expirada sem assistant conv=%s channel=%s",
				rec.ConversationID, rec.Channel)
			continue
		}

		// Sem resposta ainda — re-registra callback in-memory para Notify futuro.
		// Limpa memória antes para não acumular callbacks (Notify dispararia todos).
		remaining := time.Until(rec.CreatedAt.Add(callbackTTL))
		if remaining <= 0 {
			if err := store.Delete(recCtx, rec.ConversationID); err != nil {
				logging.Warnf(recCtx, "messaging.gateway", "[Gateway] reconcile: delete expirado conv=%s: %v", rec.ConversationID, err)
			}
			continue
		}
		g.notifier.clearMemoryCallbacks(rec.ConversationID)
		recCopy := rec
		ownerID := recCopy.OwnerUserID
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
				cbCtx := context.Background()
				if ownerID != "" {
					cbCtx = database.WithUserID(cbCtx, ownerID)
				}
				if err := g.deliverChannelResponse(cbCtx, recCopy.Channel, recCopy.ChatID, response, assistantMsgID, recCopy.AudioOnly, recCopy.ReplyToMsgID, recCopy.TraceID, recCopy.ConversationID); err != nil {
					logging.Errorf(cbCtx, "messaging.gateway", "[Gateway] reconcile callback: send falhou conv=%s channel=%s trace=%s: %v",
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
		recCtx := context.Background()
		if rec.OwnerUserID != "" {
			recCtx = database.WithUserID(recCtx, rec.OwnerUserID)
		}
		// Só abandona por TTL se ainda não há assistant entregável — aqui já temos content.
		// Mantém tentativas enquanto o pending existir; deliver remove após sucesso.
		if err := g.deliverChannelResponse(recCtx, rec.Channel, rec.ChatID, content, msgID, rec.AudioOnly, rec.ReplyToMsgID, rec.TraceID, rec.ConversationID); err != nil {
			logging.Warnf(recCtx, "messaging.gateway", "[Gateway] reconcile retry: send falhou conv=%s: %v", rec.ConversationID, err)
			continue
		}
		logging.Infof(recCtx, "messaging.gateway", "[Gateway] reconcile retry: reenviou resposta órfã conv=%s channel=%s msg=%s",
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

// clearMemoryCallbacks remove callbacks in-memory sem tocar no pending store.
// Usado pelo reconcile ao re-registrar, para não acumular deliveries duplicados.
func (n *ResponseNotifier) clearMemoryCallbacks(conversationID string) {
	if n == nil {
		return
	}
	n.mu.Lock()
	delete(n.callbacks, conversationID)
	n.mu.Unlock()
}
