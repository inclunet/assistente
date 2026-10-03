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
	if err := db.AutoMigrate(&LLMProvider{}, &CredentialEntry{}); err != nil {
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
	if !db.Migrator().HasColumn(&LLMProvider{}, "ConfigRevision") {
		t.Fatal("revisão de configuração não foi adicionada ao provider")
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

func TestLLMModelCatalogBindingRenewalPreservesVerifiedHistory(t *testing.T) {
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
	base := time.Now().UTC().Truncate(time.Microsecond)
	oldVerifiedAt := base.Add(-6 * time.Hour)
	oldValidUntil := base.Add(-4 * time.Hour)
	oldBinding := &LLMModelCatalogBinding{
		ModelID: model.ID, ProviderCompatibilityRevision: 1, Source: string(llmcapabilities.SourceOfficialCatalog),
		ExternalProviderID: "official-vendor", ExternalModelID: "remote-model", VerifiedAt: oldVerifiedAt, ValidUntil: &oldValidUntil,
	}
	if err := repository.BindCatalogModel(ctx, oldBinding); err != nil {
		t.Fatalf("criar vínculo inicial: %v", err)
	}
	claim := &LLMModelCapability{
		ModelID: model.ID, CapabilityKey: string(llmcapabilities.CapabilityReasoning), SupportState: string(llmcapabilities.Supported),
		Source: string(llmcapabilities.SourceOfficialCatalog), Scope: string(llmcapabilities.ScopeExternalBinding),
		ProviderCompatibilityRevision: 1, BindingID: &oldBinding.ID, ObservedAt: oldVerifiedAt.Add(time.Hour),
	}
	if err := repository.RecordCapability(ctx, claim); err != nil {
		t.Fatalf("registrar fato observado durante validade inicial: %v", err)
	}
	fieldClaim := &LLMModelCapabilityField{
		ModelID: model.ID, CapabilityKey: string(llmcapabilities.CapabilityChat), FieldKey: string(llmcapabilities.FieldTemperature),
		SupportState: string(llmcapabilities.Supported), Source: string(llmcapabilities.SourceOfficialCatalog),
		Scope: string(llmcapabilities.ScopeExternalBinding), ProviderCompatibilityRevision: 1,
		BindingID: &oldBinding.ID, ObservedAt: oldVerifiedAt.Add(time.Hour),
	}
	if err := repository.RecordField(ctx, fieldClaim, nil); err != nil {
		t.Fatalf("registrar campo vinculado: %v", err)
	}
	if err := db.Exec("UPDATE llm_model_capabilities SET source = ? WHERE id = ?", string(llmcapabilities.SourceThirdPartyCatalog), claim.ID).Error; err == nil {
		t.Fatal("SQLite permitiu alterar a proveniência de uma afirmação de capability")
	}
	if err := db.Exec("UPDATE llm_model_capability_fields SET source = ? WHERE id = ?", string(llmcapabilities.SourceThirdPartyCatalog), fieldClaim.ID).Error; err == nil {
		t.Fatal("SQLite permitiu alterar a proveniência de uma afirmação de campo")
	}

	renewedVerifiedAt := base.Add(-2 * time.Hour)
	renewedValidUntil := base.Add(2 * time.Hour)
	renewedBinding := &LLMModelCatalogBinding{
		ModelID: model.ID, ProviderCompatibilityRevision: 1, Source: oldBinding.Source,
		ExternalProviderID: oldBinding.ExternalProviderID, ExternalModelID: oldBinding.ExternalModelID,
		VerifiedAt: renewedVerifiedAt, ValidUntil: &renewedValidUntil,
	}
	if err := repository.BindCatalogModel(ctx, renewedBinding); err != nil {
		t.Fatalf("renovar vínculo: %v", err)
	}
	if renewedBinding.ID == oldBinding.ID {
		t.Fatal("renovação reutilizou o ID da verificação anterior")
	}
	var storedOld LLMModelCatalogBinding
	if err := db.First(&storedOld, "id = ?", oldBinding.ID).Error; err != nil {
		t.Fatal(err)
	}
	if !storedOld.VerifiedAt.Equal(oldVerifiedAt) || !sameNullableTime(storedOld.ValidUntil, &oldValidUntil) {
		t.Fatalf("renovação alterou a validade histórica: %+v", storedOld)
	}
	if err := db.Exec("UPDATE llm_model_catalog_bindings SET valid_until = ? WHERE id = ?", base.Add(3*time.Hour), oldBinding.ID).Error; err == nil {
		t.Fatal("SQLite permitiu estender diretamente a validade de uma verificação histórica")
	}
	resolved, err := repository.Resolve(ctx, model.ID, base)
	if err != nil {
		t.Fatal(err)
	}
	if got := resolved.Capabilities[llmcapabilities.CapabilityReasoning]; got.State != llmcapabilities.Unknown {
		t.Fatalf("fato da verificação expirada foi revalidado retroativamente: %+v", got)
	}

	verificationWithDifferentOffsets := &LLMModelCatalogBinding{
		ModelID: model.ID, ProviderCompatibilityRevision: 1, Source: oldBinding.Source,
		ExternalProviderID: oldBinding.ExternalProviderID, ExternalModelID: oldBinding.ExternalModelID,
		VerifiedAt: renewedVerifiedAt.In(time.FixedZone("offset-plus-two", 2*60*60)),
		ValidUntil: timePointer(renewedValidUntil.In(time.FixedZone("offset-minus-five", -5*60*60))),
	}
	if err := repository.BindCatalogModel(ctx, verificationWithDifferentOffsets); err != nil {
		t.Fatalf("mesmo instante com offsets diferentes deveria ser idempotente: %v", err)
	}
	if verificationWithDifferentOffsets.ID != renewedBinding.ID {
		t.Fatal("mesmo instante com offsets diferentes criou outro vínculo")
	}

	changedExpiry := base.Add(3 * time.Hour)
	duplicateVerification := &LLMModelCatalogBinding{
		ModelID: model.ID, ProviderCompatibilityRevision: 1, Source: oldBinding.Source,
		ExternalProviderID: oldBinding.ExternalProviderID, ExternalModelID: oldBinding.ExternalModelID,
		VerifiedAt: renewedVerifiedAt, ValidUntil: &changedExpiry,
	}
	if err := repository.BindCatalogModel(ctx, duplicateVerification); !errors.Is(err, ErrInvalidCatalogBinding) {
		t.Fatalf("mesmo instante de verificação permitiu alterar validade: %v", err)
	}
}

func TestLLMModelCapabilityAssertionsNormalizeTimesToUTC(t *testing.T) {
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
	observedUTC := time.Now().UTC().Add(-3 * time.Hour).Truncate(time.Second)
	validUntilUTC := observedUTC.Add(90 * time.Minute)
	observedAt := observedUTC.In(time.FixedZone("offset-plus-two", 2*60*60))
	validUntil := validUntilUTC.In(time.FixedZone("offset-minus-five", -5*60*60))
	capability := &LLMModelCapability{
		ModelID: model.ID, CapabilityKey: string(llmcapabilities.CapabilityChat), SupportState: string(llmcapabilities.Supported),
		Source: string(llmcapabilities.SourceEndpointDiscovery), Scope: string(llmcapabilities.ScopeConnection),
		ProviderCompatibilityRevision: 1, ObservedAt: observedAt, ValidUntil: &validUntil,
	}
	if err := repository.RecordCapability(ctx, capability); err != nil {
		t.Fatalf("persistir timestamps equivalentes em offsets diferentes: %v", err)
	}
	if capability.ObservedAt.Location() != time.UTC || capability.ValidUntil.Location() != time.UTC || !capability.ObservedAt.Equal(observedUTC) || !capability.ValidUntil.Equal(validUntilUTC) {
		t.Fatalf("timestamps de capability não foram normalizados: observed=%v validUntil=%v", capability.ObservedAt, capability.ValidUntil)
	}
	field := &LLMModelCapabilityField{
		ModelID: model.ID, CapabilityKey: string(llmcapabilities.CapabilityChat), FieldKey: string(llmcapabilities.FieldTemperature),
		SupportState: string(llmcapabilities.Supported), Source: string(llmcapabilities.SourceEndpointDiscovery),
		Scope: string(llmcapabilities.ScopeConnection), ProviderCompatibilityRevision: 1,
		ObservedAt: observedAt, ValidUntil: &validUntil,
	}
	if err := repository.RecordField(ctx, field, nil); err != nil {
		t.Fatalf("persistir timestamps do campo: %v", err)
	}
	if field.ObservedAt.Location() != time.UTC || field.ValidUntil.Location() != time.UTC || !field.ObservedAt.Equal(observedUTC) || !field.ValidUntil.Equal(validUntilUTC) {
		t.Fatalf("timestamps do campo não foram normalizados: observed=%v validUntil=%v", field.ObservedAt, field.ValidUntil)
	}
}

func TestLLMModelCapabilityHistoryIsAppendOnlyAndCascadesWithProvider(t *testing.T) {
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
	now := time.Now().UTC().Truncate(time.Second)
	binding := &LLMModelCatalogBinding{
		ModelID: model.ID, ProviderCompatibilityRevision: 1, Source: string(llmcapabilities.SourceOfficialCatalog),
		ExternalProviderID: "vendor", ExternalModelID: "remote-model", VerifiedAt: now.Add(-time.Minute),
	}
	if err := repository.BindCatalogModel(ctx, binding); err != nil {
		t.Fatalf("registrar vínculo: %v", err)
	}
	capability := &LLMModelCapability{
		ModelID: model.ID, CapabilityKey: string(llmcapabilities.CapabilityChat), SupportState: string(llmcapabilities.Supported),
		Source: binding.Source, Scope: string(llmcapabilities.ScopeExternalBinding), ProviderCompatibilityRevision: 1,
		BindingID: &binding.ID, ObservedAt: now,
	}
	if err := repository.RecordCapability(ctx, capability); err != nil {
		t.Fatalf("registrar capability: %v", err)
	}
	field := &LLMModelCapabilityField{
		ModelID: model.ID, CapabilityKey: string(llmcapabilities.CapabilityTTS), FieldKey: string(llmcapabilities.FieldVoice),
		SupportState: string(llmcapabilities.Supported), Source: binding.Source, Scope: string(llmcapabilities.ScopeExternalBinding),
		ProviderCompatibilityRevision: 1, BindingID: &binding.ID, ObservedAt: now,
	}
	if err := repository.RecordField(ctx, field, []LLMModelCapabilityFieldOption{{Value: "alloy", Label: "Alloy", SupportState: string(llmcapabilities.Supported)}}); err != nil {
		t.Fatalf("registrar campo e opção: %v", err)
	}

	for _, mutation := range []struct {
		name  string
		query string
		args  []any
	}{
		{"alterar opção", "UPDATE llm_model_capability_field_options SET label = ? WHERE assertion_id = ? AND value = ?", []any{"Changed", field.ID, "alloy"}},
		{"apagar opção", "DELETE FROM llm_model_capability_field_options WHERE assertion_id = ? AND value = ?", []any{field.ID, "alloy"}},
		{"apagar campo", "DELETE FROM llm_model_capability_fields WHERE id = ?", []any{field.ID}},
		{"apagar capability", "DELETE FROM llm_model_capabilities WHERE id = ?", []any{capability.ID}},
		{"apagar vínculo", "DELETE FROM llm_model_catalog_bindings WHERE id = ?", []any{binding.ID}},
	} {
		if err := db.Exec(mutation.query, mutation.args...).Error; err == nil {
			t.Errorf("SQLite permitiu %s enquanto o modelo existe", mutation.name)
		}
	}
	if err := db.Exec("DELETE FROM llm_models WHERE id = ?", model.ID).Error; err != nil {
		t.Fatalf("excluir modelo deveria remover fatos em cascata: %v", err)
	}
	for _, table := range []string{"llm_model_catalog_bindings", "llm_model_capabilities", "llm_model_capability_fields", "llm_model_capability_field_options"} {
		var count int64
		if err := db.Table(table).Count(&count).Error; err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Errorf("excluir modelo deixou %d linhas em %s", count, table)
		}
	}

	// Também cobre o caminho de exclusão do provedor com fatos ainda presentes.
	secondModel, err := repository.SaveModel(ctx, "provider-a", "remote-model-2", "Remote 2")
	if err != nil {
		t.Fatal(err)
	}
	secondBinding := &LLMModelCatalogBinding{
		ModelID: secondModel.ID, ProviderCompatibilityRevision: 1, Source: string(llmcapabilities.SourceOfficialCatalog),
		ExternalProviderID: "vendor", ExternalModelID: "remote-model-2", VerifiedAt: now.Add(-time.Minute),
	}
	if err := repository.BindCatalogModel(ctx, secondBinding); err != nil {
		t.Fatalf("registrar segundo vínculo: %v", err)
	}
	secondCapability := &LLMModelCapability{
		ModelID: secondModel.ID, CapabilityKey: string(llmcapabilities.CapabilityChat), SupportState: string(llmcapabilities.Supported),
		Source: secondBinding.Source, Scope: string(llmcapabilities.ScopeExternalBinding), ProviderCompatibilityRevision: 1,
		BindingID: &secondBinding.ID, ObservedAt: now,
	}
	if err := repository.RecordCapability(ctx, secondCapability); err != nil {
		t.Fatalf("registrar segunda capability: %v", err)
	}
	secondField := &LLMModelCapabilityField{
		ModelID: secondModel.ID, CapabilityKey: string(llmcapabilities.CapabilityTTS), FieldKey: string(llmcapabilities.FieldVoice),
		SupportState: string(llmcapabilities.Supported), Source: secondBinding.Source, Scope: string(llmcapabilities.ScopeExternalBinding),
		ProviderCompatibilityRevision: 1, BindingID: &secondBinding.ID, ObservedAt: now,
	}
	if err := repository.RecordField(ctx, secondField, []LLMModelCapabilityFieldOption{{Value: "alloy", Label: "Alloy", SupportState: string(llmcapabilities.Supported)}}); err != nil {
		t.Fatalf("registrar segundo campo e opção: %v", err)
	}

	if err := db.Exec("DELETE FROM llm_providers WHERE id = ?", "provider-a").Error; err != nil {
		t.Fatalf("excluir provedor deveria remover modelo e fatos em cascata: %v", err)
	}
	for _, table := range []string{"llm_models", "llm_model_catalog_bindings", "llm_model_capabilities", "llm_model_capability_fields", "llm_model_capability_field_options"} {
		var count int64
		if err := db.Table(table).Count(&count).Error; err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Errorf("excluir provedor deixou %d linhas em %s", count, table)
		}
	}
}

