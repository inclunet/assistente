package database

import (
	"context"
	"errors"
	"sync"
	"testing"

	"assistente/internal/llmcapabilities"
	"gorm.io/gorm"
)

func TestMigrateLLMModelCompatibilityStateIsMinimalAndIdempotent(t *testing.T) {
	db := llmModelCapabilitiesTestDB(t)
	if err := MigrateLLMModelCapabilities(db); err != nil {
		t.Fatalf("segunda migração: %v", err)
	}
	for _, table := range []string{"llm_models", "llm_model_capabilities", "llm_model_capability_fields"} {
		if !db.Migrator().HasTable(table) {
			t.Errorf("tabela %s ausente", table)
		}
	}
	for _, removed := range []string{
		"llm_capabilities", "llm_capability_fields", "llm_model_catalog_bindings",
		"llm_model_capability_field_options", "llm_model_capability_field_seals",
		"llm_model_prices",
	} {
		if db.Migrator().HasTable(removed) {
			t.Errorf("tabela retirada do AEP continua criada: %s", removed)
		}
	}
	for _, column := range []string{"user_id", "api_format"} {
		if db.Migrator().HasColumn("llm_models", column) {
			t.Errorf("llm_models não pode duplicar %s", column)
		}
	}
	for _, column := range []string{"compatibility_revision"} {
		if !db.Migrator().HasColumn("llm_providers", column) {
			t.Errorf("revisão ausente no provedor: %s", column)
		}
	}
	for _, column := range []string{"compatibility_revision", "recognizer_id", "updated_at"} {
		if !db.Migrator().HasColumn("llm_model_capability_fields", column) {
			t.Errorf("metadado da restrição ausente: %s", column)
		}
	}
}

func TestLLMModelRepositoryScopesIdentityAndList(t *testing.T) {
	db := llmModelCapabilitiesTestDB(t)
	ctxA := WithUserID(context.Background(), "owner-a")
	ctxB := WithUserID(context.Background(), "owner-b")
	providers := []*LLMProvider{
		{ID: "provider-a", UserID: "owner-a", Name: "A", Type: "custom", BaseURL: "https://a.example/v1"},
		{ID: "provider-b", UserID: "owner-b", Name: "B", Type: "custom", BaseURL: "https://b.example/v1"},
	}
	for _, provider := range providers {
		if err := NewProviderRepository(db).SaveLLMProvider(WithUserID(context.Background(), provider.UserID), provider); err != nil {
			t.Fatal(err)
		}
	}
	repository := NewLLMModelCapabilitiesRepository(db)
	first, err := repository.SaveModel(ctxA, "provider-a", "remote-model", "Original")
	if err != nil {
		t.Fatal(err)
	}
	repeated, err := repository.SaveModel(ctxA, "provider-a", "remote-model", "Renamed")
	if err != nil {
		t.Fatal(err)
	}
	if repeated.ID != first.ID || repeated.DisplayName != "Renamed" {
		t.Fatalf("identidade repetida não foi atualizada idempotentemente: first=%+v repeated=%+v", first, repeated)
	}
	other, err := repository.SaveModel(ctxB, "provider-b", "remote-model", "Same remote id")
	if err != nil {
		t.Fatal(err)
	}
	if other.ID == first.ID {
		t.Fatal("ID remoto igual compartilhou identidade entre provedores")
	}
	for _, remoteID := range []string{" remote-model", "remote-model ", "\tremote-model"} {
		if _, err := repository.SaveModel(ctxA, "provider-a", remoteID, "Invalid"); err == nil {
			t.Errorf("ID remoto com whitespace periférico foi normalizado em silêncio: %q", remoteID)
		}
	}
	models, err := repository.ListModels(ctxA, "provider-a")
	if err != nil || len(models) != 1 || models[0].ID != first.ID {
		t.Fatalf("lista de modelos: %+v, %v", models, err)
	}
	if _, err := repository.ListModels(ctxB, "provider-a"); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("usuário alheio leu modelos: %v", err)
	}
}

