package llm

import (
	"context"
	"encoding/json"
	"log"
	"strings"
	"time"

	"assistente/internal/credentials"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
	"github.com/openai/openai-go/packages/param"
	"github.com/openai/openai-go/shared"
)

// OpenAIProvider implementa ChatProvider usando a SDK openai-go.
// Cobre OpenAI nativo e qualquer endpoint OpenAI-compatible (OpenRouter, Ollama, etc).
type OpenAIProvider struct {
	client   *openai.Client
	provider *ProviderConfig
}

// NewOpenAIProvider cria um provider OpenAI-compatible com a SDK oficial.
func NewOpenAIProvider(provider *ProviderConfig, credMgr *credentials.Manager) *OpenAIProvider {
	httpClient := newHTTPClientForProvider(provider, credMgr)

	opts := []option.RequestOption{
		option.WithHTTPClient(httpClient),
		option.WithAPIKey("managed-by-credential-transport"),
	}

	baseURL := strings.TrimSuffix(provider.BaseURL, "/")
	if !strings.HasSuffix(baseURL, "/v1") && !strings.HasSuffix(baseURL, "/v1beta") {
		baseURL += "/"
	} else {
		baseURL += "/"
	}
	opts = append(opts, option.WithBaseURL(baseURL))

	client := openai.NewClient(opts...)

	return &OpenAIProvider{
		client:   &client,
		provider: provider,
	}
}

func (p *OpenAIProvider) SupportsNativeMCP() bool {
	return true
}

func (p *OpenAIProvider) WithMCPServers(_ []MCPServerConfig) ChatProvider {
	// TODO: Fase 4 — Responses API com type:"mcp"
	return p
}

func (p *OpenAIProvider) StreamChat(ctx context.Context, messages []Message, params ChatParams, handler StreamHandler, tools ...ToolDefinition) {
	model := params.Model
	if model == "" {
		if p.provider.Model != "" {
			model = p.provider.Model
		} else if p.provider.DefaultModel != "" {
			model = p.provider.DefaultModel
		}
		if model == "" {
			handler.OnError("Nenhum modelo especificado e nenhum modelo padrão configurado")
			return
		}
	}

	sdkParams := openai.ChatCompletionNewParams{
		Model:    shared.ChatModel(model),
		Messages: convertMessages(messages),
		StreamOptions: openai.ChatCompletionStreamOptionsParam{
			IncludeUsage: param.NewOpt(true),
		},
	}

	if params.Temperature > 0 {
		sdkParams.Temperature = param.NewOpt(params.Temperature)
	}

	if params.MaxTokensMode == "completion_tokens" {
		sdkParams.MaxCompletionTokens = param.NewOpt(int64(params.MaxTokens))
	} else if params.MaxTokens > 0 {
		sdkParams.MaxTokens = param.NewOpt(int64(params.MaxTokens))
	}

	if params.TopP > 0 && params.TopP != 1.0 {
		sdkParams.TopP = param.NewOpt(params.TopP)
	}

	switch params.ReasoningEffort {
	case "low", "medium", "high":
		sdkParams.ReasoningEffort = shared.ReasoningEffort(params.ReasoningEffort)
	}

	if len(tools) > 0 {
		sdkParams.Tools = convertTools(tools)
		toolChoice := "auto"
		if choice, ok := toolChoiceFromContext(ctx); ok {
			if s, ok := choice.(string); ok {
				toolChoice = s
			}
		}
		sdkParams.ToolChoice = makeToolChoice(toolChoice)
	}

	const maxAttempts = 10
	bk := 500 * time.Millisecond
	maxBk := 8 * time.Second

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		select {
		case <-ctx.Done():
			handler.OnError("Streaming cancelado: " + ctx.Err().Error())
			return
		default:
		}

		done := p.doStream(ctx, sdkParams, handler, &sdkParams)
		if done {
			return
		}

		if attempt < maxAttempts {
			sleepWithJitter(ctx, bk)
			bk = nextBackoff(bk, maxBk)
			continue
		}

		handler.OnError("Máximo de tentativas de streaming excedido")
	}
}

// doStream executa uma tentativa de streaming. Retorna true se concluiu (sucesso ou erro terminal).
func (p *OpenAIProvider) doStream(ctx context.Context, params openai.ChatCompletionNewParams, handler StreamHandler, origParams *openai.ChatCompletionNewParams) bool {
	stream := p.client.Chat.Completions.NewStreaming(ctx, params)
	acc := openai.ChatCompletionAccumulator{}

	var fullResponse strings.Builder
	var fullReasoning strings.Builder
	var isThinking bool
	var thinkingBuffer strings.Builder
	var emittedAnything bool

	// Coletar tool calls finalizadas durante streaming
	var finishedToolCalls []ToolCall

	for stream.Next() {
		chunk := stream.Current()
		acc.AddChunk(chunk)

		if tool, ok := acc.JustFinishedToolCall(); ok {
			finishedToolCalls = append(finishedToolCalls, ToolCall{
				ID:   tool.ID,
				Type: "function",
				Function: FunctionCall{
					Name:      tool.Name,
					Arguments: tool.Arguments,
				},
			})
		}

		if len(chunk.Choices) == 0 {
			continue
		}

		delta := chunk.Choices[0].Delta

		if delta.Content != "" {
			content := delta.Content

			content = processThinkingTags(content, &isThinking, &thinkingBuffer, &fullReasoning, handler)

			if content != "" {
				fullResponse.WriteString(content)
				emittedAnything = true
				handler.OnChunk(content)
			}
		}
	}

	if err := stream.Err(); err != nil {
		errStr := err.Error()
		log.Printf("[OpenAIProvider] Stream error: %s", errStr)

		if !emittedAnything {
			// tool_choice downgrade
			if origParams.ToolChoice.OfAuto.Valid() && origParams.ToolChoice.OfAuto.Value == "required" {
				if strings.Contains(strings.ToLower(errStr), "tool_choice") || strings.Contains(strings.ToLower(errStr), "tool choice") {
					origParams.ToolChoice = makeToolChoice("auto")
					return false
				}
			}

			if isRetryableError(errStr) {
				return false
			}
		}

		handler.OnError(errStr)
		return true
	}

	if fullReasoning.Len() > 0 {
		handler.OnThinkingDone(fullReasoning.String())
	}

	usage := Usage{}
	if acc.Usage.TotalTokens > 0 {
		usage = Usage{
			PromptTokens:     int(acc.Usage.PromptTokens),
			CompletionTokens: int(acc.Usage.CompletionTokens),
			TotalTokens:      int(acc.Usage.TotalTokens),
		}
	}

	model := acc.Model

	if len(finishedToolCalls) > 0 {
		handler.OnToolCalls(finishedToolCalls, fullResponse.String(), usage, model)
		return true
	}

	handler.OnDone(fullResponse.String(), usage, model)
	return true
}

