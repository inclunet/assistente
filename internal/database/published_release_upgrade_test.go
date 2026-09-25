package database

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

const (
	publishedFixtureUserA = "018f0000-0000-7000-8000-000000000001"
	publishedFixtureUserB = "018f0000-0000-7000-8000-000000000002"
)

type publishedReleaseFixture struct {
	version          string
	hasReasoningMode bool
}

var publishedReleaseFixtures = []publishedReleaseFixture{
	{version: "0.2.0", hasReasoningMode: false},
	{version: "0.3.0", hasReasoningMode: true},
	{version: "0.4.0", hasReasoningMode: true},
	{version: "0.5.0", hasReasoningMode: true},
}

func loadPublishedReleaseFixture(t *testing.T, version string) *gorm.DB {
	t.Helper()
	database := newMigratorTestDB(t)
	raw, err := os.ReadFile(fmt.Sprintf("testdata/published/%s.sql", version))
	if err != nil {
		t.Fatal(err)
	}
	if err := database.Exec(string(raw)).Error; err != nil {
		t.Fatalf("carregar fixture %s: %v", version, err)
	}
	return database
}

func runCurrentUpgrade(t *testing.T, database *gorm.DB) {
	t.Helper()
	previous := db
	db = database
	defer func() { db = previous }()

	if err := runMigrations(database, phasePreAutoMigrate); err != nil {
		t.Fatalf("migrações pré-AutoMigrate: %v", err)
	}
	fullAutoMigrate(t, database)
	if err := runMigrations(database, phasePostAutoMigrate); err != nil {
		t.Fatalf("migrações pós-AutoMigrate: %v", err)
	}
}

func rowCount(t *testing.T, database *gorm.DB, table string) int {
	t.Helper()
	var count int
	if err := database.Raw(fmt.Sprintf("SELECT COUNT(*) FROM `%s`", table)).Scan(&count).Error; err != nil {
		t.Fatalf("contar %s: %v", table, err)
	}
	return count
}

func queryCount(t *testing.T, database *gorm.DB, query string, args ...any) int {
	t.Helper()
	var count int
	if err := database.Raw(query, args...).Scan(&count).Error; err != nil {
		t.Fatalf("consulta de contagem falhou: %v", err)
	}
	return count
}

func populatedTableCounts(t *testing.T, database *gorm.DB) map[string]int {
	t.Helper()
	tables := []string{
		"users", "sessions", "llm_providers", "conversations", "chat_messages",
		"memory_records", "credential_entries", "task_lists", "task_list_workflows",
		"tasks", "task_notes", "mcp_servers", "mcp_server_logs", "tool_catalog",
		"tool_invocations", "tags", "tag_assignments", "job_pipelines", "jobs",
		"job_triggers", "job_runs", "job_events", "job_run_events", "channels",
		"channel_contacts", "channel_contact_conversations", "channel_response_pending",
		"acp_sessions", "sub_agent_runs",
	}
	counts := make(map[string]int, len(tables))
	for _, table := range tables {
		counts[table] = rowCount(t, database, table)
	}
	return counts
}

func normalizeCreateTable(sql string) string {
	if !strings.HasPrefix(sql, "CREATE TABLE") || !strings.Contains(sql, ",CONSTRAINT ") {
		return sql
	}
	body := strings.TrimSuffix(sql, ")")
	parts := strings.Split(body, ",CONSTRAINT ")
	constraints := make([]string, 0, len(parts)-1)
	for _, constraint := range parts[1:] {
		constraints = append(constraints, "CONSTRAINT "+constraint)
	}
	sort.Strings(constraints)
	return parts[0] + "," + strings.Join(constraints, ",") + ")"
}