func TestRecordUnsupportedFieldUpdatesCurrentStateAndRejectsUnknownData(t *testing.T) {
	db := llmModelCapabilitiesTestDB(t)
	ctx := WithUserID(context.Background(), "owner")
	provider := &LLMProvider{ID: "provider", UserID: "owner", Name: "Provider", Type: "custom", BaseURL: "https://provider.example/v1"}
	if err := NewProviderRepository(db).SaveLLMProvider(ctx, provider); err != nil {
		t.Fatal(err)
	}
	repository := NewLLMModelCapabilitiesRepository(db)
	model, err := repository.SaveModel(ctx, provider.ID, "remote-model", "Model")
	if err != nil {
		t.Fatal(err)
	}
	before, err := repository.ListUnsupportedFields(ctx, model.ID)
	if err != nil || len(before) != 0 {
		t.Fatalf("ausência deveria significar unknown: %+v, %v", before, err)
	}
	if err := repository.RecordUnsupportedField(ctx, model.ID, llmcapabilities.CapabilityChatCompletions, llmcapabilities.FieldTemperature, provider.CompatibilityRevision, "openai.chat.unsupported_parameter"); err != nil {
		t.Fatal(err)
	}
	first, err := repository.ListUnsupportedFields(ctx, model.ID)
	if err != nil || len(first) != 1 {
		t.Fatalf("restrição não persistida: %+v, %v", first, err)
	}
	if first[0].CapabilityCode != string(llmcapabilities.CapabilityChatCompletions) ||
		first[0].FieldCode != string(llmcapabilities.FieldTemperature) ||
		first[0].RecognizerID != "openai.chat.unsupported_parameter" ||
		first[0].CompatibilityRevision != provider.CompatibilityRevision {
		t.Fatalf("restrição persistida diverge do contrato: %+v", first[0])
	}
	if err := repository.RecordUnsupportedField(ctx, model.ID, llmcapabilities.CapabilityChatCompletions, llmcapabilities.FieldTemperature, provider.CompatibilityRevision, "litellm.chat.unsupported_parameter"); err != nil {
		t.Fatal(err)
	}
	second, err := repository.ListUnsupportedFields(ctx, model.ID)
	if err != nil || len(second) != 1 || second[0].RecognizerID != "litellm.chat.unsupported_parameter" {
		t.Fatalf("repetição acumulou histórico em vez de atualizar: %+v, %v", second, err)
	}
	if err := repository.RecordUnsupportedField(ctx, model.ID, llmcapabilities.CapabilityChatCompletions, llmcapabilities.FieldTopP, provider.CompatibilityRevision, "openai.chat.unsupported_parameter"); err != nil {
		t.Fatalf("segundo campo na mesma capability: %v", err)
	}
	third, err := repository.ListUnsupportedFields(ctx, model.ID)
	if err != nil || len(third) != 2 {
		t.Fatalf("segundo campo não reutilizou a capability: %+v, %v", third, err)
	}
	for _, test := range []struct {
		capability llmcapabilities.Capability
		field      llmcapabilities.FieldKey
		recognizer string
	}{
		{"unlisted.operation", "temperature", "openai.chat.error"},
		{llmcapabilities.CapabilityTextToSpeech, llmcapabilities.FieldTemperature, "openai.chat.error"},
		{llmcapabilities.CapabilityChatCompletions, llmcapabilities.FieldTemperature, "raw provider message"},
	} {
		if err := repository.RecordUnsupportedField(ctx, model.ID, test.capability, test.field, provider.CompatibilityRevision, test.recognizer); err == nil {
			t.Errorf("persistiu entrada inválida: %+v", test)
		}
	}
}

