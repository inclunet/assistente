package messaging

import (
	"log"
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

	// AudioOnly indica que a mensagem original era apenas áudio.
	// A resposta deve ser sintetizada em áudio (TTS) e enviada como attachment.
	AudioOnly bool

	// Callback é chamado com a resposta completa do assistente e o ID da mensagem salva.
	Callback func(response string, assistantMessageID string)
}

// pendingCallback é o registro interno do Notifier — guarda o callback
// e o instante de registro para a checagem de TTL.
type pendingCallback struct {
	cb         ResponseCallback
	registered time.Time
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
// LIMITAÇÃO CONHECIDA — callbacks in-memory only (M14 / P0-3 do
// re-review da Fatia 2). Se o app crashar (panic, OOM, sigkill) entre
// o Register e o Notify, o callback é perdido junto com o processo.
// Consequência: a resposta do assistente nunca chega ao mensageiro
// externo (Telegram, Signal, Slack) — sem retry, sem feedback ao
// remetente. O bot pode aparentar ter "sumido com a mensagem" do
// ponto de vista do usuário externo.
//
// Mitigações ATUAIS:
//   - TTL evita callback órfão acumulado em memória (B7).
//   - panic/recover no callback evita derrubar o processo (M11).
//   - O remetente externo pode reenviar a mensagem.
//
// Mitigações PENDENTES:
//   - Persistir intent (channel_response_pending) no DB para que
//     uma varredura no startup re-dispare callbacks órfãos por crash.
//     TODO(aep-0052): tracking issue + schema + startup hook em fatia
//     futura. Hoje é aceito por: (a) crash do app é raro em operação
//     normal, (b) integrações externas já são best-effort do ponto de
//     vista de SLA, (c) a mensagem do usuário externo permanece no
//     histórico do app — nada se perde ali, só a resposta perdida no
//     vácuo do pipeline.
type ResponseNotifier struct {
	mu        sync.Mutex
	callbacks map[string][]pendingCallback // conversationID -> callbacks pendentes
	now       func() time.Time             // injetável para testes
	stopCh    chan struct{}
	stopOnce  sync.Once
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
	cutoff := n.now().Add(-callbackTTL)
	n.mu.Lock()
	defer n.mu.Unlock()
	for convID, pendings := range n.callbacks {
		fresh := pendings[:0]
		var expired []pendingCallback
		for _, p := range pendings {
			if p.registered.Before(cutoff) {
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
			log.Printf("[Notifier] Callback expirado por TTL trace=%s conv=%s channel=%s (>%.0fmin sem resposta)",
				p.cb.TraceID, convID, p.cb.Channel, callbackTTL.Minutes())
		}
	}
}

// Register registra um callback para ser chamado quando a resposta de uma
// conversa ficar pronta. O callback é removido automaticamente após ser chamado,
// cancelado, ou expirado por TTL.
func (n *ResponseNotifier) Register(conversationID string, cb ResponseCallback) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.callbacks[conversationID] = append(n.callbacks[conversationID], pendingCallback{
		cb:         cb,
		registered: n.now(),
	})
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

	if !ok || len(pendings) == 0 {
		return
	}

	for _, p := range pendings {
		go func(cb ResponseCallback) {
			defer func() {
				if r := recover(); r != nil {
					log.Printf("[Notifier] panic em callback trace=%s channel=%s conv=%s: %v",
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
	n.mu.Unlock()
	if ok && len(pendings) > 0 {
		// Mi9: era logado apenas o primeiro trace. Agora loga todos
		// para correlação completa quando há múltiplos callbacks
		// pendentes (ex.: race entre canal e UI na mesma conversa).
		for _, p := range pendings {
			log.Printf("[Messaging] Callback cancelado trace=%s conv=%s channel=%s (count=%d)",
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
	n.mu.Lock()
	defer n.mu.Unlock()
	cancelled := 0
	for convID, pendings := range n.callbacks {
		fresh := pendings[:0]
		for _, p := range pendings {
			if p.cb.Channel == channel {
				log.Printf("[Notifier] Callback cancelado por canal removido trace=%s conv=%s channel=%s",
					p.cb.TraceID, convID, p.cb.Channel)
				cancelled++
				continue
			}
			fresh = append(fresh, p)
		}
		if len(fresh) == 0 {
			delete(n.callbacks, convID)
		} else {
			n.callbacks[convID] = fresh
		}
	}
	return cancelled
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
