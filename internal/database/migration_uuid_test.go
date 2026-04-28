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
			pattern TEXT, auth_type TEXT,
			token_enc TEXT, username TEXT, password_enc TEXT,
			headers_enc TEXT, expires_at INTEGER DEFAULT 0,
			refresh_token_enc TEXT, client_id_enc TEXT, client_secret_enc TEXT,
			created_at DATETIME, updated_at DATETIME
		)`,
		`CREATE TABLE credential_key_wraps (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			kind TEXT, salt TEXT, wrapped_dek TEXT,
			argon_time INTEGER DEFAULT 0, argon_memory INTEGER DEFAULT 0,
			argon_threads INTEGER DEFAULT 0,
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
			external_source TEXT, external_id TEXT,
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
		} else if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
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

	mustExec(t, sqlDB, `INSERT INTO credential_entries (id, pattern, auth_type, token_enc, username, password_enc, headers_enc, expires_at, refresh_token_enc, client_id_enc, client_secret_enc, created_at, updated_at) VALUES (1, 'api.openai.com', 'bearer', 'enc_token_1', '', '', '', 0, '', '', '', '2026-01-01', '2026-01-01')`)
	mustExec(t, sqlDB, `INSERT INTO credential_entries (id, pattern, auth_type, token_enc, username, password_enc, headers_enc, expires_at, refresh_token_enc, client_id_enc, client_secret_enc, created_at, updated_at) VALUES (2, 'llm.inclunet.com.br', 'bearer', 'enc_token_2', '', '', '', 0, '', '', '', '2026-01-02', '2026-01-02')`)
	mustExec(t, sqlDB, `INSERT INTO credential_key_wraps (id, kind, salt, wrapped_dek, argon_time, argon_memory, argon_threads, created_at, updated_at) VALUES (1, 'master', 'salt_abc', 'wrapped_dek_xyz', 3, 65536, 4, '2026-01-01', '2026-01-01')`)

	if err := migrateToUUIDv7(); err != nil {
		t.Fatalf("migração: %v", err)
	}

	// Verificar credential_entries: todos os campos preservados
	var pattern, authType, tokenEnc string
	if err := sqlDB.QueryRow("SELECT pattern, auth_type, token_enc FROM credential_entries WHERE pattern = 'api.openai.com'").Scan(&pattern, &authType, &tokenEnc); err != nil {
		t.Fatalf("query credential_entries: %v", err)
	}
	if pattern != "api.openai.com" || authType != "bearer" || tokenEnc != "enc_token_1" {
		t.Errorf("dados corrompidos: pattern=%q, auth_type=%q, token_enc=%q", pattern, authType, tokenEnc)
	}

	// Verificar segunda credencial
	var tokenEnc2 string
	if err := sqlDB.QueryRow("SELECT token_enc FROM credential_entries WHERE pattern = 'llm.inclunet.com.br'").Scan(&tokenEnc2); err != nil {
		t.Fatalf("query segunda credencial: %v", err)
	}
	if tokenEnc2 != "enc_token_2" {
		t.Errorf("segunda credencial token_enc = %q, esperado 'enc_token_2'", tokenEnc2)
	}

	// Verificar credential_key_wraps: todos os campos preservados
	var kind, salt, wrappedDEK string
	var argonTime, argonMemory, argonThreads int
	if err := sqlDB.QueryRow("SELECT kind, salt, wrapped_dek, argon_time, argon_memory, argon_threads FROM credential_key_wraps").Scan(&kind, &salt, &wrappedDEK, &argonTime, &argonMemory, &argonThreads); err != nil {
		t.Fatalf("query credential_key_wraps: %v", err)
	}
	if kind != "master" {
		t.Errorf("kind = %q, esperado 'master'", kind)
	}
	if salt != "salt_abc" {
		t.Errorf("salt = %q, esperado 'salt_abc'", salt)
	}
	if wrappedDEK != "wrapped_dek_xyz" {
		t.Errorf("wrapped_dek = %q, esperado 'wrapped_dek_xyz'", wrappedDEK)
	}
	if argonTime != 3 || argonMemory != 65536 || argonThreads != 4 {
		t.Errorf("argon params: time=%d memory=%d threads=%d, esperado 3/65536/4", argonTime, argonMemory, argonThreads)
	}

	// IDs são UUID
	var credID string
	if err := sqlDB.QueryRow("SELECT id FROM credential_entries WHERE pattern = 'api.openai.com'").Scan(&credID); err != nil {
		t.Fatal(err)
	}
	if !isUUID(credID) {
		t.Errorf("credential_entries.id não é UUID: %q", credID)
	}

	var kwID string
	if err := sqlDB.QueryRow("SELECT id FROM credential_key_wraps").Scan(&kwID); err != nil {
		t.Fatal(err)
	}
	if !isUUID(kwID) {
		t.Errorf("credential_key_wraps.id não é UUID: %q", kwID)
	}

	// Verificar contagem
	var credCount int
	if err := sqlDB.QueryRow("SELECT count(*) FROM credential_entries").Scan(&credCount); err != nil {
		t.Fatal(err)
	}
	if credCount != 2 {
		t.Errorf("esperado 2 credential_entries, obtido %d", credCount)
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

// TestMigration_CredentialColumnsMatchGORMModel verifica que as colunas da tabela
// migrada correspondem exatamente ao modelo GORM. Este teste existe para prevenir
// regressões onde a definição de migração diverge do modelo — o que causaria perda
// silenciosa de dados (DEK, tokens criptografados, etc.).
func TestMigration_CredentialColumnsMatchGORMModel(t *testing.T) {
	sqlDB := createOldSchemaDB(t)

	if err := migrateToUUIDv7(); err != nil {
		t.Fatalf("migração: %v", err)
	}

	// AutoMigrate deve rodar sem adicionar colunas novas se a migração estiver correta
	if err := db.AutoMigrate(&CredentialEntry{}, &CredentialKeyWrap{}); err != nil {
		t.Fatalf("AutoMigrate: %v", err)
	}

	// Verificar que credential_entries tem todas as colunas esperadas
	expectedCredCols := map[string]bool{
		"id": true, "pattern": true, "auth_type": true, "token_enc": true,
		"username": true, "password_enc": true, "headers_enc": true,
		"expires_at": true, "refresh_token_enc": true, "client_id_enc": true,
		"client_secret_enc": true, "created_at": true, "updated_at": true,
	}
	actualCredCols := getTableColumns(t, sqlDB, "credential_entries")
	for col := range expectedCredCols {
		if !actualCredCols[col] {
			t.Errorf("credential_entries: coluna %q ausente após migração", col)
		}
	}
	// Nenhuma coluna fantasma (que existia na migração errada mas não no modelo)
	phantomCols := []string{"name", "type", "api_key_enc", "client_id", "access_token_enc", "token_url", "token_expiry", "scopes", "extra_enc"}
	for _, col := range phantomCols {
		if actualCredCols[col] {
			t.Errorf("credential_entries: coluna fantasma %q presente — não existe no modelo GORM", col)
		}
	}

	// Verificar que credential_key_wraps tem todas as colunas esperadas
	expectedKWCols := map[string]bool{
		"id": true, "kind": true, "salt": true, "wrapped_dek": true,
		"argon_time": true, "argon_memory": true, "argon_threads": true,
		"created_at": true, "updated_at": true,
	}
	actualKWCols := getTableColumns(t, sqlDB, "credential_key_wraps")
	for col := range expectedKWCols {
		if !actualKWCols[col] {
			t.Errorf("credential_key_wraps: coluna %q ausente após migração", col)
		}
	}
	// Nenhuma coluna fantasma
	if actualKWCols["wrapped_key"] {
		t.Error("credential_key_wraps: coluna fantasma 'wrapped_key' presente — o campo correto é 'wrapped_dek'")
	}
}

// TestMigration_CredentialKeyWrapPreservesAllFields testa que TODOS os campos
// da credential_key_wraps são preservados — não apenas o ID.
// Este teste previne o bug onde a DEK era perdida na migração e o app
// pedia novamente a senha mestre.
func TestMigration_CredentialKeyWrapPreservesAllFields(t *testing.T) {
	sqlDB := createOldSchemaDB(t)

	mustExec(t, sqlDB, `INSERT INTO credential_key_wraps (id, kind, salt, wrapped_dek, argon_time, argon_memory, argon_threads, created_at, updated_at) VALUES (1, 'master', 'base64salt==', 'base64wrappeddek==', 3, 65536, 4, '2026-01-01', '2026-01-01')`)
	mustExec(t, sqlDB, `INSERT INTO credential_key_wraps (id, kind, salt, wrapped_dek, argon_time, argon_memory, argon_threads, created_at, updated_at) VALUES (2, 'recovery', 'recoverysalt', 'recoverydek', 1, 32768, 2, '2026-01-02', '2026-01-02')`)

	if err := migrateToUUIDv7(); err != nil {
		t.Fatalf("migração: %v", err)
	}

	// Verificar master key wrap
	var kind, salt, dek string
	var argonTime, argonMemory, argonThreads int
	if err := sqlDB.QueryRow("SELECT kind, salt, wrapped_dek, argon_time, argon_memory, argon_threads FROM credential_key_wraps WHERE kind = 'master'").Scan(&kind, &salt, &dek, &argonTime, &argonMemory, &argonThreads); err != nil {
		t.Fatalf("master key wrap perdida na migração: %v", err)
	}
	if salt != "base64salt==" {
		t.Errorf("master salt = %q, esperado 'base64salt=='", salt)
	}
	if dek != "base64wrappeddek==" {
		t.Errorf("master wrapped_dek = %q, esperado 'base64wrappeddek=='", dek)
	}
	if argonTime != 3 || argonMemory != 65536 || argonThreads != 4 {
		t.Errorf("master argon: time=%d memory=%d threads=%d, esperado 3/65536/4", argonTime, argonMemory, argonThreads)
	}

	// Verificar recovery key wrap
	var rKind, rSalt, rDek string
	if err := sqlDB.QueryRow("SELECT kind, salt, wrapped_dek FROM credential_key_wraps WHERE kind = 'recovery'").Scan(&rKind, &rSalt, &rDek); err != nil {
		t.Fatalf("recovery key wrap perdida na migração: %v", err)
	}
	if rSalt != "recoverysalt" || rDek != "recoverydek" {
		t.Errorf("recovery dados corrompidos: salt=%q dek=%q", rSalt, rDek)
	}

	// Ambos devem ter UUID
	var count int
	if err := sqlDB.QueryRow("SELECT count(*) FROM credential_key_wraps").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Errorf("esperado 2 key wraps, obtido %d", count)
	}
}

