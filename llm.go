package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"assistente/internal/config"

	"assistente/internal/database"
	"assistente/internal/llm"
	"assistente/internal/profiles"
	"assistente/internal/skills"
	"assistente/internal/tools"

	"github.com/wailsapp/wails/v2/pkg/runtime"
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
type SettingsInput = llm.SettingsInput

// Re-exporta funções utilitárias
var strPtr = llm.StrPtr

// ==================== StreamHandler Implementation ====================

// appStreamHandler implementa llm.StreamHandler usando *App
// NOVA ARQUITETURA v2: Hierarquia baseada na mensagem do usuário
// - n0: user/assistant (parentID=null)
// - n1: interações com agentes (parentID=userMessageID)
// - n2: interações do agente com tools (parentID=agentMessageID)
type appStreamHandler struct {
	app                *App
	conversationID     uint   // ID da conversa
	userMessageID      uint   // ID da mensagem do usuário (raiz da thread)
	accumulatedContent string // Conteúdo acumulado durante streaming

	// Reasoning/Thinking - chain of thought de modelos como DeepSeek, Claude, o1
	accumulatedReasoning string // Reasoning acumulado durante streaming
	isThinking           bool   // Flag indicando se está recebendo reasoning

	// Throttling para eventos de streaming
	mu            sync.Mutex  // Protege accumulatedContent durante throttling
	lastEmitTime  time.Time   // Última vez que um evento foi emitido
	throttleTimer *time.Timer // Timer para throttling
	pendingEmit   bool        // Flag indicando se há emissão pendente

	// Throttling para eventos de thinking
	lastThinkingEmitTime time.Time   // Última vez que um evento de thinking foi emitido
	thinkingTimer        *time.Timer // Timer para throttling de thinking
	pendingThinkingEmit  bool        // Flag indicando se há emissão de thinking pendente
}

func (h *appStreamHandler) OnChunk(content string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	// Acumula o conteúdo
	h.accumulatedContent += content

	// Throttling: emite eventos apenas a cada 50ms
	const throttleInterval = 50 * time.Millisecond
	now := time.Now()

	// Se já passou tempo suficiente desde última emissão, emite imediatamente
	if now.Sub(h.lastEmitTime) >= throttleInterval {
		h.emitStreamEvent()
		h.lastEmitTime = now
		h.pendingEmit = false

		// Cancela timer pendente se houver
		if h.throttleTimer != nil {
			h.throttleTimer.Stop()
			h.throttleTimer = nil
		}
		return
	}

	// Caso contrário, agenda emissão se ainda não há uma pendente
	if !h.pendingEmit {
		h.pendingEmit = true

		// Calcula tempo restante até próxima emissão permitida
		remainingTime := throttleInterval - now.Sub(h.lastEmitTime)

		h.throttleTimer = time.AfterFunc(remainingTime, func() {
			h.mu.Lock()
			defer h.mu.Unlock()

			if h.pendingEmit {
				h.emitStreamEvent()
				h.lastEmitTime = time.Now()
				h.pendingEmit = false
			}
		})
	}
}

// emitStreamEvent emite o evento de streaming (deve ser chamado com mutex locked)
func (h *appStreamHandler) emitStreamEvent() {
	runtime.EventsEmit(h.app.ctx, "chat:stream", StreamEvent{
		Content:        h.accumulatedContent,
		Done:           false,
		ConversationId: h.conversationID,
	})
}

// OnThinking é chamado quando um chunk de reasoning/thinking é recebido
func (h *appStreamHandler) OnThinking(content string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	// Marca que está pensando
	if !h.isThinking {
		h.isThinking = true
		// Emite evento de início de thinking imediatamente
		runtime.EventsEmit(h.app.ctx, "chat:thinking", map[string]interface{}{
			"content":        content,
			"done":           false,
			"conversationId": h.conversationID,
			"started":        true,
		})
	}

	// Acumula o reasoning
	h.accumulatedReasoning += content

	// Throttling: emite eventos apenas a cada 50ms
	const throttleInterval = 50 * time.Millisecond
	now := time.Now()

	// Se já passou tempo suficiente desde última emissão, emite imediatamente
	if now.Sub(h.lastThinkingEmitTime) >= throttleInterval {
		h.emitThinkingEvent()
		h.lastThinkingEmitTime = now
		h.pendingThinkingEmit = false

		// Cancela timer pendente se houver
		if h.thinkingTimer != nil {
			h.thinkingTimer.Stop()
			h.thinkingTimer = nil
		}
		return
	}

	// Caso contrário, agenda emissão se ainda não há uma pendente
	if !h.pendingThinkingEmit {
		h.pendingThinkingEmit = true

		// Calcula tempo restante até próxima emissão permitida
		remainingTime := throttleInterval - now.Sub(h.lastThinkingEmitTime)

		h.thinkingTimer = time.AfterFunc(remainingTime, func() {
			h.mu.Lock()
			defer h.mu.Unlock()

			if h.pendingThinkingEmit {
				h.emitThinkingEvent()
				h.lastThinkingEmitTime = time.Now()
				h.pendingThinkingEmit = false
			}
		})
	}
}

// emitThinkingEvent emite o evento de thinking (deve ser chamado com mutex locked)
func (h *appStreamHandler) emitThinkingEvent() {
	runtime.EventsEmit(h.app.ctx, "chat:thinking", map[string]interface{}{
		"content":        h.accumulatedReasoning,
		"done":           false,
		"conversationId": h.conversationID,
	})
}

// OnThinkingDone é chamado quando o reasoning termina
func (h *appStreamHandler) OnThinkingDone(fullReasoning string) {
	h.mu.Lock()
	// Cancela qualquer timer pendente
	if h.thinkingTimer != nil {
		h.thinkingTimer.Stop()
		h.thinkingTimer = nil
	}
	h.pendingThinkingEmit = false
	h.isThinking = false
	h.mu.Unlock()

	// Emite evento final de thinking
	runtime.EventsEmit(h.app.ctx, "chat:thinking", map[string]interface{}{
		"content":        fullReasoning,
		"done":           true,
		"conversationId": h.conversationID,
	})

	fmt.Printf("🧠 [THINKING] Reasoning completo: %d caracteres\n", len(fullReasoning))
}

func (h *appStreamHandler) OnError(err string) {
	h.mu.Lock()
	// Cancela qualquer timer pendente
	if h.throttleTimer != nil {
		h.throttleTimer.Stop()
		h.throttleTimer = nil
	}
	h.pendingEmit = false
	content := h.accumulatedContent
	h.mu.Unlock()

	// Emite evento final de erro (sempre emite, sem throttling)
	runtime.EventsEmit(h.app.ctx, "chat:stream", StreamEvent{
		Content:        content,
		Done:           true,
		Error:          err,
		ConversationId: h.conversationID,
	})
}

// OnToolCalls é chamado quando o LLM solicita execução de ferramentas.
// Por enquanto (antes do agentic loop), loga e emite como done.
// Será substituído pelo agentic loop na Fase 5.
func (h *appStreamHandler) OnToolCalls(calls []llm.ToolCall, fullResponse string, usage llm.Usage, model string) {
	fmt.Printf("🔧 [TOOL_CALLS] LLM solicitou %d ferramentas (ainda não implementado)\n", len(calls))
	for _, call := range calls {
		fmt.Printf("   - %s(%s)\n", call.Function.Name, call.Function.Arguments)
	}
	// Delega para OnDone até o agentic loop ser implementado
	h.OnDone(fullResponse, usage, model)
}

