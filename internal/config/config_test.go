package config

import (
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

func TestMain(m *testing.M) {
	// O resolver de configuração usa os diretórios derivados do ambiente do
	// processo; ASSISTENTE_HOME, sozinho, não o isola. Este harness altera
	// somente o processo de teste, restaura cada variável (inclusive unset) e
	// nunca executa os testes se a sandbox não puder ser preparada.
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
	_ = os.RemoveAll(tmpDir)
	os.Exit(code)
}
