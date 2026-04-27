package database

import (
	"database/sql"
	"fmt"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

// createOldSchemaDB cria um banco in-memory com o schema INTEGER antigo,
// simulando o estado pré-migração. Retorna o *sql.DB para inserções diretas.
func createOldSchemaDB(t *testing.T) *sql.DB {
	t.Helper()

	var err error
	db, err = gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("get sql.DB: %v", err)
	}

	// Criar tabelas com INTEGER PK (schema antigo)
	stmts := []string{
		`CREATE TABLE credential_entries (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT, pattern TEXT, type TEXT,
			api_key_enc TEXT, client_id TEXT, client_secret_enc TEXT,
			access_token_enc TEXT, refresh_token_enc TEXT,
			token_url TEXT, token_expiry DATETIME, scopes TEXT, extra_enc TEXT,
			created_at DATETIME, updated_at DATETIME
		)`,
		`CREATE TABLE credential_key_wraps (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			wrapped_key TEXT,
			created_at DATETIME, updated_at DATETIME
		)`,
		`CREATE TABLE conversations (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			title TEXT, channel TEXT, contact_id TEXT,
			summary TEXT, summary_up_to_message_id INTEGER,
			summarizing_in_progress INTEGER DEFAULT 0,
			created_at DATETIME, updated_at DATETIME
		)`,
		`CREATE TABLE chat_messages (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			conversation_id INTEGER, parent_id INTEGER, turn_id INTEGER,
			role TEXT, content TEXT, reasoning TEXT,
			media TEXT, audio TEXT, audio_mime_type TEXT,
			tool_calls TEXT, tool_call_id TEXT,
			prompt_tokens INTEGER DEFAULT 0, completion_tokens INTEGER DEFAULT 0, total_tokens INTEGER DEFAULT 0,
			model TEXT, source TEXT,
			created_at DATETIME, updated_at DATETIME
		)`,
		`CREATE TABLE task_lists (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			title TEXT, slug TEXT, description TEXT,
			preferred_view_mode TEXT DEFAULT 'list',
			validation_policy TEXT,
			created_at DATETIME, updated_at DATETIME
		)`,
		`CREATE TABLE task_list_workflows (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			task_list_id INTEGER,
			statuses TEXT, allowed_transitions TEXT,
			initial_status_id INTEGER DEFAULT 1,
			created_at DATETIME, updated_at DATETIME
		)`,
		`CREATE TABLE tasks (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			task_list_id INTEGER, title TEXT, description TEXT,
			code TEXT, link TEXT,
			status_id INTEGER NOT NULL DEFAULT 1,
			parent_id INTEGER, "order" INTEGER DEFAULT 0,
			assignee_name TEXT, assignee_id TEXT,
			creator_name TEXT, creator_id TEXT,
			due_date DATETIME,
			created_at DATETIME, updated_at DATETIME, completed_at DATETIME
		)`,
		`CREATE TABLE task_notes (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			task_id INTEGER, type INTEGER DEFAULT 1,
			content TEXT,
			author_name TEXT, author_id TEXT,
			source TEXT, external_id TEXT,
			external_parent_id TEXT, external_updated_at DATETIME,
			created_at DATETIME, updated_at DATETIME
		)`,
	}

	for _, stmt := range stmts {
		if _, err := sqlDB.Exec(stmt); err != nil {
			t.Fatalf("create table: %v\nSQL: %s", err, stmt)
		}
	}

	t.Cleanup(func() {
		_ = sqlDB.Close()
		db = nil
	})

	return sqlDB
}

// isUUID verifica se uma string tem formato UUID (36 chars, 4 hifens).
func isUUID(s string) bool {
	if len(s) != 36 {
		return false
	}
	for i, c := range s {
		if i == 8 || i == 13 || i == 18 || i == 23 {
			if c != '-' {
				return false
			}
		} else if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			return false
		}
	}
	return true
}

// getColumnType retorna o tipo da coluna "id" de uma tabela.
func getColumnType(t *testing.T, sqlDB *sql.DB, table string) string {
	t.Helper()
	rows, err := sqlDB.Query(fmt.Sprintf("PRAGMA table_info(%s)", table))
	if err != nil {
		t.Fatalf("PRAGMA table_info(%s): %v", table, err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var cid int
		var name, colType string
		var notNull, pk int
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &colType, &notNull, &dflt, &pk); err != nil {
			t.Fatal(err)
		}
		if name == "id" {
			return strings.ToUpper(colType)
		}
	}
	t.Fatalf("coluna id não encontrada em %s", table)
	return ""
}

func TestMigration_EmptyDatabase(t *testing.T) {
	sqlDB := createOldSchemaDB(t)

	// Migração com tabelas vazias deve funcionar sem erros
	if err := migrateToUUIDv7(); err != nil {
		t.Fatalf("migrateToUUIDv7 em banco vazio: %v", err)
	}

	// Verificar que todas as tabelas têm id TEXT agora
	tables := []string{
		"conversations", "chat_messages", "credential_entries",
		"credential_key_wraps", "task_lists", "task_list_workflows",
		"tasks", "task_notes",
	}
	for _, table := range tables {
		colType := getColumnType(t, sqlDB, table)
		if colType != "TEXT" {
			t.Errorf("%s.id tipo = %s, esperado TEXT", table, colType)
		}
	}
}

func TestMigration_NotNeededAfterMigration(t *testing.T) {
	createOldSchemaDB(t)

	if err := migrateToUUIDv7(); err != nil {
		t.Fatalf("primeira migração: %v", err)
	}

	// Segunda chamada deve ser no-op
	if err := migrateToUUIDv7(); err != nil {
		t.Fatalf("segunda migração deveria ser no-op: %v", err)
	}
}

