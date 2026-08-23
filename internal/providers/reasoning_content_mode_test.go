package providers

import (
	"testing"

	"assistente/internal/database"
	"assistente/internal/llm"
)

func TestBuiltinDeepSeekDeclaraCapabilityDeReasoningContent(t *testing.T) {
	provider, err := BuiltinTemplate("deepseek")
	if err != nil {
		t.Fatal(err)
	}
	if provider.ReasoningContentMode != llm.ReasoningContentReplayWithTools {
		t.Fatalf("reasoning_content_mode = %q, want %q",
			provider.ReasoningContentMode, llm.ReasoningContentReplayWithTools)
	}
}

func TestReasoningContentModeSobreviveAoBanco(t *testing.T) {
	original := &llm.ProviderConfig{
		ID:                   "proxy",
		Name:                 "Proxy",
		Type:                 llm.ProviderCustom,
		APIFormat:            llm.APIFormatOpenAI,
		BaseURL:              "https://proxy.example/v1",
		ReasoningContentMode: llm.ReasoningContentReplayWithTools,
	}

	dbModel := toDBModel(original)
	if dbModel.ReasoningContentMode != string(llm.ReasoningContentReplayWithTools) {
		t.Fatalf("valor no banco = %q", dbModel.ReasoningContentMode)
	}
	reloaded, err := fromDBModel(&database.LLMProvider{
		ID:                   dbModel.ID,
		Name:                 dbModel.Name,
		Type:                 dbModel.Type,
		APIFormat:            dbModel.APIFormat,
		BaseURL:              dbModel.BaseURL,
		ReasoningContentMode: dbModel.ReasoningContentMode,
	})
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.ReasoningContentMode != llm.ReasoningContentReplayWithTools {
		t.Fatalf("valor recarregado = %q", reloaded.ReasoningContentMode)
	}
}
