package messaging

import (
	"assistente/internal/logging"
	"context"
	"sync"
	"time"
)

// callbackTTL é o tempo máximo que um callback pode ficar pendente antes
// de ser descartado pela goroutine de housekeeping. Mensagens de canal
// que não recebem resposta nesse intervalo geralmente travaram em algum
// lugar do pipeline (LLM cancelado, app crash, agent worker preso).
//
// 5 minutos é generoso para o pipeline de chat normal (resposta típica
// chega em <30s) e curto o bastante para que callbacks órfãos não se
// acumulem em conversas de canal de alta vazão.
const callbackTTL = 5 * time.Minute

// callbackCleanupInterval é a frequência de varredura do housekeeping.
// Ficar abaixo do TTL garante que callbacks vencidos não fiquem mais que
// um intervalo extra na fila.
const callbackCleanupInterval = time.Minute

// ResponseCallback é um callback registrado pelo Gateway para receber a resposta
// do assistente e reenviá-la ao mensageiro de origem.
type ResponseCallback struct {
	// Channel identifica a plataforma ("telegram", "signal", etc.).
	Channel string

	// TraceID permite correlacionar logs entre gateway/notifier/envio.
	TraceID string

	// ChatID é o identificador do chat de destino para a resposta.
	ChatID string

	// OwnerUserID é o dono do canal (AEP-0052); usado na persistência M14.
	OwnerUserID string

	// AudioOnly indica que a mensagem original era apenas áudio.
	// A resposta deve ser sintetizada em áudio (TTS) e enviada como attachment.
	AudioOnly bool

	// ReplyToMsgID é o ID da mensagem entrante (thread/reply no Slack etc.).
	ReplyToMsgID string

	// Callback é chamado com a resposta completa do assistente e o ID da mensagem salva.
	Callback func(response string, assistantMessageID string)

	// TTL define por quanto tempo este callback pode ficar pendente antes de ser
	// descartado pelo housekeeping. Zero usa o padrão (callbackTTL, 5min).
	TTL time.Duration

	// SkipPersist evita gravar no ChannelPendingStore (ex.: re-registro no
	// reconcile de startup — a linha já existe no DB).
	SkipPersist bool
}

// pendingCallback é o registro interno do Notifier — guarda o callback, o
// instante de registro e o instante de expiração já calculado (registro + TTL
// efetivo) para a checagem do housekeeping.
type pendingCallback struct {
	cb         ResponseCallback
	registered time.Time
	expiresAt  time.Time
}

// ResponseNotifier permite ao Gateway registrar callbacks para capturar respostas
// do assistente e reenviá-las ao mensageiro de origem.
//
// Fluxo:
//  1. Gateway recebe mensagem do Telegram
//  2. Gateway registra callback via Register(conversationID, cb)
//  3. Gateway chama App.SendMessage(conversationID, ...)
//  4. Agentic loop roda normalmente (streaming para Wails)
//  5. saveAndFinish chama Notify(conversationID, resposta, messageID)
//  6. Callback dispara → Gateway reenvia ao Telegram
//
// Em qualquer falha do pipeline (sendMessage retorna erro, LLM cancelado,
// conversa deletada, channel removido), o caller deve chamar Cancel para
// não vazar callbacks. O Notifier também roda housekeeping em background
// que descarta callbacks com mais de callbackTTL — defesa em camadas
// contra paths de erro esquecidos pelos callers (B7 do review da Fatia 2).
//
// Thread-safe para uso concorrente.
//
// Persistência (M14): quando um ChannelPendingStore está configurado via
// SetPendingStore, callbacks de canal (telegram/signal/slack + OwnerUserID)
// são gravados no DB no Register. Remoção do store: após messenger.Send OK
// (DeleteIfTrace), Cancel explícito, ou reconcile sem assistant após idade.
// Notify e TTL in-memory NÃO apagam o store (LLM longo / crash antes do Send).
// No startup, ReconcilePending reenvia respostas já salvas ou re-registra
// callbacks ainda válidos.
type ResponseNotifier struct {
	mu        sync.Mutex
	callbacks map[string][]pendingCallback // conversationID -> callbacks pendentes
	now       func() time.Time             // injetável para testes
	stopCh    chan struct{}
	stopOnce  sync.Once
	store     ChannelPendingStore
}