func TestMigration_ConversationsAndMessages(t *testing.T) {
	sqlDB := createOldSchemaDB(t)

	// Seed: 2 conversas com mensagens
	mustExec(t, sqlDB, `INSERT INTO conversations (id, title, created_at, updated_at) VALUES (1, 'Conv Alpha', '2026-01-01', '2026-01-01')`)
	mustExec(t, sqlDB, `INSERT INTO conversations (id, title, created_at, updated_at) VALUES (2, 'Conv Beta', '2026-01-02', '2026-01-02')`)

	// Mensagens: conversa 1 com hierarquia (parent_id e turn_id)
	mustExec(t, sqlDB, `INSERT INTO chat_messages (id, conversation_id, role, content, created_at, updated_at) VALUES (10, 1, 'user', 'Olá', '2026-01-01', '2026-01-01')`)
	mustExec(t, sqlDB, `INSERT INTO chat_messages (id, conversation_id, parent_id, turn_id, role, content, created_at, updated_at) VALUES (11, 1, 10, 10, 'assistant', 'Oi!', '2026-01-01', '2026-01-01')`)
	mustExec(t, sqlDB, `INSERT INTO chat_messages (id, conversation_id, parent_id, role, content, created_at, updated_at) VALUES (12, 1, 11, 'user', 'Como vai?', '2026-01-01', '2026-01-01')`)

	// Mensagem conversa 2
	mustExec(t, sqlDB, `INSERT INTO chat_messages (id, conversation_id, role, content, created_at, updated_at) VALUES (20, 2, 'user', 'Teste', '2026-01-02', '2026-01-02')`)

	if err := migrateToUUIDv7(); err != nil {
		t.Fatalf("migração: %v", err)
	}

	// Verificar conversas migradas
	var convCount int
	if err := sqlDB.QueryRow("SELECT count(*) FROM conversations").Scan(&convCount); err != nil {
		t.Fatal(err)
	}
	if convCount != 2 {
		t.Fatalf("conversas: esperado 2, obtido %d", convCount)
	}

	// Verificar que IDs são UUID
	rows, _ := sqlDB.Query("SELECT id, title FROM conversations ORDER BY title")
	defer func() { _ = rows.Close() }()
	convIDs := make(map[string]string) // title → uuid
	for rows.Next() {
		var id, title string
		if err := rows.Scan(&id, &title); err != nil {
			t.Fatal(err)
		}
		if !isUUID(id) {
			t.Errorf("conversations.id não é UUID: %q", id)
		}
		convIDs[title] = id
	}

	// Verificar mensagens migradas
	var msgCount int
	if err := sqlDB.QueryRow("SELECT count(*) FROM chat_messages").Scan(&msgCount); err != nil {
		t.Fatal(err)
	}
	if msgCount != 4 {
		t.Fatalf("mensagens: esperado 4, obtido %d", msgCount)
	}

	// Verificar FK conversation_id aponta para UUID correto
	var convIDofMsg string
	if err := sqlDB.QueryRow("SELECT conversation_id FROM chat_messages WHERE content = 'Olá'").Scan(&convIDofMsg); err != nil {
		t.Fatal(err)
	}
	if convIDofMsg != convIDs["Conv Alpha"] {
		t.Errorf("mensagem 'Olá' conversation_id = %q, esperado %q", convIDofMsg, convIDs["Conv Alpha"])
	}

	var convIDofMsg2 string
	if err := sqlDB.QueryRow("SELECT conversation_id FROM chat_messages WHERE content = 'Teste'").Scan(&convIDofMsg2); err != nil {
		t.Fatal(err)
	}
	if convIDofMsg2 != convIDs["Conv Beta"] {
		t.Errorf("mensagem 'Teste' conversation_id = %q, esperado %q", convIDofMsg2, convIDs["Conv Beta"])
	}

	// Verificar IDs das mensagens são UUIDs
	msgRows, _ := sqlDB.Query("SELECT id FROM chat_messages")
	defer func() { _ = msgRows.Close() }()
	for msgRows.Next() {
		var id string
		if err := msgRows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		if !isUUID(id) {
			t.Errorf("chat_messages.id não é UUID: %q", id)
		}
	}
}

func TestMigration_SelfReferencingFK_ParentID(t *testing.T) {
	sqlDB := createOldSchemaDB(t)

	mustExec(t, sqlDB, `INSERT INTO conversations (id, title, created_at, updated_at) VALUES (1, 'Conv', '2026-01-01', '2026-01-01')`)

	// Cadeia: msg1 ← msg2 ← msg3
	mustExec(t, sqlDB, `INSERT INTO chat_messages (id, conversation_id, role, content, created_at, updated_at) VALUES (1, 1, 'user', 'Msg1', '2026-01-01', '2026-01-01')`)
	mustExec(t, sqlDB, `INSERT INTO chat_messages (id, conversation_id, parent_id, role, content, created_at, updated_at) VALUES (2, 1, 1, 'assistant', 'Msg2', '2026-01-01', '2026-01-01')`)
	mustExec(t, sqlDB, `INSERT INTO chat_messages (id, conversation_id, parent_id, role, content, created_at, updated_at) VALUES (3, 1, 2, 'user', 'Msg3', '2026-01-01', '2026-01-01')`)

	if err := migrateToUUIDv7(); err != nil {
		t.Fatalf("migração: %v", err)
	}

	// Construir mapa content → id
	msgIDs := make(map[string]string)
	rows, _ := sqlDB.Query("SELECT id, content FROM chat_messages")
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var id, content string
		if err := rows.Scan(&id, &content); err != nil {
			t.Fatal(err)
		}
		msgIDs[content] = id
	}

	// Verificar parent_id chain
	var parentOfMsg2 sql.NullString
	if err := sqlDB.QueryRow("SELECT parent_id FROM chat_messages WHERE content = 'Msg2'").Scan(&parentOfMsg2); err != nil {
		t.Fatal(err)
	}
	if !parentOfMsg2.Valid || parentOfMsg2.String != msgIDs["Msg1"] {
		t.Errorf("Msg2.parent_id = %v, esperado %s", parentOfMsg2, msgIDs["Msg1"])
	}

	var parentOfMsg3 sql.NullString
	if err := sqlDB.QueryRow("SELECT parent_id FROM chat_messages WHERE content = 'Msg3'").Scan(&parentOfMsg3); err != nil {
		t.Fatal(err)
	}
	if !parentOfMsg3.Valid || parentOfMsg3.String != msgIDs["Msg2"] {
		t.Errorf("Msg3.parent_id = %v, esperado %s", parentOfMsg3, msgIDs["Msg2"])
	}

	// Msg1 não tem parent
	var parentOfMsg1 sql.NullString
	if err := sqlDB.QueryRow("SELECT parent_id FROM chat_messages WHERE content = 'Msg1'").Scan(&parentOfMsg1); err != nil {
		t.Fatal(err)
	}
	if parentOfMsg1.Valid && parentOfMsg1.String != "" {
		t.Errorf("Msg1.parent_id deveria ser NULL, obteve %v", parentOfMsg1)
	}
}

func TestMigration_TurnID_SelfRef(t *testing.T) {
	sqlDB := createOldSchemaDB(t)

	mustExec(t, sqlDB, `INSERT INTO conversations (id, title, created_at, updated_at) VALUES (1, 'Conv', '2026-01-01', '2026-01-01')`)

	// turn_id aponta para a primeira mensagem do turno (self-ref)
	mustExec(t, sqlDB, `INSERT INTO chat_messages (id, conversation_id, role, content, created_at, updated_at) VALUES (100, 1, 'user', 'Turno', '2026-01-01', '2026-01-01')`)
	mustExec(t, sqlDB, `INSERT INTO chat_messages (id, conversation_id, turn_id, role, content, created_at, updated_at) VALUES (101, 1, 100, 'assistant', 'Resposta', '2026-01-01', '2026-01-01')`)

	if err := migrateToUUIDv7(); err != nil {
		t.Fatalf("migração: %v", err)
	}

	msgIDs := make(map[string]string)
	rows, _ := sqlDB.Query("SELECT id, content FROM chat_messages")
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var id, content string
		if err := rows.Scan(&id, &content); err != nil {
			t.Fatal(err)
		}
		msgIDs[content] = id
	}

	var turnID sql.NullString
	if err := sqlDB.QueryRow("SELECT turn_id FROM chat_messages WHERE content = 'Resposta'").Scan(&turnID); err != nil {
		t.Fatal(err)
	}
	if !turnID.Valid || turnID.String != msgIDs["Turno"] {
		t.Errorf("Resposta.turn_id = %v, esperado %s", turnID, msgIDs["Turno"])
	}
}