func (h *appStreamHandler) OnDone(fullResponse string, usage llm.Usage, model string) {
	// Cancela qualquer timer pendente e obtém conteúdo acumulado
	h.mu.Lock()
	if h.throttleTimer != nil {
		h.throttleTimer.Stop()
		h.throttleTimer = nil
	}
	h.pendingEmit = false
	accumulatedContent := h.accumulatedContent
	accumulatedReasoning := h.accumulatedReasoning
	h.mu.Unlock()

	// Usa o conteúdo acumulado ou o fullResponse
	finalContent := fullResponse
	if finalContent == "" {
		finalContent = accumulatedContent
	}

	// Salva resposta final do assistant no nível 0 (sem parentID)
	// Inclui reasoning se houver
	var savedMsgID uint
	if h.conversationID > 0 && finalContent != "" {
		msg, err := database.CreateMessage(database.MessageOptions{
			ConversationID:   h.conversationID,
			Role:             "assistant",
			Content:          finalContent,
			Reasoning:        accumulatedReasoning,
			PromptTokens:     usage.PromptTokens,
			CompletionTokens: usage.CompletionTokens,
			TotalTokens:      usage.TotalTokens,
			Model:            model,
		})
		if err != nil {
			// Se a conversa foi deletada ou mensagem pai não existe, aborta silenciosamente
			if errors.Is(err, database.ErrConversationDeleted) || errors.Is(err, database.ErrParentMessageDeleted) {
				fmt.Printf("🛑 Conversa %d foi deletada/limpa - abortando processamento\n", h.conversationID)
				return
			}
			fmt.Printf("❌ Erro ao salvar resposta do assistant: %v\n", err)
		} else {
			if accumulatedReasoning != "" {
				fmt.Printf("✅ Resposta do assistant salva: ID=%d (nível 0) com %d chars de reasoning\n", msg.ID, len(accumulatedReasoning))
			} else {
				fmt.Printf("✅ Resposta do assistant salva: ID=%d (nível 0)\n", msg.ID)
			}
			savedMsgID = msg.ID
		}
	}

	// Notifica o gateway de mensageria (se há callbacks pendentes para esta conversa)
	if h.app.responseNotifier != nil {
		h.app.responseNotifier.Notify(h.conversationID, finalContent, savedMsgID)
	}

	// Emite evento final de streaming
	runtime.EventsEmit(h.app.ctx, "chat:stream", StreamEvent{
		Content:        finalContent,
		Done:           true,
		ConversationId: h.conversationID,
		FullResponse:   finalContent,
	})

	// Emite evento para frontend recarregar a conversa
	runtime.EventsEmit(h.app.ctx, "chat:done", map[string]interface{}{
		"conversationId": h.conversationID,
	})

	// Verifica uso do contexto e emite aviso se necessário
	h.checkAndEmitContextWarning()

	// Verifica se precisa sumarizar (após resposta concluída, não bloqueia nada)
	go func() {
		defer h.app.recoverFromPanic(h.conversationID, "checkAndTriggerSummarization")
		h.app.checkAndTriggerSummarization(h.conversationID)
	}()
}

// checkAndEmitContextWarning verifica se a conversa está próxima do limite de contexto
// e emite um evento de aviso para o frontend
func (h *appStreamHandler) checkAndEmitContextWarning() {
	if h.conversationID == 0 {
		return
	}

	stats, err := h.app.GetConversationTokenStats(h.conversationID)
	if err != nil {
		return
	}

	if stats.ContextLimit == 0 {
		return
	}

	runtime.EventsEmit(h.app.ctx, "chat:token_stats", map[string]interface{}{
		"conversationId":   h.conversationID,
		"totalTokens":      stats.TotalTokens,
		"contextLimit":     stats.ContextLimit,
		"contextUsage":     stats.ContextUsage,
		"isNearLimit":      stats.IsNearLimit,
		"isCritical":       stats.IsCritical,
		"promptTokens":     stats.PromptTokens,
		"completionTokens": stats.CompletionTokens,
		"messageCount":     stats.MessageCount,
	})

	if stats.IsCritical {
		runtime.EventsEmit(h.app.ctx, "chat:context_warning", map[string]interface{}{
			"conversationId": h.conversationID,
			"level":          "critical",
			"message": fmt.Sprintf("Atenção: Contexto em %0.1f%% (%d/%d tokens). Considere limpar a conversa ou resumir o histórico.",
				stats.ContextUsage, stats.TotalTokens, stats.ContextLimit),
			"percentage":   stats.ContextUsage,
			"totalTokens":  stats.TotalTokens,
			"contextLimit": stats.ContextLimit,
		})
		fmt.Printf("⚠️  [CONTEXT] Conversa %d em nível CRÍTICO: %0.1f%% (%d/%d tokens)\n",
			h.conversationID, stats.ContextUsage, stats.TotalTokens, stats.ContextLimit)
	} else if stats.IsNearLimit {
		runtime.EventsEmit(h.app.ctx, "chat:context_warning", map[string]interface{}{
			"conversationId": h.conversationID,
			"level":          "warning",
			"message": fmt.Sprintf("Contexto em %0.1f%% (%d/%d tokens). Considere limpar a conversa em breve.",
				stats.ContextUsage, stats.TotalTokens, stats.ContextLimit),
			"percentage":   stats.ContextUsage,
			"totalTokens":  stats.TotalTokens,
			"contextLimit": stats.ContextLimit,
		})
		fmt.Printf("⚠️  [CONTEXT] Conversa %d próxima do limite: %0.1f%% (%d/%d tokens)\n",
			h.conversationID, stats.ContextUsage, stats.TotalTokens, stats.ContextLimit)
	}
}

// ==================== Wails Bindings ====================

// GetModels retorna a lista de modelos disponíveis na API
func (a *App) GetModels() ([]string, error) {
	if a.llmStreamClient == nil {
		log.Printf("[GetModels] llmStreamClient não inicializado. Verifique se há perfil ativo com provedor configurado.")
		return nil, fmt.Errorf("cliente LLM não inicializado. Configure um perfil com provedor LLM primeiro")
	}
	log.Printf("[GetModels] Buscando modelos do provedor...")
	models, err := a.llmStreamClient.GetModels(a.ctx)
	if err != nil {
		log.Printf("[GetModels] Erro ao buscar modelos: %v", err)
		return nil, fmt.Errorf("erro ao buscar modelos: %w", err)
	}
	log.Printf("[GetModels] %d modelos encontrados", len(models))
	return models, nil
}

// GetModelsByProvider retorna a lista de modelos de um provedor específico
func (a *App) GetModelsByProvider(providerID string) ([]string, error) {
	if providerID == "" {
		return []string{}, nil
	}

	provider := a.llmRegistry.Get(providerID)
	if provider == nil {
		return nil, fmt.Errorf("provedor '%s' não encontrado", providerID)
	}

	log.Printf("[GetModelsByProvider] Provedor: %s, Type: %s, Hostname: '%s'", provider.Name, provider.Type, provider.CredentialPattern)

	// Buscar credencial do provedor (opcional para provedores locais)
	var apiKey string
	authCfg, err := a.credMgr.GetByPattern(provider.CredentialPattern)
	if err == nil && authCfg != nil {
		apiKey = authCfg.Token
		log.Printf("[GetModelsByProvider] ✓ Credencial encontrada (len=%d chars)", len(apiKey))
	} else {
		log.Printf("[GetModelsByProvider] ✗ Credencial NÃO encontrada para hostname '%s': %v", provider.CredentialPattern, err)
	}
	// Se não houver credencial, apiKey fica vazio (OK para provedores locais)

	// Criar cliente temporário para buscar modelos
	tempCfg := &config.Config{
		APIKey:     apiKey,
		APIBaseURL: provider.BaseURL,
	}

	if apiKey != "" {
		log.Printf("[GetModelsByProvider] APIKey passada para o client (primeiros 10 chars): %s...", apiKey[:min(10, len(apiKey))])
	} else {
		log.Printf("[GetModelsByProvider] ATENÇÃO: APIKey VAZIA sendo passada para o client!")
	}

	tempClient := llm.NewClient(provider, tempCfg, a.credMgr)

	models, err := tempClient.GetModels(a.ctx)
	if err != nil {
		log.Printf("[GetModelsByProvider] ERRO ao buscar modelos: %v", err)
		return nil, fmt.Errorf("erro ao buscar modelos do provedor '%s': %w", providerID, err)
	}

	log.Printf("[GetModelsByProvider] Sucesso! %d modelos encontrados", len(models))

	return models, nil
}

// Constantes de validação de input
const (
	// MaxMessageContentSize é o tamanho máximo permitido para o conteúdo de uma mensagem (500KB)
	MaxMessageContentSize = 500 * 1024
	// MaxMediaSize é o tamanho máximo permitido para mídia em base64 (10MB)
	MaxMediaSize = 10 * 1024 * 1024
)

// recoverFromPanic captura panics em goroutines de processamento de mensagens,
// evitando que o app inteiro morra silenciosamente. Emite o erro para o frontend.
func (a *App) recoverFromPanic(conversationID uint, source string) {
	if r := recover(); r != nil {
		errMsg := fmt.Sprintf("Erro interno inesperado em %s: %v", source, r)
		log.Printf("🔴 [PANIC RECOVERED] %s (conversa %d): %v", source, conversationID, r)

		// EventsEmit requer contexto Wails válido; protege contra panic duplo
		// (ex: em testes ou se o contexto foi invalidado)
		func() {
			defer func() { recover() }()
			if a != nil && a.ctx != nil {
				runtime.EventsEmit(a.ctx, "chat:stream", StreamEvent{
					Content:        "",
					Done:           true,
					Error:          errMsg,
					ConversationId: conversationID,
				})
			}
		}()
	}
}

