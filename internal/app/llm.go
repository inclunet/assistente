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

// Re-exporta funções utilitárias

// ==================== Wails Bindings ====================

// GetModels retorna a lista de modelos disponíveis na API do provedor ativo.
func (a *App) GetModels() ([]string, error) {
	ctx, err := a.requireAuthenticatedContext()
	if err != nil {
		return nil, err
	}
	activeProfile, _ := a.profileManager.GetActive()
	return a.providerSvc.GetModels(ctx, activeProfile)
}

// GetModelsByProvider retorna a lista de modelos de um provedor específico.
func (a *App) GetModelsByProvider(providerID string) ([]string, error) {
	ctx, err := a.requireAuthenticatedContext()
	if err != nil {
		return nil, err
	}
	return a.providerSvc.GetModelsByProvider(ctx, providerID)
}

// RefreshModels relista os modelos do provedor ativo descartando o que ele tiver
// guardado. É o recarregar da tela: para um agente de código, é a única forma de
// ver um modelo que ele passou a oferecer (AEP-0084 D6).
func (a *App) RefreshModels() ([]string, error) {
	ctx, err := a.requireAuthenticatedContext()
	if err != nil {
		return nil, err
	}
	activeProfile, _ := a.profileManager.GetActive()
	return a.providerSvc.RefreshModels(ctx, activeProfile)
}

// RefreshModelsByProvider é o mesmo para um provedor escolhido pelo identificador.
func (a *App) RefreshModelsByProvider(providerID string) ([]string, error) {
	ctx, err := a.requireAuthenticatedContext()
	if err != nil {
		return nil, err
	}
	return a.providerSvc.RefreshModelsByProvider(ctx, providerID)
}

// GetModelCatalogByProvider lista os modelos de um provedor com o nome pelo qual
// cada um quer ser chamado, e diz se quem respondeu é um agente de código. É o
// que a escolha de modelo do perfil usa: o identificador de um modelo de agente
// não é feito para ser lido, e a lista vazia dele quer dizer "quem escolhe sou
// eu" (AEP-0084, Fase 8).
func (a *App) GetModelCatalogByProvider(providerID string) (llm.ModelCatalog, error) {
	ctx, err := a.requireAuthenticatedContext()
	if err != nil {
		return llm.ModelCatalog{}, err
	}
	return a.providerSvc.GetModelCatalogByProvider(ctx, providerID)
}

// RefreshModelCatalogByProvider é o mesmo descartando o que o provedor tiver
// guardado (AEP-0084 D6).
func (a *App) RefreshModelCatalogByProvider(providerID string) (llm.ModelCatalog, error) {
	ctx, err := a.requireAuthenticatedContext()
	if err != nil {
		return llm.ModelCatalog{}, err
	}
	return a.providerSvc.RefreshModelCatalogByProvider(ctx, providerID)
}

// Constantes de validação de input — re-exportadas de internal/chat para uso no pacote main.
const (
	MaxMessageContentSize = chat.MaxMessageContentSize
	MaxMediaSize          = chat.MaxMediaSize
)

// CancelStreamingForConversation cancela o streaming LLM em andamento para uma conversa.
// Usado pelo pipeline SIP para barge-in.
func (a *App) CancelStreamingForConversation(conversationID string) {
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