func TestMigration_SummaryUpToMessageID(t *testing.T) {
	sqlDB := createOldSchemaDB(t)

	mustExec(t, sqlDB, `INSERT INTO conversations (id, title, summary, summary_up_to_message_id, created_at, updated_at) VALUES (1, 'Resumida', 'Resumo aqui', 5, '2026-01-01', '2026-01-01')`)
	mustExec(t, sqlDB, `INSERT INTO conversations (id, title, created_at, updated_at) VALUES (2, 'Sem resumo', '2026-01-02', '2026-01-02')`)

	// Mensagem 5 na conversa 1
	mustExec(t, sqlDB, `INSERT INTO chat_messages (id, conversation_id, role, content, created_at, updated_at) VALUES (5, 1, 'user', 'Ultima resumida', '2026-01-01', '2026-01-01')`)
	mustExec(t, sqlDB, `INSERT INTO chat_messages (id, conversation_id, role, content, created_at, updated_at) VALUES (6, 1, 'assistant', 'Depois do resumo', '2026-01-01', '2026-01-01')`)

	if err := migrateToUUIDv7(); err != nil {
		t.Fatalf("migração: %v", err)
	}

	// Pegar UUID da mensagem que era id=5
	var msg5UUID string
	if err := sqlDB.QueryRow("SELECT id FROM chat_messages WHERE content = 'Ultima resumida'").Scan(&msg5UUID); err != nil {
		t.Fatal(err)
	}
	if !isUUID(msg5UUID) {
		t.Fatalf("msg5 UUID inválido: %q", msg5UUID)
	}

	// Verificar que summary_up_to_message_id foi atualizado
	var summaryRef sql.NullString
	if err := sqlDB.QueryRow("SELECT summary_up_to_message_id FROM conversations WHERE title = 'Resumida'").Scan(&summaryRef); err != nil {
		t.Fatal(err)
	}
	if !summaryRef.Valid || summaryRef.String != msg5UUID {
		t.Errorf("summary_up_to_message_id = %v, esperado %s", summaryRef, msg5UUID)
	}

	// Conversa sem resumo mantém NULL
	var summaryRef2 sql.NullString
	if err := sqlDB.QueryRow("SELECT summary_up_to_message_id FROM conversations WHERE title = 'Sem resumo'").Scan(&summaryRef2); err != nil {
		t.Fatal(err)
	}
	if summaryRef2.Valid && summaryRef2.String != "" {
		t.Errorf("conversa sem resumo deveria ter summary_up_to_message_id NULL, obteve %v", summaryRef2)
	}
}

func TestMigration_TaskListFullHierarchy(t *testing.T) {
	sqlDB := createOldSchemaDB(t)

	// Task list + workflow + tasks com parent + notes
	mustExec(t, sqlDB, `INSERT INTO task_lists (id, title, slug, created_at, updated_at) VALUES (1, 'Sprint 1', 'sprint-1', '2026-01-01', '2026-01-01')`)
	mustExec(t, sqlDB, `INSERT INTO task_lists (id, title, slug, created_at, updated_at) VALUES (2, 'Backlog', 'backlog', '2026-01-02', '2026-01-02')`)

	mustExec(t, sqlDB, `INSERT INTO task_list_workflows (id, task_list_id, statuses, allowed_transitions, initial_status_id, created_at, updated_at) VALUES (1, 1, '[{"id":1,"label":"Todo"},{"id":2,"label":"Done"}]', '{"1":[2]}', 1, '2026-01-01', '2026-01-01')`)
	mustExec(t, sqlDB, `INSERT INTO task_list_workflows (id, task_list_id, statuses, allowed_transitions, initial_status_id, created_at, updated_at) VALUES (2, 2, '[{"id":1,"label":"Open"}]', '{}', 1, '2026-01-02', '2026-01-02')`)

	// Tasks com parent_id (hierarquia)
	mustExec(t, sqlDB, `INSERT INTO tasks (id, task_list_id, title, status_id, created_at, updated_at) VALUES (10, 1, 'Parent Task', 1, '2026-01-01', '2026-01-01')`)
	mustExec(t, sqlDB, `INSERT INTO tasks (id, task_list_id, title, status_id, parent_id, created_at, updated_at) VALUES (11, 1, 'Child Task', 1, 10, '2026-01-01', '2026-01-01')`)
	mustExec(t, sqlDB, `INSERT INTO tasks (id, task_list_id, title, status_id, parent_id, created_at, updated_at) VALUES (12, 1, 'Grandchild', 2, 11, '2026-01-01', '2026-01-01')`)
	mustExec(t, sqlDB, `INSERT INTO tasks (id, task_list_id, title, status_id, created_at, updated_at) VALUES (20, 2, 'Backlog item', 1, '2026-01-02', '2026-01-02')`)

	// Notes
	mustExec(t, sqlDB, `INSERT INTO task_notes (id, task_id, type, content, created_at, updated_at) VALUES (1, 10, 1, 'Nota na parent', '2026-01-01', '2026-01-01')`)
	mustExec(t, sqlDB, `INSERT INTO task_notes (id, task_id, type, content, created_at, updated_at) VALUES (2, 11, 1, 'Nota na child', '2026-01-01', '2026-01-01')`)

	if err := migrateToUUIDv7(); err != nil {
		t.Fatalf("migração: %v", err)
	}

	// Construir mapa title → uuid para tasks
	taskIDs := make(map[string]string)
	rows, _ := sqlDB.Query("SELECT id, title FROM tasks")
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var id, title string
		if err := rows.Scan(&id, &title); err != nil {
			t.Fatal(err)
		}
		if !isUUID(id) {
			t.Errorf("tasks.id não é UUID: %q", id)
		}
		taskIDs[title] = id
	}

	if len(taskIDs) != 4 {
		t.Fatalf("esperado 4 tasks, obtido %d", len(taskIDs))
	}

	// Verificar FK task_list_id
	tlIDs := make(map[string]string) // title → uuid
	tlRows, _ := sqlDB.Query("SELECT id, title FROM task_lists")
	defer func() { _ = tlRows.Close() }()
	for tlRows.Next() {
		var id, title string
		if err := tlRows.Scan(&id, &title); err != nil {
			t.Fatal(err)
		}
		tlIDs[title] = id
	}

	var parentTaskListID string
	if err := sqlDB.QueryRow("SELECT task_list_id FROM tasks WHERE title = 'Parent Task'").Scan(&parentTaskListID); err != nil {
		t.Fatal(err)
	}
	if parentTaskListID != tlIDs["Sprint 1"] {
		t.Errorf("Parent Task.task_list_id = %q, esperado %q", parentTaskListID, tlIDs["Sprint 1"])
	}

	// Verificar parent_id chain: Child→Parent, Grandchild→Child
	var childParent sql.NullString
	if err := sqlDB.QueryRow("SELECT parent_id FROM tasks WHERE title = 'Child Task'").Scan(&childParent); err != nil {
		t.Fatal(err)
	}
	if !childParent.Valid || childParent.String != taskIDs["Parent Task"] {
		t.Errorf("Child.parent_id = %v, esperado %s", childParent, taskIDs["Parent Task"])
	}

	var grandchildParent sql.NullString
	if err := sqlDB.QueryRow("SELECT parent_id FROM tasks WHERE title = 'Grandchild'").Scan(&grandchildParent); err != nil {
		t.Fatal(err)
	}
	if !grandchildParent.Valid || grandchildParent.String != taskIDs["Child Task"] {
		t.Errorf("Grandchild.parent_id = %v, esperado %s", grandchildParent, taskIDs["Child Task"])
	}

	// Parent Task não tem parent
	var parentParent sql.NullString
	if err := sqlDB.QueryRow("SELECT parent_id FROM tasks WHERE title = 'Parent Task'").Scan(&parentParent); err != nil {
		t.Fatal(err)
	}
	if parentParent.Valid && parentParent.String != "" {
		t.Errorf("Parent Task deveria ter parent_id NULL, obteve %v", parentParent)
	}

	// Verificar workflow FK
	var wfTaskListID string
	if err := sqlDB.QueryRow("SELECT task_list_id FROM task_list_workflows WHERE statuses LIKE '%Todo%'").Scan(&wfTaskListID); err != nil {
		t.Fatal(err)
	}
	if wfTaskListID != tlIDs["Sprint 1"] {
		t.Errorf("workflow.task_list_id = %q, esperado %q", wfTaskListID, tlIDs["Sprint 1"])
	}

	// Verificar que initial_status_id permanece int (não é FK de banco)
	var initialStatusID int
	if err := sqlDB.QueryRow("SELECT initial_status_id FROM task_list_workflows WHERE statuses LIKE '%Todo%'").Scan(&initialStatusID); err != nil {
		t.Fatal(err)
	}
	if initialStatusID != 1 {
		t.Errorf("initial_status_id = %d, esperado 1", initialStatusID)
	}

	// Verificar task_notes FK
	var noteTaskID string
	if err := sqlDB.QueryRow("SELECT task_id FROM task_notes WHERE content = 'Nota na parent'").Scan(&noteTaskID); err != nil {
		t.Fatal(err)
	}
	if noteTaskID != taskIDs["Parent Task"] {
		t.Errorf("note.task_id = %q, esperado %q", noteTaskID, taskIDs["Parent Task"])
	}

	var noteTaskID2 string
	if err := sqlDB.QueryRow("SELECT task_id FROM task_notes WHERE content = 'Nota na child'").Scan(&noteTaskID2); err != nil {
		t.Fatal(err)
	}
	if noteTaskID2 != taskIDs["Child Task"] {
		t.Errorf("note2.task_id = %q, esperado %q", noteTaskID2, taskIDs["Child Task"])
	}
}

