package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"assistente/adapters/noop"
	"assistente/controllers"
	"assistente/internal/configdir"
	"assistente/internal/credentials"
	"assistente/internal/database"
	"assistente/internal/llm"
	"assistente/internal/profiles"
	"assistente/internal/providers"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func setupWizardTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("falha ao criar banco em memória: %v", err)
	}
	if err := db.AutoMigrate(&database.LLMProvider{}, &database.CredentialEntry{}); err != nil {
		t.Fatalf("falha ao migrar tabelas: %v", err)
	}
	database.SetDB(db)
	return db
}

func setupWizardTestApp(t *testing.T) *App {
	t.Helper()
	_ = setupWizardTestDB(t)

	credMgr := credentials.NewManager([]byte("test-key-exactly-32-bytes-long!!"))
	llmRegistry := llm.NewProviderRegistry()
	svc := providers.NewService(providers.ServiceConfig{
		Registry: llmRegistry,
		CredMgr:  credMgr,
		Store:    providers.NewDBStore(),
	})

	a := &App{
		ctx:         context.Background(),
		credMgr:     credMgr,
		llmRegistry: llmRegistry,
		providerSvc: svc,
	}
	a.llmCtrl = controllers.NewLLMController(controllers.LLMControllerConfig{
		LLMRegistry: llmRegistry,
		ProviderSvc: svc,
		Emitter:     &noop.EmitterAdapter{},
	})
	a.welcomeCtrl = controllers.NewWelcomeController(controllers.WelcomeControllerConfig{
		CredMgr:          credMgr,
		ProviderSvc:      svc,
		LLMRegistry:      llmRegistry,
		SaveLLMProviders: func() error { return svc.Save() },
	})
	return a
}

func setupWizardTestAppWithProfiles(t *testing.T) (*App, *profiles.Manager) {
	t.Helper()
	_ = setupWizardTestDB(t)

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

	credMgr := credentials.NewManager([]byte("test-key-exactly-32-bytes-long!!"))
	pm := profiles.NewManager()
	llmRegistry := llm.NewProviderRegistry()
	svc := providers.NewService(providers.ServiceConfig{
		Registry: llmRegistry,
		CredMgr:  credMgr,
		Store:    providers.NewDBStore(),
	})

	a := &App{
		ctx:            context.Background(),
		credMgr:        credMgr,
		llmRegistry:    llmRegistry,
		profileManager: pm,
		providerSvc:    svc,
	}
	a.llmCtrl = controllers.NewLLMController(controllers.LLMControllerConfig{
		LLMRegistry: llmRegistry,
		ProviderSvc: svc,
		Emitter:     &noop.EmitterAdapter{},
	})
	a.welcomeCtrl = controllers.NewWelcomeController(controllers.WelcomeControllerConfig{
		CredMgr:          credMgr,
		ProviderSvc:      svc,
		LLMRegistry:      llmRegistry,
		SaveLLMProviders: func() error { return svc.Save() },
	})
	return a, pm
}

// --- getWizardProviderInfo ---

func TestGetWizardProviderInfo_AllProviders(t *testing.T) {
	tests := []struct {
		choice   string
		wantID   string
		wantName string
		wantType llm.ProviderType
	}{
		{"OpenAI", "openai-default", "OpenAI", llm.ProviderOpenAI},
		{"Anthropic (Claude)", "anthropic-claude", "Claude (Anthropic)", llm.ProviderClaude},
		{"Google (Gemini)", "google-gemini", "Google (Gemini)", llm.ProviderOpenAI},
		{"DeepSeek", "deepseek-default", "DeepSeek", llm.ProviderDeepSeek},
		{"xAI (Grok)", "xai-grok", "xAI (Grok)", llm.ProviderGrok},
		{"OpenRouter", "openrouter-default", "OpenRouter", llm.ProviderOpenAI},
		{"Mistral AI", "mistral-default", "Mistral AI", llm.ProviderMistral},
		{"Groq", "groq-default", "Groq", llm.ProviderGroq},
		{"Together AI", "together-default", "Together AI", llm.ProviderTogether},
		{"Fireworks AI", "fireworks-default", "Fireworks AI", llm.ProviderFireworks},
		{"Perplexity", "perplexity-default", "Perplexity", llm.ProviderPerplexity},
		{"Azure OpenAI", "azure-openai", "Azure OpenAI", llm.ProviderOpenAI},
		{"Ollama (Local)", "ollama-local", "Ollama (Local)", llm.ProviderOllama},
		{"LiteLLM", "litellm", "LiteLLM", llm.ProviderOpenAI},
		{"Outro (URL personalizada)", "custom", "Custom Provider", llm.ProviderCustom},
		{"DesconhecidoXYZ", "custom", "Custom Provider", llm.ProviderCustom},
	}

	for _, tt := range tests {
		t.Run(tt.choice, func(t *testing.T) {
			info := getWizardProviderInfo(tt.choice)
			if info.ID != tt.wantID {
				t.Errorf("ID: got %s, want %s", info.ID, tt.wantID)
			}
			if info.Name != tt.wantName {
				t.Errorf("Name: got %s, want %s", info.Name, tt.wantName)
			}
			if info.Type != tt.wantType {
				t.Errorf("Type: got %s, want %s", info.Type, tt.wantType)
			}
		})
	}
}

func TestGetWizardProviderInfo_IDsMatchCreateDefaultLLMProvider(t *testing.T) {
	// IDs do wizard devem ser consistentes com CreateDefaultLLMProvider
	mapping := map[string]string{
		"OpenAI":             "openai-default",
		"Anthropic (Claude)": "anthropic-claude",
		"Google (Gemini)":    "google-gemini",
		"DeepSeek":           "deepseek-default",
		"xAI (Grok)":         "xai-grok",
		"OpenRouter":         "openrouter-default",
		"Mistral AI":         "mistral-default",
		"Groq":               "groq-default",
		"Together AI":        "together-default",
		"Fireworks AI":       "fireworks-default",
		"Perplexity":         "perplexity-default",
		"Ollama (Local)":     "ollama-local",
	}
	for choice, expectedID := range mapping {
		info := getWizardProviderInfo(choice)
		if info.ID != expectedID {
			t.Errorf("choice=%s: ID got %s, want %s", choice, info.ID, expectedID)
		}
	}
}

