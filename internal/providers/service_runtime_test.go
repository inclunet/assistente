package providers

import (
	"context"
	"errors"
	"strings"
	"testing"

	"assistente/internal/database"
	"assistente/internal/llm"
	"assistente/internal/profiles"
	"assistente/internal/sqlite"

	"gorm.io/gorm"
)

func TestChatStartupAcceptsEmptyStore(t *testing.T) {
	cfg := ServiceConfig{Registry: llm.NewProviderRegistry(), Store: NewMemoryStore(), SkipBootRepairs: true}
	svc := NewService(cfg)
	if err := svc.Load(t.Context()); err != nil {
		t.Fatalf("restauração com catálogo de provedores vazio: %v", err)
	}
	cfg.SkipBootRepairs = false
	if err := NewService(cfg).Load(t.Context()); err == nil {
		t.Fatal("bootstrap de configuração perdeu a indicação de cadastro vazio")
	}
}

// Reproduz a escolha de um padrão no desktop após a primeira leitura da CLI.
type defaultChangedStore struct {
	*MemoryStore
	selected string
}

func (s defaultChangedStore) GetDefault(ctx context.Context) (*llm.ProviderConfig, error) {
	current, err := s.MemoryStore.GetDefault(ctx)
	if err != nil {
		return nil, err
	}
	if s.selected != "" {
		err = s.SetDefault(ctx, s.selected)
	}
	return current, err
}

func TestProfileDefaultWithStartupRepairsDisabled(t *testing.T) {
	for _, tc := range []struct {
		name               string
		registered, stored []string
		selected, want     string
	}{
		{name: "padrão escolhido entre leituras", registered: []string{"a", "b"}, stored: []string{"a", "b"}, selected: "b", want: "b"},
		{name: "registro e cadastro divergem", registered: []string{"a", "c"}, stored: []string{"b", "c"}, want: "c"},
		{name: "cadastro vazio", registered: []string{"a"}, want: profiles.DefaultProviderSentinel},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := t.Context()
			store := defaultChangedStore{MemoryStore: NewMemoryStore(), selected: tc.selected}
			registry := llm.NewProviderRegistry()
			makeProvider := func(id string) *llm.ProviderConfig {
				return &llm.ProviderConfig{ID: id, Name: id, Type: llm.ProviderOpenAI, BaseURL: "https://example.org/v1"}
			}
			for _, id := range tc.stored {
				if err := store.Save(ctx, []*llm.ProviderConfig{makeProvider(id)}); err != nil {
					t.Fatal(err)
				}
			}
			for _, id := range tc.registered {
				if err := registry.Register(makeProvider(id)); err != nil {
					t.Fatal(err)
				}
			}
			svc := NewService(ServiceConfig{Registry: registry, Store: store, SkipBootRepairs: true})
			profile := &profiles.Profile{Chat: profiles.ChatConfig{LLMProvider: profiles.DefaultProviderSentinel}}
			resolved := svc.ResolveProfileDefaults(ctx, profile)
			if resolved.Chat.LLMProvider != tc.want {
				t.Fatalf("provedor selecionado=%q, esperado=%q", resolved.Chat.LLMProvider, tc.want)
			}
		})
	}
}

func loadedChatSnapshot(t *testing.T, row database.LLMProvider) (llm.ChatProvider, *llm.ProviderConfig, error) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	t.Cleanup(database.SetDB(db))
	if err := db.AutoMigrate(&database.LLMProvider{}); err != nil {
		t.Fatal(err)
	}
	row.ID, row.Name, row.UserID = "registro", " Cadastro ", "conta"
	if err := db.Create(&row).Error; err != nil {
		t.Fatal(err)
	}
	ctx := database.WithUserID(t.Context(), row.UserID)
	svc := NewService(ServiceConfig{Registry: llm.NewProviderRegistry(), Store: NewDBStore(), SkipBootRepairs: true})
	if err := svc.Load(ctx); err != nil {
		t.Fatal(err)
	}
	return svc.GetChatProviderWithConfigSnapshot(ctx, row.ID)
}

func TestChatSnapshotDeCadastroPersistido(t *testing.T) {
	for _, tc := range []struct {
		name      string
		row       database.LLMProvider
		format    llm.APIFormat
		wantErr   error
		errorText string
	}{
		{name: "inferido por URL", row: database.LLMProvider{Type: "openai", BaseURL: "https://api.openai.com/v1"}, format: llm.APIFormatOpenAIResponses},
		{name: "OAuth", row: database.LLMProvider{Type: "chatgpt", BaseURL: "https://api.openai.com/v1", APIFormat: "openai_responses", CredentialPattern: "oauth:teste", AuthMode: "required"}, format: llm.APIFormatOpenAIResponses},
		{name: "HTTP pendente", row: database.LLMProvider{Type: "custom", BaseURL: "http://example.org/v1", AuthMode: "required", CredentialPattern: "teste"}, wantErr: llm.ErrInsecureHTTPAuthorizationRequired},
		{name: "URL pendente", row: database.LLMProvider{Type: "custom", BaseURL: "://"}, wantErr: llm.ErrInvalidProviderBaseURL},
		{name: "configuração rejeitada", row: database.LLMProvider{Type: "custom", BaseURL: "https://example.org", ReasoningContentMode: "desconhecido"}, errorText: "reasoning_content_mode"},
		{name: "linha ilegível", row: database.LLMProvider{Type: "custom", BaseURL: "https://example.org", ACPArgs: "JSON inválido"}, errorText: "ilegíveis"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			adapter, cfg, err := loadedChatSnapshot(t, tc.row)
			if tc.wantErr != nil || tc.errorText != "" {
				if adapter != nil || cfg != nil || err == nil {
					t.Fatalf("resultado inesperado: config=%v, err=%v", cfg, err)
				}
				if tc.wantErr != nil && !errors.Is(err, tc.wantErr) {
					t.Fatalf("categoria esperada %v, recebida %v", tc.wantErr, err)
				}
				if tc.errorText != "" && !strings.Contains(err.Error(), tc.errorText) {
					t.Fatalf("diagnóstico esperado %q, recebido %v", tc.errorText, err)
				}
				return
			}
			if err != nil || adapter == nil || cfg == nil || cfg.APIFormat != tc.format || cfg.Name != "Cadastro" {
				t.Fatalf("cadastro não preservado: config=%v, err=%v", cfg, err)
			}
		})
	}
}

// Só materializa o adaptador e confere seu cadastro; executado no CI.
func TestACPChatSnapshotDeCadastroPersistido(t *testing.T) {
	adapter, cfg, err := loadedChatSnapshot(t, database.LLMProvider{Type: "custom", APIFormat: "acp", ACPCommand: "agente", ACPArgs: `["--modo"]`, ACPEnv: `{"IDIOMA":"pt"}`, ACPCredentialEnv: `{"TOKEN":" perfil "}`})
	if err != nil || adapter == nil || cfg == nil || len(cfg.ACPArgs) != 1 || cfg.ACPArgs[0] != "--modo" || cfg.ACPEnv["IDIOMA"] != "pt" || cfg.ACPCredentialEnv["TOKEN"] != "perfil" {
		t.Fatalf("cadastro de agente não preservado: config=%v, err=%v", cfg, err)
	}
}
