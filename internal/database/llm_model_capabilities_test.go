package database

import (
	"context"
	"errors"
	"testing"
	"time"

	"assistente/internal/llmcapabilities"
	"gorm.io/gorm"
)

func TestMigrateLLMModelCapabilitiesCreatesStrictCatalogIdempotently(t *testing.T) {
	db := newMigratorTestDB(t)
	if err := db.AutoMigrate(&LLMProvider{}); err != nil {
		t.Fatal(err)
	}
	if err := MigrateLLMModelCapabilities(db); err != nil {
		t.Fatalf("migração: %v", err)
	}
	if err := MigrateLLMModelCapabilities(db); err != nil {
		t.Fatalf("migração idempotente: %v", err)
	}
	for _, table := range []string{
		"llm_models", "llm_capabilities", "llm_capability_fields", "llm_model_catalog_bindings",
		"llm_model_capabilities", "llm_model_capability_fields", "llm_model_capability_field_options",
	} {
		if !db.Migrator().HasTable(table) {
			t.Errorf("tabela %s ausente", table)
		}
	}
	var capabilityCount, fieldCount int64
	if err := db.Table("llm_capabilities").Count(&capabilityCount).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Table("llm_capability_fields").Count(&fieldCount).Error; err != nil {
		t.Fatal(err)
	}
	if int(capabilityCount) != len(llmcapabilities.Capabilities()) || int(fieldCount) != len(llmcapabilities.Fields()) {
		t.Fatalf("catálogo semeado: capabilities=%d campos=%d", capabilityCount, fieldCount)
	}
	for _, forbidden := range []string{"user_id", "api_format"} {
		if db.Migrator().HasColumn("llm_models", forbidden) {
			t.Errorf("llm_models não pode conter %s", forbidden)
		}
	}
	if !db.Migrator().HasColumn(&LLMProvider{}, "CompatibilityRevision") {
		t.Fatal("revisão de compatibilidade não foi adicionada ao provider")
	}
	if err := db.Exec(`INSERT INTO llm_capabilities(key) VALUES ('arbitrary_capability')`).Error; err == nil {
		t.Fatal("SQLite aceitou capability fora do vocabulário controlado")
	}
}

