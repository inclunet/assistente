package providers

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

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
	if len(store.bumps) != 2 || !slices.Contains(store.bumps, "existing-provider") || !slices.Contains(store.bumps, "new-provider") {
		t.Fatalf("criação não invalidou os consumidores existentes e o novo provider já persistido: %v", store.bumps)
	}
	if service.registry.Get("existing-provider").CompatibilityRevision <= previousRevision {
		t.Fatal("registry manteve revisão antiga no provider que compartilha o pattern")
	}
}

func TestCreateRejectsPersistedProviderWhenRegistrySnapshotIsStale(t *testing.T) {
	store := NewMemoryStore()
	credentials := &credSpy{}
	service := NewService(ServiceConfig{
		Registry: llm.NewProviderRegistry(),
		CredMgr:  credentials,
		Store:    store,
	})
	ctx := context.Background()
	request := CreateRequest{
		ID: "existing-provider", Name: "Existing", Type: string(llm.ProviderCustom),
		APIFormat: string(llm.APIFormatOpenAI), BaseURL: "https://api.example.test/v1",
	}
	if _, err := service.Create(ctx, request); err != nil {
		t.Fatalf("criar provider inicial: %v", err)
	}
	service.registry.BeginCredentialPatternRevisionSync("api.example.test")
	request.APIKey = "must-not-be-written"
	if _, err := service.Create(ctx, request); err == nil || !strings.Contains(err.Error(), "já existe") {
		t.Fatalf("provider persistido com snapshot stale deveria ser reconhecido como duplicado: %v", err)
	}
	if len(credentials.registrados) != 0 {
		t.Fatalf("credencial foi escrita antes da checagem de duplicidade: %v", credentials.registrados)
	}
}