// NewResponseNotifier cria um novo ResponseNotifier e inicia a goroutine
// de housekeeping que descarta callbacks vencidos (TTL).
func NewResponseNotifier() *ResponseNotifier {
	return newResponseNotifierWithClock(time.Now)
}

func newResponseNotifierWithClock(now func() time.Time) *ResponseNotifier {
	n := &ResponseNotifier{
		callbacks: make(map[string][]pendingCallback),
		now:       now,
		stopCh:    make(chan struct{}),
	}
	go n.runCleanup(callbackCleanupInterval)
	return n
}

// SetPendingStore habilita persistência de callbacks de canal (M14).
func (n *ResponseNotifier) SetPendingStore(store ChannelPendingStore) {
	if n == nil {
		return
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	n.store = store
}

// Stop encerra a goroutine de housekeeping. Idempotente. Use em tear-down
// (testes, shutdown) — em produção o Notifier vive enquanto o app vive.
func (n *ResponseNotifier) Stop() {
	n.stopOnce.Do(func() {
		close(n.stopCh)
	})
}

func (n *ResponseNotifier) runCleanup(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-n.stopCh:
			return
		case <-ticker.C:
			n.expireOldCallbacks()
		}
	}
}

func (n *ResponseNotifier) expireOldCallbacks() {
	now := n.now()

	var toLog []expiredLogEntry
	n.mu.Lock()
	for convID, pendings := range n.callbacks {
		fresh := pendings[:0]
		var expired []pendingCallback
		for _, p := range pendings {
			// M14: callbacks de canal externos (telegram/signal/slack com
			// ChatID+OwnerUserID) ficam até Notify/Cancel — inclusive os
			// re-registrados pelo reconcile com SkipPersist. Expirar só a
			// memória fazia Notify no-op enquanto o store durável existia.
			if shouldPersistChannelCallback(p.cb) {
				fresh = append(fresh, p)
				continue
			}
			if p.expiresAt.Before(now) {
				expired = append(expired, p)
				continue
			}
			fresh = append(fresh, p)
		}
		if len(fresh) == 0 {
			delete(n.callbacks, convID)
		} else {
			n.callbacks[convID] = fresh
		}
		for _, p := range expired {
			toLog = append(toLog, expiredLogEntry{
				traceID: p.cb.TraceID,
				convID:  convID,
				channel: p.cb.Channel,
				minutes: p.expiresAt.Sub(p.registered).Minutes(),
			})
		}
	}
	n.mu.Unlock()

	for _, e := range toLog {
		logging.Debugf(context.Background(), "messaging.notifier", "[Notifier] Callback expirado por TTL trace=%s conv=%s channel=%s (>%.0fmin sem resposta)",
			e.traceID, e.convID, e.channel, e.minutes)
	}
	// Store channel_response_pending também só sai em DeleteIfTrace (Send OK),
	// Cancel/CancelTrace, ou reconcile sem assistant após idade.
}

// expiredLogEntry carrega os dados (já copiados sob o lock) para logar a
// expiração de um callback FORA do lock — evita segurar n.mu durante o I/O.
type expiredLogEntry struct {
	traceID string
	convID  string
	channel string
	minutes float64
}

