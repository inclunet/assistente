package profiles

import (
	"assistente/internal/configdir"
	"os"
	"testing"
)

func setupProfileTestEnv(t *testing.T) *Manager {
	t.Helper()

	tempDir := t.TempDir()
	oldHome := os.Getenv("HOME")
	oldUserProfile := os.Getenv("USERPROFILE")
	oldCwd, _ := os.Getwd()

	if err := os.Setenv("HOME", tempDir); err != nil {
		t.Fatalf("set HOME: %v", err)
	}
	if err := os.Setenv("USERPROFILE", tempDir); err != nil {
		t.Fatalf("set USERPROFILE: %v", err)
	}
	if err := os.Chdir(tempDir); err != nil {
		t.Fatalf("chdir: %v", err)
	}

	configdir.ResetForTests()

	t.Cleanup(func() {
		_ = os.Chdir(oldCwd)
		_ = os.Setenv("HOME", oldHome)
		_ = os.Setenv("USERPROFILE", oldUserProfile)
		configdir.ResetForTests()
	})

	return NewManager()
}

func TestManagerGetActiveSlug(t *testing.T) {
	manager := setupProfileTestEnv(t)

	p1 := DefaultProfile()
	p1.Name = "Perfil Um"
	p1.Active = false
	if _, err := manager.Create(p1); err != nil {
		t.Fatalf("create p1: %v", err)
	}

	p2 := DefaultProfile()
	p2.Name = "Perfil Dois"
	p2.Active = true
	slug2, err := manager.Create(p2)
	if err != nil {
		t.Fatalf("create p2: %v", err)
	}

	if got := manager.GetActiveSlug(); got != slug2 {
		t.Fatalf("GetActiveSlug: got %s, want %s", got, slug2)
	}
}

func TestManagerGetActiveSlugFallback(t *testing.T) {
	manager := setupProfileTestEnv(t)

	if got := manager.GetActiveSlug(); got != "padrao" {
		t.Fatalf("GetActiveSlug fallback: got %s, want padrao", got)
	}
}

func TestManagerGetActiveSlugUsesActiveFlag(t *testing.T) {
	manager := setupProfileTestEnv(t)

	p1 := DefaultProfile()
	p1.Name = "Perfil A"
	p1.Active = true
	slug1, err := manager.Create(p1)
	if err != nil {
		t.Fatalf("create p1: %v", err)
	}

	p2 := DefaultProfile()
	p2.Name = "Perfil B"
	p2.Active = false
	if _, err := manager.Create(p2); err != nil {
		t.Fatalf("create p2: %v", err)
	}

	if got := manager.GetActiveSlug(); got != slug1 {
		t.Fatalf("GetActiveSlug (active flag): got %s, want %s", got, slug1)
	}
}

// --- GetActive() returns full Profile with correct fallback behavior ---

func TestManagerGetActive_ReturnsActiveProfile(t *testing.T) {
	manager := setupProfileTestEnv(t)

	p := DefaultProfile()
	p.Name = "Meu Perfil"
	p.Active = true
	p.Chat.LLMProvider = "my-provider"
	p.Chat.Model = "my-model"
	_, err := manager.Create(p)
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	active, err := manager.GetActive()
	if err != nil {
		t.Fatalf("GetActive: %v", err)
	}
	if active.Name != "Meu Perfil" {
		t.Errorf("Name: got %q, want 'Meu Perfil'", active.Name)
	}
	if active.Chat.LLMProvider != "my-provider" {
		t.Errorf("LLMProvider: got %q, want 'my-provider'", active.Chat.LLMProvider)
	}
}

