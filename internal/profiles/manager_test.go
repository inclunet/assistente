package profiles

import (
	"assistente/internal/configdir"
	"encoding/json"
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

// TestManagerNativeMCPPersistsTriState garante que o override tri-state de MCP
// nativo (Chat.NativeMCP) sobrevive ao Create/Get (persistência JSON). Cobre o
// caminho de sub-agentes também: eles carregam o mesmo Profile por slug, então
// o override precisa persistir para ser aplicado no run do sub-agente.
func TestManagerNativeMCPPersistsTriState(t *testing.T) {
	manager := setupProfileTestEnv(t)

	cases := []struct {
		name string
		val  *bool
	}{
		{"auto-nil", nil},
		{"forca-true", boolPtr(true)},
		{"forca-false", boolPtr(false)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := DefaultProfile()
			p.Name = "Perfil " + tc.name
			p.Active = false
			p.Chat.NativeMCP = tc.val
			slug, err := manager.Create(p)
			if err != nil {
				t.Fatalf("create: %v", err)
			}
			got, err := manager.Get(slug)
			if err != nil {
				t.Fatalf("get: %v", err)
			}
			switch {
			case tc.val == nil && got.Chat.NativeMCP != nil:
				t.Errorf("esperava NativeMCP nil (auto), obteve %v", *got.Chat.NativeMCP)
			case tc.val != nil && got.Chat.NativeMCP == nil:
				t.Errorf("esperava NativeMCP=%v, obteve nil", *tc.val)
			case tc.val != nil && *got.Chat.NativeMCP != *tc.val:
				t.Errorf("esperava NativeMCP=%v, obteve %v", *tc.val, *got.Chat.NativeMCP)
			}
		})
	}
}

func TestManagerContextProvidersPersistProfileOverrides(t *testing.T) {
	manager := setupProfileTestEnv(t)

	enabled := false
	p := DefaultProfile()
	p.Name = "Perfil Context Providers"
	p.ContextProviders = map[string]ContextProviderProfileConfig{
		"memory": {
			Enabled:  &enabled,
			Budget:   900,
			Settings: map[string]any{"mode": "pinned_plus_auto"},
		},
	}
	slug, err := manager.Create(p)
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	got, err := manager.Get(slug)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	cfg := got.ContextProviders["memory"]
	if cfg.Enabled == nil || *cfg.Enabled {
		t.Fatalf("Enabled = %v, want false", cfg.Enabled)
	}
	if cfg.Budget != 900 {
		t.Fatalf("Budget = %d, want 900", cfg.Budget)
	}
	if cfg.Settings["mode"] != "pinned_plus_auto" {
		t.Fatalf("Settings[mode] = %v, want pinned_plus_auto", cfg.Settings["mode"])
	}
}

func TestManagerPromptCachePersistsProfileConfig(t *testing.T) {
	manager := setupProfileTestEnv(t)

	p := DefaultProfile()
	p.Name = "Perfil Prompt Cache"
	p.Chat.PromptCache = PromptCacheConfig{
		Enabled:              true,
		ProviderHints:        true,
		ExplicitCacheControl: false,
	}
	slug, err := manager.Create(p)
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	got, err := manager.Get(slug)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if !got.Chat.PromptCache.Enabled {
		t.Fatal("PromptCache.Enabled = false, want true")
	}
	if !got.Chat.PromptCache.ProviderHints {
		t.Fatal("PromptCache.ProviderHints = false, want true")
	}
	if got.Chat.PromptCache.ExplicitCacheControl {
		t.Fatal("PromptCache.ExplicitCacheControl = true, want false")
	}
}

func TestManagerLLMDebugPersistsProfileConfig(t *testing.T) {
	manager := setupProfileTestEnv(t)

	p := DefaultProfile()
	p.Name = "Perfil Debug"
	p.Chat.Debug = &ChatDebugConfig{
		Enabled:       true,
		DumpRequests:  false,
		DumpResponses: false,
		MaxFiles:      25,
	}
	slug, err := manager.Create(p)
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	got, err := manager.Get(slug)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Chat.Debug == nil {
		t.Fatal("Debug = nil, want persisted config")
	}
	if !got.Chat.Debug.Enabled {
		t.Fatal("Debug.Enabled = false, want true")
	}
	if got.Chat.Debug.DumpRequests {
		t.Fatal("Debug.DumpRequests = true, want false")
	}
	if got.Chat.Debug.DumpResponses {
		t.Fatal("Debug.DumpResponses = true, want false")
	}
	if got.Chat.Debug.MaxFiles != 25 {
		t.Fatalf("Debug.MaxFiles = %d, want 25", got.Chat.Debug.MaxFiles)
	}
}

