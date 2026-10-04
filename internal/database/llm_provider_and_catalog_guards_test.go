package database

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestMigration39PreventsReplacingProvidersAndPreservesModelHistory(t *testing.T) {
	for _, withModel := range []bool{false, true} {
		name := "empty-provider"
		if withModel {
			name = "provider-with-facts"
		}
		t.Run(name, func(t *testing.T) {
			db := llmModelCapabilitiesTestDB(t)
			for _, pragma := range []string{"PRAGMA foreign_keys = ON", "PRAGMA recursive_triggers = OFF"} {
				if err := db.Exec(pragma).Error; err != nil {
					t.Fatal(err)
				}
			}
			ctx := WithUserID(context.Background(), "owner")
			provider := &LLMProvider{ID: "protected", UserID: "owner", Name: "Original", Type: "custom", BaseURL: "https://example.com", CompatibilityRevision: 4, ConfigRevision: 8}
			if err := db.Create(provider).Error; err != nil {
				t.Fatal(err)
			}
			repository := NewLLMModelCapabilitiesRepository(db)
			var model *LLMModel
			if withModel {
				var err error
				model, err = repository.SaveModel(ctx, provider.ID, "model", "Model")
				if err != nil {
					t.Fatal(err)
				}
				fact := &LLMModelCapabilityField{ModelID: model.ID, CapabilityKey: "chat", FieldKey: "temperature", SupportState: "unsupported", Source: "execution_observation", Scope: "connection", ProviderCompatibilityRevision: 4, ObservedAt: time.Now().Add(-time.Second)}
				if err := repository.RecordField(ctx, fact, nil); err != nil {
					t.Fatal(err)
				}
			}
			for range 2 {
				if err := migrateLLMProviderAndCatalogGuards(db); err != nil {
					t.Fatal(err)
				}
			}
			err := db.Exec("INSERT OR REPLACE INTO llm_providers SELECT * FROM llm_providers WHERE id = ?", provider.ID).Error
			if err == nil || !strings.Contains(err.Error(), "provider identity cannot be replaced") {
				t.Fatalf("provider replacement accepted or wrong guard: %v", err)
			}
			if err := db.Exec("UPDATE llm_providers SET id = ? WHERE id = ?", "renamed", provider.ID).Error; err == nil || !strings.Contains(err.Error(), "provider identity is immutable") {
				t.Fatalf("provider rename accepted or wrong guard: %v", err)
			}
			if err := db.Exec("UPDATE llm_providers SET id = id WHERE id = ?", provider.ID).Error; err != nil {
				t.Fatalf("unchanged provider identity blocked: %v", err)
			}
			var saved LLMProvider
			if err := db.First(&saved, "id = ?", provider.ID).Error; err != nil {
				t.Fatal(err)
			}
			if saved.CompatibilityRevision != 4 || saved.ConfigRevision != 8 {
				t.Fatalf("revisions changed: %+v", saved)
			}
			if withModel {
				resolution, err := repository.Resolve(ctx, model.ID, time.Now())
				if err != nil || resolution.Fields["chat"]["temperature"].State != "unsupported" {
					t.Fatalf("history lost: %+v %v", resolution, err)
				}
			}
			saved.Name = "Updated normally"
			if err := NewProviderRepository(db).SaveLLMProvider(ctx, &saved); err != nil {
				t.Fatalf("normal update blocked: %v", err)
			}
		})
	}
}

func TestMigration39CanonicalCatalogIsImmutableAndMigrationRemainsIdempotent(t *testing.T) {
	db := llmModelCapabilitiesTestDB(t)
	if err := db.Exec("PRAGMA recursive_triggers = OFF").Error; err != nil {
		t.Fatal(err)
	}
	for _, mutation := range []string{
		"UPDATE llm_capability_fields SET value_type = 'number' WHERE capability_key = 'tts' AND key = 'voice'",
		"UPDATE llm_capability_fields SET unit = 'other' WHERE capability_key = 'chat' AND key = 'max_output_tokens'",
		"DELETE FROM llm_capability_fields WHERE capability_key = 'tts' AND key = 'voice'",
		"DELETE FROM llm_capabilities WHERE key = 'music_generation'",
		"UPDATE llm_capabilities SET key = 'chat' WHERE key = 'music_generation'",
		"INSERT OR REPLACE INTO llm_capabilities (key) VALUES ('music_generation')",
		"INSERT OR REPLACE INTO llm_capability_fields (capability_key,key,value_type,unit) VALUES ('tts','voice','number','')",
	} {
		if err := db.Exec(mutation).Error; err == nil || !strings.Contains(err.Error(), "canonical model catalog is immutable") {
			t.Fatalf("canonical mutation accepted or wrong guard: %s %v", mutation, err)
		}
	}
	for range 2 {
		if err := MigrateLLMModelCapabilities(db); err != nil {
			t.Fatalf("idempotent seed blocked: %v", err)
		}
		if err := migrateLLMProviderAndCatalogGuards(db); err != nil {
			t.Fatal(err)
		}
	}
	if err := validateLLMCapabilityCatalog(db); err != nil {
		t.Fatalf("catalog changed: %v", err)
	}
}

func TestMigration39RefusesLegacyCatalogDivergenceBeforeInstallingGuards(t *testing.T) {
	db := llmModelCapabilitiesTestDB(t)
	if err := db.Exec("DROP TRIGGER trg_llm_capability_fields_canonical_update").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("UPDATE llm_capability_fields SET value_type = 'number' WHERE capability_key = 'tts' AND key = 'voice'").Error; err != nil {
		t.Fatal(err)
	}
	if err := migrateLLMProviderAndCatalogGuards(db); err == nil || !strings.Contains(err.Error(), "definição divergente") {
		t.Fatalf("corrupted legacy vocabulary accepted: %v", err)
	}
}
