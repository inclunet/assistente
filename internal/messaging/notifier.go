package messaging

import (
	"fmt"
	"sync"
)

// ResponseCallback é um callback registrado pelo Gateway para receber a resposta
// do assistente e reenviá-la ao mensageiro de origem.
type ResponseCallback struct {
	// Channel identifica a plataforma ("telegram", "signal", etc.).
	Channel string

	// ChatID é o identificador do chat de destino para a resposta.
	ChatID string

	// Callback é chamado com a resposta completa do assistente.
	Callback func(response string)
}

// ResponseNotifier permite ao Gateway registrar callbacks para capturar respostas
// do assistente e reenviá-las ao mensageiro de origem.
//
// Fluxo:
//  1. Gateway recebe mensagem do Telegram
//  2. Gateway registra callback via Register(conversationID, cb)
//  3. Gateway chama App.SendMessage(conversationID, ...)
//  4. Agentic loop roda normalmente (streaming para Wails)
//  5. saveAndFinish chama Notify(conversationID, resposta)
//  6. Callback dispara → Gateway reenvia ao Telegram
//
// Thread-safe para uso concorrente.
type ResponseNotifier struct {
	mu        sync.Mutex
	callbacks map[uint][]ResponseCallback // conversationID -> callbacks pendentes
}

// NewResponseNotifier cria um novo ResponseNotifier.
func NewResponseNotifier() *ResponseNotifier {
	return &ResponseNotifier{
		callbacks: make(map[uint][]ResponseCallback),
	}
}

// Register registra um callback para ser chamado quando a resposta de uma
// conversa ficar pronta. O callback é removido automaticamente após ser chamado.
func (n *ResponseNotifier) Register(conversationID uint, cb ResponseCallback) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.callbacks[conversationID] = append(n.callbacks[conversationID], cb)
	fmt.Printf("[Messaging] Callback registrado para conversa %d (canal: %s, chat: %s)\n",
		conversationID, cb.Channel, cb.ChatID)
}

// Notify chama todos os callbacks registrados para uma conversa e os remove.
// Se não há callbacks, não faz nada (zero overhead no fluxo normal do Wails).
func (n *ResponseNotifier) Notify(conversationID uint, response string) {
	n.mu.Lock()
	cbs, ok := n.callbacks[conversationID]
	if ok {
		delete(n.callbacks, conversationID)
	}
	n.mu.Unlock()

	if !ok || len(cbs) == 0 {
		return
	}

	fmt.Printf("[Messaging] Notificando %d callback(s) para conversa %d\n", len(cbs), conversationID)
	for _, cb := range cbs {
		// Chama em goroutine para não bloquear o saveAndFinish
		go func(c ResponseCallback) {
			c.Callback(response)
		}(cb)
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