// TestMigration_CredentialTransportPreservesTokens testa o cenário end-to-end
// onde credenciais com token_enc são migradas e depois podem ser lidas pelo
// credential manager via GORM. Este é o caminho que falhava em produção: o
// CredentialTransport tentava ler token_enc mas o campo estava vazio.
func TestMigration_CredentialTransportPreservesTokens(t *testing.T) {
	sqlDB := createOldSchemaDB(t)

	// Simular credencial real: pattern de domínio + bearer token criptografado
	mustExec(t, sqlDB, `INSERT INTO credential_entries (id, pattern, auth_type, token_enc, created_at, updated_at) VALUES (1, 'api.openai.com', 'bearer', 'AES256_encrypted_sk_token_data', '2026-01-01', '2026-01-01')`)
	mustExec(t, sqlDB, `INSERT INTO credential_entries (id, pattern, auth_type, token_enc, created_at, updated_at) VALUES (2, 'llm.inclunet.com.br', 'bearer', 'AES256_encrypted_litellm_key', '2026-01-01', '2026-01-01')`)

	if err := migrateToUUIDv7(); err != nil {
		t.Fatalf("migração: %v", err)
	}

	// AutoMigrate (como o app real faz após migração)
	if err := db.AutoMigrate(&CredentialEntry{}); err != nil {
		t.Fatalf("AutoMigrate: %v", err)
	}

	// Ler via GORM como o credential manager faria
	var entries []CredentialEntry
	if err := db.Find(&entries).Error; err != nil {
		t.Fatalf("GORM Find: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("esperado 2 entries, obtido %d", len(entries))
	}

	found := make(map[string]CredentialEntry)
	for _, e := range entries {
		found[e.Pattern] = e
	}

	openai, ok := found["api.openai.com"]
	if !ok {
		t.Fatal("credencial api.openai.com não encontrada via GORM")
	}
	if openai.AuthType != "bearer" {
		t.Errorf("openai AuthType = %q, esperado 'bearer'", openai.AuthType)
	}
	if openai.TokenEnc != "AES256_encrypted_sk_token_data" {
		t.Errorf("openai TokenEnc = %q — token perdido na migração!", openai.TokenEnc)
	}

	litellm, ok := found["llm.inclunet.com.br"]
	if !ok {
		t.Fatal("credencial llm.inclunet.com.br não encontrada via GORM")
	}
	if litellm.TokenEnc != "AES256_encrypted_litellm_key" {
		t.Errorf("litellm TokenEnc = %q — token perdido na migração!", litellm.TokenEnc)
	}
}

// getTableColumns retorna um set com os nomes das colunas de uma tabela SQLite.
func getTableColumns(t *testing.T, sqlDB *sql.DB, table string) map[string]bool {
	t.Helper()
	rows, err := sqlDB.Query(fmt.Sprintf("PRAGMA table_info(%s)", table))
	if err != nil {
		t.Fatalf("PRAGMA table_info(%s): %v", table, err)
	}
	defer func() { _ = rows.Close() }()
	cols := make(map[string]bool)
	for rows.Next() {
		var cid int
		var name, colType string
		var notNull, pk int
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &colType, &notNull, &dflt, &pk); err != nil {
			t.Fatal(err)
		}
		cols[name] = true
	}
	return cols
}