func publishedSchemaDefinition(t *testing.T, database *gorm.DB) []string {
	t.Helper()
	var rows []struct {
		Type string
		Name string
		SQL  string
	}
	if err := database.Raw(`
		SELECT type, name, sql
		  FROM sqlite_master
		 WHERE type IN ('table', 'index')
		   AND name NOT LIKE 'sqlite_%'
		   AND sql IS NOT NULL
		 ORDER BY type, name`).Scan(&rows).Error; err != nil {
		t.Fatal(err)
	}
	definition := make([]string, 0, len(rows))
	for _, row := range rows {
		definition = append(definition, row.Type+"\x00"+row.Name+"\x00"+normalizeCreateTable(row.SQL))
	}
	return definition
}

func TestPublishedReleaseFixtureSchemasAreTraceable(t *testing.T) {
	definitions := make(map[string][]string, len(publishedReleaseFixtures))
	for _, fixture := range publishedReleaseFixtures {
		database := loadPublishedReleaseFixture(t, fixture.version)

		if got := database.Migrator().HasColumn("llm_providers", "reasoning_content_mode"); got != fixture.hasReasoningMode {
			t.Fatalf("%s: reasoning_content_mode=%v, esperado %v", fixture.version, got, fixture.hasReasoningMode)
		}
		if database.Migrator().HasTable("skills") {
			t.Fatalf("%s: skills era filesystem e não deveria ter tabela SQLite", fixture.version)
		}
		if got := userVersion(t, database); got != 9 {
			t.Fatalf("%s: user_version publicado=%d, esperado 9", fixture.version, got)
		}
		definitions[fixture.version] = publishedSchemaDefinition(t, database)
	}

	if reflect.DeepEqual(definitions["0.2.0"], definitions["0.3.0"]) {
		t.Fatal("0.2.0 deveria diferir da 0.3.0 pela coluna reasoning_content_mode")
	}
	for _, version := range []string{"0.4.0", "0.5.0"} {
		if !reflect.DeepEqual(definitions["0.3.0"], definitions[version]) {
			t.Fatalf("schema %s deveria ser semanticamente equivalente ao 0.3.0", version)
		}
	}
}

