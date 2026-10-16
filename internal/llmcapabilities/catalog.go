package llmcapabilities

// Package llmcapabilities valida o vocabulário fechado de operações e campos
// canônicos usados pelo estado de compatibilidade dos modelos.
type Capability string
type FieldKey string

const (
	CapabilityChatCompletions Capability = "chat.completions"
	CapabilityResponses       Capability = "responses"
	CapabilityTextToSpeech    Capability = "tts"
	CapabilitySpeechToText    Capability = "stt"
)

const (
	FieldMaxOutputTokens    FieldKey = "max_output_tokens"
	FieldTemperature        FieldKey = "temperature"
	FieldTopP               FieldKey = "top_p"
	FieldFrequencyPenalty   FieldKey = "frequency_penalty"
	FieldPresencePenalty    FieldKey = "presence_penalty"
	FieldSeed               FieldKey = "seed"
	FieldReasoningEffort    FieldKey = "reasoning_effort"
	FieldMaxReasoningTokens FieldKey = "max_reasoning_tokens"
	FieldParallelToolCalls  FieldKey = "parallel_tool_calls"
	FieldVoice              FieldKey = "voice"
	FieldSpeed              FieldKey = "speed"
	FieldAudioFormat        FieldKey = "audio_format"
	FieldLanguage           FieldKey = "language"
)

// fields associa os campos canônicos a contextos de operação. Cada chave é
// validada antes de chegar à persistência; novas operações exigem uma decisão
// explícita neste vocabulário.
var fields = map[Capability]map[FieldKey]struct{}{
	CapabilityChatCompletions: {
		FieldMaxOutputTokens: {}, FieldTemperature: {}, FieldTopP: {},
		FieldFrequencyPenalty: {}, FieldPresencePenalty: {}, FieldSeed: {},
		FieldReasoningEffort: {}, FieldMaxReasoningTokens: {}, FieldParallelToolCalls: {},
	},
	CapabilityResponses: {
		FieldMaxOutputTokens: {}, FieldTemperature: {}, FieldTopP: {},
		FieldReasoningEffort: {}, FieldMaxReasoningTokens: {}, FieldParallelToolCalls: {},
	},
	CapabilityTextToSpeech: {
		FieldVoice: {}, FieldSpeed: {}, FieldAudioFormat: {},
	},
	CapabilitySpeechToText: {
		FieldLanguage: {}, FieldAudioFormat: {},
	},
}

func HasCapability(capability Capability) bool {
	_, ok := fields[capability]
	return ok
}

func HasField(capability Capability, field FieldKey) bool {
	known, ok := fields[capability]
	if !ok {
		return false
	}
	_, ok = known[field]
	return ok
}

func Capabilities() []Capability {
	return []Capability{
		CapabilityChatCompletions,
		CapabilityResponses,
		CapabilitySpeechToText,
		CapabilityTextToSpeech,
	}
}

func Fields(capability Capability) []FieldKey {
	known := fields[capability]
	result := make([]FieldKey, 0, len(known))
	for field := range known {
		result = append(result, field)
	}
	return result
}