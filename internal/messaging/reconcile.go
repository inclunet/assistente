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
//  1. busca a assistant do turno (user após CreatedAt → turn_id)
//  2. se houver → envia via messenger; apaga só após Send OK
//  3. se não houver e expirou (> callbackTTL) → apaga
//  4. senão → ensureMemoryCallback (re-registra só se não houver turno vivo)
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
			// Send já concluiu antes do Delete (crash entre MarkDelivered e Delete).
			if rec.DeliveredAssistantID != "" && rec.DeliveredAssistantID == msgID {
				if err := store.DeleteIfTrace(recCtx, rec.ConversationID, rec.TraceID); err != nil {
					logging.Warnf(recCtx, "messaging.gateway", "[Gateway] reconcile: delete já-entregue conv=%s: %v", rec.ConversationID, err)
				}
				logging.Infof(recCtx, "messaging.gateway", "[Gateway] reconcile: pending já entregue conv=%s msg=%s — só limpeza",
					rec.ConversationID, msgID)
				continue
			}
			// Upsert de turno novo durante o reconcile (Connect concorrente):
			// não reenviar snapshot obsoleto — DeleteIfTrace do deliver também
			// falharia em silêncio e a pendência nova ficaria para o turno vivo.
			matches, known := pendingTraceState(store, rec.ConversationID, rec.TraceID)
			if known && !matches {
				logging.Debugf(recCtx, "messaging.gateway", "[Gateway] reconcile: pending supersedido conv=%s trace=%s — pula reenvio",
					rec.ConversationID, rec.TraceID)
				continue
			}
			if err := g.deliverChannelResponse(recCtx, rec.Channel, rec.ChatID, content, msgID, rec.AudioOnly, rec.ReplyToMsgID, rec.TraceID, rec.ConversationID); err != nil {
				logging.Errorf(recCtx, "messaging.gateway", "[Gateway] reconcile: send falhou conv=%s: %v (agendando retry)", rec.ConversationID, err)
				g.scheduleRetryReconcileSend(rec, content, msgID)
				continue
			}
			// deliverChannelResponse já remove o pending após Send OK.
			logging.Infof(recCtx, "messaging.gateway", "[Gateway] reconcile: reenviou resposta órfã conv=%s channel=%s msg=%s",
				rec.ConversationID, rec.Channel, msgID)
			continue
		}

		expired := rec.CreatedAt.Add(callbackTTL).Before(now)
		if expired {
			// DeleteIfTrace: Upsert de turno novo entre List e Delete não pode
			// apagar a pendência mais recente (PK = conversation_id).
			if err := store.DeleteIfTrace(recCtx, rec.ConversationID, rec.TraceID); err != nil {
				logging.Warnf(recCtx, "messaging.gateway", "[Gateway] reconcile: delete expirado conv=%s: %v", rec.ConversationID, err)
			}
			logging.Debugf(recCtx, "messaging.gateway", "[Gateway] reconcile: pendência expirada sem assistant conv=%s channel=%s",
				rec.ConversationID, rec.Channel)
			continue
		}

		// Sem resposta ainda — re-registra callback in-memory para Notify futuro.
		// ensureMemoryCallback é atômico: se um turno vivo registrou entre o
		// check e o write, preserva o callback atual (não limpa/substitui).
		remaining := time.Until(rec.CreatedAt.Add(callbackTTL))
		if remaining <= 0 {
			if err := store.DeleteIfTrace(recCtx, rec.ConversationID, rec.TraceID); err != nil {
				logging.Warnf(recCtx, "messaging.gateway", "[Gateway] reconcile: delete expirado conv=%s: %v", rec.ConversationID, err)
			}
			continue
		}
		recCopy := rec
		ownerID := recCopy.OwnerUserID
		registered := g.notifier.ensureMemoryCallback(rec.ConversationID, ResponseCallback{
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
		if !registered {
			logging.Debugf(recCtx, "messaging.gateway", "[Gateway] reconcile: callback vivo conv=%s — pula re-registro",
				rec.ConversationID)
		}
	}
}

// scheduleRetryReconcileSend enfileira retry com limite de concorrência.
// Se o semáforo estiver cheio, a pendência permanece no store para o próximo restart.
func (g *Gateway) scheduleRetryReconcileSend(rec ChannelPendingRecord, content, msgID string) {
	if g == nil {
		return
	}
	sem := g.reconcileRetrySem
	if sem == nil {
		go g.retryReconcileSend(rec, content, msgID)
		return
	}
	select {
	case sem <- struct{}{}:
		go func() {
			defer func() { <-sem }()
			g.retryReconcileSend(rec, content, msgID)
		}()
	default:
		logging.Warnf(context.Background(), "messaging.gateway", "[Gateway] reconcile retry: limite de concorrência atingido conv=%s channel=%s — pending permanece para próximo restart",
			rec.ConversationID, rec.Channel)
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
		store := g.notifier.pendingStore()
		if store != nil {
			matches, known := pendingTraceState(store, rec.ConversationID, rec.TraceID)
			if known && !matches {
				logging.Debugf(recCtx, "messaging.gateway", "[Gateway] reconcile retry: pending supersedido conv=%s trace=%s — abortando",
					rec.ConversationID, rec.TraceID)
				return
			}
			// known=false (ex.: erro transitório de List): segue o retry;
			// DeleteIfTrace ainda protege contra apagar turno errado.
		}
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

// pendingTraceState indica se a pendência ainda é deste turno.
// known=false significa estado indefinido (ex.: List falhou) — o caller
// não deve abortar o retry; DeleteIfTrace protege a deleção.
func pendingTraceState(store ChannelPendingStore, conversationID, traceID string) (matches bool, known bool) {
	if store == nil || conversationID == "" {
		return false, true
	}
	rec, ok, err := store.Get(context.Background(), conversationID)
	if err != nil {
		logging.Warnf(context.Background(), "messaging.gateway", "[Gateway] reconcile retry: get pending falhou conv=%s: %v (seguindo retry)",
			conversationID, err)
		return false, false
	}
	if !ok {
		return false, true
	}
	return rec.TraceID == traceID, true
}

func (n *ResponseNotifier) pendingStore() ChannelPendingStore {
	if n == nil {
		return nil
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.store
}

// ensureMemoryCallback registra cb só se a conversa ainda não tiver callback
// in-memory. Check+write sob o mesmo lock — evita race em que um turno vivo
// registra entre o check e o write no reconcile.
// Não toca o pending store (uso típico: re-registro SkipPersist no startup).
// Retorna true se registrou, false se já havia turno vivo.
func (n *ResponseNotifier) ensureMemoryCallback(conversationID string, cb ResponseCallback) bool {
	if n == nil || conversationID == "" || cb.Callback == nil {
		return false
	}
	ttl := callbackTTL
	if cb.TTL > 0 {
		ttl = cb.TTL
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	if len(n.callbacks[conversationID]) > 0 {
		return false
	}
	now := n.now()
	n.callbacks[conversationID] = []pendingCallback{{
		cb:         cb,
		registered: now,
		expiresAt:  now.Add(ttl),
	}}
	return true
}
