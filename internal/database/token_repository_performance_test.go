package database

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

const legacyCanonicalModelCallCountSQL = `
	SELECT COUNT(*) FROM (
		SELECT origin_id,
		       COALESCE(
		         CASE WHEN json_valid(metadata)
		              THEN json_extract(metadata, '$.display.iteration')
		         END, 0
		       ) AS model_iteration
		  FROM tool_invocations
		 WHERE user_id = ?
		   AND conversation_id = ?
		   AND origin_type = 'chat'
		   AND trim(tool_call_id) <> ''
		   AND COALESCE(
		         CASE WHEN json_valid(metadata)
		              THEN json_extract(metadata, '$.external')
		         END, 0
		       ) = 0
		 GROUP BY origin_id, model_iteration
	)`

const materializedCanonicalModelCallCountSQL = `
	SELECT COUNT(*) FROM (
		SELECT origin_id, model_iteration
		  FROM tool_invocations
		 WHERE user_id = ?
		   AND conversation_id = ?
		   AND origin_type = 'chat'
		   AND trim(tool_call_id) <> ''
		   AND external = 0
		 GROUP BY origin_id, model_iteration
	)`

func newToolModelCallPerformanceDB(tb testing.TB) *gorm.DB {
	tb.Helper()
	database, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		tb.Fatal(err)
	}
	if err := database.Exec(`
		CREATE TABLE tool_invocations (
			id TEXT PRIMARY KEY,
			user_id TEXT NOT NULL,
			conversation_id TEXT,
			turn_id TEXT,
			origin_type TEXT NOT NULL,
			origin_id TEXT,
			tool_call_id TEXT,
			metadata TEXT,
			model_iteration INTEGER NOT NULL DEFAULT 0,
			external NUMERIC NOT NULL DEFAULT 0
		)`).Error; err != nil {
		tb.Fatal(err)
	}
	if err := migrateToolModelCallProjection(database); err != nil {
		tb.Fatal(err)
	}
	return database
}

func insertModelCallRows(tb testing.TB, database *gorm.DB, userID, conversationID string, modelCalls, toolsPerCall int) {
	tb.Helper()
	err := database.Transaction(func(tx *gorm.DB) error {
		for call := 0; call < modelCalls; call++ {
			originID := fmt.Sprintf("origin-%06d", call)
			iteration := call % 8
			for tool := 0; tool < toolsPerCall; tool++ {
				id := fmt.Sprintf("%s-inv-%06d-%02d", userID, call, tool)
				metadata := fmt.Sprintf(`{"display":{"iteration":%d,"name":"tool"},"padding":"%s"}`,
					iteration, strings.Repeat("x", 256))
				if err := tx.Exec(`
					INSERT INTO tool_invocations
						(id, user_id, conversation_id, turn_id, origin_type, origin_id,
						 tool_call_id, metadata, model_iteration, external)
					VALUES (?, ?, ?, ?, 'chat', ?, ?, ?, ?, 0)`,
					id, userID, conversationID, originID, originID,
					fmt.Sprintf("call-%d", tool), metadata, iteration,
				).Error; err != nil {
					return err
				}
			}
		}
		return nil
	})
	if err != nil {
		tb.Fatal(err)
	}
}

func rawModelCallCount(tb testing.TB, database *gorm.DB, query, userID, conversationID string) int {
	tb.Helper()
	var count int
	if err := database.Raw(query, userID, conversationID).Scan(&count).Error; err != nil {
		tb.Fatal(err)
	}
	return count
}

func TestToolModelCallProjectionMigrationBackfillsUpgradeFixtureIdempotently(t *testing.T) {
	database, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	if err := database.Exec(`
		CREATE TABLE tool_invocations (
			id TEXT PRIMARY KEY,
			user_id TEXT NOT NULL,
			conversation_id TEXT,
			turn_id TEXT,
			origin_type TEXT NOT NULL,
			origin_id TEXT,
			tool_call_id TEXT,
			metadata TEXT
		);
		INSERT INTO tool_invocations VALUES
			('local','user-a','conv-a','turn-a','chat','origin-a','call-a',
			 '{"display":{"iteration":7}}'),
			('external','user-a','conv-a','turn-a','chat','origin-b','call-b',
			 '{"external":true,"display":{"iteration":3}}'),
			('invalid','user-a','conv-a','turn-a','chat','origin-c','call-c',
			 '{invalid');
		ALTER TABLE tool_invocations ADD COLUMN model_iteration INTEGER NOT NULL DEFAULT 0;
		ALTER TABLE tool_invocations ADD COLUMN external NUMERIC NOT NULL DEFAULT 0;
	`).Error; err != nil {
		t.Fatal(err)
	}

	for pass := 1; pass <= 2; pass++ {
		if err := migrateToolModelCallProjection(database); err != nil {
			t.Fatalf("passagem %d: %v", pass, err)
		}
	}
	var rows []struct {
		ID             string
		ModelIteration int
		External       bool
	}
	if err := database.Raw(`
		SELECT id, model_iteration, external
		  FROM tool_invocations
		 ORDER BY id`).Scan(&rows).Error; err != nil {
		t.Fatal(err)
	}
	expected := map[string]struct {
		iteration int
		external  bool
	}{
		"local":    {iteration: 7},
		"external": {iteration: 3, external: true},
		"invalid":  {},
	}
	for _, row := range rows {
		want := expected[row.ID]
		if row.ModelIteration != want.iteration || row.External != want.external {
			t.Fatalf("%s materializado=(%d,%v), esperado=(%d,%v)",
				row.ID, row.ModelIteration, row.External, want.iteration, want.external)
		}
	}
	if got := queryCount(t, database, `
		SELECT COUNT(*) FROM pragma_index_list('tool_invocations')
		 WHERE name IN (?, ?)`,
		toolModelCallsConversationIndex, toolModelCallsTurnIndex,
	); got != 2 {
		t.Fatalf("índices materializados ausentes: %d", got)
	}
}

