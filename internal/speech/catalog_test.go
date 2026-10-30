package speech

import (
	"context"
	"encoding/json"
	"testing"

	"assistente/internal/llm"
	"assistente/internal/llmcatalog"
	"assistente/internal/profiles"
)

func TestVoiceCatalogUsesConnectionAndExactModel(t *testing.T) {
	doc := llmcatalog.Document{Exposures: []llmcatalog.Exposure{{
		ServerProfileID: "proxy", APIFormat: "openai", ExposedModelID: "vendor/private:model",
		SelectionMode: "model_and_voice", Voices: &llmcatalog.VoiceList{
			Completeness: "partial", Items: []llmcatalog.Voice{{ID: "tenant:voice-id", Name: "Minha voz", Language: "pt-BR"}},
		},
	}}}
	reg := &recordingRegistry{configs: map[string]*llm.ProviderConfig{
		"connection": {ID: "connection", ServerProfileID: "proxy", APIFormat: llm.APIFormatOpenAI},
		"other":      {ID: "other", ServerProfileID: "proxy", APIFormat: llm.APIFormatOpenAIResponses},
	}}
	svc := NewService(ServiceConfig{Registry: reg, CatalogSnapshot: func() llmcatalog.Snapshot { return llmcatalog.Snapshot{Document: doc} }})
	ctx := context.Background()
	got := svc.GetTTSVoices(ctx, "connection", "vendor/private:model")
	if got.Completeness != "partial" || got.SelectionMode != "model_and_voice" || len(got.Items) != 1 || got.Items[0].ID != "tenant:voice-id" || got.Items[0].Language != "pt-BR" {
		t.Fatalf("lost catalog data: %+v", got)
	}
	got.Items[0].Name = "changed"
	if doc.Exposures[0].Voices.Items[0].Name != "Minha voz" {
		t.Fatal("mutated snapshot")
	}
	for _, request := range [][2]string{{"other", "vendor/private:model"}, {"connection", "private:model"}, {"connection", "kokoro"}, {"connection", "voice-pt_BR-cadu"}} {
		got := svc.GetTTSVoices(ctx, request[0], request[1])
		if got.Completeness != "unknown" || got.SelectionMode != "" || len(got.Items) != 0 || got.Items == nil {
			t.Fatalf("inferred metadata for %v: %+v", request, got)
		}
	}
	models := svc.GetTTSModels(ctx, "connection")
	if len(models) != 1 || models[0].ID != "vendor/private:model" || models[0].SelectionMode != "model_and_voice" {
		t.Fatalf("models: %+v", models)
	}
	if len(svc.GetTTSModels(ctx, "other")) != 0 {
		t.Fatal("crossed API formats")
	}
}

func TestTTSRequestPreservesExactVoiceAndOmitsAbsentVoice(t *testing.T) {
	for _, tc := range []struct {
		model, voice string
		mode         TTSSelectionMode
	}{
		{"kokoro", "tenant:af-heart", TTSSelectionModelAndVoice},
		{"voice-private", "tenant:voice-id", TTSSelectionModelAndVoice},
		{"private-model", "", TTSSelectionModelOnly},
		{"unknown-model", "", ""},
	} {
		client := &TTSClient{config: TTSConfig{Model: TTSModel(tc.model), SelectionMode: tc.mode, Speed: 1}}
		params, err := client.buildParams("Teste", TTSVoice(tc.voice))
		if err != nil {
			t.Fatal(err)
		}
		raw, err := json.Marshal(params)
		if err != nil {
			t.Fatal(err)
		}
		var payload map[string]any
		if err := json.Unmarshal(raw, &payload); err != nil {
			t.Fatal(err)
		}
		voice, present := payload["voice"]
		if tc.voice == "" && present {
			t.Fatalf("voice must be absent: %s", raw)
		}
		if tc.voice != "" && voice != tc.voice {
			t.Fatalf("voice ID changed: %s", raw)
		}
		if payload["model"] != tc.model {
			t.Fatalf("model ID changed: %s", raw)
		}
	}
}

func TestCatalogModeIsSharedByPreviewAndProfileRuntime(t *testing.T) {
	cfg := &llm.ProviderConfig{ID: "proxy", Name: "Proxy", Type: llm.ProviderOpenAI, BaseURL: "https://example.com/v1", APIFormat: llm.APIFormatOpenAI, ServerProfileID: "proxy"}
	registry := &recordingRegistry{configs: map[string]*llm.ProviderConfig{"proxy": cfg}}
	doc := llmcatalog.Document{Exposures: []llmcatalog.Exposure{{ServerProfileID: "proxy", APIFormat: "openai", ExposedModelID: "private-model", SelectionMode: "model_and_voice"}}}
	svc := NewService(ServiceConfig{Registry: registry, CatalogSnapshot: func() llmcatalog.Snapshot { return llmcatalog.Snapshot{Document: doc} }})
	profile := &profiles.Profile{Voice: profiles.VoiceConfig{Assistant: profiles.VoiceRoleConfig{Provider: "openai", LLMProviderID: "proxy", Model: "private-model", SelectionMode: "model_only", Rate: 1}}}
	preview := svc.CreateTTSClient(context.Background(), "proxy", "private-model")
	if preview == nil {
		t.Fatal("missing preview client")
	}
	manager := NewSpeechManagerFromProfile(context.Background(), profile, registry, nil, func() llmcatalog.Snapshot { return llmcatalog.Snapshot{Document: doc} })
	runtime := manager.getTTSClientForRole("assistant")
	if runtime == nil {
		t.Fatal("missing runtime client")
	}
	for _, client := range []*TTSClient{preview, runtime} {
		if _, err := client.buildParams("test", ""); err == nil {
			t.Fatal("accepted missing required voice")
		}
	}
	doc.Exposures[0].SelectionMode = "model_only"
	for _, client := range []*TTSClient{preview, runtime} {
		if _, err := client.buildParams("test", ""); err != nil {
			t.Fatalf("stale contract after catalog refresh: %v", err)
		}
		if _, err := client.buildParams("test", "voice"); err == nil {
			t.Fatal("accepted voice after catalog switched to model_only")
		}
	}
	if profile.Voice.Assistant.SelectionMode != "model_only" {
		t.Fatal("mutated saved profile")
	}
}