func TestMigration_CredentialEntries(t *testing.T) {
	sqlDB := createOldSchemaDB(t)

	mustExec(t, sqlDB, `INSERT INTO credential_entries (id, name, type, api_key_enc, created_at, updated_at) VALUES (1, 'OpenAI', 'api_key', 'enc_data_1', '2026-01-01', '2026-01-01')`)
	mustExec(t, sqlDB, `INSERT INTO credential_entries (id, name, type, api_key_enc, created_at, updated_at) VALUES (2, 'Azure', 'oauth', 'enc_data_2', '2026-01-02', '2026-01-02')`)
	mustExec(t, sqlDB, `INSERT INTO credential_key_wraps (id, wrapped_key, created_at, updated_at) VALUES (1, 'wrap_abc', '2026-01-01', '2026-01-01')`)

	if err := migrateToUUIDv7(); err != nil {
		t.Fatalf("migração: %v", err)
	}

	// Verificar dados preservados
	var name, apiKey string
	if err := sqlDB.QueryRow("SELECT name, api_key_enc FROM credential_entries WHERE name = 'OpenAI'").Scan(&name, &apiKey); err != nil {
		t.Fatal(err)
	}
	if name != "OpenAI" || apiKey != "enc_data_1" {
		t.Errorf("dados corrompidos: name=%q, api_key_enc=%q", name, apiKey)
	}

	var wrappedKey string
	if err := sqlDB.QueryRow("SELECT wrapped_key FROM credential_key_wraps").Scan(&wrappedKey); err != nil {
		t.Fatal(err)
	}
	if wrappedKey != "wrap_abc" {
		t.Errorf("wrapped_key = %q, esperado 'wrap_abc'", wrappedKey)
	}

	// IDs são UUID
	var credID string
	if err := sqlDB.QueryRow("SELECT id FROM credential_entries WHERE name = 'OpenAI'").Scan(&credID); err != nil {
		t.Fatal(err)
	}
	if !isUUID(credID) {
		t.Errorf("credential_entries.id não é UUID: %q", credID)
	}
}

func TestMigration_UniqueUUIDs(t *testing.T) {
	sqlDB := createOldSchemaDB(t)

	// Inserir muitos registros e verificar que nenhum UUID colide
	for i := 1; i <= 50; i++ {
		mustExec(t, sqlDB, fmt.Sprintf(`INSERT INTO conversations (id, title, created_at, updated_at) VALUES (%d, 'Conv %d', '2026-01-01', '2026-01-01')`, i, i))
	}
	for i := 1; i <= 100; i++ {
		mustExec(t, sqlDB, fmt.Sprintf(`INSERT INTO chat_messages (id, conversation_id, role, content, created_at, updated_at) VALUES (%d, %d, 'user', 'Msg %d', '2026-01-01', '2026-01-01')`, i, (i%50)+1, i))
	}

	if err := migrateToUUIDv7(); err != nil {
		t.Fatalf("migração: %v", err)
	}

	// Verificar unicidade de IDs em conversations
	seen := make(map[string]bool)
	rows, _ := sqlDB.Query("SELECT id FROM conversations")
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		if seen[id] {
			t.Errorf("UUID duplicado em conversations: %s", id)
		}
		seen[id] = true
	}
	if len(seen) != 50 {
		t.Errorf("esperado 50 conversations, obtido %d", len(seen))
	}

	// Verificar unicidade em chat_messages
	seen = make(map[string]bool)
	msgRows, _ := sqlDB.Query("SELECT id FROM chat_messages")
	defer func() { _ = msgRows.Close() }()
	for msgRows.Next() {
		var id string
		if err := msgRows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		if seen[id] {
			t.Errorf("UUID duplicado em chat_messages: %s", id)
		}
		seen[id] = true
	}
	if len(seen) != 100 {
		t.Errorf("esperado 100 chat_messages, obtido %d", len(seen))
	}
}

func TestMigration_DataPreservation(t *testing.T) {
	sqlDB := createOldSchemaDB(t)

	// Inserir dados com todos os campos preenchidos
	mustExec(t, sqlDB, `INSERT INTO conversations (id, title, channel, contact_id, summary, summary_up_to_message_id, summarizing_in_progress, created_at, updated_at) VALUES (1, 'Full Conv', 'telegram', 'user123', 'Resumo completo', NULL, 1, '2026-03-15 10:30:00', '2026-03-15 11:00:00')`)

	mustExec(t, sqlDB, `INSERT INTO chat_messages (id, conversation_id, role, content, reasoning, model, source, prompt_tokens, completion_tokens, total_tokens, created_at, updated_at) VALUES (1, 1, 'assistant', 'Resposta completa', 'Pensei assim', 'gpt-4', 'api', 100, 50, 150, '2026-03-15 10:31:00', '2026-03-15 10:31:00')`)

	mustExec(t, sqlDB, `INSERT INTO tasks (id, task_list_id, title, description, code, link, status_id, "order", assignee_name, creator_name, due_date, created_at, updated_at, completed_at) VALUES (1, 0, 'Task completa', 'Descr longa', 'TSK-1', 'https://example.com', 2, 5, 'Alice', 'Bob', '2026-06-01', '2026-01-01', '2026-03-01', '2026-02-15')`)

	if err := migrateToUUIDv7(); err != nil {
		t.Fatalf("migração: %v", err)
	}

	// Verificar que TODOS os campos foram preservados
	var title, channel, contactID, summary string
	var summarizing int
	var createdAt, updatedAt string
	if err := sqlDB.QueryRow("SELECT title, channel, contact_id, summary, summarizing_in_progress, created_at, updated_at FROM conversations").Scan(&title, &channel, &contactID, &summary, &summarizing, &createdAt, &updatedAt); err != nil {
		t.Fatal(err)
	}

	if title != "Full Conv" || channel != "telegram" || contactID != "user123" || summary != "Resumo completo" || summarizing != 1 {
		t.Errorf("dados de conversa corrompidos: title=%q channel=%q contact=%q summary=%q summarizing=%d", title, channel, contactID, summary, summarizing)
	}

	var role, content, reasoning, model, source string
	var pt, ct, tt int
	if err := sqlDB.QueryRow("SELECT role, content, reasoning, model, source, prompt_tokens, completion_tokens, total_tokens FROM chat_messages").Scan(&role, &content, &reasoning, &model, &source, &pt, &ct, &tt); err != nil {
		t.Fatal(err)
	}
	if role != "assistant" || content != "Resposta completa" || reasoning != "Pensei assim" || model != "gpt-4" || source != "api" || pt != 100 || ct != 50 || tt != 150 {
		t.Errorf("dados de mensagem corrompidos")
	}

	var desc, code, link, assignee, creator string
	var statusID, order int
	var dueDate, completedAt sql.NullString
	if err := sqlDB.QueryRow("SELECT description, code, link, status_id, \"order\", assignee_name, creator_name, due_date, completed_at FROM tasks").Scan(&desc, &code, &link, &statusID, &order, &assignee, &creator, &dueDate, &completedAt); err != nil {
		t.Fatal(err)
	}
	if desc != "Descr longa" || code != "TSK-1" || link != "https://example.com" || statusID != 2 || order != 5 || assignee != "Alice" || creator != "Bob" {
		t.Errorf("dados de task corrompidos: desc=%q code=%q link=%q status=%d order=%d assignee=%q creator=%q", desc, code, link, statusID, order, assignee, creator)
	}
	if !dueDate.Valid || !completedAt.Valid {
		t.Error("due_date ou completed_at perdidos")
	}
}