func TestLLMModelCapabilitiesRepositoryScopesModelsAndResolvesLocalFacts(t *testing.T) {
	db := llmModelCapabilitiesTestDB(t)
	ctx := WithUserID(context.Background(), "owner-a")
	if err := db.Create(&LLMProvider{ID: "provider-a", UserID: "owner-a", Name: "A", Type: "custom", APIFormat: "openai", BaseURL: "https://one.example/v1"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&LLMProvider{ID: "provider-b", UserID: "owner-a", Name: "B", Type: "custom", APIFormat: "openai", BaseURL: "https://two.example/v1"}).Error; err != nil {
		t.Fatal(err)
	}
	repository := NewLLMModelCapabilitiesRepository(db)
	first, err := repository.SaveModel(ctx, "provider-a", "gpt-shared", "GPT shared")
	if err != nil {
		t.Fatalf("salvar primeiro modelo: %v", err)
	}
	second, err := repository.SaveModel(ctx, "provider-b", "gpt-shared", "Gateway B model")
	if err != nil {
		t.Fatalf("salvar modelo no segundo endpoint: %v", err)
	}
	if first.ID == second.ID {
		t.Fatal("mesmo ID remoto em provedores distintos compartilhou registro")
	}
	updated, err := repository.SaveModel(ctx, "provider-a", "gpt-shared", "Nome atualizado")
	if err != nil || updated.ID != first.ID || updated.DisplayName != "Nome atualizado" {
		t.Fatalf("upsert de modelo: row=%+v err=%v", updated, err)
	}
	models, err := repository.ListModels(ctx, "provider-a")
	if err != nil || len(models) != 1 {
		t.Fatalf("modelos do provider: count=%d err=%v", len(models), err)
	}
	if _, err := repository.ListModels(WithUserID(context.Background(), "owner-b"), "provider-a"); err == nil {
		t.Fatal("outro usuário conseguiu listar modelos de um provider privado")
	}

	now := time.Now().UTC().Truncate(time.Microsecond)
	voice := &LLMModelCapabilityField{ModelID: first.ID, CapabilityKey: string(llmcapabilities.CapabilityTTS), FieldKey: string(llmcapabilities.FieldVoice), SupportState: string(llmcapabilities.Supported), Source: string(llmcapabilities.SourceEndpointDiscovery), Scope: string(llmcapabilities.ScopeConnection), ProviderCompatibilityRevision: 1, ObservedAt: now}
	options := []LLMModelCapabilityFieldOption{
		{Value: "alloy", Label: "Alloy", SupportState: string(llmcapabilities.Supported)},
		{Value: "nova", Label: "Nova", SupportState: string(llmcapabilities.Supported)},
	}
	if err := repository.RecordField(ctx, voice, options); err != nil {
		t.Fatalf("registrar vozes: %v", err)
	}
	if err := repository.RecordCapability(ctx, &LLMModelCapability{ModelID: first.ID, CapabilityKey: string(llmcapabilities.CapabilityTTS), SupportState: string(llmcapabilities.Supported), Source: string(llmcapabilities.SourceEndpointDiscovery), Scope: string(llmcapabilities.ScopeConnection), ProviderCompatibilityRevision: 1, ObservedAt: now}); err != nil {
		t.Fatalf("registrar capability TTS: %v", err)
	}
	resolved, err := repository.Resolve(ctx, first.ID, now.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if got := resolved.Capabilities[llmcapabilities.CapabilityTTS]; got.State != llmcapabilities.Supported {
		t.Fatalf("TTS não resolvido localmente: %+v", got)
	}
	voices := resolved.Fields[llmcapabilities.CapabilityTTS][llmcapabilities.FieldVoice]
	if voices.State != llmcapabilities.Supported || len(voices.Options) != 2 || voices.Options[0].Value != "alloy" || voices.Options[1].Value != "nova" {
		t.Fatalf("opções de voz incorretas: %+v", voices)
	}
}

func TestLLMModelCapabilitiesSystemProviderWritesRequireBootstrap(t *testing.T) {
	db := llmModelCapabilitiesTestDB(t)
	if err := db.Create(&LLMProvider{ID: "system-provider", Name: "System", Type: "system", APIFormat: "openai", BaseURL: "https://system.example/v1"}).Error; err != nil {
		t.Fatal(err)
	}
	repository := NewLLMModelCapabilitiesRepository(db)
	userCtx := WithUserID(context.Background(), "owner-a")
	bootstrapCtx := WithBootstrap(context.Background())
	model, err := repository.SaveModel(bootstrapCtx, "system-provider", "shared-model", "Shared")
	if err != nil {
		t.Fatalf("bootstrap deveria criar o modelo compartilhado: %v", err)
	}

	if _, err := repository.SaveModel(userCtx, "system-provider", "another-model", "Other"); !errors.Is(err, ErrSystemProviderWriteRequiresBootstrap) {
		t.Fatalf("usuário publicou modelo em provider global: %v", err)
	}
	now := time.Now().UTC().Truncate(time.Microsecond)
	capability := &LLMModelCapability{ModelID: model.ID, CapabilityKey: string(llmcapabilities.CapabilityChat), SupportState: string(llmcapabilities.Supported), Source: string(llmcapabilities.SourceEndpointDiscovery), Scope: string(llmcapabilities.ScopeConnection), ProviderCompatibilityRevision: 1, ObservedAt: now}
	if err := repository.RecordCapability(userCtx, capability); !errors.Is(err, ErrSystemProviderWriteRequiresBootstrap) {
		t.Fatalf("usuário publicou fato global: %v", err)
	}
	field := &LLMModelCapabilityField{ModelID: model.ID, CapabilityKey: string(llmcapabilities.CapabilityChat), FieldKey: string(llmcapabilities.FieldTemperature), SupportState: string(llmcapabilities.Unsupported), Source: string(llmcapabilities.SourceExecutionObservation), Scope: string(llmcapabilities.ScopeConnection), ProviderCompatibilityRevision: 1, ObservedAt: now}
	if err := repository.RecordField(userCtx, field, nil); !errors.Is(err, ErrSystemProviderWriteRequiresBootstrap) {
		t.Fatalf("usuário publicou campo global: %v", err)
	}
	binding := &LLMModelCatalogBinding{ModelID: model.ID, ProviderCompatibilityRevision: 1, Source: string(llmcapabilities.SourceAppCuration), ExternalProviderID: "vendor", ExternalModelID: "shared-model", VerifiedAt: now}
	if err := repository.BindCatalogModel(userCtx, binding); !errors.Is(err, ErrSystemProviderWriteRequiresBootstrap) {
		t.Fatalf("usuário publicou vínculo global: %v", err)
	}

	if err := repository.RecordCapability(bootstrapCtx, capability); err != nil {
		t.Fatalf("bootstrap deveria registrar fatos compartilhados: %v", err)
	}
	models, err := repository.ListModels(userCtx, "system-provider")
	if err != nil || len(models) != 1 {
		t.Fatalf("usuário deveria poder ler catálogo compartilhado: models=%d err=%v", len(models), err)
	}
	resolved, err := repository.Resolve(userCtx, model.ID, now.Add(time.Second))
	if err != nil || resolved.Capabilities[llmcapabilities.CapabilityChat].State != llmcapabilities.Supported {
		t.Fatalf("fato compartilhado não foi lido: resolution=%+v err=%v", resolved, err)
	}
}

func TestLLMModelCapabilityRepositoryRequiresCapturedRevisionAndSafeReferences(t *testing.T) {
	db := llmModelCapabilitiesTestDB(t)
	ctx := WithUserID(context.Background(), "owner-a")
	if err := db.Create(&LLMProvider{ID: "provider-a", UserID: "owner-a", Name: "A", Type: "custom", APIFormat: "openai", BaseURL: "https://one.example/v1"}).Error; err != nil {
		t.Fatal(err)
	}
	repository := NewLLMModelCapabilitiesRepository(db)
	model, err := repository.SaveModel(ctx, "provider-a", "model-x", "Model X")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Microsecond)
	base := LLMModelCapability{ModelID: model.ID, CapabilityKey: string(llmcapabilities.CapabilityChat), SupportState: string(llmcapabilities.Unsupported), Source: string(llmcapabilities.SourceExecutionObservation), Scope: string(llmcapabilities.ScopeConnection), ObservedAt: now}
	if err := repository.RecordCapability(ctx, &base); !errors.Is(err, ErrStaleCompatibility) {
		t.Fatalf("fato sem revisão capturada deveria ser recusado: %v", err)
	}
	base.ProviderCompatibilityRevision = 1
	future := base
	future.ObservedAt = now.Add(time.Hour)
	if err := repository.RecordCapability(ctx, &future); !errors.Is(err, llmcapabilities.ErrInvalidAssertion) {
		t.Fatalf("observação futura deveria ser recusada: %v", err)
	}
	for _, reference := range []string{
		"https://user:secret@example.test/catalog",
		"https://example.test/catalog?token=secret",
		"https://example.test/catalog#secret",
		"catalog/model-x",
	} {
		claim := base
		claim.SourceReference = reference
		if err := repository.RecordCapability(ctx, &claim); !errors.Is(err, llmcapabilities.ErrInvalidAssertion) {
			t.Errorf("referência insegura %q aceita: %v", reference, err)
		}
	}
	for _, reference := range []string{"https://docs.example.test/models/model-x", "HTTPS://Docs.Example.test/models/model-x"} {
		claim := base
		claim.SourceReference = reference
		if err := repository.RecordCapability(ctx, &claim); err != nil {
			t.Fatalf("URL pública sem credenciais deveria ser aceita (%s): %v", reference, err)
		}
	}
	if err := db.Exec(`INSERT INTO llm_model_capabilities (id, model_id, capability_key, support_state, source, scope, provider_compatibility_revision, observed_at, source_reference, created_at, updated_at) VALUES ('raw-reference', ?, 'chat', 'unsupported', 'execution_observation', 'connection', 1, ?, 'https://docs.example.test/models?token=secret', ?, ?)`, model.ID, now, now, now).Error; err == nil {
		t.Fatal("SQLite aceitou query string com potencial segredo em source_reference")
	}
}

func TestLLMModelCapabilityFactsBecomeIneligibleAfterProviderIdentityChanges(t *testing.T) {
	db := llmModelCapabilitiesTestDB(t)
	ctx := WithUserID(context.Background(), "owner-a")
	provider := &LLMProvider{ID: "provider-a", UserID: "owner-a", Name: "A", Type: "custom", APIFormat: "openai", BaseURL: "https://one.example/v1"}
	providerRepository := NewProviderRepository(db)
	if err := providerRepository.SaveLLMProvider(ctx, provider); err != nil {
		t.Fatal(err)
	}
	if provider.CompatibilityRevision != 1 {
		t.Fatalf("revisão inicial=%d, esperava 1", provider.CompatibilityRevision)
	}
	models := NewLLMModelCapabilitiesRepository(db)
	model, err := models.SaveModel(ctx, provider.ID, "model-x", "Model X")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Microsecond)
	claim := &LLMModelCapabilityField{ModelID: model.ID, CapabilityKey: string(llmcapabilities.CapabilityChat), FieldKey: string(llmcapabilities.FieldTemperature), SupportState: string(llmcapabilities.Unsupported), Source: string(llmcapabilities.SourceExecutionObservation), Scope: string(llmcapabilities.ScopeConnection), ProviderCompatibilityRevision: 1, ObservedAt: now}
	if err := models.RecordField(ctx, claim, nil); err != nil {
		t.Fatal(err)
	}

	provider.BaseURL = "https://changed.example/v1"
	if err := providerRepository.SaveLLMProvider(ctx, provider); err != nil {
		t.Fatal(err)
	}
	if provider.CompatibilityRevision != 2 {
		t.Fatalf("troca de endpoint deixou revisão em %d, esperava 2", provider.CompatibilityRevision)
	}
	resolved, err := models.Resolve(ctx, model.ID, now.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if got := resolved.Fields[llmcapabilities.CapabilityChat][llmcapabilities.FieldTemperature]; got.State != llmcapabilities.Unknown {
		t.Fatalf("fato da revisão anterior continuou efetivo: %+v", got)
	}
	var historical int64
	if err := db.Model(&LLMModelCapabilityField{}).Where("model_id = ? AND provider_compatibility_revision = 1", model.ID).Count(&historical).Error; err != nil || historical != 1 {
		t.Fatalf("histórico antigo foi perdido: count=%d err=%v", historical, err)
	}
	if err := models.RecordCapability(ctx, &LLMModelCapability{ModelID: model.ID, CapabilityKey: string(llmcapabilities.CapabilityChat), SupportState: string(llmcapabilities.Supported), Source: string(llmcapabilities.SourceExecutionObservation), Scope: string(llmcapabilities.ScopeConnection), ProviderCompatibilityRevision: 1, ObservedAt: now}); !errors.Is(err, ErrStaleCompatibility) {
		t.Fatalf("escrita com revisão velha deveria falhar: %v", err)
	}
	if err := providerRepository.BumpCompatibilityRevision(ctx, provider.ID); err != nil {
		t.Fatal(err)
	}
	current, err := providerRepository.GetLLMProvider(ctx, provider.ID)
	if err != nil || current.CompatibilityRevision != 3 {
		t.Fatalf("troca de identidade da credencial não avançou revisão: provider=%+v err=%v", current, err)
	}
}

func TestLLMModelCapabilityAssertionsRequireVerifiedCurrentBinding(t *testing.T) {
	db := llmModelCapabilitiesTestDB(t)
	ctx := WithUserID(context.Background(), "owner-a")
	if err := db.Create(&LLMProvider{ID: "provider-a", UserID: "owner-a", Name: "A", Type: "custom", APIFormat: "openai", BaseURL: "https://one.example/v1"}).Error; err != nil {
		t.Fatal(err)
	}
	repository := NewLLMModelCapabilitiesRepository(db)
	model, err := repository.SaveModel(ctx, "provider-a", "remote-model", "Remote")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Microsecond)
	binding := &LLMModelCatalogBinding{ModelID: model.ID, ProviderCompatibilityRevision: 1, Source: string(llmcapabilities.SourceOfficialCatalog), ExternalProviderID: "official-vendor", ExternalModelID: "remote-model", VerifiedAt: now}
	if err := repository.BindCatalogModel(ctx, binding); err != nil {
		t.Fatalf("vincular identidade externa: %v", err)
	}
	claim := &LLMModelCapability{ModelID: model.ID, CapabilityKey: string(llmcapabilities.CapabilityReasoning), SupportState: string(llmcapabilities.Supported), Source: string(llmcapabilities.SourceOfficialCatalog), Scope: string(llmcapabilities.ScopeExternalBinding), ProviderCompatibilityRevision: 1, BindingID: &binding.ID, ObservedAt: now}
	if err := repository.RecordCapability(ctx, claim); err != nil {
		t.Fatalf("registrar fato vinculado: %v", err)
	}
	resolved, err := repository.Resolve(ctx, model.ID, now.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if got := resolved.Capabilities[llmcapabilities.CapabilityReasoning]; got.State != llmcapabilities.Supported {
		t.Fatalf("fato com vínculo verificado foi ignorado: %+v", got)
	}

	provider := &LLMProvider{ID: "provider-a", UserID: "owner-a", Name: "A", Type: "custom", APIFormat: "anthropic", BaseURL: "https://one.example/v1"}
	if err := NewProviderRepository(db).SaveLLMProvider(ctx, provider); err != nil {
		t.Fatal(err)
	}
	resolved, err = repository.Resolve(ctx, model.ID, now.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if got := resolved.Capabilities[llmcapabilities.CapabilityReasoning]; got.State != llmcapabilities.Unknown {
		t.Fatalf("vínculo de revisão velha continuou efetivo: %+v", got)
	}
}

func TestLLMModelCapabilityRepositoryRejectsTypedConstraintViolations(t *testing.T) {
	db := llmModelCapabilitiesTestDB(t)
	ctx := WithUserID(context.Background(), "owner-a")
	if err := db.Create(&LLMProvider{ID: "provider-a", UserID: "owner-a", Name: "A", Type: "custom", APIFormat: "openai", BaseURL: "https://one.example/v1"}).Error; err != nil {
		t.Fatal(err)
	}
	repository := NewLLMModelCapabilitiesRepository(db)
	model, err := repository.SaveModel(ctx, "provider-a", "remote-model", "Remote")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Microsecond)
	claim := &LLMModelCapabilityField{ModelID: model.ID, CapabilityKey: string(llmcapabilities.CapabilityChat), FieldKey: string(llmcapabilities.FieldMaxOutputTokens), SupportState: string(llmcapabilities.Supported), Source: string(llmcapabilities.SourceEndpointDiscovery), Scope: string(llmcapabilities.ScopeConnection), ProviderCompatibilityRevision: 1, Minimum: floatPtr(0.5), ObservedAt: now}
	if err := repository.RecordField(ctx, claim, nil); err == nil {
		t.Fatal("campo integer aceitou limite fracionário")
	}
	claim = &LLMModelCapabilityField{ModelID: model.ID, CapabilityKey: string(llmcapabilities.CapabilityChat), FieldKey: string(llmcapabilities.FieldTemperature), SupportState: string(llmcapabilities.Unsupported), Source: string(llmcapabilities.SourceEndpointDiscovery), Scope: string(llmcapabilities.ScopeConnection), ProviderCompatibilityRevision: 1, Minimum: floatPtr(0), ObservedAt: now}
	if err := repository.RecordField(ctx, claim, nil); err == nil {
		t.Fatal("campo unsupported aceitou limite")
	}
	if err := db.Exec(`INSERT INTO llm_model_capability_fields (id, model_id, capability_key, field_key, support_state, source, scope, provider_compatibility_revision, observed_at, created_at, updated_at, minimum) VALUES ('raw', ?, 'chat', 'max_output_tokens', 'supported', 'endpoint_discovery', 'connection', 1, ?, ?, ?, 0.5)`, model.ID, now, now, now).Error; err == nil {
		t.Fatal("trigger SQLite aceitou limite fracionário em campo inteiro")
	}
}

func TestProviderCompatibilityRevisionChangesWithOnlyConnectionIdentity(t *testing.T) {
	db := newMigratorTestDB(t)
	if err := db.AutoMigrate(&LLMProvider{}); err != nil {
		t.Fatal(err)
	}
	ctx := WithUserID(context.Background(), "owner-a")
	repository := NewProviderRepository(db)
	provider := &LLMProvider{ID: "provider-a", UserID: "owner-a", Name: "A", Type: "custom", APIFormat: "openai", BaseURL: "https://one.example/v1"}
	if err := repository.SaveLLMProvider(ctx, provider); err != nil {
		t.Fatal(err)
	}
	if provider.CompatibilityRevision != 1 {
		t.Fatalf("revisão inicial=%d, esperava 1", provider.CompatibilityRevision)
	}
	steps := []struct {
		name string
		edit func(*LLMProvider)
		want int
	}{
		{"nome", func(p *LLMProvider) { p.Name = "Renomeado" }, 1},
		{"modelo padrão", func(p *LLMProvider) { p.DefaultModel = "model-a" }, 1},
		{"endpoint", func(p *LLMProvider) { p.BaseURL = "https://two.example/v1" }, 2},
		{"formato da API", func(p *LLMProvider) { p.APIFormat = "anthropic" }, 3},
		{"adaptador", func(p *LLMProvider) { p.Type = "anthropic" }, 4},
		{"referência da credencial", func(p *LLMProvider) { p.CredentialPattern = "account-b" }, 5},
		{"modo de autenticação", func(p *LLMProvider) { p.AuthMode = "none" }, 6},
		{"identidade do agente", func(p *LLMProvider) { p.ACPAgentID = "agent-b" }, 7},
	}
	for _, step := range steps {
		t.Run(step.name, func(t *testing.T) {
			step.edit(provider)
			if err := repository.SaveLLMProvider(ctx, provider); err != nil {
				t.Fatal(err)
			}
			if provider.CompatibilityRevision != step.want {
				t.Fatalf("revisão=%d, esperava %d", provider.CompatibilityRevision, step.want)
			}
		})
	}
	if err := repository.BumpCompatibilityRevision(ctx, provider.ID); err != nil {
		t.Fatal(err)
	}
	current, err := repository.GetLLMProvider(ctx, provider.ID)
	if err != nil || current.CompatibilityRevision != 8 {
		t.Fatalf("revisão após troca explícita de credencial: provider=%+v err=%v", current, err)
	}
}

func llmModelCapabilitiesTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db := newMigratorTestDB(t)
	if err := db.Exec("PRAGMA foreign_keys = ON").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&LLMProvider{}); err != nil {
		t.Fatal(err)
	}
	if err := MigrateLLMModelCapabilities(db); err != nil {
		t.Fatal(err)
	}
	return db
}

func floatPtr(value float64) *float64 { return &value }