func TestGetWizardProviderInfo_APIFormats(t *testing.T) {
	tests := []struct {
		choice  string
		wantFmt llm.APIFormat
	}{
		{"OpenAI", llm.APIFormatOpenAIResponses},
		{"Anthropic (Claude)", llm.APIFormatAnthropic},
		{"Google (Gemini)", ""},
		{"OpenRouter", ""},
		{"Groq", ""},
		{"Ollama (Local)", ""},
	}
	for _, tt := range tests {
		t.Run(tt.choice, func(t *testing.T) {
			info := getWizardProviderInfo(tt.choice)
			if info.APIFormat != tt.wantFmt {
				t.Errorf("APIFormat: got %q, want %q", info.APIFormat, tt.wantFmt)
			}
		})
	}
}

// --- createWizardProvider ---

func TestCreateWizardProvider_OpenAI(t *testing.T) {
	app := setupWizardTestApp(t)

	providerID, err := app.createWizardProvider("OpenAI", "https://api.openai.com/v1", "sk-test123", "gpt-4o-mini")
	if err != nil {
		t.Fatalf("createWizardProvider: %v", err)
	}

	if providerID != "openai-default" {
		t.Errorf("providerID: got %s, want openai-default", providerID)
	}

	// Provider must be in registry
	provider := app.llmRegistry.Get("openai-default")
	if provider == nil {
		t.Fatal("provider not found in registry")
	}
	if provider.BaseURL != "https://api.openai.com/v1" {
		t.Errorf("BaseURL: got %s, want https://api.openai.com/v1", provider.BaseURL)
	}
	if provider.Model != "gpt-4o-mini" {
		t.Errorf("Model: got %s, want gpt-4o-mini", provider.Model)
	}
	if provider.Type != llm.ProviderOpenAI {
		t.Errorf("Type: got %s, want openai", provider.Type)
	}
	if provider.CredentialPattern != "api.openai.com" {
		t.Errorf("CredentialPattern: got %s, want api.openai.com", provider.CredentialPattern)
	}

	// Credential must be stored
	auth, err := app.credMgr.GetByPattern("api.openai.com")
	if err != nil {
		t.Fatalf("GetByPattern: %v", err)
	}
	if auth.Token != "sk-test123" {
		t.Errorf("Token: got %s, want sk-test123", auth.Token)
	}
}

func TestCreateWizardProvider_Claude(t *testing.T) {
	app := setupWizardTestApp(t)

	providerID, err := app.createWizardProvider("Anthropic (Claude)", "https://api.anthropic.com/v1", "sk-ant-test", "claude-3-5-sonnet")
	if err != nil {
		t.Fatalf("createWizardProvider: %v", err)
	}

	if providerID != "anthropic-claude" {
		t.Errorf("providerID: got %s, want anthropic-claude", providerID)
	}

	provider := app.llmRegistry.Get("anthropic-claude")
	if provider == nil {
		t.Fatal("provider not found in registry")
	}
	if provider.Type != llm.ProviderClaude {
		t.Errorf("Type: got %s, want claude", provider.Type)
	}
}

func TestCreateWizardProvider_Ollama_NoAPIKey(t *testing.T) {
	app := setupWizardTestApp(t)

	providerID, err := app.createWizardProvider("Ollama (Local)", "http://localhost:11434/v1", "", "llama2")
	if err != nil {
		t.Fatalf("createWizardProvider: %v", err)
	}

	if providerID != "ollama-local" {
		t.Errorf("providerID: got %s, want ollama-local", providerID)
	}

	provider := app.llmRegistry.Get("ollama-local")
	if provider == nil {
		t.Fatal("provider not found in registry")
	}
	if provider.Timeout != 300 {
		t.Errorf("Timeout: got %d, want 300 for Ollama", provider.Timeout)
	}
}

func TestCreateWizardProvider_DeepSeek(t *testing.T) {
	app := setupWizardTestApp(t)

	providerID, err := app.createWizardProvider("DeepSeek", "https://api.deepseek.com/v1", "sk-deep-test", "deepseek-chat")
	if err != nil {
		t.Fatalf("createWizardProvider: %v", err)
	}

	if providerID != "deepseek-default" {
		t.Errorf("providerID: got %s, want deepseek-default", providerID)
	}

	provider := app.llmRegistry.Get("deepseek-default")
	if provider == nil {
		t.Fatal("provider not found in registry")
	}
	if provider.Type != llm.ProviderDeepSeek {
		t.Errorf("Type: got %s, want deepseek", provider.Type)
	}
	if provider.CredentialPattern != "api.deepseek.com" {
		t.Errorf("CredentialPattern: got %s, want api.deepseek.com", provider.CredentialPattern)
	}

	auth, err := app.credMgr.GetByPattern("api.deepseek.com")
	if err != nil {
		t.Fatalf("GetByPattern: %v", err)
	}
	if auth.Token != "sk-deep-test" {
		t.Errorf("Token: got %s, want sk-deep-test", auth.Token)
	}
}

func TestCreateWizardProvider_Grok(t *testing.T) {
	app := setupWizardTestApp(t)

	providerID, err := app.createWizardProvider("xAI (Grok)", "https://api.x.ai/v1", "xai-key-test", "grok-2")
	if err != nil {
		t.Fatalf("createWizardProvider: %v", err)
	}

	if providerID != "xai-grok" {
		t.Errorf("providerID: got %s, want xai-grok", providerID)
	}

	provider := app.llmRegistry.Get("xai-grok")
	if provider == nil {
		t.Fatal("provider not found in registry")
	}
	if provider.Type != llm.ProviderGrok {
		t.Errorf("Type: got %s, want grok", provider.Type)
	}
	if provider.CredentialPattern != "api.x.ai" {
		t.Errorf("CredentialPattern: got %s, want api.x.ai", provider.CredentialPattern)
	}
}

