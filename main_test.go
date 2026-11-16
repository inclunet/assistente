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
	"assistente/internal/desktopinstance"

	"github.com/wailsapp/wails/v2/pkg/options"
)

func TestRunNaoAbreLogAntesDeReservarInstancia(t *testing.T) {
	isolateDesktopReservation(t)
	previous := acquireDesktopInstance
	t.Cleanup(func() { acquireDesktopInstance = previous })
	path := filepath.Join(t.TempDir(), "flag.log")
	acquireDesktopInstance = func(_ string, _ func()) (desktopReservation, error) {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("arquivo aberto antes da reserva: %v", err)
		}
		return nil, desktopinstance.ErrAlreadyRunning
	}
	if code := run([]string{"assistente", "--log-file", path}); code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("arquivo criado para instância secundária: %v", err)
	}
}

func TestRunGravaFalhasAntesDaReservaNaFlag(t *testing.T) {
	for _, stage := range []string{"banco", "reserva"} {
		t.Run(stage, func(t *testing.T) {
			isolateDesktopReservation(t)
			previousResolve, previousAcquire, previousNative := resolveDesktopDatabase, acquireDesktopInstance, showNativeFatalError
			t.Cleanup(func() {
				resolveDesktopDatabase, acquireDesktopInstance, showNativeFatalError = previousResolve, previousAcquire, previousNative
			})
			showNativeFatalError = func(string, string) {}
			if stage == "banco" {
				resolveDesktopDatabase = func() (string, error) { return "", errors.New("falha antes da reserva") }
			} else {
				acquireDesktopInstance = func(string, func()) (desktopReservation, error) { return nil, errors.New("falha antes da reserva") }
			}
			path := filepath.Join(t.TempDir(), "flag.log")
			if code := run([]string{"assistente", "--log-file", path}); code != 1 {
				t.Fatalf("exit code = %d", code)
			}
			data, err := os.ReadFile(path)
			if err != nil || !strings.Contains(string(data), "falha antes da reserva") {
				t.Fatalf("log = %q, erro = %v", data, err)
			}
		})
	}
}

func TestRunSemDiretorioDoUsuarioContinuaComFlagDeLog(t *testing.T) {
	isolateDesktopReservation(t)
	previousDir, previousRun := logUserDirectory, runDesktop
	t.Cleanup(func() { logUserDirectory, runDesktop = previousDir, previousRun })
	logUserDirectory = func() string { return "" }
	runDesktop = func(*options.App) error { return nil }
	path := filepath.Join(t.TempDir(), "flag.log")
	if code := run([]string{"assistente", "--log-file", path}); code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("flag não abriu o arquivo: %v", err)
	}
}

func TestRunSemDiretorioDoUsuarioContinuaSemLogDeArquivo(t *testing.T) {
	isolateDesktopReservation(t)
	previousDir, previousRun := logUserDirectory, runDesktop
	t.Cleanup(func() { logUserDirectory, runDesktop = previousDir, previousRun })
	logUserDirectory = func() string { return "" }
	started := false
	runDesktop = func(*options.App) error { started = true; return nil }
	if code := run([]string{"assistente"}); code != 0 || !started {
		t.Fatalf("exit code = %d, runtime iniciado = %t", code, started)
	}
}

func TestRunRemoveFlagPropagaStartupErrorEFechaLog(t *testing.T) {
	isolateDesktopReservation(t)
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
	showNativeFatalError = func(_ string, message string) {
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

func TestRunWorkspaceArgumentPreservesDatabaseResolutionAndCWD(t *testing.T) {
	isolateDesktopReservation(t)
	launcher := t.TempDir()
	project := t.TempDir()
	t.Chdir(launcher)
	previousRun, previousResolve := runDesktop, resolveDesktopDatabase
	t.Cleanup(func() { runDesktop, resolveDesktopDatabase = previousRun, previousResolve })
	resolveCalls := 0
	resolveDesktopDatabase = func() (string, error) {
		resolveCalls++
		if cwd, _ := os.Getwd(); cwd != launcher {
			t.Fatalf("database resolution cwd changed: %q", cwd)
		}
		return previousResolve()
	}
	runDesktop = func(*options.App) error {
		if !slices.Equal(os.Args, []string{"assistente", "--debug"}) {
			t.Fatalf("workspace leaked to Wails: %v", os.Args)
		}
		if cwd, _ := os.Getwd(); cwd != launcher {
			t.Fatalf("runtime cwd changed: %q", cwd)
		}
		return nil
	}
	logPath := filepath.Join(launcher, "startup.log")
	if code := run([]string{"assistente", project, "--log-file", logPath, "--debug"}); code != 0 {
		t.Fatalf("exit = %d", code)
	}
	if resolveCalls != 1 {
		t.Fatalf("database resolved %d times", resolveCalls)
	}
	if _, err := os.Stat(logPath); err != nil {
		t.Fatal(err)
	}
}

func TestRunPropagaErroDoRunnerEFechaLog(t *testing.T) {
	isolateDesktopReservation(t)
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
	showNativeFatalError = func(_ string, message string) {
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

func TestRunReportaArquivoDeLogInacessivelAntesDoWails(t *testing.T) {
	isolateDesktopReservation(t)
	previousRunDesktop := runDesktop
	previousNativeError := showNativeFatalError
	previousLocaleProvider := startupLocaleProvider
	t.Cleanup(func() {
		runDesktop = previousRunDesktop
		showNativeFatalError = previousNativeError
		startupLocaleProvider = previousLocaleProvider
	})
	startupLocaleProvider = func() string { return "pt-BR" }

	runnerCalled := false
	runDesktop = func(*options.App) error {
		runnerCalled = true
		return nil
	}
	var nativeError string
	showNativeFatalError = func(_ string, message string) {
		nativeError = message
	}

	directoryPath := t.TempDir()
	exitCode := run([]string{"assistente", "--log-file", directoryPath})

	if exitCode != 2 {
		t.Fatalf("exitCode = %d, esperado 2", exitCode)
	}
	if runnerCalled {
		t.Fatal("Wails foi executado apesar da falha ao abrir o arquivo")
	}
	if !strings.Contains(nativeError, "Não foi possível abrir o arquivo de log") ||
		!strings.Contains(nativeError, directoryPath) {
		t.Fatalf("erro nativo não explica a falha: %q", nativeError)
	}
}
