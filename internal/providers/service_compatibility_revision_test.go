package providers

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"testing"

	"assistente/internal/credentials"
	"assistente/internal/database"
	"assistente/internal/llm"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

type revisionAwareProviderStore struct {
	ProviderStore
	bumps []string
}

func (s *revisionAwareProviderStore) BumpCompatibilityRevisionsForCredentialPattern(ctx context.Context, pattern string) (map[string]int, error) {
	revisionStore, ok := s.ProviderStore.(CredentialPatternRevisionStore)
	if !ok {
		return nil, fmt.Errorf("store subjacente sem suporte a revisions por pattern")
	}
	revisions, err := revisionStore.BumpCompatibilityRevisionsForCredentialPattern(ctx, pattern)
	if err != nil {
		return nil, err
	}
	for id := range revisions {
		s.bumps = append(s.bumps, id)
	}
	return revisions, nil
}

func TestUpdateAPIKeyInvalidatesConnectionCompatibilityRevision(t *testing.T) {
	store := &revisionAwareProviderStore{ProviderStore: NewMemoryStore()}
	credentials := &credSpy{}
	service := NewService(ServiceConfig{
		Registry: llm.NewProviderRegistry(),
		CredMgr:  credentials,
		Store:    store,
	})
	ctx := context.Background()
	if _, err := service.Create(ctx, CreateRequest{
		ID: "custom-provider", Name: "Custom", Type: string(llm.ProviderCustom),
		APIFormat: string(llm.APIFormatOpenAI), BaseURL: "https://api.example.test/v1",
	}); err != nil {
		t.Fatalf("criar provedor: %v", err)
	}
	if _, err := service.Create(ctx, CreateRequest{
		ID: "shared-provider", Name: "Shared", Type: string(llm.ProviderCustom),
		APIFormat: string(llm.APIFormatOpenAI), BaseURL: "https://api.example.test/v1",
	}); err != nil {
		t.Fatalf("criar segundo provedor com a mesma credencial: %v", err)
	}
	if _, err := service.Create(ctx, CreateRequest{
		ID: "other-provider", Name: "Other", Type: string(llm.ProviderCustom),
		APIFormat: string(llm.APIFormatOpenAI), BaseURL: "https://other.example.test/v1",
	}); err != nil {
		t.Fatalf("criar provedor com credencial distinta: %v", err)
	}
	firstRevision := service.registry.Get("custom-provider").CompatibilityRevision
	sharedRevision := service.registry.Get("shared-provider").CompatibilityRevision
	otherRevision := service.registry.Get("other-provider").CompatibilityRevision
	if _, err := service.Update(ctx, "custom-provider", UpdateRequest{APIKey: "new-secret"}); err != nil {
		t.Fatalf("atualizar chave: %v", err)
	}
	if len(store.bumps) != 2 || store.bumps[0] == store.bumps[1] {
		t.Fatalf("troca da chave não invalidou exatamente os provedores que compartilham o pattern: %v", store.bumps)
	}
	if service.registry.Get("custom-provider").CompatibilityRevision <= firstRevision || service.registry.Get("shared-provider").CompatibilityRevision <= sharedRevision {
		t.Fatal("registry não publicou a nova revisão em todos os provedores que compartilham a credencial")
	}
	if service.registry.Get("other-provider").CompatibilityRevision != otherRevision {
		t.Fatal("a troca da credencial alterou a revisão de um provedor com pattern distinto")
	}
	if len(credentials.registrados) != 1 || credentials.registrados[0] != "api.example.test" {
		t.Fatalf("a credencial não foi registrada no pattern esperado: %v", credentials.registrados)
	}
}

func TestCreateAPIKeyInvalidatesExistingCredentialConsumers(t *testing.T) {
	store := &revisionAwareProviderStore{ProviderStore: NewMemoryStore()}
	service := NewService(ServiceConfig{
		Registry: llm.NewProviderRegistry(),
		CredMgr:  &credSpy{},
		Store:    store,
	})
	ctx := context.Background()
	if _, err := service.Create(ctx, CreateRequest{
		ID: "existing-provider", Name: "Existing", Type: string(llm.ProviderCustom),
		APIFormat: string(llm.APIFormatOpenAI), BaseURL: "https://api.example.test/v1",
	}); err != nil {
		t.Fatalf("criar consumidor existente: %v", err)
	}
	previousRevision := service.registry.Get("existing-provider").CompatibilityRevision
	if _, err := service.Create(ctx, CreateRequest{
		ID: "new-provider", Name: "New", Type: string(llm.ProviderCustom),
		APIFormat: string(llm.APIFormatOpenAI), BaseURL: "https://api.example.test/v1", APIKey: "replacement-key",
	}); err != nil {
		t.Fatalf("criar provider com credencial compartilhada: %v", err)
	}
	if len(store.bumps) != 1 || store.bumps[0] != "existing-provider" {
		t.Fatalf("criação não invalidou exatamente os consumidores existentes: %v", store.bumps)
	}
	if service.registry.Get("existing-provider").CompatibilityRevision <= previousRevision {
		t.Fatal("registry manteve revisão antiga no provider que compartilha o pattern")
	}
}

