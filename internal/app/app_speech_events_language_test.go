package app

import (
	"os"
	"strings"
	"testing"

	"assistente/internal/configdir"
	"assistente/internal/profiles"
)

// setupSpeechProfileEnv isola o diretório de configuração para que os perfis
// criados no teste não vazem para o ambiente do desenvolvedor.
func setupSpeechProfileEnv(t *testing.T) *profiles.Manager {
	t.Helper()

	tempDir := t.TempDir()
	oldHome := os.Getenv("HOME")
	oldUserProfile := os.Getenv("USERPROFILE")
	oldCwd, _ := os.Getwd()

	if err := os.Setenv("HOME", tempDir); err != nil {
		t.Fatalf("set HOME: %v", err)
	}
	if err := os.Setenv("USERPROFILE", tempDir); err != nil {
		t.Fatalf("set USERPROFILE: %v", err)
	}
	if err := os.Chdir(tempDir); err != nil {
		t.Fatalf("chdir: %v", err)
	}

	configdir.ResetForTests()

	t.Cleanup(func() {
		_ = os.Chdir(oldCwd)
		_ = os.Setenv("HOME", oldHome)
		_ = os.Setenv("USERPROFILE", oldUserProfile)
		configdir.ResetForTests()
	})

	return profiles.NewManager()
}

func TestDispatchSpeechEventLocalizesCodeBlockLabel(t *testing.T) {
	cases := []struct {
		name      string
		language  string
		wantLabel string
	}{
		{name: "portugues", language: "pt-BR", wantLabel: "bloco de código"},
		{name: "espanhol", language: "es-ES", wantLabel: "bloque de código"},
		{name: "ingles", language: "en-US", wantLabel: "code block"},
		{name: "desconhecido", language: "ja-JP", wantLabel: "code block"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			pm := setupSpeechProfileEnv(t)

			p := profiles.DefaultProfile()
			p.Name = "Perfil Fala"
			p.Input.Language = tc.language
			slug, err := pm.Create(p)
			if err != nil {
				t.Fatalf("create profile: %v", err)
			}

			emitter := &testEmitter{}
			app := &App{profileManager: pm, emitter: emitter}

			event, err := app.dispatchSpeechEvent(ChatSpeakRequest{
				ConversationID: "1",
				ProfileSlug:    slug,
				Role:           "assistant",
				Text:           "veja:\n```go\nfmt.Println(1)\n```",
				Origin:         ChatSpeakOriginAssistantMessage,
			})
			if err != nil {
				t.Fatalf("dispatchSpeechEvent: %v", err)
			}
			if event == nil {
				t.Fatal("dispatchSpeechEvent devolveu evento nil")
			}
			if !strings.Contains(event.Text, tc.wantLabel) {
				t.Fatalf("texto falado = %q, esperava conter %q", event.Text, tc.wantLabel)
			}
			// backend_audio regera o áudio pelo SpeakMessage: o idioma precisa
			// viajar no evento para o rótulo não sair do perfil ativo.
			if event.SpeechLanguage != tc.language {
				t.Fatalf("speechLanguage = %q, esperava %q", event.SpeechLanguage, tc.language)
			}
			if len(emitter.find("chat:speak")) != 1 {
				t.Fatalf("esperava 1 evento chat:speak, obteve %d", len(emitter.find("chat:speak")))
			}
		})
	}
}

func TestDispatchSpeechEventIgnoresEmptyText(t *testing.T) {
	pm := setupSpeechProfileEnv(t)
	emitter := &testEmitter{}
	app := &App{profileManager: pm, emitter: emitter}

	event, err := app.dispatchSpeechEvent(ChatSpeakRequest{
		ConversationID: "1",
		Role:           "assistant",
		Text:           "   ",
	})
	if err != nil {
		t.Fatalf("dispatchSpeechEvent: %v", err)
	}
	if event != nil {
		t.Fatalf("esperava evento nil para texto vazio, obteve %+v", event)
	}
	if emitter.count() != 0 {
		t.Fatalf("esperava nenhum evento emitido, obteve %d", emitter.count())
	}
}
