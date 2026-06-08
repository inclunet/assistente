package usecases_test

import (
	"context"
	"errors"
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
	t.Setenv("USERPROFILE", tmp) // Windows: os.UserHomeDir() usa USERPROFILE
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
	return newTestUseCaseWithProviders(t, profileMgr)
}

func newTestUseCaseWithProviders(t *testing.T, profileMgr *profiles.Manager, providerConfigs ...*llm.ProviderConfig) *usecases.SendMessageUseCase {
	t.Helper()
	llmRegistry := llm.NewProviderRegistry()
	provSvc := providers.NewService(providers.ServiceConfig{
		Registry: llmRegistry,
		Store:    providers.NewDBStore(),
	})
	if len(providerConfigs) > 0 {
		ctx := database.WithUserID(context.Background(), "test-user")
		for _, provider := range providerConfigs {
			if err := llmRegistry.Register(provider); err != nil {
				t.Fatalf("register provider %s: %v", provider.ID, err)
			}
		}
		if err := providers.NewDBStore().Save(ctx, providerConfigs); err != nil {
			t.Fatalf("save providers: %v", err)
		}
	}
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

func createRetryableUserMessage(t *testing.T, ctx context.Context) (conversationID string, userMessageID string) {
	t.Helper()
	conv, err := database.CreateConversationWithContext(ctx, "retry-conv", "")
	if err != nil {
		t.Fatalf("create conversation: %v", err)
	}
	userMsg, err := database.CreateMessageWithContext(ctx, database.MessageOptions{
		ConversationID: conv.ID,
		Role:           "user",
		Content:        "mensagem original",
		Source:         "wails",
	})
	if err != nil {
		t.Fatalf("create user message: %v", err)
	}
	return conv.ID, userMsg.ID
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
	// Ctx precisa carregar userID por causa do fail-closed em Execute (B14 / AEP-0052).
	ctx := database.WithUserID(context.Background(), "test-user")
	_, err := uc.Execute(usecases.SendMessageRequest{
		Ctx:            ctx,
		ConversationID: "1",
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

// TestSendMessageUseCase_RejectsUnauthenticatedContext garante o fail-closed
// de B14: ctx sem userID retorna ErrUserScopeRequired antes de qualquer
// query no banco/registry.
func TestSendMessageUseCase_RejectsUnauthenticatedContext(t *testing.T) {
	setupTestDB(t)
	mgr := setupProfileDir(t)
	uc := newTestUseCase(t, mgr)

	_, err := uc.Execute(usecases.SendMessageRequest{
		Ctx:            context.Background(),
		ConversationID: "1",
		UserContent:    "hello",
		Source:         "test",
	})
	if err == nil {
		t.Fatal("ctx sem userID deveria falhar")
	}
	if !errors.Is(err, database.ErrUserScopeRequired) {
		t.Errorf("esperava ErrUserScopeRequired, recebeu: %v", err)
	}
}

func TestSendMessageUseCase_RejectsAssistantPrefillWithoutRetryMessage(t *testing.T) {
	setupTestDB(t)
	mgr := setupProfileDir(t)
	uc := newTestUseCase(t, mgr)

	ctx := database.WithUserID(context.Background(), "test-user")
	_, err := uc.Execute(usecases.SendMessageRequest{
		Ctx:            ctx,
		ConversationID: "1",
		UserContent:    "hello",
		Source:         "test",
		Params:         llm.ChatParams{AllowAssistantPrefill: true},
	})
	if err == nil {
		t.Fatal("expected AllowAssistantPrefill without RetryMessageID to fail")
	}
	if !strings.Contains(err.Error(), "RetryMessage") {
		t.Fatalf("expected RetryMessage error, got %q", err.Error())
	}
}

func TestSendMessageUseCase_RejectsAssistantPrefillWhenProfileDisablesContinue(t *testing.T) {
	setupTestDB(t)
	ctx := database.WithUserID(context.Background(), "test-user")
	mgr := setupProfileDir(t)
	profile := minValidProfile("Continue Disabled", "openai-real")
	disabled := false
	profile.Chat.StreamingRecoveryShowContinue = &disabled
	setupProfileWith(t, mgr, profile)
	uc := newTestUseCaseWithProviders(t, mgr, &llm.ProviderConfig{
		ID:        "openai-real",
		Name:      "OpenAI Real",
		Type:      llm.ProviderOpenAI,
		APIFormat: llm.APIFormatOpenAIResponses,
		BaseURL:   "https://api.openai.com/v1",
	})
	conversationID, userMessageID := createRetryableUserMessage(t, ctx)

	_, err := uc.Execute(usecases.SendMessageRequest{
		Ctx:            ctx,
		ConversationID: conversationID,
		RetryMessageID: userMessageID,
		Source:         "test",
		Params:         llm.ChatParams{AllowAssistantPrefill: true},
	})
	if err == nil {
		t.Fatal("expected disabled continue profile to fail")
	}
	if !strings.Contains(err.Error(), "desabilitada") {
		t.Fatalf("expected disabled continue error, got %q", err.Error())
	}
}

// TestSendMessageUseCase_FallsBackToUserMessageWhenProviderUnsupported cobre o
// caminho (b) do Issue #124: o provider/modelo NÃO suporta assistant prefill mas
// a continuação está habilitada no perfil. Em vez de falhar, o UC deve recorrer
// ao fallback por mensagem de usuário e prosseguir o streaming (sem erro síncrono).
func TestSendMessageUseCase_FallsBackToUserMessageWhenProviderUnsupported(t *testing.T) {
	setupTestDB(t)
	ctx := database.WithUserID(context.Background(), "test-user")
	mgr := setupProfileDir(t)
	setupProfileWith(t, mgr, minValidProfile("Unsupported Prefill Provider", "openai-compatible"))
	uc := newTestUseCaseWithProviders(t, mgr, &llm.ProviderConfig{
		ID:        "openai-compatible",
		Name:      "OpenAI Compatible",
		Type:      llm.ProviderOpenAI,
		APIFormat: llm.APIFormatOpenAI,
		// Endpoint local que recusa conexão imediatamente: o streaming roda em
		// goroutine após o retorno; o teste só valida a decisão síncrona (sem erro).
		BaseURL: "http://127.0.0.1:1/v1",
	})
	conversationID, userMessageID := createRetryableUserMessage(t, ctx)

	gotConversationID, err := uc.Execute(usecases.SendMessageRequest{
		Ctx:            ctx,
		ConversationID: conversationID,
		RetryMessageID: userMessageID,
		Source:         "test",
		Params:         llm.ChatParams{AllowAssistantPrefill: true},
	})
	if err != nil {
		t.Fatalf("expected fallback to user message (no sync error), got %v", err)
	}
	if gotConversationID != conversationID {
		t.Fatalf("expected conversationID %q, got %q", conversationID, gotConversationID)
	}
}

// TestSendMessageUseCase_RejectsAssistantPrefillWhenContinueDisabledEvenIfUnsupported
// cobre o caminho (c): mesmo com provider sem suporte a prefill, se o perfil
// desabilita a continuação manual o backend deve falhar fechado (sem fallback).
func TestSendMessageUseCase_RejectsAssistantPrefillWhenContinueDisabledEvenIfUnsupported(t *testing.T) {
	setupTestDB(t)
	ctx := database.WithUserID(context.Background(), "test-user")
	mgr := setupProfileDir(t)
	profile := minValidProfile("Continue Disabled Unsupported", "openai-compatible")
	disabled := false
	profile.Chat.StreamingRecoveryShowContinue = &disabled
	setupProfileWith(t, mgr, profile)
	uc := newTestUseCaseWithProviders(t, mgr, &llm.ProviderConfig{
		ID:        "openai-compatible",
		Name:      "OpenAI Compatible",
		Type:      llm.ProviderOpenAI,
		APIFormat: llm.APIFormatOpenAI,
		BaseURL:   "https://example.com/v1",
	})
	conversationID, userMessageID := createRetryableUserMessage(t, ctx)

	_, err := uc.Execute(usecases.SendMessageRequest{
		Ctx:            ctx,
		ConversationID: conversationID,
		RetryMessageID: userMessageID,
		Source:         "test",
		Params:         llm.ChatParams{AllowAssistantPrefill: true},
	})
	if err == nil {
		t.Fatal("expected disabled continue profile to fail even when prefill unsupported")
	}
	if !strings.Contains(err.Error(), "desabilitada") {
		t.Fatalf("expected disabled continue error, got %q", err.Error())
	}
}

// TestSendMessageUseCase_ReturnsErrorWhenProviderNotFound verifica que o UC retorna
// erro quando o provedor referenciado no perfil não existe no registry.
func TestSendMessageUseCase_ReturnsErrorWhenProviderNotFound(t *testing.T) {
	setupTestDB(t)
	ctx := database.WithUserID(context.Background(), "test-user")

	// Insere um provider no DB para que Count() > 0 e PrepareContext passe o check inicial.
	store := providers.NewDBStore()
	if err := store.Save(ctx, []*llm.ProviderConfig{{
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

	conv, err := database.CreateConversationWithContext(ctx, "test-conv", "")
	if err != nil {
		t.Fatalf("create conversation: %v", err)
	}

	uc := newTestUseCase(t, mgr)
	_, err = uc.Execute(usecases.SendMessageRequest{
		Ctx:            ctx,
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

func TestSendMessageUseCase_RetryExistingUserMessageDoesNotDuplicateUserRow(t *testing.T) {
	setupTestDB(t)
	ctx := database.WithUserID(context.Background(), "test-user")

	store := providers.NewDBStore()
	if err := store.Save(ctx, []*llm.ProviderConfig{{
		ID:      "dummy",
		Name:    "Dummy",
		Type:    "openai",
		BaseURL: "http://localhost",
	}}); err != nil {
		t.Fatalf("save dummy provider: %v", err)
	}

	mgr := setupProfileDir(t)
	setupProfileWith(t, mgr, minValidProfile("Retry Existing Message", "nonexistent"))

	conv, err := database.CreateConversationWithContext(ctx, "retry-conv", "")
	if err != nil {
		t.Fatalf("create conversation: %v", err)
	}
	userMsg, err := database.CreateMessageWithContext(ctx, database.MessageOptions{
		ConversationID: conv.ID,
		Role:           "user",
		Content:        "mensagem original",
		Source:         "wails",
	})
	if err != nil {
		t.Fatalf("create user message: %v", err)
	}

	uc := newTestUseCase(t, mgr)
	_, err = uc.Execute(usecases.SendMessageRequest{
		Ctx:            ctx,
		ConversationID: conv.ID,
		RetryMessageID: userMsg.ID,
		Source:         "wails",
		Params:         llm.ChatParams{Model: "gpt-4o"},
	})
	if err == nil {
		t.Fatal("expected error when provider not found during retry, got nil")
	}

	var count int64
	if err := database.DB().Model(&database.ChatMessage{}).Where("conversation_id = ?", conv.ID).Count(&count).Error; err != nil {
		t.Fatalf("count messages: %v", err)
	}
	if count != 1 {
		t.Fatalf("retry should not duplicate user messages, got %d rows", count)
	}
}