func TestCreateWizardProvider_CustomURL(t *testing.T) {
	app := setupWizardTestApp(t)

	providerID, err := app.createWizardProvider("Outro (URL personalizada)", "https://my-server.example.com/v1", "my-key", "my-model")
	if err != nil {
		t.Fatalf("createWizardProvider: %v", err)
	}

	if providerID != "custom" {
		t.Errorf("providerID: got %s, want custom", providerID)
	}

	provider := app.llmRegistry.Get("custom")
	if provider == nil {
		t.Fatal("provider not found in registry")
	}
	if provider.CredentialPattern != "my-server.example.com" {
		t.Errorf("CredentialPattern: got %s, want my-server.example.com", provider.CredentialPattern)
	}
}

func TestCreateWizardProvider_OverwritesExisting(t *testing.T) {
	app := setupWizardTestApp(t)

	_, err := app.createWizardProvider("OpenAI", "https://api.openai.com/v1", "sk-old", "gpt-3.5-turbo")
	if err != nil {
		t.Fatalf("first createWizardProvider: %v", err)
	}

	// Re-run overwrites existing
	_, err = app.createWizardProvider("OpenAI", "https://api.openai.com/v1", "sk-new", "gpt-4o")
	if err != nil {
		t.Fatalf("second createWizardProvider: %v", err)
	}

	provider := app.llmRegistry.Get("openai-default")
	if provider == nil {
		t.Fatal("provider not found in registry after overwrite")
	}
	if provider.Model != "gpt-4o" {
		t.Errorf("Model after overwrite: got %s, want gpt-4o", provider.Model)
	}

	auth, err := app.credMgr.GetByPattern("api.openai.com")
	if err != nil {
		t.Fatalf("GetByPattern after overwrite: %v", err)
	}
	if auth.Token != "sk-new" {
		t.Errorf("Token after overwrite: got %s, want sk-new", auth.Token)
	}
}

func TestCreateWizardProvider_PersistsToSQLite(t *testing.T) {
	app := setupWizardTestApp(t)

	_, err := app.createWizardProvider("Google (Gemini)", "https://generativelanguage.googleapis.com/v1beta/openai/", "goog-key", "gemini-2.0-flash")
	if err != nil {
		t.Fatalf("createWizardProvider: %v", err)
	}

	dbProviders, err := database.GetLLMProviders()
	if err != nil {
		t.Fatalf("GetLLMProviders: %v", err)
	}
	if len(dbProviders) == 0 {
		t.Fatal("no providers in SQLite after createWizardProvider")
	}

	found := false
	for _, p := range dbProviders {
		if p.ID == "google-gemini" {
			found = true
			if p.BaseURL != "https://generativelanguage.googleapis.com/v1beta/openai/" {
				t.Errorf("DB BaseURL: got %s", p.BaseURL)
			}
		}
	}
	if !found {
		t.Error("google-gemini not found in SQLite")
	}
}

// --- $default sentinel resolution ---

func TestDefaultProfileUsesSentinel(t *testing.T) {
	p := profiles.DefaultProfile()
	if p.Chat.LLMProvider != profiles.DefaultProviderSentinel {
		t.Errorf("DefaultProfile LLMProvider: got %s, want %s", p.Chat.LLMProvider, profiles.DefaultProviderSentinel)
	}
	if p.Chat.Model != profiles.DefaultProviderSentinel {
		t.Errorf("DefaultProfile Model: got %s, want %s", p.Chat.Model, profiles.DefaultProviderSentinel)
	}
	if p.Voice.Assistant.LLMProviderID != profiles.DefaultProviderSentinel {
		t.Errorf("DefaultProfile Voice.Assistant.LLMProviderID: got %s, want %s", p.Voice.Assistant.LLMProviderID, profiles.DefaultProviderSentinel)
	}
	if p.Input.LLMProviderID != profiles.DefaultProviderSentinel {
		t.Errorf("DefaultProfile Input.LLMProviderID: got %s, want %s", p.Input.LLMProviderID, profiles.DefaultProviderSentinel)
	}
}

// --- Integration: createWizardProvider marks as default ---

func TestWizardIntegration_ProviderMarkedAsDefault(t *testing.T) {
	app, pm := setupWizardTestAppWithProfiles(t)

	p := profiles.DefaultProfile()
	p.Name = "Meu Perfil"
	p.Active = true
	slug, err := pm.Create(p)
	if err != nil {
		t.Fatalf("create profile: %v", err)
	}

	providerID, err := app.createWizardProvider("OpenAI", "https://api.openai.com/v1", "sk-test", "gpt-4o-mini")
	if err != nil {
		t.Fatalf("createWizardProvider: %v", err)
	}

	// Profile retains $default sentinel
	profile, err := pm.Get(slug)
	if err != nil {
		t.Fatalf("get profile: %v", err)
	}
	if profile.Chat.LLMProvider != profiles.DefaultProviderSentinel {
		t.Errorf("LLMProvider: got %s, want %s", profile.Chat.LLMProvider, profiles.DefaultProviderSentinel)
	}

	// Provider should exist in registry with IsDefault and DefaultModel
	provider := app.llmRegistry.Get(providerID)
	if provider == nil {
		t.Fatal("provider not found in registry")
	}
	if provider.BaseURL != "https://api.openai.com/v1" {
		t.Errorf("provider BaseURL: got %s", provider.BaseURL)
	}
	if !provider.IsDefault {
		t.Error("wizard provider should be marked as IsDefault")
	}
	if provider.DefaultModel != "gpt-4o-mini" {
		t.Errorf("provider DefaultModel: got %s, want gpt-4o-mini", provider.DefaultModel)
	}

	// Credential should exist for the provider's pattern
	auth, err := app.credMgr.GetByPattern(provider.CredentialPattern)
	if err != nil {
		t.Fatalf("credential not found for pattern %s: %v", provider.CredentialPattern, err)
	}
	if auth.Token != "sk-test" {
		t.Errorf("credential token: got %s, want sk-test", auth.Token)
	}
}

// --- resolveProfileDefaults ---