type failingTemplateCredentialManager struct{ credSpy }

func (m *failingTemplateCredentialManager) RegisterPatternWithContext(context.Context, string, *credentials.AuthConfig) error {
	return errors.New("credential write failed")
}

func TestCreateFromTemplateCredentialFailureDoesNotPublishUnpersistedProvider(t *testing.T) {
	store := NewMemoryStore()
	registry := llm.NewProviderRegistry()
	service := NewService(ServiceConfig{
		Registry: registry,
		CredMgr:  &failingTemplateCredentialManager{},
		Store:    store,
	})

	if err := service.CreateFromTemplate(context.Background(), "openai", "bad-key"); err == nil {
		t.Fatal("esperava erro ao persistir credencial")
	}
	if got := registry.Get("openai-default"); got != nil {
		t.Fatalf("template não persistido permaneceu visível no registry: %+v", got)
	}
	if _, err := store.Get(context.Background(), "openai-default"); err == nil {
		t.Fatal("template foi persistido apesar da falha ao salvar credencial")
	}
}

func TestCreateFromTemplateAPIKeyInvalidatesExistingCredentialConsumers(t *testing.T) {
	store := &revisionAwareProviderStore{ProviderStore: NewMemoryStore()}
	service := NewService(ServiceConfig{
		Registry: llm.NewProviderRegistry(),
		CredMgr:  &credSpy{},
		Store:    store,
	})
	ctx := context.Background()
	if _, err := service.Create(ctx, CreateRequest{
		ID: "existing-openai-consumer", Name: "Existing OpenAI", Type: string(llm.ProviderCustom),
		APIFormat: string(llm.APIFormatOpenAI), BaseURL: "https://api.openai.com/v1",
	}); err != nil {
		t.Fatalf("criar consumidor existente: %v", err)
	}
	previousRevision := service.registry.Get("existing-openai-consumer").CompatibilityRevision

	if err := service.CreateFromTemplate(ctx, "openai", "replacement-key"); err != nil {
		t.Fatalf("criar template com credencial compartilhada: %v", err)
	}
	templateProvider, err := store.Get(ctx, "openai-default")
	if err != nil || templateProvider == nil {
		t.Fatalf("template não foi persistido após refresh da credencial: provider=%+v err=%v", templateProvider, err)
	}
	if got := service.registry.Get("openai-default"); got == nil || got.CompatibilityRevision != templateProvider.CompatibilityRevision {
		t.Fatalf("registry não publicou o template persistido: %+v", got)
	}
	if len(store.bumps) != 1 || store.bumps[0] != "existing-openai-consumer" {
		t.Fatalf("template não invalidou exatamente os consumidores existentes: %v", store.bumps)
	}
	if service.registry.Get("existing-openai-consumer").CompatibilityRevision <= previousRevision {
		t.Fatal("registry manteve revisão antiga no provider que compartilha o pattern")
	}
}

type failingCredentialRevisionStore struct {
	*MemoryStore
	err error
}

func (s *failingCredentialRevisionStore) GetCompatibilityRevisionsForCredentialPattern(ctx context.Context, pattern string) (map[string]int, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.MemoryStore.GetCompatibilityRevisionsForCredentialPattern(ctx, pattern)
}

func (s *failingCredentialRevisionStore) BumpCompatibilityRevisionsForCredentialPattern(ctx context.Context, pattern string) (map[string]int, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.MemoryStore.BumpCompatibilityRevisionsForCredentialPattern(ctx, pattern)
}

