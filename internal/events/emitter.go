package events

import (
	"fmt"
	"log"

	"assistente/internal/core/ports"
)

// Emitter é um alias de compatibilidade para ports.Emitter.
// Novos pacotes devem importar diretamente "assistente/internal/core/ports".
type Emitter = ports.Emitter

// StreamEvent é um alias de compatibilidade para ports.StreamEvent.
// Novos pacotes devem importar diretamente "assistente/internal/core/ports".
type StreamEvent = ports.StreamEvent

// NoopEmitter descarta todos os eventos. Útil em testes e contextos sem UI.
type NoopEmitter struct{}

func (NoopEmitter) Emit(_ string, _ any) {}

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