func TestResolveProfileDefaults_ResolvesSentinels(t *testing.T) {
	app := setupWizardTestApp(t)

	// Save provider in DB and mark as default
	dbProv := &database.LLMProvider{
		ID:           "test-provider",
		Name:         "Test Provider",
		Type:         "openai",
		BaseURL:      "https://api.test.com/v1",
		DefaultModel: "test-model-v1",
		IsDefault:    true,
	}
	if err := database.SaveLLMProvider(dbProv); err != nil {
		t.Fatalf("SaveLLMProvider: %v", err)
	}
	if err := database.SetDefaultProvider("test-provider"); err != nil {
		t.Fatalf("SetDefaultProvider: %v", err)
	}

	provider := &llm.ProviderConfig{
		ID:           "test-provider",
		Name:         "Test Provider",
		Type:         llm.ProviderOpenAI,
		BaseURL:      "https://api.test.com/v1",
		DefaultModel: "test-model-v1",
		IsDefault:    true,
	}
	if err := app.llmRegistry.Register(provider); err != nil {
		t.Fatalf("register: %v", err)
	}

	// Create profile with $default sentinels
	profile := profiles.DefaultProfile()
	if profile.Chat.LLMProvider != profiles.DefaultProviderSentinel {
		t.Fatalf("expected sentinel, got %s", profile.Chat.LLMProvider)
	}

	resolved := app.resolveProfileDefaults(profile)

	if resolved.Chat.LLMProvider != "test-provider" {
		t.Errorf("Chat.LLMProvider: got %s, want test-provider", resolved.Chat.LLMProvider)
	}
	if resolved.Chat.Model != "test-model-v1" {
		t.Errorf("Chat.Model: got %s, want test-model-v1", resolved.Chat.Model)
	}
	if resolved.Voice.Assistant.LLMProviderID != "test-provider" {
		t.Errorf("Voice.Assistant.LLMProviderID: got %s, want test-provider", resolved.Voice.Assistant.LLMProviderID)
	}
	if resolved.Input.LLMProviderID != "test-provider" {
		t.Errorf("Input.LLMProviderID: got %s, want test-provider", resolved.Input.LLMProviderID)
	}
}

func TestResolveProfileDefaults_ConcreteIDsUnchanged(t *testing.T) {
	app := setupWizardTestApp(t)

	dbProv := &database.LLMProvider{
		ID: "default-prov", Name: "Default", Type: "openai", BaseURL: "https://api.test.com/v1",
		DefaultModel: "default-model", IsDefault: true,
	}
	if err := database.SaveLLMProvider(dbProv); err != nil {
		t.Fatalf("SaveLLMProvider: %v", err)
	}
	if err := database.SetDefaultProvider("default-prov"); err != nil {
		t.Fatalf("SetDefaultProvider: %v", err)
	}

	provider := &llm.ProviderConfig{
		ID:           "default-prov",
		Name:         "Default",
		Type:         llm.ProviderOpenAI,
		BaseURL:      "https://api.test.com/v1",
		DefaultModel: "default-model",
		IsDefault:    true,
	}
	if err := app.llmRegistry.Register(provider); err != nil {
		t.Fatalf("register: %v", err)
	}

	profile := &profiles.Profile{
		Name: "Concrete",
		Chat: profiles.ChatConfig{
			LLMProvider: "my-concrete-id",
			Model:       "gpt-4o",
		},
		Voice: profiles.VoiceConfig{
			Assistant: profiles.VoiceRoleConfig{LLMProviderID: "my-voice-id"},
		},
		Input: profiles.InputConfig{
			LLMProviderID: "my-stt-id",
		},
	}

	resolved := app.resolveProfileDefaults(profile)

	if resolved.Chat.LLMProvider != "my-concrete-id" {
		t.Errorf("Chat.LLMProvider: got %s, want my-concrete-id", resolved.Chat.LLMProvider)
	}
	if resolved.Chat.Model != "gpt-4o" {
		t.Errorf("Chat.Model: got %s, want gpt-4o", resolved.Chat.Model)
	}
	if resolved.Voice.Assistant.LLMProviderID != "my-voice-id" {
		t.Errorf("Voice.Assistant.LLMProviderID: got %s, want my-voice-id", resolved.Voice.Assistant.LLMProviderID)
	}
	if resolved.Input.LLMProviderID != "my-stt-id" {
		t.Errorf("Input.LLMProviderID: got %s, want my-stt-id", resolved.Input.LLMProviderID)
	}
}

func TestResolveProfileDefaults_NoDefaultProvider(t *testing.T) {
	app := setupWizardTestApp(t)

	profile := profiles.DefaultProfile()
	resolved := app.resolveProfileDefaults(profile)

	// Without default provider in DB, sentinels should remain
	if resolved.Chat.LLMProvider != profiles.DefaultProviderSentinel {
		t.Errorf("expected sentinel preserved, got %s", resolved.Chat.LLMProvider)
	}
}

func TestResolveProfileDefaults_NilProfile(t *testing.T) {
	app := setupWizardTestApp(t)

	result := app.resolveProfileDefaults(nil)
	if result != nil {
		t.Error("expected nil for nil profile")
	}
}

// --- Database: SetDefaultProvider / GetDefaultProvider ---

func TestSetDefaultProvider_SwitchesCorrectly(t *testing.T) {
	_ = setupWizardTestDB(t)

	p1 := &database.LLMProvider{ID: "prov-1", Name: "P1", Type: "openai", BaseURL: "https://a.com", IsDefault: true}
	p2 := &database.LLMProvider{ID: "prov-2", Name: "P2", Type: "openai", BaseURL: "https://b.com", IsDefault: false}
	database.SaveLLMProvider(p1)
	database.SaveLLMProvider(p2)

	def, err := database.GetDefaultProvider()
	if err != nil || def.ID != "prov-1" {
		t.Fatalf("initial default: got %v, err %v", def, err)
	}

	if err := database.SetDefaultProvider("prov-2"); err != nil {
		t.Fatalf("SetDefaultProvider: %v", err)
	}

	def, err = database.GetDefaultProvider()
	if err != nil {
		t.Fatalf("GetDefaultProvider after switch: %v", err)
	}
	if def.ID != "prov-2" {
		t.Errorf("expected prov-2, got %s", def.ID)
	}

	// Ensure prov-1 is no longer default
	old, _ := database.GetLLMProvider("prov-1")
	if old.IsDefault {
		t.Error("prov-1 should no longer be default")
	}
}

