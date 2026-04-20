package config

import (
	"context"
	"os"
	"testing"
)

// ── Mocks ──────────────────────────────────────────────────

type mockEmitter struct{ events []string }

func (m *mockEmitter) Emit(event string, _ any) { m.events = append(m.events, event) }

type mockCredCleaner struct{ called bool }

func (m *mockCredCleaner) DeletePattern(_ context.Context, _ string) error {
	m.called = true
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
	cred := &mockCredCleaner{}
	svc := NewSettingsService(SettingsServiceConfig{
		Emitter:     em,
		CredCleaner: cred,
	})

	if err := svc.ClearAllCredentials(context.Background()); err != nil {
		t.Fatalf("ClearAllCredentials: %v", err)
	}

	if !cred.called {
		t.Error("esperava que DeletePattern fosse chamado")
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

func TestMain(m *testing.M) {
	// Usa diretório temporário para config durante testes
	tmpDir, _ := os.MkdirTemp("", "config-test-*")
	_ = os.Setenv("ASSISTENTE_HOME", tmpDir)
	defer func() { _ = os.RemoveAll(tmpDir) }()
	defer func() { _ = os.Unsetenv("ASSISTENTE_HOME") }()

	os.Exit(m.Run())
}
