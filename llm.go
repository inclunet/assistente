package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

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
}

func (h *appStreamHandler) OnChunk(content string) {
	// Acumula o conteúdo
	h.accumulatedContent += content

	// Emite evento de streaming (sem messageId fixo - será criado no OnDone)
	runtime.EventsEmit(h.app.ctx, "chat:stream", StreamEvent{
		Content: h.accumulatedContent,
		Done:    false,
	})
}

func (h *appStreamHandler) OnError(err string) {
	runtime.EventsEmit(h.app.ctx, "chat:stream", StreamEvent{
		Content: h.accumulatedContent,
		Done:    true,
		Error:   err,
	})
}

func (h *appStreamHandler) OnDone(fullResponse string, usage llm.Usage, model string) {
	// Usa o conteúdo acumulado ou o fullResponse
	finalContent := fullResponse
	if finalContent == "" {
		finalContent = h.accumulatedContent
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
			fmt.Printf("❌ Erro ao salvar resposta do assistant: %v\n", err)
		} else {
			fmt.Printf("✅ Resposta do assistant salva: ID=%d (nível 0)\n", msg.ID)
		}
	}

	// Emite evento final de streaming
	runtime.EventsEmit(h.app.ctx, "chat:stream", StreamEvent{
		Content: finalContent,
		Done:    true,
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

		// Extrai nome do agente e a tarefa
		agentName := ""
		taskContent := ""
		allArgs := ""
		if len(toolCalls) > 0 {
			tc := toolCalls[0]
			name := tc.Function.Name
			if strings.HasPrefix(name, "delegate_to_") {
				agentName = strings.TrimPrefix(name, "delegate_to_")
			} else {
				agentName = name
			}

			// Guarda argumentos completos para debug
			allArgs = tc.Function.Arguments

			// Extrai a tarefa dos argumentos
			var args map[string]interface{}
			if err := json.Unmarshal([]byte(tc.Function.Arguments), &args); err == nil {
				if task, ok := args["task"].(string); ok {
					taskContent = task
				}
			}
		}

		// Conteúdo é a tarefa que o assistente está delegando
		// Se não houver task explícito, mostra os argumentos completos
		content := taskContent
		if content == "" && allArgs != "" {
			// Formata JSON para legibilidade
			var prettyArgs bytes.Buffer
			if err := json.Indent(&prettyArgs, []byte(allArgs), "", "  "); err == nil {
				content = fmt.Sprintf("Chamando %s:\n```json\n%s\n```", agentName, prettyArgs.String())
			} else {
				content = fmt.Sprintf("Chamando %s: %s", agentName, allArgs)
			}
		} else if content == "" {
			content = fmt.Sprintf("Solicitando ajuda de %s", agentName)
		}

		// Salva como filho da mensagem do usuário (nível 1)
		// role="assistant" porque é o assistente falando com o agente
		msg, err := database.AddChildMessage(
			h.conversationID,
			h.userMessageID, // ParentID = mensagem do usuário
			"assistant",
			content,
			string(toolCallsJSON),
			"",
			agentName, // agentName indica qual agente está sendo chamado
			model,
		)
		if err != nil {
			fmt.Printf("❌ Erro ao salvar delegação: %v\n", err)
		} else {
			fmt.Printf("✅ Delegação salva: ID=%d, parentID=%d, agente=%s (nível 1)\n", msg.ID, h.userMessageID, agentName)
			// Define no App para os agentes usarem como ParentID (nível 2)
			h.app.currentDelegationID = msg.ID

			// Emite evento para o frontend mostrar mensagem interna em tempo real
			runtime.EventsEmit(h.app.ctx, "chat:internal_message", map[string]interface{}{
				"id":        msg.ID,
				"parentId":  h.userMessageID,
				"role":      "assistant",
				"content":   content,
				"agentName": agentName,
				"toolCalls": string(toolCallsJSON),
				"internal":  true,
			})
		}

		// Emite evento de streaming
		runtime.EventsEmit(h.app.ctx, "chat:stream", StreamEvent{
			Content: fmt.Sprintf("🤖 Consultando %s...", agentName),
			Done:    false,
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

// SendMessage envia uma mensagem para a API com streaming
// NOVA ARQUITETURA: Backend gerencia todo o estado
// 1. Cria mensagens no banco ANTES de streamar
// 2. Emite evento único com IDs das mensagens criadas
// 3. Streaming emite eventos com messageId
func (a *App) SendMessage(conversationID uint, userContent string, userMedia string, params ChatParams) error {
	cfg, err := config.Load()
	if err != nil {
		runtime.EventsEmit(a.ctx, "chat:error", "Erro ao carregar configuração: "+err.Error())
		return err
	}

	if cfg.APIKey == "" {
		runtime.EventsEmit(a.ctx, "chat:error", "API Key não configurada. Por favor, configure sua API Key nas configurações.")
		return fmt.Errorf("API Key não configurada")
	}

	// Se não tem conversationID, cria uma nova conversa
	if conversationID == 0 {
		title := userContent
		if len(title) > 50 {
			title = title[:50]
		}
		conv, err := database.CreateConversation(title, params.Model)
		if err != nil {
			runtime.EventsEmit(a.ctx, "chat:error", "Erro ao criar conversa: "+err.Error())
			return err
		}
		conversationID = conv.ID
		// Emite evento com ID da nova conversa
		runtime.EventsEmit(a.ctx, "chat:conversation_created", map[string]interface{}{
			"id":    conversationID,
			"title": title,
		})
	}

	// 1. Salva mensagem do usuário no banco
	userMsg, err := database.AddMessageWithMedia(conversationID, "user", userContent, userMedia, "", "")
	if err != nil {
		runtime.EventsEmit(a.ctx, "chat:error", "Erro ao salvar mensagem: "+err.Error())
		return err
	}
	fmt.Printf("✅ Mensagem do usuário salva: ID=%d\n", userMsg.ID)

	// 2. Emite evento informando que mensagem do usuário foi criada
	runtime.EventsEmit(a.ctx, "chat:messages_ready", map[string]interface{}{
		"conversationId": conversationID,
		"userMessageId":  userMsg.ID,
	})

	// 3. Carrega histórico da conversa para contexto
	messages, err := a.loadConversationHistory(conversationID)
	if err != nil {
		runtime.EventsEmit(a.ctx, "chat:error", "Erro ao carregar histórico: "+err.Error())
		return err
	}

	// 4. Processa com LLM - userMessageID é a raiz da thread
	handler := &appStreamHandler{
		app:            a,
		conversationID: conversationID,
		userMessageID:  userMsg.ID, // Raiz da thread para interações com agentes
	}
	go llm.StreamChat(a.ctx, cfg, messages, params, handler)
	return nil
}

// loadConversationHistory carrega o histórico de mensagens de uma conversa
// NOVA ARQUITETURA v2: Apenas mensagens de nível 0 vão para a API
// As interações com agentes ficam em threads (parentID != null)
func (a *App) loadConversationHistory(conversationID uint) ([]Message, error) {
	conv, err := database.GetConversation(conversationID)
	if err != nil {
		return nil, err
	}

	fmt.Printf("📋 [HISTORY] Carregando histórico da conversa %d (%d mensagens total)\n", conversationID, len(conv.Messages))

	var messages []Message
	for i, m := range conv.Messages {
		fmt.Printf("📋 [HISTORY] Msg %d: role=%s, parentID=%v, content=%s\n",
			i, m.Role, m.ParentID, truncateStr(m.Content, 50))

		// Apenas mensagens de nível 0 (sem parentID) vão para a API
		if m.ParentID != nil {
			fmt.Printf("📋 [HISTORY]   -> IGNORADA (nível %d, parentID=%d)\n", 1, *m.ParentID)
			continue
		}
		fmt.Printf("📋 [HISTORY]   -> INCLUÍDA (nível 0)\n")

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
				for _, mp := range mediaParts {
					content = append(content, mp)
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

// SaveSettings salva as configurações
func (a *App) SaveSettings(input SettingsInput) error {
	err := config.Update(func(existing *config.Config) *config.Config {
		return &config.Config{
			APIKey:          input.APIKey,
			APIBaseURL:      input.APIBaseURL,
			DefaultModel:    input.ChatParams.Model,
			EmbeddingsModel: input.EmbeddingsParams.Model,
			ImageModel:      input.ImageModel,
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
			VoiceParams: config.VoiceParams{
				Voice:     input.VoiceParams.Voice,
				AutoSpeak: input.VoiceParams.AutoSpeak,
				Volume:    input.VoiceParams.Volume,
				Rate:      input.VoiceParams.Rate,
			},
			STTParams: config.STTParams{
				Provider:      input.STTParams.Provider,
				RecordingMode: input.STTParams.RecordingMode,
			},
			ChatDefaults: config.ChatDefaults{
				UseTools:             input.ChatDefaults.UseTools,
				ShowInternalMessages: input.ChatDefaults.ShowInternalMessages,
			},
			LastConversationID: existing.LastConversationID,
		}
	})
	if err != nil {
		return err
	}

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

// SetLastConversation salva a última conversa aberta
func (a *App) SetLastConversation(conversationID uint) error {
	return config.Update(func(cfg *config.Config) *config.Config {
		cfg.LastConversationID = conversationID
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
	return database.Init()
}
