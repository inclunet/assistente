package database

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestMigration38PreservesModelIdentityAndSealedOptions(t *testing.T) {
	db := llmModelCapabilitiesTestDB(t)
	for _, pragma := range []string{"PRAGMA foreign_keys = ON", "PRAGMA recursive_triggers = OFF"} {
		if err := db.Exec(pragma).Error; err != nil {
			t.Fatal(err)
		}
	}
	ctx := WithUserID(context.Background(), "owner")
	for _, id := range []string{"original-provider", "other-provider"} {
		if err := db.Create(&LLMProvider{ID: id, UserID: "owner", Name: id, Type: "custom", BaseURL: "https://api.example.com"}).Error; err != nil {
			t.Fatal(err)
		}
	}
	repository := NewLLMModelCapabilitiesRepository(db)
	model, err := repository.SaveModel(ctx, "original-provider", "tts-model", "Original")
	if err != nil {
		t.Fatal(err)
	}
	field := &LLMModelCapabilityField{ModelID: model.ID, CapabilityKey: "tts", FieldKey: "voice", SupportState: "supported", Source: "endpoint_discovery", Scope: "connection", ProviderCompatibilityRevision: 1, ObservedAt: time.Now().UTC().Add(-time.Second)}
	if err := repository.RecordField(ctx, field, []LLMModelCapabilityFieldOption{{Value: "alloy", Label: "Alloy", SupportState: "supported"}}); err != nil {
		t.Fatal(err)
	}
	for _, mutation := range []struct {
		query   string
		args    []any
		message string
	}{
		{"INSERT OR REPLACE INTO llm_models SELECT * FROM llm_models WHERE id = ?", []any{model.ID}, "model identity cannot be replaced"},
		{"INSERT OR REPLACE INTO llm_models (id,provider_id,remote_id,display_name,created_at,updated_at) SELECT ?,provider_id,remote_id,display_name,created_at,updated_at FROM llm_models WHERE id = ?", []any{"replacement-model", model.ID}, "model identity cannot be replaced"},
		{"UPDATE llm_models SET remote_id = ? WHERE id = ?", []any{"different-model", model.ID}, "model identity is immutable"},
		{"UPDATE llm_models SET provider_id = ? WHERE id = ?", []any{"other-provider", model.ID}, "model identity is immutable"},
		{"INSERT INTO llm_model_capability_field_options (assertion_id,value,label,support_state) VALUES (?,?,?,?)", []any{field.ID, "nova", "Nova", "supported"}, "field option set is sealed"},
		{"DELETE FROM llm_model_capability_field_seals WHERE assertion_id = ?", []any{field.ID}, "field option seal is append-only"},
		{"INSERT OR REPLACE INTO llm_model_capability_field_seals (assertion_id) VALUES (?)", []any{field.ID}, "field option seal cannot be replaced"},
		{"UPDATE llm_model_capability_field_seals SET assertion_id = ? WHERE assertion_id = ?", []any{"different", field.ID}, "field option seal is immutable"},
	} {
		if err := db.Exec(mutation.query, mutation.args...).Error; err == nil || !strings.Contains(err.Error(), mutation.message) {
			t.Fatalf("historical mutation accepted or wrong guard: %s %v", mutation.query, err)
		}
	}
	renamed, err := repository.SaveModel(ctx, "original-provider", "tts-model", "New display name")
	if err != nil || renamed.ID != model.ID {
		t.Fatalf("metadata update blocked: %+v %v", renamed, err)
	}
	resolution, err := repository.Resolve(ctx, model.ID, time.Now())
	if err != nil || len(resolution.Fields["tts"]["voice"].Options) != 1 || resolution.Fields["tts"]["voice"].Options[0].Value != "alloy" {
		t.Fatalf("historical options changed: %+v %v", resolution, err)
	}
	replacement := *field
	replacement.ID = ""
	replacement.ObservedAt = time.Now().UTC()
	if err := repository.RecordField(ctx, &replacement, []LLMModelCapabilityFieldOption{{Value: "alloy", Label: "Alloy", SupportState: "supported"}, {Value: "nova", Label: "Nova", SupportState: "supported"}}); err != nil {
		t.Fatalf("new option set blocked: %v", err)
	}
	if err := db.Exec("DELETE FROM llm_providers WHERE id = ?", "original-provider").Error; err != nil {
		t.Fatalf("cascade blocked: %v", err)
	}
	var seals int64
	if err := db.Table("llm_model_capability_field_seals").Count(&seals).Error; err != nil || seals != 0 {
		t.Fatalf("cascade retained seals: %d %v", seals, err)
	}
}

