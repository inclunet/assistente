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
// SetPendingStore, callbacks de canal (Channel+ChatID) são gravados no DB
// no Register e removidos no Notify/Cancel/TTL. No startup, ReconcilePending
// reenvia respostas já salvas ou re-registra callbacks ainda válidos.
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
	var expiredConvIDs []string
	n.mu.Lock()
	for convID, pendings := range n.callbacks {
		fresh := pendings[:0]
		var expired []pendingCallback
		for _, p := range pendings {
			if p.expiresAt.Before(now) {
				expired = append(expired, p)
				continue
			}
			fresh = append(fresh, p)
		}
		if len(fresh) == 0 {
			delete(n.callbacks, convID)
			if len(expired) > 0 {
				expiredConvIDs = append(expiredConvIDs, convID)
			}
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
	store := n.store
	n.mu.Unlock()

	for _, e := range toLog {
		logging.Debugf(context.Background(), "messaging.notifier", "[Notifier] Callback expirado por TTL trace=%s conv=%s channel=%s (>%.0fmin sem resposta)",
			e.traceID, e.convID, e.channel, e.minutes)
	}
	if store != nil {
		for _, convID := range expiredConvIDs {
			_ = store.Delete(context.Background(), convID)
		}
	}
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
func (n *ResponseNotifier) Register(conversationID string, cb ResponseCallback) {
	ttl := callbackTTL
	if cb.TTL > 0 {
		ttl = cb.TTL
	}
	n.mu.Lock()
	now := n.now()
	n.callbacks[conversationID] = append(n.callbacks[conversationID], pendingCallback{
		cb:         cb,
		registered: now,
		expiresAt:  now.Add(ttl),
	})
	store := n.store
	n.mu.Unlock()

	if store != nil && shouldPersistChannelCallback(cb) && !cb.SkipPersist {
		_ = store.Upsert(context.Background(), ChannelPendingRecord{
			ConversationID: conversationID,
			Channel:        cb.Channel,
			ChatID:         cb.ChatID,
			AudioOnly:      cb.AudioOnly,
			TraceID:        cb.TraceID,
			OwnerUserID:    cb.OwnerUserID,
			CreatedAt:      now.UTC(),
		})
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
	store := n.store
	n.mu.Unlock()

	if store != nil {
		_ = store.Delete(context.Background(), conversationID)
	}

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
// Usado quando o streaming LLM é cancelado (ex: barge-in SIP), quando
// sendMessage falha em error path (B7 do review), ou quando o canal/conversa
// é removida — evita callbacks órfãos que nunca disparariam.
func (n *ResponseNotifier) Cancel(conversationID string) {
	n.mu.Lock()
	pendings, ok := n.callbacks[conversationID]
	if ok {
		delete(n.callbacks, conversationID)
	}
	store := n.store
	n.mu.Unlock()
	if store != nil {
		_ = store.Delete(context.Background(), conversationID)
	}
	if ok && len(pendings) > 0 {
		for _, p := range pendings {
			logging.Debugf(context.Background(), "messaging.notifier", "[Messaging] Callback cancelado trace=%s conv=%s channel=%s (count=%d)",
				p.cb.TraceID, conversationID, p.cb.Channel, len(pendings))
		}
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
		removedAll := true
		for _, p := range pendings {
			if p.cb.Channel == channel {
				toLog = append(toLog, expiredLogEntry{
					traceID: p.cb.TraceID,
					convID:  convID,
					channel: p.cb.Channel,
				})
				cancelled++
				continue
			}
			removedAll = false
			fresh = append(fresh, p)
		}
		if len(fresh) == 0 {
			delete(n.callbacks, convID)
			if removedAll {
				deleteIDs = append(deleteIDs, convID)
			}
		} else {
			n.callbacks[convID] = fresh
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
			_ = store.Delete(context.Background(), id)
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
