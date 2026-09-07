package app

import (
	"assistente/internal/configdir"
	"assistente/internal/profiles"
	"context"
	"os"
	"testing"
)

func TestRateLimitPolicyResolverUsesCurrentProfileAndEffectiveSlug(t *testing.T) {
	tempDir := t.TempDir()
	oldHome := os.Getenv("HOME")
	oldUserProfile := os.Getenv("USERPROFILE")
	oldCwd, _ := os.Getwd()
	if err := os.Setenv("HOME", tempDir); err != nil {
		t.Fatal(err)
	}
	if err := os.Setenv("USERPROFILE", tempDir); err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(tempDir); err != nil {
		t.Fatal(err)
	}
	configdir.ResetForTests()
	t.Cleanup(func() {
		_ = os.Chdir(oldCwd)
		_ = os.Setenv("HOME", oldHome)
		_ = os.Setenv("USERPROFILE", oldUserProfile)
		configdir.ResetForTests()
	})

	manager := profiles.NewManager()
	disabled := false
	profile := profiles.DefaultProfile()
	profile.Name = "Tarefas longas"
	profile.Chat.RateLimitEnabled = &disabled
	profile.Chat.RateLimitRPM = 240
	profile.Chat.RateLimitBurst = 120
	slug, err := manager.Create(profile)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := manager.SetActive(slug); err != nil {
		t.Fatalf("set active: %v", err)
	}

	resolve := newRateLimitPolicyResolver(manager)
	got := resolve(context.Background(), "perfil-removido")
	if got.ProfileSlug != slug {
		t.Fatalf("slug efetivo = %q, want %q", got.ProfileSlug, slug)
	}
	if got.Config.Enabled || got.Config.RequestsPerMinute != 240 || got.Config.Burst != 120 {
		t.Fatalf("política resolvida = %#v", got.Config)
	}

	profile.Chat.RateLimitEnabled = nil
	profile.Chat.RateLimitRPM = 360
	if err := manager.Update(slug, profile); err != nil {
		t.Fatalf("update: %v", err)
	}
	got = resolve(context.Background(), slug)
	if !got.Config.Enabled || got.Config.RequestsPerMinute != 360 {
		t.Fatalf("resolver deve reler a política salva: %#v", got.Config)
	}
}