func TestCompatibilityRevisionInvalidatesFieldsAndRejectsStaleWrite(t *testing.T) {
	db := llmModelCapabilitiesTestDB(t)
	ctx := WithUserID(context.Background(), "owner")
	provider := &LLMProvider{ID: "provider", UserID: "owner", Name: "Provider", Type: "custom", APIFormat: "openai", BaseURL: "https://one.example/v1"}
	providerRepository := NewProviderRepository(db)
	if err := providerRepository.SaveLLMProvider(ctx, provider); err != nil {
		t.Fatal(err)
	}
	modelRepository := NewLLMModelCapabilitiesRepository(db)
	model, err := modelRepository.SaveModel(ctx, provider.ID, "remote-model", "Model")
	if err != nil {
		t.Fatal(err)
	}
	oldRevision := provider.CompatibilityRevision
	if err := modelRepository.RecordUnsupportedField(ctx, model.ID, llmcapabilities.CapabilityChatCompletions, llmcapabilities.FieldTemperature, oldRevision, "openai.chat.unsupported_parameter"); err != nil {
		t.Fatal(err)
	}
	provider.BaseURL = "https://two.example/v1"
	if err := providerRepository.SaveLLMProvider(ctx, provider); err != nil {
		t.Fatal(err)
	}
	if provider.CompatibilityRevision != oldRevision+1 {
		t.Fatalf("troca de endpoint não avançou revisão: %d", provider.CompatibilityRevision)
	}
	fields, err := modelRepository.ListUnsupportedFields(ctx, model.ID)
	if err != nil || len(fields) != 0 {
		t.Fatalf("restrição antiga não foi invalidada: %+v, %v", fields, err)
	}
	if err := modelRepository.RecordUnsupportedField(ctx, model.ID, llmcapabilities.CapabilityChatCompletions, llmcapabilities.FieldTemperature, oldRevision, "openai.chat.unsupported_parameter"); !errors.Is(err, ErrStaleCompatibility) {
		t.Fatalf("gravação atrasada não foi recusada: %v", err)
	}
}

func TestModelCompatibilityStateIsolatedAndCascadesWithProvider(t *testing.T) {
	db := llmModelCapabilitiesTestDB(t)
	ctx := WithUserID(context.Background(), "owner-a")
	otherCtx := WithUserID(context.Background(), "owner-b")
	provider := &LLMProvider{ID: "provider", UserID: "owner-a", Name: "Provider", Type: "custom", BaseURL: "https://provider.example/v1"}
	if err := NewProviderRepository(db).SaveLLMProvider(ctx, provider); err != nil {
		t.Fatal(err)
	}
	repository := NewLLMModelCapabilitiesRepository(db)
	model, err := repository.SaveModel(ctx, provider.ID, "remote-model", "Model")
	if err != nil {
		t.Fatal(err)
	}
	if err := repository.RecordUnsupportedField(ctx, model.ID, llmcapabilities.CapabilityResponses, llmcapabilities.FieldTopP, provider.CompatibilityRevision, "openai.responses.unsupported_parameter"); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.ListUnsupportedFields(otherCtx, model.ID); !errors.Is(err, ErrLLMModelNotFound) {
		t.Fatalf("usuário alheio consultou restrições: %v", err)
	}
	if err := repository.RecordUnsupportedField(otherCtx, model.ID, llmcapabilities.CapabilityResponses, llmcapabilities.FieldTopP, provider.CompatibilityRevision, "openai.responses.unsupported_parameter"); !errors.Is(err, ErrLLMModelNotFound) {
		t.Fatalf("usuário alheio alterou restrições: %v", err)
	}
	if err := db.Delete(&LLMProvider{}, "id = ?", provider.ID).Error; err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"llm_models", "llm_model_capabilities", "llm_model_capability_fields"} {
		var count int64
		if err := db.Table(table).Count(&count).Error; err != nil || count != 0 {
			t.Errorf("exclusão do provedor não cascadiou em %s: count=%d err=%v", table, count, err)
		}
	}
}

func TestConcurrentUnsupportedFieldLearningUpsertsOneCurrentRow(t *testing.T) {
	db := llmModelCapabilitiesTestDB(t)
	ctx := WithUserID(context.Background(), "owner")
	provider := &LLMProvider{ID: "provider", UserID: "owner", Name: "Provider", Type: "custom", BaseURL: "https://provider.example/v1"}
	if err := NewProviderRepository(db).SaveLLMProvider(ctx, provider); err != nil {
		t.Fatal(err)
	}
	repository := NewLLMModelCapabilitiesRepository(db)
	model, err := repository.SaveModel(ctx, provider.ID, "remote-model", "Model")
	if err != nil {
		t.Fatal(err)
	}
	const writers = 8
	var wait sync.WaitGroup
	errorsFound := make(chan error, writers)
	for index := 0; index < writers; index++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			errorsFound <- repository.RecordUnsupportedField(ctx, model.ID, llmcapabilities.CapabilityChatCompletions, llmcapabilities.FieldTemperature, provider.CompatibilityRevision, "openai.chat.unsupported_parameter")
		}()
	}
	wait.Wait()
	close(errorsFound)
	for err := range errorsFound {
		if err != nil {
			t.Errorf("gravação concorrente: %v", err)
		}
	}
	fields, err := repository.ListUnsupportedFields(ctx, model.ID)
	if err != nil || len(fields) != 1 {
		t.Fatalf("upsert concorrente criou mais de uma linha: %+v, %v", fields, err)
	}
}

