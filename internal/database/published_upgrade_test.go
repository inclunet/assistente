package database

import (
	"context"
	"gorm.io/gorm"
	"os"
	"reflect"
	"testing"
)

func TestPublishedDatabase019UpgradesDirectlyToLatest(t *testing.T) {
	previous := db
	database := newMigratorTestDB(t)
	db = database
	t.Cleanup(func() { db = previous })

	raw, err := os.ReadFile("testdata/published/0.1.9.sql")
	if err != nil {
		t.Fatal(err)
	}
	if err := database.Exec(string(raw)).Error; err != nil {
		t.Fatalf("carregar fixture 0.1.9: %v", err)
	}

	if err := runMigrations(database, phasePreAutoMigrate); err != nil {
		t.Fatalf("migrações pré-AutoMigrate: %v", err)
	}
	fullAutoMigrate(t, database)
	if err := runMigrations(database, phasePostAutoMigrate); err != nil {
		t.Fatalf("migrações pós-AutoMigrate: %v", err)
	}
	if userVersion(t, database) != 17 {
		t.Fatalf("backfill sem owner deveria permanecer pendente, schema=%d", userVersion(t, database))
	}
	owner := User{
		UUIDModel:    UUIDModel{ID: "018f0000-0000-7000-8000-000000009019"},
		Username:     "fixture-019-owner",
		PasswordHash: "fixture-sem-segredo",
		Role:         UserRoleAdmin,
		IsActive:     true,
	}
	if err := database.Create(&owner).Error; err != nil {
		t.Fatal(err)
	}
	if err := AdoptLegacyData(owner.ID); err != nil {
		t.Fatalf("adotar owner publicado: %v", err)
	}
	if err := runMigrations(database, phasePostAutoMigrate); err != nil {
		t.Fatalf("retomar backfill após adoção: %v", err)
	}

	var conversation Conversation
	if err := database.Where("title = ?", "Fixture 0.1.9 sem PII").First(&conversation).Error; err != nil {
		t.Fatalf("conversa não preservada: %v", err)
	}
	if len(conversation.ID) != 36 {
		t.Fatalf("ID não convertido para UUID: %q", conversation.ID)
	}
	var messages []ChatMessage
	if err := database.Where("conversation_id = ?", conversation.ID).Order("created_at").Find(&messages).Error; err != nil {
		t.Fatal(err)
	}
	if len(messages) != 2 ||
		messages[1].ParentID == nil || *messages[1].ParentID != messages[0].ID {
		t.Fatalf("hierarquia não preservada: %#v", messages)
	}
	var invocation ToolInvocation
	if err := database.Where("user_id = ? AND origin_type = ? AND tool_call_id = ?", owner.ID, "chat", "fixture-call-019").First(&invocation).Error; err != nil {
		t.Fatalf("invocação legada não migrada: %v", err)
	}
	if invocation.ConversationID == nil || *invocation.ConversationID != conversation.ID ||
		invocation.InputHash == "" || invocation.OutputHash == "" ||
		invocation.MigrationProvenance != toolLedgerMigrationProvenance {
		t.Fatalf("invocação legada incompleta: %+v", invocation)
	}
	var archival ToolCatalog
	if err := database.Where("id = ?", invocation.ToolCatalogID).First(&archival).Error; err != nil {
		t.Fatal(err)
	}
	if archival.Origin != toolLedgerArchiveOrigin ||
		archival.AvailabilityStatus != toolLedgerArchiveUnavailable ||
		archival.UserID == nil || *archival.UserID != owner.ID {
		t.Fatalf("catálogo archival não ficou isolado/indisponível: %+v", archival)
	}

	diagnostic, err := buildUpgradeDiagnostic(database)
	if err != nil {
		t.Fatal(err)
	}
	// O banco publicado termina a fase database, mas não pode inventar a
	// composição do host de comandos. As migrações 24/25 já rodaram e o
	// watermark deve permanecer em 19 enquanto 20–23 estiverem pendentes.
	if diagnostic.SchemaVersion != 20 || diagnostic.AppliedCount != len(schemaMigrations)-6 || !reflect.DeepEqual(diagnostic.PendingVersions, []int{21, 22, 23, 24, 27, 28}) {
		t.Fatalf("diagnóstico antes da composição do host: %#v", diagnostic)
	}
	// Este teste do registro valida o handshake explícito; DDL e preservação
	// dos dados de comandos são exercitados nos testes reais de commandbootstrap.
	for _, finish := range []func(context.Context, *gorm.DB, func(*gorm.DB) error) error{ApplyCommandStorageMigration, ApplyCommandEnvelopeMigration, ApplyCommandConfigMigration, ApplyCommandActivationMigration, ApplyCommandJobActivationMigration, ApplyCommandImportMigration} {
		if err := finish(context.Background(), database, func(*gorm.DB) error { return nil }); err != nil {
			t.Fatal(err)
		}
	}
	diagnostic, err = buildUpgradeDiagnostic(database)
	if err != nil {
		t.Fatal(err)
	}
	if diagnostic.SchemaVersion != diagnostic.LatestVersion ||
		diagnostic.AppliedCount != len(schemaMigrations) ||
		len(diagnostic.PendingVersions) != 0 {
		t.Fatalf("diagnóstico após upgrade: %#v", diagnostic)
	}
}

func TestUpgradeDiagnosticDoesNotExposePersistedValues(t *testing.T) {
	database := newMigratorTestDB(t)
	if err := ensureSchemaMigrationsTable(database); err != nil {
		t.Fatal(err)
	}
	if err := recordMigration(database, schemaMigrations[0]); err != nil {
		t.Fatal(err)
	}

	diagnostic, err := buildUpgradeDiagnostic(database)
	if err != nil {
		t.Fatal(err)
	}
	if diagnostic.AppliedCount != 1 || diagnostic.SchemaVersion != 1 {
		t.Fatalf("diagnóstico inesperado: %#v", diagnostic)
	}
	if len(diagnostic.PendingVersions) != len(schemaMigrations)-1 {
		t.Fatalf("pendências inesperadas: %#v", diagnostic)
	}
}

func TestUpgradeDiagnosticReadsLegacyUserVersionWithoutRegistry(t *testing.T) {
	database := newMigratorTestDB(t)
	if err := database.Exec("PRAGMA user_version = 7").Error; err != nil {
		t.Fatal(err)
	}

	diagnostic, err := buildUpgradeDiagnostic(database)
	if err != nil {
		t.Fatal(err)
	}
	if diagnostic.SchemaVersion != 7 {
		t.Fatalf("schema legado = %d, esperado 7", diagnostic.SchemaVersion)
	}
	if diagnostic.AppliedCount != 0 ||
		len(diagnostic.PendingVersions) != len(schemaMigrations) {
		t.Fatalf("diagnóstico legado inesperado: %#v", diagnostic)
	}
}