// SendMessage é o binding Wails para envio de mensagens. Source padrão: "wails".
// Se a conversa pertence a um canal externo (Signal, Telegram), a resposta do assistente
// também será reenviada ao mensageiro de origem (bridge bidirecional).
func (a *App) SendMessage(conversationID uint, userContent string, userMedia string, params ChatParams) (uint, error) {
	// Bridge: se a conversa é de canal externo, registra callback para reenviar resposta
	if conversationID > 0 && a.msgGateway != nil && a.responseNotifier != nil {
		a.registerChannelBridge(conversationID)
	}
	return a.sendMessageInternal(conversationID, userContent, userMedia, params, "wails")
}

// SendMessageFromChannel é chamado pelo Gateway de mensageria.
// Funciona como SendMessage mas permite especificar a origem (source).
func (a *App) SendMessageFromChannel(conversationID uint, content, media string, params ChatParams, source string) (uint, error) {
	return a.sendMessageInternal(conversationID, content, media, params, source)
}

// sendMessageInternal contém a lógica de processamento de mensagens.
// Usado por SendMessage (Wails) e SendMessageFromChannel (mensageiros).
func (a *App) sendMessageInternal(conversationID uint, userContent string, userMedia string, params ChatParams, source string) (uint, error) {
	// Validação de tamanho do conteúdo
	if len(userContent) > MaxMessageContentSize {
		errMsg := fmt.Sprintf("Mensagem muito grande (%d bytes). Máximo permitido: %d bytes", len(userContent), MaxMessageContentSize)
		runtime.EventsEmit(a.ctx, "chat:error", errMsg)
		return 0, fmt.Errorf("%s", errMsg)
	}

	// Validação de tamanho da mídia
	if len(userMedia) > MaxMediaSize {
		errMsg := fmt.Sprintf("Mídia muito grande (%d bytes). Máximo permitido: %d bytes", len(userMedia), MaxMediaSize)
		runtime.EventsEmit(a.ctx, "chat:error", errMsg)
		return 0, fmt.Errorf("%s", errMsg)
	}

	cfg, err := config.Load()
	if err != nil {
		runtime.EventsEmit(a.ctx, "chat:error", "Erro ao carregar configuração: "+err.Error())
		return 0, err
	}

	// Verifica se há alguma forma de autenticação disponível.
	// A APIKey no config.json é legada; provedores modernos usam o credential manager.
	// Só bloqueia se não houver NENHUMA credencial configurada (nem config, nem provedores).
	if cfg.APIKey == "" {
		providerCount, _ := database.CountLLMProviders()
		if providerCount == 0 {
			runtime.EventsEmit(a.ctx, "chat:error", "Nenhum provedor LLM configurado. Configure um provedor nas configurações.")
			return 0, fmt.Errorf("nenhum provedor LLM configurado")
		}
	}

	if conversationID == 0 {
		const errMsg = "conversationID é obrigatório — conversas devem ser criadas ao criar/resetar a tab"
		runtime.EventsEmit(a.ctx, "chat:error", errMsg)
		return 0, errors.New(errMsg)
	}

	// Auto-rename: se a conversa tem título genérico, atualiza com o conteúdo da primeira mensagem.
	if conversationID > 0 && userContent != "" {
		conv, convErr := database.GetConversationInfo(conversationID)
		if convErr == nil && conv != nil && conv.Title == "Nova Conversa" {
			title := userContent
			if len(title) > 50 {
				title = title[:50]
			}
			if err := database.UpdateConversation(conversationID, title, ""); err == nil {
				activeTab, tabErr := database.GetActiveTab()
				if tabErr == nil && activeTab != nil && activeTab.ConversationID != nil && *activeTab.ConversationID == conversationID {
					_ = database.UpdateTabTitle(activeTab.ID, title)
					runtime.EventsEmit(a.ctx, "tab:title_updated", map[string]interface{}{
						"tab_id":    activeTab.ID,
						"new_title": title,
					})
				}
				runtime.EventsEmit(a.ctx, "conversation:renamed", map[string]interface{}{
					"conversation_id": conversationID,
					"new_title":       title,
				})
			}
		}
	}

	// Obtém o perfil: usa profileSlug se especificado (canais), senão o ativo global
	var activeProfile *profiles.Profile
	var profileErr error
	if a.profileManager == nil {
		log.Printf("[SendMessage] profileManager não inicializado — continuando sem perfil")
	} else if params.ProfileSlug != "" {
		activeProfile, profileErr = a.profileManager.Get(params.ProfileSlug)
		if profileErr != nil {
			log.Printf("[SendMessage] Erro ao obter perfil '%s' do canal: %v — usando perfil ativo global", params.ProfileSlug, profileErr)
			activeProfile, profileErr = a.profileManager.GetActive()
		}
	} else {
		activeProfile, profileErr = a.profileManager.GetActive()
	}
	if profileErr != nil {
		log.Printf("[SendMessage] Erro ao obter perfil: %v", profileErr)
	}

	// Aplica configurações do perfil de chat ativo
	if activeProfile != nil {
		log.Printf("[SendMessage] Usando perfil: %s", activeProfile.Name)

		// 1. Aplica modelo do perfil (se não especificado nos params)
		if params.Model == "" && activeProfile.Chat.Model != "" {
			params.Model = activeProfile.Chat.Model
			log.Printf("[SendMessage] Modelo do perfil: %s", params.Model)
		}

		// 2. Aplica parâmetros do perfil
		if activeProfile.Chat.Temperature > 0 {
			params.Temperature = activeProfile.Chat.Temperature
		}
		if activeProfile.Chat.MaxTokens > 0 {
			params.MaxTokens = activeProfile.Chat.MaxTokens
		}
		if activeProfile.Chat.TopP > 0 {
			params.TopP = activeProfile.Chat.TopP
		}
		// MaxTokensMode: "legacy" (max_tokens) ou "completion_tokens" (max_completion_tokens)
		if activeProfile.Chat.MaxTokensMode != "" {
			params.MaxTokensMode = activeProfile.Chat.MaxTokensMode
		}

		// 3. Aplica configuração de reasoning effort
		params.ReasoningEffort = activeProfile.Chat.ReasoningEffort

		// 4. Aplica limites do agentic loop
		if activeProfile.Chat.MaxAgenticIterations > 0 {
			params.MaxAgenticIterations = activeProfile.Chat.MaxAgenticIterations
		}
		if activeProfile.Chat.ResponseTimeout > 0 {
			params.ResponseTimeout = activeProfile.Chat.ResponseTimeout
		}
	}

	// Se ainda não tem modelo, usa o padrão do config
	if params.Model == "" {
		params.Model = cfg.DefaultModel
		log.Printf("[SendMessage] Usando modelo padrão: %s", params.Model)
	}

	// 0.5. Se userContent está vazio e há mídia de áudio, transcreve automaticamente.
	// Isso garante que a transcrição fique salva no DB e visível na UI.
	// Também extrai o áudio base64 para persistir junto com a mensagem.
	var userAudioBase64 string
	var userAudioMime string
	if userMedia != "" {
		userAudioBase64, userAudioMime = extractAudioFromMedia(userMedia)
	}
	if userContent == "" && userMedia != "" {
		// Verifica se o perfil tem STT configurado como whisper_api (necessário para canais)
		if source != "wails" && activeProfile != nil {
			sttProvider := activeProfile.Interaction.STTProvider
			if sttProvider == "webspeech" || sttProvider == "" {
				log.Printf("[SendMessage] Canal %s: STT '%s' não suporta transcrição server-side — ignorando áudio", source, sttProvider)
				userContent = "[Mensagem de áudio recebida, mas transcrição automática não está configurada. Configure Whisper no perfil deste canal para processar mensagens de voz.]"
			}
		}
		if userContent == "" {
			userContent = a.transcribeAudioFromMedia(userMedia)
		}
	}

	// 1. Salva mensagem do usuário no banco (com source para badge visual e áudio persistido)
	userMsg, err := database.CreateMessage(database.MessageOptions{
		ConversationID: conversationID,
		Role:           "user",
		Content:        userContent,
		Media:          userMedia,
		Audio:          userAudioBase64,
		AudioMimeType:  userAudioMime,
		Source:         source,
	})
	if err != nil {
		runtime.EventsEmit(a.ctx, "chat:error", "Erro ao salvar mensagem: "+err.Error())
		return 0, err
	}
	fmt.Printf("✅ Mensagem do usuário salva: ID=%d (source=%s)\n", userMsg.ID, source)

	// 2. Emite evento informando que mensagem do usuário foi criada
	//    Inclui o conteúdo para que o frontend atualize mensagens de canais (ex: transcrição de áudio)
	runtime.EventsEmit(a.ctx, "chat:messages_ready", map[string]interface{}{
		"conversationId": conversationID,
		"userMessageId":  userMsg.ID,
		"userContent":    userMsg.Content,
	})

	// 3. Carrega histórico da conversa para contexto (com rolling context)
	messages, conversationSummary, err := a.loadConversationHistory(conversationID, activeProfile)
	if err != nil {
		runtime.EventsEmit(a.ctx, "chat:error", "Erro ao carregar histórico: "+err.Error())
		return 0, err
	}

	// 3.5. Detecta invocação de skill via /slash command
	var slashSkillContent string
	invokedSkillSlug := ""
	var invokedFilesystemScope *tools.FilesystemScope

	// Contexto de template para skills (disponível via {{ .Profile }} e flags derivadas)
	// Isso permite que skills ajustem instruções conforme o perfil ativo (ex.: toolcalling ligado/desligado).
	skillTplData := a.buildSkillTemplateData(activeProfile, params.ProfileSlug)
	if slug, args, ok := parseSlashCommand(userContent); ok && a.skillMgr != nil {
		skill, err := a.skillMgr.Get(slug)
		if err == nil && skill.IsUserInvocable() {
			log.Printf("[Skills] Slash command detectado: /%s args=%q", slug, args)
			invokedSkillSlug = slug
			if skill.Filesystem != nil {
				invokedFilesystemScope = &tools.FilesystemScope{
					Read:  append([]string{}, skill.Filesystem.Read...),
					Write: append([]string{}, skill.Filesystem.Write...),
					Deny:  append([]string{}, skill.Filesystem.Deny...),
				}
			}

			// Substitui $ARGUMENTS, $N, e variáveis de sessão no conteúdo
			sessionVars := map[string]string{
				"CLAUDE_SESSION_ID": fmt.Sprintf("%d", conversationID),
			}
			processedContent := skills.SubstituteArguments(skill.Content, args, sessionVars)
			processedContent = skills.ProcessTemplate(processedContent, skillTplData)

			// Preprocessa !commands (respeita permissões de bash do skill)
			var allowedBashCmds []string
			if skill.Tools != nil && skill.Tools.BashCommands != nil {
				allowedBashCmds = skill.Tools.BashCommands.Allowed
			}
			processedContent = skills.PreprocessCommands(processedContent, allowedBashCmds)

			// Monta seção de contexto do skill invocado
			var sb strings.Builder
			sb.WriteString("<invoked_skill>\n")
			sb.WriteString("## ")
			sb.WriteString(skill.GetDisplayName())
			if skill.Type != "" {
				sb.WriteString(" [")
				sb.WriteString(skill.Type)
				sb.WriteString("]")
			}
			sb.WriteString("\n")
			sb.WriteString(processedContent)
			sb.WriteString("\n")

			// Progressive file loading: lista arquivos complementares do skill
			supplementary, _ := a.skillMgr.GetSkillFiles(slug)
			if len(supplementary) > 0 {
				sb.WriteString("\nSupporting files (use read_file to access when needed):\n")
				for _, f := range supplementary {
					sb.WriteString("- `")
					sb.WriteString(f)
					sb.WriteString("`\n")
				}
			}

			sb.WriteString("</invoked_skill>")
			slashSkillContent = sb.String()
		} else if err != nil {
			log.Printf("[Skills] Skill /%s não encontrado: %v", slug, err)
		}
	}

	// 4. Compõe system prompt completo (com skills do perfil)
	var enabledSkills []string
	var disableOnDemand bool
	if activeProfile != nil {
		enabledSkills = activeProfile.Chat.EnabledSkills
		disableOnDemand = activeProfile.Chat.DisableOnDemandSkills
		if activeProfile.Chat.DisableSkills {
			enabledSkills = []string{}
		}
	}
	messages = a.buildFullSystemPrompt(messages, enabledSkills, disableOnDemand, skillTplData, slashSkillContent, conversationSummary)

	// 5. Pré-processamento de mídia:
	//    a) Converte formatos de áudio não suportados (aac, ogg, etc.) para texto via Whisper
	//    b) Se MediaSupport indica que o modelo não suporta áudio/documento, aplica fallback
	messages = a.preprocessMediaMessages(messages, activeProfile)

	// 6. Processa com LLM
	// Determina quais ferramentas estão habilitadas pelo perfil ativo
	var llmToolDefs []llm.ToolDefinition
	disableTools := activeProfile != nil && activeProfile.Chat.DisableTools

	if !disableTools && a.toolRegistry != nil && a.toolRegistry.Count() > 0 {
		var toolDefs []tools.ToolDefinition

		// Filtra ferramentas pelo perfil: nil/não especificado = todas, lista = apenas as listadas
		if activeProfile != nil && activeProfile.Chat.EnabledTools != nil {
			toolDefs = a.toolRegistry.FilterByNames(activeProfile.Chat.EnabledTools)
		} else {
			toolDefs = a.toolRegistry.ToDefinitions()
		}

		llmToolDefs = make([]llm.ToolDefinition, len(toolDefs))
		for i, td := range toolDefs {
			llmToolDefs[i] = llm.ToolDefinition{
				Type: td.Type,
				Function: llm.FunctionDefinition{
					Name:        td.Function.Name,
					Description: td.Function.Description,
					Parameters:  td.Function.Parameters,
				},
			}
		}
	}



	// Resolve o client correto para o provedor do perfil ativo.
	// Isso garante que o request seja enviado ao endpoint correto,
	// mesmo quando o perfil selecionado usa um provedor diferente do global.
	requestClient := a.llmStreamClient
	if activeProfile != nil && activeProfile.Chat.LLMProvider != "" {
		if client, err := a.getClientForProvider(activeProfile.Chat.LLMProvider); err == nil {
			requestClient = client
			log.Printf("[SendMessage] Client resolvido para provedor do perfil: %s", activeProfile.Chat.LLMProvider)
		} else {
			log.Printf("[SendMessage] Erro ao resolver client para provedor '%s': %v — usando client global", activeProfile.Chat.LLMProvider, err)
		}
	}

	if requestClient == nil {
		providerID := ""
		if activeProfile != nil {
			providerID = activeProfile.Chat.LLMProvider
		}
		errMsg := "Cliente LLM não disponível. Verifique se o provedor está configurado corretamente."
		log.Printf("[SendMessage] ERRO: requestClient é nil (llmStreamClient=%v, profile.LLMProvider=%q)",
			a.llmStreamClient == nil, providerID)
		runtime.EventsEmit(a.ctx, "chat:error", errMsg)
		return 0, fmt.Errorf("%s", errMsg)
	}

	// Se há ferramentas disponíveis, usa o agentic loop; caso contrário, streaming simples
	if len(llmToolDefs) > 0 {
		agentCtx := a.ctx
		if invokedSkillSlug != "" {
			agentCtx = tools.WithExecutionContext(agentCtx, tools.ExecutionContext{
				InvokedSkillSlug: invokedSkillSlug,
				Filesystem:       invokedFilesystemScope,
			})
		}
		go func() {
			defer a.recoverFromPanic(conversationID, "runAgenticLoop")
			a.runAgenticLoop(agentCtx, cfg, messages, params, conversationID, userMsg.ID, llmToolDefs, requestClient)
		}()
	} else {
		// Sem ferramentas: streaming simples (comportamento original)
		handler := &appStreamHandler{
			app:            a,
			conversationID: conversationID,
			userMessageID:  userMsg.ID,
		}
		go func() {
			defer a.recoverFromPanic(conversationID, "StreamChat")
			requestClient.StreamChat(a.ctx, messages, params, handler)
		}()
	}
	return conversationID, nil
}

