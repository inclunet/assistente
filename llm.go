package main

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

// Re-exporta funções utilitárias

// ==================== Wails Bindings ====================

// GetModels retorna a lista de modelos disponíveis na API do provedor ativo.
func (a *App) GetModels() ([]string, error) {
	activeProfile, _ := a.profileManager.GetActive()
	return a.providerSvc.GetModels(a.ctx, activeProfile)
}

// GetModelsByProvider retorna a lista de modelos de um provedor específico.
func (a *App) GetModelsByProvider(providerID string) ([]string, error) {
	return a.providerSvc.GetModelsByProvider(a.ctx, providerID)
}

// Constantes de validação de input — re-exportadas de internal/chat para uso no pacote main.
const (
	MaxMessageContentSize = chat.MaxMessageContentSize
	MaxMediaSize          = chat.MaxMediaSize
)

// CancelStreamingForConversation cancela o streaming LLM em andamento para uma conversa.
// Usado pelo pipeline SIP para barge-in.
func (a *App) CancelStreamingForConversation(conversationID uint) {
	a.streamMgr.Cancel(conversationID)
}

// recoverFromPanic captura panic e delega o tratamento para events.HandlePanic.
// recover() deve ser chamado diretamente no corpo da função adiada — não pode ser delegado.
func (a *App) recoverFromPanic(conversationID uint, source string) {
	r := recover()
	events.HandlePanic(a.emitter, conversationID, source, r)
}

// registerStreamingContext registra um context cancelável para uma conversa.
func (a *App) registerStreamingContext(conversationID uint, cancel context.CancelFunc) {
	a.streamMgr.Register(conversationID, cancel)
}

// unregisterStreamingContext remove o context de streaming de uma conversa.
func (a *App) unregisterStreamingContext(conversationID uint) {
	a.streamMgr.Unregister(conversationID)
}