func TestConcurrentModelCreationKeepsOneProviderRemoteIdentity(t *testing.T) {
	db := llmModelCapabilitiesTestDB(t)
	ctx := WithUserID(context.Background(), "owner")
	provider := &LLMProvider{ID: "provider", UserID: "owner", Name: "Provider", Type: "custom", BaseURL: "https://provider.example/v1"}
	if err := NewProviderRepository(db).SaveLLMProvider(ctx, provider); err != nil {
		t.Fatal(err)
	}
	repository := NewLLMModelCapabilitiesRepository(db)
	const writers = 8
	var wait sync.WaitGroup
	results := make(chan *LLMModel, writers)
	errorsFound := make(chan error, writers)
	for range writers {
		wait.Add(1)
		go func() {
			defer wait.Done()
			model, err := repository.SaveModel(ctx, provider.ID, "same-remote-model", "Model")
			if err != nil {
				errorsFound <- err
				return
			}
			results <- model
		}()
	}
	wait.Wait()
	close(results)
	close(errorsFound)
	for err := range errorsFound {
		t.Errorf("criação concorrente: %v", err)
	}
	var modelID string
	for model := range results {
		if modelID == "" {
			modelID = model.ID
		} else if model.ID != modelID {
			t.Errorf("uma identidade remota gerou IDs diferentes: %q e %q", modelID, model.ID)
		}
	}
	var count int64
	if err := db.Model(&LLMModel{}).Where("provider_id = ? AND remote_id = ?", provider.ID, "same-remote-model").Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("criações concorrentes deixaram %d modelos: %v", count, err)
	}
}

func TestModelAndProviderIdentityGuardsRejectUpdateAndReplace(t *testing.T) {
	db := llmModelCapabilitiesTestDB(t)
	ctx := WithUserID(context.Background(), "owner")
	provider := &LLMProvider{ID: "provider", UserID: "owner", Name: "Provider", Type: "custom", BaseURL: "https://provider.example/v1"}
	if err := NewProviderRepository(db).SaveLLMProvider(ctx, provider); err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("UPDATE llm_providers SET id = ? WHERE id = ?", "replacement-provider", provider.ID).Error; err == nil {
		t.Fatal("alteração direta substituiu a identidade do provedor")
	}
	if err := db.Exec("INSERT OR REPLACE INTO llm_providers (id) VALUES (?)", provider.ID).Error; err == nil {
		t.Fatal("INSERT OR REPLACE substituiu o provedor")
	}
	model, err := NewLLMModelCapabilitiesRepository(db).SaveModel(ctx, provider.ID, "remote-model", "Model")
	if err != nil {
		t.Fatal(err)
	}
	modelRepository := NewLLMModelCapabilitiesRepository(db)
	if err := modelRepository.RecordUnsupportedField(ctx, model.ID, llmcapabilities.CapabilityChatCompletions, llmcapabilities.FieldTemperature, provider.CompatibilityRevision, "openai.chat.unsupported_parameter"); err != nil {
		t.Fatal(err)
	}
	var capability LLMModelCapability
	if err := db.Where("model_id = ? AND capability_code = ?", model.ID, llmcapabilities.CapabilityChatCompletions).First(&capability).Error; err != nil {
		t.Fatal(err)
	}
	for _, update := range []struct {
		column string
		value  string
	}{
		{column: "model_id", value: "another-model"},
		{column: "capability_code", value: string(llmcapabilities.CapabilityTextToSpeech)},
	} {
		if err := db.Model(&LLMModelCapability{}).Where("id = ?", capability.ID).Update(update.column, update.value).Error; err == nil {
			t.Fatalf("alteração direta de %s transportou restrições para outra capability", update.column)
		}
	}
	if err := db.Exec(`INSERT OR REPLACE INTO llm_model_capabilities
		(id, model_id, capability_code, created_at, updated_at)
		SELECT id, model_id, capability_code, created_at, updated_at
		FROM llm_model_capabilities WHERE id = ?`, capability.ID).Error; err == nil {
		t.Fatal("INSERT OR REPLACE apagou a capability e suas restrições filhas")
	}
	if err := db.Exec("DROP TRIGGER IF EXISTS trg_llm_models_identity_immutable").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`CREATE TRIGGER trg_llm_models_identity_immutable
		BEFORE UPDATE OF provider_id, remote_id ON llm_models
		BEGIN SELECT RAISE(ABORT, 'llm model identity is immutable'); END`).Error; err != nil {
		t.Fatal(err)
	}
	if err := MigrateLLMModelCapabilities(db); err != nil {
		t.Fatalf("migração não atualizou o gatilho legado: %v", err)
	}
	if err := db.Exec("UPDATE llm_models SET remote_id = ? WHERE id = ?", "replacement-model", model.ID).Error; err == nil {
		t.Fatal("alteração direta substituiu a identidade do modelo")
	}
	if err := db.Exec(`INSERT OR REPLACE INTO llm_models
		(id, provider_id, remote_id, display_name, created_at, updated_at)
		SELECT id, provider_id, remote_id, display_name, created_at, updated_at
		FROM llm_models WHERE id = ?`, model.ID).Error; err == nil {
		t.Fatal("INSERT OR REPLACE substituiu o modelo")
	}
	if err := db.Model(&LLMModel{}).Where("id = ?", model.ID).Updates(map[string]any{
		"provider_id":  provider.ID,
		"remote_id":    "remote-model",
		"display_name": "Renamed model",
	}).Error; err != nil {
		t.Fatalf("atualização repetindo identidade imutável falhou: %v", err)
	}
	var renamed LLMModel
	if err := db.First(&renamed, "id = ?", model.ID).Error; err != nil {
		t.Fatal(err)
	}
	if renamed.ProviderID != provider.ID || renamed.RemoteID != "remote-model" || renamed.DisplayName != "Renamed model" {
		t.Fatalf("atualização de display name alterou identidade ou não foi aplicada: %+v", renamed)
	}
	var count int64
	if err := db.Model(&LLMModel{}).Where("id = ?", model.ID).Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("modelo original não foi preservado: count=%d err=%v", count, err)
	}
}