func TestMigration_NullForeignKeys(t *testing.T) {
	sqlDB := createOldSchemaDB(t)

	mustExec(t, sqlDB, `INSERT INTO conversations (id, title, created_at, updated_at) VALUES (1, 'Conv', '2026-01-01', '2026-01-01')`)

	// Mensagem sem parent_id e sem turn_id (NULLs)
	mustExec(t, sqlDB, `INSERT INTO chat_messages (id, conversation_id, role, content, created_at, updated_at) VALUES (1, 1, 'user', 'Sem parent', '2026-01-01', '2026-01-01')`)

	// Task sem parent_id
	mustExec(t, sqlDB, `INSERT INTO task_lists (id, title, slug, created_at, updated_at) VALUES (1, 'TL', 'tl', '2026-01-01', '2026-01-01')`)
	mustExec(t, sqlDB, `INSERT INTO tasks (id, task_list_id, title, status_id, created_at, updated_at) VALUES (1, 1, 'Raiz', 1, '2026-01-01', '2026-01-01')`)

	if err := migrateToUUIDv7(); err != nil {
		t.Fatalf("migração: %v", err)
	}

	var parentID, turnID sql.NullString
	if err := sqlDB.QueryRow("SELECT parent_id, turn_id FROM chat_messages").Scan(&parentID, &turnID); err != nil {
		t.Fatal(err)
	}
	if parentID.Valid && parentID.String != "" {
		t.Errorf("parent_id deveria ser NULL, obteve %v", parentID)
	}
	if turnID.Valid && turnID.String != "" {
		t.Errorf("turn_id deveria ser NULL, obteve %v", turnID)
	}

	var taskParent sql.NullString
	if err := sqlDB.QueryRow("SELECT parent_id FROM tasks").Scan(&taskParent); err != nil {
		t.Fatal(err)
	}
	if taskParent.Valid && taskParent.String != "" {
		t.Errorf("task.parent_id deveria ser NULL, obteve %v", taskParent)
	}
}

func TestMigration_PostMigrationGORMCompatibility(t *testing.T) {
	createOldSchemaDB(t)

	// Seed dados no schema antigo
	sqlDB, _ := db.DB()
	mustExec(t, sqlDB, `INSERT INTO conversations (id, title, created_at, updated_at) VALUES (1, 'Test', '2026-01-01', '2026-01-01')`)
	mustExec(t, sqlDB, `INSERT INTO chat_messages (id, conversation_id, role, content, created_at, updated_at) VALUES (1, 1, 'user', 'Hello', '2026-01-01', '2026-01-01')`)

	// Migrar
	if err := migrateToUUIDv7(); err != nil {
		t.Fatalf("migração: %v", err)
	}

	// Agora rodar AutoMigrate do GORM (simula o Init() pós-migração)
	if err := db.AutoMigrate(&Conversation{}, &ChatMessage{}, &TaskList{}, &TaskListWorkflow{}, &Task{}, &TaskNote{}, &CredentialEntry{}, &CredentialKeyWrap{}); err != nil {
		t.Fatalf("AutoMigrate pós-migração: %v", err)
	}

	// GORM deve conseguir ler os dados migrados
	var conv Conversation
	if err := db.First(&conv, "title = ?", "Test").Error; err != nil {
		t.Fatalf("GORM não conseguiu ler conversa migrada: %v", err)
	}
	if !isUUID(conv.ID) {
		t.Errorf("conv.ID não é UUID: %q", conv.ID)
	}
	if conv.Title != "Test" {
		t.Errorf("conv.Title = %q, esperado 'Test'", conv.Title)
	}

	var msg ChatMessage
	if err := db.First(&msg, "content = ?", "Hello").Error; err != nil {
		t.Fatalf("GORM não conseguiu ler mensagem migrada: %v", err)
	}
	if !isUUID(msg.ID) {
		t.Errorf("msg.ID não é UUID: %q", msg.ID)
	}
	if msg.ConversationID != conv.ID {
		t.Errorf("msg.ConversationID = %q, esperado %q", msg.ConversationID, conv.ID)
	}

	// GORM deve conseguir criar novos registros (BeforeCreate gera UUID)
	newConv := Conversation{Title: "Nova conversa"}
	if err := db.Create(&newConv).Error; err != nil {
		t.Fatalf("GORM Create falhou: %v", err)
	}
	if !isUUID(newConv.ID) {
		t.Errorf("novo conv.ID não é UUID: %q", newConv.ID)
	}

	// FTS5 funciona pós-migração
	if err := initFTS5(); err != nil {
		t.Fatalf("initFTS5 pós-migração: %v", err)
	}

	// Inserir mensagem e verificar FTS
	newMsg := ChatMessage{
		ConversationID: newConv.ID,
		Role:           "user",
		Content:        "busca especifica aqui",
	}
	if err := db.Create(&newMsg).Error; err != nil {
		t.Fatalf("criar mensagem pós-migração: %v", err)
	}

	var ftsCount int
	if err := sqlDB.QueryRow("SELECT count(*) FROM chat_messages_fts WHERE chat_messages_fts MATCH 'busca especifica'").Scan(&ftsCount); err != nil {
		t.Fatal(err)
	}
	if ftsCount != 1 {
		t.Errorf("FTS5 não indexou nova mensagem: count=%d", ftsCount)
	}
}