func TestManagerGetActive_FallsBackToFirstProfile(t *testing.T) {
	manager := setupProfileTestEnv(t)

	// Create two profiles, none marked Active
	p1 := DefaultProfile()
	p1.Name = "Primeiro"
	p1.Active = false
	_, err := manager.Create(p1)
	if err != nil {
		t.Fatalf("create p1: %v", err)
	}

	p2 := DefaultProfile()
	p2.Name = "Segundo"
	p2.Active = false
	_, err = manager.Create(p2)
	if err != nil {
		t.Fatalf("create p2: %v", err)
	}

	active, err := manager.GetActive()
	if err != nil {
		t.Fatalf("GetActive: %v", err)
	}
	// Should return the first profile found (fallback)
	if active == nil {
		t.Fatal("GetActive returned nil")
	}
	if active.Active {
		t.Error("fallback profile should not be marked Active")
	}
}

func TestManagerGetActive_NoProfilesReturnsDefaultWithSentinel(t *testing.T) {
	manager := setupProfileTestEnv(t)

	// No profiles created — GetActive should return DefaultProfile()
	active, err := manager.GetActive()
	if err != nil {
		t.Fatalf("GetActive: %v", err)
	}
	if active == nil {
		t.Fatal("GetActive returned nil")
	}
	if active.Name != "Padrão" {
		t.Errorf("Name: got %q, want 'Padrão'", active.Name)
	}
	if active.Chat.LLMProvider != DefaultProviderSentinel {
		t.Errorf("LLMProvider: got %q, want %q", active.Chat.LLMProvider, DefaultProviderSentinel)
	}
	if active.Chat.Model != DefaultProviderSentinel {
		t.Errorf("Model: got %q, want %q", active.Chat.Model, DefaultProviderSentinel)
	}
	if active.Voice.Assistant.LLMProviderID != DefaultProviderSentinel {
		t.Errorf("Voice.Assistant.LLMProviderID: got %q, want %q", active.Voice.Assistant.LLMProviderID, DefaultProviderSentinel)
	}
	if active.Input.LLMProviderID != DefaultProviderSentinel {
		t.Errorf("Input.LLMProviderID: got %q, want %q", active.Input.LLMProviderID, DefaultProviderSentinel)
	}
}

func TestManagerGetActive_ActiveProfileRetainsSentinel(t *testing.T) {
	manager := setupProfileTestEnv(t)

	// Create a profile that uses $default (as builtin profiles do)
	p := DefaultProfile()
	p.Name = "Builtin Style"
	p.Active = true
	_, err := manager.Create(p)
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	active, err := manager.GetActive()
	if err != nil {
		t.Fatalf("GetActive: %v", err)
	}
	if active.Chat.LLMProvider != DefaultProviderSentinel {
		t.Errorf("LLMProvider should be sentinel, got %q", active.Chat.LLMProvider)
	}
	if active.Chat.Model != DefaultProviderSentinel {
		t.Errorf("Model should be sentinel, got %q", active.Chat.Model)
	}
}

func TestManagerGetActive_PrefersPadraoOverArbitraryFallback(t *testing.T) {
	manager := setupProfileTestEnv(t)

	// Create profiles simulating builtin install: editor-texto, padrao, programacao
	// None marked Active — GetActive should prefer "padrao" over random
	editor := DefaultProfile()
	editor.Name = "Editor de Texto"
	editor.Active = false
	if _, err := manager.Create(editor); err != nil {
		t.Fatalf("create editor: %v", err)
	}

	padrao := DefaultProfile()
	padrao.Name = "Padrão"
	padrao.Active = false
	slug, err := manager.Create(padrao)
	if err != nil {
		t.Fatalf("create padrao: %v", err)
	}
	if slug != "padrao" {
		t.Fatalf("expected slug 'padrao', got %q", slug)
	}

	programacao := DefaultProfile()
	programacao.Name = "Programação"
	programacao.Active = false
	if _, err := manager.Create(programacao); err != nil {
		t.Fatalf("create programacao: %v", err)
	}

	active, err := manager.GetActive()
	if err != nil {
		t.Fatalf("GetActive: %v", err)
	}
	if active.Name != "Padrão" {
		t.Errorf("GetActive fallback: got %q, want 'Padrão'", active.Name)
	}
}