func TestCapabilityFieldIdentityGuardRejectsUpdate(t *testing.T) {
	db := llmModelCapabilitiesTestDB(t)
	ctx := WithUserID(context.Background(), "owner")
	provider := &LLMProvider{ID: "provider", UserID: "owner", Name: "Provider", Type: "custom", BaseURL: "https://provider.example/v1"}
	if err := NewProviderRepository(db).SaveLLMProvider(ctx, provider); err != nil {
		t.Fatal(err)
	}
	repository := NewLLMModelCapabilitiesRepository(db)
	firstModel, err := repository.SaveModel(ctx, provider.ID, "first-model", "First")
	if err != nil {
		t.Fatal(err)
	}
	secondModel, err := repository.SaveModel(ctx, provider.ID, "second-model", "Second")
	if err != nil {
		t.Fatal(err)
	}
	field := llmcapabilities.FieldTemperature
	if err := repository.RecordUnsupportedField(ctx, firstModel.ID, llmcapabilities.CapabilityChatCompletions, field, provider.CompatibilityRevision, "openai.chat.unsupported_parameter"); err != nil {
		t.Fatal(err)
	}
	targetField := llmcapabilities.FieldTopP
	if err := repository.RecordUnsupportedField(ctx, secondModel.ID, llmcapabilities.CapabilityChatCompletions, targetField, provider.CompatibilityRevision, "openai.chat.unsupported_parameter"); err != nil {
		t.Fatal(err)
	}
	var firstCapability, secondCapability LLMModelCapability
	if err := db.Where("model_id = ? AND capability_code = ?", firstModel.ID, llmcapabilities.CapabilityChatCompletions).First(&firstCapability).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Where("model_id = ? AND capability_code = ?", secondModel.ID, llmcapabilities.CapabilityChatCompletions).First(&secondCapability).Error; err != nil {
		t.Fatal(err)
	}
	for _, update := range []struct {
		column string
		value  string
	}{
		{column: "capability_id", value: secondCapability.ID},
		{column: "field_code", value: string(targetField)},
	} {
		if err := db.Model(&LLMModelCapabilityField{}).Where("capability_id = ? AND field_code = ?", firstCapability.ID, field).Update(update.column, update.value).Error; err == nil {
			t.Fatalf("alteração direta de %s transportou a restrição aprendida", update.column)
		}
	}
	var stored LLMModelCapabilityField
	if err := db.Where("capability_id = ? AND field_code = ?", firstCapability.ID, field).First(&stored).Error; err != nil {
		t.Fatalf("restrição original foi removida: %v", err)
	}
}

