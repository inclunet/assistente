package app

import (
	"context"

	"assistente/internal/chat"
	"assistente/internal/events"
	"assistente/internal/llm"
)

// Re-exporta tipos do pacote llm para compatibilidade
type Message = llm.Message
type ContentPart = llm.ContentPart
type ImageURL = llm.ImageURL
type StreamOptions = llm.StreamOptions
type ChatRequest = llm.ChatRequest
type ChatChoice = llm.ChatChoice
type Delta = llm.Delta
type Usage = llm.Usage
type ChatResponse = llm.ChatResponse
type StreamChunk = llm.StreamChunk
type Model = llm.Model
type ModelsResponse = llm.ModelsResponse
type ChatParams = llm.ChatParams

// Constantes de validação de input — re-exportadas de internal/chat para uso no pacote main.
const (
	MaxMessageContentSize = chat.MaxMessageContentSize
	MaxMediaSize          = chat.MaxMediaSize
)

// CancelStreamingForConversation cancela o streaming LLM em andamento para uma
// conversa. Helper de pacote para CLI/testes (não entra no Bind Wails — a
// superfície Wails vive em wailsapi.LLMModels).
func CancelStreamingForConversation(a *App, conversationID string) {
	if a == nil || a.streamMgr == nil {
		return
	}
	a.streamMgr.Cancel(conversationID)
}

// recoverFromPanic captura panic e delega o tratamento para events.HandlePanic.
// recover() deve ser chamado diretamente no corpo da função adiada — não pode ser delegado.
func (a *App) recoverFromPanic(conversationID string, source string) {
	r := recover()
	events.HandlePanic(a.emitter, conversationID, source, r)
}

// registerStreamingContext registra um context cancelável para uma conversa.
func (a *App) registerStreamingContext(conversationID string, cancel context.CancelFunc) {
	a.streamMgr.Register(conversationID, cancel)
}

// unregisterStreamingContext remove o context de streaming de uma conversa.
func (a *App) unregisterStreamingContext(conversationID string) {
	a.streamMgr.Unregister(conversationID)
}
