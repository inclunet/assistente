package main

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"assistente/controllers"
	"assistente/internal/apidto"
	"assistente/internal/credentials"
	"assistente/internal/database"
	"assistente/internal/llm"
	"assistente/internal/providers"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

type providerIntegrationBackend struct {
	mockProviderSetup
	ctx         context.Context
	llm         *controllers.LLMController
	credentials *controllers.CredentialsController
}

func (b *providerIntegrationBackend) GetCredentialForURL(u string) (*apidto.CredentialSummary, error) {
	return b.credentials.GetCredentialForURL(b.ctx, u)
}
func (b *providerIntegrationBackend) ListModelsRaw(r apidto.TestLLMProviderRequest) ([]string, error) {
	return b.llm.ListModelsRaw(b.ctx, r)
}
func (b *providerIntegrationBackend) CreateLLMProvider(r apidto.CreateLLMProviderRequest) (map[string]interface{}, error) {
	b.created = r
	return b.llm.CreateLLMProvider(b.ctx, r)
}

func TestProviderSetupPersistsThroughSharedController(t *testing.T) {
	// O SDK lê essas variáveis mesmo com credencial explícita. O fixture usa
	// apenas seu segredo sintético, sem herdar autenticação da máquina.
	for _, name := range []string{"ANTHROPIC_AUTH_TOKEN", "ANTHROPIC_API_KEY"} {
		t.Setenv(name, "") // registra restauração automática do ambiente original
		if err := os.Unsetenv(name); err != nil {
			t.Fatal(err)
		}
	}
	for _, api := range []string{"openai", "anthropic"} {
		t.Run(api, func(t *testing.T) {
			db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
			if err != nil {
				t.Fatal(err)
			}
			if err := db.AutoMigrate(&database.LLMProvider{}, &database.CredentialEntry{}); err != nil {
				t.Fatal(err)
			}
			previous := database.DB()
			database.SetDB(db)
			t.Cleanup(func() {
				database.SetDB(previous)
				sqlDB, err := db.DB()
				if err == nil {
					_ = sqlDB.Close()
				}
			})
			mgr := credentials.NewManagerWithStore(bytes.Repeat([]byte{4}, 32), credentials.NewDBStore(), true)
			registry := llm.NewProviderRegistry()
			service := providers.NewService(providers.ServiceConfig{Registry: registry, CredMgr: mgr, Store: providers.NewDBStore()})
			b := &providerIntegrationBackend{ctx: database.WithUserID(context.Background(), "cli-user"), llm: controllers.NewLLMController(controllers.LLMControllerConfig{LLMRegistry: registry, ProviderSvc: service}), credentials: controllers.NewCredentialsController(controllers.CredentialsControllerConfig{CredMgr: mgr})}
			headers := make(chan http.Header, 5)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				headers <- r.Header.Clone()
				w.Header().Set("Content-Type", "application/json")
				_, _ = fmt.Fprint(w, `{"data":[{"id":"proxy/alias-1","type":"model","display_name":"Alias"}],"has_more":false}`)
			}))
			defer server.Close()
			input := "n\n1\nIntegration\n" + server.URL + "/v1\n" + api + "\nrequired\n\nproxy/alias-1\n"
			err = configureProvider(b, &bytes.Buffer{}, bufio.NewReader(strings.NewReader(input)), fakePasswordReader("secret-value"), false)
			if err != nil {
				t.Fatal(err)
			}
			if len(headers) != 1 {
				t.Fatalf("expected one request, got %d", len(headers))
			}
			h := <-headers
			if api == "anthropic" {
				if h.Get("X-Api-Key") != "secret-value" || h.Get("Authorization") != "" {
					t.Fatal("wrong Anthropic headers")
				}
			} else if h.Get("Authorization") != "Bearer secret-value" {
				t.Fatal("wrong bearer header")
			}
			stored, err := providers.NewDBStore().Get(b.ctx, b.created.ID)
			if err != nil || stored.DefaultModel != "proxy/alias-1" || !stored.IsDefault {
				t.Fatalf("%v %+v", err, stored)
			}
			secret, err := mgr.GetConfigByPatternWithContext(b.ctx, stored.CredentialPattern)
			if err != nil || secret == nil {
				t.Fatalf("missing persisted credential: %v", err)
			}
			var raw database.LLMProvider
			if err := db.First(&raw, "id = ?", stored.ID).Error; err != nil {
				t.Fatal(err)
			}
			if strings.Contains(fmt.Sprintf("%+v", raw), "secret-value") {
				t.Fatal("secret leaked into provider row")
			}
		})
	}
}
