package ports

// Emitter abstrai a emissão de eventos para diferentes interfaces (Wails, CLI, REST, etc.).
// Implementações concretas: adapters/wails.EmitterAdapter (desktop), adapters/noop package (testes).
type Emitter interface {
	Emit(event string, data any)
}

// StreamEvent é o payload do evento chat:stream emitido durante o streaming LLM.
// Delta contém somente o texto novo desde o evento anterior. Reset inicia uma
// nova época de acumulação visual no mesmo turno, tanto em nova tentativa
// quanto após segment_done; BaseContent é o prefixo persistido usado em
// continuação explícita. Sequence é monotônica dentro de cada época.
type StreamEvent struct {
	MessageID            string             `json:"messageId"`
	ConversationId       string             `json:"conversationId"`
	TurnID               string             `json:"turnId"`
	Delta                string             `json:"delta,omitempty"`
	Reset                bool               `json:"reset,omitempty"`
	BaseContent          string             `json:"baseContent,omitempty"`
	Sequence             uint64             `json:"sequence"`
	Done                 bool               `json:"done"`
	Error                string             `json:"error,omitempty"`
	FinishReason         string             `json:"finishReason,omitempty"`
	RawReason            string             `json:"rawReason,omitempty"`
	Provider             string             `json:"provider,omitempty"`
	Model                string             `json:"model,omitempty"`
	EffectiveOutputLimit int                `json:"effectiveOutputLimit,omitempty"`
	OutputTokens         *int               `json:"outputTokens,omitempty"`
	ReasoningTokens      *int               `json:"reasoningTokens,omitempty"`
	ResponseBytes        *int               `json:"responseBytes,omitempty"`
	SurfaceOrigin        *ChatSurfaceOrigin `json:"surfaceOrigin,omitempty"`
}
