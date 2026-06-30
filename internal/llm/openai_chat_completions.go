package llm

import (
	"assistente/internal/logging"
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/packages/param"
	"github.com/openai/openai-go/shared"
)

// sendChatCompletions envia uma mensagem (não-streaming) via Chat Completions API.
func (p *OpenAIProvider) sendChatCompletions(ctx context.Context, model string, messages []Message, params ChatParams) (string, error) {
	if !params.AllowAssistantPrefill {
		messages = removeTrailingAssistantPrefill(messages)
	}
	sdkParams := openai.ChatCompletionNewParams{
		Model:    shared.ChatModel(model),
		Messages: convertMessages(messages),
	}
	if params.Temperature > 0 {
		sdkParams.Temperature = param.NewOpt(params.Temperature)
	}
	if params.MaxTokensMode == "completion_tokens" {
		sdkParams.MaxCompletionTokens = param.NewOpt(int64(params.MaxTokens))
	} else if params.MaxTokens > 0 {
		sdkParams.MaxTokens = param.NewOpt(int64(params.MaxTokens))
	}
	applyPromptCacheKeyToChatCompletions(&sdkParams, params)

	completion, err := p.client.Chat.Completions.New(ctx, sdkParams)
	if err != nil {
		if effectivePromptCacheKey(params) != "" && looksLikePromptCacheHintUnsupported(err.Error()) {
			params.PromptCacheHintFallback.Disable()
			if params.OnPromptCacheHintUnsupported != nil {
				params.OnPromptCacheHintUnsupported()
			}
			params.PromptCacheKey = ""
			return p.sendChatCompletions(ctx, model, messages, params)
		}
		return "", fmt.Errorf("erro ao enviar mensagem: %w", err)
	}
	if len(completion.Choices) == 0 {
		return "", fmt.Errorf("nenhuma resposta recebida")
	}
	return completion.Choices[0].Message.Content, nil
}

// streamChatCompletions faz streaming via Chat Completions API (path
// OpenAI-compatible legado: OpenRouter, Ollama, Groq, Together, etc.).
func (p *OpenAIProvider) streamChatCompletions(ctx context.Context, model string, messages []Message, params ChatParams, handler StreamHandler, tools ...ToolDefinition) {
	if !params.AllowAssistantPrefill {
		messages = removeTrailingAssistantPrefill(messages)
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
	applyPromptCacheKeyToChatCompletions(&sdkParams, params)

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

		done := p.doStream(ctx, sdkParams, handler, &sdkParams, params.OnPromptCacheHintUnsupported, params.PromptCacheHintFallback)
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
func (p *OpenAIProvider) doStream(ctx context.Context, params openai.ChatCompletionNewParams, handler StreamHandler, origParams *openai.ChatCompletionNewParams, onPromptCacheHintUnsupported func(), promptCacheFallback *PromptCacheHintFallback) bool {
	stream := p.client.Chat.Completions.NewStreaming(ctx, params)
	acc := openai.ChatCompletionAccumulator{}
	var promptTokensDetails openai.CompletionUsagePromptTokensDetails
	var usageRawJSON string

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
		accumulateChatCompletionStreamUsageExtras(&promptTokensDetails, chunk, &usageRawJSON)

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
		logging.Errorf(ctx, "llm.openai-chat-completions", "[OpenAIProvider] Stream error: %s", errStr)

		if !emittedAnything {
			// tool_choice downgrade
			if origParams.ToolChoice.OfAuto.Valid() && origParams.ToolChoice.OfAuto.Value == "required" {
				if strings.Contains(strings.ToLower(errStr), "tool_choice") || strings.Contains(strings.ToLower(errStr), "tool choice") {
					origParams.ToolChoice = makeToolChoice("auto")
					return false
				}
			}

			if origParams.PromptCacheKey.Valid() && looksLikePromptCacheHintUnsupported(errStr) {
				promptCacheFallback.Disable()
				if onPromptCacheHintUnsupported != nil {
					onPromptCacheHintUnsupported()
				}
				origParams.PromptCacheKey = param.Opt[string]{}
				return false
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
		cachedTokens := acc.Usage.PromptTokensDetails.CachedTokens
		if cachedTokens == 0 {
			cachedTokens = promptTokensDetails.CachedTokens
		}
		usage = UsageFromOpenAICompletion(
			int(acc.Usage.PromptTokens),
			int(acc.Usage.CompletionTokens),
			int(acc.Usage.TotalTokens),
			int(cachedTokens),
			usageRawJSON,
		)
	}

	model := acc.Model

	if len(finishedToolCalls) > 0 {
		handler.OnToolCalls(finishedToolCalls, fullResponse.String(), usage, model)
		return true
	}

	handler.OnDone(fullResponse.String(), usage, model)
	return true
}

func accumulateChatCompletionStreamUsageExtras(promptTokensDetails *openai.CompletionUsagePromptTokensDetails, chunk openai.ChatCompletionChunk, usageRawJSON *string) {
	// openai.ChatCompletionAccumulator.AddChunk currently accumulates only the
	// top-level usage counters. Preserve the detailed usage fields needed for
	// prompt-cache stats in the UI.
	if promptTokensDetails != nil {
		promptTokensDetails.AudioTokens += chunk.Usage.PromptTokensDetails.AudioTokens
		promptTokensDetails.CachedTokens += chunk.Usage.PromptTokensDetails.CachedTokens
	}

	if usageRawJSON == nil {
		return
	}
	raw := strings.TrimSpace(chunk.Usage.RawJSON())
	if raw == "" || raw == "null" {
		return
	}
	*usageRawJSON = raw
}

func makeToolChoice(choice string) openai.ChatCompletionToolChoiceOptionUnionParam {
	return openai.ChatCompletionToolChoiceOptionUnionParam{
		OfAuto: param.NewOpt(choice),
	}
}

func applyPromptCacheKeyToChatCompletions(sdkParams *openai.ChatCompletionNewParams, params ChatParams) {
	key := effectivePromptCacheKey(params)
	if sdkParams == nil || key == "" {
		return
	}
	sdkParams.PromptCacheKey = param.NewOpt(key)
}