func TestChatConfigEffectiveDebugUsesDefaultsWhenLegacyMissing(t *testing.T) {
	var profile Profile
	if err := json.Unmarshal([]byte(`{
		"name": "Legacy",
		"chat": {
			"llm_provider": "$default",
			"model": "$default",
			"temperature": 0.7,
			"max_tokens": 4096,
			"top_p": 1,
			"response_timeout": 180
		}
	}`), &profile); err != nil {
		t.Fatalf("unmarshal legacy profile: %v", err)
	}
	if profile.Chat.Debug != nil {
		t.Fatalf("Debug = %#v, want nil before effective default", profile.Chat.Debug)
	}
	got := profile.Chat.EffectiveDebug()
	if got.Enabled {
		t.Fatal("EffectiveDebug.Enabled = true, want false")
	}
	if !got.DumpRequests || !got.DumpResponses {
		t.Fatalf("EffectiveDebug = %#v, want request/response defaults enabled", got)
	}
	if got.MaxFiles != 200 {
		t.Fatalf("EffectiveDebug.MaxFiles = %d, want 200", got.MaxFiles)
	}
}

func TestChatConfigEffectiveDebugMergesPartialDebugBlock(t *testing.T) {
	var profile Profile
	if err := json.Unmarshal([]byte(`{
		"name": "Partial Debug",
		"chat": {
			"llm_provider": "$default",
			"model": "$default",
			"temperature": 0.7,
			"max_tokens": 4096,
			"top_p": 1,
			"response_timeout": 180,
			"debug": { "enabled": true }
		}
	}`), &profile); err != nil {
		t.Fatalf("unmarshal partial debug profile: %v", err)
	}

	got := profile.Chat.EffectiveDebug()
	if !got.Enabled {
		t.Fatal("EffectiveDebug.Enabled = false, want true")
	}
	if !got.DumpRequests || !got.DumpResponses {
		t.Fatalf("EffectiveDebug = %#v, want request/response defaults merged", got)
	}
	if got.MaxFiles != 200 {
		t.Fatalf("EffectiveDebug.MaxFiles = %d, want 200", got.MaxFiles)
	}
}

func TestChatConfigEffectiveDebugPreservesExplicitDebugFalseValues(t *testing.T) {
	var profile Profile
	if err := json.Unmarshal([]byte(`{
		"name": "Explicit Debug",
		"chat": {
			"llm_provider": "$default",
			"model": "$default",
			"temperature": 0.7,
			"max_tokens": 4096,
			"top_p": 1,
			"response_timeout": 180,
			"debug": {
				"enabled": true,
				"dump_requests": false,
				"dump_responses": false,
				"max_files": 0
			}
		}
	}`), &profile); err != nil {
		t.Fatalf("unmarshal explicit debug profile: %v", err)
	}

	got := profile.Chat.EffectiveDebug()
	if !got.Enabled {
		t.Fatal("EffectiveDebug.Enabled = false, want true")
	}
	if got.DumpRequests || got.DumpResponses {
		t.Fatalf("EffectiveDebug = %#v, want explicit request/response false preserved", got)
	}
	if got.MaxFiles != 0 {
		t.Fatalf("EffectiveDebug.MaxFiles = %d, want explicit 0 preserved", got.MaxFiles)
	}
}

func TestProfileValidateRejectsNegativeContextProviderBudget(t *testing.T) {
	p := DefaultProfile()
	p.ContextProviders = map[string]ContextProviderProfileConfig{
		"memory": {Budget: -1},
	}

	if err := p.Validate(); err == nil {
		t.Fatal("Validate succeeded, want negative context provider budget error")
	}
}

func TestProfileValidateRejectsDebugMaxFilesOutOfRange(t *testing.T) {
	cases := []struct {
		name     string
		maxFiles int
	}{
		{name: "negative", maxFiles: -1},
		{name: "above limit", maxFiles: ChatDebugMaxFilesLimit + 1},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := DefaultProfile()
			p.Chat.Debug = &ChatDebugConfig{MaxFiles: tc.maxFiles}

			if err := p.Validate(); err == nil {
				t.Fatal("Validate succeeded, want debug.max_files range error")
			}
		})
	}
}

func TestProfileValidateRejectsPromptCacheControlsWhenDisabled(t *testing.T) {
	cases := []struct {
		name string
		cfg  PromptCacheConfig
	}{
		{
			name: "provider-hints",
			cfg:  PromptCacheConfig{ProviderHints: true},
		},
		{
			name: "explicit-cache-control",
			cfg:  PromptCacheConfig{ExplicitCacheControl: true},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := DefaultProfile()
			p.Chat.PromptCache = tc.cfg

			if err := p.Validate(); err == nil {
				t.Fatal("Validate succeeded, want prompt cache dependency error")
			}
		})
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
