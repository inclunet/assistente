package config

import (
	"context"
	"errors"
	"os"
	"testing"
)

// ── Mocks ──────────────────────────────────────────────────

type mockEmitter struct{ events []string }

func (m *mockEmitter) Emit(event string, _ any) { m.events = append(m.events, event) }

type mockCredCleaner struct {
	visible []VisibleCredential
	deleted []string
	listErr error
	delErr  error
}

func (m *mockCredCleaner) ListVisible(_ context.Context) ([]VisibleCredential, error) {
	return m.visible, m.listErr
}

func (m *mockCredCleaner) DeletePattern(_ context.Context, pattern string) error {
	if m.delErr != nil {
		return m.delErr
	}
	m.deleted = append(m.deleted, pattern)
	return nil
}

type mockProfileCleaner struct {
	slugs   []string
	deleted []string
}

func (m *mockProfileCleaner) ListSlugs() ([]string, error) { return m.slugs, nil }
func (m *mockProfileCleaner) DeleteSlug(slug string) error {
	m.deleted = append(m.deleted, slug)
	return nil
}

type mockSkillCleaner struct {
	slugs   []string
	deleted []string
}

func (m *mockSkillCleaner) ListSlugs() ([]string, error) { return m.slugs, nil }
func (m *mockSkillCleaner) DeleteSlug(slug string) error {
	m.deleted = append(m.deleted, slug)
	return nil
}

// ── Tests ──────────────────────────────────────────────────

