package llm

import "encoding/json"

// UsageFromOpenAICompletion normaliza usage de Chat Completions e compatíveis.
// Além do formato OpenAI (prompt_tokens_details.cached_tokens), aceita campos
// best-effort emitidos por gateways/DeepSeek no payload bruto.
func UsageFromOpenAICompletion(promptTokens, completionTokens, totalTokens, cachedTokens int, rawJSON string) Usage {
	fields := parseUsageFields(rawJSON)
	promptTokens = tokenCountWithAliases(promptTokens, fields, "prompt_tokens", "input_tokens")
	completionTokens = tokenCountWithAliases(completionTokens, fields, "completion_tokens", "output_tokens")
	totalTokens = tokenCountWithAliases(totalTokens, fields, "total_tokens")
	usage := baseUsage(promptTokens, completionTokens, totalTokens)
	usage.OutputTokensReported = openAIOutputTokensReported(fields, completionTokens, "completion_tokens", "output_tokens")
	applyOpenAICacheUsage(&usage, cachedTokens, rawJSON)
	applyOpenAIReasoningUsage(&usage, rawJSON)
	return usage
}

// UsageFromOpenAIResponses normaliza usage da Responses API.
func UsageFromOpenAIResponses(inputTokens, outputTokens, totalTokens, cachedTokens int, rawJSON string) Usage {
	fields := parseUsageFields(rawJSON)
	inputTokens = tokenCountWithAliases(inputTokens, fields, "input_tokens", "prompt_tokens")
	outputTokens = tokenCountWithAliases(outputTokens, fields, "output_tokens", "completion_tokens")
	totalTokens = tokenCountWithAliases(totalTokens, fields, "total_tokens")
	usage := baseUsage(inputTokens, outputTokens, totalTokens)
	usage.OutputTokensReported = openAIOutputTokensReported(fields, outputTokens, "output_tokens", "completion_tokens")
	applyOpenAICacheUsage(&usage, cachedTokens, rawJSON)
	applyOpenAIReasoningUsage(&usage, rawJSON)
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
	usage.OutputTokensReported = outputTokens > 0
	usage.CacheReadTokens = cacheReadTokens
	usage.CacheWriteTokens = cacheCreationTokens
	if cacheCreationTokens > 0 || cacheReadTokens > 0 {
		usage.CacheMissTokens = inputTokens
	}
	return usage
}

func mergeAnthropicStreamingUsage(previous Usage, inputTokens, outputTokens, cacheCreationTokens, cacheReadTokens int, outputReported bool) Usage {
	if outputTokens == 0 && !outputReported {
		outputTokens = previous.CompletionTokens
	}
	if cacheCreationTokens == 0 {
		cacheCreationTokens = previous.CacheWriteTokens
	}
	if cacheReadTokens == 0 {
		cacheReadTokens = previous.CacheReadTokens
	}
	if inputTokens == 0 {
		if previous.CacheMissTokens > 0 || previous.CacheWriteTokens > 0 || previous.CacheReadTokens > 0 {
			inputTokens = previous.CacheMissTokens
		} else {
			inputTokens = previous.PromptTokens
		}
	}
	usage := UsageFromAnthropic(inputTokens, outputTokens, cacheCreationTokens, cacheReadTokens)
	usage.OutputTokensReported = previous.OutputTokensReported || outputReported
	return usage
}

// UsageFromGemini normaliza usage do SDK Gemini.
func UsageFromGemini(promptTokens, completionTokens, totalTokens, cachedContentTokens int) Usage {
	return UsageFromGeminiWithReasoning(promptTokens, completionTokens, totalTokens, cachedContentTokens, 0, false)
}

// UsageFromGeminiWithReasoning preserva thoughtsTokenCount separadamente:
// CandidatesTokenCount mede a resposta visível e TotalTokenCount também pode
// incluir os tokens ocultos de raciocínio.
func UsageFromGeminiWithReasoning(promptTokens, completionTokens, totalTokens, cachedContentTokens, reasoningTokens int, reasoningReported bool) Usage {
	return usageFromGeminiWithPresence(
		promptTokens, completionTokens, totalTokens, cachedContentTokens,
		reasoningTokens, reasoningReported, completionTokens > 0,
	)
}

