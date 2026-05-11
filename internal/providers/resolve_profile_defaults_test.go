package providers

import (
	"context"
	"testing"

	"assistente/internal/llm"
	"assistente/internal/profiles"
)

// Helper: monta um Service com MemoryStore e dois providers (um default
// "openai-default" + um "ollama-local"). Sufficient para validar o
// caminho de resolução de defaults sem subir DB ou Wails.
func setupResolveTestService(t *testing.T) (*Service, context.Context) {
	t.Helper()
	store := NewMemoryStore()
	registry := llm.NewProviderRegistry()
	svc := NewService(ServiceConfig{
		Registry: registry,
		Store:    store,
	})

	openai := &llm.ProviderConfig{
		ID:           "openai-default",
		Name:         "OpenAI",
		Type:         llm.ProviderOpenAI,
		BaseURL:      "https://api.openai.com/v1",
		Model:        "gpt-4o-mini",
		DefaultModel: "gpt-4o-mini",
	}
	ollama := &llm.ProviderConfig{
		ID:           "ollama-local",
		Name:         "Ollama (Local)",
		Type:         llm.ProviderOllama,
		BaseURL:      "http://localhost:11434/api",
		Model:        "llama3.2",
		DefaultModel: "llama3.2",
		AuthMode:     llm.AuthModeNone,
	}
	if err := registry.Register(openai); err != nil {
		t.Fatalf("register openai: %v", err)
	}
	if err := registry.Register(ollama); err != nil {
		t.Fatalf("register ollama: %v", err)
	}
	ctx := context.Background()
	if err := store.Save(ctx, []*llm.ProviderConfig{openai, ollama}); err != nil {
		t.Fatalf("store.Save: %v", err)
	}
	if err := store.SetDefault(ctx, "openai-default"); err != nil {
		t.Fatalf("SetDefault: %v", err)
	}
	return svc, ctx
}

// Bug 4 (regressão): se o profile fixou LLMProvider="ollama-local" mas
// deixou Model em $default, ResolveProfileDefaults DEVE puxar
// DefaultModel do **provider escolhido** (ollama-local → llama3.2),
// nunca do default global (openai-default → gpt-4o-mini). Esse vazamento
// cross-provider gerava o sintoma "troquei o perfil mas o app continua
// usando o modelo da OpenAI".
func TestResolveProfileDefaults_ProviderEscolhidoDefineModelo(t *testing.T) {
	svc, ctx := setupResolveTestService(t)

	profile := &profiles.Profile{
		Name: "Modelo Local",
		Chat: profiles.ChatConfig{
			LLMProvider: "ollama-local",
			Model:       profiles.DefaultProviderSentinel,
		},
		Voice: profiles.VoiceConfig{},
		Input: profiles.InputConfig{},
	}

	resolved := svc.ResolveProfileDefaults(ctx, profile)
	if resolved == nil {
		t.Fatal("ResolveProfileDefaults retornou nil")
	}
	if resolved.Chat.LLMProvider != "ollama-local" {
		t.Errorf("LLMProvider mudou para %q; deveria preservar 'ollama-local'", resolved.Chat.LLMProvider)
	}
	if resolved.Chat.Model != "llama3.2" {
		t.Errorf("Model = %q; esperado 'llama3.2' (DefaultModel do provider escolhido) e NÃO 'gpt-4o-mini'", resolved.Chat.Model)
	}
}

// Quando ambos LLMProvider e Model estão em $default, a resolução cai
// na rota antiga: provider+modelo do default global.
func TestResolveProfileDefaults_AmbosDefaultUsamGlobal(t *testing.T) {
	svc, ctx := setupResolveTestService(t)

	profile := &profiles.Profile{
		Name: "Padrão",
		Chat: profiles.ChatConfig{
			LLMProvider: profiles.DefaultProviderSentinel,
			Model:       profiles.DefaultProviderSentinel,
		},
	}

	resolved := svc.ResolveProfileDefaults(ctx, profile)
	if resolved.Chat.LLMProvider != "openai-default" {
		t.Errorf("LLMProvider = %q; esperado 'openai-default'", resolved.Chat.LLMProvider)
	}
	if resolved.Chat.Model != "gpt-4o-mini" {
		t.Errorf("Model = %q; esperado 'gpt-4o-mini'", resolved.Chat.Model)
	}
}

// Profile fixou ambos provider e modelo: ResolveProfileDefaults é no-op.
func TestResolveProfileDefaults_AmbosFixosNaoMexe(t *testing.T) {
	svc, ctx := setupResolveTestService(t)

	profile := &profiles.Profile{
		Name: "Custom",
		Chat: profiles.ChatConfig{
			LLMProvider: "ollama-local",
			Model:       "qwen2.5-coder:7b",
		},
	}

	resolved := svc.ResolveProfileDefaults(ctx, profile)
	if resolved.Chat.LLMProvider != "ollama-local" {
		t.Errorf("provider mudou: %q", resolved.Chat.LLMProvider)
	}
	if resolved.Chat.Model != "qwen2.5-coder:7b" {
		t.Errorf("modelo mudou: %q", resolved.Chat.Model)
	}
}

// Bug 7 (regressão): EffectiveAuthMode retorna AuthModeNone quando o
// provider tem CredentialPattern vazio e AuthMode não foi setado
// explicitamente — preserva o contrato histórico onde "sem pattern"
// significava "sem auth".
func TestEffectiveAuthMode_SemPatternInfereNone(t *testing.T) {
	cfg := &llm.ProviderConfig{
		ID:                "ollama-legacy",
		BaseURL:           "http://localhost:11434/api",
		CredentialPattern: "",
		AuthMode:          "",
	}
	if got := cfg.EffectiveAuthMode(); got != llm.AuthModeNone {
		t.Errorf("EffectiveAuthMode = %q; esperado %q (compat com configs antigas)", got, llm.AuthModeNone)
	}
}

func TestEffectiveAuthMode_ComPatternInfereRequired(t *testing.T) {
	cfg := &llm.ProviderConfig{
		ID:                "openai",
		BaseURL:           "https://api.openai.com/v1",
		CredentialPattern: "api.openai.com",
	}
	if got := cfg.EffectiveAuthMode(); got != llm.AuthModeRequired {
		t.Errorf("EffectiveAuthMode = %q; esperado %q", got, llm.AuthModeRequired)
	}
}

func TestEffectiveAuthMode_RespeitaExplicito(t *testing.T) {
	cfg := &llm.ProviderConfig{
		ID:                "litellm-optional",
		BaseURL:           "http://localhost:4000",
		CredentialPattern: "localhost:4000",
		AuthMode:          llm.AuthModeOptional,
	}
	if got := cfg.EffectiveAuthMode(); got != llm.AuthModeOptional {
		t.Errorf("EffectiveAuthMode explícito ignorado: got %q", got)
	}
}
