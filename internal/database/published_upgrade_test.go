package database

import (
	"os"
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
	if len(messages) != 2 || messages[1].ParentID == nil || *messages[1].ParentID != messages[0].ID {
		t.Fatalf("hierarquia não preservada: %#v", messages)
	}

	diagnostic, err := buildUpgradeDiagnostic(database)
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
