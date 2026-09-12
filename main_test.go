package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	application "assistente/internal/app"

	"github.com/wailsapp/wails/v2/pkg/options"
)

func TestRunRemoveFlagPropagaStartupErrorEFechaLog(t *testing.T) {
	previousRunDesktop := runDesktop
	previousStartDesktop := startDesktop
	previousQuitDesktop := quitDesktop
	previousNativeError := showNativeFatalError
	previousLocaleProvider := startupLocaleProvider
	t.Cleanup(func() {
		runDesktop = previousRunDesktop
		startDesktop = previousStartDesktop
		quitDesktop = previousQuitDesktop
		showNativeFatalError = previousNativeError
		startupLocaleProvider = previousLocaleProvider
	})
	startupLocaleProvider = func() string { return "pt-BR" }

	var runnerArgs []string
	runDesktop = func(appOptions *options.App) error {
		runnerArgs = slices.Clone(os.Args)
		appOptions.OnStartup(context.Background())
		return nil
	}
	startDesktop = func(*application.App, context.Context) error {
		return errors.New("startup indisponível")
	}
	quitCalled := false
	quitDesktop = func(context.Context) {
		quitCalled = true
	}
	var nativeError string
	showNativeFatalError = func(message string) {
		nativeError = message
	}

	logPath := filepath.Join(t.TempDir(), "assistente.log")
	exitCode := run([]string{"assistente", "--log-file", logPath, "--debug"})

	if exitCode != 1 {
		t.Fatalf("exitCode = %d, esperado 1", exitCode)
	}
	if !slices.Equal(runnerArgs, []string{"assistente", "--debug"}) {
		t.Fatalf("argumentos recebidos pelo Wails = %#v", runnerArgs)
	}
	if !quitCalled {
		t.Fatal("falha de startup não solicitou encerramento do Wails")
	}
	if !strings.Contains(nativeError, "Falha ao inicializar aplicação: startup indisponível") {
		t.Fatalf("erro nativo = %q", nativeError)
	}

	content, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("ler arquivo de log: %v", err)
	}
	if !strings.Contains(string(content), "startup indisponível") {
		t.Fatalf("arquivo não recebeu erro de startup:\n%s", content)
	}
	if err := os.Rename(logPath, logPath+".fechado"); err != nil {
		t.Fatalf("arquivo de log permaneceu aberto após run: %v", err)
	}
}

func TestRunPropagaErroDoRunnerEFechaLog(t *testing.T) {
	previousRunDesktop := runDesktop
	previousNativeError := showNativeFatalError
	previousLocaleProvider := startupLocaleProvider
	t.Cleanup(func() {
		runDesktop = previousRunDesktop
		showNativeFatalError = previousNativeError
		startupLocaleProvider = previousLocaleProvider
	})
	startupLocaleProvider = func() string { return "pt-BR" }

	var runnerArgs []string
	runDesktop = func(*options.App) error {
		runnerArgs = slices.Clone(os.Args)
		return errors.New("Wails indisponível")
	}
	var nativeError string
	showNativeFatalError = func(message string) {
		nativeError = message
	}

	logPath := filepath.Join(t.TempDir(), "assistente.log")
	exitCode := run([]string{"assistente", "--log-file=" + logPath, "--debug"})

	if exitCode != 1 {
		t.Fatalf("exitCode = %d, esperado 1", exitCode)
	}
	if !slices.Equal(runnerArgs, []string{"assistente", "--debug"}) {
		t.Fatalf("argumentos recebidos pelo Wails = %#v", runnerArgs)
	}
	if nativeError != "Erro: Wails indisponível" {
		t.Fatalf("erro nativo = %q", nativeError)
	}
	if err := os.Rename(logPath, logPath+".fechado"); err != nil {
		t.Fatalf("arquivo de log permaneceu aberto após erro do runner: %v", err)
	}
}