func TestGetDefaultProvider_NoDefault(t *testing.T) {
	_ = setupWizardTestDB(t)

	def, err := database.GetDefaultProvider()
	if err == nil && def != nil {
		t.Error("expected no default provider")
	}
}

// --- Save/Load roundtrip: IsDefault and DefaultModel survive DB persistence ---

func TestSaveLoadRoundtrip_DefaultFieldsSurvive(t *testing.T) {
	app := setupWizardTestApp(t)

	// Register provider with IsDefault + DefaultModel
	provider := &llm.ProviderConfig{
		ID:                "roundtrip-prov",
		Name:              "Roundtrip Test",
		Type:              llm.ProviderOpenAI,
		BaseURL:           "https://api.test.com/v1",
		Model:             "current-model",
		DefaultModel:      "my-default-model",
		IsDefault:         true,
		Timeout:           180,
		CredentialPattern: "api.test.com",
	}
	if err := app.llmRegistry.Register(provider); err != nil {
		t.Fatalf("Register: %v", err)
	}

	// Save to DB
	if err := app.saveLLMProviders(); err != nil {
		t.Fatalf("saveLLMProviders: %v", err)
	}

	// Clear registry and reload from DB
	app.llmRegistry = llm.NewProviderRegistry()
	app.providerSvc = providers.NewService(providers.ServiceConfig{
		Registry: app.llmRegistry,
		CredMgr:  app.credMgr,
		Store:    providers.NewDBStore(),
	})
	if err := app.loadLLMProviders(); err != nil {
		t.Fatalf("loadLLMProviders: %v", err)
	}

	loaded := app.llmRegistry.Get("roundtrip-prov")
	if loaded == nil {
		t.Fatal("provider not found after reload")
	}
	if loaded.DefaultModel != "my-default-model" {
		t.Errorf("DefaultModel: got %q, want my-default-model", loaded.DefaultModel)
	}
	if !loaded.IsDefault {
		t.Error("IsDefault should be true after reload")
	}
	if loaded.Model != "current-model" {
		t.Errorf("Model: got %q, want current-model", loaded.Model)
	}
	if loaded.CredentialPattern != "api.test.com" {
		t.Errorf("CredentialPattern: got %q, want api.test.com", loaded.CredentialPattern)
	}
}

// --- CreateLLMProvider: first provider auto-marked as default ---

func TestCreateLLMProvider_FirstProviderIsAutoDefault(t *testing.T) {
	app := setupWizardTestApp(t)

	result, err := app.CreateLLMProvider(CreateLLMProviderRequest{
		ID:           "first-prov",
		Name:         "First Provider",
		Type:         "openai",
		BaseURL:      "https://api.openai.com/v1",
		DefaultModel: "gpt-4o-mini",
	})
	if err != nil {
		t.Fatalf("CreateLLMProvider: %v", err)
	}
	if result["is_default"] != true {
		t.Error("first provider should be auto-marked as default")
	}
	if result["default_model"] != "gpt-4o-mini" {
		t.Errorf("default_model: got %v, want gpt-4o-mini", result["default_model"])
	}

	// Verify in DB
	def, err := database.GetDefaultProvider()
	if err != nil {
		t.Fatalf("GetDefaultProvider: %v", err)
	}
	if def.ID != "first-prov" {
		t.Errorf("DB default: got %s, want first-prov", def.ID)
	}
}

func TestCreateLLMProvider_APIFormatPersisted(t *testing.T) {
	app := setupWizardTestApp(t)

	_, err := app.CreateLLMProvider(CreateLLMProviderRequest{
		ID:        "openai-test",
		Name:      "OpenAI Test",
		Type:      "openai",
		BaseURL:   "https://api.openai.com/v1",
		APIFormat: "openai_responses",
	})
	if err != nil {
		t.Fatalf("CreateLLMProvider: %v", err)
	}

	provider := app.llmRegistry.Get("openai-test")
	if provider == nil {
		t.Fatal("provider not found in registry")
	}
	if string(provider.APIFormat) != "openai_responses" {
		t.Errorf("APIFormat: got %q, want openai_responses", provider.APIFormat)
	}
	if provider.GetAPIFormat() != llm.APIFormatOpenAIResponses {
		t.Errorf("GetAPIFormat: got %q, want %q", provider.GetAPIFormat(), llm.APIFormatOpenAIResponses)
	}

	dbProv, err := database.GetLLMProvider("openai-test")
	if err != nil {
		t.Fatalf("GetLLMProvider: %v", err)
	}
	if dbProv.APIFormat != "openai_responses" {
		t.Errorf("DB APIFormat: got %q, want openai_responses", dbProv.APIFormat)
	}
}

func TestCreateLLMProvider_OpenAICompatibleKeepsDefaultFormat(t *testing.T) {
	app := setupWizardTestApp(t)

	_, err := app.CreateLLMProvider(CreateLLMProviderRequest{
		ID:        "groq-test",
		Name:      "Groq Test",
		Type:      "groq",
		BaseURL:   "https://api.groq.com/openai/v1",
		APIFormat: "openai",
	})
	if err != nil {
		t.Fatalf("CreateLLMProvider: %v", err)
	}

	provider := app.llmRegistry.Get("groq-test")
	if provider == nil {
		t.Fatal("provider not found in registry")
	}
	if provider.GetAPIFormat() != llm.APIFormatOpenAI {
		t.Errorf("GetAPIFormat: got %q, want %q", provider.GetAPIFormat(), llm.APIFormatOpenAI)
	}
}

