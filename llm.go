package main

import (
	"fmt"
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
type appStreamHandler struct {
	app *App
}

func (h *appStreamHandler) OnChunk(content string) {
	runtime.EventsEmit(h.app.ctx, "chat:chunk", llm.StreamChunk{
		Content: content,
		Done:    false,
	})
}

func (h *appStreamHandler) OnError(err string) {
	runtime.EventsEmit(h.app.ctx, "chat:error", err)
}

func (h *appStreamHandler) OnDone(fullResponse string, usage llm.Usage, model string) {
	runtime.EventsEmit(h.app.ctx, "chat:chunk", llm.StreamChunk{
		Done:             true,
		FullResponse:     fullResponse,
		PromptTokens:     usage.PromptTokens,
		CompletionTokens: usage.CompletionTokens,
		TotalTokens:      usage.TotalTokens,
		Model:            model,
	})
}

func (h *appStreamHandler) OnToolCalls(toolCalls []llm.ToolCall, usage llm.Usage, model string) {
	// Handled internally by processToolCalls
}

func (h *appStreamHandler) OnToolsExecuting(toolNames []string) {
	runtime.EventsEmit(h.app.ctx, "chat:tools", map[string]interface{}{
		"tools":   toolNames,
		"message": fmt.Sprintf("Executando %d ferramenta(s): %s", len(toolNames), strings.Join(toolNames, ", ")),
	})
}

func (h *appStreamHandler) OnToolResults(results []string, usage llm.Usage, model string) {
	runtime.EventsEmit(h.app.ctx, "chat:tool_results", map[string]interface{}{
		"results": results,
		"usage": map[string]int{
			"promptTokens":     usage.PromptTokens,
			"completionTokens": usage.CompletionTokens,
			"totalTokens":      usage.TotalTokens,
		},
		"model": model,
	})
}

func (h *appStreamHandler) GetTools() []llm.Tool {
	return h.app.GetToolsForAPI()
}

func (h *appStreamHandler) ExecuteTool(tc llm.ToolCall) (string, error) {
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
func (a *App) SendMessage(messages []Message, params ChatParams) error {
	cfg, err := config.Load()
	if err != nil {
		runtime.EventsEmit(a.ctx, "chat:error", "Erro ao carregar configuração: "+err.Error())
		return err
	}

	if cfg.APIKey == "" {
		runtime.EventsEmit(a.ctx, "chat:error", "API Key não configurada. Por favor, configure sua API Key nas configurações.")
		return fmt.Errorf("API Key não configurada")
	}

	handler := &appStreamHandler{app: a}
	go llm.StreamChat(a.ctx, cfg, messages, params, handler)
	return nil
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
			},
			EmbeddingsParams: config.EmbeddingsParams{
				Model: input.EmbeddingsParams.Model,
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