func TestCredentialRevisionSyncFailureHidesProviderUntilRecovery(t *testing.T) {
	store := &failingCredentialRevisionStore{MemoryStore: NewMemoryStore()}
	registry := llm.NewProviderRegistry()
	provider := &llm.ProviderConfig{
		ID: "provider", Name: "Provider", Type: llm.ProviderCustom,
		APIFormat: llm.APIFormatOpenAI, BaseURL: "https://api.example.test/v1",
		CredentialPattern: "api.example.test", CompatibilityRevision: 1,
	}
	if err := store.Save(context.Background(), []*llm.ProviderConfig{provider}); err != nil {
		t.Fatal(err)
	}
	if err := registry.Register(provider); err != nil {
		t.Fatal(err)
	}
	service := NewService(ServiceConfig{Registry: registry, CredMgr: &credSpy{}, Store: store})
	store.err = errors.New("leitura indisponível")
	if err := service.RefreshCredentialPatternRevisions(context.Background(), provider.CredentialPattern); err == nil {
		t.Fatal("esperava erro de sincronização")
	}
	if registry.Get(provider.ID) != nil || len(registry.List()) != 0 {
		t.Fatal("snapshot de credencial potencialmente obsoleto continuou disponível")
	}

	store.err = nil
	if err := service.RefreshCredentialPatternRevisions(context.Background(), provider.CredentialPattern); err != nil {
		t.Fatalf("recuperar sincronização: %v", err)
	}
	if got := registry.Get(provider.ID); got == nil || got.CompatibilityRevision != 1 {
		t.Fatalf("provedor não voltou após sincronização bem-sucedida: %+v", got)
	}

	store.err = errors.New("gravação indisponível")
	if err := service.AdvanceCredentialPatternRevisions(context.Background(), provider.CredentialPattern); err == nil {
		t.Fatal("esperava erro ao avançar a revisão")
	}
	if registry.Get(provider.ID) != nil {
		t.Fatal("falha ao avançar a revisão não ocultou o provider")
	}
	store.err = nil
	if err := service.AdvanceCredentialPatternRevisions(context.Background(), provider.CredentialPattern); err != nil {
		t.Fatalf("avançar revisão após recuperação: %v", err)
	}
	if got := registry.Get(provider.ID); got == nil || got.CompatibilityRevision != 2 {
		t.Fatalf("provider não foi restaurado na revisão atualizada: %+v", got)
	}
}

func TestEphemeralCredentialMutationAdvancesProviderIdentity(t *testing.T) {
	store := NewMemoryStore()
	manager := credentials.NewManagerWithStore(bytes.Repeat([]byte{3}, 32), nil, false)
	registry := llm.NewProviderRegistry()
	service := NewService(ServiceConfig{Registry: registry, CredMgr: manager, Store: store})
	ctx := context.Background()
	created, err := service.Create(ctx, CreateRequest{
		ID: "ephemeral-provider", Name: "Ephemeral", Type: string(llm.ProviderCustom),
		APIFormat: string(llm.APIFormatOpenAI), BaseURL: "https://ephemeral.example.test/v1",
	})
	if err != nil {
		t.Fatalf("criar provedor: %v", err)
	}
	before := registry.Get(created.Provider.ID).CompatibilityRevision
	if err := manager.RegisterPatternWithContext(ctx, created.CredentialPattern, &credentials.AuthConfig{Source: "static", Type: "bearer", Token: "secret"}); err != nil {
		t.Fatalf("registrar credencial em memória: %v", err)
	}
	if got := registry.Get(created.Provider.ID); got == nil || got.CompatibilityRevision <= before {
		t.Fatalf("mutação em memória não avançou a revisão do provider: antes=%d atual=%+v", before, got)
	}
}

func TestRegisterAuthoritativeProviderReloadsStoredSnapshot(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()
	stale := &llm.ProviderConfig{
		ID: "provider", Name: "Old", Type: llm.ProviderCustom,
		APIFormat: llm.APIFormatOpenAI, BaseURL: "https://old.example.test/v1",
		CredentialPattern: "old.example.test",
	}
	if err := store.Save(ctx, []*llm.ProviderConfig{stale}); err != nil {
		t.Fatal(err)
	}
	authoritative := *stale
	authoritative.Name = "Current"
	authoritative.BaseURL = "https://new.example.test/v1"
	authoritative.CredentialPattern = "new.example.test"
	if err := store.Save(ctx, []*llm.ProviderConfig{&authoritative}); err != nil {
		t.Fatal(err)
	}
	registry := llm.NewProviderRegistry()
	if err := registry.UpdateCompatibilityRevisions(map[string]int{stale.ID: authoritative.CompatibilityRevision}); err != nil {
		t.Fatal(err)
	}
	service := NewService(ServiceConfig{Registry: registry, CredMgr: &credSpy{}, Store: store})

	registered, err := service.registerAuthoritativeProvider(ctx, stale)
	if err != nil {
		t.Fatalf("registrar snapshot persistido: %v", err)
	}
	if registered.Name != authoritative.Name || registered.BaseURL != authoritative.BaseURL ||
		registered.CompatibilityRevision != authoritative.CompatibilityRevision {
		t.Fatalf("snapshot recarregado não corresponde ao store: %+v", registered)
	}
	if got := registry.Get(stale.ID); got == nil || got.Name != authoritative.Name || got.BaseURL != authoritative.BaseURL {
		t.Fatalf("registry publicou campos de snapshot atrasado: %+v", got)
	}
}