func timePointer(value time.Time) *time.Time {
	return &value
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

func TestBumpCompatibilityRevisionsForCredentialPatternIsUserScoped(t *testing.T) {
	db := newMigratorTestDB(t)
	if err := db.AutoMigrate(&LLMProvider{}); err != nil {
		t.Fatal(err)
	}
	providers := []LLMProvider{
		{ID: "provider-a1", UserID: "owner-a", Name: "A1", Type: "custom", BaseURL: "https://one.example/v1", CredentialPattern: "shared-pattern"},
		{ID: "provider-a2", UserID: "owner-a", Name: "A2", Type: "custom", BaseURL: "https://two.example/v1", CredentialPattern: "shared-pattern"},
		{ID: "provider-a3", UserID: "owner-a", Name: "A3", Type: "custom", BaseURL: "https://three.example/v1", CredentialPattern: "other-pattern"},
		{ID: "provider-b1", UserID: "owner-b", Name: "B1", Type: "custom", BaseURL: "https://four.example/v1", CredentialPattern: "shared-pattern"},
		{ID: "provider-bootstrap", UserID: "", Name: "Bootstrap", Type: "custom", BaseURL: "https://bootstrap.example/v1", CredentialPattern: "shared-pattern"},
	}
	if err := db.Create(&providers).Error; err != nil {
		t.Fatal(err)
	}
	repository := NewProviderRepository(db)
	revisions, err := repository.BumpCompatibilityRevisionsForCredentialPattern(WithUserID(context.Background(), "owner-a"), "shared-pattern")
	if err != nil {
		t.Fatalf("avançar revisão dos consumidores do pattern: %v", err)
	}
	if len(revisions) != 2 || revisions["provider-a1"] != 2 || revisions["provider-a2"] != 2 {
		t.Fatalf("revisões retornadas incorretas: %v", revisions)
	}
	for id, want := range map[string]int{"provider-a1": 2, "provider-a2": 2, "provider-a3": 1} {
		provider, err := repository.GetLLMProvider(WithUserID(context.Background(), "owner-a"), id)
		if err != nil {
			t.Errorf("provedor %s: erro ao ler revisão esperada %d: %v", id, want, err)
			continue
		}
		if provider.CompatibilityRevision != want {
			t.Errorf("provedor %s: revisão=%d, esperada %d", id, provider.CompatibilityRevision, want)
		}
	}
	foreign, err := repository.GetLLMProvider(WithUserID(context.Background(), "owner-b"), "provider-b1")
	if err != nil {
		t.Fatalf("ler provedor de outro usuário: %v", err)
	}
	if foreign.CompatibilityRevision != 1 {
		t.Fatalf("invalidação escapou ao usuário dono: revision=%d", foreign.CompatibilityRevision)
	}
	bootstrapRevisions, err := repository.BumpCompatibilityRevisionsForCredentialPattern(WithBootstrap(context.Background()), "shared-pattern")
	if err != nil {
		t.Fatalf("invalidação no escopo bootstrap: %v", err)
	}
	if len(bootstrapRevisions) != 1 || bootstrapRevisions["provider-bootstrap"] != 2 {
		t.Fatalf("bootstrap deveria invalidar apenas providers órfãos: %v", bootstrapRevisions)
	}
	if _, err := repository.BumpCompatibilityRevisionsForCredentialPattern(context.Background(), "shared-pattern"); !errors.Is(err, ErrUserScopeRequired) {
		t.Fatalf("invalidação sem usuário deveria falhar fechada: %v", err)
	}
}

func llmModelCapabilitiesTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db := newMigratorTestDB(t)
	if err := db.Exec("PRAGMA foreign_keys = ON").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&LLMProvider{}, &CredentialEntry{}); err != nil {
		t.Fatal(err)
	}
	if err := MigrateLLMModelCapabilities(db); err != nil {
		t.Fatal(err)
	}
	return db
}

