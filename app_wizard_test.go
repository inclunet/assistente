package main

import (
	"context"
	"testing"

	"assistente/internal/configdir"
	"assistente/internal/credentials"
	"assistente/internal/database"
	"assistente/internal/llm"
	"assistente/internal/profiles"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"os"
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

	return &App{
		ctx:         context.Background(),
		credMgr:     credMgr,
		llmRegistry: llm.NewProviderRegistry(),
	}
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

	return &App{
		ctx:            context.Background(),
		credMgr:        credMgr,
		llmRegistry:    llm.NewProviderRegistry(),
		profileManager: pm,
	}, pm
}

// --- getWizardProviderInfo ---

func TestGetWizardProviderInfo_AllProviders(t *testing.T) {
	tests := []struct {
		choice     string
		wantID     string
		wantName   string
		wantType   llm.ProviderType
	}{
		{"OpenAI", "openai-default", "OpenAI", llm.ProviderOpenAI},
		{"Anthropic (Claude)", "anthropic-claude", "Claude (Anthropic)", llm.ProviderClaude},
		{"Google (Gemini)", "google-gemini", "Google (Gemini)", llm.ProviderOpenAI},
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
		"OpenAI":            "openai-default",
		"Anthropic (Claude)": "anthropic-claude",
		"Google (Gemini)":   "google-gemini",
		"Ollama (Local)":    "ollama-local",
	}
	for choice, expectedID := range mapping {
		info := getWizardProviderInfo(choice)
		if info.ID != expectedID {
			t.Errorf("choice=%s: ID got %s, want %s", choice, info.ID, expectedID)
		}
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

// --- updateAllProfilesProviderAndModel ---

func TestUpdateAllProfilesProviderAndModel_UpdatesSlugCorrectly(t *testing.T) {
	app, pm := setupWizardTestAppWithProfiles(t)

	p1 := profiles.DefaultProfile()
	p1.Name = "Perfil Teste"
	p1.Active = true
	slug, err := pm.Create(p1)
	if err != nil {
		t.Fatalf("create profile: %v", err)
	}

	err = app.updateAllProfilesProviderAndModel("anthropic-claude", "claude-3-5-sonnet")
	if err != nil {
		t.Fatalf("updateAllProfilesProviderAndModel: %v", err)
	}

	updated, err := pm.Get(slug)
	if err != nil {
		t.Fatalf("get profile after update: %v", err)
	}

	if updated.Chat.LLMProvider != "anthropic-claude" {
		t.Errorf("LLMProvider: got %s, want anthropic-claude", updated.Chat.LLMProvider)
	}
	if updated.Chat.Model != "claude-3-5-sonnet" {
		t.Errorf("Model: got %s, want claude-3-5-sonnet", updated.Chat.Model)
	}
}

func TestUpdateAllProfilesProviderAndModel_MultipleProfiles(t *testing.T) {
	app, pm := setupWizardTestAppWithProfiles(t)

	for _, name := range []string{"Perfil A", "Perfil B", "Perfil C"} {
		p := profiles.DefaultProfile()
		p.Name = name
		if _, err := pm.Create(p); err != nil {
			t.Fatalf("create %s: %v", name, err)
		}
	}

	err := app.updateAllProfilesProviderAndModel("google-gemini", "gemini-2.0-flash")
	if err != nil {
		t.Fatalf("updateAllProfilesProviderAndModel: %v", err)
	}

	list, err := pm.List()
	if err != nil {
		t.Fatalf("list profiles: %v", err)
	}

	for _, info := range list {
		profile, err := pm.Get(info.Slug)
		if err != nil {
			t.Fatalf("get %s: %v", info.Slug, err)
		}
		if profile.Chat.LLMProvider != "google-gemini" {
			t.Errorf("%s LLMProvider: got %s, want google-gemini", info.Slug, profile.Chat.LLMProvider)
		}
		if profile.Chat.Model != "gemini-2.0-flash" {
			t.Errorf("%s Model: got %s, want gemini-2.0-flash", info.Slug, profile.Chat.Model)
		}
	}
}

func TestUpdateAllProfilesProviderAndModel_NoProfilesDoesNotError(t *testing.T) {
	app, _ := setupWizardTestAppWithProfiles(t)

	err := app.updateAllProfilesProviderAndModel("openai-default", "gpt-4o")
	if err != nil {
		t.Fatalf("should not error with no profiles: %v", err)
	}
}

// --- Integration: createWizardProvider + updateAllProfilesProviderAndModel ---

func TestWizardIntegration_ProviderAndProfilesLinked(t *testing.T) {
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

	err = app.updateAllProfilesProviderAndModel(providerID, "gpt-4o-mini")
	if err != nil {
		t.Fatalf("updateAllProfilesProviderAndModel: %v", err)
	}

	// Profile should reference the created provider
	profile, err := pm.Get(slug)
	if err != nil {
		t.Fatalf("get profile: %v", err)
	}

	if profile.Chat.LLMProvider != "openai-default" {
		t.Errorf("LLMProvider: got %s, want openai-default", profile.Chat.LLMProvider)
	}
	if profile.Chat.Model != "gpt-4o-mini" {
		t.Errorf("Model: got %s, want gpt-4o-mini", profile.Chat.Model)
	}

	// Provider should exist in registry
	provider := app.llmRegistry.Get(profile.Chat.LLMProvider)
	if provider == nil {
		t.Fatal("provider referenced by profile not found in registry")
	}
	if provider.BaseURL != "https://api.openai.com/v1" {
		t.Errorf("provider BaseURL: got %s", provider.BaseURL)
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