// Register registra um callback para ser chamado quando a resposta de uma
// conversa ficar pronta. O callback é removido automaticamente após ser chamado,
// cancelado, ou expirado por TTL. O TTL efetivo é cb.TTL quando > 0 (registros de
// vida longa, ex.: sub-agente em background) ou o padrão callbackTTL (5min) caso
// contrário (canais/UI).
//
// Para callbacks de canal externo persistíveis (telegram/signal/slack com
// ChatID+OwnerUserID), um novo Register substitui os callbacks de canal
// anteriores da mesma conversa — alinhado ao store (1 pending/conversa) e
// evita Notify disparar vários Sends com a mesma resposta (M14 + TTL longo).
func (n *ResponseNotifier) Register(conversationID string, cb ResponseCallback) {
	ttl := callbackTTL
	if cb.TTL > 0 {
		ttl = cb.TTL
	}
	n.mu.Lock()
	now := n.now()
	entry := pendingCallback{
		cb:         cb,
		registered: now,
		expiresAt:  now.Add(ttl),
	}
	if shouldPersistChannelCallback(cb) {
		prev := n.callbacks[conversationID]
		kept := prev[:0]
		for _, p := range prev {
			if shouldPersistChannelCallback(p.cb) {
				continue
			}
			kept = append(kept, p)
		}
		n.callbacks[conversationID] = append(kept, entry)
	} else {
		n.callbacks[conversationID] = append(n.callbacks[conversationID], entry)
	}
	store := n.store
	n.mu.Unlock()

	if store != nil && shouldPersistChannelCallback(cb) && !cb.SkipPersist {
		if err := store.Upsert(context.Background(), ChannelPendingRecord{
			ConversationID: conversationID,
			Channel:        cb.Channel,
			ChatID:         cb.ChatID,
			AudioOnly:      cb.AudioOnly,
			TraceID:        cb.TraceID,
			OwnerUserID:    cb.OwnerUserID,
			ReplyToMsgID:   cb.ReplyToMsgID,
			CreatedAt:      now.UTC(),
		}); err != nil {
			logging.Errorf(context.Background(), "messaging.notifier", "[Notifier] falha ao persistir pending conv=%s channel=%s: %v",
				conversationID, cb.Channel, err)
		} else {
			// Compensa race com Notify/Cancel concorrente: se a conversa já
			// não tem callbacks, o Upsert pode ter “ressuscitado” a linha.
			n.mu.Lock()
			stillPending := len(n.callbacks[conversationID]) > 0
			n.mu.Unlock()
			if !stillPending {
				if delErr := store.Delete(context.Background(), conversationID); delErr != nil {
					logging.Warnf(context.Background(), "messaging.notifier", "[Notifier] falha ao limpar pending fantasma conv=%s: %v", conversationID, delErr)
				}
			}
		}
	}
}

// Notify chama todos os callbacks registrados para uma conversa e os remove.
// Se não há callbacks, não faz nada (zero overhead no fluxo normal do Wails).
// assistantMessageID é o ID da mensagem do assistente salva no DB ("" se não disponível).
//
// M11: cada callback roda em goroutine isolada com defer/recover —
// adapter de canal mal escrito que panique não derruba o app.
func (n *ResponseNotifier) Notify(conversationID string, response string, assistantMessageID string) {
	n.mu.Lock()
	pendings, ok := n.callbacks[conversationID]
	if ok {
		delete(n.callbacks, conversationID)
	}
	n.mu.Unlock()

	// Não remove channel_response_pending aqui: o Delete só ocorre após
	// messenger.Send bem-sucedido (deliverChannelResponse), Cancel ou TTL.
	// Apagar antes do Send abriria buraco M14 se o processo cair entre
	// Delete e o envio outbound.

	if !ok || len(pendings) == 0 {
		return
	}

	for _, p := range pendings {
		go func(cb ResponseCallback) {
			defer func() {
				if r := recover(); r != nil {
					logging.Errorf(context.Background(), "messaging.notifier", "[Notifier] panic em callback trace=%s channel=%s conv=%s: %v",
						cb.TraceID, cb.Channel, conversationID, r)
				}
			}()
			cb.Callback(response, assistantMessageID)
		}(p.cb)
	}
}

// Cancel remove todos os callbacks pendentes de uma conversa sem chamá-los.
// Usado quando o streaming LLM é cancelado (ex: barge-in SIP) ou a
// conversa/run é encerrada — evita callbacks órfãos que nunca disparariam.
// Para falha de um turno específico no gateway, preferir CancelTrace.
func (n *ResponseNotifier) Cancel(conversationID string) {
	n.mu.Lock()
	pendings, ok := n.callbacks[conversationID]
	if ok {
		delete(n.callbacks, conversationID)
	}
	store := n.store
	n.mu.Unlock()
	if store != nil {
		if err := store.Delete(context.Background(), conversationID); err != nil {
			logging.Warnf(context.Background(), "messaging.notifier", "[Notifier] falha ao remover pending no Cancel conv=%s: %v", conversationID, err)
		}
	}
	if ok && len(pendings) > 0 {
		for _, p := range pendings {
			logging.Debugf(context.Background(), "messaging.notifier", "[Messaging] Callback cancelado trace=%s conv=%s channel=%s (count=%d)",
				p.cb.TraceID, conversationID, p.cb.Channel, len(pendings))
		}
	}
}

