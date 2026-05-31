package database

import (
	"database/sql"
	"fmt"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

// setupChatMessageIndexTestDB inicializa um banco em memória, migra
// chat_messages (e dependências) e aplica os índices de chat_messages,
// reproduzindo o caminho do boot/AutoMigrate.
func setupChatMessageIndexTestDB(t *testing.T) *sql.DB {
	t.Helper()

	var err error
	db, err = gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(
		&User{},
		&Conversation{},
		&ChatMessage{},
	); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	ensureChatMessageWindowIndex()

	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("sql db: %v", err)
	}
	t.Cleanup(func() {
		_ = sqlDB.Close()
		db = nil
	})
	return sqlDB
}

// chatMessageIndexNames coleta os nomes dos índices existentes na tabela
// chat_messages via PRAGMA index_list.
func chatMessageIndexNames(t *testing.T, sqlDB *sql.DB) map[string]bool {
	t.Helper()
	rows, err := sqlDB.Query(`PRAGMA index_list(chat_messages)`)
	if err != nil {
		t.Fatalf("PRAGMA index_list(chat_messages): %v", err)
	}
	defer func() { _ = rows.Close() }()

	names := make(map[string]bool)
	for rows.Next() {
		// PRAGMA index_list: seq, name, unique, origin, partial
		var seq int
		var name string
		var unique int
		var origin string
		var partial int
		if err := rows.Scan(&seq, &name, &unique, &origin, &partial); err != nil {
			t.Fatalf("scan index_list: %v", err)
		}
		names[name] = true
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows err: %v", err)
	}
	return names
}

// TestChatMessageIndexesExist garante que os índices de ordenação/paginação de
// chat_messages (issue #20), incluindo os de created_at e updated_at, são
// criados após a migração.
func TestChatMessageIndexesExist(t *testing.T) {
	sqlDB := setupChatMessageIndexTestDB(t)

	got := chatMessageIndexNames(t, sqlDB)

	expected := []string{
		"idx_chat_messages_window",
		"idx_chat_messages_timeline_window",
		"idx_chat_messages_created_at",
		"idx_chat_messages_updated_at",
	}
	for _, name := range expected {
		if !got[name] {
			t.Errorf("índice esperado ausente em chat_messages: %s", name)
		}
	}
}

// TestChatMessageIndexCreationIsIdempotent garante que executar a criação de
// índices mais de uma vez (como pode ocorrer em reinicializações sucessivas)
// não gera erro e mantém os índices presentes.
func TestChatMessageIndexCreationIsIdempotent(t *testing.T) {
	sqlDB := setupChatMessageIndexTestDB(t)

	ensureChatMessageWindowIndex()
	ensureChatMessageWindowIndex()

	got := chatMessageIndexNames(t, sqlDB)
	for _, name := range []string{
		"idx_chat_messages_created_at",
		"idx_chat_messages_updated_at",
	} {
		if !got[name] {
			t.Errorf("índice ausente após reexecução idempotente: %s", name)
		}
	}
}

// indexColumns retorna as colunas (na ordem) de um índice via PRAGMA index_info.
func indexColumns(t *testing.T, sqlDB *sql.DB, index string) []string {
	t.Helper()
	rows, err := sqlDB.Query(fmt.Sprintf("PRAGMA index_info(%s)", index))
	if err != nil {
		t.Fatalf("PRAGMA index_info(%s): %v", index, err)
	}
	defer func() { _ = rows.Close() }()

	var cols []string
	for rows.Next() {
		// PRAGMA index_info: seqno, cid, name
		var seqno, cid int
		var name string
		if err := rows.Scan(&seqno, &cid, &name); err != nil {
			t.Fatalf("scan index_info: %v", err)
		}
		cols = append(cols, name)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows err: %v", err)
	}
	return cols
}

// TestChatMessageIndexColumns confirma que os índices de created_at e updated_at
// indexam as colunas esperadas, na ordem correta. O índice de created_at inclui
// `id` para resolver empates de created_at sem ordenação adicional (queries como
// GetAllConversationMessagesWithContext ordenam por created_at ASC, id ASC).
func TestChatMessageIndexColumns(t *testing.T) {
	sqlDB := setupChatMessageIndexTestDB(t)

	cases := []struct {
		index string
		want  []string
	}{
		{"idx_chat_messages_created_at", []string{"conversation_id", "created_at", "id"}},
		{"idx_chat_messages_updated_at", []string{"conversation_id", "updated_at"}},
	}
	for _, tc := range cases {
		got := indexColumns(t, sqlDB, tc.index)
		if fmt.Sprint(got) != fmt.Sprint(tc.want) {
			t.Errorf("colunas de %s = %v, esperado %v", tc.index, got, tc.want)
		}
	}
}
