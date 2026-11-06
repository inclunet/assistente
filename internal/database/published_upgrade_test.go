package database

import (
	"context"
	"gorm.io/gorm"
	"os"
	"reflect"
	"strings"
	"testing"
)

func TestPublishedDatabase019UpgradesDirectlyToLatest(t *testing.T) {
	for _, invalid := range []bool{false, true} {
		t.Run(map[bool]string{false: "valid", true: "retry_invalid_audio"}[invalid], func(t *testing.T) { testPublishedDatabase019Upgrade(t, invalid, false) })
	}
}
func TestPublishedDatabase019AdoptionBlocksAmbiguityUntilRepair(t *testing.T) {
	testPublishedDatabase019Upgrade(t, false, true)
}
func testPublishedDatabase019Upgrade(t *testing.T, invalid, ambiguous bool) {
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

	if err := database.Exec("UPDATE chat_messages SET audio='AQID',audio_mime_type='audio/wav' WHERE role='user'").Error; err != nil {
		t.Fatal(err)
	}
	if invalid {
		if err := database.Exec("UPDATE chat_messages SET audio='invalid!' WHERE role='user'").Error; err != nil {
			t.Fatal(err)
		}
	}

	if ambiguous {
		if err := database.Exec(`INSERT INTO chat_messages(id,conversation_id,parent_id,turn_id,role,content,tool_call_id) VALUES(44,7,42,41,'tool','resultado duplicado injetado pela fixture','fixture-call-019')`).Error; err != nil {
			t.Fatal(err)
		}
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
	if database.Migrator().HasTable("chat_turns") || database.Migrator().HasTable("chat_turn_attempts") {
		t.Fatal("turnos não podem preceder o corte físico do ledger")
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
	if ambiguous {
		err := AdoptLegacyData(owner.ID)
		if err == nil || !strings.Contains(err.Error(), "duplicate_tool_result") {
			t.Fatalf("sem diagnóstico de ambiguidade: %v", err)
		}
		applied, err := appliedMigrationVersions(database)
		if err != nil || applied[19] || applied[45] {
			t.Fatalf("corte liberado com ambiguidade: %v", err)
		}
		var count int64
		if err := database.Table("chat_messages").Where("role='tool'").Count(&count).Error; err != nil || count != 2 {
			t.Fatalf("perdeu resultados: %d %v", count, err)
		}
		// Apenas a fixture conhece qual resultado foi inserido artificialmente.
		// O produto nunca escolhe nem descarta resultados ambíguos automaticamente.
		if err := database.Exec("DELETE FROM chat_messages WHERE content=?", "resultado duplicado injetado pela fixture").Error; err != nil {
			t.Fatal(err)
		}
	}
	if invalid {
		if err := AdoptLegacyData(owner.ID); err == nil {
			t.Fatal("invalid migration released bootstrap")
		}
		applied, err := appliedMigrationVersions(database)
		if err != nil || applied[45] {
			t.Fatal("failed migration recorded", err)
		}
		var audio string
		if err := database.Raw("SELECT audio FROM chat_messages WHERE role='user' LIMIT 1").Scan(&audio).Error; err != nil || audio != "invalid!" {
			t.Fatal("failure lost legacy audio", err)
		}
		if err := database.Exec("UPDATE chat_messages SET audio='AQID' WHERE role='user'").Error; err != nil {
			t.Fatal(err)
		}
	}

	start := make(chan struct{})
	results := make(chan error, 4)
	for range 4 {
		go func() { <-start; results <- AdoptLegacyData(owner.ID) }()
	}
	close(start)
	for range 4 {
		if err := <-results; err != nil {
			t.Fatalf("adocoes concorrentes: %v", err)
		}
	}

	applied, err := appliedMigrationVersions(database)
	if err != nil || !applied[45] || !applied[41] || !database.Migrator().HasTable("chat_turns") {
		t.Fatalf("turnos não migrados após adoção: %v %v", applied[41], err)
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
	if err := LoadMessageAudios(WithUserID(context.Background(), owner.ID), database, messages, AudioAll); err != nil {
		t.Fatal(err)
	}
	if messages[0].Audio != "AQID" || messages[0].AudioOrigin != AttachmentLegacyUnknown {
		t.Fatal("upgrade perdeu audio", messages[0].AudioOrigin)
	}

	ctx := WithUserID(context.Background(), owner.ID)
	oldResults, err := SearchMessageContentWithContext(ctx, "mensagem", 10)
	if err != nil || len(oldResults) != 1 || oldResults[0].MessageID != messages[0].ID {
		t.Fatalf("busca antiga após adoção: %#v, %v", oldResults, err)
	}
	newMessage, err := CreateMessageWithContext(ctx, MessageOptions{ConversationID: conversation.ID, Role: "user", Content: "indexacaoposadocao"})
	if err != nil {
		t.Fatal(err)
	}
	newResults, err := SearchMessageContentWithContext(ctx, "indexacaoposadocao", 10)
	if err != nil || len(newResults) != 1 || newResults[0].MessageID != newMessage.ID {
		t.Fatalf("indexação após adoção: %#v, %v", newResults, err)
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
	if diagnostic.SchemaVersion != 20 || diagnostic.AppliedCount != len(schemaMigrations)-8 || !reflect.DeepEqual(diagnostic.PendingVersions, []int{21, 22, 23, 24, 27, 28, 29, 32}) {
		t.Fatalf("diagnóstico antes da composição do host: %#v", diagnostic)
	}
	// Este teste do registro valida o handshake explícito; DDL e preservação
	// dos dados de comandos são exercitados nos testes reais de commandbootstrap.
	for _, finish := range []func(context.Context, *gorm.DB, func(*gorm.DB) error) error{ApplyCommandStorageMigration, ApplyCommandEnvelopeMigration, ApplyCommandConfigMigration, ApplyCommandActivationMigration, ApplyCommandJobActivationMigration, ApplyCommandImportMigration, ApplyCommandInstanceMigration, ApplyCommandDecisionExternalContextMigration} {
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

func TestPublishedDatabase019AdoptionReportsOwnerlessJobRuns(t *testing.T) {
	previous := db
	database := newMigratorTestDB(t)
	db = database
	t.Cleanup(func() { db = previous })
	raw, err := os.ReadFile("testdata/published/0.1.9.sql")
	if err != nil {
		t.Fatal(err)
	}
	if err := database.Exec(string(raw)).Error; err != nil {
		t.Fatal(err)
	}
	if err := runMigrations(database, phasePreAutoMigrate); err != nil {
		t.Fatal(err)
	}
	fullAutoMigrate(t, database)
	for _, statement := range []string{
		"ALTER TABLE job_runs ADD COLUMN tool_name text",
		"ALTER TABLE job_runs ADD COLUMN inputs text",
		"ALTER TABLE job_runs ADD COLUMN output text",
		"INSERT INTO job_runs (id, user_id, job_id, trigger_id, status, queued_at, tool_name, inputs, output) VALUES ('ownerless-job-run', '', 'missing-job', '', 'completed', '2026-01-01', 'legacy-tool', '{}', 'private-output')",
	} {
		if err := database.Exec(statement).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := runMigrations(database, phasePostAutoMigrate); err != nil {
		t.Fatal(err)
	}
	owner := User{UUIDModel: UUIDModel{ID: "ownerless-diagnostic-owner"}, Username: "ownerless-diagnostic", PasswordHash: "fixture", Role: UserRoleAdmin, IsActive: true}
	if err := database.Create(&owner).Error; err != nil {
		t.Fatal(err)
	}
	err = AdoptLegacyData(owner.ID)
	if err == nil || !strings.Contains(err.Error(), "job_run:1") || strings.Contains(err.Error(), "private-output") {
		t.Fatalf("diagnóstico deve identificar recurso sem expor conteúdo: %v", err)
	}
	applied, err := appliedMigrationVersions(database)
	if err != nil || applied[19] || applied[45] {
		t.Fatalf("corte liberado sem dono: %v", err)
	}
	var row struct {
		UserID string
		Output string
	}
	if err := database.Table("job_runs").Select("user_id, output").Where("id = ?", "ownerless-job-run").Scan(&row).Error; err != nil {
		t.Fatal(err)
	}
	if row.UserID != "" || row.Output != "private-output" {
		t.Fatalf("execução órfã alterada: %+v", row)
	}
	var count int64
	if err := database.Model(&ToolLedgerMigrationState{}).Where("resource_id = ?", "ownerless-job-run").Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("fixture precisa cobrir recurso sem estado de ledger: %d %v", count, err)
	}
}
