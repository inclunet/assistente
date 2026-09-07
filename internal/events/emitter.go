package events

import (
	"assistente/internal/core/ports"
	"assistente/internal/logging"
	"context"
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
// chat:stream com Done=true e um código de erro estável para o frontend.
// O emitter pode ser nil — a função protege contra double-panic nesse caso.
func RecoverFromPanic(emitter Emitter, conversationID string, source string) {
	HandlePanic(emitter, conversationID, source, recover())
}

// HandlePanic processa um valor já recuperado de um panic (r = result of recover()).
// Útil quando o chamador precisa chamar recover() diretamente no seu próprio corpo
// (ex.: dentro de um wrapper method como App.recoverFromPanic).
func HandlePanic(emitter Emitter, conversationID string, source string, r any) {
	if r == nil {
		return
	}
	logging.Errorf(context.Background(), "events.emitter", "🔴 [PANIC RECOVERED] %s (conversa %s): %v", source, conversationID, r)
	func() {
		defer func() { _ = recover() }()
		if emitter != nil {
			emitter.Emit("chat:stream", StreamEvent{
				Done:           true,
				Error:          ports.ChatErrorInternal,
				ConversationId: conversationID,
			})
		}
	}()
}
