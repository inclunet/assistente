package usecases_test

import (
	"context"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"assistente/adapters/noop"
	"assistente/internal/agent"
	"assistente/internal/chat"
	"assistente/internal/config"
	"assistente/internal/configdir"
	"assistente/internal/core/usecases"
	"assistente/internal/database"
	"assistente/internal/events"
	"assistente/internal/llm"
	"assistente/internal/profiles"
	"assistente/internal/providers"
	"assistente/internal/speech"
)

// setupTestDB inicializa um banco SQLite em memória e injeta via database.SetDB.
func setupTestDB(t *testing.T) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	if err := db.AutoMigrate(
		&database.Conversation{},
		&database.ChatMessage{},
		&database.CredentialEntry{},
		&database.CredentialKeyWrap{},
		&database.LLMProvider{},
		&database.TaskListWorkflow{},
		&database.TaskList{},
		&database.Task{},
		&database.TaskNote{},
	); err != nil {
		t.Fatalf("auto-migrate: %v", err)
	}
	database.SetDB(db)
}

// setupProfileDir prepara um HOME temporário isolado para o configdir resolver.
// Retorna o *profiles.Manager pronto, com EnsureDefaults() já chamado.
func setupProfileDir(t *testing.T) *profiles.Manager {
	t.Helper()
	tmp := t.TempDir()
	configdir.ResetForTests()
	t.Setenv("HOME", tmp)
	t.Setenv("USERPROFILE", tmp)
	t.Cleanup(configdir.ResetForTests)
	mgr := profiles.NewManager()
	_ = mgr.EnsureDefaults() // apenas cria o diretório base
	return mgr
}

// setupProfileWith cria e ativa um perfil com os dados fornecidos.
// Exige que o perfil passe em profiles.Validate() — use campos mínimos válidos.
func setupProfileWith(t *testing.T, mgr *profiles.Manager, p profiles.Profile) {
	t.Helper()
	slug, err := mgr.Create(&p)
	if err != nil {
		t.Fatalf("create profile: %v", err)
	}
	if err := mgr.SetActive(slug); err != nil {
		t.Fatalf("set active profile: %v", err)
	}
}

// minValidProfile retorna um perfil válido mínimo para os testes.
// temperature=0.7, maxTokens=4096, topP=1.0, responseTimeout=180.
func minValidProfile(name, llmProvider string) profiles.Profile {
	return profiles.Profile{
		Name: name,
		Chat: profiles.ChatConfig{
			LLMProvider:     llmProvider,
			Temperature:     0.7,
			MaxTokens:       4096,
			TopP:            1.0,
			ResponseTimeout: 180,
		},
	}
}

// newTestUseCase cria um SendMessageUseCase com dependências mínimas para testes.
func newTestUseCase(t *testing.T, profileMgr *profiles.Manager) *usecases.SendMessageUseCase {
	t.Helper()

	llmRegistry := llm.NewProviderRegistry()
	provSvc := providers.NewService(providers.ServiceConfig{
		Registry: llmRegistry,
		Store:    providers.NewDBStore(),
	})
	settingsSvc := config.NewSettingsService(config.SettingsServiceConfig{})
	speechSvc := speech.NewService(speech.ServiceConfig{
		Emitter:  events.NoopEmitter{},
		Registry: llmRegistry,
	})
	streamMgr := chat.NewStreamingManager(nil)
	agentSvc := agent.NewService(agent.ServiceConfig{
		Emitter: events.NoopEmitter{},
		MsgRepo: chat.NewDBMessageStore(),
	})
	interactor := chat.NewInteractor(chat.InteractorConfig{
		Emitter:     events.NoopEmitter{},
		Repo:        chat.NewDBMessageStore(),
		ConvRepo:    chat.NewDBConversationStore(),
		ProviderSvc: provSvc,
		ProfileMgr:  profileMgr,
	})

	return usecases.NewSendMessageUseCase(usecases.SendMessageConfig{
		ChatInteractor: interactor,
		ProviderSvc:    provSvc,
		StreamMgr:      streamMgr,
		SpeechSvc:      speechSvc,
		SettingsSvc:    settingsSvc,
		Emitter:        noop.EmitterAdapter{},
		AgentSvc:       agentSvc,
	})
}

// TestSendMessageUseCase_ReturnsErrorWhenNoLLMProviders verifica que o UC retorna
// erro síncrono quando não há nenhum provedor LLM cadastrado no banco.
// O check de Count()==0 é feito em PrepareContext antes da resolução de provider do perfil.
func TestSendMessageUseCase_ReturnsErrorWhenNoLLMProviders(t *testing.T) {
	setupTestDB(t) // banco vazio: 0 providers
	mgr := setupProfileDir(t)
	// GetActive() fallback -> DefaultProfile() com LLMProvider="$default", válido
	uc := newTestUseCase(t, mgr)

	// ConversationID pode ser qualquer valor pois Count==0 falha antes do check de conv.
	_, err := uc.Execute(usecases.SendMessageRequest{
		Ctx:            context.Background(),
		ConversationID: 1,
		UserContent:    "hello",
		Source:         "test",
	})

	if err == nil {
		t.Fatal("expected error when no LLM providers in DB, got nil")
	}
	if !strings.Contains(err.Error(), "provedor LLM") {
		t.Errorf("unexpected error message: %q", err.Error())
	}
}

// TestSendMessageUseCase_ReturnsErrorWhenProviderNotFound verifica que o UC retorna
// erro quando o provedor referenciado no perfil não existe no registry.
func TestSendMessageUseCase_ReturnsErrorWhenProviderNotFound(t *testing.T) {
	setupTestDB(t)

	// Insere um provider no DB para que Count() > 0 e PrepareContext passe o check inicial.
	store := providers.NewDBStore()
	if err := store.Save([]*llm.ProviderConfig{{
		ID:      "dummy",
		Name:    "Dummy",
		Type:    "openai",
		BaseURL: "http://localhost",
	}}); err != nil {
		t.Fatalf("save dummy provider: %v", err)
	}

	mgr := setupProfileDir(t)
	// Perfil aponta para "nonexistent" — diferente do "dummy" no BD.
	setupProfileWith(t, mgr, minValidProfile("Test Provider", "nonexistent"))

	conv, err := database.CreateConversation("test-conv", "")
	if err != nil {
		t.Fatalf("create conversation: %v", err)
	}

	uc := newTestUseCase(t, mgr)
	_, err = uc.Execute(usecases.SendMessageRequest{
		Ctx:            context.Background(),
		ConversationID: conv.ID,
		UserContent:    "hello",
		Source:         "test",
		Params:         llm.ChatParams{Model: "gpt-4o"},
	})

	if err == nil {
		t.Fatal("expected error when provider not found, got nil")
	}
	if !strings.Contains(err.Error(), "nonexistent") {
		t.Errorf("expected error to mention provider ID, got: %q", err.Error())
	}
}