func usageFromGeminiWithPresence(promptTokens, completionTokens, totalTokens, cachedContentTokens, reasoningTokens int, reasoningReported, outputReported bool) Usage {
	usage := baseUsage(promptTokens, completionTokens, totalTokens)
	usage.OutputTokensReported = outputReported
	if cachedContentTokens > 0 {
		usage.CacheReadTokens = cachedContentTokens
		if promptTokens >= cachedContentTokens {
			usage.CacheMissTokens = promptTokens - cachedContentTokens
		}
	}
	usage.ReasoningTokens = reasoningTokens
	usage.ReasoningTokensReported = reasoningReported
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
		Reported:         true,
	}
}

func openAIUsageReported(rawJSON string, tokenCounts ...int) bool {
	for _, count := range tokenCounts {
		if count > 0 {
			return true
		}
	}
	fields := parseUsageFields(rawJSON)
	for _, key := range []string{
		"prompt_tokens", "completion_tokens", "total_tokens",
		"input_tokens", "output_tokens",
		"prompt_cache_hit_tokens", "cached_tokens",
		"cache_read_tokens", "cache_read_input_tokens",
		"cache_write_tokens", "cache_creation_input_tokens", "prompt_cache_write_tokens",
		"prompt_cache_miss_tokens", "cache_miss_tokens",
		"prompt_tokens_details.cached_tokens",
		"input_tokens_details.cached_tokens",
		"completion_tokens_details.reasoning_tokens",
		"output_tokens_details.reasoning_tokens",
	} {
		if _, ok := fields[key]; ok {
			return true
		}
		if _, ok := fields["usage."+key]; ok {
			return true
		}
	}
	return false
}

func openAIOutputTokensReported(fields map[string]int, outputTokens int, keys ...string) bool {
	if outputTokens > 0 {
		return true
	}
	for _, key := range keys {
		if _, ok := fields[key]; ok {
			return true
		}
		if _, ok := fields["usage."+key]; ok {
			return true
		}
	}
	return false
}

func tokenCountWithAliases(value int, fields map[string]int, keys ...string) int {
	if value != 0 {
		return value
	}
	for _, key := range keys {
		if count, ok := fields[key]; ok {
			return count
		}
		if count, ok := fields["usage."+key]; ok {
			return count
		}
	}
	return value
}

func jsonUsageHasAnyKey(rawJSON string, keys ...string) bool {
	if rawJSON == "" {
		return false
	}
	var payload any
	if err := json.Unmarshal([]byte(rawJSON), &payload); err != nil {
		return false
	}
	wanted := make(map[string]struct{}, len(keys))
	for _, key := range keys {
		wanted[key] = struct{}{}
	}
	return jsonTopLevelUsageHasAnyKey(payload, wanted)
}

func jsonTopLevelUsageHasAnyKey(value any, keys map[string]struct{}) bool {
	switch typed := value.(type) {
	case map[string]any:
		for _, usageKey := range []string{"usageMetadata", "usage_metadata"} {
			usage, ok := typed[usageKey].(map[string]any)
			if !ok {
				continue
			}
			for key := range usage {
				if _, ok := keys[key]; ok {
					return true
				}
			}
		}
	case []any:
		for _, child := range typed {
			if jsonTopLevelUsageHasAnyKey(child, keys) {
				return true
			}
		}
	}
	return false
}

func applyOpenAIReasoningUsage(usage *Usage, rawJSON string) {
	fields := parseUsageFields(rawJSON)
	for _, key := range []string{
		"completion_tokens_details.reasoning_tokens",
		"output_tokens_details.reasoning_tokens",
		"usage.completion_tokens_details.reasoning_tokens",
		"usage.output_tokens_details.reasoning_tokens",
	} {
		if value, ok := fields[key]; ok {
			usage.ReasoningTokens = value
			usage.ReasoningTokensReported = true
			return
		}
	}
}

func applyOpenAICacheUsage(usage *Usage, cachedTokens int, rawJSON string) {
	fields := parseUsageFields(rawJSON)
	readTokens := firstPositive(cachedTokens,
		fields["prompt_cache_hit_tokens"],
		fields["cached_tokens"],
		fields["prompt_tokens_details.cached_tokens"],
		fields["input_tokens_details.cached_tokens"],
		fields["usage.prompt_tokens_details.cached_tokens"],
		fields["usage.input_tokens_details.cached_tokens"],
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
			if v >= 0 {
				if prefix == "" {
					out[key] = int(v)
				}
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
