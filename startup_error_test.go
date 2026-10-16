package main

import (
	"bytes"
	"testing"
)

func TestReportFatalErrorMantemStderrEExibeAvisoNativo(t *testing.T) {
	previous := showNativeFatalError
	previousLocaleProvider := startupLocaleProvider
	t.Cleanup(func() {
		showNativeFatalError = previous
		startupLocaleProvider = previousLocaleProvider
	})
	startupLocaleProvider = func() string { return "pt-BR" }

	var nativeTitle, nativeMessage string
	showNativeFatalError = func(title, message string) {
		nativeTitle = title
		nativeMessage = message
	}
	var output bytes.Buffer

	reportFatalError(&output, "caminho de log inacessível")

	if output.String() != "caminho de log inacessível\n" {
		t.Fatalf("saída = %q", output.String())
	}
	if nativeMessage != "caminho de log inacessível" {
		t.Fatalf("aviso nativo = %q", nativeMessage)
	}
	if nativeTitle != "Assistente IA" {
		t.Fatalf("título nativo = %q", nativeTitle)
	}
}
