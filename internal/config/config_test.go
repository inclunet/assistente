package config

import (
	"os"
	"testing"
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
	// Usa diretório temporário para config durante testes
	tmpDir, _ := os.MkdirTemp("", "config-test-*")
	_ = os.Setenv("ASSISTENTE_HOME", tmpDir)
	defer func() { _ = os.RemoveAll(tmpDir) }()
	defer func() { _ = os.Unsetenv("ASSISTENTE_HOME") }()

	os.Exit(m.Run())
}
