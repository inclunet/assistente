package app

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"assistente/internal/configdir"
)

// TestMain mantém os testes do App fora dos diretórios de configuração do
// usuário. O isolamento é apenas do processo do binário de teste: o ambiente
// original é restaurado antes de sair e nenhuma configuração real é usada
// como fallback.
func TestMain(m *testing.M) {
	tmpDir, err := os.MkdirTemp("", "app-test-*")
	if err != nil {
		fmt.Fprintf(os.Stderr, "erro ao criar diretório temporário de teste: %v\n", err)
		os.Exit(1)
	}

	envNames := []string{
		"ASSISTENTE_HOME",
		"HOME",
		"USERPROFILE",
		"APPDATA",
		"LOCALAPPDATA",
		"XDG_CONFIG_HOME",
		"XDG_DATA_HOME",
		"XDG_STATE_HOME",
		"XDG_CACHE_HOME",
	}
	type envValue struct {
		value string
		set   bool
	}
	previous := make(map[string]envValue, len(envNames))
	for _, name := range envNames {
		value, set := os.LookupEnv(name)
		previous[name] = envValue{value: value, set: set}
	}

	originalCWD, err := os.Getwd()
	if err != nil {
		_ = os.RemoveAll(tmpDir)
		fmt.Fprintf(os.Stderr, "erro ao obter diretório de trabalho do teste: %v\n", err)
		os.Exit(1)
	}

	restoreEnv := func() {
		for _, name := range envNames {
			old := previous[name]
			if old.set {
				_ = os.Setenv(name, old.value)
			} else {
				_ = os.Unsetenv(name)
			}
		}
	}
	setSandboxEnv := func() error {
		for _, name := range envNames {
			if err := os.Setenv(name, tmpDir); err != nil {
				return fmt.Errorf("set %s: %w", name, err)
			}
		}
		return nil
	}
	if err := setSandboxEnv(); err != nil {
		restoreEnv()
		_ = os.RemoveAll(tmpDir)
		fmt.Fprintf(os.Stderr, "erro ao preparar ambiente temporário de teste: %v\n", err)
		os.Exit(1)
	}
	configdir.ResetForTests()

	code := m.Run()

	configdir.ResetForTests()
	restoreEnv()
	if err := os.Chdir(originalCWD); err != nil {
		fmt.Fprintf(os.Stderr, "erro ao restaurar diretório de trabalho após testes: %v\n", err)
		if code == 0 {
			code = 1
		}
	}
	_ = os.RemoveAll(tmpDir)
	os.Exit(code)
}

func TestTestMainUsesSandboxedConfigHome(t *testing.T) {
	expected := filepath.Join(os.Getenv("HOME"), ".assistente")
	if got := filepath.Clean(configdir.GetHomeDir()); got != filepath.Clean(expected) {
		t.Fatalf("config home = %q, want sandbox path %q", got, expected)
	}
}
