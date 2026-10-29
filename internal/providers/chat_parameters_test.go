package providers

import (
	"math"
	"testing"

	"assistente/internal/llm"
	"assistente/internal/llmcapabilities"
	"assistente/internal/llmcatalog"
)

func TestChatParameterContractExposesNumericDomainWithoutCatalog(t *testing.T) {
	service := NewService(ServiceConfig{})
	fields := service.ChatParameterContract(nil, "private", nil)
	for _, tc := range []struct {
		name    string
		minimum float64
		maximum *float64
	}{
		{"temperature", 0, nil},
		{"top_p", 0, numericBound(1)},
		{"max_output_tokens", 1, numericBound(math.MaxInt32)},
	} {
		field := fields[tc.name]
		if field.Support != "unknown" || field.Default != nil || field.Minimum == nil || *field.Minimum != tc.minimum {
			t.Fatalf("%s: %+v", tc.name, field)
		}
		if tc.maximum != nil && (field.Maximum == nil || *field.Maximum != *tc.maximum) {
			t.Fatalf("%s: maximum=%v", tc.name, field.Maximum)
		}
		if field.ValidateValue(tc.name, tc.minimum-1) == nil {
			t.Fatalf("%s accepted out of domain value", tc.name)
		}
	}
}

func numericBound(value float64) *float64 { return &value }

func TestChatParameterContractDefaultsExplicitZeroAndLocalSuppression(t *testing.T) {
	minimum, maximum := 0.0, 1.0
	doc := llmcatalog.Document{
		Models: []llmcatalog.Model{{ID: "canonical", Parameters: map[string]llmcatalog.Parameter{
			"temperature":       {Support: "supported", Minimum: &minimum, Maximum: &maximum, Default: 0.7},
			"max_output_tokens": {Support: "supported", Default: 2048.0},
			"reasoning_effort":  {Support: "supported", Choices: []string{"low", "high"}, Default: "low"},
		}}},
		Exposures: []llmcatalog.Exposure{{ServerProfileID: "proxy", APIFormat: "openai", ExposedModelID: "alias", CanonicalModelID: "canonical"}},
	}
	service := NewService(ServiceConfig{CatalogSnapshot: func() llmcatalog.Snapshot { return llmcatalog.Snapshot{Document: doc} }})
	provider := &llm.ProviderConfig{ServerProfileID: "proxy", APIFormat: llm.APIFormatOpenAI, DefaultModel: "alias"}
	provider.Model, provider.DefaultModel = "alias", "other"
	input := llm.ChatParams{}
	resolved, err := service.ApplyChatParameterContract(provider, input)
	if err != nil || resolved.Model != "alias" || resolved.Temperature == nil || *resolved.Temperature != 0.7 || resolved.OutputTokenLimit() != 2048 || resolved.Reasoning() != "low" {
		t.Fatalf("defaults: %+v, %v", resolved, err)
	}
	if input.Temperature != nil || input.MaxTokens != nil || input.ReasoningEffort != nil {
		t.Fatal("resolution changed input")
	}
	zero, off := 0.0, ""
	input.Temperature, input.ReasoningEffort = &zero, &off
	resolved, err = service.ApplyChatParameterContract(provider, input)
	if err != nil || resolved.Temperature == nil || *resolved.Temperature != 0 || resolved.ReasoningEffort == nil || *resolved.ReasoningEffort != "" {
		t.Fatalf("explicit values lost: %+v, %v", resolved, err)
	}
	input.Compatibility = llm.NewCompatibilityState("model", 1, []llm.CompatibilityField{{Capability: llmcapabilities.CapabilityChatCompletions, Field: llmcapabilities.FieldTemperature}}, nil)
	resolved, err = service.ApplyChatParameterContract(provider, input)
	if err != nil || resolved.Temperature != nil || input.Temperature == nil || *input.Temperature != 0 {
		t.Fatalf("local suppression: %+v, %v", resolved, err)
	}
	manual := 1.5
	input = llm.ChatParams{Temperature: &manual}
	if _, err := service.ApplyChatParameterContract(provider, input); err == nil || manual != 1.5 {
		t.Fatal("invalid explicit value was accepted or rewritten")
	}
	input.Model = "private-alias"
	resolved, err = service.ApplyChatParameterContract(provider, input)
	if err != nil || resolved.Temperature != input.Temperature || resolved.MaxTokens != nil {
		t.Fatalf("unknown alias was constrained: %+v, %v", resolved, err)
	}
}

func TestChatParameterContractUsesPublishedOutputLimitWithoutInventingDefault(t *testing.T) {
	doc := llmcatalog.Document{
		Models:    []llmcatalog.Model{{ID: "canonical", Limits: map[string]int64{"max_output_tokens": 128, "context_tokens": 256}}},
		Exposures: []llmcatalog.Exposure{{ServerProfileID: "server", APIFormat: "openai", ExposedModelID: "model", CanonicalModelID: "canonical"}},
	}
	service := NewService(ServiceConfig{CatalogSnapshot: func() llmcatalog.Snapshot { return llmcatalog.Snapshot{Document: doc} }})
	provider := &llm.ProviderConfig{ServerProfileID: "server", APIFormat: llm.APIFormatOpenAI}
	resolved, err := service.ApplyChatParameterContract(provider, llm.ChatParams{Model: "model"})
	if err != nil || resolved.MaxTokens != nil {
		t.Fatalf("invented default: %+v, %v", resolved, err)
	}
	if resolved.ContextWindow != 0 {
		t.Fatal("changed explicitly disabled context management")
	}
	if _, err := service.ApplyChatParameterContract(provider, llm.ChatParams{Model: "model", ContextWindow: 257}); err == nil {
		t.Fatal("published context limit was ignored")
	}
	limit := 129
	if _, err := service.ApplyChatParameterContract(provider, llm.ChatParams{Model: "model", MaxTokens: &limit}); err == nil {
		t.Fatal("published limit was ignored")
	}
	doc.Models[0].Parameters = map[string]llmcatalog.Parameter{"max_output_tokens": {Support: "supported", Default: 64.0}}
	resolved, err = service.ApplyChatParameterContract(provider, llm.ChatParams{Model: "model"})
	if err != nil || resolved.OutputTokenLimit() != 64 {
		t.Fatalf("default alongside limit: %+v, %v", resolved, err)
	}
	if _, err := service.ApplyChatParameterContract(provider, llm.ChatParams{Model: "model", MaxTokens: &limit}); err == nil {
		t.Fatal("declaring a default erased the published limit")
	}
}
