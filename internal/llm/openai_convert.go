package llm

import (
	"assistente/internal/logging"
	"context"
	"encoding/json"
	"strings"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/packages/param"
	"github.com/openai/openai-go/responses"
	"github.com/openai/openai-go/shared"
)

// convertMessages converte nossas mensagens internas para o formato SDK.
func convertMessages(msgs []Message) []openai.ChatCompletionMessageParamUnion {
	return convertMessagesWithReasoningContent(msgs, false)
}

func convertMessagesWithReasoningContent(msgs []Message, includeReasoningContent bool) []openai.ChatCompletionMessageParamUnion {
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
			// O histórico preserva a assistant sem conteúdo nem tool_calls
			// quando ela carrega reasoning e a capability exige esse replay.
			// Para quem não recebe o campo ela viraria uma assistant vazia, que
			// parte dos providers OpenAI-compatible recusa com 400.
			replayingReasoning := includeReasoningContent && msg.ReasoningContent != ""
			if strings.TrimSpace(content) == "" && len(msg.ToolCalls) == 0 && !replayingReasoning {
				continue
			}
			m := openai.AssistantMessage(content)
			if replayingReasoning {
				// reasoning_content é uma extensão OpenAI-compatible, ausente
				// do tipo gerado pelo SDK. No modo replay_with_tools,
				// omiti-la invalida a continuação da chamada de ferramenta.
				m.OfAssistant.SetExtraFields(map[string]any{
					"reasoning_content": msg.ReasoningContent,
				})
			}
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
				logging.Errorf(context.Background(), "llm.openai-convert", "[OpenAIProvider] Erro ao parsear parameters de %s: %v", tool.Function.Name, err)
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

// convertToResponsesInput converte mensagens internas para o formato Responses API.
//
// Diferenças conhecidas vs convertMessages (Chat Completions):
//   - Imagens: user messages com image_url são convertidas apenas como texto (GetContentAsString).
//     A Responses API suporta imagens via input_image, mas com formato diferente.
//   - Assistant com content + tool_calls: gera items separados (message + function_call),
//     que é o formato correto para a Responses API (items são independentes).
//   - Tool results: mapeados para function_call_output (equivalente funcional).
func convertToResponsesInput(msgs []Message) responses.ResponseInputParam {
	var items []responses.ResponseInputItemUnionParam

	for _, msg := range msgs {
		content := msg.GetContentAsString()

		switch msg.Role {
		case "system":
			items = append(items, responses.ResponseInputItemParamOfMessage(
				content, responses.EasyInputMessageRoleSystem,
			))

		case "user":
			items = append(items, responses.ResponseInputItemParamOfMessage(
				content, responses.EasyInputMessageRoleUser,
			))

		case "assistant":
			if content != "" {
				items = append(items, responses.ResponseInputItemParamOfMessage(
					content, responses.EasyInputMessageRoleAssistant,
				))
			}
			for _, tc := range msg.ToolCalls {
				items = append(items, responses.ResponseInputItemParamOfFunctionCall(
					tc.Function.Arguments, tc.ID, tc.Function.Name,
				))
			}

		case "tool":
			items = append(items, responses.ResponseInputItemParamOfFunctionCallOutput(
				msg.ToolCallID, content,
			))
		}
	}

	return items
}

// removeTrailingAssistantPrefill remove mensagens assistant no fim da lista
// (prefill), preservando o slice original quando não há nada a remover.
func removeTrailingAssistantPrefill(messages []Message) []Message {
	end := len(messages)
	for end > 0 && messages[end-1].Role == "assistant" {
		end--
	}
	if end == len(messages) {
		return messages
	}
	return append([]Message(nil), messages[:end]...)
}

// effectivePromptCacheKey retorna a prompt cache key efetiva, considerando o
// fallback que pode tê-la desativado neste turno.
func effectivePromptCacheKey(params ChatParams) string {
	if params.PromptCacheHintFallback != nil && params.PromptCacheHintFallback.Disabled() {
		return ""
	}
	return params.PromptCacheKey
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

func looksLikePromptCacheHintUnsupported(errStr string) bool {
	lower := strings.ToLower(errStr)
	hasPromptCacheKey := strings.Contains(lower, "prompt_cache_key") ||
		strings.Contains(lower, "prompt cache key") ||
		strings.Contains(lower, "prompt-cache-key")
	if !hasPromptCacheKey {
		return false
	}
	return strings.Contains(lower, "unsupported parameter") ||
		strings.Contains(lower, "unsupported field") ||
		strings.Contains(lower, "unknown parameter") ||
		strings.Contains(lower, "unknown field") ||
		strings.Contains(lower, "unknown argument") ||
		strings.Contains(lower, "unrecognized parameter") ||
		strings.Contains(lower, "unrecognized field") ||
		strings.Contains(lower, "unrecognized argument") ||
		strings.Contains(lower, "unrecognized request argument") ||
		strings.Contains(lower, "unrecognised parameter") ||
		strings.Contains(lower, "unrecognised field") ||
		strings.Contains(lower, "unrecognised argument") ||
		strings.Contains(lower, "unrecognised request argument") ||
		strings.Contains(lower, "invalid parameter") ||
		strings.Contains(lower, "invalid field") ||
		strings.Contains(lower, "extra_forbidden") ||
		strings.Contains(lower, "extra inputs") ||
		strings.Contains(lower, "not permitted") ||
		strings.Contains(lower, "not allowed") ||
		strings.Contains(lower, "not supported") ||
		strings.Contains(lower, "does not support")
}