// DefaultSystemPrompt is the base system prompt used when no custom prompt is provided
const DefaultSystemPrompt = `You are a helpful, intelligent assistant. You provide accurate, thoughtful responses and assist users with various tasks.

Key behaviors:
- Be concise but thorough
- When uncertain, acknowledge limitations
- Use markdown formatting for better readability
- Adapt your communication style to the user's needs`

// buildFullSystemPrompt composes the complete system prompt with DefaultSystemPrompt, skills injection, invoked skill, and conversation summary.
// enabledSkills: nil = todos os skills, [] = nenhum, ["slug1","slug2"] = apenas esses.
// slashSkillContent: conteúdo processado de um skill invocado via /slash (pode ser vazio).
// conversationSummary: resumo de mensagens antigas da conversa (rolling context).
func (a *App) buildFullSystemPrompt(messages []Message, enabledSkills []string, disableOnDemand bool, skillTplData any, slashSkillContent string, conversationSummary string) []Message {
	// Build the system prompt parts
	var parts []string

	// 1. Base prompt (DefaultSystemPrompt)
	// Only add DefaultSystemPrompt if skills are present or slash skill is invoked
	// (avoids "Developer instruction not enabled" on simple models)
	if len(enabledSkills) > 0 || slashSkillContent != "" {
		parts = append(parts, DefaultSystemPrompt)
	}

	// 2. Skills injection (auto + available)
	skillsSection := a.buildSkillsPromptSection(enabledSkills, disableOnDemand, skillTplData)
	if skillsSection != "" {
		parts = append(parts, "\n\n"+skillsSection)
	}

	// 2.5. Invoked skill via /slash command
	if slashSkillContent != "" {
		parts = append(parts, "\n\n"+slashSkillContent)
	}

	// 3. Conversation summary (rolling context)
	if conversationSummary != "" {
		parts = append(parts, "\n\n<conversation_summary>\nSummary of earlier messages in this conversation (these messages are no longer in the context window but their content is captured below):\n\n"+conversationSummary+"\n</conversation_summary>")
	}

	// Combine all parts
	fullSystemPrompt := strings.Join(parts, "")

	// If no system prompt was built, don't add a system message at all
	if fullSystemPrompt == "" {
		return messages
	}

	// Find existing system message or create new one
	systemIndex := -1
	for i, msg := range messages {
		if msg.Role == "system" {
			systemIndex = i
			break
		}
	}

	if systemIndex == -1 {
		// No existing system message, add at the beginning
		systemMsg := Message{
			Role:    "system",
			Content: fullSystemPrompt,
		}
		return append([]Message{systemMsg}, messages...)
	}

	// Existing system message found - combine based on position
	existingContent := ""
	switch content := messages[systemIndex].Content.(type) {
	case string:
		existingContent = content
	default:
		// If not a string, replace entirely
		newMessages := make([]Message, len(messages))
		copy(newMessages, messages)
		newMessages[systemIndex].Content = fullSystemPrompt
		return newMessages
	}

	var combinedContent string
	// Always prepend system prompt before existing content
	combinedContent = fullSystemPrompt + "\n\n" + existingContent

	// Create new slice to avoid modifying the original
	newMessages := make([]Message, len(messages))
	copy(newMessages, messages)
	newMessages[systemIndex].Content = combinedContent

	return newMessages
}