func TestRegisterAuthoritativeProviderRejectsOutOfOrderModelSnapshot(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()
	registry := llm.NewProviderRegistry()
	service := NewService(ServiceConfig{Registry: registry, CredMgr: &credSpy{}, Store: store})
	created, err := service.Create(ctx, CreateRequest{
		ID: "provider", Name: "Provider", Type: string(llm.ProviderCustom),
		APIFormat: string(llm.APIFormatOpenAI), BaseURL: "https://api.example.test/v1",
	})
	if err != nil {
		t.Fatal(err)
	}

	older := *created.Provider
	older.Model = "model-a"
	if err := store.Save(ctx, []*llm.ProviderConfig{&older}); err != nil {
		t.Fatal(err)
	}
	newer := older
	newer.Model = "model-b"
	if err := store.Save(ctx, []*llm.ProviderConfig{&newer}); err != nil {
		t.Fatal(err)
	}
	if newer.ConfigRevision <= older.ConfigRevision || newer.CompatibilityRevision != older.CompatibilityRevision {
		t.Fatalf("troca de modelo não gerou revisão independente: older=%+v newer=%+v", older, newer)
	}
	if _, err := service.registerAuthoritativeProvider(ctx, &newer); err != nil {
		t.Fatalf("publicar snapshot novo: %v", err)
	}

	registered, err := service.registerAuthoritativeProvider(ctx, &older)
	if err != nil {
		t.Fatalf("recarregar snapshot atrasado: %v", err)
	}
	if registered.Model != "model-b" || registered.ConfigRevision != newer.ConfigRevision {
		t.Fatalf("snapshot atrasado venceu a configuração persistida: %+v", registered)
	}
	if got := registry.Get(created.Provider.ID); got == nil || got.Model != "model-b" || got.ConfigRevision != newer.ConfigRevision {
		t.Fatalf("registry voltou ao modelo antigo: %+v", got)
	}
}

type supersedingPatternRevisionStore struct {
	*MemoryStore
	registry *llm.ProviderRegistry
}

func (s *supersedingPatternRevisionStore) GetCompatibilityRevisionsForCredentialPattern(ctx context.Context, pattern string) (map[string]int, error) {
	s.registry.BeginCredentialPatternRevisionSync(pattern)
	return s.MemoryStore.GetCompatibilityRevisionsForCredentialPattern(ctx, pattern)
}

func TestSaveAndRegisterDoesNotReportSupersededStaleSnapshotAsSuccess(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()
	provider := &llm.ProviderConfig{
		ID: "provider", Name: "Provider", Type: llm.ProviderCustom,
		APIFormat: llm.APIFormatOpenAI, BaseURL: "https://api.example.test/v1",
		CredentialPattern: "api.example.test",
	}
	registry := llm.NewProviderRegistry()
	storeWithSupersededReads := &supersedingPatternRevisionStore{MemoryStore: store, registry: registry}
	service := NewService(ServiceConfig{Registry: registry, CredMgr: &credSpy{}, Store: storeWithSupersededReads})
	registry.BeginCredentialPatternRevisionSync(provider.CredentialPattern)

	if _, err := service.SaveAndRegister(ctx, provider); err == nil {
		t.Fatal("persistência com registro stale supersedido foi reportada como sucesso")
	}
	if registry.Get(provider.ID) != nil {
		t.Fatal("provider permaneceu visível apesar de refresh supersedido")
	}
	if stored, err := store.Get(ctx, provider.ID); err != nil || stored == nil {
		t.Fatalf("provider não foi persistido antes da falha de publicação: provider=%+v err=%v", stored, err)
	}
}

