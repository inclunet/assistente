package database

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"gorm.io/gorm"
)

func newChatLinksMigrationDB(t *testing.T) *gorm.DB {
	t.Helper()
	db := newMigratorTestDB(t)
	for _, statement := range []string{
		`PRAGMA foreign_keys=ON`,
		`CREATE TABLE conversations (id TEXT PRIMARY KEY, user_id TEXT NOT NULL)`,
		`CREATE TABLE chat_messages (id TEXT PRIMARY KEY, conversation_id TEXT REFERENCES conversations(id), turn_id TEXT)`,
		`CREATE TABLE tool_invocations (id TEXT PRIMARY KEY, user_id TEXT, origin_type TEXT, origin_id TEXT, conversation_id TEXT REFERENCES conversations(id), turn_id TEXT REFERENCES chat_messages(id), queued_at TEXT, input TEXT, output TEXT, metadata TEXT, updated_at TEXT)`,
		`INSERT INTO conversations VALUES ('conv','user'),('other','other-user')`,
		`INSERT INTO chat_messages VALUES ('turn','conv',NULL),('assistant','conv','turn'),('old-assistant','conv',NULL),('other-turn','other',NULL),('broken','conv','missing'),('cross-conv','conv','other-turn')`,
	} {
		if err := db.Exec(statement).Error; err != nil {
			t.Fatal(err)
		}
	}
	return db
}

func TestMigrateChatToolLinksPreservesRecordsAcrossBatches(t *testing.T) {
	db := newChatLinksMigrationDB(t)
	for i := 0; i < 513; i++ {
		origin := "assistant"
		if i%2 == 0 {
			origin = "old-assistant"
		}
		if err := db.Exec(`INSERT INTO tool_invocations (id,user_id,origin_type,origin_id,input,output,metadata,queued_at,updated_at) VALUES (?, 'user','chat',?, 'input','output','metadata','queued','updated')`, fmt.Sprintf("inv-%04d", i), origin).Error; err != nil {
			t.Fatal(err)
		}
	}
	for _, sql := range []string{
		`INSERT INTO tool_invocations(id,user_id,origin_type,origin_id,turn_id) VALUES ('partial-turn','user','chat','missing','turn')`,
		`INSERT INTO tool_invocations(id,user_id,origin_type,origin_id,conversation_id) VALUES ('partial-conv','user','chat','assistant','conv')`,
		`INSERT INTO tool_invocations(id,user_id,origin_type,origin_id,conversation_id,turn_id) VALUES ('canonical','user','chat','missing','conv','turn')`,
		`INSERT INTO tool_invocations(id,user_id,origin_type,origin_id) VALUES ('job','user','job','missing')`,
	} {
		if err := db.Exec(sql).Error; err != nil {
			t.Fatal(err)
		}
	}
	snapshot := func() []map[string]interface{} {
		var rows []map[string]interface{}
		if err := db.Raw(`SELECT id,user_id,origin_type,origin_id,input,output,metadata,queued_at,updated_at FROM tool_invocations ORDER BY id`).Scan(&rows).Error; err != nil {
			t.Fatal(err)
		}
		return rows
	}
	before := snapshot()
	for range 2 {
		if err := migrateChatToolLinks(db); err != nil {
			t.Fatal(err)
		}
	}
	if !reflect.DeepEqual(before, snapshot()) {
		t.Fatal("migração alterou conteúdo ou identidade")
	}
	var invalid int64
	if err := db.Raw(`SELECT count(*) FROM tool_invocations WHERE origin_type='chat' AND (conversation_id IS NOT 'conv' OR turn_id IS NOT CASE WHEN origin_id='old-assistant' THEN 'old-assistant' ELSE 'turn' END)`).Scan(&invalid).Error; err != nil {
		t.Fatal(err)
	}
	if invalid != 0 {
		t.Fatalf("vínculos incorretos: %d", invalid)
	}
	var job struct{ ConversationID, TurnID *string }
	if err := db.Raw(`SELECT conversation_id,turn_id FROM tool_invocations WHERE id='job'`).Scan(&job).Error; err != nil {
		t.Fatal(err)
	}
	if job.ConversationID != nil || job.TurnID != nil {
		t.Fatal("alterou origem não-chat")
	}
	var violations []map[string]interface{}
	if err := db.Raw(`PRAGMA foreign_key_check`).Scan(&violations).Error; err != nil {
		t.Fatal(err)
	}
	if len(violations) != 0 {
		t.Fatalf("FKs inválidas: %v", violations)
	}
}