func TestConcurrentCreateDoesNotOverwriteProviderWithDuplicateID(t *testing.T) {
	store := NewMemoryStore()
	service := NewService(ServiceConfig{Registry: llm.NewProviderRegistry(), Store: store})
	request := CreateRequest{
		ID: "shared-id", Name: "Provider", Type: string(llm.ProviderCustom),
		APIFormat: string(llm.APIFormatOpenAI), BaseURL: "https://api.example.test/v1",
	}
	results := make(chan error, 2)
	for range 2 {
		go func() {
			_, err := service.Create(context.Background(), request)
			results <- err
		}()
	}

	var created, duplicate int
	for range 2 {
		err := <-results
		switch {
		case err == nil:
			created++
		case errors.Is(err, ErrProviderAlreadyExists):
			duplicate++
		default:
			t.Errorf("erro inesperado ao criar com ID concorrente: %v", err)
		}
	}
	count, err := store.Count(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if created != 1 || duplicate != 1 || count != 1 {
		t.Fatalf("corrida de criação: criados=%d duplicados=%d no store=%d", created, duplicate, count)
	}
}

type loadObservingProviderStore struct {
	ProviderStore
	loadCalls chan struct{}
}

func (s *loadObservingProviderStore) Load(ctx context.Context) ([]*llm.ProviderConfig, error) {
	s.loadCalls <- struct{}{}
	return s.ProviderStore.Load(ctx)
}

type blockingCreateCredentialManager struct {
	credSpy
	started chan struct{}
	release chan struct{}
}

func (m *blockingCreateCredentialManager) RegisterPatternWithContext(context.Context, string, *credentials.AuthConfig) error {
	m.started <- struct{}{}
	<-m.release
	return errors.New("credential write failed")
}

func TestCreateKeepsProvisionalProviderHiddenFromConcurrentLoadAndRecoversDefault(t *testing.T) {
	ctx := context.Background()
	store := &loadObservingProviderStore{ProviderStore: NewMemoryStore(), loadCalls: make(chan struct{}, 2)}
	credentials := &blockingCreateCredentialManager{started: make(chan struct{}, 1), release: make(chan struct{})}
	defer func() {
		select {
		case <-credentials.release:
		default:
			close(credentials.release)
		}
	}()
	service := NewService(ServiceConfig{Registry: llm.NewProviderRegistry(), CredMgr: credentials, Store: store})
	createResult := make(chan error, 1)
	go func() {
		_, err := service.Create(ctx, CreateRequest{
			ID: "provisional", Name: "Provisional", Type: string(llm.ProviderCustom),
			APIFormat: string(llm.APIFormatOpenAI), BaseURL: "https://api.example.test/v1", APIKey: "bad-key",
		})
		createResult <- err
	}()
	<-credentials.started

	if count, err := store.Count(ctx); err != nil || count != 1 {
		t.Fatalf("provider deveria estar persistido provisoriamente antes da credencial: count=%d err=%v", count, err)
	}
	if service.registry.Get("provisional") != nil {
		t.Fatal("provider provisório foi publicado antes da credencial")
	}
	loadResult := make(chan error, 1)
	loadStarted := make(chan struct{})
	go func() {
		close(loadStarted)
		loadResult <- service.Load(ctx)
	}()
	<-loadStarted
	select {
	case <-store.loadCalls:
		t.Fatal("Load observou provider provisório antes do resultado da credencial")
	case <-time.After(100 * time.Millisecond):
	}

	close(credentials.release)
	if err := <-createResult; err == nil {
		t.Fatal("criação deveria falhar quando a gravação da credencial falha")
	}
	if err := <-loadResult; err == nil || !strings.Contains(err.Error(), "nenhum provedor encontrado") {
		t.Fatalf("Load deveria prosseguir após o rollback e encontrar o store vazio: %v", err)
	}
	if service.registry.Get("provisional") != nil {
		t.Fatal("rollback deixou snapshot provisório publicado no registry")
	}
	if count, err := store.Count(ctx); err != nil || count != 0 {
		t.Fatalf("rollback deveria remover o provider provisório: count=%d err=%v", count, err)
	}

	result, err := service.Create(ctx, CreateRequest{
		ID: "successful", Name: "Successful", Type: string(llm.ProviderCustom),
		APIFormat: string(llm.APIFormatOpenAI), BaseURL: "https://other.example.test/v1",
	})
	if err != nil {
		t.Fatalf("criação após rollback: %v", err)
	}
	if result.Provider == nil || !result.Provider.IsDefault {
		t.Fatalf("primeira criação bem-sucedida não se tornou default após rollback: %+v", result.Provider)
	}
}

type failingCredentialRevisionReaderStore struct {
	ProviderStore
}

func (s *failingCredentialRevisionReaderStore) GetCompatibilityRevisionsForCredentialPattern(context.Context, string) (map[string]int, error) {
	return nil, errors.New("revision refresh unavailable")
}

func TestCreateRollsBackProviderWhenAuthoritativeRegistrationFails(t *testing.T) {
	ctx := context.Background()
	store := &failingCredentialRevisionReaderStore{ProviderStore: NewMemoryStore()}
	registry := llm.NewProviderRegistry()
	registry.BeginCredentialPatternRevisionSync("api.example.test")
	service := NewService(ServiceConfig{Registry: registry, CredMgr: &credSpy{}, Store: store})
	_, err := service.Create(ctx, CreateRequest{
		ID: "registration-fails", Name: "Registration fails", Type: string(llm.ProviderCustom),
		APIFormat: string(llm.APIFormatOpenAI), BaseURL: "https://api.example.test/v1",
	})
	if err == nil || !strings.Contains(err.Error(), "revision refresh unavailable") {
		t.Fatalf("criação deveria expor falha do refresh autoritativo: %v", err)
	}
	if count, err := store.Count(ctx); err != nil || count != 0 {
		t.Fatalf("falha de registro deveria remover a linha criada: count=%d err=%v", count, err)
	}
	if exists, err := store.Exists(ctx, "registration-fails"); err != nil || exists {
		t.Fatalf("retry não deveria encontrar provider órfão: exists=%v err=%v", exists, err)
	}
	if registry.RemoveIfPresent("registration-fails") {
		t.Fatal("falha de registro deixou snapshot stale no registry")
	}
}

func TestDeleteLoadsPersistedProviderWhenRegistrySnapshotIsStale(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()
	provider := &llm.ProviderConfig{
		ID: "stale-provider", Name: "Stale provider", Type: llm.ProviderCustom,
		APIFormat: llm.APIFormatOpenAI, BaseURL: "https://api.example.test/v1", CredentialPattern: "api.example.test",
	}
	if err := store.Save(ctx, []*llm.ProviderConfig{provider}); err != nil {
		t.Fatal(err)
	}
	registry := llm.NewProviderRegistry()
	if err := registry.Register(provider); err != nil {
		t.Fatal(err)
	}
	registry.BeginCredentialPatternRevisionSync(provider.CredentialPattern)
	if registry.Get(provider.ID) != nil {
		t.Fatal("provider deveria estar oculto enquanto o pattern está stale")
	}
	service := NewService(ServiceConfig{Registry: registry, Store: store})
	if err := service.Delete(ctx, provider.ID); err != nil {
		t.Fatalf("excluir provider pelo snapshot persistido: %v", err)
	}
	if count, err := store.Count(ctx); err != nil || count != 0 {
		t.Fatalf("exclusão não removeu o provider persistido: count=%d err=%v", count, err)
	}
	if registry.RemoveIfPresent(provider.ID) {
		t.Fatal("exclusão deixou estado do provider no registry")
	}
}

func TestDeleteClearsWatermarkOnlyStateBeforeProviderIDReuse(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()
	provider := &llm.ProviderConfig{
		ID: "watermark-only", Name: "Persisted provider", Type: llm.ProviderCustom,
		APIFormat: llm.APIFormatOpenAI, BaseURL: "https://api.example.test/v1",
		CredentialPattern: "api.example.test", CompatibilityRevision: 1,
	}
	if err := store.Save(ctx, []*llm.ProviderConfig{provider}); err != nil {
		t.Fatal(err)
	}
	registry := llm.NewProviderRegistry()
	firstGeneration := registry.BeginCredentialPatternRevisionSync(provider.CredentialPattern)
	if err := registry.PublishCredentialPatternRevisions(provider.CredentialPattern, firstGeneration, map[string]int{provider.ID: 5}); err != nil {
		t.Fatal(err)
	}
	if registry.Get(provider.ID) != nil {
		t.Fatal("registry não deveria conter snapshot do provider")
	}
	service := NewService(ServiceConfig{Registry: registry, Store: store})
	if err := service.Delete(ctx, provider.ID); err != nil {
		t.Fatalf("excluir provider persistido sem snapshot: %v", err)
	}
	if _, err := service.Create(ctx, CreateRequest{
		ID: provider.ID, Name: "Recreated provider", Type: string(llm.ProviderCustom),
		APIFormat: string(llm.APIFormatOpenAI), BaseURL: provider.BaseURL,
	}); err != nil {
		t.Fatalf("reutilizar ID depois da exclusão: %v", err)
	}
}

func TestDeleteRefreshesOtherPatternsThatPublishedWatermarkOnlyState(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()
	removed := &llm.ProviderConfig{
		ID: "old-provider", Name: "Removed", Type: llm.ProviderCustom,
		APIFormat: llm.APIFormatOpenAI, BaseURL: "https://new.example.test/v1",
		CredentialPattern: "new.example.test", CompatibilityRevision: 1,
	}
	sibling := &llm.ProviderConfig{
		ID: "sibling-provider", Name: "Sibling", Type: llm.ProviderCustom,
		APIFormat: llm.APIFormatOpenAI, BaseURL: "https://old.example.test/v1",
		CredentialPattern: "old.example.test", CompatibilityRevision: 1,
	}
	if err := store.Save(ctx, []*llm.ProviderConfig{removed, sibling}); err != nil {
		t.Fatal(err)
	}
	registry := llm.NewProviderRegistry()
	if err := registry.Register(sibling); err != nil {
		t.Fatal(err)
	}
	oldGeneration := registry.BeginCredentialPatternRevisionSync(sibling.CredentialPattern)
	if registry.Get(sibling.ID) != nil {
		t.Fatal("sync pendente deveria manter sibling stale até novo refresh")
	}
	service := NewService(ServiceConfig{Registry: registry, Store: store})
	if err := service.Delete(ctx, removed.ID); err != nil {
		t.Fatalf("excluir provider e atualizar o pattern do sibling: %v", err)
	}
	if err := registry.PublishCredentialPatternRevisions(sibling.CredentialPattern, oldGeneration, map[string]int{removed.ID: 5}); err != nil {
		t.Fatal(err)
	}
	if siblingAfterDelete := registry.Get(sibling.ID); siblingAfterDelete == nil || siblingAfterDelete.CompatibilityRevision != 1 {
		t.Fatalf("sibling do pattern antigo não recuperou visibilidade: %+v", siblingAfterDelete)
	}
	if _, err := service.Create(ctx, CreateRequest{
		ID: removed.ID, Name: "Recreated", Type: string(llm.ProviderCustom),
		APIFormat: string(llm.APIFormatOpenAI), BaseURL: removed.BaseURL,
	}); err != nil {
		t.Fatalf("reutilizar ID após publicação atrasada do pattern antigo: %v", err)
	}
}

type updateDeleteLifecycleStore struct {
	ProviderStore
	saveEntered  chan struct{}
	deleteCalled chan struct{}
	releaseSave  chan struct{}
}

func (s *updateDeleteLifecycleStore) Save(ctx context.Context, providers []*llm.ProviderConfig) error {
	s.saveEntered <- struct{}{}
	<-s.releaseSave
	return s.ProviderStore.Save(ctx, providers)
}

func (s *updateDeleteLifecycleStore) Delete(ctx context.Context, id string) error {
	s.deleteCalled <- struct{}{}
	return s.ProviderStore.Delete(ctx, id)
}

func TestUpdateAndDeleteShareProviderLifecycleLock(t *testing.T) {
	ctx := context.Background()
	store := &updateDeleteLifecycleStore{
		ProviderStore: NewMemoryStore(), saveEntered: make(chan struct{}, 1),
		deleteCalled: make(chan struct{}, 1), releaseSave: make(chan struct{}),
	}
	service := NewService(ServiceConfig{Registry: llm.NewProviderRegistry(), Store: store})
	if _, err := service.Create(ctx, CreateRequest{
		ID: "update-delete", Name: "Before", Type: string(llm.ProviderCustom),
		APIFormat: string(llm.APIFormatOpenAI), BaseURL: "https://api.example.test/v1",
	}); err != nil {
		t.Fatalf("criar provider de teste: %v", err)
	}
	updateResult := make(chan error, 1)
	go func() {
		_, err := service.Update(ctx, "update-delete", UpdateRequest{Name: "After"})
		updateResult <- err
	}()
	<-store.saveEntered
	deleteResult := make(chan error, 1)
	go func() { deleteResult <- service.Delete(ctx, "update-delete") }()
	select {
	case <-store.deleteCalled:
		t.Fatal("Delete executou durante a persistência de Update")
	case <-time.After(100 * time.Millisecond):
	}

	close(store.releaseSave)
	if err := <-updateResult; err != nil {
		t.Fatalf("Update serializado: %v", err)
	}
	if err := <-deleteResult; err != nil {
		t.Fatalf("Delete após Update: %v", err)
	}
	if exists, err := store.Exists(ctx, "update-delete"); err != nil || exists {
		t.Fatalf("Update ressuscitou provider após Delete: exists=%v err=%v", exists, err)
	}
	if service.registry.Get("update-delete") != nil {
		t.Fatal("Delete deixou provider no registry após Update")
	}
}

func TestEnsureDefaultPersistsAndPublishesClonedProviderConfiguration(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()
	provider := &llm.ProviderConfig{
		ID: "provider", Name: "Provider", Type: llm.ProviderCustom,
		APIFormat: llm.APIFormatOpenAI, BaseURL: "https://api.example.test/v1", Model: "model-a",
	}
	if err := store.Save(ctx, []*llm.ProviderConfig{provider}); err != nil {
		t.Fatal(err)
	}
	service := NewService(ServiceConfig{Registry: llm.NewProviderRegistry(), Store: store})
	if err := service.Load(ctx); err != nil {
		t.Fatalf("carregar e definir provider default: %v", err)
	}

	persisted, err := store.Get(ctx, provider.ID)
	if err != nil {
		t.Fatal(err)
	}
	registered := service.registry.Get(provider.ID)
	if !persisted.IsDefault || persisted.DefaultModel != provider.Model {
		t.Fatalf("configuração default não foi persistida: %+v", persisted)
	}
	if registered == nil || !registered.IsDefault || registered.DefaultModel != provider.Model ||
		registered.ConfigRevision != persisted.ConfigRevision {
		t.Fatalf("registry não publicou o snapshot default persistido: registered=%+v persisted=%+v", registered, persisted)
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