func TestMigration38SealsLegacyOptionSetsIdempotently(t *testing.T) {
	db := newMigratorTestDB(t)
	for _, statement := range []string{
		`CREATE TABLE llm_models (id TEXT PRIMARY KEY,provider_id TEXT,remote_id TEXT)`,
		`CREATE TABLE llm_model_capability_fields (id TEXT PRIMARY KEY)`,
		`CREATE TABLE llm_model_capability_field_options (assertion_id TEXT,value TEXT,label TEXT,support_state TEXT,PRIMARY KEY(assertion_id,value))`,
		`INSERT INTO llm_model_capability_fields VALUES ('legacy')`,
		`INSERT INTO llm_model_capability_field_options VALUES ('legacy','alloy','Alloy','supported')`,
	} {
		if err := db.Exec(statement).Error; err != nil {
			t.Fatal(err)
		}
	}
	for range 2 {
		if err := migrateLLMModelIdentityAndOptionSeals(db); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Exec(`INSERT INTO llm_model_capability_field_options VALUES ('legacy','nova','Nova','supported')`).Error; err == nil || !strings.Contains(err.Error(), "field option set is sealed") {
		t.Fatalf("legacy set was not sealed: %v", err)
	}
	var count int64
	if err := db.Table("llm_model_capability_field_options").Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("legacy options rewritten: %d %v", count, err)
	}
}

func TestLLMModelFieldPublicationRequiresSealAndRollsBackFailedOptions(t *testing.T) {
	db := llmModelCapabilitiesTestDB(t)
	for _, pragma := range []string{"PRAGMA foreign_keys = ON", "PRAGMA recursive_triggers = OFF"} {
		if err := db.Exec(pragma).Error; err != nil {
			t.Fatal(err)
		}
	}
	ctx := WithUserID(context.Background(), "owner")
	if err := db.Create(&LLMProvider{ID: "publication", UserID: "owner", Name: "Publication", Type: "custom"}).Error; err != nil {
		t.Fatal(err)
	}
	repository := NewLLMModelCapabilitiesRepository(db)
	model, err := repository.SaveModel(ctx, "publication", "tts", "")
	if err != nil {
		t.Fatal(err)
	}
	unsealed := &LLMModelCapabilityField{ModelID: model.ID, CapabilityKey: "tts", FieldKey: "voice", SupportState: "supported", Source: "endpoint_discovery", Scope: "connection", ProviderCompatibilityRevision: 1, ObservedAt: time.Now().UTC().Add(-time.Second)}
	if err := db.Create(unsealed).Error; err != nil {
		t.Fatal(err)
	}
	resolution, err := repository.Resolve(ctx, model.ID, time.Now())
	if err != nil || resolution.Fields["tts"]["voice"].State == "supported" {
		t.Fatalf("unsealed field governed resolution: %+v %v", resolution, err)
	}
	if err := db.Exec(`CREATE TRIGGER fail_test_option BEFORE INSERT ON llm_model_capability_field_options
        WHEN NEW.value = 'blocked' BEGIN SELECT RAISE(ABORT,'test option insert failed'); END`).Error; err != nil {
		t.Fatal(err)
	}
	failed := *unsealed
	failed.ID = ""
	if err := repository.RecordField(ctx, &failed, []LLMModelCapabilityFieldOption{{Value: "blocked", Label: "Blocked", SupportState: "supported"}}); err == nil || !strings.Contains(err.Error(), "test option insert failed") {
		t.Fatalf("option failure not propagated: %v", err)
	}
	var count int64
	if err := db.Model(&LLMModelCapabilityField{}).Where("id = ?", failed.ID).Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("failed publication retained field: %d %v", count, err)
	}
	if err := db.Table("llm_model_capability_field_seals").Where("assertion_id = ?", failed.ID).Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("failed publication retained seal: %d %v", count, err)
	}
	if err := db.Exec("DROP TRIGGER fail_test_option").Error; err != nil {
		t.Fatal(err)
	}
	if err := migrateLLMModelIdentityAndOptionSeals(db); err != nil {
		t.Fatal(err)
	}
	if err := db.Table("llm_model_capability_field_seals").Where("assertion_id = ?", unsealed.ID).Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("idempotent migration published unfinished field: %d %v", count, err)
	}
}
