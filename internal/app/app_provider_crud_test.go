package app

import (
	"context"
	"testing"

	"assistente/adapters/noop"
	"assistente/controllers"
	"assistente/internal/credentials"
	"assistente/internal/database"
	"assistente/internal/llm"
	"assistente/internal/providers"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func setupTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("Falha ao criar banco de dados em memória: %v", err)
	}

	// Auto-migrate
	if err := db.AutoMigrate(&database.LLMProvider{}); err != nil {
		t.Fatalf("Falha ao migrar tabelas: %v", err)
	}

	// Define o banco globalmente para os testes
	database.SetDB(db)

	return db
}

// newAppForTest cria um App configurado para testes com providerSvc inicializado.
func newAppForTest(credMgr *credentials.Manager, llmRegistry *llm.ProviderRegistry) *App {
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
	return a
}

// TestCreateProviderWithAPIKey valida a criação de provider com API key
func TestCreateProviderWithAPIKey(t *testing.T) {
	// Setup database
	_ = setupTestDB(t)

	// Setup
	credMgr := credentials.NewManager([]byte("test-key-exactly-32-bytes-long!!"))
	llmRegistry := llm.NewProviderRegistry()

	// Simular App com componentes necessários
	app := newAppForTest(credMgr, llmRegistry)

	// Request
	req := CreateLLMProviderRequest{
		ID:      "test-openai",
		Name:    "Test OpenAI",
		Type:    "openai",
		BaseURL: "https://api.openai.com/v1",
		APIKey:  "sk-test123456",
	}

	// Criar provider
	resp, err := app.CreateLLMProvider(req)
	if err != nil {
		t.Fatalf("CreateLLMProvider falhou: %v", err)
	}

	// Validar resposta
	if resp["id"] != "test-openai" {
		t.Errorf("ID incorreto: %v", resp["id"])
	}
	if resp["credential_pattern"] != "api.openai.com" {
		t.Errorf("Pattern incorreto: %v", resp["credential_pattern"])
	}
	if !resp["credential_configured"].(bool) {
		t.Error("Credencial deveria estar configurada")
	}

	// Verificar que provider foi registrado
	provider := llmRegistry.Get("test-openai")
	if provider == nil {
		t.Fatal("Provider não foi registrado")
	}

	// Verificar que credencial foi salva
	cred, err := credMgr.GetByPattern("api.openai.com")
	if err != nil {
		t.Fatalf("Credencial não foi salva: %v", err)
	}
	if cred.Token != "sk-test123456" {
		t.Errorf("Token incorreto: %s", cred.Token)
	}
}

// TestUpdateProvider valida atualização de provider
func TestUpdateProvider(t *testing.T) {
	// Setup database
	_ = setupTestDB(t)

	credMgr := credentials.NewManager([]byte("test-key-exactly-32-bytes-long!!"))
	llmRegistry := llm.NewProviderRegistry()

	app := newAppForTest(credMgr, llmRegistry)

	// Criar provider inicial
	initialReq := CreateLLMProviderRequest{
		ID:      "test-update",
		Name:    "Initial Name",
		Type:    "openai",
		BaseURL: "https://api.openai.com/v1",
		APIKey:  "sk-old-key",
	}
	_, err := app.CreateLLMProvider(initialReq)
	if err != nil {
		t.Fatalf("Setup falhou: %v", err)
	}

	// Atualizar provider
	updateReq := UpdateLLMProviderRequest{
		Name:   "Updated Name",
		APIKey: "sk-new-key",
	}
	resp, err := app.UpdateLLMProvider("test-update", updateReq)
	if err != nil {
		t.Fatalf("UpdateLLMProvider falhou: %v", err)
	}

	// Validar resposta
	if resp["name"] != "Updated Name" {
		t.Errorf("Nome não foi atualizado: %v", resp["name"])
	}

	// Verificar que credencial foi atualizada
	cred, err := credMgr.GetByPattern("api.openai.com")
	if err != nil {
		t.Fatalf("Credencial não encontrada: %v", err)
	}
	if cred.Token != "sk-new-key" {
		t.Errorf("Token não foi atualizado: %s", cred.Token)
	}
}

// TestDeleteProvider valida remoção de provider
func TestDeleteProvider(t *testing.T) {
	// Setup database
	_ = setupTestDB(t)

	credMgr := credentials.NewManager([]byte("test-key-exactly-32-bytes-long!!"))
	llmRegistry := llm.NewProviderRegistry()

	app := newAppForTest(credMgr, llmRegistry)

	ctx := context.Background()

	// Criar provider
	req := CreateLLMProviderRequest{
		ID:      "test-delete",
		Name:    "To Delete",
		Type:    "openai",
		BaseURL: "https://api.openai.com/v1",
		APIKey:  "sk-delete-me",
	}
	_, err := app.CreateLLMProvider(req)
	if err != nil {
		t.Fatalf("Setup falhou: %v", err)
	}

	// Deletar provider
	err = app.DeleteLLMProvider(ctx, "test-delete")
	if err != nil {
		t.Fatalf("DeleteLLMProvider falhou: %v", err)
	}

	// Verificar que provider foi removido
	provider := llmRegistry.Get("test-delete")
	if provider != nil {
		t.Error("Provider não foi removido do registry")
	}
}

// TestListProvidersWithStatus valida listagem com status de credenciais
func TestListProvidersWithStatus(t *testing.T) {
	credMgr := credentials.NewManager([]byte("test-key-exactly-32-bytes-long!!"))
	llmRegistry := llm.NewProviderRegistry()

	app := newAppForTest(credMgr, llmRegistry)

	// Criar provider COM credencial
	req1 := CreateLLMProviderRequest{
		ID:      "with-cred",
		Name:    "With Credential",
		Type:    "openai",
		BaseURL: "https://api.openai.com/v1",
		APIKey:  "sk-has-key",
	}
	_, _ = app.CreateLLMProvider(req1)

	// Criar provider SEM credencial (Ollama local)
	req2 := CreateLLMProviderRequest{
		ID:      "no-cred",
		Name:    "No Credential",
		Type:    "ollama",
		BaseURL: "http://localhost:11434/api",
		APIKey:  "", // Sem credencial
	}
	_, _ = app.CreateLLMProvider(req2)

	// Listar providers
	providers := app.GetLLMProvidersWithStatus()

	// Validar
	if len(providers) != 2 {
		t.Fatalf("Esperado 2 providers, obteve %d", len(providers))
	}

	// Procurar provider com credencial
	var withCred map[string]interface{}
	for _, p := range providers {
		if p["id"] == "with-cred" {
			withCred = p
			break
		}
	}

	if withCred == nil {
		t.Fatal("Provider 'with-cred' não encontrado")
	}

	if !withCred["credential_configured"].(bool) {
		t.Error("Credencial deveria estar configurada")
	}
}
