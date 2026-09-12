package main

import (
	"errors"
	"strings"
	"testing"

	"assistente/internal/logging"
)

func TestStartupMessagesSuportamTresIdiomas(t *testing.T) {
	previous := startupLocaleProvider
	t.Cleanup(func() {
		startupLocaleProvider = previous
	})

	tests := []struct {
		locale        string
		expected      string
		expectedTitle string
	}{
		{locale: "pt-BR", expected: "requer um caminho", expectedTitle: "Assistente IA"},
		{locale: "en-US", expected: "requires a path", expectedTitle: "AI Assistant"},
		{locale: "es-ES", expected: "requiere una ruta", expectedTitle: "Asistente IA"},
	}
	for _, test := range tests {
		t.Run(test.locale, func(t *testing.T) {
			startupLocaleProvider = func() string { return test.locale }
			got := startupLogConfigurationError(logging.ErrLogFilePathRequired)
			if !strings.Contains(got, test.expected) {
				t.Fatalf("mensagem para %s = %q", test.locale, got)
			}
			if title := startupDialogTitle(); title != test.expectedTitle {
				t.Fatalf("título para %s = %q", test.locale, title)
			}
		})
	}
}

func TestStartupLogConfigurationErrorPreservaDetalhesDeAbertura(t *testing.T) {
	previous := startupLocaleProvider
	startupLocaleProvider = func() string { return "en" }
	t.Cleanup(func() {
		startupLocaleProvider = previous
	})

	got := startupLogConfigurationError(&logging.FileOpenError{
		Path: "assistente.log",
		Err:  errors.New("access denied"),
	})
	if !strings.Contains(got, `"assistente.log"`) || !strings.Contains(got, "access denied") {
		t.Fatalf("mensagem perdeu detalhes do erro: %q", got)
	}
}
