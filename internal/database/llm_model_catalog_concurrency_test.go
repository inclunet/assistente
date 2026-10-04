package database

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"assistente/internal/llmcapabilities"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestLLMModelCatalogConcurrentDiscoveryIsIdempotent(t *testing.T) {
	for _, operation := range []string{"model", "binding"} {
		t.Run(operation, func(t *testing.T) {
			db, err := gorm.Open(sqlite.Open(sqliteDSN(filepath.Join(t.TempDir(), "catalog.db"))), &gorm.Config{})
			if err != nil {
				t.Fatal(err)
			}
			sqlDB, err := db.DB()
			if err != nil {
				t.Fatal(err)
			}
			defer func() {
				if err := sqlDB.Close(); err != nil {
					t.Errorf("fechar banco: %v", err)
				}
			}()
			configureSQLitePool(sqlDB)
			if err := db.Exec("PRAGMA journal_mode=WAL").Error; err != nil {
				t.Fatal(err)
			}
			if err := db.AutoMigrate(&LLMProvider{}, &CredentialEntry{}); err != nil {
				t.Fatal(err)
			}
			if err := MigrateLLMModelCapabilities(db); err != nil {
				t.Fatal(err)
			}
			if err := db.Create(&LLMProvider{ID: "provider", UserID: "owner", Name: "Original", Type: "custom", APIFormat: "openai", BaseURL: "https://provider.example/v1"}).Error; err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(WithUserID(context.Background(), "owner"), 10*time.Second)
			defer cancel()
			repository := NewLLMModelCapabilitiesRepository(db)
			var modelID string
			if operation == "binding" {
				model, err := repository.SaveModel(ctx, "provider", "remote", "Remote")
				if err != nil {
					t.Fatal(err)
				}
				modelID = model.ID
			}
			writer, err := sqlDB.Conn(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer func() {
				if err := writer.Close(); err != nil {
					t.Errorf("fechar conexão writer: %v", err)
				}
			}()
			if _, err := writer.ExecContext(ctx, "BEGIN IMMEDIATE"); err != nil {
				t.Fatal(err)
			}
			committed := false
			defer func() {
				if !committed {
					if _, err := writer.ExecContext(context.Background(), "ROLLBACK"); err != nil {
						t.Errorf("rollback do writer: %v", err)
					}
				}
			}()
			if _, err := writer.ExecContext(ctx, "UPDATE llm_providers SET name = 'Atualizado' WHERE id = 'provider'"); err != nil {
				t.Fatal(err)
			}
			contended := make(chan struct{}, 2)
			released := make(chan struct{})
			notify := func() {
				select {
				case contended <- struct{}{}:
				default:
				}
			}
			// A leitura com transação diferida captura um snapshot anterior ao
			// commit do writer. BEGIN IMMEDIATE aguarda antes de qualquer leitura.
			if err := db.Callback().Query().After("gorm:query").Register("test:catalog_snapshot", func(tx *gorm.DB) {
				notify()
				select {
				case <-released:
				case <-ctx.Done():
				}
			}); err != nil {
				t.Fatal(err)
			}
			if err := db.Callback().Raw().After("gorm:raw").Register("test:catalog_writer", func(tx *gorm.DB) {
				if IsSQLiteBusyError(tx.Error) {
					notify()
				}
			}); err != nil {
				t.Fatal(err)
			}
			type result struct {
				id  string
				err error
			}
			results := make(chan result, 2)
			observedAt := time.Now().UTC().Add(-time.Second).Truncate(time.Microsecond)
			for i := 0; i < 2; i++ {
				go func() {
					if operation == "model" {
						model, err := repository.SaveModel(ctx, "provider", "remote", "Remote")
						id := ""
						if model != nil {
							id = model.ID
						}
						results <- result{id: id, err: err}
						return
					}
					binding := &LLMModelCatalogBinding{ModelID: modelID, ProviderCompatibilityRevision: 1, Source: string(llmcapabilities.SourceOfficialCatalog), ExternalProviderID: "vendor", ExternalModelID: "remote", VerifiedAt: observedAt}
					err := repository.BindCatalogModel(ctx, binding)
					results <- result{id: binding.ID, err: err}
				}()
			}
			select {
			case <-contended:
			case <-ctx.Done():
				close(released)
				t.Fatal("descobertas não alcançaram a contenção", ctx.Err())
			}
			_, commitErr := writer.ExecContext(ctx, "COMMIT")
			close(released)
			if commitErr != nil {
				t.Fatal(commitErr)
			}
			committed = true
			first, second := <-results, <-results
			if first.err != nil || second.err != nil {
				t.Fatalf("descobertas concorrentes falharam: %v / %v", first.err, second.err)
			}
			if first.id == "" || first.id != second.id {
				t.Fatalf("identidades divergentes: %q / %q", first.id, second.id)
			}
			table := "llm_models"
			if operation == "binding" {
				table = "llm_model_catalog_bindings"
			}
			var count int64
			if err := db.Table(table).Count(&count).Error; err != nil || count != 1 {
				t.Fatalf("registros duplicados: count=%d err=%v", count, err)
			}
		})
	}
}
