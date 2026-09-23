package app

import "testing"

func TestCommandSettingsBuiltinText(t *testing.T) {
	tests := []struct {
		locale              string
		paletteName         string
		paletteDescription  string
		keyboardName        string
		keyboardDescription string
	}{
		{"pt-BR", "Comandos padrão", "Disponibiliza comandos na paleta sem definir atalhos.", "Mapa de teclado padrão", "Associa atalhos padrão a comandos."},
		{"en", "Default commands", "Makes commands available in the palette without defining shortcuts.", "Default keyboard map", "Associates default shortcuts with commands."},
		{"es", "Comandos predeterminados", "Pone comandos a disposición en la paleta sin definir atajos.", "Mapa de teclado predeterminado", "Asocia atajos predeterminados a comandos."},
	}
	for _, tt := range tests {
		t.Run(tt.locale, func(t *testing.T) {
			paletteName, paletteDescription := commandSettingsBuiltinText(tt.locale, commandPaletteLayerID)
			if paletteName != tt.paletteName || paletteDescription != tt.paletteDescription {
				t.Fatalf("texto da camada de comandos incorreto: %q, %q", paletteName, paletteDescription)
			}
			keyboardName, keyboardDescription := commandSettingsBuiltinText(tt.locale, commandKeyboardLayerID)
			if keyboardName != tt.keyboardName || keyboardDescription != tt.keyboardDescription {
				t.Fatalf("texto do mapa de teclado incorreto: %q, %q", keyboardName, keyboardDescription)
			}
		})
	}
}

func TestCommandSettingsBuiltinTextFallbackUnknown(t *testing.T) {
	name, description := commandSettingsBuiltinText("fr", "builtin.unknown")
	if name != "builtin.unknown" || description != "" {
		t.Fatalf("fallback inesperado: %q, %q", name, description)
	}
}
