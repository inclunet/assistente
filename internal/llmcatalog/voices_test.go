package llmcatalog

import "testing"

func TestResolveTTSSelectionUsesExactExposure(t *testing.T) {
	doc := Document{Exposures: []Exposure{{
		ServerProfileID: "proxy", APIFormat: "openai", ExposedModelID: "vendor/voice-model",
		SelectionMode: "model_and_voice",
		Voices:        &VoiceList{Completeness: "partial", Items: []Voice{{ID: "voice", Name: "Voice", Language: "pt-BR"}}},
	}}}
	for _, tc := range []struct{ profile, format, model string }{
		{"other", "openai", "vendor/voice-model"},
		{"proxy", "openai_responses", "vendor/voice-model"},
		{"proxy", "openai", "voice-model"},
		{"proxy", "openai", "Vendor/voice-model"},
		{"", "openai", "vendor/voice-model"},
	} {
		got := doc.ResolveTTSSelection(tc.profile, tc.format, tc.model)
		if got.SelectionMode != "" || got.Voices.Completeness != "unknown" || got.Voices.Items == nil || len(got.Voices.Items) != 0 {
			t.Fatalf("%+v: unexpected inference: %+v", tc, got)
		}
	}
	got := doc.ResolveTTSSelection("proxy", "openai", "vendor/voice-model")
	if got.SelectionMode != "model_and_voice" || got.Voices.Completeness != "partial" || len(got.Voices.Items) != 1 || got.Voices.Items[0].Language != "pt-BR" {
		t.Fatalf("lost metadata: %+v", got)
	}
	got.Voices.Items[0].Name = "changed"
	if doc.Exposures[0].Voices.Items[0].Name != "Voice" {
		t.Fatal("resolution mutated the snapshot")
	}
}

func TestResolveTTSSelectionPreservesCompletenessAndMode(t *testing.T) {
	for _, completeness := range []string{"complete", "partial", "unknown"} {
		doc := Document{Exposures: []Exposure{{
			ServerProfileID: "local", APIFormat: "openai", ExposedModelID: "private-model",
			SelectionMode: "model_only", Voices: &VoiceList{Completeness: completeness, Items: []Voice{}},
		}}}
		got := doc.ResolveTTSSelection("local", "openai", "private-model")
		if got.SelectionMode != "model_only" || got.Voices.Completeness != completeness || got.Voices.Items == nil {
			t.Fatalf("lost empty-list contract: %+v", got)
		}
		doc.Exposures[0].Voices = nil
		got = doc.ResolveTTSSelection("local", "openai", "private-model")
		if got.SelectionMode != "model_only" || got.Voices.Completeness != "unknown" {
			t.Fatalf("missing voices changed the mode: %+v", got)
		}
	}
}
