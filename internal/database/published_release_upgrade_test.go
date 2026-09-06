package database

import (
	"fmt"
	"os"
	"reflect"
	"sort"
	"strings"
	"testing"

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
		"chat_messages": 4, "memory_records": 2, "credential_entries": 2,
		"task_lists": 2, "task_list_workflows": 2, "tasks": 4, "task_notes": 2,
		"mcp_servers": 2, "mcp_server_logs": 2, "tool_catalog": 2,
		"tool_invocations": 2, "tags": 2, "tag_assignments": 2, "job_pipelines": 2,
		"jobs": 2, "job_triggers": 2, "job_runs": 2, "job_events": 2,
		"job_run_events": 2, "channels": 2, "channel_contacts": 2,
		"channel_contact_conversations": 2, "channel_response_pending": 2,
		"acp_sessions": 2, "sub_agent_runs": 2,
	}

	for _, fixture := range publishedReleaseFixtures {
		t.Run(fixture.version, func(t *testing.T) {
			database := loadPublishedReleaseFixture(t, fixture.version)
			if before := populatedTableCounts(t, database); !reflect.DeepEqual(before, expectedCounts) {
				t.Fatalf("fixture incompleta antes do upgrade:\nobtido: %#v\nesperado: %#v", before, expectedCounts)
			}

			runCurrentUpgrade(t, database)
			verifyPublishedFixtureData(t, database)
			afterFirstBoot := populatedTableCounts(t, database)
			if !reflect.DeepEqual(afterFirstBoot, expectedCounts) {
				t.Fatalf("contagens mudaram no primeiro upgrade: %#v", afterFirstBoot)
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
		   AND invocation.user_id = server.user_id`); got != 2 {
		t.Fatalf("relações MCP/tool não preservadas: %d", got)
	}

	userScoped := map[string]int{
		"llm_providers": 1, "conversations": 2, "memory_records": 1,
		"credential_entries": 1, "task_lists": 1, "task_notes": 1,
		"mcp_servers": 1, "tool_catalog": 1, "tool_invocations": 1,
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
	if database.Migrator().HasTable("skills") {
		t.Fatal("upgrade não deveria inventar persistência SQLite para skills")
	}
}
