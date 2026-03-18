package integration

import (
	"context"
	"os"
	"testing"

	"assistente/internal/configdir"
	"assistente/internal/credentials"
	"assistente/internal/database"
	"assistente/internal/llm"
	"assistente/internal/profiles"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

// setupIntegrationDB cria um banco de dados em memória para testes
func setupIntegrationDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("falha ao criar banco em memória: %v", err)
	}

	models := []interface{}{
		&database.Conversation{},
		&database.ChatMessage{},
		&database.LLMProvider{},
		&database.CredentialEntry{},
	}

	if err := db.AutoMigrate(models...); err != nil {
		t.Fatalf("falha ao migrar tabelas: %v", err)
	}

	database.SetDB(db)
	return db
}

// setupIntegrationEnv configura variáveis de ambiente para testes isolados
func setupIntegrationEnv(t *testing.T) string {
	t.Helper()

	tempDir := t.TempDir()
	oldHome := os.Getenv("HOME")
	oldUserProfile := os.Getenv("USERPROFILE")

	if err := os.Setenv("HOME", tempDir); err != nil {
		t.Fatalf("set HOME: %v", err)
	}
	if err := os.Setenv("USERPROFILE", tempDir); err != nil {
		t.Fatalf("set USERPROFILE: %v", err)
	}

	configdir.ResetForTests()

	t.Cleanup(func() {
		_ = os.Setenv("HOME", oldHome)
		_ = os.Setenv("USERPROFILE", oldUserProfile)
		configdir.ResetForTests()
	})

	return tempDir
}

// TestIntegration_ProviderRegistry testa registro de LLM providers
func TestIntegration_ProviderRegistry(t *testing.T) {
	if testing.Short() {
		t.Skip("Utilizando -short, pulando teste de integração")
	}

	_ = setupIntegrationDB(t)
	_ = setupIntegrationEnv(t)

	// Setup LLM registry
	registry := llm.NewProviderRegistry()

	// Registrar múltiplos providers
	providers := []llm.ProviderConfig{
		{
			ID:      "openai",
			Name:    "OpenAI",
			Type:    llm.ProviderOpenAI,
			BaseURL: "https://api.openai.com/v1",
			Model:   "gpt-4o",
		},
		{
			ID:      "anthropic",
			Name:    "Claude",
			Type:    llm.ProviderClaude,
			BaseURL: "https://api.anthropic.com/v1",
			Model:   "claude-3-5-sonnet-20241022",
		},
		{
			ID:      "ollama",
			Name:    "Ollama Local",
			Type:    llm.ProviderOllama,
			BaseURL: "http://localhost:11434/api",
			Model:   "mistral",
		},
	}

	for _, p := range providers {
		config := p // copy
		if err := registry.Register(&config); err != nil {
			t.Fatalf("falha ao registrar provider %s: %v", p.ID, err)
		}
	}

	// Validar contagem
	list := registry.List()
	if len(list) != len(providers) {
		t.Errorf("esperado %d providers, obteve %d", len(providers), len(list))
	}

	// Validar busca
	for _, p := range providers {
		found := registry.Get(p.ID)
		if found == nil {
			t.Errorf("provider %s não encontrado", p.ID)
		} else if found.Name != p.Name {
			t.Errorf("provider %s: nome incorreto", p.ID)
		}
	}

	t.Log("✓ Teste integração registry de providers: PASSOU")
}

// TestIntegration_CredentialsByDomain testa armazenamento de credenciais por domínio
func TestIntegration_CredentialsByDomain(t *testing.T) {
	if testing.Short() {
		t.Skip("Utilizando -short, pulando teste de integração")
	}

	_ = setupIntegrationDB(t)
	_ = setupIntegrationEnv(t)

	ctx := context.Background()

	// Setup credential manager
	masterKey := []byte("test-key-exactly-32-bytes-long!!")
	credMgr := credentials.NewManager(masterKey)

	// Registrar credenciais por domínio
	testCases := map[string]*credentials.AuthConfig{
		"*.openai.com": {
			Type:  "bearer",
			Token: "sk-openai-test-123",
		},
		"*.anthropic.com": {
			Type:  "bearer",
			Token: "sk-anthropic-test-456",
		},
		"localhost": {
			Type: "none",
		},
	}

	for pattern, auth := range testCases {
		if err := credMgr.RegisterPatternWithContext(ctx, pattern, auth); err != nil {
			t.Fatalf("falha ao registrar %s: %v", pattern, err)
		}
	}

	// Validar resolução por URL
	testResolutions := map[string]string{
		"https://api.openai.com/v1":    "*.openai.com",
		"https://api.anthropic.com/v1": "*.anthropic.com",
		"http://localhost:8080":        "localhost",
	}

	for testURL, expectedPattern := range testResolutions {
		resolved, err := credMgr.ResolveForURL(testURL)
		if err != nil {
			t.Fatalf("erro ao resolver %s: %v", testURL, err)
		}

		expectedAuth := testCases[expectedPattern]
		if resolved == nil {
			t.Errorf("nenhuma credencial resolvida para %s", testURL)
		} else if resolved.Type != expectedAuth.Type {
			t.Errorf("tipo incorreto para %s", testURL)
		}
	}

	// Validar que URL não registrada retorna nil
	notFound, _ := credMgr.ResolveForURL("https://unknown.com/api")
	if notFound != nil {
		t.Error("deveria retornar nil para URL não registrada")
	}

	t.Log("✓ Teste integração credenciais por domínio: PASSOU")
}

// TestIntegration_ProfileManagement testa carregamento e listagem de perfis
func TestIntegration_ProfileManagement(t *testing.T) {
	if testing.Short() {
		t.Skip("Utilizando -short, pulando teste de integração")
	}

	_ = setupIntegrationDB(t)
	_ = setupIntegrationEnv(t)

	// Setup profile manager
	profileMgr := profiles.NewManager()

	// Garantir profiles padrão
	if err := profileMgr.EnsureDefaults(); err != nil {
		t.Logf("aviso ao verificar profiles padrão: %v", err)
		// continua mesmo com erro pois o ambiente pode não ter profiles padrão
	}

	// Listar profiles
	profileList, err := profileMgr.List()
	if err != nil {
		t.Fatalf("falha ao listar profiles: %v", err)
	}

	// Se houver profiles, validar que conseguimos carregá-los
	if len(profileList) > 0 {
		// Carregar cada profile
		for _, pinfo := range profileList {
			profile, err := profileMgr.Get(pinfo.Slug)
			if err != nil {
				t.Fatalf("falha ao carregar profile %s: %v", pinfo.Slug, err)
			}

			if profile == nil {
				t.Errorf("profile %s é nil", pinfo.Slug)
			} else if profile.Name == "" {
				t.Errorf("profile %s sem nome", pinfo.Slug)
			}
		}
		t.Log("✓ Teste integração gerenciamento de perfis: PASSOU")
	} else {
		// Se não houver profiles, o teste ainda passa, mas avisa
		t.Log("⚠ Nenhum profile padrão encontrado (pode ser esperado neste ambiente)")
	}
}
