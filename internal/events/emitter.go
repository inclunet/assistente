package events

import (
	"fmt"
	"log"
)

// Emitter abstrai a emissão de eventos para diferentes interfaces (Wails, CLI, REST, etc.).
// Implementações concretas: WailsEmitter (main package), NoopEmitter (testes).
type Emitter interface {
	Emit(event string, data any)
}

// NoopEmitter descarta todos os eventos. Útil em testes e contextos sem UI.
type NoopEmitter struct{}

func (NoopEmitter) Emit(_ string, _ any) {}

// StreamEvent é o payload do evento chat:stream emitido durante o streaming LLM.
// Definido aqui para permitir uso tanto no package main quanto em internal/agent.
type StreamEvent struct {
	MessageID      uint   `json:"messageId"`
	ConversationId uint   `json:"conversationId"`
	Content        string `json:"content"`
	Done           bool   `json:"done"`
	FullResponse   string `json:"fullResponse,omitempty"`
	Error          string `json:"error,omitempty"`
}

// RecoverFromPanic captura um panic em andamento (deve ser chamado via defer) e emite
// chat:stream com Done=true e a mensagem de erro para o frontend.
// O emitter pode ser nil — a função protege contra double-panic nesse caso.
func RecoverFromPanic(emitter Emitter, conversationID uint, source string) {
	HandlePanic(emitter, conversationID, source, recover())
}

// HandlePanic processa um valor já recuperado de um panic (r = result of recover()).
// Útil quando o chamador precisa chamar recover() diretamente no seu próprio corpo
// (ex.: dentro de um wrapper method como App.recoverFromPanic).
func HandlePanic(emitter Emitter, conversationID uint, source string, r any) {
	if r == nil {
		return
	}
	errMsg := fmt.Sprintf("Erro interno inesperado em %s: %v", source, r)
	log.Printf("🔴 [PANIC RECOVERED] %s (conversa %d): %v", source, conversationID, r)
	func() {
		defer func() { recover() }()
		if emitter != nil {
			emitter.Emit("chat:stream", StreamEvent{
				Done:           true,
				Error:          errMsg,
				ConversationId: conversationID,
			})
		}
	}()
}