func TestCredentialEntryTriggersAdvanceOnlyMatchingProviderRevisions(t *testing.T) {
	db := llmModelCapabilitiesTestDB(t)
	repository := NewProviderRepository(db)
	ownerA := WithUserID(context.Background(), "owner-a")
	ownerB := WithUserID(context.Background(), "owner-b")
	for _, provider := range []*LLMProvider{
		{ID: "owner-a-1", Name: "A1", Type: "custom", APIFormat: "openai", BaseURL: "https://one.example.test/v1", CredentialPattern: "shared-pattern"},
		{ID: "owner-a-2", Name: "A2", Type: "custom", APIFormat: "openai", BaseURL: "https://two.example.test/v1", CredentialPattern: "shared-pattern"},
		{ID: "owner-a-other", Name: "A other", Type: "custom", APIFormat: "openai", BaseURL: "https://other.example.test/v1", CredentialPattern: "other-pattern"},
		{ID: "owner-a-oauth", Name: "A OAuth", Type: "chatgpt", APIFormat: "openai_responses", BaseURL: "https://api.openai.com/v1", CredentialPattern: "oauth:grant"},
		{ID: "owner-b-1", Name: "B1", Type: "custom", APIFormat: "openai", BaseURL: "https://one.example.test/v1", CredentialPattern: "shared-pattern"},
	} {
		ctx := ownerA
		if provider.ID == "owner-b-1" {
			ctx = ownerB
		}
		if err := repository.SaveLLMProvider(ctx, provider); err != nil {
			t.Fatalf("criar provedor %s: %v", provider.ID, err)
		}
	}

	credential := CredentialEntry{
		UUIDModel: UUIDModel{ID: "credential-a"},
		UserID:    "owner-a", Pattern: "shared-pattern", TokenEnc: "ciphertext-one",
	}
	if err := db.Create(&credential).Error; err != nil {
		t.Fatalf("inserir credencial: %v", err)
	}

	assertRevisions := func(stage string, expected map[string]int) {
		t.Helper()
		for id, want := range expected {
			provider, err := repository.GetLLMProvider(ownerA, id)
			if id == "owner-b-1" {
				provider, err = repository.GetLLMProvider(ownerB, id)
			}
			if err != nil {
				t.Errorf("%s: ler %s: %v", stage, id, err)
				continue
			}
			if provider.CompatibilityRevision != want {
				t.Errorf("%s: %s revision=%d, esperado %d", stage, id, provider.CompatibilityRevision, want)
			}
		}
	}
	assertRevisions("insert", map[string]int{"owner-a-1": 2, "owner-a-2": 2, "owner-a-other": 1, "owner-b-1": 1})

	if err := db.Model(&CredentialEntry{}).Where("id = ?", credential.ID).Update("token_enc", "ciphertext-two").Error; err != nil {
		t.Fatalf("atualizar credencial: %v", err)
	}
	assertRevisions("update", map[string]int{"owner-a-1": 3, "owner-a-2": 3, "owner-a-other": 1, "owner-b-1": 1})

	if err := db.Where("id = ?", credential.ID).Delete(&CredentialEntry{}).Error; err != nil {
		t.Fatalf("excluir credencial: %v", err)
	}
	assertRevisions("delete", map[string]int{"owner-a-1": 4, "owner-a-2": 4, "owner-a-other": 1, "owner-b-1": 1})

	oauthCredential := CredentialEntry{
		UUIDModel: UUIDModel{ID: "oauth-credential"},
		UserID:    "owner-a", Pattern: "oauth:grant", TokenEnc: "oauth-token-one",
	}
	if err := db.Create(&oauthCredential).Error; err != nil {
		t.Fatalf("inserir token OAuth: %v", err)
	}
	assertRevisions("oauth insert", map[string]int{"owner-a-oauth": 1})
	if err := db.Model(&CredentialEntry{}).Where("id = ?", oauthCredential.ID).Update("token_enc", "oauth-token-two").Error; err != nil {
		t.Fatalf("rotacionar token OAuth: %v", err)
	}
	assertRevisions("oauth token rotation", map[string]int{"owner-a-oauth": 1})
	if err := db.Where("id = ?", oauthCredential.ID).Delete(&CredentialEntry{}).Error; err != nil {
		t.Fatalf("excluir token OAuth: %v", err)
	}
	assertRevisions("oauth delete", map[string]int{"owner-a-oauth": 1})
}

