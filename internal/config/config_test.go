package config

import (
	"errors"
	"fmt"
	"os"
	"testing"

	"assistente/internal/configdir"
)

// writeConfigJSON grava um config.json bruto no diretório de teste e agenda a
// remoção. Permite exercitar objetos `maintenance` parciais (AEP-0074).
func writeConfigJSON(t *testing.T, raw string) {
	t.Helper()
	path, err := GetConfigPath()
	if err != nil {
		t.Fatalf("GetConfigPath: %v", err)
	}
	if err := os.WriteFile(path, []byte(raw), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	t.Cleanup(func() { _ = os.Remove(path) })
}

// Um objeto `maintenance` parcial deve preservar os defaults dos campos ausentes
// (loadUnsafe parte de DefaultConfig e json.Unmarshal só sobrescreve as chaves
// presentes) — não cair em 0 (AEP-0074).
func TestGetMaintenance_PartialJSONPreservesDefaults(t *testing.T) {
	writeConfigJSON(t, `{"maintenance":{"job_retention_hours":48}}`)

	m, err := GetMaintenance()
	if err != nil {
		t.Fatalf("GetMaintenance: %v", err)
	}
	if m.JobRetentionHours != 48 {
		t.Errorf("job_retention_hours = %d, want 48", m.JobRetentionHours)
	}
	if m.RunsPerJobKeep != DefaultRunsPerJobKeep {
		t.Errorf("runs_per_job_keep = %d, want default %d", m.RunsPerJobKeep, DefaultRunsPerJobKeep)
	}
	if m.VacuumMinFreeBytes != DefaultVacuumMinFreeBytes {
		t.Errorf("vacuum_min_free_bytes = %d, want default %d", m.VacuumMinFreeBytes, DefaultVacuumMinFreeBytes)
	}
	if m.ChatToolCallsRetentionDays != DefaultChatToolCallsRetentionDays {
		t.Errorf("chat_tool_calls_retention_days = %d, want default %d", m.ChatToolCallsRetentionDays, DefaultChatToolCallsRetentionDays)
	}
	if m.CommandInvocationRetentionDays != DefaultCommandInvocationRetentionDays ||
		m.CommandInvocationsPerUserKeep != DefaultCommandInvocationsPerUserKeep ||
		m.CommandInvocationsSystemKeep != DefaultCommandInvocationsSystemKeep ||
		m.CommandActivationTerminalRetentionDays != DefaultCommandActivationTerminalRetentionDays ||
		m.CommandActivationTerminalKeepPerUser != DefaultCommandActivationTerminalKeepPerUser ||
		m.CommandJobActivationLeaseSeconds != DefaultCommandJobActivationLeaseSeconds {
		t.Errorf("command maintenance defaults = %+v", m)
	}
}

// Um objeto `maintenance` vazio mantém todos os defaults.
func TestGetMaintenance_EmptyObjectKeepsDefaults(t *testing.T) {
	writeConfigJSON(t, `{"maintenance":{}}`)

	m, err := GetMaintenance()
	if err != nil {
		t.Fatalf("GetMaintenance: %v", err)
	}
	if m != DefaultMaintenanceSettings() {
		t.Errorf("maintenance = %+v, want defaults %+v", m, DefaultMaintenanceSettings())
	}
}

// Valores 0 EXPLÍCITOS são escolhas do usuário e devem ser respeitados:
// runs_per_job_keep=0 desativa o cap; vacuum_min_free_bytes=0 = sempre compacta.
func TestGetMaintenance_ExplicitZeroIsRespected(t *testing.T) {
	writeConfigJSON(t, `{"maintenance":{"runs_per_job_keep":0,"vacuum_min_free_bytes":0}}`)

	m, err := GetMaintenance()
	if err != nil {
		t.Fatalf("GetMaintenance: %v", err)
	}
	if m.RunsPerJobKeep != 0 {
		t.Errorf("runs_per_job_keep = %d, want 0 (cap desativado explicitamente)", m.RunsPerJobKeep)
	}
	if m.VacuumMinFreeBytes != 0 {
		t.Errorf("vacuum_min_free_bytes = %d, want 0 (sempre compacta)", m.VacuumMinFreeBytes)
	}
	// Campo ausente continua no default.
	if m.JobRetentionHours != DefaultJobRetentionHours {
		t.Errorf("job_retention_hours = %d, want default %d", m.JobRetentionHours, DefaultJobRetentionHours)
	}
}

func TestGetMaintenance_CommandFieldsNormalizeNonPositiveValues(t *testing.T) {
	writeConfigJSON(t, `{"maintenance":{"command_invocation_retention_days":0,"command_invocations_per_user_keep":-1,"command_invocations_system_keep":0,"command_activation_terminal_retention_days":-1,"command_activation_terminal_keep_per_user":0,"command_job_activation_lease_seconds":-1}}`)

	m, err := GetMaintenance()
	if err != nil {
		t.Fatal(err)
	}
	want := DefaultMaintenanceSettings()
	if m.CommandInvocationRetentionDays != want.CommandInvocationRetentionDays ||
		m.CommandInvocationsPerUserKeep != want.CommandInvocationsPerUserKeep ||
		m.CommandInvocationsSystemKeep != want.CommandInvocationsSystemKeep ||
		m.CommandActivationTerminalRetentionDays != want.CommandActivationTerminalRetentionDays ||
		m.CommandActivationTerminalKeepPerUser != want.CommandActivationTerminalKeepPerUser ||
		m.CommandJobActivationLeaseSeconds != want.CommandJobActivationLeaseSeconds {
		t.Errorf("valores não positivos não normalizados: got %+v want %+v", m, want)
	}
}

func TestSaveMaintenance_CommandFieldsRoundTrip(t *testing.T) {
	want := DefaultMaintenanceSettings()
	want.CommandInvocationRetentionDays = 91
	want.CommandInvocationsPerUserKeep = 12001
	want.CommandInvocationsSystemKeep = 2001
	want.CommandActivationTerminalRetentionDays = 73
	want.CommandActivationTerminalKeepPerUser = 14001
	want.CommandJobActivationLeaseSeconds = 257

	if err := SaveMaintenance(want); err != nil {
		t.Fatalf("SaveMaintenance: %v", err)
	}

	got, err := GetMaintenance()
	if err != nil {
		t.Fatalf("GetMaintenance: %v", err)
	}
	if got != want {
		t.Fatalf("maintenance after save/load = %+v, want %+v", got, want)
	}
}

func TestLoad_MissingConfigUsesDefaults(t *testing.T) {
	path, err := GetConfigPath()
	if err != nil {
		t.Fatalf("GetConfigPath: %v", err)
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("remove config fixture: %v", err)
	}

	got, err := Load()
	if err != nil {
		t.Fatalf("Load missing config: %v", err)
	}
	if got == nil || got.Maintenance != DefaultMaintenanceSettings() {
		t.Fatalf("missing config = %+v, want defaults", got)
	}
}

func TestLoad_ReadErrorIsNotTreatedAsMissing(t *testing.T) {
	path, err := GetConfigPath()
	if err != nil {
		t.Fatalf("GetConfigPath: %v", err)
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("remove config fixture: %v", err)
	}
	if err := os.Mkdir(path, 0o700); err != nil {
		t.Fatalf("create unreadable config fixture: %v", err)
	}
	t.Cleanup(func() { _ = os.Remove(path) })

	got, err := Load()
	if err == nil {
		t.Fatalf("Load directory config = %+v, want read error", got)
	}
	if errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Load directory config reported not-exist: %v", err)
	}
}