type skillTemplateData struct {
	Profile            *profiles.Profile
	ProfileSlug        string
	ToolCallingEnabled bool
	EnabledTools       []string
	EnabledToolCount   int
}

func (a *App) buildSkillTemplateData(activeProfile *profiles.Profile, profileSlug string) skillTemplateData {
	enabledToolNames := a.computeEnabledToolNames(activeProfile)
	return skillTemplateData{
		Profile:            activeProfile,
		ProfileSlug:        profileSlug,
		ToolCallingEnabled: len(enabledToolNames) > 0,
		EnabledTools:       enabledToolNames,
		EnabledToolCount:   len(enabledToolNames),
	}
}

func (a *App) computeEnabledToolNames(activeProfile *profiles.Profile) []string {
	if activeProfile != nil && activeProfile.Chat.DisableTools {
		return nil
	}
	if a.toolRegistry == nil || a.toolRegistry.Count() == 0 {
		return nil
	}

	var toolDefs []tools.ToolDefinition
	if activeProfile != nil && activeProfile.Chat.EnabledTools != nil {
		toolDefs = a.toolRegistry.FilterByNames(activeProfile.Chat.EnabledTools)
	} else {
		toolDefs = a.toolRegistry.ToDefinitions()
	}
	if len(toolDefs) == 0 {
		return nil
	}

	names := make([]string, 0, len(toolDefs))
	for _, td := range toolDefs {
		names = append(names, td.Function.Name)
	}
	return names
}

// buildSkillsPromptSection constrói a seção de skills para o system prompt.
// enabledSkills: nil = usa auto_load do skill, [] = nenhum (skills desabilitados), ["slug1","slug2"] = autoload ordenado.
// disableOnDemand: true = não incluir skills sob demanda.
func (a *App) buildSkillsPromptSection(enabledSkills []string, disableOnDemand bool, skillTemplateData any) string {
	if a.skillMgr == nil {
		return ""
	}

	// Se enabledSkills é um slice vazio (não nil), skills estão explicitamente desabilitados
	if enabledSkills != nil && len(enabledSkills) == 0 {
		return ""
	}

	var autoSkills []skills.Skill
	var availableSkills []skills.Skill

	if enabledSkills != nil {
		// Perfil define lista explícita de autoload (ordenada)
		allSkills, err := a.skillMgr.GetAllSkillsFull()
		if err != nil {
			log.Printf("[Skills] Erro ao carregar skills: %v", err)
			return ""
		}

		autoSkills = skills.FilterByNamesOrdered(allSkills, enabledSkills)
		if !disableOnDemand {
			availableSkills = skills.FilterExcludeNames(allSkills, enabledSkills)
		}
	} else {
		// Sem lista no perfil: usa auto_load do próprio skill (backward compat)
		var err error
		autoSkills, err = a.skillMgr.GetAutoSkills()
		if err != nil {
			log.Printf("[Skills] Erro ao carregar auto skills: %v", err)
		}

		if !disableOnDemand {
			availableSkills, err = a.skillMgr.GetAvailableSkills()
			if err != nil {
				log.Printf("[Skills] Erro ao carregar available skills: %v", err)
			}
		}
	}

	if len(autoSkills) == 0 && len(availableSkills) == 0 {
		return ""
	}

	var sb strings.Builder

	// Seção de skills auto_load (conteúdo completo injetado)
	if len(autoSkills) > 0 {
		sb.WriteString("<auto_skills>\n")
		for i, s := range autoSkills {
			if i > 0 {
				sb.WriteString("\n")
			}
			sb.WriteString("## ")
			sb.WriteString(s.GetDisplayName())
			if s.Type != "" {
				sb.WriteString(" [")
				sb.WriteString(s.Type)
				sb.WriteString("]")
			}
			sb.WriteString("\n")

			autoContent := s.Content
			autoContent = skills.ProcessTemplate(autoContent, skillTemplateData)
			var allowedBashCmds []string
			if s.Tools != nil && s.Tools.BashCommands != nil {
				allowedBashCmds = s.Tools.BashCommands.Allowed
			}
			autoContent = skills.PreprocessCommands(autoContent, allowedBashCmds)

			sb.WriteString(autoContent)
			sb.WriteString("\n")

			// Progressive file loading: lista arquivos complementares do auto skill
			if a.skillMgr != nil {
				supplementary, _ := a.skillMgr.GetSkillFiles(s.Slug)
				if len(supplementary) > 0 {
					sb.WriteString("\nSupporting files (use read_file to access when needed):\n")
					for _, f := range supplementary {
						sb.WriteString("- `")
						sb.WriteString(f)
						sb.WriteString("`\n")
					}
				}
			}
		}
		sb.WriteString("</auto_skills>")
	}

	// Seção de skills disponíveis (referência para leitura via read_file)
	// Filtra skills com disable-model-invocation: true (modelo não pode invocá-los)
	var modelInvocableSkills []skills.Skill
	for _, s := range availableSkills {
		if s.IsModelInvocable() {
			modelInvocableSkills = append(modelInvocableSkills, s)
		}
	}

	if len(modelInvocableSkills) > 0 {
		if sb.Len() > 0 {
			sb.WriteString("\n\n")
		}
		sb.WriteString("<available_skills>\n")
		sb.WriteString("You have skills available that provide specialized instructions for specific tasks.\n")
		sb.WriteString("To use a skill, read its file using the read_file tool with the path indicated below.\n")
		sb.WriteString("Only read a skill when it's relevant to the current task.\n\n")
		for _, s := range modelInvocableSkills {
			sb.WriteString("- **")
			sb.WriteString(s.GetDisplayName())
			sb.WriteString("** (`")
			sb.WriteString(s.Slug)
			sb.WriteString("`)")
			if s.Type != "" {
				sb.WriteString(" [")
				sb.WriteString(s.Type)
				sb.WriteString("]")
			}
			sb.WriteString(": ")
			sb.WriteString(s.Description)
			sb.WriteString("\n  Path: `")
			sb.WriteString(s.Path)
			sb.WriteString("`\n")

			// Progressive file loading: lista arquivos complementares do skill
			if a.skillMgr != nil {
				supplementary, _ := a.skillMgr.GetSkillFiles(s.Slug)
				if len(supplementary) > 0 {
					sb.WriteString("  Supporting files:\n")
					for _, f := range supplementary {
						sb.WriteString("    - `")
						sb.WriteString(f)
						sb.WriteString("`\n")
					}
				}
			}
		}
		sb.WriteString("</available_skills>")
	}

	return sb.String()
}

// DefaultMaxContextMessages é o limite padrão de mensagens no contexto
const DefaultMaxContextMessages = 50

