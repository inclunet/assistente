package llmcatalog

import "testing"

func TestResolveModelsUsesExactExposureAndIndependentMetadata(t *testing.T) {
	doc := Document{
		Models:    []Model{{ID: "vendor/model", Name: "Model", Capabilities: map[string]string{"tools.function_calling": "supported", "reasoning": "unknown"}, Limits: map[string]int64{"context_tokens": 1000}}},
		Exposures: []Exposure{{ServerProfileID: "proxy", APIFormat: "openai", ExposedModelID: "openai/team/model", CanonicalModelID: "vendor/model", CapabilityOverrides: map[string]string{"tools.function_calling": "unsupported"}}},
	}
	for _, tc := range []struct {
		profile, format, id string
		match               bool
	}{
		{"proxy", "openai", "openai/team/model", true},
		{"proxy", "openai_responses", "openai/team/model", false},
		{"other", "openai", "openai/team/model", false},
		{"", "openai", "openai/team/model", false},
		{"proxy", "openai", "team/model", false},
		{"proxy", "openai", "vendor/model", false},
		{"proxy", "openai", "OpenAI/team/model", false},
		{"proxy", "openai", "openai/team/model ", false},
	} {
		result := doc.ResolveModels(tc.profile, tc.format, []string{tc.id})
		model, found := result[tc.id]
		if found != tc.match {
			t.Fatalf("%+v: match=%t", tc, found)
		}
		if !found {
			continue
		}
		if model.ID != "vendor/model" || model.Capabilities["tools.function_calling"] != "unsupported" || model.Capabilities["reasoning"] != "unknown" || model.Limits["context_tokens"] != 1000 {
			t.Fatalf("metadata: %+v", model)
		}
		model.Capabilities["reasoning"] = "supported"
		model.Limits["context_tokens"] = 1
	}
	if doc.Models[0].Capabilities["reasoning"] != "unknown" || doc.Models[0].Limits["context_tokens"] != 1000 {
		t.Fatal("resolution mutated snapshot")
	}
	if len(doc.ResolveModels("proxy", "openai", []string{"private-alias"})) != 0 {
		t.Fatal("catalog added inaccessible model")
	}
}
