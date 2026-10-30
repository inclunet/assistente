package database

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"assistente/internal/llmcapabilities"

	"gorm.io/gorm"
)

func legacyModelIdentityDB(t *testing.T) *gorm.DB {
	t.Helper()
	db := llmModelCapabilitiesTestDB(t)
	for _, table := range []string{"llm_model_capability_fields", "llm_model_capabilities", "llm_models"} {
		if err := db.Exec("DROP TABLE " + table).Error; err != nil {
			t.Fatal(err)
		}
	}
	for _, statement := range llmModelCapabilityDDL {
		old := strings.ReplaceAll(statement, "length(trim(remote_id)) > 0", "trim(remote_id) = remote_id")
		if err := db.Exec(old).Error; err != nil {
			t.Fatal(err)
		}
	}
	return db
}

func TestMigrateExactModelIDsPreservesEvidenceAndForeignKeys(t *testing.T) {
	db := legacyModelIdentityDB(t)
	ctx := WithUserID(context.Background(), "owner")
	provider := &LLMProvider{ID: "proxy", UserID: "owner", Name: "Proxy", Type: "custom", BaseURL: "https://example.com/v1"}
	if err := NewProviderRepository(db).SaveLLMProvider(ctx, provider); err != nil {
		t.Fatal(err)
	}
	repo := NewLLMModelCapabilitiesRepository(db)
	model, err := repo.SaveModel(ctx, provider.ID, "model", "Readable")
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.RecordUnsupportedField(ctx, model.ID, llmcapabilities.CapabilityChatCompletions, llmcapabilities.FieldTemperature, provider.CompatibilityRevision, "openai.chat.unsupported_parameter"); err != nil {
		t.Fatal(err)
	}
	before, err := repo.ListUnsupportedFieldsForRevision(ctx, model.ID, provider.CompatibilityRevision)
	if err != nil {
		t.Fatal(err)
	}
	for run := 0; run < 2; run++ {
		if err := migrateLLMModelExactIDs(db); err != nil {
			t.Fatal(err)
		}
		var enabled int
		if err := db.Raw("PRAGMA foreign_keys").Scan(&enabled).Error; err != nil || enabled != 1 {
			t.Fatalf("foreign keys=%d err=%v", enabled, err)
		}
		after, err := repo.ListUnsupportedFieldsForRevision(ctx, model.ID, provider.CompatibilityRevision)
		if err != nil || !reflect.DeepEqual(before, after) {
			t.Fatalf("evidence changed: before=%+v after=%+v err=%v", before, after, err)
		}
		reloaded, err := repo.GetOrCreateModel(ctx, provider.ID, "model", "Ignored")
		if err != nil || reloaded.ID != model.ID || reloaded.DisplayName != "Readable" {
			t.Fatalf("identity changed: %+v %v", reloaded, err)
		}
	}
	padded, err := repo.GetOrCreateModel(ctx, provider.ID, " model ", " model ")
	if err != nil || padded.ID == model.ID || padded.RemoteID != " model " {
		t.Fatalf("padded model: %+v %v", padded, err)
	}
	if err := repo.RecordUnsupportedField(ctx, padded.ID, llmcapabilities.CapabilityChatCompletions, llmcapabilities.FieldTopP, provider.CompatibilityRevision, "openai.chat.unsupported_parameter"); err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&LLMModel{}).Where("id = ?", padded.ID).Update("remote_id", "changed").Error; err == nil {
		t.Fatal("identity guard lost")
	}
	var backups int
	if err := db.Raw("SELECT count(*) FROM sqlite_master WHERE name LIKE '%_exact_id_backup'").Scan(&backups).Error; err != nil || backups != 0 {
		t.Fatalf("backup tables=%d %v", backups, err)
	}
}

func TestMigrateExactModelIDsRollsBackOnInvalidLegacyReference(t *testing.T) {
	db := legacyModelIdentityDB(t)
	// Construct a corrupt legacy fixture; the migration itself runs with FKs ON.
	if err := db.Exec("PRAGMA foreign_keys = OFF").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`INSERT INTO llm_model_capabilities (id,model_id,capability_code,created_at,updated_at) VALUES ('orphan','missing','responses',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("PRAGMA foreign_keys = ON").Error; err != nil {
		t.Fatal(err)
	}
	if err := migrateLLMModelExactIDs(db); err == nil {
		t.Fatal("corrupt reference accepted")
	}
	var schema string
	if err := db.Raw("SELECT sql FROM sqlite_master WHERE name='llm_models'").Scan(&schema).Error; err != nil || !strings.Contains(schema, "trim(remote_id) = remote_id") {
		t.Fatal("original schema not restored")
	}
	var count int64
	if err := db.Table("llm_model_capabilities").Where("id='orphan'").Count(&count).Error; err != nil || count != 1 {
		t.Fatal("original rows not restored")
	}
	if err := db.Raw("SELECT count(*) FROM sqlite_master WHERE name LIKE '%_exact_id_backup'").Scan(&count).Error; err != nil || count != 0 {
		t.Fatal("backup tables leaked after rollback")
	}
}