// loadConversationHistory carrega o histórico de mensagens de uma conversa.
// Respeita rolling context: se há resumo, exclui mensagens já resumidas do contexto.
// O perfil define MaxContextMessages (limite de msgs) e ContextWindow (trigger de sumarização).
func (a *App) loadConversationHistory(conversationID uint, profile *profiles.Profile) ([]Message, string, error) {
	maxCtxMsgs := DefaultMaxContextMessages
	if profile != nil {
		maxCtxMsgs = profile.GetMaxContextMessages()
	}

	// Busca resumo existente da conversa
	existingSummary, summaryUpToID, err := database.GetConversationSummary(conversationID)
	if err != nil {
		log.Printf("[HISTORY] Erro ao buscar resumo da conversa %d: %v", conversationID, err)
		existingSummary = ""
		summaryUpToID = 0
	}

	// Busca todas as mensagens raiz para avaliação de sumarização
	allRootMessages, err := database.GetMessages(conversationID, nil)
	if err != nil {
		return nil, "", err
	}

	// Filtra mensagens para o contexto: apenas as que vêm depois do resumo
	var dbMessages []database.ChatMessage
	if summaryUpToID > 0 {
		for _, m := range allRootMessages {
			if m.ID > summaryUpToID {
				dbMessages = append(dbMessages, m)
			}
		}
		fmt.Printf("📋 [HISTORY] Conversa %d: %d msgs total, %d após resumo (upToID=%d)\n",
			conversationID, len(allRootMessages), len(dbMessages), summaryUpToID)
	} else {
		dbMessages = allRootMessages
	}

	total := len(dbMessages)

	// Truncação por limite de mensagens no contexto (MaxContextMessages).
	// Corta no limite de uma mensagem role="user", preservando turns completos.
	if total > maxCtxMsgs {
		cutIndex := -1
		for i := total - 1; i >= 2; i-- {
			if dbMessages[i].Role == "user" {
				msgCount := 2 + (total - i)
				if msgCount > maxCtxMsgs {
					break
				}
				cutIndex = i
			}
		}

		if cutIndex > 2 {
			dbMessages = append(dbMessages[:2], dbMessages[cutIndex:]...)
			fmt.Printf("📋 [HISTORY] Conversa %d: truncado para %d msgs (corte em user msg idx %d)\n",
				conversationID, len(dbMessages), cutIndex)
		} else {
			kept := maxCtxMsgs - 2
			if kept > total {
				kept = total
			}
			dbMessages = append(dbMessages[:2], dbMessages[total-kept:]...)
			fmt.Printf("📋 [HISTORY] Conversa %d: truncado para %d msgs (fallback)\n",
				conversationID, len(dbMessages))
		}
	} else {
		fmt.Printf("📋 [HISTORY] Conversa %d: %d mensagens no contexto\n", conversationID, total)
	}

	// Safety net: ensure every tool_use has its tool_result and vice-versa.
	offeredIDs := make(map[string]bool)
	answeredIDs := make(map[string]bool)
	for _, m := range dbMessages {
		if m.ToolCalls != "" {
			var tcs []struct {
				ID string `json:"id"`
			}
			if json.Unmarshal([]byte(m.ToolCalls), &tcs) == nil {
				for _, tc := range tcs {
					offeredIDs[tc.ID] = true
				}
			}
		}
		if m.Role == "tool" && m.ToolCallID != "" {
			answeredIDs[m.ToolCallID] = true
		}
	}

	// Pass 2: remove orphaned tool_results and strip orphaned tool_calls from assistant messages.
	cleaned := make([]database.ChatMessage, 0, len(dbMessages))
	for _, m := range dbMessages {
		if m.Role == "tool" && m.ToolCallID != "" && !offeredIDs[m.ToolCallID] {
			fmt.Printf("📋 [HISTORY] Removendo tool_result órfão: %s\n", m.ToolCallID)
			continue
		}
		if m.ToolCalls != "" {
			var tcs []json.RawMessage
			var tcsParsed []struct {
				ID string `json:"id"`
			}
			if json.Unmarshal([]byte(m.ToolCalls), &tcs) == nil && json.Unmarshal([]byte(m.ToolCalls), &tcsParsed) == nil {
				var kept []json.RawMessage
				for i, tc := range tcsParsed {
					if answeredIDs[tc.ID] {
						kept = append(kept, tcs[i])
					} else {
						fmt.Printf("📋 [HISTORY] Removendo tool_use órfão: %s\n", tc.ID)
					}
				}
				if len(kept) == 0 {
					m.ToolCalls = ""
				} else if len(kept) < len(tcs) {
					if j, err := json.Marshal(kept); err == nil {
						m.ToolCalls = string(j)
					}
				}
			}
		}
		cleaned = append(cleaned, m)
	}
	dbMessages = cleaned

	// Garante que a primeira mensagem no contexto é uma user message
	for len(dbMessages) > 0 && dbMessages[0].Role != "user" {
		dbMessages = dbMessages[1:]
	}

	messages := make([]Message, 0, len(dbMessages))
	for _, m := range dbMessages {
		msg := Message{
			Role:       m.Role,
			ToolCallID: m.ToolCallID,
		}

		// Reconstrói tool_calls do JSON armazenado no banco (para mensagens assistant com tool calls)
		if m.ToolCalls != "" {
			var toolCalls []llm.ToolCall
			if err := json.Unmarshal([]byte(m.ToolCalls), &toolCalls); err == nil {
				msg.ToolCalls = toolCalls
			}
		}

		// Processa conteúdo (pode ser texto simples ou multimodal)
		if m.Media != "" {
			var mediaParts []map[string]interface{}
			if err := json.Unmarshal([]byte(m.Media), &mediaParts); err == nil {
				var content []interface{}
				// Verifica se a mensagem já tem conteúdo textual (ex: transcrição de áudio já feita)
				hasTextContent := m.Content != ""
				if hasTextContent {
					content = append(content, map[string]interface{}{
						"type": "text",
						"text": m.Content,
					})
				}
				// Converte formato do banco para formato OpenAI (multimodal)
				for _, mp := range mediaParts {
					mediaType, _ := mp["type"].(string)
					data, _ := mp["data"].(string)
					name, _ := mp["name"].(string)

					if strings.HasPrefix(mediaType, "image/") {
						// Imagens: formato image_url com data URI
						content = append(content, map[string]interface{}{
							"type": "image_url",
							"image_url": map[string]interface{}{
								"url": fmt.Sprintf("data:%s;base64,%s", mediaType, data),
							},
						})
					} else if strings.HasPrefix(mediaType, "audio/") {
						// Se já temos transcrição no Content, não re-transcreve o áudio
						// (evita duplicação: content já tem o texto transcrito)
						if hasTextContent {
							log.Printf("[Media] Áudio ignorado no histórico — já temos transcrição no content")
							continue
						}
						audioFmt := strings.TrimPrefix(mediaType, "audio/")
						if supportedAudioFormats[audioFmt] {
							// Formato suportado (wav, mp3): envia direto como input_audio
							content = append(content, map[string]interface{}{
								"type": "input_audio",
								"input_audio": map[string]interface{}{
									"data":   data,
									"format": audioFmt,
								},
							})
						} else {
							// Formato não suportado (aac, ogg, webm, etc.):
							// tenta transcrever com Whisper imediatamente
							transcribed := false
							if a.ensureSpeechManager() {
								filename := whisperFilename(audioFmt)
								log.Printf("[Media] Tentando transcrever áudio %s via Whisper (filename=%s)", audioFmt, filename)
								result, err := a.speechManager.Transcribe(data, filename)
								if err == nil && result.Text != "" {
									log.Printf("[Media] Áudio %s transcrito via Whisper ao carregar histórico: %s", audioFmt, truncateStr(result.Text, 100))
									content = append(content, map[string]interface{}{
										"type": "text",
										"text": result.Text,
									})
									transcribed = true
								} else if err != nil {
									log.Printf("[Media] Erro ao transcrever %s via Whisper: %v", audioFmt, err)
								}
							}
							if !transcribed {
								// NUNCA enviar formato não suportado como input_audio.
								// Placeholder textual para não quebrar a API.
								log.Printf("[Media] Áudio %s não transcrito — adicionando placeholder textual", audioFmt)
								content = append(content, map[string]interface{}{
									"type": "text",
									"text": fmt.Sprintf("[Mensagem de áudio recebida (%s) — não foi possível transcrever]", audioFmt),
								})
							}
						}
					} else if mediaType == "application/pdf" || strings.HasPrefix(mediaType, "text/") {
						// Documentos (PDF, texto): envia como file para modelos que suportam
						content = append(content, map[string]interface{}{
							"type": "file",
							"file": map[string]interface{}{
								"filename":  name,
								"data":      data,
								"mime_type": mediaType,
							},
						})
					} else if strings.HasPrefix(mediaType, "video/") {
						// Vídeo: tenta enviar direto (Gemini suporta)
						content = append(content, map[string]interface{}{
							"type": "video",
							"video": map[string]interface{}{
								"data":      data,
								"mime_type": mediaType,
							},
						})
					} else {
						// Outros formatos: informa ao modelo como texto
						content = append(content, map[string]interface{}{
							"type": "text",
							"text": fmt.Sprintf("[Arquivo anexado: %s (%s)]", name, mediaType),
						})
					}
				}
				msg.Content = content
			} else {
				msg.Content = m.Content
			}
		} else {
			msg.Content = m.Content
		}

		messages = append(messages, msg)
	}

	return messages, existingSummary, nil
}