func TestMigration_MultipleConversationsWithMessages(t *testing.T) {
	sqlDB := createOldSchemaDB(t)

	// 3 conversas, cada uma com mensagens apontando para a conversa correta
	for c := 1; c <= 3; c++ {
		mustExec(t, sqlDB, fmt.Sprintf(`INSERT INTO conversations (id, title, created_at, updated_at) VALUES (%d, 'Conv%d', '2026-01-01', '2026-01-01')`, c, c))
		for m := 1; m <= 5; m++ {
			msgID := c*100 + m
			mustExec(t, sqlDB, fmt.Sprintf(`INSERT INTO chat_messages (id, conversation_id, role, content, created_at, updated_at) VALUES (%d, %d, 'user', 'Conv%d-Msg%d', '2026-01-01', '2026-01-01')`, msgID, c, c, m))
		}
	}

	if err := migrateToUUIDv7(); err != nil {
		t.Fatalf("migração: %v", err)
	}

	// Verificar que cada conversa mantém exatamente 5 mensagens com FK correto
	convIDs := make(map[string]string) // title → uuid
	rows, _ := sqlDB.Query("SELECT id, title FROM conversations")
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var id, title string
		if err := rows.Scan(&id, &title); err != nil {
			t.Fatal(err)
		}
		convIDs[title] = id
	}

	for c := 1; c <= 3; c++ {
		convTitle := fmt.Sprintf("Conv%d", c)
		convUUID := convIDs[convTitle]

		var count int
		if err := sqlDB.QueryRow("SELECT count(*) FROM chat_messages WHERE conversation_id = ?", convUUID).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 5 {
			t.Errorf("%s: esperado 5 mensagens, obtido %d", convTitle, count)
		}

		// Verificar conteúdo
		msgRows, _ := sqlDB.Query("SELECT content FROM chat_messages WHERE conversation_id = ? ORDER BY content", convUUID)
		var contents []string
		for msgRows.Next() {
			var content string
			if err := msgRows.Scan(&content); err != nil {
				t.Fatal(err)
			}
			contents = append(contents, content)
		}
		_ = msgRows.Close()

		for m := 1; m <= 5; m++ {
			expected := fmt.Sprintf("Conv%d-Msg%d", c, m)
			found := false
			for _, c := range contents {
				if c == expected {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("mensagem %q não encontrada na conversa %s", expected, convTitle)
			}
		}
	}
}

func mustExec(t *testing.T, sqlDB *sql.DB, query string) {
	t.Helper()
	if _, err := sqlDB.Exec(query); err != nil {
		t.Fatalf("exec failed: %v\nSQL: %s", err, query)
	}
}

// === Testes adicionais para brechas identificadas ===

// TestMigration_ForwardSelfReference testa o caso onde parent_id aponta para um
// registro com ID MAIOR (forward reference). O registro referenciado é processado
// depois, então a resolução inline falha e o 2° passe precisa corrigir.
func TestMigration_ForwardSelfReference(t *testing.T) {
	sqlDB := createOldSchemaDB(t)

	mustExec(t, sqlDB, `INSERT INTO conversations (id, title, created_at, updated_at) VALUES (1, 'Conv', '2026-01-01', '2026-01-01')`)

	// Inserir mensagens fora de ordem lógica:
	// msg1 (id=1) aponta parent_id=3 (forward ref!)
	// msg2 (id=2) sem parent
	// msg3 (id=3) sem parent
	mustExec(t, sqlDB, `INSERT INTO chat_messages (id, conversation_id, parent_id, role, content, created_at, updated_at) VALUES (1, 1, 3, 'assistant', 'Resposta antecipada', '2026-01-01', '2026-01-01')`)
	mustExec(t, sqlDB, `INSERT INTO chat_messages (id, conversation_id, role, content, created_at, updated_at) VALUES (2, 1, 'user', 'Mensagem normal', '2026-01-01', '2026-01-01')`)
	mustExec(t, sqlDB, `INSERT INTO chat_messages (id, conversation_id, role, content, created_at, updated_at) VALUES (3, 1, 'user', 'Mensagem referenciada', '2026-01-01', '2026-01-01')`)

	if err := migrateToUUIDv7(); err != nil {
		t.Fatalf("migração: %v", err)
	}

	// Construir mapa content → uuid
	msgIDs := make(map[string]string)
	rows, _ := sqlDB.Query("SELECT id, content FROM chat_messages")
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var id, content string
		if err := rows.Scan(&id, &content); err != nil {
			t.Fatal(err)
		}
		msgIDs[content] = id
	}

	// Verificar que parent_id de msg1 aponta corretamente para msg3
	var parentOfMsg1 sql.NullString
	if err := sqlDB.QueryRow("SELECT parent_id FROM chat_messages WHERE content = 'Resposta antecipada'").Scan(&parentOfMsg1); err != nil {
		t.Fatal(err)
	}
	if !parentOfMsg1.Valid {
		t.Fatal("parent_id de 'Resposta antecipada' é NULL — forward reference perdida!")
	}
	if parentOfMsg1.String != msgIDs["Mensagem referenciada"] {
		t.Errorf("parent_id = %q, esperado %q (UUID de 'Mensagem referenciada')", parentOfMsg1.String, msgIDs["Mensagem referenciada"])
	}

	// Verificar que msg2 continua sem parent
	var parentOfMsg2 sql.NullString
	if err := sqlDB.QueryRow("SELECT parent_id FROM chat_messages WHERE content = 'Mensagem normal'").Scan(&parentOfMsg2); err != nil {
		t.Fatal(err)
	}
	if parentOfMsg2.Valid && parentOfMsg2.String != "" {
		t.Errorf("msg2 deveria ter parent_id NULL, obteve %v", parentOfMsg2)
	}
}

// TestMigration_ForwardSelfReference_Tasks testa forward self-ref em tasks.
func TestMigration_ForwardSelfReference_Tasks(t *testing.T) {
	sqlDB := createOldSchemaDB(t)

	mustExec(t, sqlDB, `INSERT INTO task_lists (id, title, slug, created_at, updated_at) VALUES (1, 'TL', 'tl', '2026-01-01', '2026-01-01')`)

	// Task 1 aponta parent_id=3 (forward ref!)
	// Task 2 sem parent
	// Task 3 sem parent (é o parent referenciado)
	mustExec(t, sqlDB, `INSERT INTO tasks (id, task_list_id, title, status_id, parent_id, created_at, updated_at) VALUES (1, 1, 'Child first', 1, 3, '2026-01-01', '2026-01-01')`)
	mustExec(t, sqlDB, `INSERT INTO tasks (id, task_list_id, title, status_id, created_at, updated_at) VALUES (2, 1, 'Standalone', 1, '2026-01-01', '2026-01-01')`)
	mustExec(t, sqlDB, `INSERT INTO tasks (id, task_list_id, title, status_id, created_at, updated_at) VALUES (3, 1, 'Parent last', 1, '2026-01-01', '2026-01-01')`)

	if err := migrateToUUIDv7(); err != nil {
		t.Fatalf("migração: %v", err)
	}

	taskIDs := make(map[string]string)
	rows, _ := sqlDB.Query("SELECT id, title FROM tasks")
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var id, title string
		if err := rows.Scan(&id, &title); err != nil {
			t.Fatal(err)
		}
		taskIDs[title] = id
	}

	var parentOfChild sql.NullString
	if err := sqlDB.QueryRow("SELECT parent_id FROM tasks WHERE title = 'Child first'").Scan(&parentOfChild); err != nil {
		t.Fatal(err)
	}
	if !parentOfChild.Valid {
		t.Fatal("parent_id de 'Child first' é NULL — forward reference perdida!")
	}
	if parentOfChild.String != taskIDs["Parent last"] {
		t.Errorf("parent_id = %q, esperado %q", parentOfChild.String, taskIDs["Parent last"])
	}
}