func TestMigrateChatToolLinksRollsBackUnrecoverableRows(t *testing.T) {
	for name, values := range map[string]string{
		"missing-referenced-turn":      `'user','broken',NULL,NULL`,
		"cross-conversation-reference": `'user','cross-conv',NULL,NULL`,
		"missing-origin":               `'user','missing',NULL,NULL`,
		"wrong-owner":                  `'other-user','assistant',NULL,NULL`,
		"conflicting-conversation":     `'user','assistant','other',NULL`,
		"foreign-turn":                 `'user','assistant',NULL,'other-turn'`,
		"blank-owner":                  `'','assistant',NULL,NULL`,
	} {
		t.Run(name, func(t *testing.T) {
			db := newChatLinksMigrationDB(t)
			for i := 0; i < 257; i++ {
				if err := db.Exec(`INSERT INTO tool_invocations(id,user_id,origin_type,origin_id) VALUES (?,'user','chat','assistant')`, fmt.Sprintf("a-%04d", i)).Error; err != nil {
					t.Fatal(err)
				}
			}
			if err := db.Exec(`INSERT INTO tool_invocations(id,origin_type,user_id,origin_id,conversation_id,turn_id) VALUES ('z-invalid','chat',` + values + `)`).Error; err != nil {
				t.Fatal(err)
			}
			err := migrateChatToolLinks(db)
			if err == nil || !strings.Contains(err.Error(), "z-invalid") {
				t.Fatalf("esperado erro identificável, recebido %v", err)
			}
			var changed int64
			if err := db.Raw(`SELECT count(*) FROM tool_invocations WHERE id LIKE 'a-%' AND (conversation_id IS NOT NULL OR turn_id IS NOT NULL)`).Scan(&changed).Error; err != nil {
				t.Fatal(err)
			}
			if changed != 0 {
				t.Fatalf("rollback incompleto: %d", changed)
			}
			if db.Migrator().HasIndex("tool_invocations", "idx_tool_invocations_chat_turn") {
				t.Fatal("índice não reverteu com transação")
			}
		})
	}
}

func TestMigrateChatToolLinksIndexesTargetCanonicalScope(t *testing.T) {
	db := newChatLinksMigrationDB(t)
	if err := migrateChatToolLinks(db); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ sql, index, scope string }{
		{`SELECT id FROM tool_invocations WHERE user_id='user' AND origin_type='chat' AND conversation_id='conv'`, "idx_tool_invocations_chat_conversation", "conversation_id=?"},
		{`SELECT id FROM tool_invocations WHERE user_id='user' AND origin_type='chat' AND turn_id IN ('turn','old-assistant') ORDER BY turn_id,queued_at,id`, "idx_tool_invocations_chat_turn", "turn_id=?"},
	} {
		var plan []struct{ Detail string }
		if err := db.Raw("EXPLAIN QUERY PLAN " + tc.sql).Scan(&plan).Error; err != nil {
			t.Fatal(err)
		}
		found := false
		for _, row := range plan {
			if strings.Contains(row.Detail, tc.index) && strings.Contains(row.Detail, tc.scope) {
				found = true
			}
		}
		if !found {
			t.Fatalf("consulta não usa escopo indexado: %+v", plan)
		}
	}
}

func TestChatToolLinksMigrationRegisteredAndFailureNotRecorded(t *testing.T) {
	db := newChatLinksMigrationDB(t)
	var selected migration
	for _, m := range schemaMigrations {
		if m.Version == 39 {
			selected = m
		}
	}
	if selected.Run == nil || selected.Phase != phasePostAutoMigrate {
		t.Fatal("v39 ausente ou fase incorreta")
	}
	if err := db.Exec(`INSERT INTO tool_invocations(id,user_id,origin_type,origin_id) VALUES ('broken','user','chat','missing')`).Error; err != nil {
		t.Fatal(err)
	}
	if err := runMigrationList(db, phasePostAutoMigrate, []migration{selected}); err == nil {
		t.Fatal("migração inconsistente deve impedir boot")
	}
	if got := schemaMigrationRows(t, db); len(got) != 0 {
		t.Fatalf("migração incompleta registrada: %v", got)
	}
	if err := db.Exec(`PRAGMA foreign_keys=OFF`).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`UPDATE tool_invocations SET origin_id='assistant', conversation_id='', turn_id='' WHERE id='broken'`).Error; err != nil {
		t.Fatal(err)
	}
	if err := runMigrationList(db, phasePostAutoMigrate, []migration{selected}); err != nil {
		t.Fatal(err)
	}
	if got := schemaMigrationRows(t, db); len(got) != 1 || got[0] != 39 {
		t.Fatalf("migração concluída não registrada: %v", got)
	}
}