// CancelTrace remove só os callbacks do TraceID informado e apaga o pending
// persistido apenas se ainda corresponder a esse turno (DeleteIfTrace).
// Evita que falha de sendMessage de um turno antigo apague a intenção M14
// de um turno mais novo na mesma conversa.
func (n *ResponseNotifier) CancelTrace(conversationID, traceID string) {
	if conversationID == "" {
		return
	}
	n.mu.Lock()
	pendings := n.callbacks[conversationID]
	fresh := pendings[:0]
	var removed []pendingCallback
	for _, p := range pendings {
		if p.cb.TraceID == traceID {
			removed = append(removed, p)
			continue
		}
		fresh = append(fresh, p)
	}
	if len(fresh) == 0 {
		delete(n.callbacks, conversationID)
	} else {
		n.callbacks[conversationID] = fresh
	}
	store := n.store
	n.mu.Unlock()

	if store != nil {
		if err := store.DeleteIfTrace(context.Background(), conversationID, traceID); err != nil {
			logging.Warnf(context.Background(), "messaging.notifier", "[Notifier] falha ao remover pending no CancelTrace conv=%s trace=%s: %v",
				conversationID, traceID, err)
		}
	}
	for _, p := range removed {
		logging.Debugf(context.Background(), "messaging.notifier", "[Messaging] Callback cancelado por trace=%s conv=%s channel=%s",
			p.cb.TraceID, conversationID, p.cb.Channel)
	}
}

// CancelByChannel cancela todos os callbacks pendentes pertencentes a um
// determinado canal. Usado em Unregister/Shutdown do Gateway para que o
// desligamento de um adapter (ex.: usuário desabilitou Telegram) não deixe
// callbacks órfãos que nunca dispariam (B7 do review da Fatia 2).
//
// Retorna a quantidade de callbacks cancelados (útil para logs/testes).
func (n *ResponseNotifier) CancelByChannel(channel string) int {
	if channel == "" {
		return 0
	}
	var toLog []expiredLogEntry
	var deleteIDs []string
	n.mu.Lock()
	cancelled := 0
	for convID, pendings := range n.callbacks {
		fresh := pendings[:0]
		removedPersisted := false
		for _, p := range pendings {
			if p.cb.Channel == channel {
				toLog = append(toLog, expiredLogEntry{
					traceID: p.cb.TraceID,
					convID:  convID,
					channel: p.cb.Channel,
				})
				cancelled++
				if shouldPersistChannelCallback(p.cb) {
					removedPersisted = true
				}
				continue
			}
			fresh = append(fresh, p)
		}
		if len(fresh) == 0 {
			delete(n.callbacks, convID)
			deleteIDs = append(deleteIDs, convID)
		} else {
			n.callbacks[convID] = fresh
			// Mesmo com callbacks internos restantes, remove pending de canal
			// externo para não reenviar indevidamente no reconcile.
			if removedPersisted {
				deleteIDs = append(deleteIDs, convID)
			}
		}
	}
	store := n.store
	n.mu.Unlock()

	for _, e := range toLog {
		logging.Debugf(context.Background(), "messaging.notifier", "[Notifier] Callback cancelado por canal removido trace=%s conv=%s channel=%s",
			e.traceID, e.convID, e.channel)
	}
	if store != nil {
		for _, id := range deleteIDs {
			if err := store.Delete(context.Background(), id); err != nil {
				logging.Warnf(context.Background(), "messaging.notifier", "[Notifier] falha ao remover pending no CancelByChannel conv=%s: %v", id, err)
			}
		}
	}
	return cancelled
}

// PendingExpiry retorna o instante de expiração (registro + TTL efetivo) do
// primeiro callback pendente de uma conversa. Útil para debug/testes (ex.:
// verificar que um registro de vida longa — sub-agente — tem TTL bem além do
// padrão). ok=false quando não há callback pendente para a conversa.
func (n *ResponseNotifier) PendingExpiry(conversationID string) (time.Time, bool) {
	n.mu.Lock()
	defer n.mu.Unlock()
	pendings := n.callbacks[conversationID]
	if len(pendings) == 0 {
		return time.Time{}, false
	}
	return pendings[0].expiresAt, true
}

// PendingCount retorna quantos callbacks estão pendentes (útil para debug/testes).
func (n *ResponseNotifier) PendingCount() int {
	n.mu.Lock()
	defer n.mu.Unlock()
	count := 0
	for _, pendings := range n.callbacks {
		count += len(pendings)
	}
	return count
}
