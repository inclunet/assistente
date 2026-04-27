package messaging

import (
	"log"
	"sync"
)

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
// Thread-safe para uso concorrente.
type ResponseNotifier struct {
	mu        sync.Mutex
	callbacks map[string][]ResponseCallback // conversationID -> callbacks pendentes
}

// NewResponseNotifier cria um novo ResponseNotifier.
func NewResponseNotifier() *ResponseNotifier {
	return &ResponseNotifier{
		callbacks: make(map[string][]ResponseCallback),
	}
}

// Register registra um callback para ser chamado quando a resposta de uma
// conversa ficar pronta. O callback é removido automaticamente após ser chamado.
func (n *ResponseNotifier) Register(conversationID string, cb ResponseCallback) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.callbacks[conversationID] = append(n.callbacks[conversationID], cb)
}

// Notify chama todos os callbacks registrados para uma conversa e os remove.
// Se não há callbacks, não faz nada (zero overhead no fluxo normal do Wails).
// assistantMessageID é o ID da mensagem do assistente salva no DB (0 se não disponível).
func (n *ResponseNotifier) Notify(conversationID string, response string, assistantMessageID string) {
	n.mu.Lock()
	cbs, ok := n.callbacks[conversationID]
	if ok {
		delete(n.callbacks, conversationID)
	}
	n.mu.Unlock()

	if !ok || len(cbs) == 0 {
		return
	}

	for _, cb := range cbs {
		// Chama em goroutine para não bloquear o saveAndFinish
		go func(c ResponseCallback) {
			c.Callback(response, assistantMessageID)
		}(cb)
	}
}

// Cancel remove todos os callbacks pendentes de uma conversa sem chamá-los.
// Usado quando o streaming LLM é cancelado (ex: barge-in SIP) e a resposta
// não será gerada — evita callbacks órfãos que nunca disparariam.
func (n *ResponseNotifier) Cancel(conversationID string) {
	n.mu.Lock()
	cbs, ok := n.callbacks[conversationID]
	if ok {
		delete(n.callbacks, conversationID)
	}
	n.mu.Unlock()
	if ok && len(cbs) > 0 {
		traceID := cbs[0].TraceID
		log.Printf("[Messaging] Callbacks cancelados trace=%s conv=%s count=%d (barge-in)",
			traceID, conversationID, len(cbs))
	}
}

// PendingCount retorna quantos callbacks estão pendentes (útil para debug/testes).
func (n *ResponseNotifier) PendingCount() int {
	n.mu.Lock()
	defer n.mu.Unlock()
	count := 0
	for _, cbs := range n.callbacks {
		count += len(cbs)
	}
	return count
}