func isRetryableError(errStr string) bool {
	lower := strings.ToLower(errStr)
	return strings.Contains(lower, "timeout") ||
		strings.Contains(lower, "connection refused") ||
		strings.Contains(lower, "connection reset") ||
		strings.Contains(lower, "524") ||
		strings.Contains(lower, "502") ||
		strings.Contains(lower, "503") ||
		strings.Contains(lower, "429")
}

// convertMessages converte nossas mensagens internas para o formato SDK.
func convertMessages(msgs []Message) []openai.ChatCompletionMessageParamUnion {
	result := make([]openai.ChatCompletionMessageParamUnion, 0, len(msgs))

	for _, msg := range msgs {
		content := msg.GetContentAsString()

		switch msg.Role {
		case "system":
			result = append(result, openai.SystemMessage(content))

		case "user":
			if parts := extractImageParts(msg); parts != nil {
				result = append(result, openai.UserMessage(parts))
			} else {
				result = append(result, openai.UserMessage(content))
			}

		case "assistant":
			m := openai.AssistantMessage(content)
			if len(msg.ToolCalls) > 0 {
				toolCalls := make([]openai.ChatCompletionMessageToolCallParam, 0, len(msg.ToolCalls))
				for _, tc := range msg.ToolCalls {
					toolCalls = append(toolCalls, openai.ChatCompletionMessageToolCallParam{
						ID: tc.ID,
						Function: openai.ChatCompletionMessageToolCallFunctionParam{
							Name:      tc.Function.Name,
							Arguments: tc.Function.Arguments,
						},
					})
				}
				m.OfAssistant.ToolCalls = toolCalls
			}
			result = append(result, m)

		case "tool":
			result = append(result, openai.ToolMessage(content, msg.ToolCallID))
		}
	}

	return result
}

// extractImageParts extrai partes multimodal (texto + imagens) de uma mensagem user.
// Retorna nil se não houver imagens.
func extractImageParts(msg Message) []openai.ChatCompletionContentPartUnionParam {
	rawParts, ok := msg.Content.([]interface{})
	if !ok {
		return nil
	}

	hasImage := false
	for _, part := range rawParts {
		if partMap, ok := part.(map[string]interface{}); ok {
			if partMap["type"] == "image_url" {
				hasImage = true
				break
			}
		}
	}
	if !hasImage {
		return nil
	}

	var parts []openai.ChatCompletionContentPartUnionParam
	for _, part := range rawParts {
		partMap, ok := part.(map[string]interface{})
		if !ok {
			continue
		}

		switch partMap["type"] {
		case "text":
			if text, ok := partMap["text"].(string); ok {
				parts = append(parts, openai.TextContentPart(text))
			}
		case "image_url":
			if imgURLObj, ok := partMap["image_url"].(map[string]interface{}); ok {
				if urlStr, ok := imgURLObj["url"].(string); ok {
					imgParam := openai.ChatCompletionContentPartImageImageURLParam{
						URL:    urlStr,
						Detail: "auto",
					}
					if d, ok := imgURLObj["detail"].(string); ok && d != "" {
						imgParam.Detail = d
					}
					parts = append(parts, openai.ImageContentPart(imgParam))
				}
			}
		}
	}

	return parts
}

// convertTools converte nossas definições de ferramentas para o formato SDK.
func convertTools(tools []ToolDefinition) []openai.ChatCompletionToolParam {
	result := make([]openai.ChatCompletionToolParam, 0, len(tools))

	for _, tool := range tools {
		var params shared.FunctionParameters
		if len(tool.Function.Parameters) > 0 {
			if err := json.Unmarshal(tool.Function.Parameters, &params); err != nil {
				log.Printf("[OpenAIProvider] Erro ao parsear parameters de %s: %v", tool.Function.Name, err)
				continue
			}
		}

		result = append(result, openai.ChatCompletionToolParam{
			Function: shared.FunctionDefinitionParam{
				Name:        tool.Function.Name,
				Description: param.NewOpt(tool.Function.Description),
				Parameters:  params,
			},
		})
	}

	return result
}

func makeToolChoice(choice string) openai.ChatCompletionToolChoiceOptionUnionParam {
	return openai.ChatCompletionToolChoiceOptionUnionParam{
		OfAuto: param.NewOpt(choice),
	}
}

var _ ChatProvider = (*OpenAIProvider)(nil)