func TestCanonicalToolModelCallProjectionPreservesSemanticsAndOwnership(t *testing.T) {
	database := newToolModelCallPerformanceDB(t)
	rows := []string{
		`('a-1','user-a','conv-a','turn-a','chat','origin-a','call-1','{"display":{"iteration":0}}',0,0)`,
		`('a-2','user-a','conv-a','turn-a','chat','origin-a','call-2','{"display":{"iteration":0}}',0,0)`,
		`('a-3','user-a','conv-a','turn-a','chat','origin-a','call-3','{"display":{"iteration":1}}',1,0)`,
		`('a-4','user-a','conv-a','turn-b','chat','origin-b','call-4','{"display":{"iteration":0}}',0,0)`,
		`('a-external','user-a','conv-a','turn-a','chat','origin-external','call-5','{"external":true,"display":{"iteration":2}}',2,1)`,
		`('a-empty','user-a','conv-a','turn-a','chat','origin-empty','  ','{"display":{"iteration":3}}',3,0)`,
		`('b-1','user-b','conv-a','turn-a','chat','origin-b-user','call-1','{"display":{"iteration":0}}',0,0)`,
		`('other-conv','user-a','conv-other','turn-a','chat','origin-other','call-1','{"display":{"iteration":0}}',0,0)`,
	}
	for _, row := range rows {
		if err := database.Exec(`INSERT INTO tool_invocations
			(id,user_id,conversation_id,turn_id,origin_type,origin_id,tool_call_id,metadata,model_iteration,external)
			VALUES ` + row).Error; err != nil {
			t.Fatal(err)
		}
	}

	legacy := rawModelCallCount(t, database, legacyCanonicalModelCallCountSQL, "user-a", "conv-a")
	materialized := rawModelCallCount(t, database, materializedCanonicalModelCallCountSQL, "user-a", "conv-a")
	if legacy != 3 || materialized != legacy {
		t.Fatalf("paridade quebrada: legado=%d materializado=%d", legacy, materialized)
	}
	got, err := countCanonicalToolModelCalls(context.Background(), database, "user-a", "conv-a", "")
	if err != nil || got != legacy {
		t.Fatalf("contador canônico=%d, erro=%v, esperado=%d", got, err, legacy)
	}
	turnGot, err := countCanonicalToolModelCalls(context.Background(), database, "user-a", "conv-a", "turn-a")
	if err != nil || turnGot != 2 {
		t.Fatalf("contador do turno=%d, erro=%v, esperado=2", turnGot, err)
	}
	otherUser, err := countCanonicalToolModelCalls(context.Background(), database, "user-b", "conv-a", "")
	if err != nil || otherUser != 1 {
		t.Fatalf("isolamento multiusuário falhou: contador=%d erro=%v", otherUser, err)
	}
}

func TestCanonicalToolModelCallCountUsesMaterializedIndexes(t *testing.T) {
	database := newToolModelCallPerformanceDB(t)
	assertQueryPlanUsesIndex(t, database, `EXPLAIN QUERY PLAN
		SELECT origin_id, model_iteration
		  FROM tool_invocations
		 WHERE user_id = 'user-a'
		   AND conversation_id = 'conv-a'
		   AND origin_type = 'chat'
		   AND trim(tool_call_id) <> ''
		   AND external = 0
		 GROUP BY origin_id, model_iteration`,
		toolModelCallsConversationIndex,
	)
	assertQueryPlanUsesIndex(t, database, `EXPLAIN QUERY PLAN
		SELECT origin_id, model_iteration
		  FROM tool_invocations
		 WHERE user_id = 'user-a'
		   AND conversation_id = 'conv-a'
		   AND turn_id = 'turn-a'
		   AND origin_type = 'chat'
		   AND trim(tool_call_id) <> ''
		   AND external = 0
		 GROUP BY origin_id, model_iteration`,
		toolModelCallsTurnIndex,
	)
}

func TestCanonicalToolModelCallCountLargeConversationUnderBudget(t *testing.T) {
	if testing.Short() {
		t.Skip("teste de desempenho de conversa grande")
	}
	database := newToolModelCallPerformanceDB(t)
	insertModelCallRows(t, database, "user-large", "conv-large", 5_000, 4)

	const repetitions = 5
	start := time.Now()
	for range repetitions {
		if got := rawModelCallCount(t, database, materializedCanonicalModelCallCountSQL, "user-large", "conv-large"); got != 5_000 {
			t.Fatalf("contagem grande=%d, esperado=5000", got)
		}
	}
	average := time.Since(start) / repetitions
	if average >= 100*time.Millisecond {
		t.Fatalf("contagem materializada excedeu orçamento de 100ms: média=%s", average)
	}
	t.Logf("contagem materializada de 20.000 invocações: média=%s", average)
}

func BenchmarkCanonicalToolModelCallCount(b *testing.B) {
	database := newToolModelCallPerformanceDB(b)
	insertModelCallRows(b, database, "bench-user", "bench-conv", 5_000, 4)

	b.Run("legacy_json", func(b *testing.B) {
		for b.Loop() {
			if got := rawModelCallCount(b, database, legacyCanonicalModelCallCountSQL, "bench-user", "bench-conv"); got != 5_000 {
				b.Fatalf("contagem=%d", got)
			}
		}
	})
	b.Run("materialized_index", func(b *testing.B) {
		for b.Loop() {
			if got := rawModelCallCount(b, database, materializedCanonicalModelCallCountSQL, "bench-user", "bench-conv"); got != 5_000 {
				b.Fatalf("contagem=%d", got)
			}
		}
	})
}
