package database

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestMigration39ProtectsLegacyOptionWhitespaceWithoutRewritingFacts(t *testing.T) {
	for _, invalidExisting := range []bool{false, true} {
		name := "valid-legacy"
		if invalidExisting {
			name = "invalid-legacy"
		}
		t.Run(name, func(t *testing.T) {
			db := newMigratorTestDB(t)
			if err := db.AutoMigrate(&LLMProvider{}, &CredentialEntry{}); err != nil {
				t.Fatal(err)
			}
			for _, ddl := range llmModelCapabilitiesDDL {
				legacyDDL := strings.Replace(ddl, "length(trim(value, "+llmOptionWhitespaceSQL+")) > 0", "length(value) > 0", 1)
				if err := db.Exec(legacyDDL).Error; err != nil {
					t.Fatal(err)
				}
			}
			if err := seedLLMCapabilityCatalog(db); err != nil {
				t.Fatal(err)
			}
			if err := db.Create(&LLMProvider{ID: "provider", UserID: "owner", Name: "Provider", Type: "custom"}).Error; err != nil {
				t.Fatal(err)
			}
			repository := NewLLMModelCapabilitiesRepository(db)
			model, err := repository.SaveModel(WithUserID(context.Background(), "owner"), "provider", "remote", "Remote")
			if err != nil {
				t.Fatal(err)
			}
			field := LLMModelCapabilityField{ModelID: model.ID, CapabilityKey: "tts", FieldKey: "voice", SupportState: "supported", Source: "endpoint_discovery", Scope: "connection", ProviderCompatibilityRevision: 1, ObservedAt: time.Now().Add(-time.Second)}
			if err := db.Create(&field).Error; err != nil {
				t.Fatal(err)
			}
			if invalidExisting {
				if err := db.Create(&LLMModelCapabilityFieldOption{AssertionID: field.ID, Value: "\t\u00a0", SupportState: "supported"}).Error; err != nil {
					t.Fatalf("semear opção legada: %v", err)
				}
				if err := MigrateLLMModelCapabilities(db); err == nil || !strings.Contains(err.Error(), "somente whitespace") {
					t.Fatalf("legado divergente aceito: %v", err)
				}
				var saved LLMModelCapabilityFieldOption
				if err := db.First(&saved, "assertion_id = ?", field.ID).Error; err != nil || saved.Value != "\t\u00a0" {
					t.Fatalf("histórico legado reescrito: %+v %v", saved, err)
				}
				return
			}
			if err := MigrateLLMModelCapabilities(db); err != nil {
				t.Fatal(err)
			}
			// Criar campo aberto após a migração evita a recusa pelo selo legado.
			field.UUIDModel = UUIDModel{}
			if err := db.Create(&field).Error; err != nil {
				t.Fatal(err)
			}
			if err := db.Create(&LLMModelCapabilityFieldOption{AssertionID: field.ID, Value: "\u00a0", SupportState: "supported"}).Error; err == nil || !strings.Contains(err.Error(), "non-whitespace value") {
				t.Fatalf("guard do schema legado não recusou NBSP: %v", err)
			}
			if err := db.Create(&LLMModelCapabilityFieldOption{AssertionID: field.ID, Value: "voice", SupportState: "supported"}).Error; err != nil {
				t.Fatal(err)
			}
			if err := MigrateLLMModelCapabilities(db); err != nil {
				t.Fatalf("migração não idempotente: %v", err)
			}
		})
	}
}
