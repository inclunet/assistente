package providers

import (
	"testing"

	"assistente/internal/database"
	"assistente/internal/llm"
)

func TestFromDBModelPreservesExplicitErrorFamilyForWizardProviders(t *testing.T) {
	tests := []struct {
		id          string
		typeName    string
		errorFamily string
		want        llm.ProviderErrorFamily
	}{
		{id: "openai-default", typeName: "openai", want: llm.ProviderErrorFamilyOpenAI},
		{id: "openrouter-default", typeName: "openai", want: llm.ProviderErrorFamilyOpenRouter},
		{id: "litellm", typeName: "openai", want: llm.ProviderErrorFamilyLiteLLM},
		{id: "custom-provider", typeName: "openai", want: ""},
		{id: "typed-litellm", typeName: "litellm", want: llm.ProviderErrorFamilyLiteLLM},
	}
	for _, test := range tests {
		t.Run(test.id, func(t *testing.T) {
			provider, err := fromDBModel(&database.LLMProvider{
				ID: test.id, Name: test.id, Type: test.typeName,
				ErrorFamily: test.errorFamily, APIFormat: string(llm.APIFormatOpenAI),
			})
			if err != nil {
				t.Fatal(err)
			}
			if provider.ErrorFamily != test.want {
				t.Fatalf("error family = %q, want %q", provider.ErrorFamily, test.want)
			}
			if got := toDBModel(provider).ErrorFamily; got != string(test.want) {
				t.Fatalf("persisted error family = %q, want %q", got, test.want)
			}
		})
	}
}

func TestBuiltinTemplatesDoNotGuessGoogleErrorFamily(t *testing.T) {
	google, err := BuiltinTemplate("google")
	if err != nil {
		t.Fatal(err)
	}
	if google.ErrorFamily != "" {
		t.Fatalf("família de erro Gemini = %q, queria classificação genérica", google.ErrorFamily)
	}

	openrouter, err := BuiltinTemplate("openrouter")
	if err != nil {
		t.Fatal(err)
	}
	if openrouter.ErrorFamily != llm.ProviderErrorFamilyOpenRouter {
		t.Fatalf("família OpenRouter = %q, queria identidade explícita", openrouter.ErrorFamily)
	}
}
