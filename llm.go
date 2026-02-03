package main

import (
	"bytes"
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
type ToolCall = llm.ToolCall
type FunctionCall = llm.FunctionCall
type Tool = llm.Tool
type ToolFunction = llm.ToolFunction
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
	conversationID     uint           // ID da conversa
	userMessageID      uint           // ID da mensagem do usuário (raiz da thread)
	accumulatedContent string         // Conteúdo acumulado durante streaming
	accumulatedCalls   []llm.ToolCall // Acumula tool calls
	accumulatedResults []string       // Acumula resultados de tools

	// Chat Profile - tools filtradas
	filteredTools []llm.Tool // Lista de tools filtradas pelo perfil

	// Throttling para eventos de streaming
	mu            sync.Mutex  // Protege accumulatedContent durante throttling
	lastEmitTime  time.Time   // Última vez que um evento foi emitido
	throttleTimer *time.Timer // Timer para throttling
	pendingEmit   bool        // Flag indicando se há emissão pendente
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
	h.mu.Unlock()

	// Usa o conteúdo acumulado ou o fullResponse
	finalContent := fullResponse
	if finalContent == "" {
		finalContent = accumulatedContent
	}

	// Salva resposta final do assistant no nível 0 (sem parentID)
	if h.conversationID > 0 && finalContent != "" {
		msg, err := database.CreateMessage(database.MessageOptions{
			ConversationID:   h.conversationID,
			Role:             "assistant",
			Content:          finalContent,
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
			fmt.Printf("✅ Resposta do assistant salva: ID=%d (nível 0)\n", msg.ID)
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

func (h *appStreamHandler) OnToolCalls(toolCalls []llm.ToolCall, usage llm.Usage, model string) {
	// Acumula as tool calls
	h.accumulatedCalls = append(h.accumulatedCalls, toolCalls...)

	// Salva mensagem do assistant com tool_calls como filha da mensagem do usuário (nível 1)
	if h.conversationID > 0 && h.userMessageID > 0 {
		toolCallsJSON, _ := json.Marshal(toolCalls)

		// Formata conteúdo markdown com as tool calls
		var contentBuilder strings.Builder

		if len(toolCalls) == 1 {
			// Uma única tool call - formato mais simples
			tc := toolCalls[0]
			name := tc.Function.Name
			displayName := name
			if strings.HasPrefix(name, "delegate_to_") {
				displayName = strings.TrimPrefix(name, "delegate_to_")
			}

			// Extrai tarefa se houver
			var args map[string]interface{}
			taskContent := ""
			if err := json.Unmarshal([]byte(tc.Function.Arguments), &args); err == nil {
				if task, ok := args["task"].(string); ok {
					taskContent = task
				}
			}

			if taskContent != "" {
				contentBuilder.WriteString(fmt.Sprintf("🤖 **Consultando %s**\n\n", displayName))
				contentBuilder.WriteString(fmt.Sprintf("**Tarefa:** %s\n\n", taskContent))
				contentBuilder.WriteString("**Argumentos:**\n```json\n")
			} else {
				contentBuilder.WriteString(fmt.Sprintf("🔧 **Executando:** `%s`\n\n", displayName))
				contentBuilder.WriteString("**Argumentos:**\n```json\n")
			}

			// Formata JSON
			var prettyArgs bytes.Buffer
			if err := json.Indent(&prettyArgs, []byte(tc.Function.Arguments), "", "  "); err == nil {
				contentBuilder.WriteString(prettyArgs.String())
			} else {
				contentBuilder.WriteString(tc.Function.Arguments)
			}
			contentBuilder.WriteString("\n```")
		} else {
			// Múltiplas tool calls
			contentBuilder.WriteString(fmt.Sprintf("🔧 **Executando %d ferramentas:**\n\n", len(toolCalls)))
			for i, tc := range toolCalls {
				contentBuilder.WriteString(fmt.Sprintf("%d. **%s**\n```json\n", i+1, tc.Function.Name))
				var prettyArgs bytes.Buffer
				if err := json.Indent(&prettyArgs, []byte(tc.Function.Arguments), "", "  "); err == nil {
					contentBuilder.WriteString(prettyArgs.String())
				} else {
					contentBuilder.WriteString(tc.Function.Arguments)
				}
				contentBuilder.WriteString("\n```\n")
				if i < len(toolCalls)-1 {
					contentBuilder.WriteString("\n")
				}
			}
		}

		content := contentBuilder.String()

		// Salva como filho da mensagem do usuário (nível 1)
		// role="assistant" porque é o assistente falando com o agente
		// agentName VAZIO porque é o assistente fazendo a chamada (não o agente respondendo)
		msg, err := database.AddChildMessage(
			h.conversationID,
			h.userMessageID, // ParentID = mensagem do usuário
			"assistant",
			content,
			string(toolCallsJSON),
			"",
			"", // agentName VAZIO - é o assistente chamando, não o agente respondendo
			model,
		)
		if err != nil {
			// Se a conversa foi deletada ou mensagem pai não existe, aborta silenciosamente
			if errors.Is(err, database.ErrConversationDeleted) || errors.Is(err, database.ErrParentMessageDeleted) {
				fmt.Printf("🛑 Conversa %d foi deletada/limpa - abortando processamento de tool calls\n", h.conversationID)
				return
			}
			fmt.Printf("❌ Erro ao salvar delegação: %v\n", err)
		} else {
			fmt.Printf("✅ Delegação salva: ID=%d, parentID=%d (nível 1)\n", msg.ID, h.userMessageID)
			// Define no App para os agentes usarem como ParentID (nível 2)
			h.app.currentDelegationID = msg.ID

			// Emite evento para o frontend mostrar mensagem interna em tempo real
			runtime.EventsEmit(h.app.ctx, "chat:internal_message", map[string]interface{}{
				"id":        msg.ID,
				"parentId":  h.userMessageID,
				"role":      "assistant",
				"content":   content,
				"toolCalls": string(toolCallsJSON),
				"internal":  true,
			})
		}

		// Emite evento de streaming
		runtime.EventsEmit(h.app.ctx, "chat:stream", StreamEvent{
			Content:        "🔧 Executando ferramentas...",
			Done:           false,
			ConversationId: h.conversationID,
		})
	}
}

func (h *appStreamHandler) OnToolsExecuting(toolNames []string) {
	runtime.EventsEmit(h.app.ctx, "chat:tools", map[string]interface{}{
		"tools":   toolNames,
		"message": fmt.Sprintf("Executando %d ferramenta(s): %s", len(toolNames), strings.Join(toolNames, ", ")),
	})
}

func (h *appStreamHandler) OnToolResults(results []string, usage llm.Usage, model string) {
	// Acumula os resultados
	h.accumulatedResults = append(h.accumulatedResults, results...)

	// Salva cada tool result como filha da mensagem do usuário (nível 1)
	// Estas são as respostas dos agentes
	if h.conversationID > 0 && h.userMessageID > 0 {
		for i, result := range results {
			var toolCallID string
			var agentName string
			callIndex := len(h.accumulatedCalls) - len(results) + i
			if callIndex >= 0 && callIndex < len(h.accumulatedCalls) {
				tc := h.accumulatedCalls[callIndex]
				toolCallID = tc.ID
				toolName := tc.Function.Name

				// Extrai nome do agente
				if strings.HasPrefix(toolName, "delegate_to_") {
					agentName = strings.TrimPrefix(toolName, "delegate_to_")
				} else {
					agentName = toolName
				}
			}

			// Salva como filho da mensagem do usuário (nível 1)
			// role="agent" para indicar que é o agente respondendo (não "tool")
			msg, err := database.AddChildMessage(
				h.conversationID,
				h.userMessageID, // ParentID = mensagem do usuário
				"agent",         // role="agent" para distinguir de tool results internos
				result,
				"",         // toolCalls
				toolCallID, // toolCallID
				agentName,  // agentName identifica qual agente respondeu
				model,
			)
			if err != nil {
				// Se a conversa foi deletada ou mensagem pai não existe, aborta silenciosamente
				if errors.Is(err, database.ErrConversationDeleted) || errors.Is(err, database.ErrParentMessageDeleted) {
					fmt.Printf("🛑 Conversa %d foi deletada/limpa - abortando processamento de resposta do agente\n", h.conversationID)
					return
				}
				fmt.Printf("❌ Erro ao salvar resposta do agente: %v\n", err)
			} else {
				fmt.Printf("✅ Resposta do agente salva: ID=%d, parentID=%d, agent=%s (nível 1)\n",
					msg.ID, h.userMessageID, agentName)

				// Emite evento para o frontend mostrar resposta do agente em tempo real
				runtime.EventsEmit(h.app.ctx, "chat:internal_message", map[string]interface{}{
					"id":         msg.ID,
					"parentId":   h.userMessageID,
					"role":       "agent",
					"content":    result,
					"agentName":  agentName,
					"toolCallId": toolCallID,
					"internal":   true,
				})
			}
		}
	}

	// Emite evento para feedback visual
	runtime.EventsEmit(h.app.ctx, "chat:tool_results", map[string]interface{}{
		"results": results,
	})
}

func (h *appStreamHandler) GetTools() []llm.Tool {
	// Se há tools filtradas pelo perfil, usa elas
	if h.filteredTools != nil {
		return h.filteredTools
	}
	// Fallback: retorna todas as tools
	return h.app.GetToolsForAPI()
}

func (h *appStreamHandler) ExecuteTool(tc llm.ToolCall) (string, error) {
	// Define o contexto no App para os agentes poderem salvar mensagens
	h.app.currentConversationID = h.conversationID
	// currentDelegationID é definido em OnToolCalls (nível 1 → nível 2)

	fmt.Printf("🔧 [EXECUTE] Tool: %s, conversationID=%d, delegationID=%d\n",
		tc.Function.Name, h.conversationID, h.app.currentDelegationID)

	return h.app.ExecuteTool(tc)
}

func (h *appStreamHandler) SupportsVision(model string) (bool, error) {
	return h.app.ModelSupportsVision(model)
}

func (h *appStreamHandler) SetVisionSupport(model string, supports bool) {
	h.app.SetModelVisionSupport(model, supports)
}

func (h *appStreamHandler) GetImageModel() string {
	cfg, err := config.Load()
	if err != nil {
		return ""
	}
	return cfg.ImageModel
}

func (h *appStreamHandler) GenerateImageDescription(imageBase64, model string) (string, error) {
	return h.app.GenerateImageDescription(imageBase64, model)
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

	// Obtém o perfil de conversa efetivo (da conversa ou padrão)
	chatProfile, err := database.GetEffectiveChatProfile(conversationID)
	if err != nil {
		log.Printf("[SendMessage] Erro ao obter perfil de conversa: %v (usando defaults)", err)
	}

	// Aplica configurações do perfil se disponível
	var filteredTools []llm.Tool
	var profileSystemPrompt string
	var systemPromptPosition string

	if chatProfile != nil {
		log.Printf("[SendMessage] Usando perfil de conversa: %s (ID=%d)", chatProfile.Name, chatProfile.ID)

		// 1. Aplica modelo do perfil (se não especificado nos params e perfil tem modelo)
		if params.Model == "" && chatProfile.Model != "" {
			params.Model = chatProfile.Model
			log.Printf("[SendMessage] Modelo do perfil: %s", params.Model)
		}

		// 2. Aplica parâmetros do perfil (sobrescreve defaults)
		if chatProfile.Temperature > 0 {
			params.Temperature = chatProfile.Temperature
		}
		if chatProfile.MaxTokens > 0 {
			params.MaxTokens = chatProfile.MaxTokens
		}
		if chatProfile.TopP > 0 {
			params.TopP = chatProfile.TopP
		}

		// 3. Aplica configuração de tools
		params.UseTools = chatProfile.UseTools

		// 4. Filtra tools baseado no perfil
		if chatProfile.UseTools {
			allowedTools := chatProfile.GetToolsList()
			if len(allowedTools) > 0 {
				// Filtra tools para apenas as permitidas
				allTools := a.GetToolsForAPI()
				filteredTools = make([]llm.Tool, 0)
				for _, tool := range allTools {
					for _, allowed := range allowedTools {
						if tool.Function.Name == allowed || strings.HasSuffix(tool.Function.Name, "_"+allowed) || strings.HasPrefix(tool.Function.Name, "delegate_to_"+allowed) {
							filteredTools = append(filteredTools, tool)
							break
						}
					}
				}
				log.Printf("[SendMessage] Tools filtradas: %d de %d", len(filteredTools), len(allTools))
			} else {
				// Lista vazia = todas as tools
				filteredTools = a.GetToolsForAPI()
			}
		}

		// 5. System prompt do perfil
		profileSystemPrompt = chatProfile.SystemPrompt
		systemPromptPosition = chatProfile.SystemPromptPosition
		if systemPromptPosition == "" {
			systemPromptPosition = "before"
		}
	}

	// Se ainda não tem modelo, tenta obter da conversa ou usa o padrão
	if params.Model == "" {
		if conversationID > 0 {
			effectiveModel, err := a.GetEffectiveModel(conversationID)
			if err == nil && effectiveModel != "" {
				params.Model = effectiveModel
				log.Printf("[SendMessage] Usando modelo da conversa: %s", params.Model)
			}
		}
		if params.Model == "" {
			params.Model = cfg.DefaultModel
			log.Printf("[SendMessage] Usando modelo padrão: %s", params.Model)
		}
	}

	// 1. Salva mensagem do usuário no banco
	userMsg, err := database.AddMessageWithMedia(conversationID, "user", userContent, userMedia, "", "")
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

	// 4. Compõe system prompt com o perfil (se houver)
	if profileSystemPrompt != "" {
		messages = a.composeSystemPrompt(messages, profileSystemPrompt, systemPromptPosition)
	}

	// 5. Processa com LLM - userMessageID é a raiz da thread
	handler := &appStreamHandler{
		app:            a,
		conversationID: conversationID,
		userMessageID:  userMsg.ID, // Raiz da thread para interações com agentes
		filteredTools:  filteredTools,
	}
	go llm.StreamChat(a.ctx, cfg, messages, params, handler)
	return conversationID, nil
}

// composeSystemPrompt adiciona o system prompt do perfil às mensagens
func (a *App) composeSystemPrompt(messages []Message, profilePrompt string, position string) []Message {
	if profilePrompt == "" {
		return messages
	}

	// Verifica se já existe uma mensagem system
	hasSystem := false
	systemIndex := -1
	for i, msg := range messages {
		if msg.Role == "system" {
			hasSystem = true
			systemIndex = i
			break
		}
	}

	if !hasSystem {
		// Não tem system prompt, adiciona o do perfil no início
		systemMsg := Message{
			Role:    "system",
			Content: profilePrompt,
		}
		return append([]Message{systemMsg}, messages...)
	}

	// Já tem system prompt, combina baseado na posição
	existingContent := ""
	switch content := messages[systemIndex].Content.(type) {
	case string:
		existingContent = content
	default:
		// Se não é string, mantém como está
		return messages
	}

	var combinedContent string
	if position == "after" {
		combinedContent = existingContent + "\n\n" + profilePrompt
	} else {
		// "before" é o padrão
		combinedContent = profilePrompt + "\n\n" + existingContent
	}

	// Cria nova slice para não modificar a original
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
			BraveAPIKey:     input.BraveAPIKey,
			DefaultModel:    input.ChatParams.Model,
			EmbeddingsModel: input.EmbeddingsParams.Model,
			ImageModel:      input.ImageModel,
			ResponseTimeout: responseTimeout,
			ChatParams: config.ModelParams{
				Model:       input.ChatParams.Model,
				Temperature: input.ChatParams.Temperature,
				MaxTokens:   input.ChatParams.MaxTokens,
				TopP:        input.ChatParams.TopP,
			},
			EmbeddingsParams: config.EmbeddingsParams{
				Model:      input.EmbeddingsParams.Model,
				Dimensions: input.EmbeddingsParams.Dimensions,
			},
			STTParams: config.STTParams{
				Provider:      input.STTParams.Provider,
				RecordingMode: input.STTParams.RecordingMode,
			},
			ChatDefaults: config.ChatDefaults{
				UseTools:             input.ChatDefaults.UseTools,
				ShowInternalMessages: input.ChatDefaults.ShowInternalMessages,
			},
		}
	})
	if err != nil {
		return err
	}

	// Atualiza o timeout do HTTP client
	llm.ConfigureResponseTimeout(responseTimeout)

	// Reinicializa o embeddings service
	embeddingsModel := input.EmbeddingsParams.Model
	if a.embeddingsService != nil && embeddingsModel != "" {
		a.embeddingsService = llm.NewEmbeddingsService(llm.EmbeddingsConfig{
			APIKey:  input.APIKey,
			BaseURL: input.APIBaseURL,
			Model:   embeddingsModel,
		})
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

// TestEmbeddings testa se o modelo de embeddings está funcionando
func (a *App) TestEmbeddings() (string, error) {
	if a.embeddingsService == nil {
		return "", fmt.Errorf("serviço de embeddings não inicializado")
	}

	embedding, err := a.embeddingsService.Generate("teste de conexão")
	if err != nil {
		return "", fmt.Errorf("erro ao gerar embedding: %v", err)
	}

	return fmt.Sprintf("Sucesso! Embedding gerado com %d dimensões", len(embedding)), nil
}

// GetImageModel retorna o modelo configurado para processar imagens
func (a *App) GetImageModel() (string, error) {
	cfg, err := config.Load()
	if err != nil {
		return "", err
	}

	if cfg.ImageModel != "" {
		return cfg.ImageModel, nil
	}

	// Tenta encontrar um modelo com suporte a visão
	caps, err := database.GetVisionCapableModels()
	if err == nil && len(caps) > 0 {
		return caps[0].ModelName, nil
	}

	// Fallback: tenta encontrar por nome
	visionModels := []string{"gpt-4o", "gpt-4o-mini", "gpt-4-turbo", "claude-3-5-sonnet", "gemini-1.5-pro"}
	models, _ := a.GetModels()
	for _, vm := range visionModels {
		for _, m := range models {
			if strings.Contains(strings.ToLower(m), strings.ToLower(vm)) {
				return m, nil
			}
		}
	}

	return "", fmt.Errorf("nenhum modelo com suporte a visão encontrado")
}

// SetImageModel define o modelo auxiliar para processar imagens
func (a *App) SetImageModel(model string) error {
	return config.Update(func(cfg *config.Config) *config.Config {
		cfg.ImageModel = model
		return cfg
	})
}

// HasImages verifica se a lista de mensagens contém imagens
func (a *App) HasImages(messages []Message) bool {
	return llm.HasImages(messages)
}

// GenerateImageDescription gera uma descrição acessível para uma imagem
func (a *App) GenerateImageDescription(imageBase64 string, model string) (string, error) {
	cfg, err := a.GetConfig()
	if err != nil {
		return "", fmt.Errorf("erro ao obter configuração: %v", err)
	}

	return llm.GenerateImageDescription(cfg, imageBase64, model, a.GetModels)
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