func TestPublishedReleaseDatabasesUpgradeDirectlyAndIdempotently(t *testing.T) {
	expectedCounts := map[string]int{
		"users": 2, "sessions": 1, "llm_providers": 2, "conversations": 4,
		"chat_messages": 6, "memory_records": 2, "credential_entries": 2,
		"task_lists": 2, "task_list_workflows": 2, "tasks": 4, "task_notes": 2,
		"mcp_servers": 2, "mcp_server_logs": 2, "tool_catalog": 2,
		"tool_invocations": 2, "tags": 2, "tag_assignments": 2, "job_pipelines": 2,
		"jobs": 2, "job_triggers": 2, "job_runs": 2, "job_events": 2,
		"job_run_events": 2, "channels": 2, "channel_contacts": 2,
		"channel_contact_conversations": 2, "channel_response_pending": 2,
		"acp_sessions": 2, "sub_agent_runs": 2,
	}
	expectedAfterCounts := make(map[string]int, len(expectedCounts))
	for table, count := range expectedCounts {
		expectedAfterCounts[table] = count
	}
	expectedAfterCounts["tool_invocations"] = 4
	expectedAfterCounts["chat_messages"] = 4

	for _, fixture := range publishedReleaseFixtures {
		t.Run(fixture.version, func(t *testing.T) {
			database := loadPublishedReleaseFixture(t, fixture.version)
			if before := populatedTableCounts(t, database); !reflect.DeepEqual(before, expectedCounts) {
				t.Fatalf("fixture incompleta antes do upgrade:\nobtido: %#v\nesperado: %#v", before, expectedCounts)
			}

			runCurrentUpgrade(t, database)
			if !database.Migrator().HasTable(&JobProfileGrant{}) {
				t.Fatal("upgrade não criou job_profile_grants")
			}
			if !database.Migrator().HasTable(&JobProfileGrantEpoch{}) {
				t.Fatal("upgrade não criou job_profile_grant_epochs")
			}
			if !database.Migrator().HasTable(&ProfileGrantRevocationIntent{}) {
				t.Fatal("upgrade não criou profile_grant_revocation_intents")
			}
			if got := rowCount(t, database, "job_profile_grants"); got != 0 {
				t.Fatalf("upgrade não pode fabricar grants: %d", got)
			}
			if got := queryCount(t, database, `SELECT COUNT(*) FROM pragma_index_list('job_profile_grants') WHERE name IN ('ux_job_profile_grants_generation', 'ux_job_profile_grants_active') AND "unique" = 1`); got != 2 {
				t.Fatalf("índices de histórico e grant ativo ausentes: %d", got)
			}
			verifyPublishedFixtureData(t, database)
			afterFirstBoot := populatedTableCounts(t, database)
			if !reflect.DeepEqual(afterFirstBoot, expectedAfterCounts) {
				t.Fatalf("contagens mudaram no primeiro upgrade: %#v", afterFirstBoot)
			}
			if got := queryCount(t, database, `
				SELECT COUNT(*)
				  FROM tool_invocations
				 WHERE origin_type = 'chat'
				   AND conversation_id IS NOT NULL
				   AND turn_id IS NOT NULL
				   AND model_iteration = CASE
				         WHEN json_valid(metadata)
				          AND json_type(metadata, '$.display.iteration') IN ('integer', 'real')
				         THEN CAST(json_extract(metadata, '$.display.iteration') AS INTEGER)
				         ELSE 0
				       END
				   AND external = 0
				   AND input_hash <> ''
				   AND output_hash <> ''
				   AND migration_provenance = ?`, toolLedgerMigrationProvenance); got != 2 {
				t.Fatalf("backfill de chat publicado incompleto: %d", got)
			}
			if got := queryCount(t, database, `
				SELECT COUNT(*) FROM pragma_index_list('tool_invocations')
				 WHERE name IN (?, ?)`,
				toolModelCallsConversationIndex, toolModelCallsTurnIndex,
			); got != 2 {
				t.Fatalf("índices de model calls ausentes após upgrade publicado: %d", got)
			}
			if got := queryCount(t, database, `
				SELECT COUNT(*)
				  FROM tool_ledger_migration_states
				 WHERE state = 'canonical'
				   AND ambiguous_count = 0
				   AND last_error_code = ''
				   AND legacy_input_digest = ledger_input_digest
				   AND legacy_output_digest = ledger_output_digest`); got != 4 {
				t.Fatalf("estados de backfill publicado incompletos: %d", got)
			}

			diagnostic, err := buildUpgradeDiagnostic(database)
			if err != nil {
				t.Fatal(err)
			}
			if diagnostic.SchemaVersion != 20 || diagnostic.AppliedCount != len(schemaMigrations)-8 || !reflect.DeepEqual(diagnostic.PendingVersions, []int{21, 22, 23, 24, 27, 28, 29, 32}) {
				t.Fatalf("pendências antes da composição de comandos: %#v", diagnostic)
			}
			// O registro exige handshake explícito do host; o DDL real desses
			// callbacks é validado nos testes de commandbootstrap.
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

			runCurrentUpgrade(t, database)
			verifyPublishedFixtureData(t, database)
			afterSecondBoot := populatedTableCounts(t, database)
			if !reflect.DeepEqual(afterFirstBoot, afterSecondBoot) {
				t.Fatalf("segundo boot não foi idempotente:\nprimeiro: %#v\nsegundo: %#v", afterFirstBoot, afterSecondBoot)
			}
			if got := len(schemaMigrationRows(t, database)); got != len(schemaMigrations) {
				t.Fatalf("migrações duplicadas ou ausentes após segundo boot: %d", got)
			}
		})
	}
}

