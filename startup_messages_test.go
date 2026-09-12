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
		locale   string
		expected string
	}{
		{locale: "pt-BR", expected: "requer um caminho"},
		{locale: "en-US", expected: "requires a path"},
		{locale: "es-ES", expected: "requiere una ruta"},
	}
	for _, test := range tests {
		t.Run(test.locale, func(t *testing.T) {
			startupLocaleProvider = func() string { return test.locale }
			got := startupLogConfigurationError(logging.ErrLogFilePathRequired)
			if !strings.Contains(got, test.expected) {
				t.Fatalf("mensagem para %s = %q", test.locale, got)
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
