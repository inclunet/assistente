package main

import (
	"context"
	"fmt"
	"log"

	"assistente/internal/chat"
	"assistente/internal/config"
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
type SettingsInput = config.SettingsInput

// Re-exporta funções utilitárias
// ==================== StreamHandler Implementation ====================

// appStreamHandler implementa llm.StreamHandler usando *App
// NOVA ARQUITETURA v2: Hierarquia baseada na mensagem do usuário
// - n0: user/assistant (parentID=null)
// - n1: interações com agentes (parentID=userMessageID)
// - n2: interações do agente com tools (parentID=agentMessageID)

// ==================== Wails Bindings ====================

// GetModels retorna a lista de modelos disponíveis na API do provedor ativo.
func (a *App) GetModels() ([]string, error) {
	activeProfile, _ := a.profileManager.GetActive()
	activeProfile = a.resolveProfileDefaults(activeProfile)

	if activeProfile == nil || activeProfile.Chat.LLMProvider == "" {
		return nil, fmt.Errorf("nenhum provedor LLM configurado no perfil ativo")
	}

	cp, err := a.getChatProviderForProvider(activeProfile.Chat.LLMProvider)
	if err != nil {
		return nil, err
	}
	return cp.GetModels(a.ctx)
}

// GetModelsByProvider retorna a lista de modelos de um provedor específico.
func (a *App) GetModelsByProvider(providerID string) ([]string, error) {
	if providerID == "" {
		return []string{}, nil
	}

	cp, err := a.getChatProviderForProvider(providerID)
	if err != nil {
		return nil, err
	}
	return cp.GetModels(a.ctx)
}

// Constantes de validação de input — re-exportadas de internal/chat para uso no pacote main.
const (
	MaxMessageContentSize = chat.MaxMessageContentSize
	MaxMediaSize          = chat.MaxMediaSize
)

// recoverFromPanic captura panics em goroutines de processamento de mensagens,
// evitando que o app inteiro morra silenciosamente. Emite o erro para o frontend.
func (a *App) recoverFromPanic(conversationID uint, source string) {
	if r := recover(); r != nil {
		errMsg := fmt.Sprintf("Erro interno inesperado em %s: %v", source, r)
		log.Printf("🔴 [PANIC RECOVERED] %s (conversa %d): %v", source, conversationID, r)

		// Emitter pode ser nil (ex: em testes); protege contra panic duplo
		func() {
			defer func() { recover() }()
			if a != nil && a.emitter != nil {
				a.emitter.Emit("chat:stream", StreamEvent{
					Content:        "",
					Done:           true,
					Error:          errMsg,
					ConversationId: conversationID,
				})
			}
		}()
	}
}

// registerStreamingContext registra um context cancelável para uma conversa.
// Permite que barge-in (SIP) ou outros mecanismos cancelem o streaming em andamento.
func (a *App) registerStreamingContext(conversationID uint, cancel context.CancelFunc) {
	a.streamingMu.Lock()
	// Cancela contexto anterior se houver (nova mensagem sobrepõe a anterior)
	if prev, ok := a.streamingContexts[conversationID]; ok {
		prev()
	}
	a.streamingContexts[conversationID] = cancel
	a.streamingMu.Unlock()
}

// unregisterStreamingContext remove o context de streaming de uma conversa.
func (a *App) unregisterStreamingContext(conversationID uint) {
	a.streamingMu.Lock()
	delete(a.streamingContexts, conversationID)
	a.streamingMu.Unlock()
}

// CancelStreamingForConversation cancela o streaming LLM em andamento para uma conversa.
// Usado pelo pipeline SIP para barge-in: quando o usuário fala durante a resposta,
// o LLM é cancelado imediatamente para processar a nova entrada.
func (a *App) CancelStreamingForConversation(conversationID uint) {
	a.streamingMu.Lock()
	cancel, ok := a.streamingContexts[conversationID]
	if ok {
		cancel()
		delete(a.streamingContexts, conversationID)
	}
	a.streamingMu.Unlock()

	// Limpa callbacks pendentes no notifier (resposta não será gerada)
	if ok && a.responseNotifier != nil {
		a.responseNotifier.Cancel(conversationID)
	}

	if ok {
		log.Printf("[LLM] Streaming cancelado para conversa %d (barge-in)", conversationID)
	}
}