func TestPublishedReleaseCutoverCreatesRestorableBackup(t *testing.T) {
	path := filepath.Join(t.TempDir(), "assistente.db")
	database, err := gorm.Open(sqlite.Open(path), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		sqlDB, dbErr := database.DB()
		if dbErr == nil {
			_ = sqlDB.Close()
		}
	})
	raw, err := os.ReadFile("testdata/published/0.5.0.sql")
	if err != nil {
		t.Fatal(err)
	}
	if err := database.Exec(string(raw)).Error; err != nil {
		t.Fatal(err)
	}

	var foreignKeysBefore int
	if err := database.Raw(`PRAGMA foreign_keys`).Scan(&foreignKeysBefore).Error; err != nil {
		t.Fatal(err)
	}
	runCurrentUpgrade(t, database)
	var foreignKeysAfter int
	if err := database.Raw(`PRAGMA foreign_keys`).Scan(&foreignKeysAfter).Error; err != nil {
		t.Fatal(err)
	}
	if foreignKeysAfter != foreignKeysBefore {
		t.Fatalf("cutover alterou foreign_keys: antes=%d depois=%d", foreignKeysBefore, foreignKeysAfter)
	}
	backupPath := path + ".pre-tool-ledger-v19.bak"
	digest, size, err := fileSHA256(backupPath)
	if err != nil {
		t.Fatalf("backup ausente: %v", err)
	}
	manifest, err := os.ReadFile(backupPath + ".manifest")
	if err != nil {
		t.Fatalf("manifesto ausente: %v", err)
	}
	if size == 0 || !strings.Contains(string(manifest), "sha256="+digest) {
		t.Fatalf("backup sem checksum verificável: bytes=%d manifesto=%q", size, manifest)
	}

	backup, err := gorm.Open(sqlite.Open(backupPath), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		sqlDB, dbErr := backup.DB()
		if dbErr == nil {
			_ = sqlDB.Close()
		}
	})
	if !backup.Migrator().HasColumn("chat_messages", "tool_calls") ||
		!backup.Migrator().HasColumn("job_runs", "output") {
		t.Fatal("backup não preservou o schema restaurável anterior ao cutover")
	}
	if database.Migrator().HasColumn("chat_messages", "tool_calls") ||
		database.Migrator().HasColumn("chat_messages", "tool_call_id") ||
		database.Migrator().HasColumn("job_runs", "tool_name") ||
		database.Migrator().HasColumn("job_runs", "inputs") ||
		database.Migrator().HasColumn("job_runs", "output") {
		t.Fatal("schema canônico reteve colunas técnicas legadas")
	}
	if got := queryCount(t, database, "SELECT COUNT(*) FROM chat_messages WHERE lower(trim(role)) = 'tool'"); got != 0 {
		t.Fatalf("schema canônico reteve %d mensagens role=tool", got)
	}
	if err := database.Exec(`
		UPDATE chat_messages SET role = 'tool'
		WHERE id = (SELECT id FROM chat_messages LIMIT 1)`).Error; err == nil {
		t.Fatal("constraint canônica permitiu persistir role=tool")
	}
	if got := queryCount(t, database, "SELECT COUNT(*) FROM chat_messages WHERE lower(trim(role)) = 'tool'"); got != 0 {
		t.Fatalf("tentativa rejeitada deixou %d mensagens role=tool", got)
	}
}