func TestSettingsService_SetChatModel(t *testing.T) {
	em := &mockEmitter{}
	reloaded := false
	svc := NewSettingsService(SettingsServiceConfig{
		Emitter:   em,
		ReloadLLM: func() { reloaded = true },
	})

	if err := svc.SetChatModel("gpt-4o"); err != nil {
		t.Fatalf("SetChatModel: %v", err)
	}

	if !reloaded {
		t.Error("esperava que ReloadLLM fosse chamado")
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.DefaultModel != "gpt-4o" {
		t.Errorf("DefaultModel = %q, want %q", cfg.DefaultModel, "gpt-4o")
	}
	if cfg.ChatParams.Model != "gpt-4o" {
		t.Errorf("ChatParams.Model = %q, want %q", cfg.ChatParams.Model, "gpt-4o")
	}
}

func TestSettingsService_SaveSettings(t *testing.T) {
	em := &mockEmitter{}
	svc := NewSettingsService(SettingsServiceConfig{Emitter: em})

	err := svc.SaveSettings(SettingsInput{
		APIKey:     "test-key",
		APIBaseURL: "https://example.com/v1",
		ChatParams: SettingsModelParams{
			Model:       "claude-3",
			Temperature: 0.5,
			MaxTokens:   2048,
		},
	})
	if err != nil {
		t.Fatalf("SaveSettings: %v", err)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.APIKey != "test-key" {
		t.Errorf("APIKey = %q, want %q", cfg.APIKey, "test-key")
	}
	if cfg.ResponseTimeout != 180 {
		t.Errorf("ResponseTimeout = %d, want 180 (default)", cfg.ResponseTimeout)
	}
}

func TestSettingsService_SetDefaultModel(t *testing.T) {
	em := &mockEmitter{}
	svc := NewSettingsService(SettingsServiceConfig{Emitter: em})

	if err := svc.SetDefaultModel("llama-3"); err != nil {
		t.Fatalf("SetDefaultModel: %v", err)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.DefaultModel != "llama-3" {
		t.Errorf("DefaultModel = %q, want %q", cfg.DefaultModel, "llama-3")
	}
}

func TestSettingsService_ResetConfig(t *testing.T) {
	em := &mockEmitter{}
	svc := NewSettingsService(SettingsServiceConfig{Emitter: em})

	// Cria um config para poder resetar
	_ = svc.SetDefaultModel("temp-model")

	err := svc.ResetConfig()
	if err != nil {
		t.Fatalf("ResetConfig: %v", err)
	}

	// Verifica que o config voltou ao padrão
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load after reset: %v", err)
	}
	if cfg.DefaultModel != "gpt-4o-mini" {
		t.Errorf("DefaultModel após reset = %q, want %q (default)", cfg.DefaultModel, "gpt-4o-mini")
	}
}

func TestSettingsService_ClearAllCredentials(t *testing.T) {
	em := &mockEmitter{}
	cred := &mockCredCleaner{visible: []VisibleCredential{
		{Pattern: "api.openai.com"},
		{Pattern: "api.anthropic.com"},
	}}
	svc := NewSettingsService(SettingsServiceConfig{
		Emitter:     em,
		CredCleaner: cred,
	})

	if err := svc.ClearAllCredentials(context.Background()); err != nil {
		t.Fatalf("ClearAllCredentials: %v", err)
	}

	if len(cred.deleted) != 2 {
		t.Fatalf("esperava 2 deletes, obteve %d (%v)", len(cred.deleted), cred.deleted)
	}
	want := map[string]bool{"api.openai.com": true, "api.anthropic.com": true}
	for _, p := range cred.deleted {
		if !want[p] {
			t.Errorf("pattern inesperado deletado: %q", p)
		}
	}
	if len(em.events) == 0 || em.events[0] != "credentials:cleared" {
		t.Errorf("evento emitido = %v, want [credentials:cleared]", em.events)
	}
}

func TestSettingsService_ClearAllCredentials_NilCleaner(t *testing.T) {
	svc := NewSettingsService(SettingsServiceConfig{Emitter: &mockEmitter{}})

	err := svc.ClearAllCredentials(context.Background())
	if err == nil {
		t.Error("esperava erro quando credCleaner é nil")
	}
}

// TestSettingsService_ClearAllCredentials_NeverPassesEmptyPattern
// documenta o contrato: o cleaner JAMAIS recebe `pattern=""`, mesmo
// quando a lista visível inclui uma entrada degenerada.
func TestSettingsService_ClearAllCredentials_NeverPassesEmptyPattern(t *testing.T) {
	em := &mockEmitter{}
	cred := &mockCredCleaner{visible: []VisibleCredential{
		{Pattern: ""}, // entrada degenerada — service tem que ignorar.
		{Pattern: "api.openai.com"},
	}}
	svc := NewSettingsService(SettingsServiceConfig{Emitter: em, CredCleaner: cred})

	if err := svc.ClearAllCredentials(context.Background()); err != nil {
		t.Fatalf("ClearAllCredentials: %v", err)
	}

	for _, p := range cred.deleted {
		if p == "" {
			t.Fatalf("ClearAllCredentials passou pattern vazio para o cleaner")
		}
	}
	if len(cred.deleted) != 1 || cred.deleted[0] != "api.openai.com" {
		t.Fatalf("esperava apenas api.openai.com deletado, obteve %v", cred.deleted)
	}
}

// TestSettingsService_ClearAllCredentials_StopsOnDeleteErr garante que
// um erro no meio do mass-delete não é engolido — o caller precisa saber
// que a operação não terminou. Isso evita o cenário "log de sucesso,
// metade das credenciais sobreviveu" que mascarou o incident original.
func TestSettingsService_ClearAllCredentials_StopsOnDeleteErr(t *testing.T) {
	cred := &mockCredCleaner{
		visible: []VisibleCredential{{Pattern: "a"}, {Pattern: "b"}},
		delErr:  errors.New("boom"),
	}
	svc := NewSettingsService(SettingsServiceConfig{Emitter: &mockEmitter{}, CredCleaner: cred})

	if err := svc.ClearAllCredentials(context.Background()); err == nil {
		t.Fatal("esperava erro propagado quando DeletePattern falha")
	}
}

func TestSettingsService_ClearAllProfiles(t *testing.T) {
	em := &mockEmitter{}
	prof := &mockProfileCleaner{slugs: []string{"perfil-1", "perfil-2"}}
	svc := NewSettingsService(SettingsServiceConfig{
		Emitter:        em,
		ProfileCleaner: prof,
	})

	if err := svc.ClearAllProfiles(); err != nil {
		t.Fatalf("ClearAllProfiles: %v", err)
	}

	if len(prof.deleted) != 2 {
		t.Errorf("deletados %d perfis, want 2", len(prof.deleted))
	}
	if len(em.events) == 0 || em.events[0] != "profiles:cleared" {
		t.Errorf("evento emitido = %v, want [profiles:cleared]", em.events)
	}
}

func TestSettingsService_ClearAllSkills(t *testing.T) {
	em := &mockEmitter{}
	sk := &mockSkillCleaner{slugs: []string{"skill-a", "skill-b", "skill-c"}}
	svc := NewSettingsService(SettingsServiceConfig{
		Emitter:      em,
		SkillCleaner: sk,
	})

	if err := svc.ClearAllSkills(); err != nil {
		t.Fatalf("ClearAllSkills: %v", err)
	}

	if len(sk.deleted) != 3 {
		t.Errorf("deletados %d skills, want 3", len(sk.deleted))
	}
	if len(em.events) == 0 || em.events[0] != "skills:cleared" {
		t.Errorf("evento emitido = %v, want [skills:cleared]", em.events)
	}
}

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
