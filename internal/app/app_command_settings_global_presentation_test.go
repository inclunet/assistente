package app

import (
	"context"
	"testing"

	"assistente/internal/commandconfig"
)

func TestCommandSettingsGlobalTriggerPresentationRoundTrip(t *testing.T) {
	ctx := context.Background()
	for _, identity := range []string{"keyboard.global:Control+Alt+KeyG", "keyboard.global:Control+Space", "keyboard.global:Meta+F12"} {
		if got := commandSettingsTriggerType(identity); got != "keyboard.global" {
			t.Fatalf("origem global publicada como %q", got)
		}
		raw, err := commandSettingsTriggerSpec(ctx, identity)
		if err != nil {
			t.Fatal(err)
		}
		got, err := (commandconfig.KeyboardGlobalTriggerPort{}).Normalize(ctx, []byte(raw))
		if err != nil || got != identity {
			t.Fatalf("identidade não preservada: %s => %s => %s, %v", identity, raw, got, err)
		}
	}
	for _, identity := range []string{"keyboard.global:Control+KeyK KeyC", "keyboard.global:Alt+Control+KeyG", "keyboard.global:"} {
		if _, err := commandSettingsTriggerSpec(ctx, identity); err == nil {
			t.Fatalf("identidade global inválida aceita: %s", identity)
		}
	}
	for _, tc := range []struct{ locale, name string }{
		{"pt-BR", "Atalhos globais de voz e jobs"},
		{"en", "Global voice and job shortcuts"},
		{"es", "Atajos globales de voz y tareas"},
	} {
		name, description := commandSettingsBuiltinText(tc.locale, commandGlobalLayerID)
		if name != tc.name || description == "" {
			t.Fatalf("camada sem apresentação localizada: %s, %q, %q", tc.locale, name, description)
		}
	}
}
