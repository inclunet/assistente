package profiles

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"assistente/internal/configdir"
)

func TestPublishedProfilesUpgradeDirectly(t *testing.T) {
	for _, version := range []string{"0.1.9", "0.2.0", "0.3.0", "0.4.0", "0.5.0"} {
		t.Run(version, func(t *testing.T) {
			tempDir := t.TempDir()
			t.Setenv("HOME", tempDir)
			t.Setenv("USERPROFILE", tempDir)
			configdir.ResetForTests()
			t.Cleanup(configdir.ResetForTests)

			fixture, err := os.ReadFile(filepath.Join("testdata", "published", version+".json"))
			if err != nil {
				t.Fatal(err)
			}
			profileDir := filepath.Join(tempDir, ".assistente", "profiles")
			if err := os.MkdirAll(profileDir, 0700); err != nil {
				t.Fatal(err)
			}
			profilePath := filepath.Join(profileDir, "publicado.json")
			if err := os.WriteFile(profilePath, fixture, 0600); err != nil {
				t.Fatal(err)
			}

			manager := NewManager()
			first, err := manager.Get("publicado")
			if err != nil {
				t.Fatalf("carregar perfil %s: %v", version, err)
			}
			if first.Name != "Perfil sintético "+version {
				t.Fatalf("nome do perfil %s não preservado: %q", version, first.Name)
			}
			if first.Chat.LLMProvider != "provedor-sintetico" || first.Chat.Model != "modelo-sintetico" {
				t.Fatalf("routing do perfil %s não preservado: %+v", version, first.Chat)
			}
			if err := first.Validate(); err != nil {
				t.Fatalf("perfil %s inválido após leitura atual: %v", version, err)
			}

			second, err := manager.Get("publicado")
			if err != nil || second.Name != first.Name {
				t.Fatalf("segunda leitura do perfil %s: %+v, %v", version, second, err)
			}
			after, err := os.ReadFile(profilePath)
			if err != nil || !bytes.Equal(after, fixture) {
				t.Fatalf("leitura idempotente alterou fixture %s: %v", version, err)
			}

			if version == "0.1.9" {
				assertPublishedProfile019Migration(t, first)
			}
		})
	}
}

func assertPublishedProfile019Migration(t *testing.T, profile *Profile) {
	t.Helper()
	if profile.Chat.NativeMCP == nil || *profile.Chat.NativeMCP {
		t.Fatalf("mcp_mode adapter não foi preservado: %+v", profile.Chat.NativeMCP)
	}
	if !profile.Voice.Assistant.Enabled ||
		profile.Voice.Assistant.Provider != "openai" ||
		profile.Voice.Assistant.VoiceID != "voz-sintetica" ||
		profile.Voice.Assistant.Model != "tts-1" {
		t.Fatalf("voz do assistente 0.1.9 não preservada: %+v", profile.Voice.Assistant)
	}
	if profile.Voice.User.Enabled || profile.Voice.System.Enabled {
		t.Fatalf("escopo das roles de voz 0.1.9 mudou: %+v", profile.Voice)
	}
	if !profile.Input.Enabled ||
		profile.Input.STTProvider != "whisper_api" ||
		profile.Input.LLMProviderID != "provedor-stt-sintetico" ||
		len(profile.Input.Triggers) != 1 {
		t.Fatalf("entrada 0.1.9 não preservada: %+v", profile.Input)
	}
	if profile.Channels.ResponseMode != ChannelResponseAlwaysAudio {
		t.Fatalf("modo de canal 0.1.9 não preservado: %q", profile.Channels.ResponseMode)
	}
	if profile.Chat.EnabledTools != nil {
		t.Fatalf("nil legado de enabled_tools perdeu semântica: %#v", profile.Chat.EnabledTools)
	}
}