func TestCreateLLMProvider_SecondProviderIsNotAutoDefault(t *testing.T) {
	app := setupWizardTestApp(t)

	// Create first (becomes default)
	_, err := app.CreateLLMProvider(CreateLLMProviderRequest{
		ID: "prov-1", Name: "First", Type: "openai", BaseURL: "https://api.openai.com/v1",
	})
	if err != nil {
		t.Fatalf("CreateLLMProvider first: %v", err)
	}

	// Create second (should NOT become default)
	result, err := app.CreateLLMProvider(CreateLLMProviderRequest{
		ID: "prov-2", Name: "Second", Type: "openai", BaseURL: "https://api.anthropic.com",
	})
	if err != nil {
		t.Fatalf("CreateLLMProvider second: %v", err)
	}
	if result["is_default"] == true {
		t.Error("second provider should NOT be auto-default")
	}

	// Default should still be prov-1
	def, err := database.GetDefaultProvider()
	if err != nil {
		t.Fatalf("GetDefaultProvider: %v", err)
	}
	if def.ID != "prov-1" {
		t.Errorf("default should still be prov-1, got %s", def.ID)
	}
}

// --- Partial $default: concrete provider + $default model ---

func TestResolveProfileDefaults_PartialSentinel_OnlyModel(t *testing.T) {
	app := setupWizardTestApp(t)

	dbProv := &database.LLMProvider{
		ID: "default-prov", Name: "Default", Type: "openai", BaseURL: "https://api.test.com/v1",
		DefaultModel: "fallback-model", IsDefault: true,
	}
	database.SaveLLMProvider(dbProv)
	database.SetDefaultProvider("default-prov")

	// Concrete provider but $default model
	profile := &profiles.Profile{
		Name: "Partial",
		Chat: profiles.ChatConfig{
			LLMProvider: "my-concrete-provider",
			Model:       profiles.DefaultProviderSentinel,
		},
		Voice: profiles.VoiceConfig{
			Assistant: profiles.VoiceRoleConfig{LLMProviderID: "my-voice-provider"},
		},
		Input: profiles.InputConfig{
			LLMProviderID: profiles.DefaultProviderSentinel,
		},
	}

	resolved := app.resolveProfileDefaults(profile)

	if resolved.Chat.LLMProvider != "my-concrete-provider" {
		t.Errorf("Chat.LLMProvider should stay concrete, got %s", resolved.Chat.LLMProvider)
	}
	if resolved.Chat.Model != "fallback-model" {
		t.Errorf("Chat.Model should resolve to fallback-model, got %s", resolved.Chat.Model)
	}
	if resolved.Voice.Assistant.LLMProviderID != "my-voice-provider" {
		t.Errorf("Voice.Assistant.LLMProviderID should stay concrete, got %s", resolved.Voice.Assistant.LLMProviderID)
	}
	if resolved.Input.LLMProviderID != "default-prov" {
		t.Errorf("Input.LLMProviderID should resolve, got %s", resolved.Input.LLMProviderID)
	}
}

func TestResolveProfileDefaults_PartialSentinel_OnlyProvider(t *testing.T) {
	app := setupWizardTestApp(t)

	dbProv := &database.LLMProvider{
		ID: "default-prov", Name: "Default", Type: "openai", BaseURL: "https://api.test.com/v1",
		DefaultModel: "fallback-model", IsDefault: true,
	}
	database.SaveLLMProvider(dbProv)
	database.SetDefaultProvider("default-prov")

	// $default provider but concrete model
	profile := &profiles.Profile{
		Name: "Partial Provider",
		Chat: profiles.ChatConfig{
			LLMProvider: profiles.DefaultProviderSentinel,
			Model:       "gpt-4o",
		},
	}

	resolved := app.resolveProfileDefaults(profile)

	if resolved.Chat.LLMProvider != "default-prov" {
		t.Errorf("Chat.LLMProvider should resolve, got %s", resolved.Chat.LLMProvider)
	}
	if resolved.Chat.Model != "gpt-4o" {
		t.Errorf("Chat.Model should stay concrete, got %s", resolved.Chat.Model)
	}
}

// --- ensureDefaultProvider migrates legacy providers ---

func TestEnsureDefaultProvider_MarksFirstWhenNoneIsDefault(t *testing.T) {
	app := setupWizardTestApp(t)

	// Simulate legacy provider (created before IsDefault feature)
	legacyProv := &database.LLMProvider{
		ID:      "litellm-legacy",
		Name:    "LiteLLM Legacy",
		Type:    "openai",
		BaseURL: "http://localhost:4000/v1",
		Model:   "gpt-4o",
	}
	if err := database.SaveLLMProvider(legacyProv); err != nil {
		t.Fatalf("save legacy provider: %v", err)
	}

	cfg := &llm.ProviderConfig{
		ID:      legacyProv.ID,
		Name:    legacyProv.Name,
		Type:    llm.ProviderType(legacyProv.Type),
		BaseURL: legacyProv.BaseURL,
		Model:   legacyProv.Model,
	}
	app.llmRegistry.Register(cfg)

	// Verify no default exists
	defProv, _ := database.GetDefaultProvider()
	if defProv != nil {
		t.Fatal("expected no default provider before migration")
	}

	app.ensureDefaultProvider()

	// Now should have a default
	defProv, err := database.GetDefaultProvider()
	if err != nil {
		t.Fatalf("GetDefaultProvider: %v", err)
	}
	if defProv.ID != "litellm-legacy" {
		t.Errorf("default provider: got %q, want 'litellm-legacy'", defProv.ID)
	}
	if defProv.DefaultModel != "gpt-4o" {
		t.Errorf("default model: got %q, want 'gpt-4o' (migrated from Model)", defProv.DefaultModel)
	}
}

