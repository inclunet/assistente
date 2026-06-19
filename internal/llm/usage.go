package llm

import "encoding/json"

// UsageFromOpenAICompletion normaliza usage de Chat Completions e compatíveis.
// Além do formato OpenAI (prompt_tokens_details.cached_tokens), aceita campos
// best-effort emitidos por gateways/DeepSeek no payload bruto.
func UsageFromOpenAICompletion(promptTokens, completionTokens, totalTokens, cachedTokens int, rawJSON string) Usage {
	usage := baseUsage(promptTokens, completionTokens, totalTokens)
	applyOpenAICacheUsage(&usage, cachedTokens, rawJSON)
	return usage
}

// UsageFromOpenAIResponses normaliza usage da Responses API.
func UsageFromOpenAIResponses(inputTokens, outputTokens, totalTokens, cachedTokens int, rawJSON string) Usage {
	usage := baseUsage(inputTokens, outputTokens, totalTokens)
	applyOpenAICacheUsage(&usage, cachedTokens, rawJSON)
	return usage
}

// UsageFromAnthropic normaliza usage Anthropic. Segundo o SDK, input_tokens,
// cache_creation_input_tokens e cache_read_input_tokens compõem o total de input.
func UsageFromAnthropic(inputTokens, outputTokens, cacheCreationTokens, cacheReadTokens int) Usage {
	promptTokens := inputTokens
	if cacheCreationTokens > 0 || cacheReadTokens > 0 {
		promptTokens = inputTokens + cacheCreationTokens + cacheReadTokens
	}
	usage := baseUsage(promptTokens, outputTokens, 0)
	usage.CacheReadTokens = cacheReadTokens
	usage.CacheWriteTokens = cacheCreationTokens
	if cacheCreationTokens > 0 || cacheReadTokens > 0 {
		usage.CacheMissTokens = inputTokens
	}
	return usage
}

// UsageFromGemini normaliza usage do SDK Gemini.
func UsageFromGemini(promptTokens, completionTokens, totalTokens, cachedContentTokens int) Usage {
	usage := baseUsage(promptTokens, completionTokens, totalTokens)
	if cachedContentTokens > 0 {
		usage.CacheReadTokens = cachedContentTokens
		if promptTokens >= cachedContentTokens {
			usage.CacheMissTokens = promptTokens - cachedContentTokens
		}
	}
	return usage
}

func baseUsage(promptTokens, completionTokens, totalTokens int) Usage {
	if totalTokens == 0 && (promptTokens > 0 || completionTokens > 0) {
		totalTokens = promptTokens + completionTokens
	}
	return Usage{
		PromptTokens:     promptTokens,
		CompletionTokens: completionTokens,
		TotalTokens:      totalTokens,
	}
}

func applyOpenAICacheUsage(usage *Usage, cachedTokens int, rawJSON string) {
	fields := parseUsageFields(rawJSON)
	readTokens := firstPositive(cachedTokens,
		fields["prompt_cache_hit_tokens"],
		fields["cached_tokens"],
		fields["cache_read_tokens"],
		fields["cache_read_input_tokens"],
	)
	writeTokens := firstPositive(
		fields["cache_write_tokens"],
		fields["cache_creation_input_tokens"],
		fields["prompt_cache_write_tokens"],
	)
	missTokens := firstPositive(
		fields["prompt_cache_miss_tokens"],
		fields["cache_miss_tokens"],
	)
	if missTokens == 0 && readTokens > 0 && usage.PromptTokens >= readTokens {
		missTokens = usage.PromptTokens - readTokens
	}
	usage.CacheReadTokens = readTokens
	usage.CacheWriteTokens = writeTokens
	usage.CacheMissTokens = missTokens
}

func parseUsageFields(rawJSON string) map[string]int {
	if rawJSON == "" {
		return map[string]int{}
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(rawJSON), &payload); err != nil {
		return map[string]int{}
	}
	fields := make(map[string]int)
	flattenUsageFields("", payload, fields)
	return fields
}

func flattenUsageFields(prefix string, in map[string]any, out map[string]int) {
	for key, value := range in {
		fullKey := key
		if prefix != "" {
			fullKey = prefix + "." + key
		}
		switch v := value.(type) {
		case float64:
			if v > 0 {
				out[key] = int(v)
				out[fullKey] = int(v)
			}
		case map[string]any:
			flattenUsageFields(fullKey, v, out)
		}
	}
}

func firstPositive(values ...int) int {
	for _, value := range values {
		if value > 0 {
			return value
		}
	}
	return 0
}
