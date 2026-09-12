//go:build windows

package main

import (
	"bytes"
	"testing"
)

func TestReportFatalErrorMantemStderrEExibeAvisoNativo(t *testing.T) {
	previous := showNativeFatalError
	t.Cleanup(func() {
		showNativeFatalError = previous
	})

	var nativeMessage string
	showNativeFatalError = func(message string) {
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
}