func TestSystemProviderCompatibilityWritesRequireBootstrap(t *testing.T) {
	db := llmModelCapabilitiesTestDB(t)
	userCtx := WithUserID(context.Background(), "owner")
	bootstrapCtx := WithBootstrap(context.Background())
	provider := &LLMProvider{ID: "system-provider", UserID: "", Name: "System", Type: "custom", BaseURL: "https://provider.example/v1"}
	if err := NewProviderRepository(db).SaveLLMProvider(bootstrapCtx, provider); err != nil {
		t.Fatal(err)
	}
	repository := NewLLMModelCapabilitiesRepository(db)
	if _, err := repository.SaveModel(userCtx, provider.ID, "remote-model", "Model"); !errors.Is(err, ErrSystemProviderWriteRequiresBootstrap) {
		t.Fatalf("usuário gravou modelo em provedor global: %v", err)
	}
	model, err := repository.SaveModel(bootstrapCtx, provider.ID, "remote-model", "Model")
	if err != nil {
		t.Fatal(err)
	}
	field := llmcapabilities.FieldTemperature
	if err := repository.RecordUnsupportedField(userCtx, model.ID, llmcapabilities.CapabilityChatCompletions, field, provider.CompatibilityRevision, "openai.chat.unsupported_parameter"); !errors.Is(err, ErrSystemProviderWriteRequiresBootstrap) {
		t.Fatalf("usuário publicou restrição em provedor global: %v", err)
	}
	if err := repository.RecordUnsupportedField(bootstrapCtx, model.ID, llmcapabilities.CapabilityChatCompletions, field, provider.CompatibilityRevision, "openai.chat.unsupported_parameter"); err != nil {
		t.Fatalf("bootstrap não pôde publicar restrição: %v", err)
	}
	fields, err := repository.ListUnsupportedFields(userCtx, model.ID)
	if err != nil || len(fields) != 1 {
		t.Fatalf("usuário não leu estado global aplicável: %+v %v", fields, err)
	}
}

func TestSaveLLMProviderRejectsNewForeignUserPayload(t *testing.T) {
	db := llmModelCapabilitiesTestDB(t)
	ctx := WithUserID(context.Background(), "owner-a")
	provider := &LLMProvider{ID: "foreign-provider", UserID: "owner-b", Name: "Foreign", Type: "custom", BaseURL: "https://provider.example/v1"}

	if err := NewProviderRepository(db).SaveLLMProvider(ctx, provider); !errors.Is(err, ErrProviderUserScopeMismatch) {
		t.Fatalf("SaveLLMProvider() error = %v, want ErrProviderUserScopeMismatch", err)
	}
	var count int64
	if err := db.Model(&LLMProvider{}).Where("id = ?", provider.ID).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("provider foi gravado no escopo de outro usuário: count=%d", count)
	}
}

func TestSaveLLMProviderRejectsForeignUserPayload(t *testing.T) {
	db := llmModelCapabilitiesTestDB(t)
	ctx := WithUserID(context.Background(), "owner-a")
	provider := &LLMProvider{ID: "owned-provider", UserID: "owner-a", Name: "Owned", Type: "custom", BaseURL: "https://provider.example/v1"}
	repository := NewProviderRepository(db)
	if err := repository.SaveLLMProvider(ctx, provider); err != nil {
		t.Fatalf("SaveLLMProvider(): %v", err)
	}

	provider.UserID = "owner-b"
	if err := repository.SaveLLMProvider(ctx, provider); !errors.Is(err, ErrProviderUserScopeMismatch) {
		t.Fatalf("SaveLLMProvider() error = %v, want ErrProviderUserScopeMismatch", err)
	}
	var stored LLMProvider
	if err := db.First(&stored, "id = ?", provider.ID).Error; err != nil {
		t.Fatal(err)
	}
	if stored.UserID != "owner-a" {
		t.Fatalf("provider mudou de proprietário: user_id=%q", stored.UserID)
	}
}

func llmModelCapabilitiesTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db := newMigratorTestDB(t)
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	sqlDB.SetMaxOpenConns(1)
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
