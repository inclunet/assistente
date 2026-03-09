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
