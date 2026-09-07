package app

import (
	"testing"

	"assistente/internal/profiles"
)

// Bug 1b: mergeBuiltinPreservingRuntime preserva Active e MediaSupport
// do disco mesmo que o embedded tenha valores diferentes. Sem isso, cada
// upgrade builtin (incremento de _builtin_version) escreveria por cima
// do flag Active escolhido pelo usuário e do MediaSupport detectado em
// runtime — e o app voltaria a usar o profile builtin como ativo.
func TestMergeBuiltinPreservingRuntime_PreservaActive(t *testing.T) {
	embedded := profiles.Profile{
		Name:    "Padrão",
		Active:  false,
		Chat:    profiles.ChatConfig{LLMProvider: "openai-default", Model: "gpt-4o-mini"},
		Voice:   profiles.VoiceConfig{},
		Input:   profiles.InputConfig{},
	}
	existing := profiles.Profile{
		Name:   "Padrão",
		Active: true,
	}

	merged := mergeBuiltinPreservingRuntime(embedded, &existing)
	if !merged.Active {
		t.Error("merged.Active deveria ter vindo do disco (true), não do embedded (false)")
	}
	if merged.Chat.LLMProvider != "openai-default" {
		t.Errorf("merged.Chat.LLMProvider = %q; embedded deveria ter aplicado", merged.Chat.LLMProvider)
	}
}

func TestMergeBuiltinPreservingRuntime_PreservaMediaSupport(t *testing.T) {
	tru := true
	fls := false
	embedded := profiles.Profile{Name: "Padrão"}
	existing := profiles.Profile{
		Name: "Padrão",
		MediaSupport: &profiles.MediaSupport{
			Image:    &tru,
			Audio:    &fls,
			Document: &tru,
		},
	}

	merged := mergeBuiltinPreservingRuntime(embedded, &existing)
	if merged.MediaSupport == nil {
		t.Fatal("MediaSupport deveria ter sido preservado")
	}
	if merged.MediaSupport.Image == nil || !*merged.MediaSupport.Image {
		t.Error("Image=true do disco deveria ter sido preservado")
	}
	if merged.MediaSupport.Audio == nil || *merged.MediaSupport.Audio {
		t.Error("Audio=false do disco deveria ter sido preservado")
	}
}

// Bug 1b corolário: instalação inicial (sem profile no disco) usa
// embedded inteiro (sem dados de runtime para preservar).
func TestMergeBuiltinPreservingRuntime_InstalacaoInicial(t *testing.T) {
	embedded := profiles.Profile{
		Name:   "Programação",
		Active: false,
		Chat:   profiles.ChatConfig{LLMProvider: "$default", Model: "$default"},
	}

	merged := mergeBuiltinPreservingRuntime(embedded, nil)
	if merged.Name != "Programação" {
		t.Errorf("Name = %q; esperado %q", merged.Name, "Programação")
	}
	if merged.Active {
		t.Error("Active embedded é false; merged deveria refletir false")
	}
}