func TestArchivePreviousToolLedgerBackupPreservesDatabaseAndManifest(t *testing.T) {
	backupPath := filepath.Join(t.TempDir(), "assistente.db.pre-tool-ledger-v19.bak")
	if err := os.WriteFile(backupPath, []byte("backup-anterior"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(backupPath+".manifest", []byte("manifesto-anterior"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := archivePreviousToolLedgerBackup(backupPath); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(backupPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("backup fixo não foi rotacionado: %v", err)
	}
	archived, err := filepath.Glob(backupPath + ".previous-*")
	if err != nil {
		t.Fatal(err)
	}
	var archivedBackup string
	for _, path := range archived {
		if !strings.HasSuffix(path, ".manifest") {
			archivedBackup = path
			break
		}
	}
	if archivedBackup == "" {
		t.Fatalf("backup anterior não foi preservado: %v", archived)
	}
	if content, err := os.ReadFile(archivedBackup); err != nil || string(content) != "backup-anterior" {
		t.Fatalf("conteúdo do backup arquivado mudou: conteúdo=%q erro=%v", content, err)
	}
	if content, err := os.ReadFile(archivedBackup + ".manifest"); err != nil || string(content) != "manifesto-anterior" {
		t.Fatalf("manifesto arquivado mudou: conteúdo=%q erro=%v", content, err)
	}
}

func TestPublishedReleaseUpgradeDisablesLegacySubagentJobsWithoutGrants(t *testing.T) {
	database := loadPublishedReleaseFixture(t, "0.5.0")
	if err := database.Exec(`UPDATE jobs SET tool_name = 'subagent', inputs = '{"profile":"pesquisa","prompt":"x"}', enabled = 1`).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.Exec(`UPDATE tool_catalog SET name = ' subagent ' WHERE id = '018f0000-0000-7000-8000-000000000032'`).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.Exec(`UPDATE jobs SET tool_name = '' WHERE id = '018f0000-0000-7000-8000-000000000041'`).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.Exec(`UPDATE jobs SET tool_name = ' subagent ' WHERE id = '018f0000-0000-7000-8000-000000000141'`).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.Exec(`
		INSERT INTO jobs (id, created_at, updated_at, user_id, slug, name, enabled, tool_catalog_id, tool_name, inputs)
		VALUES ('018f0000-0000-7000-8000-000000000999', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP,
		        ?, 'herda-profile', 'Herda profile', 1,
		        '018f0000-0000-7000-8000-000000000032', 'subagent', '{"prompt":"x"}')`,
		publishedFixtureUserA).Error; err != nil {
		t.Fatal(err)
	}
	runCurrentUpgrade(t, database)
	if got := queryCount(t, database, `SELECT COUNT(*) FROM jobs WHERE json_type(inputs, '$.profile') = 'text' AND enabled = 1`); got != 0 {
		t.Fatalf("upgrade deixou %d job(s) subagent legado(s) grantável(is) habilitado(s)", got)
	}
	if got := queryCount(t, database, `SELECT COUNT(*) FROM jobs WHERE slug = 'herda-profile' AND enabled = 1`); got != 1 {
		t.Fatalf("upgrade desabilitou job que herda profile: %d", got)
	}
}

func verifyPublishedFixtureData(t *testing.T, database *gorm.DB) {
	t.Helper()

	if got := queryCount(t, database, `
		SELECT COUNT(*)
		  FROM chat_messages child
		  JOIN chat_messages parent ON parent.id = child.parent_id
		 WHERE child.role = 'assistant'
		   AND child.turn_id = parent.id
		   AND child.conversation_id = parent.conversation_id`); got != 2 {
		t.Fatalf("hierarquia de mensagens não preservada: %d", got)
	}
	if got := queryCount(t, database, `
		SELECT COUNT(*)
		  FROM tasks child
		  JOIN tasks parent ON parent.id = child.parent_id
		 WHERE child.task_list_id = parent.task_list_id`); got != 2 {
		t.Fatalf("hierarquia de tarefas não preservada: %d", got)
	}
	if got := queryCount(t, database, `
		SELECT COUNT(*)
		  FROM channel_contact_conversations mapping
		  JOIN channels channel ON channel.id = mapping.channel_id
		  JOIN channel_contacts contact
		    ON contact.channel_id = channel.id
		   AND contact.external_id = mapping.contact_external_id
		  JOIN conversations conversation ON conversation.id = mapping.conversation_id
		 WHERE channel.user_id = contact.user_id
		   AND channel.user_id = conversation.user_id`); got != 2 {
		t.Fatalf("relações de canais não preservadas: %d", got)
	}
	if got := queryCount(t, database, `
		SELECT COUNT(*)
		  FROM jobs job
		  JOIN job_pipelines pipeline ON pipeline.id = job.pipeline_id
		  JOIN tool_catalog tool ON tool.id = job.tool_catalog_id
		  JOIN job_triggers trigger ON trigger.job_id = job.id
		  JOIN job_runs run ON run.job_id = job.id AND run.trigger_id = trigger.id
		 WHERE job.user_id = pipeline.user_id
		   AND job.user_id = tool.user_id
		   AND job.user_id = trigger.user_id
		   AND job.user_id = run.user_id`); got != 2 {
		t.Fatalf("relações de jobs não preservadas: %d", got)
	}
	if got := queryCount(t, database, `
		SELECT COUNT(*)
		  FROM tool_invocations invocation
		  JOIN tool_catalog tool ON tool.id = invocation.tool_catalog_id
		  JOIN mcp_servers server ON server.id = tool.mcp_server_id
		 WHERE invocation.user_id = tool.user_id
		   AND invocation.user_id = server.user_id`); got != 4 {
		t.Fatalf("relações MCP/tool não preservadas: %d", got)
	}

	userScoped := map[string]int{
		"llm_providers": 1, "conversations": 2, "memory_records": 1,
		"credential_entries": 1, "task_lists": 1, "task_notes": 1,
		"mcp_servers": 1, "tool_catalog": 1, "tool_invocations": 2,
		"tags": 1, "tag_assignments": 1, "job_pipelines": 1, "jobs": 1,
		"job_triggers": 1, "job_runs": 1, "job_events": 1, "job_run_events": 1,
		"channels": 1, "channel_contacts": 1, "acp_sessions": 1, "sub_agent_runs": 1,
	}
	for table, expected := range userScoped {
		for _, userID := range []string{publishedFixtureUserA, publishedFixtureUserB} {
			query := fmt.Sprintf("SELECT COUNT(*) FROM `%s` WHERE user_id = ?", table)
			if got := queryCount(t, database, query, userID); got != expected {
				t.Fatalf("%s: user_id %s retornou %d linhas, esperado %d", table, userID, got, expected)
			}
		}
	}
	if got := queryCount(t, database, `
		SELECT COUNT(*) FROM credential_entries
		 WHERE COALESCE(token_enc, '') = ''
		   AND COALESCE(password_enc, '') = ''
		   AND COALESCE(headers_enc, '') = ''
		   AND COALESCE(refresh_token_enc, '') = ''
		   AND COALESCE(client_id_enc, '') = ''
		   AND COALESCE(client_secret_enc, '') = ''`); got != 2 {
		t.Fatalf("fixtures devem permanecer sem segredos: %d", got)
	}
	if got := queryCount(t, database, `
		SELECT COUNT(*) FROM (
			SELECT note.id
			  FROM task_notes note
			  JOIN tasks task ON task.id = note.task_id
			  JOIN task_lists list ON list.id = task.task_list_id
			 WHERE note.user_id <> list.user_id
			UNION ALL
			SELECT assignment.id
			  FROM tag_assignments assignment
			  JOIN tags tag ON tag.id = assignment.tag_id
			 WHERE assignment.user_id <> tag.user_id
			UNION ALL
			SELECT pending.conversation_id
			  FROM channel_response_pending pending
			  JOIN conversations conversation ON conversation.id = pending.conversation_id
			 WHERE pending.owner_user_id <> conversation.user_id
			UNION ALL
			SELECT run.id
			  FROM sub_agent_runs run
			  JOIN conversations parent ON parent.id = run.parent_conversation_id
			  JOIN conversations child ON child.id = run.child_conversation_id
			 WHERE run.user_id <> parent.user_id OR run.user_id <> child.user_id
		)`); got != 0 {
		t.Fatalf("relações cruzaram o isolamento entre pessoas: %d", got)
	}
	if got := queryCount(t, database, "SELECT COUNT(*) FROM pragma_foreign_key_check"); got != 0 {
		t.Fatalf("upgrade deixou violações de chave estrangeira: %d", got)
	}
	var integrity string
	if err := database.Raw("PRAGMA integrity_check").Scan(&integrity).Error; err != nil {
		t.Fatal(err)
	}
	if integrity != "ok" {
		t.Fatalf("integrity_check=%q", integrity)
	}
	if database.Migrator().HasTable("skills") {
		t.Fatal("upgrade não deveria inventar persistência SQLite para skills")
	}
}