func TestEnsureDefaultProvider_DoesNotOverrideExistingDefault(t *testing.T) {
	app := setupWizardTestApp(t)

	// Create two providers, second one is default
	prov1 := &database.LLMProvider{
		ID:      "provider-1",
		Name:    "Provider 1",
		Type:    "openai",
		BaseURL: "http://localhost:4000/v1",
	}
	prov2 := &database.LLMProvider{
		ID:           "provider-2",
		Name:         "Provider 2",
		Type:         "openai",
		BaseURL:      "http://localhost:5000/v1",
		IsDefault:    true,
		DefaultModel: "my-model",
	}
	database.SaveLLMProvider(prov1)
	database.SaveLLMProvider(prov2)
	database.SetDefaultProvider(prov2.ID)

	for _, p := range []*database.LLMProvider{prov1, prov2} {
		app.llmRegistry.Register(&llm.ProviderConfig{
			ID:      p.ID,
			Name:    p.Name,
			Type:    llm.ProviderType(p.Type),
			BaseURL: p.BaseURL,
		})
	}

	app.ensureDefaultProvider()

	defProv, err := database.GetDefaultProvider()
	if err != nil {
		t.Fatalf("GetDefaultProvider: %v", err)
	}
	if defProv.ID != "provider-2" {
		t.Errorf("default should still be provider-2, got %q", defProv.ID)
	}
}

// --- Builtin padrao.json has active:true ---

func TestBuiltinPadraoJSON_HasActiveTrue(t *testing.T) {
	data, err := fs.ReadFile(builtinProfilesFS, "builtin/profiles/padrao.json")
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	var p profiles.Profile
	if err := json.Unmarshal(data, &p); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if !p.Active {
		t.Error("padrao.json should have active=true")
	}
}

// --- ensureActiveProfile sets padrao active when none is ---

func TestEnsureActiveProfile_SetsPadraoWhenNoneActive(t *testing.T) {
	app, _ := setupWizardTestAppWithProfiles(t)

	for _, name := range []string{"Editor de Texto", "Padrão", "Programação"} {
		p := profiles.DefaultProfile()
		p.Name = name
		p.Active = false
		if _, err := app.profileManager.Create(p); err != nil {
			t.Fatalf("create %s: %v", name, err)
		}
	}

	app.ensureActiveProfile()

	active, err := app.profileManager.GetActive()
	if err != nil {
		t.Fatalf("GetActive: %v", err)
	}
	if active.Name != "Padrão" {
		t.Errorf("active profile: got %q, want 'Padrão'", active.Name)
	}
	if !active.Active {
		t.Error("padrao should be marked Active after ensureActiveProfile")
	}
}

func TestEnsureActiveProfile_DoesNotOverrideExistingActive(t *testing.T) {
	app, _ := setupWizardTestAppWithProfiles(t)

	padrao := profiles.DefaultProfile()
	padrao.Name = "Padrão"
	padrao.Active = false
	if _, err := app.profileManager.Create(padrao); err != nil {
		t.Fatalf("create padrao: %v", err)
	}

	prog := profiles.DefaultProfile()
	prog.Name = "Programação"
	prog.Active = true
	if _, err := app.profileManager.Create(prog); err != nil {
		t.Fatalf("create programacao: %v", err)
	}

	app.ensureActiveProfile()

	active, err := app.profileManager.GetActive()
	if err != nil {
		t.Fatalf("GetActive: %v", err)
	}
	if active.Name != "Programação" {
		t.Errorf("active profile should still be 'Programação', got %q", active.Name)
	}
}

// --- Builtin profile JSONs contain $default ---

func TestBuiltinProfileJSONs_ContainDefaultSentinel(t *testing.T) {
	entries, err := fs.ReadDir(builtinProfilesFS, "builtin/profiles")
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}

	if len(entries) == 0 {
		t.Fatal("no builtin profiles found")
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		data, err := fs.ReadFile(builtinProfilesFS, "builtin/profiles/"+entry.Name())
		if err != nil {
			t.Errorf("ReadFile %s: %v", entry.Name(), err)
			continue
		}

		var raw map[string]interface{}
		if err := json.Unmarshal(data, &raw); err != nil {
			t.Errorf("Unmarshal %s: %v", entry.Name(), err)
			continue
		}

		chat, _ := raw["chat"].(map[string]interface{})
		if chat == nil {
			t.Errorf("%s: missing chat section", entry.Name())
			continue
		}

		// llm_provider must be $default
		if prov, ok := chat["llm_provider"].(string); !ok || prov != profiles.DefaultProviderSentinel {
			t.Errorf("%s: chat.llm_provider = %q, want %q", entry.Name(), prov, profiles.DefaultProviderSentinel)
		}
		// model must be $default
		if model, ok := chat["model"].(string); !ok || model != profiles.DefaultProviderSentinel {
			t.Errorf("%s: chat.model = %q, want %q", entry.Name(), model, profiles.DefaultProviderSentinel)
		}

		// voice.assistant.llm_provider_id must be $default (if voice section exists)
		if voice, ok := raw["voice"].(map[string]interface{}); ok {
			if assistant, ok := voice["assistant"].(map[string]interface{}); ok {
				if vid, ok := assistant["llm_provider_id"].(string); ok && vid != profiles.DefaultProviderSentinel {
					t.Errorf("%s: voice.assistant.llm_provider_id = %q, want %q", entry.Name(), vid, profiles.DefaultProviderSentinel)
				}
			}
		}

		// input.llm_provider_id must be $default (if input section exists)
		if input, ok := raw["input"].(map[string]interface{}); ok {
			if iid, ok := input["llm_provider_id"].(string); ok && iid != profiles.DefaultProviderSentinel {
				t.Errorf("%s: input.llm_provider_id = %q, want %q", entry.Name(), iid, profiles.DefaultProviderSentinel)
			}
		}
	}
}

// --- validateWizardURL ---

func TestValidateWizardURL_ValidFormats(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	app := setupWizardTestApp(t)
	if err := app.validateWizardURL(srv.URL); err != nil {
		t.Errorf("should accept valid reachable URL: %v", err)
	}
}

func TestValidateWizardURL_InvalidScheme(t *testing.T) {
	app := setupWizardTestApp(t)
	if err := app.validateWizardURL("ftp://example.com"); err == nil {
		t.Error("should reject ftp:// scheme")
	}
}

func TestValidateWizardURL_EmptyHost(t *testing.T) {
	app := setupWizardTestApp(t)
	if err := app.validateWizardURL("http://"); err == nil {
		t.Error("should reject URL with empty host")
	}
}