func TestUpdateProviderRefreshesCredentialPatternsAfterMove(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()
	registry := llm.NewProviderRegistry()
	service := NewService(ServiceConfig{Registry: registry, CredMgr: &credSpy{}, Store: store})
	for _, request := range []CreateRequest{
		{ID: "moving", Name: "Moving", Type: string(llm.ProviderCustom), APIFormat: string(llm.APIFormatOpenAI), BaseURL: "https://shared.example.test/v1"},
		{ID: "remaining", Name: "Remaining", Type: string(llm.ProviderCustom), APIFormat: string(llm.APIFormatOpenAI), BaseURL: "https://shared.example.test/v1"},
	} {
		if _, err := service.Create(ctx, request); err != nil {
			t.Fatalf("criar %q: %v", request.ID, err)
		}
	}

	updated, err := service.Update(ctx, "moving", UpdateRequest{BaseURL: "https://new.example.test/v1"})
	if err != nil {
		t.Fatalf("migrar provider para novo pattern: %v", err)
	}
	if updated.Provider.CredentialPattern != "new.example.test" {
		t.Fatalf("pattern atualizado=%q", updated.Provider.CredentialPattern)
	}
	if got := registry.Get("moving"); got == nil || got.CredentialPattern != "new.example.test" {
		t.Fatalf("provider migrado continuou indisponível: %+v", got)
	}
	if got := registry.Get("remaining"); got == nil || got.CredentialPattern != "shared.example.test" {
		t.Fatalf("provider restante foi ocultado: %+v", got)
	}

	created, err := service.Create(ctx, CreateRequest{
		ID: "future-shared", Name: "Future shared", Type: string(llm.ProviderCustom),
		APIFormat: string(llm.APIFormatOpenAI), BaseURL: "https://shared.example.test/v1",
	})
	if err != nil {
		t.Fatalf("criar provider depois da migração: %v", err)
	}
	if got := registry.Get(created.Provider.ID); got == nil {
		t.Fatal("pattern antigo manteve marca stale órfã após a migração")
	}
}

func TestClearLegacyMCPAuthorizationRefreshesSharedProviderRevision(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	restoreDB := database.SetDB(db)
	t.Cleanup(restoreDB)
	if err := db.AutoMigrate(&database.LLMProvider{}, &database.CredentialEntry{}, &database.MCPServer{}); err != nil {
		t.Fatal(err)
	}
	if err := database.MigrateLLMModelCapabilities(db); err != nil {
		t.Fatal(err)
	}

	ctx := database.WithUserID(context.Background(), "owner")
	registry := llm.NewProviderRegistry()
	service := NewService(ServiceConfig{
		Registry: registry,
		CredMgr:  credentials.NewManagerWithStore(bytes.Repeat([]byte{7}, 32), credentials.NewDBStore(), true),
		Store:    NewDBStore(),
	})
	provider, err := service.Create(ctx, CreateRequest{
		ID: "shared-provider", Name: "Shared", Type: string(llm.ProviderCustom),
		APIFormat: string(llm.APIFormatOpenAI), BaseURL: "https://shared.example.test/v1",
	})
	if err != nil {
		t.Fatalf("criar provedor: %v", err)
	}
	consumer := database.MCPServer{
		UserID: "owner", Slug: "legacy", Name: "Legacy", Transport: "streamable",
		AuthType: "oauth2_pkce", Enabled: true,
	}
	if err := db.Create(&consumer).Error; err != nil {
		t.Fatal(err)
	}
	manager := service.credMgr.(*credentials.Manager)
	if err := manager.RegisterPatternWithContext(ctx, "mcp-tokens:legacy", &credentials.AuthConfig{Source: "static", Type: "oauth2", Token: "token"}); err != nil {
		t.Fatal(err)
	}
	if err := manager.RegisterPatternWithContext(ctx, "shared.example.test", &credentials.AuthConfig{Source: "static", Type: "bearer", Token: "shared-token"}); err != nil {
		t.Fatal(err)
	}
	before := registry.Get(provider.Provider.ID).CompatibilityRevision

	if err := manager.ClearLegacyOAuthWithConsumer(ctx, "legacy", consumer.ID, "shared.example.test", nil, nil); err != nil {
		t.Fatalf("limpar autorização MCP: %v", err)
	}
	var persisted database.LLMProvider
	if err := db.Where("id = ?", provider.Provider.ID).First(&persisted).Error; err != nil {
		t.Fatal(err)
	}
	got := registry.Get(provider.Provider.ID)
	if persisted.CompatibilityRevision <= before || got == nil || got.CompatibilityRevision != persisted.CompatibilityRevision {
		t.Fatalf("registry não refletiu remoção da credencial compartilhada: antes=%d banco=%d registry=%+v", before, persisted.CompatibilityRevision, got)
	}
	var remaining int64
	if err := db.Model(&database.CredentialEntry{}).Where("user_id = ? AND pattern IN ?", "owner", []string{"mcp-tokens:legacy", "shared.example.test"}).Count(&remaining).Error; err != nil {
		t.Fatal(err)
	}
	if remaining != 0 {
		t.Fatalf("Clear deixou credenciais MCP/hostname no banco: %d", remaining)
	}
}