// extractAudioFromMedia extrai o primeiro áudio base64 e MIME do mediaJSON.
// Retorna ("", "") se não houver áudio.
func extractAudioFromMedia(mediaJSON string) (string, string) {
	var mediaParts []map[string]interface{}
	if err := json.Unmarshal([]byte(mediaJSON), &mediaParts); err != nil {
		return "", ""
	}
	for _, mp := range mediaParts {
		mediaType, _ := mp["type"].(string)
		data, _ := mp["data"].(string)
		if strings.HasPrefix(mediaType, "audio/") && data != "" {
			return data, mediaType
		}
	}
	return "", ""
}

// transcribeAudioFromMedia extrai áudio do mediaJSON e transcreve via Whisper.
// Retorna o texto transcrito, ou "" se não houver áudio ou falhar.
func (a *App) transcribeAudioFromMedia(mediaJSON string) string {
	var mediaParts []map[string]interface{}
	if err := json.Unmarshal([]byte(mediaJSON), &mediaParts); err != nil {
		return ""
	}

	var transcriptions []string
	for _, mp := range mediaParts {
		mediaType, _ := mp["type"].(string)
		data, _ := mp["data"].(string)

		if !strings.HasPrefix(mediaType, "audio/") || data == "" {
			continue
		}

		audioFmt := strings.TrimPrefix(mediaType, "audio/")
		if !a.ensureSpeechManager() {
			log.Printf("[Transcribe] speechManager indisponível para transcrever áudio %s", audioFmt)
			continue
		}

		filename := whisperFilename(audioFmt)
		log.Printf("[Transcribe] Transcrevendo áudio %s antes de salvar (filename=%s)", audioFmt, filename)
		result, err := a.speechManager.Transcribe(data, filename)
		if err != nil {
			log.Printf("[Transcribe] Erro ao transcrever áudio %s: %v", audioFmt, err)
			continue
		}
		if result.Text != "" {
			log.Printf("[Transcribe] Áudio transcrito: %s", truncateStr(result.Text, 100))
			transcriptions = append(transcriptions, result.Text)
		}
	}

	return strings.Join(transcriptions, "\n")
}

// Formatos de áudio suportados nativamente pela API OpenAI input_audio.
var supportedAudioFormats = map[string]bool{
	"wav": true, "mp3": true,
}

// whisperExtensionMap mapeia extensões/formatos para extensões aceitas pelo Whisper.
// Whisper aceita: flac, m4a, mp3, mp4, mpeg, mpga, oga, ogg, wav, webm.
// AAC é o codec dentro de M4A, então mapeamos aac → m4a.
var whisperExtensionMap = map[string]string{
	"aac":  "m4a",
	"opus": "ogg",
}

// whisperFilename retorna um nome de arquivo com extensão compatível com Whisper.
func whisperFilename(format string) string {
	if mapped, ok := whisperExtensionMap[format]; ok {
		return fmt.Sprintf("audio.%s", mapped)
	}
	return fmt.Sprintf("audio.%s", format)
}

// preprocessMediaMessages percorre as mensagens e:
//   - Converte formatos de áudio não suportados (aac, ogg, webm, etc.) para texto via Whisper
//   - Se MediaSupport.Audio = false, transcreve todo áudio com Whisper
//   - Se MediaSupport.Document = false, converte documentos em texto placeholder
func (a *App) preprocessMediaMessages(messages []Message, profile *profiles.Profile) []Message {
	var ms *profiles.MediaSupport
	if profile != nil {
		ms = profile.MediaSupport
	}

	for i, msg := range messages {
		content, ok := msg.Content.([]interface{})
		if !ok {
			continue
		}

		var newContent []interface{}
		for _, part := range content {
			partMap, ok := part.(map[string]interface{})
			if !ok {
				newContent = append(newContent, part)
				continue
			}

			partType, _ := partMap["type"].(string)

			// Áudio: verifica formato e suporte do modelo
			if partType == "input_audio" {
				audioMap, _ := partMap["input_audio"].(map[string]interface{})
				if audioMap != nil {
					audioData, _ := audioMap["data"].(string)
					audioFmt, _ := audioMap["format"].(string)

					// Decide se precisa transcrever com Whisper:
					// 1. Formato não suportado pela API (aac, ogg, webm, m4a, etc.)
					// 2. Perfil explicitamente marca que modelo não suporta áudio
					needsWhisper := !supportedAudioFormats[audioFmt]
					if ms != nil && ms.Audio != nil && !*ms.Audio {
						needsWhisper = true
					}

					if needsWhisper {
						transcribed := false
						if audioData != "" && a.ensureSpeechManager() {
							filename := whisperFilename(audioFmt)
							log.Printf("[Preprocess] Tentando transcrever áudio %s via Whisper (filename=%s)", audioFmt, filename)
							result, err := a.speechManager.Transcribe(audioData, filename)
							if err == nil && result.Text != "" {
								log.Printf("[Preprocess] Áudio %s transcrito via Whisper: %s", audioFmt, truncateStr(result.Text, 100))
								newContent = append(newContent, map[string]interface{}{
									"type": "text",
									"text": result.Text,
								})
								transcribed = true
							} else if err != nil {
								log.Printf("[Preprocess] Erro ao transcrever áudio %s: %v", audioFmt, err)
							}
						}
						if !transcribed {
							// NUNCA enviar formato não suportado — placeholder textual
							log.Printf("[Preprocess] Áudio %s não transcrito — placeholder textual", audioFmt)
							newContent = append(newContent, map[string]interface{}{
								"type": "text",
								"text": fmt.Sprintf("[Mensagem de áudio recebida (%s) — não foi possível transcrever]", audioFmt),
							})
						}
						continue
					}
				}
			}

			// Documento: verifica suporte do modelo
			if partType == "file" && ms != nil && ms.Document != nil && !*ms.Document {
				fileMap, _ := partMap["file"].(map[string]interface{})
				if fileMap != nil {
					fname, _ := fileMap["filename"].(string)
					mime, _ := fileMap["mime_type"].(string)
					newContent = append(newContent, map[string]interface{}{
						"type": "text",
						"text": fmt.Sprintf("[Documento anexado: %s (%s) — modelo não suporta documentos nativamente]", fname, mime),
					})
					continue
				}
			}

			newContent = append(newContent, part)
		}
		messages[i].Content = newContent
	}

	return messages
}

// UpdateProfileMediaSupport atualiza o MediaSupport de um perfil e salva.
// Chamado quando detectamos que um modelo não suporta determinado tipo de mídia.
func (a *App) UpdateProfileMediaSupport(mediaType string, supported bool) {
	if a.profileManager == nil {
		return
	}

	profile, err := a.profileManager.GetActive()
	if err != nil || profile == nil {
		return
	}

	if profile.MediaSupport == nil {
		profile.MediaSupport = &profiles.MediaSupport{}
	}

	switch mediaType {
	case "audio":
		profile.MediaSupport.Audio = &supported
	case "image":
		profile.MediaSupport.Image = &supported
	case "document":
		profile.MediaSupport.Document = &supported
	case "video":
		profile.MediaSupport.Video = &supported
	}

	slug := a.profileManager.GetActiveSlug()
	if slug == "" {
		return
	}
	if err := a.profileManager.Update(slug, profile); err != nil {
		log.Printf("[MediaSupport] Erro ao salvar perfil: %v", err)
	} else {
		log.Printf("[MediaSupport] Perfil atualizado: %s=%v", mediaType, supported)
	}
}

