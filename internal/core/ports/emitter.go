package ports

// Emitter abstrai a emissão de eventos para diferentes interfaces (Wails, CLI, REST, etc.).
// Implementações concretas: adapters/wails.EmitterAdapter (desktop), adapters/noop package (testes).
type Emitter interface {
	Emit(event string, data any)
}

// StreamEvent é o payload do evento chat:stream emitido durante o streaming LLM.
type StreamEvent struct {
	MessageID      string             `json:"messageId"`
	ConversationId string             `json:"conversationId"`
	Content        string             `json:"content"`
	Done           bool               `json:"done"`
	FullResponse   string             `json:"fullResponse,omitempty"`
	Error          string             `json:"error,omitempty"`
	SurfaceOrigin  *ChatSurfaceOrigin `json:"surfaceOrigin,omitempty"`
}
