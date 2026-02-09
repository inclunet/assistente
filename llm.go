package main

import (
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
		}
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
}

// ==================== Wails Bindings ====================

// GetModels retorna a lista de modelos disponíveis na API
func (a *App) GetModels() ([]string, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}
	return llm.GetModels(cfg)
}

// Constantes de validação de input
const (
	// MaxMessageContentSize é o tamanho máximo permitido para o conteúdo de uma mensagem (500KB)
	MaxMessageContentSize = 500 * 1024
	// MaxMediaSize é o tamanho máximo permitido para mídia em base64 (10MB)
	MaxMediaSize = 10 * 1024 * 1024
)

// SendMessage envia uma mensagem para a API com streaming
// NOVA ARQUITETURA: Backend gerencia todo o estado
// 1. Cria mensagens no banco ANTES de streamar
// 2. Emite evento único com IDs das mensagens criadas
// 3. Streaming emite eventos com messageId
func (a *App) SendMessage(conversationID uint, userContent string, userMedia string, params ChatParams) (uint, error) {
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

	if cfg.APIKey == "" {
		runtime.EventsEmit(a.ctx, "chat:error", "API Key não configurada. Por favor, configure sua API Key nas configurações.")
		return 0, fmt.Errorf("API Key não configurada")
	}

	// Se não tem conversationID, cria uma nova conversa
	createdNew := false
	if conversationID == 0 {
		title := userContent
		if len(title) > 50 {
			title = title[:50]
		}
		conv, err := database.CreateConversation(title, params.Model)
		if err != nil {
			runtime.EventsEmit(a.ctx, "chat:error", "Erro ao criar conversa: "+err.Error())
			return 0, err
		}
		conversationID = conv.ID
		createdNew = true
		fmt.Printf("✅ Nova conversa criada: ID=%d, título=%s\n", conversationID, title)

		// Atualiza a tab ativa com o novo conversation_id
		activeTab, err := database.GetActiveTab()
		if err == nil && activeTab != nil {
			err = database.LoadConversationInTab(activeTab.ID, conversationID)
			if err != nil {
				fmt.Printf("⚠️ Erro ao vincular conversa à tab: %v\n", err)
			} else {
				fmt.Printf("✅ Conversa %d vinculada à tab %d\n", conversationID, activeTab.ID)
			}
		}

		// Emite evento de criação para atualizar UI
		runtime.EventsEmit(a.ctx, "chat:conversation_created", map[string]interface{}{
			"id":    conversationID,
			"title": title,
		})
	}

	// Obtém o perfil de chat efetivo da conversa
	chatProfile, err := database.GetEffectiveChatProfile(conversationID)
	if err != nil {
		log.Printf("[SendMessage] Erro ao obter perfil de chat: %v", err)
		// Continua com valores padrão se houver erro
	}

	// Aplica configurações do perfil de chat
	if chatProfile != nil {
		log.Printf("[SendMessage] Usando perfil: %s", chatProfile.Name)

		// 1. Aplica modelo do perfil (se não especificado nos params)
		if params.Model == "" && chatProfile.Model != "" {
			params.Model = chatProfile.Model
			log.Printf("[SendMessage] Modelo do perfil: %s", params.Model)
		}

		// 2. Aplica parâmetros do perfil
		if chatProfile.Temperature > 0 {
			params.Temperature = chatProfile.Temperature
		}
		if chatProfile.MaxTokens > 0 {
			params.MaxTokens = chatProfile.MaxTokens
		}
		if chatProfile.TopP > 0 {
			params.TopP = chatProfile.TopP
		}

		// 3. Aplica configuração de thinking/reasoning
		params.EnableThinking = chatProfile.EnableThinking
	}

	// Se ainda não tem modelo, usa o padrão do config
	if params.Model == "" {
		params.Model = cfg.DefaultModel
		log.Printf("[SendMessage] Usando modelo padrão: %s", params.Model)
	}

	// 1. Salva mensagem do usuário no banco
	userMsg, err := database.AddMessageWithMedia(conversationID, "user", userContent, userMedia)
	if err != nil {
		runtime.EventsEmit(a.ctx, "chat:error", "Erro ao salvar mensagem: "+err.Error())
		return 0, err
	}
	fmt.Printf("✅ Mensagem do usuário salva: ID=%d\n", userMsg.ID)

	// 2. Emite evento informando que mensagem do usuário foi criada
	runtime.EventsEmit(a.ctx, "chat:messages_ready", map[string]interface{}{
		"conversationId": conversationID,
		"userMessageId":  userMsg.ID,
		"createdNew":     createdNew,
	})

	// 3. Carrega histórico da conversa para contexto
	messages, err := a.loadConversationHistory(conversationID)
	if err != nil {
		runtime.EventsEmit(a.ctx, "chat:error", "Erro ao carregar histórico: "+err.Error())
		return 0, err
	}

	// 4. Compõe system prompt completo
	var profileSystemPrompt string
	var systemPromptPosition string
	if chatProfile != nil {
		profileSystemPrompt = chatProfile.SystemPrompt
		systemPromptPosition = chatProfile.SystemPromptPosition
		if systemPromptPosition == "" {
			systemPromptPosition = "before"
		}
	}
	messages = a.buildFullSystemPrompt(messages, profileSystemPrompt, systemPromptPosition)

	// 5. Processa com LLM - userMessageID é a raiz da thread
	handler := &appStreamHandler{
		app:            a,
		conversationID: conversationID,
		userMessageID:  userMsg.ID, // Raiz da thread para interações com agentes
	}
	go llm.StreamChat(a.ctx, cfg, messages, params, handler)
	return conversationID, nil
}