// truncateStr encurta uma string para exibição em logs.
func truncateStr(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// SendMessageSync envia uma mensagem sem streaming (para acessibilidade)
func (a *App) SendMessageSync(messages []Message, params ChatParams) (string, error) {
	if a.llmStreamClient == nil {
		return "", fmt.Errorf("cliente LLM não inicializado. Configure um perfil com provedor LLM primeiro")
	}
	return a.llmStreamClient.SendMessageSync(a.ctx, messages, params)
}

// GetConfig retorna a configuração atual
func (a *App) GetConfig() (*config.Config, error) {
	return config.Load()
}

// SetChatModel atualiza apenas o modelo de chat na configuração
func (a *App) SetChatModel(model string) error {
	err := config.Update(func(existing *config.Config) *config.Config {
		existing.DefaultModel = model
		existing.ChatParams.Model = model
		return existing
	})
	if err != nil {
		return err
	}

	// Recarrega o cliente LLM para usar o novo modelo
	a.initLLMClient()

	log.Printf("[SetChatModel] Modelo atualizado para: %s", model)
	return nil
}

// SaveSettings salva as configurações
func (a *App) SaveSettings(input SettingsInput) error {
	// Aplica timeout padrão se não especificado
	responseTimeout := input.ResponseTimeout
	if responseTimeout <= 0 {
		responseTimeout = 180
	}

	err := config.Update(func(existing *config.Config) *config.Config {
		return &config.Config{
			APIKey:          input.APIKey,
			APIBaseURL:      input.APIBaseURL,
			DefaultModel:    input.ChatParams.Model,
			ResponseTimeout: responseTimeout,
			ChatParams: config.ModelParams{
				Model:       input.ChatParams.Model,
				Temperature: input.ChatParams.Temperature,
				MaxTokens:   input.ChatParams.MaxTokens,
				TopP:        input.ChatParams.TopP,
			},
			STTParams: config.STTParams{
				Provider:      input.STTParams.Provider,
				RecordingMode: input.STTParams.RecordingMode,
			},
		}
	})
	if err != nil {
		return err
	}

	return nil
}

// SetDefaultModel salva o modelo padrão
func (a *App) SetDefaultModel(model string) error {
	return config.Update(func(cfg *config.Config) *config.Config {
		cfg.DefaultModel = model
		return cfg
	})
}

// TestConnection testa a conexão com a API
func (a *App) TestConnection() (bool, error) {
	models, err := a.GetModels()
	if err != nil {
		return false, err
	}
	if len(models) > 0 {
		return true, nil
	}
	return false, fmt.Errorf("nenhum modelo encontrado")
}

// TestConnectionWithModels testa a conexão e retorna os modelos disponíveis
func (a *App) TestConnectionWithModels() ([]string, error) {
	models, err := a.GetModels()
	if err != nil {
		return nil, fmt.Errorf("erro ao conectar com a API: %v", err)
	}
	if len(models) == 0 {
		return nil, fmt.Errorf("nenhum modelo encontrado na API")
	}
	return models, nil
}

// ResetConfig apaga o arquivo de configuração, resetando ao estado padrão
func (a *App) ResetConfig() error {
	configPath, err := config.GetConfigPath()
	if err != nil {
		return fmt.Errorf("erro ao obter caminho da configuração: %v", err)
	}

	// Verifica se o arquivo existe
	if _, err := os.Stat(configPath); err == nil {
		// Remove o arquivo
		if err := os.Remove(configPath); err != nil {
			return fmt.Errorf("erro ao remover arquivo de configuração: %v", err)
		}
	}

	return nil
}

// ResetDatabase apaga o banco de dados, resetando ao estado inicial
func (a *App) ResetDatabase() error {
	configPath, err := config.GetConfigPath()
	if err != nil {
		return fmt.Errorf("erro ao obter caminho do banco de dados: %v", err)
	}

	dbPath := filepath.Join(filepath.Dir(configPath), "conversations.db")

	// Fecha a conexão com o banco de dados antes de deletar
	if err := database.Close(); err != nil {
		return fmt.Errorf("erro ao fechar banco de dados: %v", err)
	}

	// Aguarda um momento para garantir que o arquivo foi liberado
	time.Sleep(100 * time.Millisecond)

	// Verifica se o arquivo existe
	if _, err := os.Stat(dbPath); err == nil {
		// Remove o banco de dados
		if err := os.Remove(dbPath); err != nil {
			return fmt.Errorf("erro ao remover banco de dados: %v", err)
		}

		// Remove arquivos auxiliares do SQLite (WAL e SHM)
		os.Remove(dbPath + "-wal")
		os.Remove(dbPath + "-shm")
	}

	// Reinicializa o banco de dados
	if err := database.Init(); err != nil {
		return fmt.Errorf("erro ao reinicializar banco: %v", err)
	}

	// Limpa as tabs (já que todas as conversas foram deletadas)
	if err := database.ClearAllTabs(); err != nil {
		log.Printf("[ResetDatabase] Erro ao limpar tabs: %v", err)
	}

	log.Println("[ResetDatabase] Banco resetado com sucesso")

	// Emite evento para o frontend limpar o estado
	runtime.EventsEmit(a.ctx, "database:reset")

	return nil
}

// ClearMessages apaga todas as mensagens e conversas, mantendo a estrutura do banco
func (a *App) ClearMessages() error {
	if err := database.ClearAllTabs(); err != nil {
		return fmt.Errorf("erro ao limpar mensagens e conversas: %v", err)
	}

	log.Println("[ClearMessages] Mensagens e conversas apagadas")
	runtime.EventsEmit(a.ctx, "messages:cleared")

	return nil
}

// ClearAllCredentials apaga todas as credenciais armazenadas
func (a *App) ClearAllCredentials() error {
	if a.credMgr == nil {
		return fmt.Errorf("gerenciador de credenciais não disponível")
	}

	// Limpa todas as credenciais usando DeletePattern com um padrão que pega tudo
	// (isso é uma abordagem simples - em produção seria melhor ter um método Clear específico)
	if err := a.credMgr.DeletePattern(context.Background(), ""); err != nil {
		return fmt.Errorf("erro ao limpar credenciais: %v", err)
	}

	log.Println("[ClearAllCredentials] Credenciais apagadas")
	runtime.EventsEmit(a.ctx, "credentials:cleared")

	return nil
}

// ClearAllProfiles apaga todos os perfis, mantendo apenas o ativo padrão
func (a *App) ClearAllProfiles() error {
	if a.profileManager == nil {
		return fmt.Errorf("gerenciador de perfis não disponível")
	}

	profiles, err := a.profileManager.List()
	if err != nil {
		return fmt.Errorf("erro ao listar perfis: %v", err)
	}

	for _, profile := range profiles {
		if err := a.profileManager.Delete(profile.Slug); err != nil {
			log.Printf("[ClearAllProfiles] Erro ao deletar perfil %s: %v", profile.Slug, err)
		}
	}

	log.Println("[ClearAllProfiles] Perfis apagados")
	runtime.EventsEmit(a.ctx, "profiles:cleared")

	return nil
}

// ClearAllSkills apaga todos os skills
func (a *App) ClearAllSkills() error {
	if a.skillMgr == nil {
		return fmt.Errorf("gerenciador de skills não disponível")
	}

	skills, err := a.skillMgr.List()
	if err != nil {
		return fmt.Errorf("erro ao listar skills: %v", err)
	}

	for _, skill := range skills {
		if err := a.skillMgr.Delete(skill.Slug); err != nil {
			log.Printf("[ClearAllSkills] Erro ao deletar skill %s: %v", skill.Slug, err)
		}
	}

	log.Println("[ClearAllSkills] Skills apagados")
	runtime.EventsEmit(a.ctx, "skills:cleared")

	return nil
}

// ClearAllChannels apaga todos os canais de comunicação
func (a *App) ClearAllChannels() error {
	if a.msgGateway == nil {
		return fmt.Errorf("gateway de mensageria não disponível")
	}

	status := a.msgGateway.GetStatus()
	for channelType := range status {
		if err := a.RestartChannel(channelType); err != nil {
			log.Printf("[ClearAllChannels] Erro ao resetar canal %s: %v", channelType, err)
		}
	}

	log.Println("[ClearAllChannels] Canais apagados")
	runtime.EventsEmit(a.ctx, "channels:cleared")

	return nil
}

// parseSlashCommand detecta se uma mensagem é um slash command para invocar um skill.
// Formato: /skill-slug [argumentos...]
// Retorna (slug, args, true) se for um slash command válido, ("", "", false) caso contrário.
func parseSlashCommand(content string) (slug string, args string, ok bool) {
	content = strings.TrimSpace(content)

	// Deve começar com /
	if !strings.HasPrefix(content, "/") {
		return "", "", false
	}

	// Remove o /
	rest := content[1:]
	if rest == "" {
		return "", "", false
	}

	// Não deve começar com espaço (evita confusão com paths como "/ something")
	if rest[0] == ' ' {
		return "", "", false
	}

	// Separa slug dos argumentos (pelo primeiro espaço)
	parts := strings.SplitN(rest, " ", 2)
	slug = strings.ToLower(parts[0])

	// Valida que o slug parece um nome de skill (letras, números, hifens)
	for _, ch := range slug {
		if !((ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9') || ch == '-') {
			return "", "", false
		}
	}

	if len(parts) > 1 {
		args = strings.TrimSpace(parts[1])
	}

	return slug, args, true
}