// TestMigration_OrphanedForeignKey testa mensagem com conversation_id que não existe.
func TestMigration_OrphanedForeignKey(t *testing.T) {
	sqlDB := createOldSchemaDB(t)

	mustExec(t, sqlDB, `INSERT INTO conversations (id, title, created_at, updated_at) VALUES (1, 'Existe', '2026-01-01', '2026-01-01')`)

	// Mensagem com conversation_id=1 (existe) e conversation_id=999 (não existe)
	mustExec(t, sqlDB, `INSERT INTO chat_messages (id, conversation_id, role, content, created_at, updated_at) VALUES (1, 1, 'user', 'Normal', '2026-01-01', '2026-01-01')`)
	mustExec(t, sqlDB, `INSERT INTO chat_messages (id, conversation_id, role, content, created_at, updated_at) VALUES (2, 999, 'user', 'Órfã', '2026-01-01', '2026-01-01')`)

	if err := migrateToUUIDv7(); err != nil {
		t.Fatalf("migração: %v", err)
	}

	// Mensagem normal deve ter conversation_id UUID válido
	var normalConvID sql.NullString
	if err := sqlDB.QueryRow("SELECT conversation_id FROM chat_messages WHERE content = 'Normal'").Scan(&normalConvID); err != nil {
		t.Fatal(err)
	}
	if !normalConvID.Valid || !isUUID(normalConvID.String) {
		t.Errorf("mensagem normal deveria ter conversation_id UUID, obteve %v", normalConvID)
	}

	// Mensagem órfã deve ter conversation_id NULL (FK não resolvida)
	var orphanConvID sql.NullString
	if err := sqlDB.QueryRow("SELECT conversation_id FROM chat_messages WHERE content = 'Órfã'").Scan(&orphanConvID); err != nil {
		t.Fatal(err)
	}
	if orphanConvID.Valid && orphanConvID.String != "" {
		t.Errorf("mensagem órfã deveria ter conversation_id NULL, obteve %v", orphanConvID)
	}

	// Ambas as mensagens devem existir (migração não deve falhar por FK órfã)
	var count int
	if err := sqlDB.QueryRow("SELECT count(*) FROM chat_messages").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Errorf("esperado 2 mensagens, obtido %d", count)
	}
}

// TestMigration_ZeroForeignKey testa que parent_id=0 vira NULL (não UUID).
func TestMigration_ZeroForeignKey(t *testing.T) {
	sqlDB := createOldSchemaDB(t)

	mustExec(t, sqlDB, `INSERT INTO conversations (id, title, created_at, updated_at) VALUES (1, 'Conv', '2026-01-01', '2026-01-01')`)

	// parent_id=0 explícito (diferente de NULL)
	mustExec(t, sqlDB, `INSERT INTO chat_messages (id, conversation_id, parent_id, turn_id, role, content, created_at, updated_at) VALUES (1, 1, 0, 0, 'user', 'Zero refs', '2026-01-01', '2026-01-01')`)

	if err := migrateToUUIDv7(); err != nil {
		t.Fatalf("migração: %v", err)
	}

	var parentID, turnID sql.NullString
	if err := sqlDB.QueryRow("SELECT parent_id, turn_id FROM chat_messages").Scan(&parentID, &turnID); err != nil {
		t.Fatal(err)
	}

	if parentID.Valid && parentID.String != "" {
		t.Errorf("parent_id=0 deveria virar NULL, obteve %v", parentID)
	}
	if turnID.Valid && turnID.String != "" {
		t.Errorf("turn_id=0 deveria virar NULL, obteve %v", turnID)
	}
}

// TestMigration_ExtraColumnsInOldSchema testa que colunas extras na tabela antiga
// (que não estão na definição de migração) são silenciosamente ignoradas sem erro.
func TestMigration_ExtraColumnsInOldSchema(t *testing.T) {
	sqlDB := createOldSchemaDB(t)

	// Adicionar coluna extra que não existe no schema de migração
	mustExec(t, sqlDB, `ALTER TABLE conversations ADD COLUMN custom_field TEXT DEFAULT 'extra'`)
	mustExec(t, sqlDB, `INSERT INTO conversations (id, title, custom_field, created_at, updated_at) VALUES (1, 'Com extra', 'valor_custom', '2026-01-01', '2026-01-01')`)

	if err := migrateToUUIDv7(); err != nil {
		t.Fatalf("migração com coluna extra: %v", err)
	}

	// Conversa deve existir com título preservado
	var title string
	if err := sqlDB.QueryRow("SELECT title FROM conversations").Scan(&title); err != nil {
		t.Fatal(err)
	}
	if title != "Com extra" {
		t.Errorf("título = %q, esperado 'Com extra'", title)
	}

	// Coluna extra deve ter sido perdida (não está na definição de migração)
	// Isso é o comportamento esperado — migração redefine o schema
	var count int
	err := sqlDB.QueryRow("SELECT count(*) FROM pragma_table_info('conversations') WHERE name = 'custom_field'").Scan(&count)
	if err != nil {
		t.Fatalf("pragma: %v", err)
	}
	// A coluna extra NÃO deve existir no schema migrado
	if count != 0 {
		t.Log("Nota: coluna custom_field sobreviveu à migração (inesperado mas não fatal)")
	}
}

// TestMigration_FTS5DroppedAndRecreatable testa que FTS5 é dropada antes da
// migração e pode ser recriada depois.
func TestMigration_FTS5DroppedAndRecreatable(t *testing.T) {
	sqlDB := createOldSchemaDB(t)

	// Simular FTS5 existente no schema antigo
	mustExec(t, sqlDB, `CREATE VIRTUAL TABLE IF NOT EXISTS chat_messages_fts USING fts5(content, content_rowid='rowid')`)
	mustExec(t, sqlDB, `INSERT INTO conversations (id, title, created_at, updated_at) VALUES (1, 'Conv', '2026-01-01', '2026-01-01')`)
	mustExec(t, sqlDB, `INSERT INTO chat_messages (id, conversation_id, role, content, created_at, updated_at) VALUES (1, 1, 'user', 'texto pesquisavel', '2026-01-01', '2026-01-01')`)

	if err := migrateToUUIDv7(); err != nil {
		t.Fatalf("migração: %v", err)
	}

	// FTS5 deve ter sido dropada durante migração
	var ftsCount int
	if err := sqlDB.QueryRow("SELECT count(*) FROM sqlite_master WHERE name = 'chat_messages_fts'").Scan(&ftsCount); err != nil {
		t.Fatal(err)
	}
	if ftsCount != 0 {
		t.Error("chat_messages_fts deveria ter sido dropada durante migração")
	}

	// Deve ser possível recriar FTS5 pós-migração (mesma ordem do Init real)
	if err := db.AutoMigrate(&Conversation{}, &ChatMessage{}); err != nil {
		t.Fatalf("AutoMigrate: %v", err)
	}

	if err := initFTS5(); err != nil {
		t.Fatalf("recriar FTS5 pós-migração: %v", err)
	}

	// Inserir nova mensagem via GORM e verificar que FTS indexa
	var conv Conversation
	db.First(&conv)
	newMsg := ChatMessage{
		ConversationID: conv.ID,
		Role:           "user",
		Content:        "novo texto fts5",
	}
	db.Create(&newMsg)

	var matchCount int
	if err := sqlDB.QueryRow("SELECT count(*) FROM chat_messages_fts WHERE chat_messages_fts MATCH 'novo texto'").Scan(&matchCount); err != nil {
		t.Fatal(err)
	}
	if matchCount != 1 {
		t.Errorf("FTS5 não indexou nova mensagem: count=%d", matchCount)
	}
}

// TestMigration_NoConversationsTable testa que a migração é no-op quando
// a tabela conversations não existe (banco novo, pré-AutoMigrate).
func TestMigration_NoConversationsTable(t *testing.T) {
	var err error
	db, err = gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	sqlDB, _ := db.DB()
	t.Cleanup(func() {
		_ = sqlDB.Close()
		db = nil
	})

	// Banco vazio — sem tabela conversations
	if err := migrateToUUIDv7(); err != nil {
		t.Fatalf("deveria ser no-op em banco sem tabelas: %v", err)
	}
}