// DefaultSystemPrompt is the base system prompt used when no custom prompt is provided
const DefaultSystemPrompt = `You are a helpful, intelligent assistant. You provide accurate, thoughtful responses and assist users with various tasks.

Key behaviors:
- Be concise but thorough
- When uncertain, acknowledge limitations
- Use markdown formatting for better readability
- Adapt your communication style to the user's needs`

// buildFullSystemPrompt composes the complete system prompt with base prompt and custom prompt
func (a *App) buildFullSystemPrompt(messages []Message, customPrompt string, customPosition string) []Message {
	// Build the system prompt parts
	var parts []string

	// 1. Base prompt (custom or default)
	basePrompt := customPrompt
	if basePrompt == "" {
		basePrompt = DefaultSystemPrompt
	}
	parts = append(parts, basePrompt)

	// Combine all parts
	fullSystemPrompt := strings.Join(parts, "")

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
	if customPosition == "after" {
		// Custom prompt goes after existing content
		combinedContent = existingContent + "\n\n" + fullSystemPrompt
	} else {
		// "before" is the default - custom prompt goes before existing content
		combinedContent = fullSystemPrompt + "\n\n" + existingContent
	}

	// Create new slice to avoid modifying the original
	newMessages := make([]Message, len(messages))
	copy(newMessages, messages)
	newMessages[systemIndex].Content = combinedContent

	return newMessages
}

// MaxContextMessages define o limite de mensagens no contexto para evitar contextos muito grandes
const MaxContextMessages = 50

// loadConversationHistory carrega o histórico de mensagens de uma conversa
// OTIMIZADO: Busca apenas mensagens de nível 0 direto do SQL (sem carregar threads de agentes)
func (a *App) loadConversationHistory(conversationID uint) ([]Message, error) {
	// Busca apenas mensagens raiz (parentID IS NULL) direto do banco
	dbMessages, err := database.GetMessages(conversationID, nil)
	if err != nil {
		return nil, err
	}

	total := len(dbMessages)

	// Limita o contexto às últimas N mensagens para evitar contextos muito grandes
	if total > MaxContextMessages {
		// Mantém as primeiras 2 (system prompt + contexto inicial) + últimas (MaxContextMessages-2)
		kept := MaxContextMessages - 2
		dbMessages = append(dbMessages[:2], dbMessages[total-kept:]...)
		fmt.Printf("📋 [HISTORY] Conversa %d: %d msgs total, truncado para %d (limite: %d)\n",
			conversationID, total, len(dbMessages), MaxContextMessages)
	} else {
		fmt.Printf("📋 [HISTORY] Conversa %d: %d mensagens carregadas\n", conversationID, total)
	}

	messages := make([]Message, 0, len(dbMessages))
	for _, m := range dbMessages {
		msg := Message{
			Role: m.Role,
		}

		// Processa conteúdo (pode ser texto simples ou multimodal)
		if m.Media != "" {
			var mediaParts []map[string]interface{}
			if err := json.Unmarshal([]byte(m.Media), &mediaParts); err == nil {
				var content []interface{}
				if m.Content != "" {
					content = append(content, map[string]interface{}{
						"type": "text",
						"text": m.Content,
					})
				}
				// Converte formato do banco para formato OpenAI
				for _, mp := range mediaParts {
					mediaType, _ := mp["type"].(string)
					data, _ := mp["data"].(string)

					// Determina o formato correto baseado no MIME type
					if strings.HasPrefix(mediaType, "image/") {
						content = append(content, map[string]interface{}{
							"type": "image_url",
							"image_url": map[string]interface{}{
								"url": fmt.Sprintf("data:%s;base64,%s", mediaType, data),
							},
						})
					} else if strings.HasPrefix(mediaType, "audio/") {
						content = append(content, map[string]interface{}{
							"type": "input_audio",
							"input_audio": map[string]interface{}{
								"data":   data,
								"format": strings.TrimPrefix(mediaType, "audio/"),
							},
						})
					} else {
						content = append(content, map[string]interface{}{
							"type": "text",
							"text": fmt.Sprintf("[Arquivo: %s (%s)]", mp["name"], mediaType),
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

	return messages, nil
}

// SendMessageSync envia uma mensagem sem streaming (para acessibilidade)
func (a *App) SendMessageSync(messages []Message, params ChatParams) (string, error) {
	cfg, err := config.Load()
	if err != nil {
		return "", err
	}

	if cfg.APIKey == "" {
		return "", fmt.Errorf("API Key não configurada")
	}

	return llm.SendMessageSync(cfg, messages, params)
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

	// Atualiza o timeout do HTTP client
	llm.ConfigureResponseTimeout(responseTimeout)

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