func TestSetDefaultProviderAdvancesConfigRevisionsAtomically(t *testing.T) {
	db := llmModelCapabilitiesTestDB(t)
	repository := NewProviderRepository(db)
	ctx := WithUserID(context.Background(), "owner-a")
	providers := []*LLMProvider{
		{ID: "default-a", Name: "A", Type: "custom", APIFormat: "openai", BaseURL: "https://a.example/v1", IsDefault: true},
		{ID: "default-b", Name: "B", Type: "custom", APIFormat: "openai", BaseURL: "https://b.example/v1"},
	}
	for _, provider := range providers {
		if err := repository.SaveLLMProvider(ctx, provider); err != nil {
			t.Fatal(err)
		}
	}

	if err := repository.SetDefaultProvider(ctx, "default-b"); err != nil {
		t.Fatal(err)
	}
	first, err := repository.GetLLMProvider(ctx, "default-a")
	if err != nil {
		t.Fatal(err)
	}
	second, err := repository.GetLLMProvider(ctx, "default-b")
	if err != nil {
		t.Fatal(err)
	}
	if first.IsDefault || first.ConfigRevision != 2 || !second.IsDefault || second.ConfigRevision != 2 {
		t.Fatalf("revisions after default switch: previous=%+v next=%+v", first, second)
	}

	if err := repository.SetDefaultProvider(ctx, "default-b"); err != nil {
		t.Fatal(err)
	}
	second, err = repository.GetLLMProvider(ctx, "default-b")
	if err != nil || second.ConfigRevision != 2 {
		t.Fatalf("idempotent set changed config revision: provider=%+v err=%v", second, err)
	}
	if err := repository.SetDefaultProvider(ctx, "missing"); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("missing provider error = %v", err)
	}
	second, err = repository.GetLLMProvider(ctx, "default-b")
	if err != nil || !second.IsDefault || second.ConfigRevision != 2 {
		t.Fatalf("failed default switch changed state: provider=%+v err=%v", second, err)
	}
}

func floatPtr(value float64) *float64 { return &value }