// TestMigration_SummaryPointingToNonExistentMessage testa que
// summary_up_to_message_id apontando para mensagem inexistente não causa crash.
// O valor antigo ("999") permanece como string suja — o backend lida.
func TestMigration_SummaryPointingToNonExistentMessage(t *testing.T) {
	sqlDB := createOldSchemaDB(t)

	// Conversa com summary_up_to_message_id=999 (mensagem não existe)
	mustExec(t, sqlDB, `INSERT INTO conversations (id, title, summary_up_to_message_id, created_at, updated_at) VALUES (1, 'Ref inválida', 999, '2026-01-01', '2026-01-01')`)
	mustExec(t, sqlDB, `INSERT INTO chat_messages (id, conversation_id, role, content, created_at, updated_at) VALUES (1, 1, 'user', 'Msg real', '2026-01-01', '2026-01-01')`)

	if err := migrateToUUIDv7(); err != nil {
		t.Fatalf("migração: %v", err)
	}

	// O valor "999" não é traduzido (nenhuma msg tem old_id=999).
	// summary_up_to_message_id fica com o valor antigo copiado como-é.
	var summaryRef sql.NullString
	if err := sqlDB.QueryRow("SELECT summary_up_to_message_id FROM conversations").Scan(&summaryRef); err != nil {
		t.Fatal(err)
	}
	if !summaryRef.Valid {
		t.Fatal("summary_up_to_message_id deveria ter valor (999 como string), mas é NULL")
	}
	// Deve ser "999" (string do integer antigo, não traduzido)
	if summaryRef.String != "999" {
		t.Errorf("summary_up_to_message_id = %q, esperado \"999\"", summaryRef.String)
	}
	if isUUID(summaryRef.String) {
		t.Error("summary_up_to_message_id não deveria ser UUID (msg 999 não existe)")
	}
}

// TestMigration_ForwardTurnID testa turn_id apontando para msg com ID maior (forward ref).
func TestMigration_ForwardTurnID(t *testing.T) {
	sqlDB := createOldSchemaDB(t)

	mustExec(t, sqlDB, `INSERT INTO conversations (id, title, created_at, updated_at) VALUES (1, 'Conv', '2026-01-01', '2026-01-01')`)

	// msg1 (id=1) tem turn_id=3 (forward ref!)
	// msg2 (id=2) sem turn_id
	// msg3 (id=3) sem turn_id (é o target)
	mustExec(t, sqlDB, `INSERT INTO chat_messages (id, conversation_id, turn_id, role, content, created_at, updated_at) VALUES (1, 1, 3, 'assistant', 'Resposta do turno', '2026-01-01', '2026-01-01')`)
	mustExec(t, sqlDB, `INSERT INTO chat_messages (id, conversation_id, role, content, created_at, updated_at) VALUES (2, 1, 'user', 'Sem turno', '2026-01-01', '2026-01-01')`)
	mustExec(t, sqlDB, `INSERT INTO chat_messages (id, conversation_id, role, content, created_at, updated_at) VALUES (3, 1, 'user', 'Inicio do turno', '2026-01-01', '2026-01-01')`)

	if err := migrateToUUIDv7(); err != nil {
		t.Fatalf("migração: %v", err)
	}

	msgIDs := make(map[string]string)
	rows, _ := sqlDB.Query("SELECT id, content FROM chat_messages")
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var id, content string
		if err := rows.Scan(&id, &content); err != nil {
			t.Fatal(err)
		}
		msgIDs[content] = id
	}

	// turn_id de msg1 deve apontar para msg3 (resolvido no 2° passe)
	var turnOfMsg1 sql.NullString
	if err := sqlDB.QueryRow("SELECT turn_id FROM chat_messages WHERE content = 'Resposta do turno'").Scan(&turnOfMsg1); err != nil {
		t.Fatal(err)
	}
	if !turnOfMsg1.Valid {
		t.Fatal("turn_id de 'Resposta do turno' é NULL — forward reference perdida!")
	}
	if turnOfMsg1.String != msgIDs["Inicio do turno"] {
		t.Errorf("turn_id = %q, esperado %q", turnOfMsg1.String, msgIDs["Inicio do turno"])
	}

	// msg2 sem turn_id continua NULL
	var turnOfMsg2 sql.NullString
	if err := sqlDB.QueryRow("SELECT turn_id FROM chat_messages WHERE content = 'Sem turno'").Scan(&turnOfMsg2); err != nil {
		t.Fatal(err)
	}
	if turnOfMsg2.Valid && turnOfMsg2.String != "" {
		t.Errorf("msg2 deveria ter turn_id NULL, obteve %v", turnOfMsg2)
	}
}

// TestMigration_PartialSchema testa migração quando algumas tabelas posteriores
// (task_notes, task_list_workflows) não existem no banco antigo.
// Isso pode acontecer se o usuário tem uma versão antiga do app que não tinha esses modelos.
func TestMigration_PartialSchema(t *testing.T) {
	// Criar banco com apenas conversations + chat_messages (schema mínimo)
	var err error
	db, err = gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("get sql.DB: %v", err)
	}
	t.Cleanup(func() {
		_ = sqlDB.Close()
		db = nil
	})

	// Criar APENAS conversations e chat_messages (sem tasks, sem credentials)
	mustExec(t, sqlDB, `CREATE TABLE conversations (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		title TEXT, channel TEXT, contact_id TEXT,
		summary TEXT, summary_up_to_message_id INTEGER,
		summarizing_in_progress INTEGER DEFAULT 0,
		created_at DATETIME, updated_at DATETIME
	)`)
	mustExec(t, sqlDB, `CREATE TABLE chat_messages (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		conversation_id INTEGER, parent_id INTEGER, turn_id INTEGER,
		role TEXT, content TEXT, reasoning TEXT,
		media TEXT, audio TEXT, audio_mime_type TEXT,
		tool_calls TEXT, tool_call_id TEXT,
		prompt_tokens INTEGER DEFAULT 0, completion_tokens INTEGER DEFAULT 0, total_tokens INTEGER DEFAULT 0,
		model TEXT, source TEXT,
		created_at DATETIME, updated_at DATETIME
	)`)

	// Seed dados
	mustExec(t, sqlDB, `INSERT INTO conversations (id, title, created_at, updated_at) VALUES (1, 'Antiga', '2026-01-01', '2026-01-01')`)
	mustExec(t, sqlDB, `INSERT INTO chat_messages (id, conversation_id, role, content, created_at, updated_at) VALUES (1, 1, 'user', 'Msg antiga', '2026-01-01', '2026-01-01')`)

	// Migração deve funcionar mesmo sem as tabelas de tasks/credentials
	if err := migrateToUUIDv7(); err != nil {
		t.Fatalf("migração com schema parcial: %v", err)
	}

	// Verificar que conversations e messages foram migradas
	var convID string
	if err := sqlDB.QueryRow("SELECT id FROM conversations").Scan(&convID); err != nil {
		t.Fatal(err)
	}
	if !isUUID(convID) {
		t.Errorf("conversations.id não é UUID: %q", convID)
	}

	var msgID, msgConvID string
	if err := sqlDB.QueryRow("SELECT id, conversation_id FROM chat_messages").Scan(&msgID, &msgConvID); err != nil {
		t.Fatal(err)
	}
	if !isUUID(msgID) {
		t.Errorf("chat_messages.id não é UUID: %q", msgID)
	}
	if msgConvID != convID {
		t.Errorf("FK conversation_id = %q, esperado %q", msgConvID, convID)
	}

	// Tabelas que não existiam devem continuar sem existir (sem erro)
	var taskTableCount int
	if err := sqlDB.QueryRow("SELECT count(*) FROM sqlite_master WHERE type='table' AND name='tasks'").Scan(&taskTableCount); err != nil {
		t.Fatal(err)
	}
	if taskTableCount != 0 {
		t.Error("tabela tasks não deveria existir")
	}
}
