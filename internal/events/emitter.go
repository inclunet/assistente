package events

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