func TestValidateWizardURL_Unreachable(t *testing.T) {
	app := setupWizardTestApp(t)
	if err := app.validateWizardURL("http://192.0.2.1:1"); err == nil {
		t.Error("should reject unreachable URL")
	}
}

func TestValidateWizardURL_Accepts401(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	app := setupWizardTestApp(t)
	if err := app.validateWizardURL(srv.URL); err != nil {
		t.Errorf("should accept 401 (server is alive): %v", err)
	}
}

func TestValidateWizardURL_RejectsServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	app := setupWizardTestApp(t)
	if err := app.validateWizardURL(srv.URL); err == nil {
		t.Error("should reject 500 server error")
	}
}

// --- validateWizardConnection ---

func modelsJSON(ids ...string) []byte {
	type model struct {
		ID string `json:"id"`
	}
	type resp struct {
		Data []model `json:"data"`
	}
	r := resp{}
	for _, id := range ids {
		r.Data = append(r.Data, model{ID: id})
	}
	b, _ := json.Marshal(r)
	return b
}

func TestValidateWizardConnection_SuccessWithModels(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(modelsJSON("gpt-4o", "gpt-4o-mini"))
	}))
	defer srv.Close()

	app := setupWizardTestApp(t)
	result := app.validateWizardConnection(srv.URL, "sk-test")

	if result.ErrorType != "" {
		t.Fatalf("unexpected error: %s - %s", result.ErrorType, result.ErrorDetail)
	}
	if !result.URLReachable {
		t.Error("URLReachable should be true")
	}
	if !result.AuthOK {
		t.Error("AuthOK should be true")
	}
	if !result.ModelsAvailable {
		t.Error("ModelsAvailable should be true")
	}
	if len(result.Models) != 2 {
		t.Errorf("expected 2 models, got %d", len(result.Models))
	}
}

func TestValidateWizardConnection_Unauthorized_WithKey(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprint(w, `{"error": "invalid api key"}`)
	}))
	defer srv.Close()

	app := setupWizardTestApp(t)
	result := app.validateWizardConnection(srv.URL, "bad-key")

	if result.ErrorType != "auth_invalid" {
		t.Errorf("expected auth_invalid, got %s", result.ErrorType)
	}
	if !result.URLReachable {
		t.Error("URLReachable should be true (server responded)")
	}
}

func TestValidateWizardConnection_Unauthorized_NoKey(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	app := setupWizardTestApp(t)
	result := app.validateWizardConnection(srv.URL, "")

	if result.ErrorType != "auth_required" {
		t.Errorf("expected auth_required, got %s", result.ErrorType)
	}
}

func TestValidateWizardConnection_Forbidden(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	app := setupWizardTestApp(t)
	result := app.validateWizardConnection(srv.URL, "some-key")

	if result.ErrorType != "auth_invalid" {
		t.Errorf("expected auth_invalid, got %s", result.ErrorType)
	}
}

func TestValidateWizardConnection_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	app := setupWizardTestApp(t)
	result := app.validateWizardConnection(srv.URL, "")

	if result.ErrorType != "server_error" {
		t.Errorf("expected server_error, got %s", result.ErrorType)
	}
	if !result.URLReachable {
		t.Error("URLReachable should be true")
	}
}

func TestValidateWizardConnection_Unreachable(t *testing.T) {
	app := setupWizardTestApp(t)
	result := app.validateWizardConnection("http://192.0.2.1:1", "")

	if result.ErrorType != "url_unreachable" {
		t.Errorf("expected url_unreachable, got %s", result.ErrorType)
	}
	if result.URLReachable {
		t.Error("URLReachable should be false")
	}
}

func TestValidateWizardConnection_InvalidURL(t *testing.T) {
	app := setupWizardTestApp(t)

	tests := []string{
		"",
		"not-a-url",
		"ftp://example.com",
		"://missing-scheme",
	}

	for _, u := range tests {
		t.Run(u, func(t *testing.T) {
			result := app.validateWizardConnection(u, "")
			if result.ErrorType != "url_invalid" && result.ErrorType != "url_unreachable" {
				t.Errorf("expected url_invalid or url_unreachable for %q, got %s", u, result.ErrorType)
			}
		})
	}
}

func TestValidateWizardConnection_NotFound_ModelsEndpoint(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	app := setupWizardTestApp(t)
	result := app.validateWizardConnection(srv.URL, "sk-test")

	if result.ErrorType != "" {
		t.Errorf("unexpected error: %s", result.ErrorType)
	}
	if !result.URLReachable {
		t.Error("URLReachable should be true")
	}
	if !result.AuthOK {
		t.Error("AuthOK should be true for 404")
	}
	if result.ModelsAvailable {
		t.Error("ModelsAvailable should be false for 404")
	}
}

func TestValidateWizardConnection_EmptyModelsResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"data": []}`))
	}))
	defer srv.Close()

	app := setupWizardTestApp(t)
	result := app.validateWizardConnection(srv.URL, "sk-test")

	if result.ErrorType != "" {
		t.Errorf("unexpected error: %s", result.ErrorType)
	}
	if !result.AuthOK {
		t.Error("AuthOK should be true")
	}
	if result.ModelsAvailable {
		t.Error("ModelsAvailable should be false with empty data")
	}
}

func TestValidateWizardConnection_AuthHeaderSent(t *testing.T) {
	var receivedAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		w.Write(modelsJSON("model-1"))
	}))
	defer srv.Close()

	app := setupWizardTestApp(t)
	app.validateWizardConnection(srv.URL, "my-secret-key")

	if receivedAuth != "Bearer my-secret-key" {
		t.Errorf("expected Bearer header, got %q", receivedAuth)
	}
}

func TestValidateWizardConnection_NoAuthHeaderWhenEmpty(t *testing.T) {
	var receivedAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		w.Write(modelsJSON("model-1"))
	}))
	defer srv.Close()

	app := setupWizardTestApp(t)
	app.validateWizardConnection(srv.URL, "")

	if receivedAuth != "" {
		t.Errorf("should not send auth header when key is empty, got %q", receivedAuth)
	}
}
