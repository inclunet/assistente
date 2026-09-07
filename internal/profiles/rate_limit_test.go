package profiles

import (
	"encoding/json"
	"testing"
)

func TestLegacyProfileUsesDefaultLLMRateLimit(t *testing.T) {
	var profile Profile
	if err := json.Unmarshal([]byte(`{"chat":{}}`), &profile); err != nil {
		t.Fatal(err)
	}

	if !profile.IsLLMRateLimitEnabled() {
		t.Fatal("perfil legado deve manter o rate limit habilitado")
	}
	if got := profile.GetLLMRateLimitRPM(); got != DefaultLLMRateLimitRPM {
		t.Fatalf("RPM efetivo = %d, want %d", got, DefaultLLMRateLimitRPM)
	}
	if got := profile.GetLLMRateLimitBurst(); got != DefaultLLMRateLimitBurst {
		t.Fatalf("burst efetivo = %d, want %d", got, DefaultLLMRateLimitBurst)
	}
}

func TestProfileCanDisableLLMRateLimit(t *testing.T) {
	disabled := false
	profile := DefaultProfile()
	profile.Chat.RateLimitEnabled = &disabled

	if profile.IsLLMRateLimitEnabled() {
		t.Fatal("configuração explícita false deve desabilitar o rate limit")
	}
}

func TestManagerPersistsLLMRateLimit(t *testing.T) {
	manager := setupProfileTestEnv(t)
	disabled := false
	profile := DefaultProfile()
	profile.Name = "Tarefas longas"
	profile.Chat.RateLimitEnabled = &disabled
	profile.Chat.RateLimitRPM = 240
	profile.Chat.RateLimitBurst = 120

	slug, err := manager.Create(profile)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	got, err := manager.Get(slug)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.IsLLMRateLimitEnabled() {
		t.Fatal("enabled = true, want false")
	}
	if got.Chat.RateLimitRPM != 240 || got.Chat.RateLimitBurst != 120 {
		t.Fatalf("config persistida = %d/%d, want 240/120", got.Chat.RateLimitRPM, got.Chat.RateLimitBurst)
	}
}

func TestProfileValidateRejectsInvalidLLMRateLimit(t *testing.T) {
	profile := DefaultProfile()
	profile.Chat.RateLimitRPM = MaxLLMRateLimitValue + 1
	if err := profile.Validate(); err == nil {
		t.Fatal("RPM acima do máximo deveria ser rejeitado")
	}

	profile = DefaultProfile()
	profile.Chat.RateLimitBurst = -1
	if err := profile.Validate(); err == nil {
		t.Fatal("burst negativo deveria ser rejeitado")
	}
}