func TestManagerGetActiveSlug_PrefersPadraoOverArbitraryFallback(t *testing.T) {
	manager := setupProfileTestEnv(t)

	// Same scenario for GetActiveSlug
	for _, name := range []string{"Editor de Texto", "Padrão", "Programação"} {
		p := DefaultProfile()
		p.Name = name
		p.Active = false
		if _, err := manager.Create(p); err != nil {
			t.Fatalf("create %s: %v", name, err)
		}
	}

	if got := manager.GetActiveSlug(); got != "padrao" {
		t.Fatalf("GetActiveSlug fallback: got %s, want padrao", got)
	}
}

func TestManagerGetActive_PadraoFallbackHasSentinelValues(t *testing.T) {
	manager := setupProfileTestEnv(t)

	// Create "padrao" with $default sentinels (as builtin does), not active
	padrao := DefaultProfile()
	padrao.Active = false
	_, err := manager.Create(padrao)
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	active, err := manager.GetActive()
	if err != nil {
		t.Fatalf("GetActive: %v", err)
	}
	if active.Chat.LLMProvider != DefaultProviderSentinel {
		t.Errorf("LLMProvider: got %q, want %q", active.Chat.LLMProvider, DefaultProviderSentinel)
	}
	if active.Chat.Model != DefaultProviderSentinel {
		t.Errorf("Model: got %q, want %q", active.Chat.Model, DefaultProviderSentinel)
	}
	if active.Voice.Assistant.LLMProviderID != DefaultProviderSentinel {
		t.Errorf("Voice.Assistant.LLMProviderID: got %q, want %q", active.Voice.Assistant.LLMProviderID, DefaultProviderSentinel)
	}
	if active.Input.LLMProviderID != DefaultProviderSentinel {
		t.Errorf("Input.LLMProviderID: got %q, want %q", active.Input.LLMProviderID, DefaultProviderSentinel)
	}
}

func TestManagerSetActive_MarksProfileAndDeactivatesOthers(t *testing.T) {
	manager := setupProfileTestEnv(t)

	p1 := DefaultProfile()
	p1.Name = "Perfil A"
	p1.Active = true
	slug1, err := manager.Create(p1)
	if err != nil {
		t.Fatalf("create p1: %v", err)
	}

	p2 := DefaultProfile()
	p2.Name = "Perfil B"
	p2.Active = false
	slug2, err := manager.Create(p2)
	if err != nil {
		t.Fatalf("create p2: %v", err)
	}

	// Switch active to p2
	if err := manager.SetActive(slug2); err != nil {
		t.Fatalf("SetActive: %v", err)
	}

	active, err := manager.GetActive()
	if err != nil {
		t.Fatalf("GetActive after switch: %v", err)
	}
	if active.Name != "Perfil B" {
		t.Errorf("active after switch: got %q, want 'Perfil B'", active.Name)
	}

	// Verify p1 is no longer active
	oldP1, err := manager.Get(slug1)
	if err != nil {
		t.Fatalf("Get p1: %v", err)
	}
	if oldP1.Active {
		t.Error("p1 should no longer be active after SetActive(p2)")
	}
}

func TestManagerDuplicateProfile(t *testing.T) {
	manager := setupProfileTestEnv(t)

	profile := DefaultProfile()
	profile.Name = "Perfil Base"
	profile.Active = true
	originalSlug, err := manager.Create(profile)
	if err != nil {
		t.Fatalf("create profile: %v", err)
	}

	duplicateSlug, err := manager.Duplicate(originalSlug)
	if err != nil {
		t.Fatalf("duplicate profile: %v", err)
	}

	if duplicateSlug != Slugify("Perfil Base (Copia)") {
		t.Fatalf("duplicate slug: got %s, want %s", duplicateSlug, Slugify("Perfil Base (Copia)"))
	}

	duplicated, err := manager.Get(duplicateSlug)
	if err != nil {
		t.Fatalf("get duplicate: %v", err)
	}

	if duplicated.Name != "Perfil Base (Copia)" {
		t.Fatalf("duplicate name: got %s, want %s", duplicated.Name, "Perfil Base (Copia)")
	}
	if duplicated.Active {
		t.Fatalf("duplicate active: got true, want false")
	}
}