func TestMain(m *testing.M) {
	// O resolver de configuração usa os diretórios derivados do ambiente do
	// processo; ASSISTENTE_HOME, sozinho, não o isola. Este harness isola
	// também o cwd e usa um ResolverWithBase apontando para o diretório temporário,
	// restaura cada variável (inclusive unset) e nunca executa os testes se a
	// sandbox não puder ser preparada.
	tmpDir, err := os.MkdirTemp("", "config-test-*")
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
	previousWD, err := os.Getwd()
	if err != nil {
		restoreEnv()
		_ = os.RemoveAll(tmpDir)
		fmt.Fprintf(os.Stderr, "erro ao obter cwd de teste: %v\n", err)
		os.Exit(1)
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
	if err := os.Chdir(tmpDir); err != nil {
		restoreEnv()
		_ = os.RemoveAll(tmpDir)
		fmt.Fprintf(os.Stderr, "erro ao isolar cwd de teste: %v\n", err)
		os.Exit(1)
	}
	configdir.ResetForTests()
	rootResolver = configdir.NewResolverWithBase(configdir.GetHomeDir())

	code := m.Run()

	configdir.ResetForTests()
	_ = os.Chdir(previousWD)
	restoreEnv()
	_ = os.RemoveAll(tmpDir)
	os.Exit(code)
}
