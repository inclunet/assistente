// Package llmcompat contém normalizações puras compartilhadas pelo runtime e
// pela persistência para identificar a configuração efetiva de um provedor.
package llmcompat

import "strings"

const (
	APIFormatOpenAI          = "openai"
	APIFormatOpenAIResponses = "openai_responses"
	ProviderTypeLocalAI      = "localai"
	ProviderTypeOllama       = "ollama"
	ProviderTypeLlamaCPP     = "llamacpp"
	ReasoningDisabled        = "disabled"
	ReasoningReplayWithTools = "replay_with_tools"
)

// EffectiveAPIFormat aplica os defaults e as restrições de formato do runtime.
func EffectiveAPIFormat(providerType, apiFormat, baseURL string) string {
	if apiFormat != "" {
		if isOpenAICompatibleProvider(providerType) && apiFormat == APIFormatOpenAIResponses {
			return APIFormatOpenAI
		}
		return apiFormat
	}
	if isOpenAICompatibleProvider(providerType) {
		return APIFormatOpenAI
	}
	normalizedURL := strings.ToLower(strings.TrimSuffix(baseURL, "/"))
	if strings.Contains(normalizedURL, "api.openai.com") {
		return APIFormatOpenAIResponses
	}
	return APIFormatOpenAI
}

// EffectiveReasoningContentMode preserva somente o modo explicitamente aceito.
func EffectiveReasoningContentMode(mode string) string {
	if mode == ReasoningReplayWithTools {
		return ReasoningReplayWithTools
	}
	return ReasoningDisabled
}

func isOpenAICompatibleProvider(providerType string) bool {
	switch providerType {
	case ProviderTypeLocalAI, ProviderTypeOllama, ProviderTypeLlamaCPP:
		return true
	default:
		return false
	}
}
