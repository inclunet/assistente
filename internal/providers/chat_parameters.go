package providers

import (
	"assistente/internal/llm"
	"assistente/internal/llmcapabilities"
	"assistente/internal/llmcatalog"
)

// ChatParameterContract resolves the exact connection snapshot used by the
// caller. Local restrictions are supplied by the authenticated compatibility
// lookup, never inferred from catalog metadata.
func (s *Service) ChatParameterContract(provider *llm.ProviderConfig, modelID string, unsupported []string) map[string]llmcatalog.Parameter {
	fields := make(map[string]llmcatalog.Parameter)
	if provider != nil && s.catalogSnapshot != nil {
		model := s.catalogSnapshot().Document.ResolveModels(provider.ServerProfileID, string(provider.GetAPIFormat()), []string{modelID}, provider.ModelAliases)[modelID]
		for name, field := range model.Parameters {
			fields[name] = field
		}
		if limit := model.Limits["context_tokens"]; limit > 0 {
			maximum := float64(limit)
			fields["context_window"] = llmcatalog.Parameter{Support: "unknown", Maximum: &maximum}
		}
	}
	if provider != nil && (provider.GetAPIFormat() == llm.APIFormatAnthropic || provider.GetAPIFormat() == llm.APIFormatGoogle) {
		// These adapters do not yet translate the canonical reasoning field.
		// Do not offer or default a value that the transport would discard.
		fields["reasoning_effort"] = llmcatalog.Parameter{Support: "unsupported"}
	}
	for _, name := range unsupported {
		fields[name] = llmcatalog.Parameter{Support: "unsupported"}
	}
	for _, name := range []string{"temperature", "top_p", "max_output_tokens", "context_window"} {
		field, known := fields[name]
		if !known {
			field.Support = "unknown"
		}
		fields[name] = field.WithDomain(name)
	}
	return fields
}

// ApplyChatParameterContract prepares a request copy without writing defaults
// back to the profile. A suppressed field cannot receive a catalog default.
func (s *Service) ApplyChatParameterContract(provider *llm.ProviderConfig, params llm.ChatParams) (llm.ChatParams, error) {
	if provider != nil {
		params.Model = llm.ResolveModel(provider, params.Model)
	}
	var operation llmcapabilities.Capability
	if provider != nil {
		switch provider.GetAPIFormat() {
		case llm.APIFormatOpenAI:
			operation = llmcapabilities.CapabilityChatCompletions
		case llm.APIFormatOpenAIResponses:
			operation = llmcapabilities.CapabilityResponses
		}
	}
	var unsupported []string
	for _, name := range []llmcapabilities.FieldKey{llmcapabilities.FieldTemperature, llmcapabilities.FieldTopP, llmcapabilities.FieldMaxOutputTokens, llmcapabilities.FieldReasoningEffort} {
		if params.Compatibility.IsUnsupported(operation, name) {
			unsupported = append(unsupported, string(name))
		}
	}
	fields := s.ChatParameterContract(provider, params.Model, unsupported)
	if err := fields["context_window"].ValidateValue("context_window", params.ContextWindow); err != nil {
		return params, err
	}
	for _, entry := range []struct {
		name  string
		value **float64
	}{
		{"temperature", &params.Temperature},
		{"top_p", &params.TopP},
	} {
		field := fields[entry.name]
		if field.Support == "unsupported" {
			*entry.value = nil
			continue
		}
		if *entry.value == nil {
			*entry.value = field.NumericDefault()
		}
		if *entry.value != nil {
			if err := field.ValidateValue(entry.name, **entry.value); err != nil {
				return params, err
			}
		}
	}
	tokens := fields["max_output_tokens"]
	if tokens.Support == "unsupported" {
		params.MaxTokens = nil
	} else {
		if params.MaxTokens == nil {
			if value := tokens.NumericDefault(); value != nil {
				if err := tokens.ValidateValue("max_output_tokens", *value); err != nil {
					return params, err
				}
				limit := int(*value)
				params.MaxTokens = &limit
			}
		}
		if params.MaxTokens != nil {
			if err := tokens.ValidateValue("max_output_tokens", *params.MaxTokens); err != nil {
				return params, err
			}
		}
	}
	reasoning := fields["reasoning_effort"]
	if reasoning.Support == "unsupported" {
		params.ReasoningEffort = nil
	} else {
		if params.ReasoningEffort == nil {
			if value, ok := reasoning.Default.(string); ok {
				params.ReasoningEffort = &value
			}
		}
		// Empty/off are the persisted application choice to omit reasoning,
		// distinct from nil (use the model default).
		if params.ReasoningEffort != nil && params.Reasoning() != "" && params.Reasoning() != "off" {
			if err := reasoning.ValidateValue("reasoning_effort", *params.ReasoningEffort); err != nil {
				return params, err
			}
		}
	}
	return params, nil
}