// TestMigration_AllTablesMatchGORMModels verifica que TODAS as tabelas migradas
// têm as mesmas colunas que os modelos GORM esperam. Após migração + AutoMigrate,
// nenhuma coluna deveria estar ausente. Este teste genérico previne regressões
// como a dos credential_entries (onde colunas fantasma causaram perda de dados).
func TestMigration_AllTablesMatchGORMModels(t *testing.T) {
	sqlDB := createOldSchemaDB(t)

	// Inserir ao menos 1 registro em cada tabela para exercitar a cópia de dados
	mustExec(t, sqlDB, `INSERT INTO credential_entries (id, pattern, auth_type, token_enc, created_at, updated_at) VALUES (1, 'test.com', 'bearer', 'enc', '2026-01-01', '2026-01-01')`)
	mustExec(t, sqlDB, `INSERT INTO credential_key_wraps (id, kind, salt, wrapped_dek, argon_time, argon_memory, argon_threads, created_at, updated_at) VALUES (1, 'master', 's', 'dek', 1, 1, 1, '2026-01-01', '2026-01-01')`)
	mustExec(t, sqlDB, `INSERT INTO conversations (id, title, created_at, updated_at) VALUES (1, 'Conv', '2026-01-01', '2026-01-01')`)
	mustExec(t, sqlDB, `INSERT INTO chat_messages (id, conversation_id, role, content, created_at, updated_at) VALUES (1, 1, 'user', 'Hi', '2026-01-01', '2026-01-01')`)
	mustExec(t, sqlDB, `INSERT INTO task_lists (id, title, slug, created_at, updated_at) VALUES (1, 'TL', 'tl', '2026-01-01', '2026-01-01')`)
	mustExec(t, sqlDB, `INSERT INTO task_list_workflows (id, task_list_id, statuses, allowed_transitions, created_at, updated_at) VALUES (1, 1, '[]', '{}', '2026-01-01', '2026-01-01')`)
	mustExec(t, sqlDB, `INSERT INTO tasks (id, task_list_id, title, status_id, created_at, updated_at) VALUES (1, 1, 'Task', 1, '2026-01-01', '2026-01-01')`)
	mustExec(t, sqlDB, `INSERT INTO task_notes (id, task_id, type, content, external_source, external_id, created_at, updated_at) VALUES (1, 1, 1, 'Nota', 'jira', 'EXT-1', '2026-01-01', '2026-01-01')`)

	if err := migrateToUUIDv7(); err != nil {
		t.Fatalf("migração: %v", err)
	}

	// AutoMigrate com TODOS os modelos (como o app real faz)
	if err := db.AutoMigrate(
		&Conversation{}, &ChatMessage{},
		&CredentialEntry{}, &CredentialKeyWrap{},
		&TaskList{}, &TaskListWorkflow{}, &Task{}, &TaskNote{},
	); err != nil {
		t.Fatalf("AutoMigrate: %v", err)
	}

	// Para cada tabela, verificar que as colunas esperadas existem
	tests := []struct {
		table    string
		expected []string
	}{
		{"conversations", []string{"id", "title", "channel", "contact_id", "summary", "summary_up_to_message_id", "summarizing_in_progress", "created_at", "updated_at"}},
		{"chat_messages", []string{"id", "conversation_id", "parent_id", "turn_id", "role", "content", "reasoning", "media", "audio", "audio_mime_type", "tool_calls", "tool_call_id", "prompt_tokens", "completion_tokens", "total_tokens", "model", "source", "created_at"}},
		{"credential_entries", []string{"id", "pattern", "auth_type", "token_enc", "username", "password_enc", "headers_enc", "expires_at", "refresh_token_enc", "client_id_enc", "client_secret_enc", "created_at", "updated_at"}},
		{"credential_key_wraps", []string{"id", "kind", "salt", "wrapped_dek", "argon_time", "argon_memory", "argon_threads", "created_at", "updated_at"}},
		{"task_lists", []string{"id", "title", "slug", "description", "preferred_view_mode", "validation_policy", "created_at", "updated_at"}},
		{"task_list_workflows", []string{"id", "task_list_id", "statuses", "allowed_transitions", "initial_status_id", "created_at", "updated_at"}},
		{"tasks", []string{"id", "task_list_id", "title", "description", "code", "link", "status_id", "parent_id", "order", "assignee_name", "assignee_id", "creator_name", "creator_id", "due_date", "created_at", "updated_at", "completed_at"}},
		{"task_notes", []string{"id", "task_id", "type", "content", "author_name", "author_id", "external_source", "external_id", "external_parent_id", "external_updated_at", "created_at", "updated_at"}},
	}

	for _, tt := range tests {
		cols := getTableColumns(t, sqlDB, tt.table)
		for _, expected := range tt.expected {
			if !cols[expected] {
				t.Errorf("%s: coluna %q ausente após migração + AutoMigrate", tt.table, expected)
			}
		}
	}

	// Verificar que os dados foram preservados (leitura via GORM)
	var convCount int64
	db.Model(&Conversation{}).Count(&convCount)
	if convCount != 1 {
		t.Errorf("conversations: esperado 1, obtido %d", convCount)
	}

	var msgCount int64
	db.Model(&ChatMessage{}).Count(&msgCount)
	if msgCount != 1 {
		t.Errorf("chat_messages: esperado 1, obtido %d", msgCount)
	}

	var credCount int64
	db.Model(&CredentialEntry{}).Count(&credCount)
	if credCount != 1 {
		t.Errorf("credential_entries: esperado 1, obtido %d", credCount)
	}

	var kwCount int64
	db.Model(&CredentialKeyWrap{}).Count(&kwCount)
	if kwCount != 1 {
		t.Errorf("credential_key_wraps: esperado 1, obtido %d", kwCount)
	}

	var tlCount int64
	db.Model(&TaskList{}).Count(&tlCount)
	if tlCount != 1 {
		t.Errorf("task_lists: esperado 1, obtido %d", tlCount)
	}

	var wfCount int64
	db.Model(&TaskListWorkflow{}).Count(&wfCount)
	if wfCount != 1 {
		t.Errorf("task_list_workflows: esperado 1, obtido %d", wfCount)
	}

	var taskCount int64
	db.Model(&Task{}).Count(&taskCount)
	if taskCount != 1 {
		t.Errorf("tasks: esperado 1, obtido %d", taskCount)
	}

	var noteCount int64
	db.Model(&TaskNote{}).Count(&noteCount)
	if noteCount != 1 {
		t.Errorf("task_notes: esperado 1, obtido %d", noteCount)
	}

	// Verificar que external_source em task_notes foi preservado
	var note TaskNote
	if err := db.First(&note).Error; err != nil {
		t.Fatalf("leitura task_note: %v", err)
	}
	if note.ExternalSource != "jira" {
		t.Errorf("task_notes.external_source = %q, esperado 'jira' — coluna renomeada perdeu dados!", note.ExternalSource)
	}
	if note.ExternalID != "EXT-1" {
		t.Errorf("task_notes.external_id = %q, esperado 'EXT-1'", note.ExternalID)
	}
	if note.Content != "Nota" {
		t.Errorf("task_notes.content = %q, esperado 'Nota'", note.Content)
	}
}

