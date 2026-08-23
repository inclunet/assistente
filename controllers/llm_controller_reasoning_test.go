package controllers

import (
	"context"
	"testing"

	"assistente/internal/llm"
)

func TestCRUDProviderPreservaReasoningContentModeExplicito(t *testing.T) {
	ctrl, registry, _ := controladorDeProvedores(t)
	ctx := context.Background()

	created, err := ctrl.CreateLLMProvider(ctx, CreateLLMProviderRequest{
		ID:                   "proxy-reasoning",
		Name:                 "Proxy reasoning",
		Type:                 string(llm.ProviderCustom),
		APIFormat:            string(llm.APIFormatOpenAI),
		BaseURL:              "https://proxy.example/v1",
		ReasoningContentMode: string(llm.ReasoningContentReplayWithTools),
	})
	if err != nil {
		t.Fatal(err)
	}
	if created["reasoning_content_mode"] != string(llm.ReasoningContentReplayWithTools) {
		t.Fatalf("create devolveu mode = %#v", created["reasoning_content_mode"])
	}
	if registry.Get("proxy-reasoning").ReasoningContentMode != llm.ReasoningContentReplayWithTools {
		t.Fatal("create não persistiu a capability no registry")
	}

	updated, err := ctrl.UpdateLLMProvider(ctx, "proxy-reasoning", UpdateLLMProviderRequest{
		ReasoningContentMode: string(llm.ReasoningContentDisabled),
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated["reasoning_content_mode"] != string(llm.ReasoningContentDisabled) {
		t.Fatalf("update devolveu mode = %#v", updated["reasoning_content_mode"])
	}
	if registry.Get("proxy-reasoning").EffectiveReasoningContentMode() != llm.ReasoningContentDisabled {
		t.Fatal("update não desabilitou a capability")
	}
}