// TestMigration_SummarizingInProgressNormalized testa que valores não-booleanos
// em summarizing_in_progress são normalizados para 0/1 durante a migração.
// Bug real: valor "4" causava "sql/driver: couldn't convert 4 into type bool".
func TestMigration_SummarizingInProgressNormalized(t *testing.T) {
	sqlDB := createOldSchemaDB(t)

	// Valor correto (0 = false)
	mustExec(t, sqlDB, `INSERT INTO conversations (id, title, summarizing_in_progress, created_at, updated_at) VALUES (1, 'Normal false', 0, '2026-01-01', '2026-01-01')`)
	// Valor correto (1 = true)
	mustExec(t, sqlDB, `INSERT INTO conversations (id, title, summarizing_in_progress, created_at, updated_at) VALUES (2, 'Normal true', 1, '2026-01-01', '2026-01-01')`)
	// Valor corrompido (4 — o caso real do bug)
	mustExec(t, sqlDB, `INSERT INTO conversations (id, title, summarizing_in_progress, created_at, updated_at) VALUES (3, 'Corrompido 4', 4, '2026-01-01', '2026-01-01')`)
	// Outro valor corrompido
	mustExec(t, sqlDB, `INSERT INTO conversations (id, title, summarizing_in_progress, created_at, updated_at) VALUES (4, 'Corrompido 99', 99, '2026-01-01', '2026-01-01')`)

	if err := migrateToUUIDv7(); err != nil {
		t.Fatalf("migração: %v", err)
	}

	// AutoMigrate para que GORM consiga ler
	if err := db.AutoMigrate(&Conversation{}); err != nil {
		t.Fatalf("AutoMigrate: %v", err)
	}

	// Ler TODAS as conversas via GORM — isso falhava com "couldn't convert 4 into type bool"
	var convs []Conversation
	if err := db.Order("title").Find(&convs).Error; err != nil {
		t.Fatalf("GORM Find falhou (provavelmente valor não-booleano): %v", err)
	}
	if len(convs) != 4 {
		t.Fatalf("esperado 4 conversas, obtido %d", len(convs))
	}

	// Verificar normalização
	expected := map[string]bool{
		"Normal false":  false,
		"Normal true":   true,
		"Corrompido 4":  true, // 4 > 0 → true
		"Corrompido 99": true, // 99 > 0 → true
	}
	for _, conv := range convs {
		exp, ok := expected[conv.Title]
		if !ok {
			t.Errorf("conversa inesperada: %q", conv.Title)
			continue
		}
		if conv.SummarizingInProgress != exp {
			t.Errorf("%q: summarizing_in_progress = %v, esperado %v", conv.Title, conv.SummarizingInProgress, exp)
		}
	}
}
